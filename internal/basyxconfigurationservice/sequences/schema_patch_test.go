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

package sequences

import (
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestSchemaPatchExecuteAppliesPatchAndUpdatesVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	patchSQL := "ALTER TABLE IF EXISTS aas_identifier ADD COLUMN IF NOT EXISTS test_column TEXT;"
	patchPath := writeTempSchema(t, patchSQL)

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_lock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "schema_version" FROM "basyxsystem" ORDER BY "identifier" ASC LIMIT 1`)).
		WillReturnRows(sqlmock.NewRows([]string{"schema_version"}).AddRow("v1.0.0"))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(patchSQL)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "basyxsystem" SET "schema_version"=$1,"state"=$2`)).
		WithArgs("v1.0.1", schemaStateClean).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	step := NewSchemaPatch(&ExecutionContext{DB: db}, patchPath, "v1.0.1")
	statusCode, execErr := step.Execute(3)
	if execErr != nil {
		t.Fatalf("unexpected error: %v", execErr)
	}
	if statusCode != 0 {
		t.Fatalf("expected status code 0, got %d", statusCode)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestSchemaPatchSeedsVersionRowWhenMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	patchSQL := "ALTER TABLE IF EXISTS aas_identifier ADD COLUMN IF NOT EXISTS test_column TEXT;"
	patchPath := writeTempSchema(t, patchSQL)

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_lock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "schema_version" FROM "basyxsystem" ORDER BY "identifier" ASC LIMIT 1`)).
		WillReturnRows(sqlmock.NewRows([]string{"schema_version"}))
	mock.ExpectExec(regexp.QuoteMeta(seedSystemTableQuery)).
		WithArgs(initialSchemaVersion, schemaStateClean).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(patchSQL)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "basyxsystem" SET "schema_version"=$1,"state"=$2`)).
		WithArgs("v1.0.1", schemaStateClean).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	step := NewSchemaPatch(&ExecutionContext{DB: db}, patchPath, "v1.0.1")
	statusCode, execErr := step.Execute(3)
	if execErr != nil {
		t.Fatalf("unexpected error: %v", execErr)
	}
	if statusCode != 0 {
		t.Fatalf("expected status code 0, got %d", statusCode)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestSchemaPatchMarksDatabaseDirtyWhenPatchExecutionFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	patchSQL := "ALTER TABLE IF EXISTS aas_identifier ADD COLUMN test_column TEXT;"
	patchPath := writeTempSchema(t, patchSQL)

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_lock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "schema_version" FROM "basyxsystem" ORDER BY "identifier" ASC LIMIT 1`)).
		WillReturnRows(sqlmock.NewRows([]string{"schema_version"}).AddRow("v1.0.0"))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(patchSQL)).
		WillReturnError(errors.New("patch failed"))
	mock.ExpectRollback()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "basyxsystem" SET "state"=$1`)).
		WithArgs(schemaStateDirty).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	step := NewSchemaPatch(&ExecutionContext{DB: db}, patchPath, "v1.0.1")
	statusCode, execErr := step.Execute(3)
	if execErr == nil {
		t.Fatal("expected error, got nil")
	}
	if statusCode != 1 {
		t.Fatalf("expected status code 1, got %d", statusCode)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestCompareSemanticVersions(t *testing.T) {
	testCases := []struct {
		name    string
		current string
		target  string
		want    int
		wantErr bool
	}{
		{name: "equal", current: "v1.0.1", target: "1.0.1", want: 0},
		{name: "current lower", current: "1.0.1", target: "1.0.2", want: -1},
		{name: "current higher", current: "1.2.0", target: "1.1.9", want: 1},
		{name: "invalid current", current: "1.0", target: "1.0.1", wantErr: true},
		{name: "invalid target", current: "1.0.0", target: "abc", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compareSemanticVersions(tc.current, tc.target)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("compareSemanticVersions()=%d want=%d", got, tc.want)
			}
		})
	}
}
