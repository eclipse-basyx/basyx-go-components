package common

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestValidateDatabaseVersion(t *testing.T) {
	t.Run("matches expected version", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New() failed: %v", err)
		}
		defer func() {
			_ = db.Close()
		}()

		mock.ExpectQuery("SELECT database_version").
			WillReturnRows(sqlmock.NewRows([]string{"database_version"}).AddRow("v1.0.1"))

		if err = ValidateDatabaseVersion(db, "v1.0.1"); err != nil {
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

		mock.ExpectQuery("SELECT database_version").
			WillReturnRows(sqlmock.NewRows([]string{"database_version"}).AddRow("v1.0.0"))

		err = ValidateDatabaseVersion(db, "v1.0.1")
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

		mock.ExpectQuery("SELECT database_version").
			WillReturnRows(sqlmock.NewRows([]string{"database_version"}))

		err = ValidateDatabaseVersion(db, "v1.0.1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet SQL expectations: %v", err)
		}
	})
}
