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
	"context"
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/conditional"
)

// BeginReadTransaction starts a read-only transaction with a stable PostgreSQL
// snapshot. The revision of a conditional request's target is read as the
// first statement, so its entity tag belongs to the same snapshot.
func BeginReadTransaction(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	if db == nil {
		return nil, NewErrBadRequest("COMMON-BEGINREADTX-NILDB database handle must not be nil")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return nil, err
	}
	if err = conditional.ObserveReadTx(ctx, tx); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

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

	err = CommitTransaction(tx)
	if err != nil {
		if commitErrorCode == "" {
			return CommitError("COMMON-EXECINTX-COMMIT", err)
		}
		return CommitError(commitErrorCode, err)
	}

	return nil
}

// ExecuteInReadTransaction runs fn in a read-only transaction with one stable
// PostgreSQL snapshot for all statements.
func ExecuteInReadTransaction(
	ctx context.Context,
	db *sql.DB,
	startErrorCode string,
	commitErrorCode string,
	fn func(tx *sql.Tx) error,
) (err error) {
	if db == nil {
		return NewErrBadRequest("COMMON-EXECINREADTX-NILDB database handle must not be nil")
	}
	if fn == nil {
		return NewErrBadRequest("COMMON-EXECINREADTX-NILFN transaction callback must not be nil")
	}

	tx, err := BeginReadTransaction(ctx, db)
	if err != nil {
		if startErrorCode == "" {
			return NewInternalServerError("COMMON-EXECINREADTX-STARTTX " + err.Error())
		}
		return NewInternalServerError(startErrorCode + " " + err.Error())
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err = fn(tx); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		if commitErrorCode == "" {
			return NewInternalServerError("COMMON-EXECINREADTX-COMMIT " + err.Error())
		}
		return NewInternalServerError(commitErrorCode + " " + err.Error())
	}

	return nil
}
