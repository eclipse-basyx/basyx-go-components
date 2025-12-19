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

// Package submodelelements provides persistence handlers for Asset Administration Shell (AAS) submodel elements
// in Eclipse BaSyx. This package implements the repository pattern for storing and retrieving various types of
// submodel elements in a PostgreSQL database, including their hierarchical relationships, metadata, and type-specific
// attributes.
//
// The package supports all AAS submodel element types defined in the specification, such as Properties, Collections,
// Lists, RelationshipElements, ReferenceElements, and more. Each handler uses a decorator pattern to add type-specific
// functionality on top of the base CRUD operations provided by PostgreSQLSMECrudHandler.
package submodelelements

import (
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	jsoniter "github.com/json-iterator/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLReferenceElementHandler is a persistence handler for ReferenceElement submodel elements.
// It implements the decorator pattern by wrapping PostgreSQLSMECrudHandler to add ReferenceElement-specific
// persistence logic for managing references to other AAS elements or external resources.
//
// A ReferenceElement contains a value that is a Reference, which consists of a type and a list of keys
// that together identify a specific element within an AAS environment or an external resource.
//
// The handler manages:
//   - Base SubmodelElement attributes (via decorated handler)
//   - Reference value with type and keys
//   - Null reference handling for empty values
//   - Key ordering and position management
//
// Database structure:
//   - reference_element table: Links submodel element ID to reference ID
//   - reference table: Stores reference type
//   - reference_key table: Stores ordered keys with positions
type PostgreSQLReferenceElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLReferenceElementHandler creates a new ReferenceElementHandler with the specified database connection.
// It initializes the decorated PostgreSQLSMECrudHandler for base SubmodelElement operations.
//
// Parameters:
//   - db: Active PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLReferenceElementHandler: Initialized handler instance
//   - error: Any error encountered during handler initialization
func NewPostgreSQLReferenceElementHandler(db *sql.DB) (*PostgreSQLReferenceElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLReferenceElementHandler{db: db, decorated: decoratedHandler}, nil
}

// Create persists a new root-level ReferenceElement to the database within the provided transaction.
// The operation is atomic and includes both base SubmodelElement attributes and ReferenceElement-specific
// reference value with its keys.
//
// The method performs the following operations:
//  1. Type assertion to ensure the element is a ReferenceElement
//  2. Delegates base SubmodelElement creation to the decorated handler
//  3. Persists the reference value (type and keys) or NULL if empty
//  4. Maintains key ordering through position attributes
//
// Parameters:
//   - tx: Active database transaction for atomic operations
//   - submodelID: The ID of the parent submodel
//   - submodelElement: The ReferenceElement to persist (must be *gen.ReferenceElement)
//
// Returns:
//   - int: The database ID of the created ReferenceElement
//   - error: Any error during type assertion or database operations
//
// Example:
//
//	handler, _ := NewPostgreSQLReferenceElementHandler(db)
//	refElem := &gen.ReferenceElement{
//	    IdShort: "ExampleReference",
//	    Value: &gen.Reference{
//	        Type: "ExternalReference",
//	        Keys: []gen.Key{
//	            {Type: "GlobalReference", Value: "https://example.com/resource"},
//	        },
//	    },
//	}
//	id, err := handler.Create(tx, "submodel123", refElem)
func (p PostgreSQLReferenceElementHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	refElem, ok := submodelElement.(*gen.ReferenceElement)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type ReferenceElement")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// ReferenceElement-specific database insertion
	err = insertReferenceElement(refElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested persists a new nested ReferenceElement within a SubmodelElementCollection or SubmodelElementList.
// This method supports the creation of ReferenceElements that are children of other submodel elements,
// maintaining the hierarchical structure and path relationships.
//
// The method performs the following operations:
//  1. Type assertion to ensure the element is a ReferenceElement
//  2. Delegates nested element creation with path tracking to the decorated handler
//  3. Persists the reference value (type and keys) specific to this element
//  4. Maintains proper parent-child relationships and position ordering
//
// Parameters:
//   - tx: Active database transaction for atomic operations
//   - submodelID: The ID of the parent submodel
//   - parentID: The database ID of the parent SubmodelElement (Collection or List)
//   - idShortPath: The full path to this element (e.g., "Collection1.SubCollection.RefElement")
//   - submodelElement: The ReferenceElement to persist (must be *gen.ReferenceElement)
//   - pos: The position of this element within its parent (for ordering in Lists)
//
// Returns:
//   - int: The database ID of the created nested ReferenceElement
//   - error: Any error during type assertion or database operations
//
// Example:
//
//	handler, _ := NewPostgreSQLReferenceElementHandler(db)
//	nestedRefElem := &gen.ReferenceElement{
//	    IdShort: "NestedReference",
//	    Value: &gen.Reference{
//	        Type: "ModelReference",
//	        Keys: []gen.Key{
//	            {Type: "Submodel", Value: "SubmodelID"},
//	            {Type: "Property", Value: "PropertyID"},
//	        },
//	    },
//	}
//	id, err := handler.CreateNested(tx, "submodel123", parentID, "Collection.NestedReference", nestedRefElem, 0)
func (p PostgreSQLReferenceElementHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	refElem, ok := submodelElement.(*gen.ReferenceElement)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type ReferenceElement")
	}

	// Create the nested refElem with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
	if err != nil {
		return 0, err
	}

	// ReferenceElement-specific database insertion for nested element
	err = insertReferenceElement(refElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing ReferenceElement identified by its idShort or full path.
// This method delegates to the decorated handler which implements the base update logic.
//
// Parameters:
//   - idShortOrPath: The idShort or full path of the ReferenceElement to update
//   - submodelElement: The updated ReferenceElement with new values
//
// Returns:
//   - error: Any error encountered during the update operation
//
// Note: This is currently a placeholder that delegates to the decorated handler.
// Full implementation would include updating the reference value and keys.
func (p PostgreSQLReferenceElementHandler) Update(submodelID string, idShortOrPath string, submodelElement gen.SubmodelElement) error {
	return p.decorated.Update(submodelID, idShortOrPath, submodelElement)
}

func (p PostgreSQLReferenceElementHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	refElemVal, ok := valueOnly.(*gen.ReferenceElementValue)
	if !ok {
		return common.NewErrBadRequest("valueOnly is not of type ReferenceElementValue")
	}

	// Marshal reference value to JSON using helper function
	referenceJSONString, err := marshalReferenceValueToJSON(refElemVal)
	if err != nil {
		return err
	}

	// Build and execute update query using GoQu
	query, args, err := goqu.Update("reference_element").
		Set(goqu.Record{
			"value": referenceJSONString,
		}).
		Where(goqu.I("id").Eq(
			goqu.From("submodel_element").
				Select("id").
				Where(goqu.Ex{
					"submodel_id": submodelID,
				}).
				Where(goqu.Or(
					goqu.I("id_short").Eq(idShortOrPath),
					goqu.I("id_short_path").Eq(idShortOrPath),
				)),
		)).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = p.db.Exec(query, args...)
	return err
}

// Parameters:
//   - idShortOrPath: The idShort or full path of the ReferenceElement to delete
//
// Returns:
//   - error: Any error encountered during the deletion operation
//
// Note: Database foreign key constraints ensure cascading deletion of related records.
func (p PostgreSQLReferenceElementHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

// insertReferenceElement is an internal helper function that persists the ReferenceElement-specific data
// to the database. It handles both populated references and empty/null references.
//
// The function performs the following operations:
//  1. Checks if the reference value is empty using isEmptyReference
//  2. If empty, inserts a reference_element record with NULL value_ref
//  3. If populated:
//     a. Inserts the reference record with type
//     b. Inserts all reference keys with their positions (maintaining order)
//     c. Links the reference to the reference_element via value_ref
//
// Parameters:
//   - refElem: The ReferenceElement containing the reference value to persist
//   - tx: Active database transaction for atomic operations
//   - id: The database ID of the SubmodelElement record
//
// Returns:
//   - error: Any error encountered during database operations
//
// Database operations:
//   - INSERT INTO reference (type) - Creates reference record
//   - INSERT INTO reference_key (reference_id, position, type, value) - Creates ordered keys
//   - INSERT INTO reference_element (id, value_ref) - Links element to reference
func insertReferenceElement(refElem *gen.ReferenceElement, tx *sql.Tx, id int) error {
	var referenceJSONString sql.NullString
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if !isEmptyReference(refElem.Value) {
		bytes, err := json.Marshal(refElem.Value)
		if err != nil {
			return err
		}
		referenceJSONString = sql.NullString{String: string(bytes), Valid: true}
	} else {
		referenceJSONString = sql.NullString{Valid: false}
	}

	// Insert reference_element
	_, err := tx.Exec(`INSERT INTO reference_element (id, value) VALUES ($1, $2)`, id, referenceJSONString)
	return err
}

// marshalReferenceValueToJSON converts a ReferenceElementValue to a JSON string for database storage.
// Returns a sql.NullString with Valid=true if the reference has keys, otherwise Valid=false.
func marshalReferenceValueToJSON(refElemVal *gen.ReferenceElementValue) (sql.NullString, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var referenceJSONString sql.NullString

	if len(refElemVal.Keys) > 0 {
		bytes, err := json.Marshal(refElemVal)
		if err != nil {
			return sql.NullString{}, err
		}
		referenceJSONString = sql.NullString{String: string(bytes), Valid: true}
	} else {
		referenceJSONString = sql.NullString{Valid: false}
	}

	return referenceJSONString, nil
}
