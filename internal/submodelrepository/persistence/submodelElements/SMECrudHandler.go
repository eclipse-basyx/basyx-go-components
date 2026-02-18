/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/
// Author: Jannik Fried ( Fraunhofer IESE )

// Package submodelelements provides handlers for CRUD operations on Submodel Elements in a PostgreSQL database.
//
// This package implements the base CRUD handler that manages common submodel element operations
// including creation, path management, and position tracking within hierarchical structures.
package submodelelements

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLSMECrudHandler provides base CRUD operations for submodel elements in PostgreSQL.
//
// This handler implements common functionality for all submodel element types, including
// database ID management, path tracking, position management, and semantic ID handling.
// Type-specific handlers extend this base functionality for specialized element types.
//
// The handler operates within transaction contexts to ensure atomicity of operations
// and maintains hierarchical relationships through parent-child linkage and path tracking.
type PostgreSQLSMECrudHandler struct {
	Db *sql.DB
}

// isEmptyReference checks if a Reference is empty (zero value).
//
// This utility function determines whether a Reference pointer is nil or contains
// only zero values, which is useful for determining if optional semantic IDs should
// be persisted to the database.
//
// Parameters:
//   - ref: Reference pointer to check
//
// Returns:
//   - bool: true if the reference is nil or contains only zero values, false otherwise
func isEmptyReference(ref types.IReference) bool {
	if ref == nil {
		return true
	}
	return reflect.DeepEqual(ref, types.Reference{})
}

// toIClassSlice converts a slice of any IClass-implementing type to []types.IClass.
func toIClassSlice[T types.IClass](items []T) []types.IClass {
	result := make([]types.IClass, len(items))
	for i, item := range items {
		result[i] = item
	}
	return result
}

// serializeIClassSliceToJSON converts a slice of IClass instances to a JSON array string.
// Each element is first converted to a jsonable map via jsonization.ToJsonable,
// then the resulting slice is marshalled to JSON.
func serializeIClassSliceToJSON(items []types.IClass, errCode string) (string, error) {
	if len(items) == 0 {
		return "[]", nil
	}
	toJSON := make([]map[string]any, 0, len(items))
	for _, item := range items {
		jsonObj, err := jsonization.ToJsonable(item)
		if err != nil {
			return "", common.NewErrBadRequest("Failed to convert object to jsonable - no changes applied - " + errCode)
		}
		toJSON = append(toJSON, jsonObj)
	}
	resBytes, err := json.Marshal(toJSON)
	if err != nil {
		return "", err
	}
	return string(resBytes), nil
}

// NewPostgreSQLSMECrudHandler creates a new PostgreSQL submodel element CRUD handler.
//
// This constructor initializes a handler with a database connection that will be used
// for all database operations. The handler can then perform CRUD operations on submodel
// elements within transaction contexts.
//
// Parameters:
//   - db: PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLSMECrudHandler: Initialized handler ready for CRUD operations
//   - error: Always nil in current implementation, kept for interface consistency
func NewPostgreSQLSMECrudHandler(db *sql.DB) (*PostgreSQLSMECrudHandler, error) {
	return &PostgreSQLSMECrudHandler{Db: db}, nil
}

