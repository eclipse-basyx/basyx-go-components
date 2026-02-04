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

	"github.com/FriedJannik/aas-go-sdk/types"
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

// Update modifies an existing Range submodel element identified by its idShort or path.
// It updates both the common submodel element properties via the decorated handler
// and the Range-specific fields such as min/max values based on the value type.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - submodelElement: The updated Range element data
//   - tx: Optional database transaction (created if nil)
//   - isPut: true for PUT (replace all), false for PATCH (update only provided fields)
//
// Returns:
//   - error: An error if the update operation fails or if the element is not a Range type
func (p PostgreSQLRangeHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	rangeElem, ok := submodelElement.(*types.Range)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type Range")
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

	elementID, err := p.decorated.GetDatabaseID(submodelID, idShortOrPath)
	if err != nil {
		return err
	}

	dialect := goqu.Dialect("postgres")

	// Build update record for Range-specific fields
	updateRecord := buildUpdateRangeRecordObject(rangeElem, isPut)

	// Execute update
	updateQuery, updateArgs, err := dialect.Update("range_element").
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

	var valueType types.DataTypeDefXSD
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

// GetInsertQueryPart returns the type-specific insert query part for batch insertion of Range elements.
// It returns the table name and record for inserting into the range_element table.
//
// Parameters:
//   - tx: Active database transaction (not used for Range)
//   - id: The database ID of the base submodel_element record
//   - element: The Range element to insert
//
// Returns:
//   - *InsertQueryPart: The table name and record for range_element insert
//   - error: An error if the element is not of type Range or min/max values are missing
func (p PostgreSQLRangeHandler) GetInsertQueryPart(_ *sql.Tx, id int, element types.ISubmodelElement) (*InsertQueryPart, error) {
	rangeElem, ok := element.(*types.Range)
	if !ok {
		return nil, common.NewErrBadRequest("submodelElement is not of type Range")
	}

	minVal := ""
	maxVal := ""
	if rangeElem.Min() != nil {
		minVal = *rangeElem.Min()
	}
	if rangeElem.Max() != nil {
		maxVal = *rangeElem.Max()
	}
	typedValue := persistenceutils.MapRangeValueByType(rangeElem.ValueType(), minVal, maxVal)

	return &InsertQueryPart{
		TableName: "range_element",
		Record: goqu.Record{
			"id":           id,
			"value_type":   rangeElem.ValueType(),
			"min_text":     typedValue.MinText,
			"max_text":     typedValue.MaxText,
			"min_num":      typedValue.MinNumeric,
			"max_num":      typedValue.MaxNumeric,
			"min_time":     typedValue.MinTime,
			"max_time":     typedValue.MaxTime,
			"min_datetime": typedValue.MinDateTime,
			"max_datetime": typedValue.MaxDateTime,
		},
	}, nil
}

// getRangeColumnNames returns the appropriate column names for min and max values
// based on the XML Schema datatype of the Range element.
func getRangeColumnNames(valueType types.DataTypeDefXSD) (minCol, maxCol string) {
	return persistenceutils.GetRangeColumnNames(valueType)
}

func buildUpdateRangeRecordObject(rangeElem *types.Range, isPut bool) goqu.Record {
	updateRecord := goqu.Record{}

	// ValueType is always updated (required field)
	updateRecord["value_type"] = rangeElem.ValueType()

	// Handle min and max based on isPut flag
	if isPut {
		// PUT: Always replace min/max values
		if rangeElem.Min() == nil || rangeElem.Max() == nil {
			// For PUT, both min and max must be provided
			panic("Both 'Min' and 'Max' values must be provided for Range element in PUT operation")
		}
		typedValue := persistenceutils.MapRangeValueByType(rangeElem.ValueType(), *rangeElem.Min(), *rangeElem.Max())
		updateRecord["min_text"] = typedValue.MinText
		updateRecord["max_text"] = typedValue.MaxText
		updateRecord["min_num"] = typedValue.MinNumeric
		updateRecord["max_num"] = typedValue.MaxNumeric
		updateRecord["min_time"] = typedValue.MinTime
		updateRecord["max_time"] = typedValue.MaxTime
		updateRecord["min_datetime"] = typedValue.MinDateTime
		updateRecord["max_datetime"] = typedValue.MaxDateTime
	} else { //nolint:all - elseif: can replace 'else {if cond {}}' with 'else if cond {}' -> this would make the code less readable and has differing semantics
		// PATCH: Only update if minVal/max are provided
		minVal := ""
		if rangeElem.Min() != nil {
			minVal = *rangeElem.Min()
		}
		maxVal := ""
		if rangeElem.Max() != nil {
			maxVal = *rangeElem.Max()
		}
		typedValue := persistenceutils.MapRangeValueByType(rangeElem.ValueType(), minVal, maxVal)
		if minVal != "" {
			updateRecord["min_text"] = typedValue.MinText
			updateRecord["min_num"] = typedValue.MinNumeric
			updateRecord["min_time"] = typedValue.MinTime
			updateRecord["min_datetime"] = typedValue.MinDateTime
		}
		if maxVal != "" {
			updateRecord["max_text"] = typedValue.MaxText
			updateRecord["max_num"] = typedValue.MaxNumeric
			updateRecord["max_time"] = typedValue.MaxTime
			updateRecord["max_datetime"] = typedValue.MaxDateTime
		}

	}
	return updateRecord
}
