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

// Package submodelelements provides persistence handlers for various submodel element types
// in the Eclipse BaSyx submodel repository. It implements CRUD operations for different
// submodel element types such as Range, Property, Collection, and others, with PostgreSQL
// as the underlying database.
package submodelelements

import (
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLRangeHandler handles persistence operations for Range submodel elements.
// It uses the decorator pattern to extend the base PostgreSQLSMECrudHandler with
// Range-specific functionality. Range elements represent intervals with min and max
// values that can be of various data types (string, numeric, time, datetime).
type PostgreSQLRangeHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLRangeHandler creates a new PostgreSQLRangeHandler instance.
// It initializes the decorated PostgreSQLSMECrudHandler for base submodel element operations.
//
// Parameters:
//   - db: A PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLRangeHandler: A new handler instance
//   - error: An error if the decorated handler initialization fails
func NewPostgreSQLRangeHandler(db *sql.DB) (*PostgreSQLRangeHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLRangeHandler{db: db, decorated: decoratedHandler}, nil
}

// Create persists a new Range submodel element to the database within a transaction.
// It first creates the base submodel element using the decorated handler, then inserts
// Range-specific data including min/max values categorized by value type.
//
// Parameters:
//   - tx: The database transaction
//   - submodelID: The ID of the parent submodel
//   - submodelElement: The Range element to create (must be of type *gen.Range)
//
// Returns:
//   - int: The database ID of the created element
//   - error: An error if the element is not a Range type or if database operations fail
func (p PostgreSQLRangeHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	rangeElem, ok := submodelElement.(*gen.Range)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type Range")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// Range-specific database insertion
	err = insertRange(rangeElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested persists a new nested Range submodel element to the database within a transaction.
// This method is used when creating Range elements within collection-like structures (e.g., SubmodelElementCollection).
// It creates the base nested element with the provided idShortPath and position, then inserts Range-specific data.
//
// Parameters:
//   - tx: The database transaction
//   - submodelID: The ID of the parent submodel
//   - parentID: The database ID of the parent collection element
//   - idShortPath: The path identifying the element's location within the hierarchy
//   - submodelElement: The Range element to create (must be of type *gen.Range)
//   - pos: The position of the element within the parent collection
//
// Returns:
//   - int: The database ID of the created element
//   - error: An error if the element is not a Range type or if database operations fail
func (p PostgreSQLRangeHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	rangeElem, ok := submodelElement.(*gen.Range)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type Range")
	}

	// Create the nested range with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
	if err != nil {
		return 0, err
	}

	// Range-specific database insertion for nested element
	err = insertRange(rangeElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing Range submodel element in the database.
// Currently delegates all update operations to the decorated handler for base submodel element properties.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifying the element to update
//   - submodelElement: The updated Range element data
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLRangeHandler) Update(submodelID string, idShortOrPath string, submodelElement gen.SubmodelElement, tx *sql.Tx, isPut bool) error {
	return p.decorated.Update(submodelID, idShortOrPath, submodelElement, tx, isPut)
}

// UpdateValueOnly updates only the value-specific fields of an existing Range submodel element.
// It updates the min and max values based on the value type of the Range element,
// ensuring that only the relevant columns are modified while others are set to NULL.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The RangeValue containing the new min and max values
//
// Returns:
//   - error: An error if the update operation fails or if the valueOnly is not of type RangeValue
func (p PostgreSQLRangeHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	rangeValue, ok := valueOnly.(gen.RangeValue)
	if !ok {
		return common.NewErrBadRequest("valueOnly is not of type Range")
	}

	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	dialect := goqu.Dialect("postgres")

	// Get Value Type to determine which columns to update
	selectQuery, selectArgs, err := dialect.From(goqu.T("submodel_element").As("sme")).
		InnerJoin(
			goqu.T("range_element").As("re"),
			goqu.On(goqu.I("sme.id").Eq(goqu.I("re.id"))),
		).
		Select(goqu.I("re.value_type")).
		Where(
			goqu.I("sme.submodel_id").Eq(submodelID),
			goqu.I("sme.idshort_path").Eq(idShortOrPath),
		).
		ToSQL()
	if err != nil {
		return err
	}

	var valueType string
	err = p.db.QueryRow(selectQuery, selectArgs...).Scan(&valueType)
	if err != nil {
		return err
	}

	// Determine column names based on value type
	minCol, maxCol := getRangeColumnNames(valueType)

	// Build subquery to get the submodel element ID
	var elementID int
	idQuery, args, err := dialect.From("submodel_element").
		Select("id").
		Where(
			goqu.C("submodel_id").Eq(submodelID),
			goqu.C("idshort_path").Eq(idShortOrPath),
		).ToSQL()
	if err != nil {
		return err
	}
	err = tx.QueryRow(idQuery, args...).Scan(&elementID)
	if err != nil {
		return err
	}

	// Build update record with all columns, setting unused ones to NULL
	updateRecord := goqu.Record{
		"min_text":     nil,
		"max_text":     nil,
		"min_num":      nil,
		"max_num":      nil,
		"min_time":     nil,
		"max_time":     nil,
		"min_datetime": nil,
		"max_datetime": nil,
	}
	// Set the appropriate columns based on value type
	updateRecord[minCol] = rangeValue.Min
	updateRecord[maxCol] = rangeValue.Max

	// Build and execute update query
	updateQuery, updateArgs, err := dialect.Update("range_element").
		Set(updateRecord).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = tx.Exec(updateQuery, updateArgs...)
	if err != nil {
		return err
	}

	err = tx.Commit()
	return err
}

// Delete removes a Range submodel element from the database.
// Currently delegates all delete operations to the decorated handler. Range-specific data
// is typically removed via database cascade constraints.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifying the element to delete
//
// Returns:
//   - error: An error if the delete operation fails
func (p PostgreSQLRangeHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

// insertRange is a helper function that inserts Range-specific data into the range_element table.
// It categorizes min and max values into appropriate columns based on the valueType:
//   - Text types (xs:string, xs:anyURI, xs:base64Binary, xs:hexBinary) -> min_text, max_text
//   - Numeric types (xs:int, xs:decimal, xs:double, xs:float, etc.) -> min_num, max_num
//   - Time types (xs:time) -> min_time, max_time
//   - Datetime types (xs:date, xs:dateTime, xs:duration, etc.) -> min_datetime, max_datetime
//
// Parameters:
//   - rangeElem: The Range element containing the data to insert
//   - tx: The database transaction
//   - id: The database ID of the parent submodel element
//
// Returns:
//   - error: An error if the database insert operation fails
func insertRange(rangeElem *gen.Range, tx *sql.Tx, id int) error {
	// Use centralized value type mapper for min/max values
	typedValue := persistenceutils.MapRangeValueByType(string(rangeElem.ValueType), rangeElem.Min, rangeElem.Max)

	// Insert Range-specific data
	dialect := goqu.Dialect("postgres")
	insertQuery, insertArgs, err := dialect.Insert("range_element").
		Rows(goqu.Record{
			"id":           id,
			"value_type":   rangeElem.ValueType,
			"min_text":     typedValue.MinText,
			"max_text":     typedValue.MaxText,
			"min_num":      typedValue.MinNumeric,
			"max_num":      typedValue.MaxNumeric,
			"min_time":     typedValue.MinTime,
			"max_time":     typedValue.MaxTime,
			"min_datetime": typedValue.MinDateTime,
			"max_datetime": typedValue.MaxDateTime,
		}).
		ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(insertQuery, insertArgs...)
	return err
}

// getRangeColumnNames returns the appropriate column names for min and max values
// based on the XML Schema datatype of the Range element.
func getRangeColumnNames(valueType string) (minCol, maxCol string) {
	return persistenceutils.GetRangeColumnNames(valueType)
}
