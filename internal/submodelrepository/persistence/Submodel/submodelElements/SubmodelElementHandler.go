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

// Package submodelelements provides persistence layer functionality for managing submodel elements
// in the Eclipse BaSyx submodel repository. It implements CRUD operations for all types of
// submodel elements defined in the AAS specification, including properties, collections,
// relationships, events, and more.
//
// The package uses a factory pattern to create type-specific handlers and provides efficient
// database queries with hierarchical data retrieval for nested element structures.
package submodelelements

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// GetSMEHandler creates the appropriate CRUD handler for a submodel element.
//
// This function uses the Factory Pattern to instantiate the correct handler based on
// the model type of the provided submodel element. It provides a clean, type-safe way
// to obtain handlers without requiring client code to know the concrete handler types.
//
// Parameters:
//   - submodelElement: The submodel element for which to create a handler
//   - db: Database connection to be used by the handler
//
// Returns:
//   - PostgreSQLSMECrudInterface: Type-specific handler implementing CRUD operations
//   - error: An error if the model type is unsupported or handler creation fails
func GetSMEHandler(submodelElement gen.SubmodelElement, db *sql.DB) (PostgreSQLSMECrudInterface, error) {
	return GetSMEHandlerByModelType(string(submodelElement.GetModelType()), db)
}

// GetSMEHandlerByModelType creates a handler by model type string.
//
// This function implements the Single Responsibility Principle by focusing solely on
// the logic for determining and instantiating the correct handler based on a model
// type string. It supports all AAS submodel element types defined in the specification.
//
// Supported model types:
//   - AnnotatedRelationshipElement: Relationship with annotations
//   - BasicEventElement: Event element for monitoring and notifications
//   - Blob: Binary data element
//   - Capability: Functional capability description
//   - Entity: Logical or physical entity
//   - EventElement: Base event element
//   - File: File reference element
//   - MultiLanguageProperty: Property with multi-language support
//   - Operation: Invocable operation
//   - Property: Single-valued property
//   - Range: Value range element
//   - ReferenceElement: Reference to another element
//   - RelationshipElement: Relationship between elements
//   - SubmodelElementCollection: Collection of submodel elements
//   - SubmodelElementList: Ordered list of submodel elements
//
// Parameters:
//   - modelType: String representation of the submodel element type
//   - db: Database connection to be used by the handler
//
// Returns:
//   - PostgreSQLSMECrudInterface: Type-specific handler implementing CRUD operations
//   - error: An error if the model type is unsupported or handler creation fails
func GetSMEHandlerByModelType(modelType string, db *sql.DB) (PostgreSQLSMECrudInterface, error) {
	var handler PostgreSQLSMECrudInterface

	switch modelType {
	case "AnnotatedRelationshipElement":
		areHandler, err := NewPostgreSQLAnnotatedRelationshipElementHandler(db)
		if err != nil {
			fmt.Println("Error creating AnnotatedRelationshipElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create AnnotatedRelationshipElement handler. See console for details.")
		}
		handler = areHandler
	case "BasicEventElement":
		beeHandler, err := NewPostgreSQLBasicEventElementHandler(db)
		if err != nil {
			fmt.Println("Error creating BasicEventElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create BasicEventElement handler. See console for details.")
		}
		handler = beeHandler
	case "Blob":
		blobHandler, err := NewPostgreSQLBlobHandler(db)
		if err != nil {
			fmt.Println("Error creating Blob handler:", err)
			return nil, common.NewInternalServerError("Failed to create Blob handler. See console for details.")
		}
		handler = blobHandler
	case "Capability":
		capHandler, err := NewPostgreSQLCapabilityHandler(db)
		if err != nil {
			fmt.Println("Error creating Capability handler:", err)
			return nil, common.NewInternalServerError("Failed to create Capability handler. See console for details.")
		}
		handler = capHandler
	case "Entity":
		entityHandler, err := NewPostgreSQLEntityHandler(db)
		if err != nil {
			fmt.Println("Error creating Entity handler:", err)
			return nil, common.NewInternalServerError("Failed to create Entity handler. See console for details.")
		}
		handler = entityHandler
	case "EventElement":
		eventElemHandler, err := NewPostgreSQLEventElementHandler(db)
		if err != nil {
			fmt.Println("Error creating EventElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create EventElement handler. See console for details.")
		}
		handler = eventElemHandler
	case "File":
		fileHandler, err := NewPostgreSQLFileHandler(db)
		if err != nil {
			fmt.Println("Error creating File handler:", err)
			return nil, common.NewInternalServerError("Failed to create File handler. See console for details.")
		}
		handler = fileHandler
	case "MultiLanguageProperty":
		mlpHandler, err := NewPostgreSQLMultiLanguagePropertyHandler(db)
		if err != nil {
			fmt.Println("Error creating MultiLanguageProperty handler:", err)
			return nil, common.NewInternalServerError("Failed to create MultiLanguageProperty handler. See console for details.")
		}
		handler = mlpHandler
	case "Operation":
		opHandler, err := NewPostgreSQLOperationHandler(db)
		if err != nil {
			fmt.Println("Error creating Operation handler:", err)
			return nil, common.NewInternalServerError("Failed to create Operation handler. See console for details.")
		}
		handler = opHandler
	case "Property":
		propHandler, err := NewPostgreSQLPropertyHandler(db)
		if err != nil {
			fmt.Println("Error creating Property handler:", err)
			return nil, common.NewInternalServerError("Failed to create Property handler. See console for details.")
		}
		handler = propHandler
	case "Range":
		rangeHandler, err := NewPostgreSQLRangeHandler(db)
		if err != nil {
			fmt.Println("Error creating Range handler:", err)
			return nil, common.NewInternalServerError("Failed to create Range handler. See console for details.")
		}
		handler = rangeHandler
	case "ReferenceElement":
		refElemHandler, err := NewPostgreSQLReferenceElementHandler(db)
		if err != nil {
			fmt.Println("Error creating ReferenceElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create ReferenceElement handler. See console for details.")
		}
		handler = refElemHandler
	case "RelationshipElement":
		relElemHandler, err := NewPostgreSQLRelationshipElementHandler(db)
		if err != nil {
			fmt.Println("Error creating RelationshipElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create RelationshipElement handler. See console for details.")
		}
		handler = relElemHandler
	case "SubmodelElementCollection":
		smeColHandler, err := NewPostgreSQLSubmodelElementCollectionHandler(db)
		if err != nil {
			fmt.Println("Error creating SubmodelElementCollection handler:", err)
			return nil, common.NewInternalServerError("Failed to create SubmodelElementCollection handler. See console for details.")
		}
		handler = smeColHandler
	case "SubmodelElementList":
		smeListHandler, err := NewPostgreSQLSubmodelElementListHandler(db)
		if err != nil {
			fmt.Println("Error creating SubmodelElementList handler:", err)
			return nil, common.NewInternalServerError("Failed to create SubmodelElementList handler. See console for details.")
		}
		handler = smeListHandler
	default:
		return nil, errors.New("ModelType " + modelType + " unsupported.")
	}
	return handler, nil
}

