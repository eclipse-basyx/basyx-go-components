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
