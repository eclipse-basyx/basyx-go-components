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
	"fmt"
	"reflect"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	jsoniter "github.com/json-iterator/go"
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

// CreateWithPath performs base SubmodelElement creation with explicit path and position management.
//
// This method creates a new submodel element within an existing transaction context,
// handling parent-child relationships, position ordering, and full path tracking. It's
// used when creating elements within hierarchical structures like SubmodelElementCollection
// or SubmodelElementList where explicit path and position control is required.
//
// The method:
//   - Creates the semantic ID reference if provided
//   - Validates that no element with the same path already exists
//   - Inserts the element with specified parent, position, and path
//   - Returns the database ID for use in type-specific operations
//
// Parameters:
//   - tx: Active transaction context for atomic operations
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent element (0 for root elements)
//   - idShortPath: Full path from root (e.g., "collection1.property2" or "list[0]")
//   - submodelElement: The submodel element to create
//   - position: Position index within parent (used for ordering in lists/collections)
//
// Returns:
//   - int: Database ID of the newly created element
//   - error: An error if semantic ID creation fails, element already exists, or insertion fails
//
// Example:
//
//	id, err := handler.CreateWithPath(tx, "submodel123", parentDbID, "sensors.temperature", tempProp, 0)
//
//nolint:revive // cyclomatic-complexity is acceptable here due to the multiple steps involved in creation
func (p *PostgreSQLSMECrudHandler) CreateWithPath(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement types.ISubmodelElement, position int, rootSubmodelElementID int) (int, error) {
	var referenceID sql.NullInt64
	var err error
	if submodelElement.SemanticID() != nil {
		referenceID, err = persistenceutils.CreateReference(tx, submodelElement.SemanticID(), sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			return 0, err
		}
	}

	descriptionID, err := persistenceutils.CreateLangStringTextTypes(tx, submodelElement.Description())
	if err != nil {
		_, _ = fmt.Println(err)
		return 0, common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}

	displayNameID, err := persistenceutils.CreateLangStringNameTypes(tx, submodelElement.DisplayName())
	if err != nil {
		_, _ = fmt.Println(err)
		return 0, common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}

	var parentDBId sql.NullInt64
	if parentID == 0 {
		parentDBId = sql.NullInt64{}
	} else {
		parentDBId = sql.NullInt64{Int64: int64(parentID), Valid: true}
	}

	var rootDbID sql.NullInt64
	if rootSubmodelElementID == 0 {
		rootDbID = sql.NullInt64{}
	} else {
		rootDbID = sql.NullInt64{Int64: int64(rootSubmodelElementID), Valid: true}
	}

	edsJSONString := "[]"
	eds := submodelElement.EmbeddedDataSpecifications()
	if len(eds) > 0 {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		edsBytes, err := json.Marshal(eds)
		if err != nil {
			return 0, err
		}
		edsJSONString = string(edsBytes)
	}

	supplementalSemanticIDsJSONString := "[]"
	supplementalSemanticIDs := submodelElement.SupplementalSemanticIDs()
	if len(supplementalSemanticIDs) > 0 {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		supplementalSemanticIDsBytes, err := json.Marshal(supplementalSemanticIDs)
		if err != nil {
			return 0, err
		}
		supplementalSemanticIDsJSONString = string(supplementalSemanticIDsBytes)
	}

	extensionsJSONString := "[]"
	extension := submodelElement.Extensions()
	if len(extension) > 0 {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		extensionsBytes, err := json.Marshal(extension)
		if err != nil {
			return 0, err
		}
		extensionsJSONString = string(extensionsBytes)
	}

	var id int
	err = tx.QueryRow(`	INSERT INTO
	 					submodel_element(submodel_id, parent_sme_id, position, id_short, category, model_type, semantic_id, idshort_path, description_id, displayname_id, root_sme_id, embedded_data_specification, supplemental_semantic_ids, extensions)
						VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING id`,
		submodelID,
		parentDBId,
		position,
		submodelElement.IDShort(),
		submodelElement.Category(),
		submodelElement.ModelType(),
		referenceID, // This will be NULL if no semantic ID was provided
		idShortPath, // Use the provided idShortPath instead of just GetIdShort()
		descriptionID,
		displayNameID,
		rootDbID,
		edsJSONString,
		supplementalSemanticIDsJSONString,
		extensionsJSONString,
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	// For root elements, set root_sme_id to their own ID
	if rootSubmodelElementID == 0 {
		dialect := goqu.Dialect("postgres")
		updateQuery, updateArgs, err := dialect.Update("submodel_element").
			Set(goqu.Record{"root_sme_id": id}).
			Where(goqu.C("id").Eq(id)).
			ToSQL()
		if err != nil {
			return 0, err
		}
		_, err = tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			return 0, err
		}
	}

	qualifiers := submodelElement.Qualifiers()
	if len(qualifiers) > 0 {
		for i, qualifier := range qualifiers {
			qualifierID, err := persistenceutils.CreateQualifier(tx, qualifier, i)
			if err != nil {
				return 0, err
			}

			dialect := goqu.Dialect("postgres")
			insertQuery, insertArgs, err := dialect.Insert("submodel_element_qualifier").
				Rows(goqu.Record{
					"sme_id":       id,
					"qualifier_id": qualifierID,
				}).
				ToSQL()
			if err != nil {
				return 0, err
			}
			_, err = tx.Exec(insertQuery, insertArgs...)
			if err != nil {
				_, _ = fmt.Println(err)
				return 0, common.NewInternalServerError("Failed to Create Qualifier for Submodel Element with ID '" + fmt.Sprintf("%d", id) + "'. See console for details.")
			}
		}
	}

	// println("Inserted SubmodelElement with idShort: " + submodelElement.GetIdShort())

	return id, nil
}

