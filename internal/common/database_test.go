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
	"os"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/telemetry"
)

func TestResolvePostgresPoolSettings(t *testing.T) {
	tests := []struct {
		name    string
		cfg     PostgresConfig
		want    PostgresPoolSettings
		errCode string
	}{
		{
			name: "uses common defaults for zero values",
			want: PostgresPoolSettings{
				MaxOpenConnections:     50,
				MaxIdleConnections:     25,
				ConnMaxLifetimeMinutes: 5,
				ConnMaxIdleTimeMinutes: 0,
			},
		},
		{
			name: "uses explicit settings",
			cfg: PostgresConfig{
				MaxOpenConnections:     12,
				MaxIdleConnections:     4,
				ConnMaxLifetimeMinutes: 30,
				ConnMaxIdleTimeMinutes: 7,
			},
			want: PostgresPoolSettings{
				MaxOpenConnections:     12,
				MaxIdleConnections:     4,
				ConnMaxLifetimeMinutes: 30,
				ConnMaxIdleTimeMinutes: 7,
			},
		},
		{
			name: "caps default max idle at explicit max open",
			cfg: PostgresConfig{
				MaxOpenConnections: 10,
			},
			want: PostgresPoolSettings{
				MaxOpenConnections:     10,
				MaxIdleConnections:     10,
				ConnMaxLifetimeMinutes: 5,
				ConnMaxIdleTimeMinutes: 0,
			},
		},
		{
			name:    "rejects negative max open",
			cfg:     PostgresConfig{MaxOpenConnections: -1},
			errCode: "CONFIG-POSTGRES-MAXOPEN",
		},
		{
			name:    "rejects negative max idle",
			cfg:     PostgresConfig{MaxIdleConnections: -1},
			errCode: "CONFIG-POSTGRES-MAXIDLE",
		},
		{
			name:    "rejects negative max lifetime",
			cfg:     PostgresConfig{ConnMaxLifetimeMinutes: -1},
			errCode: "CONFIG-POSTGRES-CONNMAXLIFETIME",
		},
		{
			name:    "rejects negative max idle time",
			cfg:     PostgresConfig{ConnMaxIdleTimeMinutes: -1},
			errCode: "CONFIG-POSTGRES-CONNMAXIDLETIME",
		},
		{
			name: "rejects max idle above max open",
			cfg: PostgresConfig{
				MaxOpenConnections: 5,
				MaxIdleConnections: 6,
			},
			errCode: "CONFIG-POSTGRES-IDLEEXCEEDSOPEN",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolvePostgresPoolSettings(test.cfg)
			if test.errCode != "" {
				if err == nil || !strings.Contains(err.Error(), test.errCode) {
					t.Fatalf("expected %s error, got %v", test.errCode, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolvePostgresPoolSettings() failed: %v", err)
			}
			if got != test.want {
				t.Fatalf("ResolvePostgresPoolSettings() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestConfigurePostgresPool(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	settings, err := ConfigurePostgresPool(db, PostgresConfig{
		MaxOpenConnections:     12,
		MaxIdleConnections:     4,
		ConnMaxLifetimeMinutes: 30,
		ConnMaxIdleTimeMinutes: 7,
	})
	if err != nil {
		t.Fatalf("ConfigurePostgresPool() failed: %v", err)
	}
	if got := db.Stats().MaxOpenConnections; got != settings.MaxOpenConnections {
		t.Fatalf("MaxOpenConnections = %d, want %d", got, settings.MaxOpenConnections)
	}
}

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

func TestValidateSchemaVersionContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectQuery(`SELECT "schema_version", "state" FROM "basyxsystem"`).
		WillReturnRows(sqlmock.NewRows([]string{"schema_version", "state"}).AddRow(CURRENT_DATABASE_VERSION, cleanSchemaState))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err = ValidateSchemaVersionContext(ctx, db, CURRENT_DATABASE_VERSION); err != nil {
		t.Fatalf("ValidateSchemaVersionContext() failed: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestSchemaValidationFailureReleasesDatabasePoolRole(t *testing.T) {
	clearOpenTelemetryEnvironment(t)
	t.Setenv("OTEL_METRICS_EXPORTER", "console")
	runtime, err := telemetry.Configure(t.Context(), "testservice")
	if err != nil {
		t.Fatalf("configure telemetry: %v", err)
	}
	t.Cleanup(func() { runtime.Shutdown(t.Context()) })

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	mock.ExpectClose()
	if err = telemetry.RegisterDatabasePool(db, telemetry.DatabasePoolRoleWriter); err != nil {
		t.Fatalf("register database pool: %v", err)
	}

	validationErr := errors.New("DB-CHECKVER-MISMATCH test schema mismatch")
	if got := closePostgresAfterSchemaValidationFailure(db, validationErr); !errors.Is(got, validationErr) {
		t.Fatalf("schema validation error was not preserved: %v", got)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}

	replacementDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("replacement sqlmock.New() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = telemetry.UnregisterDatabasePool(replacementDB)
		_ = replacementDB.Close()
	})
	if err = telemetry.RegisterDatabasePool(replacementDB, telemetry.DatabasePoolRoleWriter); err != nil {
		t.Fatalf("register replacement database pool: %v", err)
	}
}

func clearOpenTelemetryEnvironment(t *testing.T) {
	t.Helper()
	for _, environment := range os.Environ() {
		key, _, found := strings.Cut(environment, "=")
		if found && strings.HasPrefix(key, "OTEL_") {
			t.Setenv(key, "")
		}
	}
}
