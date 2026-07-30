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
	"database/sql/driver"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

const readTransactionOptionsDriverName = "basyx-read-transaction-options-test"

var (
	registerReadTransactionOptionsDriver sync.Once
	readTransactionOptions               = make(chan driver.TxOptions, 1)
)

type readTransactionOptionsDriver struct{}

type readTransactionOptionsConn struct{}

type readTransactionOptionsTx struct{}

func (readTransactionOptionsDriver) Open(string) (driver.Conn, error) {
	return readTransactionOptionsConn{}, nil
}

func (readTransactionOptionsConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}

func (readTransactionOptionsConn) Close() error {
	return nil
}

func (readTransactionOptionsConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions must use BeginTx")
}

func (readTransactionOptionsConn) BeginTx(_ context.Context, options driver.TxOptions) (driver.Tx, error) {
	readTransactionOptions <- options
	return readTransactionOptionsTx{}, nil
}

func (readTransactionOptionsTx) Commit() error {
	return nil
}

func (readTransactionOptionsTx) Rollback() error {
	return nil
}

func TestBeginReadTransactionUsesRepeatableReadAndReadOnly(t *testing.T) {
	registerReadTransactionOptionsDriver.Do(func() {
		sql.Register(readTransactionOptionsDriverName, readTransactionOptionsDriver{})
	})

	db, err := sql.Open(readTransactionOptionsDriverName, "")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	tx, err := BeginReadTransaction(t.Context(), db)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())

	options := <-readTransactionOptions
	require.Equal(t, driver.IsolationLevel(sql.LevelRepeatableRead), options.Isolation)
	require.True(t, options.ReadOnly)
}
