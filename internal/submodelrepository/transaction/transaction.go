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

// Package transaction provides transaction management utilities for the submodel repository.
package transaction

import (
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/logger"
)

// TxScope provides a consistent pattern for managing database transactions.
// It handles both owned transactions (created internally) and borrowed transactions
// (passed in from outside), ensuring proper commit/rollback semantics.
type TxScope struct {
	tx        *sql.Tx
	owned     bool // true if this scope owns the transaction (created it)
	committed bool // true if Commit() has been called successfully
}

// NewTxScope creates a new transaction scope.
// If existingTx is provided, it will be used (borrowed transaction).
// Otherwise, a new transaction will be started (owned transaction).
//
// Parameters:
//   - db: The database connection to use for creating a new transaction
//   - existingTx: An existing transaction to use, or nil to create a new one
//
// Returns:
//   - *TxScope: The transaction scope
//   - error: An error if a new transaction could not be started
func NewTxScope(db *sql.DB, existingTx *sql.Tx) (*TxScope, error) {
	if existingTx != nil {
		return &TxScope{tx: existingTx, owned: false}, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	return &TxScope{tx: tx, owned: true}, nil
}

// Tx returns the underlying transaction.
// Use this to execute database operations within the transaction scope.
func (s *TxScope) Tx() *sql.Tx {
	return s.tx
}

// Commit commits the transaction if this scope owns it.
// If the transaction is borrowed, this is a no-op.
// Calling Commit multiple times is safe.
//
// Returns:
//   - error: An error if the commit fails
func (s *TxScope) Commit() error {
	if s.owned && !s.committed {
		s.committed = true
		return s.tx.Commit()
	}
	return nil
}

// Rollback rolls back the transaction if this scope owns it and it hasn't been committed.
// If the transaction is borrowed or already committed, this is a no-op.
// This method is safe to call in defer statements.
func (s *TxScope) Rollback() {
	if s.owned && !s.committed {
		if err := s.tx.Rollback(); err != nil {
			logger.LogError("rolling back transaction", err)
		}
	}
}

// IsOwned returns true if this scope created/owns the transaction.
func (s *TxScope) IsOwned() bool {
	return s.owned
}