// Create creates a root-level SubmodelElement within an existing transaction.
//
// This method creates a new submodel element at the root level (no parent) within
// the specified submodel. It's used when adding top-level elements directly to a
// submodel rather than within a collection or list.
//
// The method:
//   - Creates the semantic ID reference if provided
//   - Validates that no element with the same idShort already exists
//   - Inserts the element as a root element (no parent, position 0)
//   - Creates supplemental semantic IDs if provided
//   - Returns the database ID for use in type-specific operations
//
// Parameters:
//   - tx: Active transaction context for atomic operations
//   - submodelID: ID of the parent submodel
//   - submodelElement: The submodel element to create at root level
//
// Returns:
//   - int: Database ID of the newly created element
//   - error: An error if semantic ID creation fails, element already exists, or insertion fails
//
// Example:
//
//	id, err := handler.Create(tx, "submodel123", propertyElement)
func (p *PostgreSQLSMECrudHandler) Create(tx *sql.Tx, submodelID string, submodelElement types.ISubmodelElement) (int, error) {
	return p.CreateWithPath(tx, submodelID, 0, *submodelElement.IDShort(), submodelElement, 0, 0)
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

	// First, get the existing element ID and verify it exists in the correct submodel
	var existingID int
	var oldSemanticID, oldDescriptionID, oldDisplayNameID sql.NullInt64

	selectQuery := dialect.From(goqu.T("submodel_element")).
		Select(
			goqu.C("id"),
			goqu.C("semantic_id"),
			goqu.C("description_id"),
			goqu.C("displayname_id"),
		).
		Where(
			goqu.C("idshort_path").Eq(idShortOrPath),
			goqu.C("submodel_id").Eq(submodelID),
		)

	selectSQL, selectArgs, err := selectQuery.ToSQL()
	if err != nil {
		return err
	}

	err = localTx.QueryRow(selectSQL, selectArgs...).Scan(&existingID, &oldSemanticID, &oldDescriptionID, &oldDisplayNameID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("SubmodelElement with path '" + idShortOrPath + "' not found in submodel '" + submodelID + "'")
		}
		return err
	}

	// Handle semantic ID update
	var newSemanticID sql.NullInt64
	semanticID := submodelElement.SemanticID()
	if semanticID != nil {
		newSemanticID, err = persistenceutils.CreateReference(localTx, semanticID, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			return err
		}
	}

	// Handle description update
	newDescriptionID, err := persistenceutils.CreateLangStringTextTypes(localTx, submodelElement.Description())
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to update Description - see console for details")
	}

	// Handle display name update
	newDisplayNameID, err := persistenceutils.CreateLangStringNameTypes(localTx, submodelElement.DisplayName())
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to update DisplayName - see console for details")
	}

	// Handle embedded data specifications
	edsJSONString := "[]"
	if len(submodelElement.EmbeddedDataSpecifications()) > 0 {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		edsBytes, err := json.Marshal(submodelElement.EmbeddedDataSpecifications())
		if err != nil {
			return err
		}
		edsJSONString = string(edsBytes)
	}

	// Handle supplemental semantic IDs
	supplementalSemanticIDsJSONString := "[]"
	if len(submodelElement.SupplementalSemanticIDs()) > 0 {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		supplementalSemanticIDsBytes, err := json.Marshal(submodelElement.SupplementalSemanticIDs())
		if err != nil {
			return err
		}
		supplementalSemanticIDsJSONString = string(supplementalSemanticIDsBytes)
	}

	// Handle extensions
	extensionsJSONString := "[]"
	if len(submodelElement.Extensions()) > 0 {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		extensionsBytes, err := json.Marshal(submodelElement.Extensions())
		if err != nil {
			return err
		}
		extensionsJSONString = string(extensionsBytes)
	}

	// Update the main submodel_element record FIRST to release foreign key constraints
	updateQuery := dialect.Update(goqu.T("submodel_element")).
		Set(goqu.Record{
			"category":                    submodelElement.Category(),
			"semantic_id":                 newSemanticID,
			"description_id":              newDescriptionID,
			"displayname_id":              newDisplayNameID,
			"embedded_data_specification": edsJSONString,
			"supplemental_semantic_ids":   supplementalSemanticIDsJSONString,
			"extensions":                  extensionsJSONString,
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

	// NOW delete old references after the foreign keys have been updated
	if oldSemanticID.Valid {
		err = persistenceutils.DeleteReference(localTx, int(oldSemanticID.Int64))
		if err != nil {
			_, _ = fmt.Println(err)
			return common.NewInternalServerError("Failed to delete old semantic ID - see console for details")
		}
	}

	if oldDescriptionID.Valid {
		err = persistenceutils.DeleteLangStringTextTypes(localTx, int(oldDescriptionID.Int64))
		if err != nil {
			_, _ = fmt.Println(err)
			return common.NewInternalServerError("Failed to delete old description - see console for details")
		}
	}

	if oldDisplayNameID.Valid {
		err = persistenceutils.DeleteLangStringNameTypes(localTx, int(oldDisplayNameID.Int64))
		if err != nil {
			_, _ = fmt.Println(err)
			return common.NewInternalServerError("Failed to delete old display name - see console for details")
		}
	}

	// Handle qualifiers update - first retrieve old qualifier IDs, then delete them properly
	selectQualifiersQuery := dialect.From(goqu.T("submodel_element_qualifier")).
		Select(goqu.C("qualifier_id")).
		Where(goqu.C("sme_id").Eq(existingID))

	selectQualifiersSQL, selectQualifiersArgs, err := selectQualifiersQuery.ToSQL()
	if err != nil {
		return err
	}

	rows, err := localTx.Query(selectQualifiersSQL, selectQualifiersArgs...)
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to retrieve old qualifiers - see console for details")
	}

	var oldQualifierIDs []int
	for rows.Next() {
		var qualifierID int
		if err := rows.Scan(&qualifierID); err != nil {
			_ = rows.Close()
			return err
		}
		oldQualifierIDs = append(oldQualifierIDs, qualifierID)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Delete junction table entries
	deleteQualifiersQuery := dialect.Delete(goqu.T("submodel_element_qualifier")).
		Where(goqu.C("sme_id").Eq(existingID))

	deleteSQL, deleteArgs, err := deleteQualifiersQuery.ToSQL()
	if err != nil {
		return err
	}

	_, err = localTx.Exec(deleteSQL, deleteArgs...)
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to delete old qualifier associations - see console for details")
	}

	// Delete the actual qualifier records and their associated data
	for _, qualifierID := range oldQualifierIDs {
		if err := persistenceutils.DeleteQualifier(localTx, qualifierID); err != nil {
			_, _ = fmt.Println(err)
			return common.NewInternalServerError("Failed to delete old qualifier - see console for details")
		}
	}

	// Create new qualifiers
	qualifiers := submodelElement.Qualifiers()
	if len(qualifiers) > 0 {
		for i, qualifier := range qualifiers {
			qualifierID, err := persistenceutils.CreateQualifier(localTx, qualifier, i)
			if err != nil {
				return err
			}

			insertQualifierQuery := dialect.Insert(goqu.T("submodel_element_qualifier")).
				Cols("sme_id", "qualifier_id").
				Vals(goqu.Vals{existingID, qualifierID})

			insertSQL, insertArgs, err := insertQualifierQuery.ToSQL()
			if err != nil {
				return err
			}

			_, err = localTx.Exec(insertSQL, insertArgs...)
			if err != nil {
				_, _ = fmt.Println(err)
				return common.NewInternalServerError("Failed to create qualifier for Submodel Element with ID Short'" + idShortOrPath + "'. See console for details.")
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
func (p *PostgreSQLSMECrudHandler) GetDatabaseID(submodelID string, idShortPath string) (int, error) {
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
	err = p.Db.QueryRow(selectQuery, selectArgs...).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, common.NewErrNotFound("SubmodelElement with path '" + idShortPath + "' not found in submodel '" + submodelID + "'")
		}
		return 0, err
	}
	return id, nil
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
func (p *PostgreSQLSMECrudHandler) GetSubmodelElementType(idShortPath string) (string, error) {
	dialect := goqu.Dialect("postgres")
	selectQuery, selectArgs, err := dialect.From("submodel_element").
		Select("model_type").
		Where(goqu.C("idshort_path").Eq(idShortPath)).
		ToSQL()
	if err != nil {
		return "", err
	}

	var modelType string
	err = p.Db.QueryRow(selectQuery, selectArgs...).Scan(&modelType)
	if err != nil {
		return "", err
	}
	return modelType, nil
}
