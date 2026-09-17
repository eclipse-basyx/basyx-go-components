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

package integrationtests

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/basyxconfigurationservice"
	"github.com/eclipse-basyx/basyx-go-components/internal/basyxconfigurationservice/sequences"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

const (
	patchPath    = "../../../database/patches/1_2_2.sql"
	basePath     = "../../../database/base.sql"
	patchVersion = "v1.2.2"
)

var affectedTables = []string{
	"submodel_element",
	"submodel_element_payload",
	"submodel_element_semantic_id_reference_payload",
}

func TestAutovacuumPatchPostgres16And18(t *testing.T) {
	for _, version := range []string{"16", "18"} {
		t.Run("postgres-"+version, func(t *testing.T) {
			t.Run("fresh-and-upgrade-with-table-override", func(t *testing.T) {
				db := startPostgres(t, version, false)
				installFreshSchema(t, db)
				require.Equal(t, "default", settingSource(t, db))

				assertMaintenanceOptions(t, db, map[string]string{
					"submodel_element":                               "0.002",
					"submodel_element_payload":                       "0.002",
					"submodel_element_semantic_id_reference_payload": "0.002",
				})
				require.Empty(t, toastOptions(t, db, "submodel_element_payload"))
				assertVersion(t, db)
				applyPatch(t, db)
				assertMaintenanceOptions(t, db, map[string]string{
					"submodel_element":                               "0.002",
					"submodel_element_payload":                       "0.002",
					"submodel_element_semantic_id_reference_payload": "0.002",
				})

				resetOptions(t, db)
				updateVersion(t, db, "v1.2.1")
				applySQLFile(t, db, "testdata/table_override.sql")
				applyPatch(t, db)
				assertMaintenanceOptions(t, db, map[string]string{
					"submodel_element_semantic_id_reference_payload": "0.002",
				})
				assertTableOption(t, db, "submodel_element", "autovacuum_vacuum_threshold=5000")
				assertTableOption(t, db, "submodel_element_payload", "autovacuum_enabled=false")
				assertVersion(t, db)

				resetOptions(t, db)
				updateVersion(t, db, "v1.2.1")
				applySQLFile(t, db, "testdata/scale_override.sql")
				applySQLFile(t, db, "testdata/toast_override.sql")
				applyPatch(t, db)
				assertMaintenanceOptions(t, db, map[string]string{
					"submodel_element": "0.07",
					"submodel_element_semantic_id_reference_payload": "0.002",
				})
				require.Contains(t, toastOptions(t, db, "submodel_element_payload"), "autovacuum_vacuum_threshold=5000")
				assertVersion(t, db)

				resetOptions(t, db)
				updateVersion(t, db, "v1.2.1")
				applySQLFile(t, db, "testdata/index_cleanup_override.sql")
				applyPatch(t, db)
				assertMaintenanceOptions(t, db, map[string]string{
					"submodel_element_payload":                       "0.002",
					"submodel_element_semantic_id_reference_payload": "0.002",
				})
				assertTableOption(t, db, "submodel_element", "vacuum_index_cleanup=off")
				assertVersion(t, db)
			})

			t.Run("global-override", func(t *testing.T) {
				db := startPostgres(t, version, true)
				installFreshSchema(t, db)
				require.Equal(t, "command line", settingSource(t, db))
				assertMaintenanceOptions(t, db, map[string]string{})
				assertVersion(t, db)
			})
		})
	}
}

func startPostgres(t *testing.T, version string, globalOverride bool) *sql.DB {
	t.Helper()
	containerName := fmt.Sprintf("basyx-autovacuum-%s-%d", version, time.Now().UnixNano())
	args := []string{
		"run", "--rm", "-d", "--name", containerName,
		"-e", "POSTGRES_PASSWORD=basyx-test-only",
		"-e", "POSTGRES_DB=basyx_test",
		"-p", "127.0.0.1::5432", "postgres:" + version,
	}
	if globalOverride {
		args = append(args, "postgres", "-c", "autovacuum_vacuum_scale_factor=0.07")
	}
	// #nosec G204 -- arguments use fixed test image tags and a generated container name.
	output, err := exec.CommandContext(t.Context(), "docker", args...).CombinedOutput()
	require.NoErrorf(t, err, "docker run failed: %s", output)
	t.Cleanup(func() {
		// #nosec G204 -- containerName is generated by this test.
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
	})

	// #nosec G204 -- containerName is generated by this test.
	output, err = exec.CommandContext(t.Context(), "docker", "port", containerName, "5432/tcp").CombinedOutput()
	require.NoErrorf(t, err, "docker port failed: %s", output)
	address := strings.TrimSpace(string(output))
	db, err := sql.Open("pgx", fmt.Sprintf("postgres://postgres:basyx-test-only@%s/basyx_test?sslmode=disable", address))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		if db.PingContext(t.Context()) == nil {
			return db
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("BASYXCFG-AUTOVACUUM-POSTGRESREADY PostgreSQL did not become ready")
	return nil
}

func installFreshSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := &sequences.ExecutionContext{Context: t.Context(), DB: db}
	initializer := basyxconfigurationservice.NewSchemaInitializer()
	initializer.Register(sequences.NewSystemTable(ctx))
	initializer.Register(sequences.NewSchemaUpload(ctx, basePath))
	patches, err := filepath.Glob("../../../database/patches/*.sql")
	require.NoError(t, err)
	sort.Slice(patches, func(i, j int) bool {
		first := patchVersionParts(t, patches[i])
		second := patchVersionParts(t, patches[j])
		for index := range first {
			if first[index] != second[index] {
				return first[index] < second[index]
			}
		}
		return false
	})
	for _, patch := range patches {
		version := "v" + strings.ReplaceAll(strings.TrimSuffix(filepath.Base(patch), ".sql"), "_", ".")
		initializer.Register(sequences.NewSchemaPatch(ctx, patch, version))
		if filepath.Base(patch) == filepath.Base(patchPath) {
			break
		}
	}
	require.NoError(t, initializer.Execute())
}

