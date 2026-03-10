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

// Package aas_repository_utils contains utility functions for the Asset Administration Shell Repository component, such as parsing and handling of aas-related data.
package aas_repository_utils

import (
	"database/sql"

	"github.com/doug-martin/goqu/v9"
)

// GetAssetAdministrationShellDatabaseID returns the internal database ID for a given AAS identifier.
func GetAssetAdministrationShellDatabaseID(tx *sql.Tx, aasId string) (int64, error) {
	var databaseID int64
	sqlQuery, args, err := goqu.Select("id").From("aas").Where(goqu.I("aas_id").Eq(aasId)).ToSQL()
	if err != nil {
		return 0, err
	}
	err = tx.QueryRow(sqlQuery, args...).Scan(&databaseID)
	if err != nil {
		return 0, err
	}
	return databaseID, nil
}
