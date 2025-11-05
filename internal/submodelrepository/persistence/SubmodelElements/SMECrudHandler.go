/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
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
	db *sql.DB
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
func isEmptyReference(ref *gen.Reference) bool {
	if ref == nil {
		return true
	}
	return reflect.DeepEqual(ref, gen.Reference{})
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
	return &PostgreSQLSMECrudHandler{db: db}, nil
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
func (p *PostgreSQLSMECrudHandler) CreateWithPath(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, position int, rootSubmodelElementID int) (int, error) {
	var referenceID sql.NullInt64
	var err error
	if submodelElement.GetSemanticID() != nil {
		referenceID, err = persistenceutils.CreateReference(tx, submodelElement.GetSemanticID(), sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			return 0, err
		}
	}

	var convertedDescription []gen.LangStringText
	for _, desc := range submodelElement.GetDescription() {
		convertedDescription = append(convertedDescription, desc)
	}
	descriptionID, err := persistenceutils.CreateLangStringTextTypes(tx, convertedDescription)
	if err != nil {
		fmt.Println(err)
		return 0, common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}

	displayNameID, err := persistenceutils.CreateLangStringNameTypes(tx, submodelElement.GetDisplayName())
	if err != nil {
		fmt.Println(err)
		return 0, common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}

	// Check if a SubmodelElement with the same submodelID and idshort_path already exists
	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM submodel_element WHERE submodel_id = $1 AND idshort_path = $2)`,
		submodelID, idShortPath).Scan(&exists)
	if err != nil {
		return 0, err
	}

	if exists {
		return 0, fmt.Errorf("SubmodelElement with submodelID '%s' and idshort_path '%s' already exists",
			submodelID, idShortPath)
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

	var id int
	err = tx.QueryRow(`	INSERT INTO
	 					submodel_element(submodel_id, parent_sme_id, position, id_short, category, model_type, semantic_id, idshort_path, description_id, displayname_id, root_sme_id)
						VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
		submodelID,
		parentDBId,
		position,
		submodelElement.GetIdShort(),
		submodelElement.GetCategory(),
		submodelElement.GetModelType(),
		referenceID, // This will be NULL if no semantic ID was provided
		idShortPath, // Use the provided idShortPath instead of just GetIdShort()
		descriptionID,
		displayNameID,
		rootDbID,
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	embeddedDataSpecifications := submodelElement.GetEmbeddedDataSpecifications()
	for i, eds := range embeddedDataSpecifications {
		edsDbID, err := persistenceutils.CreateEmbeddedDataSpecification(tx, eds, i)
		if err != nil {
			return 0, err
		}
		_, err = tx.Exec("INSERT INTO submodel_element_embedded_data_specification(submodel_element_id, embedded_data_specification_id) VALUES ($1, $2)", id, edsDbID)
		if err != nil {
			return 0, err
		}
	}

	qualifiers := submodelElement.GetQualifiers()
	if len(qualifiers) > 0 {
		for i, qualifier := range qualifiers {
			qualifierID, err := persistenceutils.CreateQualifier(tx, qualifier, i)
			if err != nil {
				return 0, err
			}
			_, err = tx.Exec(`INSERT INTO submodel_element_qualifier(sme_id, qualifier_id) VALUES($1, $2)`, id, qualifierID)
			if err != nil {
				fmt.Println(err)
				return 0, common.NewInternalServerError("Failed to Create Qualifier for Submodel Element with ID '" + fmt.Sprintf("%d", id) + "'. See console for details.")
			}
		}
	}

	supplementalSemanticIDs := submodelElement.GetSupplementalSemanticIds()
	if len(supplementalSemanticIDs) > 0 {
		for _, supplementalSemanticID := range supplementalSemanticIDs {
			supplementalSemanticIDDbID, err := persistenceutils.CreateReference(tx, &supplementalSemanticID, sql.NullInt64{}, sql.NullInt64{})
			if err != nil {
				return 0, err
			}
			_, err = tx.Exec(`INSERT INTO submodel_element_supplemental_semantic_id (submodel_element_id, reference_id) VALUES ($1, $2)`, id, supplementalSemanticIDDbID)
			if err != nil {
				return 0, err
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
func (p *PostgreSQLSMECrudHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	return p.CreateWithPath(tx, submodelID, 0, submodelElement.GetIdShort(), submodelElement, 0, 0)
}

// Update updates an existing SubmodelElement identified by its idShort or path.
//
// This method is currently a placeholder for future implementation of element updates.
// When implemented, it should handle updating element properties, semantic IDs, and
// potentially restructuring relationships if the element is moved within the hierarchy.
//
// Parameters:
//   - idShortOrPath: The idShort or full path of the element to update
//   - submodelElement: The updated element data
//
// Returns:
//   - error: Currently always returns nil (not yet implemented)
//
// nolint:revive
func (p *PostgreSQLSMECrudHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
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
func (p *PostgreSQLSMECrudHandler) GetDatabaseID(idShortPath string) (int, error) {
	var id int
	err := p.db.QueryRow(`SELECT id FROM submodel_element WHERE idshort_path = $1`, idShortPath).Scan(&id)
	if err != nil {
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
	var position sql.NullInt64
	err := p.db.QueryRow(`SELECT MAX(position) FROM submodel_element WHERE parent_sme_id = $1`, parentID).Scan(&position)
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
	var modelType string
	err := p.db.QueryRow(`SELECT model_type FROM submodel_element WHERE idshort_path = $1`, idShortPath).Scan(&modelType)
	if err != nil {
		return "", err
	}
	return modelType, nil
}
