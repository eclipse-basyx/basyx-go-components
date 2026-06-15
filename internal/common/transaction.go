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

import "database/sql"

// ExecuteInTransaction starts a transaction, executes fn, and commits on success.
func ExecuteInTransaction(db *sql.DB, startErrorCode string, commitErrorCode string, fn func(tx *sql.Tx) error) (err error) {
	if db == nil {
		return NewErrBadRequest("COMMON-EXECINTX-NILDB database handle must not be nil")
	}
	if fn == nil {
		return NewErrBadRequest("COMMON-EXECINTX-NILFN transaction callback must not be nil")
	}

	tx, cleanup, err := StartTransaction(db)
	if err != nil {
		if startErrorCode == "" {
			return NewInternalServerError("COMMON-EXECINTX-STARTTX " + err.Error())
		}
		return NewInternalServerError(startErrorCode + " " + err.Error())
	}
	defer cleanup(&err)

	err = fn(tx)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		if commitErrorCode == "" {
			return NewInternalServerError("COMMON-EXECINTX-COMMIT " + err.Error())
		}
		return NewInternalServerError(commitErrorCode + " " + err.Error())
	}

	return nil
}