func patchVersionParts(t *testing.T, path string) [3]int {
	t.Helper()
	var parts [3]int
	count, err := fmt.Sscanf(filepath.Base(path), "%d_%d_%d.sql", &parts[0], &parts[1], &parts[2])
	require.NoError(t, err)
	require.Equal(t, 3, count)
	return parts
}

func applySQLFile(t *testing.T, db *sql.DB, path string) {
	t.Helper()
	// #nosec G304 -- paths are repository-controlled schema and fixture files.
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), string(contents))
	require.NoError(t, err)
}

func applyPatch(t *testing.T, db *sql.DB) {
	t.Helper()
	step := sequences.NewSchemaPatch(&sequences.ExecutionContext{Context: t.Context(), DB: db}, patchPath, patchVersion)
	status, err := step.Execute(1)
	require.NoError(t, err)
	require.Zero(t, status)
}

func resetOptions(t *testing.T, db *sql.DB) {
	t.Helper()
	applySQLFile(t, db, "testdata/reset_options.sql")
}

func updateVersion(t *testing.T, db *sql.DB, version string) {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").Update("basyxsystem").
		Set(goqu.Record{"schema_version": version}).Prepared(true).ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
}

func settingSource(t *testing.T, db *sql.DB) string {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("pg_settings").Select("source").
		Where(goqu.C("name").Eq("autovacuum_vacuum_scale_factor")).Prepared(true).ToSQL()
	require.NoError(t, err)
	var source string
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&source))
	return source
}

func assertVersion(t *testing.T, db *sql.DB) {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("basyxsystem").Select("schema_version").
		Prepared(true).ToSQL()
	require.NoError(t, err)
	var version string
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&version))
	require.Equal(t, patchVersion, version)
}

func assertMaintenanceOptions(t *testing.T, db *sql.DB, expected map[string]string) {
	t.Helper()
	for _, table := range affectedTables {
		reloptions := tableOptions(t, db, table)
		option := "autovacuum_vacuum_scale_factor=" + expected[table]
		if _, exists := expected[table]; exists {
			require.Contains(t, reloptions, option, table)
		} else {
			require.NotContains(t, strings.Join(reloptions, ","), "autovacuum_vacuum_scale_factor=", table)
		}
		if expected[table] == "0.002" {
			require.Contains(t, reloptions, "vacuum_index_cleanup=on", table)
		} else {
			require.NotContains(t, reloptions, "vacuum_index_cleanup=on", table)
		}
	}
}

func assertTableOption(t *testing.T, db *sql.DB, table string, option string) {
	t.Helper()
	require.Contains(t, tableOptions(t, db, table), option)
}

func tableOptions(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("pg_class").
		Select(goqu.Func("array_to_string", goqu.C("reloptions"), ",")).
		Where(goqu.C("relname").Eq(table)).Prepared(true).ToSQL()
	require.NoError(t, err)
	var reloptions sql.NullString
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&reloptions))
	return strings.Split(reloptions.String, ",")
}

func toastOptions(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From(goqu.T("pg_class").As("parent")).
		Join(goqu.T("pg_class").As("toast"), goqu.On(goqu.I("toast.oid").Eq(goqu.I("parent.reltoastrelid")))).
		Select(goqu.Func("array_to_string", goqu.I("toast.reloptions"), ",")).
		Where(goqu.I("parent.relname").Eq(table)).Prepared(true).ToSQL()
	require.NoError(t, err)
	var reloptions sql.NullString
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&reloptions))
	return reloptions.String
}
