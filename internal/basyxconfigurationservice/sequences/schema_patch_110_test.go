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
	if !strings.Contains(patchSQL, "UPDATE asset_information") ||
		!strings.Contains(patchSQL, "UPDATE aas_descriptor") ||
		!strings.Contains(patchSQL, "SET asset_kind = asset_kind + 1") ||
		!strings.Contains(patchSQL, "WHERE asset_kind >= 2") {
		t.Fatalf("patch %s must migrate asset_kind indices by shifting values >= 2 by +1", schemaPatch110Path)
	}

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
		"UPDATE asset_information",
		"UPDATE aas_descriptor",
		"SET asset_kind = asset_kind + 1",
		"WHERE asset_kind >= 2",
		"INSERT INTO aas_history",
		"INSERT INTO submodel_history",
		"INSERT INTO concept_description_history",
		"INSERT INTO descriptor_history",
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
		if strings.EqualFold(os.Getenv("CI"), "true") || strings.EqualFold(os.Getenv("GITHUB_ACTIONS"), "true") {
			t.Fatalf("BASYX_SCHEMA_PATCH_TEST_DSN must be set in CI to verify asset_kind migration behavior")
		}
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
	execTestSQL(t, db, "INSERT INTO aas (id, aas_id, id_short) VALUES (1, 'urn:test:aas:existing', 'ExistingAAS')")
	execTestSQL(t, db, "INSERT INTO aas_payload (aas_id, administrative_information_payload) VALUES (1, '{\"createdAt\":\"2030-01-02T03:04:05Z\",\"updatedAt\":\"2030-01-02T03:04:06Z\"}'::jsonb)")
	execTestSQL(t, db, "INSERT INTO submodel (id, submodel_identifier, id_short, kind) VALUES (1, 'urn:test:submodel:existing', 'ExistingSubmodel', 0)")
	execTestSQL(t, db, "INSERT INTO submodel_payload (submodel_id, administrative_information_payload) VALUES (1, '{\"createdAt\":\"2030-01-03T03:04:05Z\",\"updatedAt\":\"2030-01-03T03:04:06Z\"}'::jsonb)")
	execTestSQL(t, db, "INSERT INTO descriptor (id) VALUES (1)")
	execTestSQL(t, db, "INSERT INTO aas_descriptor (descriptor_id, id, id_short, asset_kind, asset_type) VALUES (1, 'urn:test:descriptor:existing', 'ExistingDescriptor', 2, 'asset-type')")
	execTestSQL(t, db, "INSERT INTO descriptor_payload (descriptor_id, description_payload, displayname_payload, administrative_information_payload) VALUES (1, '[]'::jsonb, '[]'::jsonb, '{\"createdAt\":\"2030-01-04T03:04:05Z\",\"updatedAt\":\"2030-01-04T03:04:06Z\"}'::jsonb)")
	execTestSQL(t, db, "INSERT INTO concept_description (id, id_short, data) VALUES ('urn:test:cd:existing', 'ExistingConcept', '{\"id\":\"urn:test:cd:existing\",\"idShort\":\"ExistingConcept\",\"modelType\":\"ConceptDescription\",\"administration\":{\"createdAt\":\"2030-01-05T03:04:05Z\",\"updatedAt\":\"2030-01-05T03:04:06Z\"}}'::jsonb)")

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
	var migratedDescriptorAssetKind int
	if err = db.QueryRow("SELECT asset_kind FROM aas_descriptor WHERE descriptor_id = 1").Scan(&migratedDescriptorAssetKind); err != nil {
		t.Fatalf("read migrated descriptor asset_kind: %v", err)
	}
	if migratedDescriptorAssetKind != 3 {
		t.Fatalf("expected descriptor asset_kind 2 to migrate to 3, got %d", migratedDescriptorAssetKind)
	}
	assertHistoryBackfill(t, db, "aas_history", "urn:test:aas:existing", "Role")
	assertHistoryBackfill(t, db, "submodel_history", "urn:test:submodel:existing", "")
	assertHistoryBackfill(t, db, "descriptor_history", "urn:test:descriptor:existing", "Role")
	assertHistoryBackfill(t, db, "concept_description_history", "urn:test:cd:existing", "")
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
		"CREATE TABLE aas (id BIGINT PRIMARY KEY, aas_id VARCHAR(2048) UNIQUE NOT NULL, id_short VARCHAR(128), category VARCHAR(128), db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())",
		"CREATE TABLE aas_payload (aas_id BIGINT PRIMARY KEY, administrative_information_payload JSONB)",
		"CREATE TABLE submodel (id BIGINT PRIMARY KEY, submodel_identifier VARCHAR(2048) UNIQUE NOT NULL, id_short VARCHAR(128), category VARCHAR(128), kind INT, db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())",
		"CREATE TABLE submodel_payload (submodel_id BIGINT PRIMARY KEY, administrative_information_payload JSONB)",
		"CREATE TABLE submodel_element_payload (submodel_element_id BIGINT PRIMARY KEY, administrative_information_payload JSONB)",
		"CREATE TABLE descriptor (id BIGINT PRIMARY KEY, db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())",
		"CREATE TABLE aas_descriptor (descriptor_id BIGINT PRIMARY KEY, id VARCHAR(2048) UNIQUE NOT NULL, id_short VARCHAR(128), asset_kind INT, asset_type VARCHAR(2048), global_asset_id VARCHAR(2048), db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())",
		"CREATE TABLE descriptor_payload (descriptor_id BIGINT PRIMARY KEY, description_payload JSONB NOT NULL, displayname_payload JSONB NOT NULL, administrative_information_payload JSONB NOT NULL)",
		"CREATE TABLE concept_description (id TEXT PRIMARY KEY, id_short TEXT, data JSONB, db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())",
		"CREATE TABLE asset_information (asset_information_id BIGINT PRIMARY KEY, asset_kind INT, global_asset_id VARCHAR(2048), asset_type VARCHAR(2048), model_type INT NOT NULL DEFAULT 4, db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), db_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())",
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

func assertHistoryBackfill(t *testing.T, db *sql.DB, table string, identifier string, expectedAssetKind string) {
	t.Helper()

	var snapshot string
	query, ok := historyBackfillQuery(table)
	if !ok {
		t.Fatalf("unsupported history table %s", table)
	}
	if err := db.QueryRow(query, identifier).Scan(&snapshot); err != nil {
		t.Fatalf("read %s backfill for %s: %v", table, identifier, err)
	}
	if !strings.Contains(snapshot, identifier) {
		t.Fatalf("expected %s snapshot to contain identifier %s, got %s", table, identifier, snapshot)
	}
	if expectedAssetKind != "" && !strings.Contains(snapshot, `"assetKind": "`+expectedAssetKind+`"`) {
		t.Fatalf("expected %s snapshot to contain assetKind %s, got %s", table, expectedAssetKind, snapshot)
	}
}

func historyBackfillQuery(table string) (string, bool) {
	switch table {
	case "aas_history":
		return "SELECT snapshot::text FROM aas_history WHERE identifier = $1 AND change_type = 'Created' AND deleted = FALSE", true
	case "submodel_history":
		return "SELECT snapshot::text FROM submodel_history WHERE identifier = $1 AND change_type = 'Created' AND deleted = FALSE", true
	case "descriptor_history":
		return "SELECT snapshot::text FROM descriptor_history WHERE identifier = $1 AND change_type = 'Created' AND deleted = FALSE", true
	case "concept_description_history":
		return "SELECT snapshot::text FROM concept_description_history WHERE identifier = $1 AND change_type = 'Created' AND deleted = FALSE", true
	default:
		return "", false
	}
}
