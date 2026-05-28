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
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	_ "github.com/lib/pq"
)

const schemaPatch110Path = "../../../database/patches/1_1_0.sql"

func TestSchemaPatch110AppliesAfterVersion102(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	patchSQL := readSchemaPatch110(t)

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_lock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "schema_version" FROM "basyxsystem" ORDER BY "identifier" ASC LIMIT 1`)).
		WillReturnRows(sqlmock.NewRows([]string{"schema_version"}).AddRow("v1.0.2"))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(patchSQL)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "basyxsystem" SET "schema_version"=$1,"state"=$2`)).
		WithArgs("v1.1.0", schemaStateClean).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(schemaAdvisoryLockID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	step := NewSchemaPatch(&ExecutionContext{DB: db}, schemaPatch110Path, "v1.1.0")
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

func TestSchemaPatch110ContainsMetamodel32SchemaChanges(t *testing.T) {
	patchSQL := readSchemaPatch110(t)

	requiredSnippets := []string{
		"Patch Version  : 1.1.0",
		"Metamodel Ver. : 3.2",
		"ALTER TABLE IF EXISTS aas_payload",
		"ALTER TABLE IF EXISTS submodel_payload",
		"ALTER TABLE IF EXISTS submodel_element_payload",
		"DROP COLUMN IF EXISTS administrative_information_payload",
		"ALTER TABLE IF EXISTS descriptor_payload",
		"ALTER TABLE IF EXISTS concept_description",
		"ADD COLUMN IF NOT EXISTS administration_created_at TIMESTAMPTZ",
		"ADD COLUMN IF NOT EXISTS administration_updated_at TIMESTAMPTZ",
		"CREATE OR REPLACE FUNCTION sync_administrative_information_timestamps()",
		"CREATE OR REPLACE FUNCTION sync_concept_description_administration_timestamps()",
		"CREATE TRIGGER submodel_payload_sync_administration_timestamps",
		"CREATE TRIGGER concept_description_sync_administration_timestamps",
		"CREATE INDEX IF NOT EXISTS ix_submodel_payload_admin_created_at",
		"CREATE INDEX IF NOT EXISTS ix_submodel_payload_admin_updated_at",
		"SET schema_version = 'v1.1.0'",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(patchSQL, snippet) {
			t.Fatalf("patch %s is missing %q", schemaPatch110Path, snippet)
		}
	}

	forbiddenSnippets := []string{
		"UPDATE aas_payload\nSET administration_created_at",
		"UPDATE submodel_payload\nSET administration_created_at",
		"UPDATE descriptor_payload\nSET administration_created_at",
		"UPDATE concept_description\nSET administration_created_at",
		"ix_sme_payload_admin_created_at",
		"ix_sme_payload_admin_updated_at",
		"submodel_element_payload_sync_administration_timestamps",
		"ON submodel_element_payload(administration_created_at)",
		"ON submodel_element_payload(administration_updated_at)",
	}

	for _, snippet := range forbiddenSnippets {
		if strings.Contains(patchSQL, snippet) {
			t.Fatalf("patch %s must not contain %q", schemaPatch110Path, snippet)
		}
	}
}

func TestSchemaPatch110MigratesAssetKindIndices(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("BASYX_SCHEMA_PATCH_TEST_DSN"))
	if dsn == "" {
		t.Skip("set BASYX_SCHEMA_PATCH_TEST_DSN to run the PostgreSQL schema migration test")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open() failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()
	db.SetMaxOpenConns(1)

	if err = db.Ping(); err != nil {
		t.Fatalf("ping PostgreSQL failed: %v", err)
	}

	schemaName := fmt.Sprintf("schema_patch_110_asset_kind_%d", time.Now().UnixNano())
	quotedSchemaName := quotePostgresIdentifier(schemaName)
	execTestSQL(t, db, "CREATE SCHEMA "+quotedSchemaName)
	defer execTestSQL(t, db, "DROP SCHEMA IF EXISTS "+quotedSchemaName+" CASCADE")
	execTestSQL(t, db, "SET search_path TO "+quotedSchemaName)

	createMinimalVersion102Schema(t, db)
	execTestSQL(t, db, "INSERT INTO asset_information (asset_information_id, asset_kind) VALUES (1, 2)")

	step := NewSchemaPatch(&ExecutionContext{DB: db}, schemaPatch110Path, "v1.1.0")
	statusCode, execErr := step.Execute(1)
	if execErr != nil {
		t.Fatalf("unexpected migration error: %v", execErr)
	}
	if statusCode != 0 {
		t.Fatalf("expected status code 0, got %d", statusCode)
	}

	var migratedAssetKind int
	if err = db.QueryRow("SELECT asset_kind FROM asset_information WHERE asset_information_id = 1").Scan(&migratedAssetKind); err != nil {
		t.Fatalf("read migrated asset_kind: %v", err)
	}
	if migratedAssetKind != 3 {
		t.Fatalf("expected asset_kind 2 to migrate to 3, got %d", migratedAssetKind)
	}
}

func readSchemaPatch110(t *testing.T) string {
	t.Helper()

	content, err := os.ReadFile(schemaPatch110Path)
	if err != nil {
		t.Fatalf("read %s: %v", schemaPatch110Path, err)
	}
	return string(content)
}

func createMinimalVersion102Schema(t *testing.T, db *sql.DB) {
	t.Helper()

	statements := []string{
		"CREATE TABLE basyxsystem (identifier BIGSERIAL PRIMARY KEY, schema_version VARCHAR(16), state VARCHAR(32))",
		"INSERT INTO basyxsystem (schema_version, state) VALUES ('v1.0.2', 'clean')",
		"CREATE TABLE aas_payload (aas_id BIGINT PRIMARY KEY, administrative_information_payload JSONB)",
		"CREATE TABLE submodel_payload (submodel_id BIGINT PRIMARY KEY, administrative_information_payload JSONB)",
		"CREATE TABLE submodel_element_payload (submodel_element_id BIGINT PRIMARY KEY, administrative_information_payload JSONB)",
		"CREATE TABLE descriptor_payload (descriptor_id BIGINT PRIMARY KEY, administrative_information_payload JSONB)",
		"CREATE TABLE concept_description (id BIGSERIAL PRIMARY KEY, data JSONB)",
		"CREATE TABLE asset_information (asset_information_id BIGINT PRIMARY KEY, asset_kind INT)",
	}

	for _, statement := range statements {
		execTestSQL(t, db, statement)
	}
}

func execTestSQL(t *testing.T, db *sql.DB, statement string) {
	t.Helper()

	if _, err := db.Exec(statement); err != nil {
		t.Fatalf("exec %q: %v", statement, err)
	}
}

func quotePostgresIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
