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
	"log/slog"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
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
	cu, localTx, err := common.StartTXIfNeeded(tx, err, p.db)
	if err != nil {
		return err
	}
	defer cu(&err)
	err = p.decorated.Update(submodelID, idShortOrPath, submodelElement, localTx, isPut)
	if err != nil {
		return err
	}
	effectivePath := resolveUpdatedPath(idShortOrPath, submodelElement, isPut)

	smDbID, err := persistenceutils.GetSubmodelDatabaseID(localTx, submodelID)
	if err != nil {
		slog.Error("submodel database ID lookup failed", "error.code", "SMREPO-SMEUPDATE-GETSMDBID", "error", err)
		return common.NewInternalServerError("Failed to execute PostgreSQL Query - no changes applied - see console for details.")
	}
	elementID, err := p.decorated.GetDatabaseIDWithTx(localTx, smDbID, effectivePath)
	if err != nil {
		return err
	}

	dialect := goqu.Dialect("postgres")

	// Build update record for Range-specific fields
	updateRecord, err := buildUpdateRangeRecordObject(rangeElem, isPut)
	if err != nil {
		return err
	}

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

	return common.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly stores the decoded ValueOnly bounds of a Range in the typed columns of its valueType.
// Absent bounds stay untouched and nil bounds are cleared.
func (p PostgreSQLRangeHandler) UpdateValueOnly(tx *sql.Tx, update gen.ValueOnlyUpdate) error {
	patch, err := valueOnlyPatchAs[gen.RangeValuePatch](update)
	if err != nil {
		return err
	}
	typedValue, err := MapRangeValueByType(patch.ValueType, lexicalOrEmpty(patch.Min.Value), lexicalOrEmpty(patch.Max.Value))
	if err != nil {
		return err
	}
	record := goqu.Record{}
	if patch.Min.Set {
		record["min_text"] = typedValue.MinText
		record["min_num"] = typedValue.MinNumeric
		record["min_time"] = typedValue.MinTime
		record["min_date"] = typedValue.MinDate
		record["min_datetime"] = typedValue.MinDateTime
	}
	if patch.Max.Set {
		record["max_text"] = typedValue.MaxText
		record["max_num"] = typedValue.MaxNumeric
		record["max_time"] = typedValue.MaxTime
		record["max_date"] = typedValue.MaxDate
		record["max_datetime"] = typedValue.MaxDateTime
	}
	return execValueOnlyUpdate(tx, "range_element", record, update.ElementID, "SMREPO-UPDRANGEVALONLY")
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
	typedValue, err := MapRangeValueByType(rangeElem.ValueType(), minVal, maxVal)
	if err != nil {
		return nil, err
	}

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
			"min_date":     typedValue.MinDate,
			"max_date":     typedValue.MaxDate,
			"min_datetime": typedValue.MinDateTime,
			"max_datetime": typedValue.MaxDateTime,
		},
	}, nil
}

func buildUpdateRangeRecordObject(rangeElem *types.Range, isPut bool) (goqu.Record, error) {
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
		typedValue, err := MapRangeValueByType(rangeElem.ValueType(), *rangeElem.Min(), *rangeElem.Max())
		if err != nil {
			return nil, err
		}
		updateRecord["min_text"] = typedValue.MinText
		updateRecord["max_text"] = typedValue.MaxText
		updateRecord["min_num"] = typedValue.MinNumeric
		updateRecord["max_num"] = typedValue.MaxNumeric
		updateRecord["min_time"] = typedValue.MinTime
		updateRecord["max_time"] = typedValue.MaxTime
		updateRecord["min_date"] = typedValue.MinDate
		updateRecord["max_date"] = typedValue.MaxDate
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
		typedValue, err := MapRangeValueByType(rangeElem.ValueType(), minVal, maxVal)
		if err != nil {
			return nil, err
		}
		if minVal != "" {
			updateRecord["min_text"] = typedValue.MinText
			updateRecord["min_num"] = typedValue.MinNumeric
			updateRecord["min_time"] = typedValue.MinTime
			updateRecord["min_date"] = typedValue.MinDate
			updateRecord["min_datetime"] = typedValue.MinDateTime
		}
		if maxVal != "" {
			updateRecord["max_text"] = typedValue.MaxText
			updateRecord["max_num"] = typedValue.MaxNumeric
			updateRecord["max_time"] = typedValue.MaxTime
			updateRecord["max_date"] = typedValue.MaxDate
			updateRecord["max_datetime"] = typedValue.MaxDateTime
		}

	}
	return updateRecord, nil
}
