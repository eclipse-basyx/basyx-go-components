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

	"github.com/eclipse-basyx/basyx-go-components/internal/common/conditional"
)

// CommitTransaction writes the pending resource revisions of tx, evaluates the
// request's preconditions and commits. Every write transaction must commit
// through it, so resource revisions change in the same transaction as the
// resources. On error nothing is committed and tx is rolled back.
func CommitTransaction(tx *sql.Tx) error {
	flushed, err := conditional.FlushTx(tx)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	flushed.Committed()
	return nil
}

// RollbackTransaction rolls tx back and drops its pending revision changes.
func RollbackTransaction(tx *sql.Tx) error {
	conditional.Discard(tx)
	return tx.Rollback()
}

// CommitError wraps a CommitTransaction error with an error code. Failed or
// missing preconditions keep their type, so they are answered with 412 or 428.
func CommitError(code string, err error) error {
	if conditional.IsPreconditionError(err) {
		return err
	}
	return NewInternalServerError(code + " " + err.Error())
}

// IsErrPreconditionFailed reports whether err is a failed precondition (412).
func IsErrPreconditionFailed(err error) bool {
	return conditional.IsPreconditionFailed(err)
}

// IsErrPreconditionRequired reports whether err is a missing precondition (428).
func IsErrPreconditionRequired(err error) bool {
	return conditional.IsPreconditionRequired(err)
}

// NewConditionalGuard creates the conditional request guard of a service.
func NewConditionalGuard(cfg *Config) *conditional.Guard {
	options := conditional.Options{DecodeIdentifier: DecodeString}
	if cfg != nil {
		options.RequireIfMatch = cfg.Server.ConditionalRequests.RequireIfMatch
	}
	return conditional.NewGuard(options)
}
