package steps

import (
	"os"
	"regexp"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestSystemTableGetDescription(t *testing.T) {
	step := NewSystemTable(&ExecutionContext{})
	description := step.GetDescription(2)
	if description != "[Step 2] Initializing system table" {
		t.Fatalf("unexpected description: %q", description)
	}
}

func TestSystemTableExecuteReturnsNoDBError(t *testing.T) {
	step := NewSystemTable(&ExecutionContext{})

	statusCode, err := step.Execute(1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if statusCode != 1 {
		t.Fatalf("expected status code 1, got %d", statusCode)
	}
	if !strings.Contains(err.Error(), "BASYXCFG-SYSTEM-NODB") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSystemTableExecuteSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_lock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(createSystemTableQuery)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(seedSystemTableQuery)).
		WithArgs(initialDatabaseVersion).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	step := NewSystemTable(&ExecutionContext{DB: db})
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

func TestSystemTableExecuteReturnsCreateTableError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_lock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(createSystemTableQuery)).
		WillReturnError(os.ErrPermission)
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	step := NewSystemTable(&ExecutionContext{DB: db})
	statusCode, execErr := step.Execute(1)
	if execErr == nil {
		t.Fatal("expected error, got nil")
	}
	if statusCode != 1 {
		t.Fatalf("expected status code 1, got %d", statusCode)
	}
	if !strings.Contains(execErr.Error(), "BASYXCFG-SYSTEM-CREATETABLE") {
		t.Fatalf("unexpected error: %v", execErr)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
