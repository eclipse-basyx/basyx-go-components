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
	"os"
	"regexp"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestSchemaUploadGetDescription(t *testing.T) {
	step := NewSchemaUpload(&ExecutionContext{}, "")
	description := step.GetDescription(4)
	if description != "[Step 4] Uploading SQL schema" {
		t.Fatalf("unexpected description: %q", description)
	}
}

func TestSchemaUploadExecuteReturnsNoDBError(t *testing.T) {
	step := NewSchemaUpload(&ExecutionContext{}, "")
	statusCode, err := step.Execute(1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if statusCode != 1 {
		t.Fatalf("expected status code 1, got %d", statusCode)
	}
	if !strings.Contains(err.Error(), "BASYXCFG-SCHEMA-NODB") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchemaUploadExecuteReturnsReadFileError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	ctx := &ExecutionContext{DB: db}
	step := NewSchemaUpload(ctx, "/not/found/schema.sql")

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_lock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	statusCode, execErr := step.Execute(1)
	if execErr == nil {
		t.Fatal("expected error, got nil")
	}
	if statusCode != 1 {
		t.Fatalf("expected status code 1, got %d", statusCode)
	}
	if !strings.Contains(execErr.Error(), "BASYXCFG-SCHEMA-STAT") {
		t.Fatalf("unexpected error: %v", execErr)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestSchemaUploadExecuteSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	schema := "CREATE TABLE IF NOT EXISTS test_table (id INT PRIMARY KEY);"
	schemaPath := writeTempSchema(t, schema)

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_lock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(regexp.QuoteMeta(schema)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := &ExecutionContext{DB: db}
	step := NewSchemaUpload(ctx, schemaPath)

	statusCode, execErr := step.Execute(1)
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

func TestSchemaUploadExecuteLoadsBaseSchemaFromDirectory(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	schemaDir := t.TempDir()
	schemaSQL := "CREATE TABLE IF NOT EXISTS loaded_from_directory (id INT PRIMARY KEY);"
	if err := os.WriteFile(schemaDir+"/base.sql", []byte(schemaSQL), 0o600); err != nil {
		t.Fatalf("write base schema: %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_lock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(regexp.QuoteMeta(schemaSQL)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := &ExecutionContext{DB: db}
	step := NewSchemaUpload(ctx, schemaDir)

	statusCode, execErr := step.Execute(1)
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

func TestSchemaUploadExecuteReturnsLockError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	schemaPath := writeTempSchema(t, "SELECT 1;")
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_lock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnError(os.ErrPermission)

	ctx := &ExecutionContext{DB: db}
	step := NewSchemaUpload(ctx, schemaPath)

	statusCode, execErr := step.Execute(1)
	if execErr == nil {
		t.Fatal("expected error, got nil")
	}
	if statusCode != 1 {
		t.Fatalf("expected status code 1, got %d", statusCode)
	}
	if !strings.Contains(execErr.Error(), "BASYXCFG-SCHEMA-LOCK") {
		t.Fatalf("unexpected error: %v", execErr)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func writeTempSchema(t *testing.T, sql string) string {
	t.Helper()
	path := t.TempDir() + "/schema.sql"
	if err := os.WriteFile(path, []byte(sql), 0o600); err != nil {
		t.Fatalf("write schema file: %v", err)
	}
	return path
}
