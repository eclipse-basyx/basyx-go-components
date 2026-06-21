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
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestExecutePostgreSQLBatchInTransactionExecutesOneCollectedBlock(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New returned error: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx returned error: %v", err)
	}

	query := "INSERT INTO one VALUES (1);\nINSERT INTO two VALUES (2)"
	mock.ExpectExec(regexp.QuoteMeta(query)).WillReturnResult(sqlmock.NewResult(0, 2))
	if err = ExecutePostgreSQLBatchInTransaction(context.Background(), tx, []PostgreSQLBatchStatement{
		{SQL: "INSERT INTO one VALUES (1)"},
		{SQL: "INSERT INTO two VALUES (2)"},
	}); err != nil {
		t.Fatalf("ExecutePostgreSQLBatchInTransaction returned error: %v", err)
	}

	mock.ExpectRollback()
	if err = tx.Rollback(); err != nil {
		t.Fatalf("Rollback returned error: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestExecutePostgreSQLBatchInTransactionPreservesPostgresError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New returned error: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx returned error: %v", err)
	}

	query := "INSERT INTO one VALUES (1)"
	pgErr := &pgconn.PgError{Code: "23505"}
	mock.ExpectExec(regexp.QuoteMeta(query)).WillReturnError(pgErr)
	err = ExecutePostgreSQLBatchInTransaction(context.Background(), tx, []PostgreSQLBatchStatement{
		{SQL: query},
	})
	if err == nil {
		t.Fatal("expected batch execution error")
	}
	if !IsInternalServerError(err) {
		t.Fatalf("expected internal server error classification, got %v", err)
	}
	if !IsPostgresUniqueViolation(err) {
		t.Fatalf("expected preserved Postgres unique violation, got %v", err)
	}
	var unwrapped *pgconn.PgError
	if !errors.As(err, &unwrapped) {
		t.Fatalf("expected wrapped pgx error, got %v", err)
	}

	mock.ExpectRollback()
	if err = tx.Rollback(); err != nil {
		t.Fatalf("Rollback returned error: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
