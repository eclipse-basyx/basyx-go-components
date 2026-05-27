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

package common

import (
	"database/sql"

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/types"
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
	cu := func(_ *error) {}
	localTx := tx
	if !IsTransactionAlreadyInProgress(tx) {
		var startedTx *sql.Tx

		startedTx, cu, err = StartTransaction(db)

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