// DeleteSubmodelElementByPath removes a submodel element by its idShort or path including all nested elements.
//
// This function performs cascading deletion of a submodel element and its entire subtree.
// If the deleted element is part of a SubmodelElementList, it automatically adjusts the
// position indices of remaining elements to maintain consistency.
//
// The function handles:
//   - Direct deletion of the element and its subtree (using path pattern matching)
//   - Index recalculation for SubmodelElementList elements after deletion
//   - Path updates for remaining list elements to reflect new indices
//
// Parameters:
//   - tx: Transaction context for atomic deletion operations
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: Path to the element to delete (e.g., "prop1" or "collection.list[2]")
//
// Returns:
//   - error: An error if the element is not found or database operations fail
//
// Example:
//
//	// Delete a simple property
//	err := DeleteSubmodelElementByPath(tx, "submodel123", "temperature")
//
//	// Delete an element in a list (adjusts indices of elements after it)
//	err := DeleteSubmodelElementByPath(tx, "submodel123", "sensors[1]")
//
//	// Delete a nested collection and all its children
//	err := DeleteSubmodelElementByPath(tx, "submodel123", "properties.metadata")
func DeleteSubmodelElementByPath(tx *sql.Tx, submodelID string, idShortOrPath string) error {
	query := `DELETE FROM submodel_element WHERE submodel_id = $1 AND (idshort_path = $2 OR idshort_path LIKE $2 || '.%' OR idshort_path LIKE $2 || '[%')`
	result, err := tx.Exec(query, submodelID, idShortOrPath)
	if err != nil {
		return err
	}
	affectedRows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	// if idShortPath ends with ] it is part of a SubmodelElementList and we need to update the indices of the remaining elements
	if idShortOrPath[len(idShortOrPath)-1] == ']' {
		// extract the parent path and the index of the deleted element
		var parentPath string
		var deletedIndex int
		for i := len(idShortOrPath) - 1; i >= 0; i-- {
			if idShortOrPath[i] == '[' {
				parentPath = idShortOrPath[:i]
				indexStr := idShortOrPath[i+1 : len(idShortOrPath)-1]
				var err error
				deletedIndex, err = strconv.Atoi(indexStr)
				if err != nil {
					return err
				}
				break
			}
		}

		// get the id of the parent SubmodelElementList
		var parentID int
		err = tx.QueryRow(`SELECT id FROM submodel_element WHERE submodel_id = $1 AND idshort_path = $2`, submodelID, parentPath).Scan(&parentID)
		if err != nil {
			return err
		}

		// update the indices of the remaining elements in the SubmodelElementList
		updateQuery := `UPDATE submodel_element SET position = position - 1 WHERE parent_sme_id = $1 AND position > $2`
		_, err = tx.Exec(updateQuery, parentID, deletedIndex)
		if err != nil {
			return err
		}
		// update their idshort_path as well
		updatePathQuery := `UPDATE submodel_element SET idshort_path = regexp_replace(idshort_path, '\[' || (position + 1) || '\]', '[' || position || ']') WHERE parent_sme_id = $1 AND position >= $2`
		_, err = tx.Exec(updatePathQuery, parentID, deletedIndex)
		if err != nil {
			return err
		}
	}
	if affectedRows == 0 {
		return common.NewErrNotFound("Submodel-Element ID-Short: " + idShortOrPath)
	}
	return nil
}