// Update updates an existing SubmodelElement identified by its idShort or path.
//
// This method updates the mutable properties of an existing submodel element within
// a transaction context. It preserves the element's identity (idShort, path, parent,
// position, model type) while allowing updates to metadata fields.
//
// Updated fields include:
//   - category: Element category classification
//   - semanticId: Reference to semantic definition
//   - description: Localized descriptions
//   - displayName: Localized display names
//   - qualifiers: Qualifier constraints
//   - embeddedDataSpecifications: Embedded data specifications
//   - supplementalSemanticIds: Additional semantic references
//   - extensions: Custom extensions
//
// Immutable fields (not updated):
//   - idShort: Element identifier
//   - idShortPath: Hierarchical path
//   - parent_sme_id: Parent element reference
//   - position: Position in parent
//   - model_type: Element type
//   - submodel_id: Parent submodel
//
// Parameters:
//   - submodelID: ID of the parent submodel (used for validation)
//   - idShortOrPath: The idShort or full path of the element to update
//   - submodelElement: The updated element data
//   - tx: Active transaction context for atomic operations
//
// Returns:
//   - error: An error if the element is not found, validation fails, or update fails
//
// Example:
//
//	err := handler.Update(tx, "submodel123", "sensors.temperature", updatedProperty)
//
//nolint:revive // cyclomatic-complexity is acceptable here due to the multiple update steps
func (p *PostgreSQLSMECrudHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	// Handle transaction creation if tx is nil
	var localTx *sql.Tx
	var err error
	needsCommit := false

	if tx == nil {
		localTx, err = p.Db.Begin()
		if err != nil {
			return err
		}
		needsCommit = true
		defer func() {
			if needsCommit {
				if r := recover(); r != nil {
					_ = localTx.Rollback()
					panic(r)
				}
				if err != nil {
					_ = localTx.Rollback()
				}
			}
		}()
	} else {
		localTx = tx
	}

	dialect := goqu.Dialect("postgres")
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(localTx, submodelID)
	if err != nil {
		_, _ = fmt.Println("SMREPO-SMEUPD-GETSMDATABASEID " + err.Error())
		return common.NewInternalServerError("Failed to resolve Submodel database id - see console for details")
	}

	// First, get the existing element ID and verify it exists in the correct submodel
	var existingID int
	var existingIDShort sql.NullString

	selectQuery := dialect.From(goqu.T("submodel_element")).
		Select(goqu.C("id"), goqu.C("id_short")).
		Where(
			goqu.C("idshort_path").Eq(idShortOrPath),
			goqu.C("submodel_id").Eq(submodelDatabaseID),
		)

	selectSQL, selectArgs, err := selectQuery.ToSQL()
	if err != nil {
		return err
	}

	err = localTx.QueryRow(selectSQL, selectArgs...).Scan(&existingID, &existingIDShort)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("SubmodelElement with path '" + idShortOrPath + "' not found in submodel '" + submodelID + "'")
		}
		return err
	}

	if isPut && submodelElement.IDShort() != nil {
		newIDShort := strings.TrimSpace(*submodelElement.IDShort())
		if newIDShort != "" && (!existingIDShort.Valid || existingIDShort.String != newIDShort) {
			_, updatePathErr := p.UpdateIdShortPaths(localTx, submodelID, idShortOrPath, newIDShort)
			if updatePathErr != nil {
				return updatePathErr
			}
		}
	}

	// Update the base submodel_element row
	updateQuery := dialect.Update(goqu.T("submodel_element")).
		Set(goqu.Record{
			"category": submodelElement.Category(),
		}).
		Where(goqu.C("id").Eq(existingID))

	updateSQL, updateArgs, err := updateQuery.ToSQL()
	if err != nil {
		return err
	}

	_, err = localTx.Exec(updateSQL, updateArgs...)
	if err != nil {
		return err
	}

	// Ensure payload row exists
	ensurePayloadQuery, ensurePayloadArgs, err := dialect.Insert("submodel_element_payload").
		Rows(goqu.Record{"submodel_element_id": existingID}).
		OnConflict(goqu.DoNothing()).
		ToSQL()
	if err != nil {
		return err
	}
	_, err = localTx.Exec(ensurePayloadQuery, ensurePayloadArgs...)
	if err != nil {
		return err
	}

	payloadUpdateRecord := goqu.Record{}
	if isPut || submodelElement.Description() != nil {
		descriptionJSONString, serErr := serializeIClassSliceToJSON(toIClassSlice(submodelElement.Description()), "SMREPO-SME-UPDATE-DESCJSONIZATION")
		if serErr != nil {
			return serErr
		}
		payloadUpdateRecord["description_payload"] = goqu.L("?::jsonb", descriptionJSONString)
	}
	if isPut || submodelElement.DisplayName() != nil {
		displayNameJSONString, serErr := serializeIClassSliceToJSON(toIClassSlice(submodelElement.DisplayName()), "SMREPO-SME-UPDATE-DISPNAMEJSONIZATION")
		if serErr != nil {
			return serErr
		}
		payloadUpdateRecord["displayname_payload"] = goqu.L("?::jsonb", displayNameJSONString)
	}
	if isPut {
		payloadUpdateRecord["administrative_information_payload"] = goqu.L("?::jsonb", "[]")
	}
	if isPut || submodelElement.EmbeddedDataSpecifications() != nil {
		edsJSONString, serErr := serializeIClassSliceToJSON(toIClassSlice(submodelElement.EmbeddedDataSpecifications()), "SMREPO-SME-UPDATE-EDSJSONIZATION")
		if serErr != nil {
			return serErr
		}
		payloadUpdateRecord["embedded_data_specification_payload"] = goqu.L("?::jsonb", edsJSONString)
	}
	if isPut || submodelElement.SupplementalSemanticIDs() != nil {
		supplementalSemanticIDsJSONString, serErr := serializeIClassSliceToJSON(toIClassSlice(submodelElement.SupplementalSemanticIDs()), "SMREPO-SME-UPDATE-SUPPLJSONIZATION")
		if serErr != nil {
			return serErr
		}
		payloadUpdateRecord["supplemental_semantic_ids_payload"] = goqu.L("?::jsonb", supplementalSemanticIDsJSONString)
	}
	if isPut || submodelElement.Extensions() != nil {
		extensionsJSONString, serErr := serializeIClassSliceToJSON(toIClassSlice(submodelElement.Extensions()), "SMREPO-SME-UPDATE-EXTJSONIZATION")
		if serErr != nil {
			return serErr
		}
		payloadUpdateRecord["extensions_payload"] = goqu.L("?::jsonb", extensionsJSONString)
	}
	if isPut || submodelElement.Qualifiers() != nil {
		qualifiersJSONString, serErr := serializeIClassSliceToJSON(toIClassSlice(submodelElement.Qualifiers()), "SMREPO-SME-UPDATE-QUALJSONIZATION")
		if serErr != nil {
			return serErr
		}
		payloadUpdateRecord["qualifiers_payload"] = goqu.L("?::jsonb", qualifiersJSONString)
	}

	if len(payloadUpdateRecord) > 0 {
		payloadUpdateQuery, payloadUpdateArgs, payloadUpdateErr := dialect.Update("submodel_element_payload").
			Set(payloadUpdateRecord).
			Where(goqu.C("submodel_element_id").Eq(existingID)).
			ToSQL()
		if payloadUpdateErr != nil {
			return payloadUpdateErr
		}
		_, err = localTx.Exec(payloadUpdateQuery, payloadUpdateArgs...)
		if err != nil {
			return err
		}
	}

	semanticID := submodelElement.SemanticID()
	if isPut || semanticID != nil {
		if semanticID != nil && !isEmptyReference(semanticID) {
			_, err = CreateContextReferenceByOwnerID(localTx, int64(existingID), "submodel_element_semantic_id", semanticID)
			if err != nil {
				_, _ = fmt.Println("SMREPO-SMEUPD-CRSEMREF " + err.Error())
				return common.NewInternalServerError("Failed to update SemanticID - see console for details")
			}
		} else {
			deleteSemanticRefQuery, deleteSemanticRefArgs, deleteSemanticRefErr := dialect.Delete("submodel_element_semantic_id_reference").
				Where(goqu.C("id").Eq(existingID)).
				ToSQL()
			if deleteSemanticRefErr != nil {
				return deleteSemanticRefErr
			}
			_, err = localTx.Exec(deleteSemanticRefQuery, deleteSemanticRefArgs...)
			if err != nil {
				return err
			}
		}
	}

	// Commit transaction if we created it
	if needsCommit {
		err = localTx.Commit()
		if err != nil {
			return err
		}
		needsCommit = false
	}

	return nil
}

