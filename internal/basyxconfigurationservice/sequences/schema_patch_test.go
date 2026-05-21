package steps

import (
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestSchemaPatchExecuteAppliesPatchWithoutUpdatingVersionInCode(t *testing.T) {
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
	mock.ExpectQuery(regexp.QuoteMeta(currentDatabaseVersionQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"database_version"}).AddRow("v1.0.0"))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(patchSQL)).
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
