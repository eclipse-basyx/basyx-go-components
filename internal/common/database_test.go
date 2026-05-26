package common

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestValidateSchemaVersion(t *testing.T) {
	t.Run("matches expected version", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New() failed: %v", err)
		}
		defer func() {
			_ = db.Close()
		}()

		mock.ExpectQuery(`SELECT "schema_version", "state" FROM "basyxsystem"`).
			WillReturnRows(sqlmock.NewRows([]string{"schema_version", "state"}).AddRow(CURRENT_DATABASE_VERSION, cleanSchemaState))

		if err = ValidateSchemaVersion(db, CURRENT_DATABASE_VERSION); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet SQL expectations: %v", err)
		}
	})

	t.Run("mismatched version", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New() failed: %v", err)
		}
		defer func() {
			_ = db.Close()
		}()

		mock.ExpectQuery(`SELECT "schema_version", "state" FROM "basyxsystem"`).
			WillReturnRows(sqlmock.NewRows([]string{"schema_version", "state"}).AddRow("v1.0.0", cleanSchemaState))

		err = ValidateSchemaVersion(db, CURRENT_DATABASE_VERSION)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet SQL expectations: %v", err)
		}
	})

	t.Run("no version row", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New() failed: %v", err)
		}
		defer func() {
			_ = db.Close()
		}()

		mock.ExpectQuery(`SELECT "schema_version", "state" FROM "basyxsystem"`).
			WillReturnRows(sqlmock.NewRows([]string{"schema_version", "state"}))

		err = ValidateSchemaVersion(db, CURRENT_DATABASE_VERSION)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet SQL expectations: %v", err)
		}
	})

	t.Run("dirty schema state", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New() failed: %v", err)
		}
		defer func() {
			_ = db.Close()
		}()

		mock.ExpectQuery(`SELECT "schema_version", "state" FROM "basyxsystem"`).
			WillReturnRows(sqlmock.NewRows([]string{"schema_version", "state"}).AddRow(CURRENT_DATABASE_VERSION, "dirty"))

		err = ValidateSchemaVersion(db, CURRENT_DATABASE_VERSION)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet SQL expectations: %v", err)
		}
	})
}