// Delete removes a SubmodelElement identified by its idShort or path.
//
// This method is currently a placeholder for future implementation of element deletion.
// When implemented, it should handle cascading deletion of child elements and cleanup
// of related data such as semantic IDs and type-specific data.
//
// Parameters:
//   - idShortOrPath: The idShort or full path of the element to delete
//
// Returns:
//   - error: Currently always returns nil (not yet implemented)
//
// nolint:revive
func (p *PostgreSQLSMECrudHandler) Delete(idShortOrPath string) error {
	return nil
}

// GetDatabaseID retrieves the database primary key ID for an element by its path.
//
// This method looks up the internal database ID for a submodel element using its
// idShort path. The database ID is needed for operations that create child elements
// or establish relationships between elements.
//
// Parameters:
//   - idShortPath: The full idShort path of the element (e.g., "collection.property")
//
// Returns:
//   - int: The database primary key ID of the element
//   - error: An error if the query fails or element is not found
//
// Example:
//
//	dbID, err := handler.GetDatabaseID("sensors.temperature")
func (p *PostgreSQLSMECrudHandler) GetDatabaseID(submodelID int, idShortPath string) (int, error) {
	return p.GetDatabaseIDWithTx(nil, submodelID, idShortPath)
}

// GetDatabaseIDWithTx retrieves the database primary key ID for an element by its path,
// optionally using the provided transaction for consistent reads within a larger operation.
//
// If tx is nil, the handler's default database connection is used.
func (p *PostgreSQLSMECrudHandler) GetDatabaseIDWithTx(tx *sql.Tx, submodelID int, idShortPath string) (int, error) {
	dialect := goqu.Dialect("postgres")
	selectQuery, selectArgs, err := dialect.From("submodel_element").
		Select("id").
		Where(goqu.And(
			goqu.C("idshort_path").Eq(idShortPath),
			goqu.C("submodel_id").Eq(submodelID),
		)).
		ToSQL()
	if err != nil {
		return 0, err
	}

	var id int
	if tx != nil {
		err = tx.QueryRow(selectQuery, selectArgs...).Scan(&id)
	} else {
		err = p.Db.QueryRow(selectQuery, selectArgs...).Scan(&id)
	}
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, common.NewErrNotFound("SubmodelElement with path '" + idShortPath + "' not found in submodel '" + strconv.Itoa(submodelID) + "'")
		}
		return 0, err
	}
	return id, nil
}

