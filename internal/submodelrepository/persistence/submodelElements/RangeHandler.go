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
	"context"
	"database/sql"
	"log/slog"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
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
	elementID, err := p.decorated.UpdateElement(submodelID, idShortOrPath, submodelElement, localTx, isPut)
	if err != nil {
		return err
	}

	dialect := goqu.Dialect("postgres")

	// Build update record for Range-specific fields
	updateRecord, err := buildUpdateRangeRecordObject(rangeElem, isPut)
	if err != nil {
		return err
	}

	update := dialect.Update("range_element").
		Set(updateRecord).
		Where(goqu.C("id").Eq(elementID))
	typeGuard := rangeUpdateTypeGuard(updateRecord)
	if typeGuard != nil {
		update = update.Where(typeGuard)
	}
	updateQuery, updateArgs, err := update.ToSQL()
	if err != nil {
		err = common.NewInternalServerError("SMREPO-UPDRANGE-BUILDQ " + err.Error())
		return err
	}

	result, err := localTx.Exec(updateQuery, updateArgs...)
	if err != nil {
		slog.Error("range update failed", "error.code", "SMREPO-UPDRANGE-EXECQ", "error", err)
		err = common.NewInternalServerError("SMREPO-UPDRANGE-EXECQ failed to update range_element")
		return err
	}
	if typeGuard != nil {
		var rows int64
		rows, err = result.RowsAffected()
		if err != nil {
			err = common.NewInternalServerError("SMREPO-UPDRANGE-ROWS " + err.Error())
			return err
		}
		if rows == 0 {
			err = common.NewErrConflict("SMREPO-UPDRANGE-TYPECONFLICT Range valueType changed while an omitted bound has a stored value")
			return err
		}
	}

	return common.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly stores the decoded ValueOnly bounds of a Range.
// Absent bounds stay untouched and nil bounds are cleared.
func (p PostgreSQLRangeHandler) UpdateValueOnly(ctx context.Context, tx *sql.Tx, update gen.ValueOnlyUpdate) error {
	patch, err := valueOnlyPatchAs[gen.RangeValuePatch](update)
	if err != nil {
		return err
	}
	typedValue, err := MapRangeValueByType(patch.ValueType, patch.Min.Value, patch.Max.Value)
	if err != nil {
		return err
	}
	record := goqu.Record{}
	if patch.Min.Set {
		setRangeBound(record, "min", typedValue.MinText, typedValue.MinNumeric)
	}
	if patch.Max.Set {
		setRangeBound(record, "max", typedValue.MaxText, typedValue.MaxNumeric)
	}
	return execValueOnlyUpdate(ctx, tx, "range_element", record, update.ElementID, "SMREPO-UPDRANGEVALONLY")
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

	typedValue, err := MapRangeValueByType(rangeElem.ValueType(), rangeElem.Min(), rangeElem.Max())
	if err != nil {
		return nil, err
	}

	record := goqu.Record{"id": id, "value_type": rangeElem.ValueType()}
	setRangeBound(record, "min", typedValue.MinText, typedValue.MinNumeric)
	setRangeBound(record, "max", typedValue.MaxText, typedValue.MaxNumeric)
	return &InsertQueryPart{TableName: "range_element", Record: record}, nil
}

func buildUpdateRangeRecordObject(rangeElem *types.Range, isPut bool) (goqu.Record, error) {
	valueType := rangeElem.ValueType()
	value, err := MapRangeValueByType(valueType, rangeElem.Min(), rangeElem.Max())
	if err != nil {
		return nil, err
	}
	record := goqu.Record{"value_type": valueType}
	if isPut || isStorableLexical(valueType, rangeElem.Min()) {
		setRangeBound(record, "min", value.MinText, value.MinNumeric)
	}
	if isPut || isStorableLexical(valueType, rangeElem.Max()) {
		setRangeBound(record, "max", value.MaxText, value.MaxNumeric)
	}
	return record, nil
}

// setRangeBound writes the text and numeric columns of one Range bound into an update record.
func setRangeBound(record goqu.Record, bound string, text, numeric sql.NullString) {
	record[bound+"_text"] = text
	record[bound+"_num"] = numeric
}

// rangeUpdateTypeGuard returns a WHERE condition that rejects an update which omits a bound
// while the stored Range has a different valueType and a value for that bound. Checking it in
// the same UPDATE prevents a stale PATCH from applying a new valueType to a concurrently
// written bound. It returns nil when the record writes both bounds.
func rangeUpdateTypeGuard(record goqu.Record) exp.Expression {
	omittedBounds := goqu.Ex{}
	for _, bound := range []string{"min", "max"} {
		if _, written := record[bound+"_text"]; written {
			continue
		}
		omittedBounds[bound+"_text"] = nil
		omittedBounds[bound+"_num"] = nil
	}
	if len(omittedBounds) == 0 {
		return nil
	}
	return goqu.Or(goqu.C("value_type").Eq(record["value_type"]), omittedBounds)
}
