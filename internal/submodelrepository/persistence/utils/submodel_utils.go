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
// Author: Jannik Fried (Fraunhofer IESE), Aaron Zielstorff (Fraunhofer IESE)

// Package submodel_repository_utils contains utility functions for the Submodel Repository component, such as parsing and handling of submodel-related data.
package submodel_repository_utils

import (
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	jsoniter "github.com/json-iterator/go"
)

// JsonStringFromJsonableSlice converts a slice of AAS elements to a JSON array
// string.
//
// The function transforms each element using jsonization.ToJsonable and then
// marshals the resulting slice of maps with the provided JSON API.
//
// Parameters:
//   - json: JSON API used for marshaling.
//   - elements: AAS elements that implement types.IClass.
//
// Returns:
//   - *string: Pointer to the marshaled JSON array string.
//   - error: Non-nil if conversion to jsonable or marshaling fails.
//
// Example:
//
//	json := jsoniter.ConfigCompatibleWithStandardLibrary
//	result, err := JsonStringFromJsonableSlice(json, displayNames)
//	// result -> "[{\"language\":\"en\",\"text\":\"Name\"}]"
func JsonStringFromJsonableSlice[T types.IClass](json jsoniter.API, elements []T) (*string, error) {
	jsonable := make([]map[string]any, 0, len(elements))

	for _, element := range elements {
		converted, err := jsonization.ToJsonable(element)
		if err != nil {
			return nil, err
		}
		jsonable = append(jsonable, converted)
	}

	jsonBytes, err := json.Marshal(jsonable)
	if err != nil {
		return nil, err
	}

	jsonString := string(jsonBytes)

	return &jsonString, nil
}

// JsonStringFromJsonableObject converts an AAS element to a JSON object string.
//
// The function transforms the input element using jsonization.ToJsonable and
// marshals the resulting map with the provided JSON API.
//
// Parameters:
//   - json: JSON API used for marshaling.
//   - element: A single AAS element implementing types.IClass.
//
// Returns:
//   - *string: Pointer to the marshaled JSON object string.
//   - error: Non-nil if conversion to jsonable or marshaling fails.
//
// Example:
//
//	json := jsoniter.ConfigCompatibleWithStandardLibrary
//	result, err := JsonStringFromJsonableObject(json, administration)
//	// result -> "{\"version\":\"1\",\"revision\":\"0\"}"
func JsonStringFromJsonableObject(json jsoniter.API, element types.IClass) (*string, error) {
	converted, err := jsonization.ToJsonable(element)
	if err != nil {
		return nil, err
	}

	jsonBytes, err := json.Marshal(converted)
	if err != nil {
		return nil, err
	}

	jsonString := string(jsonBytes)

	return &jsonString, nil
}

// StartTXIfNeeded starts a new database transaction if one is not already in progress.
func StartTXIfNeeded(tx *sql.Tx, err error, db *sql.DB) (func(*error), *sql.Tx, error) {
	cu := func(*error) {}
	localTx := tx
	if !IsTransactionAlreadyInProgress(tx) {
		var startedTx *sql.Tx

		startedTx, cu, err = common.StartTransaction(db)

		localTx = startedTx
	}
	return cu, localTx, err
}

// CommitTransactionIfNeeded commits the database transaction if it was started locally.
func CommitTransactionIfNeeded(tx *sql.Tx, localTx *sql.Tx) error {
	if !IsTransactionAlreadyInProgress(tx) {
		err := localTx.Commit()
		if err != nil {
			return err
		}
	}
	return nil
}

// IsTransactionAlreadyInProgress checks if a database transaction is already in progress.
func IsTransactionAlreadyInProgress(tx *sql.Tx) bool {
	return tx != nil
}

// GetSubmodelDatabaseID resolves the internal database ID of a Submodel by its identifier.
//
// Description:
// This function queries the submodel table within an existing transaction and returns
// the numeric primary key (`id`) for the provided semantic submodel identifier.
//
// Parameters:
//   - tx: Active SQL transaction used to execute the lookup query.
//   - submodelID: External Submodel identifier (`submodel_identifier`).
//
// Returns:
//   - int: Internal database ID of the submodel.
//   - error: Non-nil if query generation fails, no row is found, or query execution fails.
//
// Usage:
//
//	id, err := GetSubmodelDatabaseID(tx, "urn:example:submodel:123")
//	if err != nil {
//		return err
//	}
func GetSubmodelDatabaseID(tx *sql.Tx, submodelID string) (int, error) {
	var databaseID int
	sqlQuery, args, err := goqu.Select("id").From("submodel").Where(goqu.I("submodel_identifier").Eq(submodelID)).ToSQL()
	if err != nil {
		return 0, err
	}
	err = tx.QueryRow(sqlQuery, args...).Scan(&databaseID)
	if err != nil {
		return 0, err
	}
	return databaseID, nil
}

// GetSubmodelDatabaseIDFromDB resolves the internal database ID of a Submodel by its identifier.
//
// Description:
// This function performs the same lookup as GetSubmodelDatabaseID, but operates directly
// on a database handle without requiring an existing transaction.
//
// Parameters:
//   - db: Database connection used to execute the lookup query.
//   - submodelID: External Submodel identifier (`submodel_identifier`).
//
// Returns:
//   - int: Internal database ID of the submodel.
//   - error: Non-nil if query generation fails, no row is found, or query execution fails.
//
// Usage:
//
//	id, err := GetSubmodelDatabaseIDFromDB(db, "urn:example:submodel:123")
//	if err != nil {
//		return err
//	}
func GetSubmodelDatabaseIDFromDB(db *sql.DB, submodelID string) (int, error) {
	var databaseID int
	sqlQuery, args, err := goqu.Select("id").From("submodel").Where(goqu.I("submodel_identifier").Eq(submodelID)).ToSQL()
	if err != nil {
		return 0, err
	}
	err = db.QueryRow(sqlQuery, args...).Scan(&databaseID)
	if err != nil {
		return 0, err
	}
	return databaseID, nil
}