// GetRootSmeIDByElementID resolves the top-level root element ID for a submodel element.
// If root_sme_id is NULL, the element is itself a root and its own ID is returned.
func (p *PostgreSQLSMECrudHandler) GetRootSmeIDByElementID(elementID int) (int, error) {
	dialect := goqu.Dialect("postgres")
	selectQuery, selectArgs, err := dialect.From("submodel_element").
		Select(goqu.COALESCE(goqu.C("root_sme_id"), goqu.C("id"))).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return 0, err
	}

	var rootID int
	err = p.Db.QueryRow(selectQuery, selectArgs...).Scan(&rootID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, common.NewErrNotFound("SubmodelElement with ID '" + strconv.Itoa(elementID) + "' not found")
		}
		return 0, err
	}

	return rootID, nil
}

// GetNextPosition determines the next available position index for a child element.
//
// This method calculates the next position value to use when adding a new child
// element to a parent (SubmodelElementCollection or SubmodelElementList). It finds
// the maximum current position among existing children and returns the next value.
//
// The position is used for:
//   - Maintaining order in SubmodelElementList elements
//   - Providing consistent ordering for SubmodelElementCollection children
//   - Supporting index-based access (e.g., "list[2]")
//
// Parameters:
//   - parentID: Database ID of the parent element
//
// Returns:
//   - int: The next position value (0 if no children exist, max+1 otherwise)
//   - error: An error if the query fails
//
// Example:
//
//	nextPos, err := handler.GetNextPosition(parentDbID)
//	// Use nextPos when creating the next child element
func (p *PostgreSQLSMECrudHandler) GetNextPosition(parentID int) (int, error) {
	dialect := goqu.Dialect("postgres")
	selectQuery, selectArgs, err := dialect.From("submodel_element").
		Select(goqu.MAX("position")).
		Where(goqu.C("parent_sme_id").Eq(parentID)).
		ToSQL()
	if err != nil {
		return 0, err
	}

	var position sql.NullInt64
	err = p.Db.QueryRow(selectQuery, selectArgs...).Scan(&position)
	if err != nil {
		return 0, err
	}
	if position.Valid {
		return int(position.Int64) + 1, nil
	}
	return 0, nil // If no children exist, start at position 0
}

