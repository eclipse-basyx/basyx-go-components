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

	"github.com/doug-martin/goqu/v9"
)

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
