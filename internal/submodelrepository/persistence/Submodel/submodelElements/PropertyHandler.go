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
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
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
		return 0, common.NewErrBadRequest("submodelElement is not of type Property")
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
func (p PostgreSQLPropertyHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	property, ok := submodelElement.(*gen.Property)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type Property")
	}

	// Create the nested property with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
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
func (p PostgreSQLPropertyHandler) Update(submodelID string, idShortOrPath string, submodelElement gen.SubmodelElement) error {
	return p.decorated.Update(submodelID, idShortOrPath, submodelElement)
}

func (p PostgreSQLPropertyHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	var elementID int
	goquQuery, args, err := goqu.From("submodel_element").
		Select("id").
		Where(goqu.Ex{
			"submodel_id":  submodelID,
			"idshort_path": idShortOrPath,
		}).ToSQL()
	if err != nil {
		return err
	}

	row := p.db.QueryRow(goquQuery, args...)
	err = row.Scan(&elementID)

	goquQuery, args, err = goqu.From("property_element").Select("value_type").Where(goqu.C("id").Eq(elementID)).ToSQL()
	if err != nil {
		return err
	}
	var valueType string
	row = p.db.QueryRow(goquQuery, args...)
	err = row.Scan(&valueType)
	// Update based on valueType
	propertyValue, ok := valueOnly.(gen.PropertyValue)
	if !ok {
		return common.NewErrBadRequest("valueOnly is not of type PropertyValue")
	}

	var valueText, valueNum, valueBool, valueTime, valueDatetime sql.NullString

	switch valueType {
	case "xs:string", "xs:anyURI", "xs:base64Binary", "xs:hexBinary":
		valueText = sql.NullString{String: propertyValue.Value, Valid: propertyValue.Value != ""}
	case "xs:int", "xs:integer", "xs:long", "xs:short", "xs:byte",
		"xs:unsignedInt", "xs:unsignedLong", "xs:unsignedShort", "xs:unsignedByte",
		"xs:positiveInteger", "xs:negativeInteger", "xs:nonNegativeInteger", "xs:nonPositiveInteger",
		"xs:decimal", "xs:double", "xs:float":
		valueNum = sql.NullString{String: propertyValue.Value, Valid: propertyValue.Value != ""}
	case "xs:boolean":
		valueBool = sql.NullString{String: propertyValue.Value, Valid: propertyValue.Value != ""}
	case "xs:time":
		valueTime = sql.NullString{String: propertyValue.Value, Valid: propertyValue.Value != ""}
	case "xs:date", "xs:dateTime", "xs:duration", "xs:gDay", "xs:gMonth",
		"xs:gMonthDay", "xs:gYear", "xs:gYearMonth":
		valueDatetime = sql.NullString{String: propertyValue.Value, Valid: propertyValue.Value != ""}
	default:
		// Fallback to text for unknown types
		valueText = sql.NullString{String: propertyValue.Value, Valid: propertyValue.Value != ""}
	}

	_, err = p.db.Exec(`UPDATE property_element SET value_text = $1, value_num = $2, value_bool = $3, value_time = $4, value_datetime = $5 WHERE id = $6`,
		valueText,
		valueNum,
		valueBool,
		valueTime,
		valueDatetime,
		elementID,
	)
	if err != nil {
		return err
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
	return p.decorated.Delete(idShortOrPath)
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
	valueIDDbID, err := persistenceutils.CreateReference(tx, property.ValueID, sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to create SemanticID - no changes applied - see console for details")
	}

	// Insert Property-specific data
	_, err = tx.Exec(`INSERT INTO property_element (id, value_type, value_text, value_num, value_bool, value_time, value_datetime, value_id)
					 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id,
		property.ValueType,
		valueText,
		valueNum,
		valueBool,
		valueTime,
		valueDatetime,
		valueIDDbID,
	)
	if err != nil {
		return err
	}

	return nil
}