// GetSubmodelElementType retrieves the model type of an element by its path.
//
// This method looks up the model type (e.g., "Property", "SubmodelElementCollection",
// "Blob") for a submodel element using its idShort path. The model type is used to
// determine which type-specific handler to use for operations on the element.
//
// Parameters:
//   - idShortPath: The full idShort path of the element
//
// Returns:
//   - string: The model type string (e.g., "Property", "File", "Range")
//   - error: An error if the query fails or element is not found
//
// Example:
//
//	modelType, err := handler.GetSubmodelElementType("sensors.temperature")
//	// Use modelType to get the appropriate handler via GetSMEHandlerByModelType
func (p *PostgreSQLSMECrudHandler) GetSubmodelElementType(idShortPath string) (*types.ModelType, error) {
	dialect := goqu.Dialect("postgres")
	selectQuery, selectArgs, err := dialect.From("submodel_element").
		Select("model_type").
		Where(goqu.C("idshort_path").Eq(idShortPath)).
		ToSQL()
	if err != nil {
		return nil, err
	}

	var modelType int64
	err = p.Db.QueryRow(selectQuery, selectArgs...).Scan(&modelType)
	if err != nil {
		return nil, err
	}
	modelTypeInstance := types.ModelType(modelType)
	return &modelTypeInstance, nil
}

// UpdateIdShortPaths updates the idShort and idShortPath of a submodel element and all its descendants
// when the element's idShort changes during a PUT operation.
//
// This method computes the new path by replacing the last segment of the old path with the new idShort,
// then cascades the path change to all child elements. It uses three precise LIKE patterns
// (exact match, dot-children, bracket-children) to avoid accidentally updating elements
// whose idShort merely starts with the same prefix.
//
// Parameters:
//   - tx: Active transaction for atomic operations
//   - submodelID: ID of the parent submodel
//   - oldPath: The current full idShortPath of the element being updated
//   - newIDShort: The new idShort value from the PUT body
//
// Returns:
//   - string: The new full idShortPath after the update
//   - error: An error if a conflict is detected or the update fails
func (p *PostgreSQLSMECrudHandler) UpdateIdShortPaths(tx *sql.Tx, submodelID string, oldPath string, newIDShort string) (string, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", common.NewErrNotFound("SMREPO-UPDPATH-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return "", common.NewInternalServerError("SMREPO-UPDPATH-GETSMDATABASEID " + err.Error())
	}

	if strings.HasSuffix(oldPath, "]") {
		dialect := goqu.Dialect("postgres")
		updateListItemQuery, updateListItemArgs, err := dialect.Update("submodel_element").
			Set(goqu.Record{
				"id_short": newIDShort,
			}).
			Where(
				goqu.C("submodel_id").Eq(submodelDatabaseID),
				goqu.C("idshort_path").Eq(oldPath),
			).
			ToSQL()
		if err != nil {
			return "", common.NewInternalServerError("SMREPO-UPDPATH-LIST-TOSQL " + err.Error())
		}

		_, err = tx.Exec(updateListItemQuery, updateListItemArgs...)
		if err != nil {
			return "", common.NewInternalServerError("SMREPO-UPDPATH-LIST-EXEC " + err.Error())
		}

		return oldPath, nil
	}

	// Compute the new path by replacing the last segment of oldPath
	newPath := computeNewPath(oldPath, newIDShort)

	if newPath == oldPath {
		return oldPath, nil
	}

	// Check for idShort conflict: does an element with the new path already exist?
	dialect := goqu.Dialect("postgres")

	conflictQuery, conflictArgs, err := dialect.From("submodel_element").
		Select(goqu.COUNT("id")).
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("idshort_path").Eq(newPath),
		).
		ToSQL()
	if err != nil {
		return "", common.NewInternalServerError("SMREPO-UPDPATH-CONFLICT-TOSQL " + err.Error())
	}

	var count int
	err = tx.QueryRow(conflictQuery, conflictArgs...).Scan(&count)
	if err != nil {
		return "", common.NewInternalServerError("SMREPO-UPDPATH-CONFLICT-EXEC " + err.Error())
	}

	if count > 0 {
		return "", common.NewErrConflict("SMREPO-UPDPATH-CONFLICT SubmodelElement with idShortPath '" + newPath + "' already exists in submodel '" + submodelID + "'")
	}

	// Update the element itself: set both id_short and idshort_path
	updateSelfQuery, updateSelfArgs, err := dialect.Update("submodel_element").
		Set(goqu.Record{
			"id_short":     newIDShort,
			"idshort_path": newPath,
		}).
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("idshort_path").Eq(oldPath),
		).
		ToSQL()
	if err != nil {
		return "", common.NewInternalServerError("SMREPO-UPDPATH-SELF-TOSQL " + err.Error())
	}

	_, err = tx.Exec(updateSelfQuery, updateSelfArgs...)
	if err != nil {
		return "", common.NewInternalServerError("SMREPO-UPDPATH-SELF-EXEC " + err.Error())
	}

	// Update children whose path starts with oldPath followed by "." (collection/entity children)
	err = updateChildPaths(tx, dialect, submodelDatabaseID, oldPath, newPath, ".")
	if err != nil {
		return "", err
	}

	// Update children whose path starts with oldPath followed by "[" (list children)
	err = updateChildPaths(tx, dialect, submodelDatabaseID, oldPath, newPath, "[")
	if err != nil {
		return "", err
	}

	return newPath, nil
}

