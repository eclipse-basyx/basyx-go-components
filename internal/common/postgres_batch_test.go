/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package common

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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
