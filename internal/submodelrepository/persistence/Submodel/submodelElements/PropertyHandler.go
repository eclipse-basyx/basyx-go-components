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

	"github.com/FriedJannik/aas-go-sdk/types"
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
func (p PostgreSQLPropertyHandler) Create(tx *sql.Tx, submodelID string, submodelElement types.ISubmodelElement) (int, error) {
	property, ok := submodelElement.(*types.Property)
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
func (p PostgreSQLPropertyHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement types.ISubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	property, ok := submodelElement.(*types.Property)
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
// This method handles both the common submodel element properties and the specific
// property data including value type, value, and value ID reference.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - submodelElement: The updated Property element data
//   - tx: Active database transaction (can be nil, will create one if needed)
//   - isPut: true: Replaces the element with the body data; false: Updates only passed data
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLPropertyHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	property, ok := submodelElement.(*types.Property)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type Property")
	}

	var err error
	cu, localTx, err := persistenceutils.StartTXIfNeeded(tx, err, p.db)
	if err != nil {
		return err
	}
	defer cu(&err)
	err = p.decorated.Update(submodelID, idShortOrPath, submodelElement, localTx, isPut)
	if err != nil {
		return err
	}

	// Get the element ID
	elementID, err := p.decorated.GetDatabaseID(submodelID, idShortOrPath)
	if err != nil {
		return err
	}

	// Build the update record
	updateRecord, err := buildUpdatePropertyRecordObject(property, isPut, localTx)
	if err != nil {
		return err
	}

	// Update property_element table
	updateQuery, updateArgs, err := goqu.Dialect("postgres").
		Update("property_element").
		Set(updateRecord).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = localTx.Exec(updateQuery, updateArgs...)
	if err != nil {
		return err
	}

	return persistenceutils.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly updates only the value of an existing Property submodel element identified by its idShort or path.
// It categorizes the new value based on the property's value type and updates the corresponding database columns.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The new value to set (must be of type gen.SubmodelElementValue)
//
// Returns:
//   - error: An error if the update operation fails or if the valueOnly type is incorrect
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
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound(fmt.Sprintf("Property element not found for the given idShortOrPath %s", idShortOrPath))
		}
		return err
	}

	goquQuery, args, err = goqu.From("property_element").Select("value_type").Where(goqu.C("id").Eq(elementID)).ToSQL()
	if err != nil {
		return err
	}
	var valueType types.DataTypeDefXSD
	row = p.db.QueryRow(goquQuery, args...)
	err = row.Scan(&valueType)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound(fmt.Sprintf("Property element not found for the given idShortOrPath %s", idShortOrPath))
		}
		return err
	}
	// Update based on valueType using centralized value type mapper
	propertyValue, ok := valueOnly.(gen.PropertyValue)
	if !ok {
		return common.NewErrBadRequest("valueOnly is not of type PropertyValue")
	}

	typedValue := persistenceutils.MapValueByType(valueType, &propertyValue.Value)

	dialect := goqu.Dialect("postgres")
	updateQuery, updateArgs, err := dialect.Update("property_element").
		Set(goqu.Record{
			"value_text":     typedValue.Text,
			"value_num":      typedValue.Numeric,
			"value_bool":     typedValue.Boolean,
			"value_time":     typedValue.Time,
			"value_date":     typedValue.Date,
			"value_datetime": typedValue.DateTime,
		}).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = p.db.Exec(updateQuery, updateArgs...)
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
func insertProperty(property *types.Property, tx *sql.Tx, id int) error {
	// Use centralized value type mapper
	typedValue := persistenceutils.MapValueByType(property.ValueType(), property.Value())

	// Handle valueID if present
	valueIDDbID, err := persistenceutils.CreateReference(tx, property.ValueID(), sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		_, _ = fmt.Println("SMREPO-INSPROP-CRVALID " + err.Error())
		return common.NewInternalServerError("Failed to create SemanticID - no changes applied - see console for details")
	}

	// Insert Property-specific data
	dialect := goqu.Dialect("postgres")
	insertQuery, insertArgs, err := dialect.Insert("property_element").
		Rows(goqu.Record{
			"id":             id,
			"value_type":     property.ValueType(),
			"value_text":     typedValue.Text,
			"value_num":      typedValue.Numeric,
			"value_bool":     typedValue.Boolean,
			"value_time":     typedValue.Time,
			"value_date":     typedValue.Date,
			"value_datetime": typedValue.DateTime,
			"value_id":       valueIDDbID,
		}).
		ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(insertQuery, insertArgs...)
	if err != nil {
		return err
	}

	return nil
}

func buildUpdatePropertyRecordObject(property *types.Property, isPut bool, localTx *sql.Tx) (goqu.Record, error) {
	updateRecord := goqu.Record{}

	// Required field - always update
	updateRecord["value_type"] = property.ValueType()

	// Map value by type - always update based on isPut or if value is provided
	if isPut || property.Value() != nil {
		typedValue := persistenceutils.MapValueByType(property.ValueType(), property.Value())
		updateRecord["value_text"] = typedValue.Text
		updateRecord["value_num"] = typedValue.Numeric
		updateRecord["value_bool"] = typedValue.Boolean
		updateRecord["value_time"] = typedValue.Time
		updateRecord["value_datetime"] = typedValue.DateTime
	}

	// Handle optional ValueID field
	// For PUT: always update (even if nil, which clears the field)
	// For PATCH: only update if provided (not nil)
	if isPut || property.ValueID() != nil {
		valueIDDbID, err := persistenceutils.CreateReference(localTx, property.ValueID(), sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			return nil, fmt.Errorf("failed to update ValueID: %w", err)
		}
		updateRecord["value_id"] = valueIDDbID
	}
	return updateRecord, nil
}