// computeNewPath replaces the last segment of an idShortPath with a new idShort.
//
// Path patterns:
//   - "propName"          → last segment is "propName" (top-level)
//   - "parent.child"      → last segment is "child" (dot-separated)
//   - "parent[0].child"   → last segment is "child" (dot-separated after bracket)
//
// Note: If the path ends with a bracket index (e.g., "list[2]"), the path is
// position-based and the idShort replacement changes the part before the bracket suffix.
func computeNewPath(oldPath string, newIDShort string) string {
	// Find the last dot separator
	lastDot := strings.LastIndex(oldPath, ".")
	if lastDot >= 0 {
		return oldPath[:lastDot+1] + newIDShort
	}

	// No dot found — this is a top-level element
	return newIDShort
}

func resolveUpdatedPath(idShortOrPath string, submodelElement types.ISubmodelElement, isPut bool) string {
	if !isPut || submodelElement == nil || submodelElement.IDShort() == nil {
		return idShortOrPath
	}

	if strings.HasSuffix(idShortOrPath, "]") {
		return idShortOrPath
	}

	newIDShort := strings.TrimSpace(*submodelElement.IDShort())
	if newIDShort == "" {
		return idShortOrPath
	}

	return computeNewPath(idShortOrPath, newIDShort)
}

// updateChildPaths updates the idshort_path of child elements whose paths start with
// the old prefix followed by the given separator ("." or "[").
//
// It uses PostgreSQL's OVERLAY function to replace the old prefix portion with the new prefix,
// ensuring only the exact prefix is replaced without affecting similar-prefix siblings.
func updateChildPaths(tx *sql.Tx, dialect goqu.DialectWrapper, submodelDatabaseID int, oldPath string, newPath string, separator string) error {
	likePattern := oldPath + separator + "%"
	oldPrefixLen := len(oldPath)

	// SET idshort_path = newPath || SUBSTRING(idshort_path FROM oldPrefixLen+1)
	// This replaces the old prefix with the new prefix, preserving the rest of the path.
	updateExpr := goqu.L(
		"? || SUBSTRING(idshort_path FROM ?)",
		newPath, oldPrefixLen+1,
	)

	updateQuery, updateArgs, err := dialect.Update("submodel_element").
		Set(goqu.Record{
			"idshort_path": updateExpr,
		}).
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("idshort_path").Like(likePattern),
		).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-UPDPATH-CHILDREN-TOSQL " + err.Error())
	}

	_, err = tx.Exec(updateQuery, updateArgs...)
	if err != nil {
		return common.NewInternalServerError("SMREPO-UPDPATH-CHILDREN-EXEC " + err.Error())
	}

	return nil
}
