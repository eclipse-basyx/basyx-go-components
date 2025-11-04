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

// Package submodelelements provides persistence handlers for various submodel element types
// in the Eclipse BaSyx submodel repository. It implements CRUD operations for different
// submodel element types such as Range, Property, Collection, and others, with PostgreSQL
// as the underlying database.
//
// Author: Jannik Fried ( Fraunhofer IESE )
package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLPropertyHandler handles persistence operations for Property submodel elements.
// It uses the decorator pattern to extend the base PostgreSQLSMECrudHandler with
// Property-specific functionality. Property elements represent single data values with
// a defined value type (string, numeric, boolean, time, or datetime).
type PostgreSQLPropertyHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLPropertyHandler creates a new PostgreSQLPropertyHandler instance.
// It initializes the decorated PostgreSQLSMECrudHandler for base submodel element operations.
//
// Parameters:
//   - db: A PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLPropertyHandler: A new handler instance
//   - error: An error if the decorated handler initialization fails
func NewPostgreSQLPropertyHandler(db *sql.DB) (*PostgreSQLPropertyHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLPropertyHandler{db: db, decorated: decoratedHandler}, nil
}

// Create persists a new Property submodel element to the database within a transaction.
// It first creates the base submodel element using the decorated handler, then inserts
// Property-specific data including the value categorized by its value type (text, numeric,
// boolean, time, or datetime).
//
// Parameters:
//   - tx: The database transaction
//   - submodelID: The ID of the parent submodel
//   - submodelElement: The Property element to create (must be of type *gen.Property)
//
// Returns:
//   - int: The database ID of the created element
//   - error: An error if the element is not a Property type or if database operations fail
func (p PostgreSQLPropertyHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	property, ok := submodelElement.(*gen.Property)
	if !ok {
		return 0, errors.New("submodelElement is not of type Property")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// Property-specific database insertion
	// Determine which column to use based on valueType
	err = insertProperty(property, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested persists a new nested Property submodel element to the database within a transaction.
// This method is used when creating Property elements within collection-like structures (e.g., SubmodelElementCollection).
// It creates the base nested element with the provided idShortPath and position, then inserts Property-specific data.
//
// Parameters:
//   - tx: The database transaction
//   - submodelID: The ID of the parent submodel
//   - parentID: The database ID of the parent collection element
//   - idShortPath: The path identifying the element's location within the hierarchy
//   - submodelElement: The Property element to create (must be of type *gen.Property)
//   - pos: The position of the element within the parent collection
//
// Returns:
//   - int: The database ID of the created element
//   - error: An error if the element is not a Property type or if database operations fail
func (p PostgreSQLPropertyHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	property, ok := submodelElement.(*gen.Property)
	if !ok {
		return 0, errors.New("submodelElement is not of type Property")
	}

	// Create the nested property with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelID, parentID, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// Property-specific database insertion for nested element
	err = insertProperty(property, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing Property submodel element in the database.
// Currently delegates all update operations to the decorated handler for base submodel element properties.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifying the element to update
//   - submodelElement: The updated Property element data
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLPropertyHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}

// Delete removes a Property submodel element from the database.
// Currently delegates all delete operations to the decorated handler. Property-specific data
// is typically removed via database cascade constraints.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifying the element to delete
//
// Returns:
//   - error: An error if the delete operation fails
func (p PostgreSQLPropertyHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

// insertProperty is a helper function that inserts Property-specific data into the property_element table.
// It categorizes the property value into appropriate columns based on the valueType:
//   - Text types (xs:string, xs:anyURI, xs:base64Binary, xs:hexBinary) -> value_text
//   - Numeric types (xs:int, xs:decimal, xs:double, xs:float, etc.) -> value_num
//   - Boolean types (xs:boolean) -> value_bool
//   - Time types (xs:time) -> value_time
//   - Datetime types (xs:date, xs:dateTime, xs:duration, etc.) -> value_datetime
//
// The valueID field is reserved for potential future use to reference other elements,
// but is currently not fully implemented.
//
// Parameters:
//   - property: The Property element containing the data to insert
//   - tx: The database transaction
//   - id: The database ID of the parent submodel element
//
// Returns:
//   - error: An error if the database insert operation fails
func insertProperty(property *gen.Property, tx *sql.Tx, id int) error {
	var valueText, valueNum, valueBool, valueTime, valueDatetime sql.NullString
	var valueID sql.NullInt64

	switch property.ValueType {
	case "xs:string", "xs:anyURI", "xs:base64Binary", "xs:hexBinary":
		valueText = sql.NullString{String: property.Value, Valid: property.Value != ""}
	case "xs:int", "xs:integer", "xs:long", "xs:short", "xs:byte",
		"xs:unsignedInt", "xs:unsignedLong", "xs:unsignedShort", "xs:unsignedByte",
		"xs:positiveInteger", "xs:negativeInteger", "xs:nonNegativeInteger", "xs:nonPositiveInteger",
		"xs:decimal", "xs:double", "xs:float":
		valueNum = sql.NullString{String: property.Value, Valid: property.Value != ""}
	case "xs:boolean":
		valueBool = sql.NullString{String: property.Value, Valid: property.Value != ""}
	case "xs:time":
		valueTime = sql.NullString{String: property.Value, Valid: property.Value != ""}
	case "xs:date", "xs:dateTime", "xs:duration", "xs:gDay", "xs:gMonth",
		"xs:gMonthDay", "xs:gYear", "xs:gYearMonth":
		valueDatetime = sql.NullString{String: property.Value, Valid: property.Value != ""}
	default:
		// Fallback to text for unknown types
		valueText = sql.NullString{String: property.Value, Valid: property.Value != ""}
	}

	// Handle valueID if present
	if property.ValueID != nil && len(property.ValueID.Keys) > 0 && property.ValueID.Keys[0].Value != "" {
		// Assuming ValueID references another element by ID - you may need to adjust this logic
		valueID = sql.NullInt64{Int64: 0, Valid: false} // Implement proper ID resolution here
	}

	// Insert Property-specific data
	_, err := tx.Exec(`INSERT INTO property_element (id, value_type, value_text, value_num, value_bool, value_time, value_datetime, value_id)
					 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id,
		property.ValueType,
		valueText,
		valueNum,
		valueBool,
		valueTime,
		valueDatetime,
		valueID,
	)
	return err
}
