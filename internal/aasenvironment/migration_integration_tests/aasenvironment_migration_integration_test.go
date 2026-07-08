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

//nolint:all
package migrationintegrationtests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

const (
	migrationComposeFile = "./docker_compose/docker_compose.yml"
)

var (
	migrationBaseURL  = testenv.LocalURLFromEnv("BASYX_IT_API_PORT", 6034)
	migrationDSN      = testenv.PostgresKeywordDSNFromEnv("BASYX_IT_DB_PORT", 6434, "basyxMigrationTestDB")
	composeBinary     string
	composePrefix     []string
	composeProject    string
	composeRuntimeEnv []string
)

type migrationFixture struct {
	endpoint string
	path     string
	id       string
}

func TestMain(m *testing.M) {
	runtime := testenv.NewComposeRuntimeOrExit("aasenvironment-migration", []testenv.PortBinding{
		{Name: "api", EnvVar: "BASYX_IT_API_PORT"},
		{Name: "db", EnvVar: "BASYX_IT_DB_PORT"},
	})
	migrationBaseURL = runtime.LocalURL("api")
	migrationDSN = runtime.PostgresKeywordDSN("db", "basyxMigrationTestDB")
	composeProject = runtime.ProjectName
	composeRuntimeEnv = runtime.Env()

	var err error
	composeBinary, composePrefix, err = testenv.FindCompose()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	_ = runCompose("down", "-v", "--remove-orphans")
	if err = runCompose("up", "-d", "migration_db", "old_configuration", "old_aas_environment"); err != nil {
		fmt.Fprintf(os.Stderr, "AASEMV-MIGRATION-STARTOLD: %v\n", err)
		_ = runCompose("down", "-v", "--remove-orphans")
		os.Exit(1)
	}
	if err = testenv.WaitHealthyURL(migrationBaseURL+"/health", 5*time.Minute); err != nil {
		fmt.Fprintf(os.Stderr, "AASEMV-MIGRATION-HEALTHOLD: %v\n", err)
		_ = runCompose("down", "-v", "--remove-orphans")
		os.Exit(1)
	}

	exitCode := m.Run()
	if err = runCompose("down", "-v", "--remove-orphans"); err != nil && exitCode == 0 {
		fmt.Fprintf(os.Stderr, "AASEMV-MIGRATION-CLEANUP: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

func TestMigrationFromReleaseCandidate5PreservesEnvironmentData(t *testing.T) {
	fixtures := []migrationFixture{
		{endpoint: "/submodels", path: "testdata/submodel.json", id: "urn:basyx:migration:submodel:1"},
		{endpoint: "/shells", path: "testdata/shell.json", id: "urn:basyx:migration:shell:1"},
		{endpoint: "/shell-descriptors", path: "testdata/shell_descriptor.json", id: "urn:basyx:migration:shell-descriptor:1"},
		{endpoint: "/submodel-descriptors", path: "testdata/submodel_descriptor.json", id: "urn:basyx:migration:submodel-descriptor:1"},
	}

	require.False(t, databaseTableExists(t, "submodel_supplemental_semantic_id_reference"))
	require.False(t, databaseTableExists(t, "submodel_element_supplemental_semantic_id_reference"))

	for _, fixture := range fixtures {
		postFixture(t, fixture)
	}
	assertCollectionsContainFixtures(t, fixtures)

	require.NoError(t, runCompose("stop", "old_aas_environment"))
	require.NoError(t, runCompose("rm", "-f", "old_aas_environment", "old_configuration"))
	require.NoError(t, runCompose("up", "-d", "--build", "new_configuration", "new_aas_environment"))
	require.NoError(t, testenv.WaitHealthyURL(migrationBaseURL+"/health", 5*time.Minute))

	assertCollectionsContainFixtures(t, fixtures)
	require.Equal(t, int64(2), tableRowCount(t, "submodel_supplemental_semantic_id_reference"))
	require.Equal(t, int64(2), tableRowCount(t, "submodel_element_supplemental_semantic_id_reference"))
	assertMigratedSupplementalSemanticIDsAreQueryable(t)
}

func runCompose(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	commandArgs := append([]string{}, composePrefix...)
	commandArgs = append(commandArgs, "-f", migrationComposeFile)
	if composeProject != "" {
		commandArgs = append(commandArgs, "-p", composeProject)
	}
	commandArgs = append(commandArgs, args...)

	cmd := exec.CommandContext(ctx, composeBinary, commandArgs...)
	cmd.Env = append(os.Environ(), composeRuntimeEnv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("AASEMV-MIGRATION-COMPOSETIMEOUT: %w", ctx.Err())
		}
		return fmt.Errorf("AASEMV-MIGRATION-COMPOSE: %w", err)
	}
	return nil
}

func postFixture(t *testing.T, fixture migrationFixture) {
	t.Helper()

	body, err := os.ReadFile(fixture.path)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, migrationBaseURL+fixture.endpoint, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testenv.HTTPClient().Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "POST %s failed: %s", fixture.endpoint, string(responseBody))
}

func assertCollectionsContainFixtures(t *testing.T, fixtures []migrationFixture) {
	t.Helper()

	for _, fixture := range fixtures {
		expected := readJSONObject(t, fixture.path)
		collection := getCollection(t, fixture.endpoint)
		actual := findResourceByID(t, collection, fixture.id)
		assertJSONContains(t, expected, actual, fixture.endpoint)
	}
}

func getCollection(t *testing.T, endpoint string) []any {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, migrationBaseURL+endpoint, nil)
	require.NoError(t, err)
	resp, err := testenv.HTTPClient().Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "GET %s failed: %s", endpoint, string(body))

	var response struct {
		Result []any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &response))
	return response.Result
}

func assertMigratedSupplementalSemanticIDsAreQueryable(t *testing.T) {
	t.Helper()

	t.Run("submodel and submodel element filters", func(t *testing.T) {
		result := postQuery(t, "/query/submodels", map[string]any{
			"$condition": map[string]any{
				"$eq": []any{
					map[string]any{"$field": "$sme#supplementalSemanticIds[].keys[].value"},
					map[string]any{"$strVal": "urn:basyx:migration:sme:supplemental:1"},
				},
			},
			"$filters": []any{
				map[string]any{
					"$fragment": "$sm#supplementalSemanticIds[]",
					"$condition": map[string]any{
						"$eq": []any{
							map[string]any{"$field": "$sm#supplementalSemanticIds[].keys[].value"},
							map[string]any{"$strVal": "urn:basyx:migration:submodel:supplemental:2"},
						},
					},
				},
				map[string]any{
					"$fragment": "$sme#supplementalSemanticIds[]",
					"$condition": map[string]any{
						"$eq": []any{
							map[string]any{"$field": "$sme#supplementalSemanticIds[].keys[].value"},
							map[string]any{"$strVal": "urn:basyx:migration:sme:supplemental:1"},
						},
					},
				},
			},
		})

		submodel := requireSingleQueryResult(t, result, "urn:basyx:migration:submodel:1")
		assertSingleSupplementalSemanticID(t, submodel, "urn:basyx:migration:submodel:supplemental:2")
		elements := requireJSONArray(t, submodel, "submodelElements")
		require.Len(t, elements, 1)
		element, ok := elements[0].(map[string]any)
		require.True(t, ok)
		assertSingleSupplementalSemanticID(t, element, "urn:basyx:migration:sme:supplemental:1")
	})

	t.Run("submodel descriptor filter", func(t *testing.T) {
		result := postQuery(t, "/query/submodel-descriptors", map[string]any{
			"$condition": map[string]any{
				"$eq": []any{
					map[string]any{"$field": "$smdesc#supplementalSemanticIds[].keys[].value"},
					map[string]any{"$strVal": "urn:basyx:migration:descriptor:supplemental:1"},
				},
			},
			"$filters": []any{
				map[string]any{
					"$fragment": "$smdesc#supplementalSemanticIds[]",
					"$condition": map[string]any{
						"$eq": []any{
							map[string]any{"$field": "$smdesc#supplementalSemanticIds[].keys[].value"},
							map[string]any{"$strVal": "urn:basyx:migration:descriptor:supplemental:2"},
						},
					},
				},
			},
		})

		descriptor := requireSingleQueryResult(t, result, "urn:basyx:migration:submodel-descriptor:1")
		assertSingleSupplementalSemanticID(t, descriptor, "urn:basyx:migration:descriptor:supplemental:2")
	})

	t.Run("nested submodel descriptor filter", func(t *testing.T) {
		result := postQuery(t, "/query/shell-descriptors", map[string]any{
			"$condition": map[string]any{
				"$eq": []any{
					map[string]any{"$field": "$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value"},
					map[string]any{"$strVal": "urn:basyx:migration:nested:supplemental:1"},
				},
			},
			"$filters": []any{
				map[string]any{
					"$fragment": "$aasdesc#submodelDescriptors[].supplementalSemanticIds[]",
					"$condition": map[string]any{
						"$eq": []any{
							map[string]any{"$field": "$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value"},
							map[string]any{"$strVal": "urn:basyx:migration:nested:supplemental:2"},
						},
					},
				},
			},
		})

		shellDescriptor := requireSingleQueryResult(t, result, "urn:basyx:migration:shell-descriptor:1")
		submodelDescriptors := requireJSONArray(t, shellDescriptor, "submodelDescriptors")
		require.Len(t, submodelDescriptors, 1)
		descriptor, ok := submodelDescriptors[0].(map[string]any)
		require.True(t, ok)
		assertSingleSupplementalSemanticID(t, descriptor, "urn:basyx:migration:nested:supplemental:2")
	})
}

func postQuery(t *testing.T, endpoint string, query map[string]any) []any {
	t.Helper()

	body, err := json.Marshal(query)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, migrationBaseURL+endpoint, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testenv.HTTPClient().Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "POST %s failed: %s", endpoint, string(responseBody))

	var response struct {
		Result []any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(responseBody, &response))
	return response.Result
}

func requireSingleQueryResult(t *testing.T, result []any, expectedID string) map[string]any {
	t.Helper()

	require.Len(t, result, 1)
	object, ok := result[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, expectedID, object["id"])
	return object
}

func assertSingleSupplementalSemanticID(t *testing.T, object map[string]any, expectedValue string) {
	t.Helper()

	references := requireJSONArray(t, object, "supplementalSemanticIds")
	require.Len(t, references, 1)
	reference, ok := references[0].(map[string]any)
	require.True(t, ok)
	keys := requireJSONArray(t, reference, "keys")
	require.Len(t, keys, 1)
	key, ok := keys[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, expectedValue, key["value"])
}

func requireJSONArray(t *testing.T, object map[string]any, field string) []any {
	t.Helper()

	value, exists := object[field]
	require.Truef(t, exists, "missing field %s", field)
	array, ok := value.([]any)
	require.Truef(t, ok, "field %s is %T, expected array", field, value)
	return array
}

func readJSONObject(t *testing.T, path string) map[string]any {
	t.Helper()

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	return result
}

func findResourceByID(t *testing.T, resources []any, id string) map[string]any {
	t.Helper()

	for _, resource := range resources {
		object, ok := resource.(map[string]any)
		if ok && object["id"] == id {
			return object
		}
	}
	t.Fatalf("resource %q not found in collection", id)
	return nil
}

func assertJSONContains(t *testing.T, expected any, actual any, path string) {
	t.Helper()

	switch expectedValue := expected.(type) {
	case map[string]any:
		actualValue, ok := actual.(map[string]any)
		require.Truef(t, ok, "%s: expected object, got %T", path, actual)
		for key, expectedChild := range expectedValue {
			actualChild, exists := actualValue[key]
			require.Truef(t, exists, "%s.%s: missing field", path, key)
			assertJSONContains(t, expectedChild, actualChild, path+"."+key)
		}
	case []any:
		actualValue, ok := actual.([]any)
		require.Truef(t, ok, "%s: expected array, got %T", path, actual)
		require.Lenf(t, actualValue, len(expectedValue), "%s: array length differs", path)
		for index := range expectedValue {
			assertJSONContains(t, expectedValue[index], actualValue[index], fmt.Sprintf("%s[%d]", path, index))
		}
	default:
		require.Equalf(t, expected, actual, "%s: value differs", path)
	}
}

func databaseTableExists(t *testing.T, tableName string) bool {
	t.Helper()

	db := openMigrationDatabase(t)
	defer func() {
		_ = db.Close()
	}()

	query, args, err := goqu.Dialect("postgres").
		From(goqu.T("tables").Schema("information_schema")).
		Select(goqu.COUNT("*")).
		Where(
			goqu.Ex{
				"table_schema": "public",
				"table_name":   tableName,
			},
		).
		ToSQL()
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&count))
	return count > 0
}

func tableRowCount(t *testing.T, tableName string) int64 {
	t.Helper()

	allowedTables := map[string]struct{}{
		"submodel_supplemental_semantic_id_reference":         {},
		"submodel_element_supplemental_semantic_id_reference": {},
	}
	_, allowed := allowedTables[tableName]
	require.True(t, allowed, "unsupported table: %s", tableName)

	db := openMigrationDatabase(t)
	defer func() {
		_ = db.Close()
	}()

	query, args, err := goqu.Dialect("postgres").
		From(goqu.T(tableName)).
		Select(goqu.COUNT("*")).
		ToSQL()
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&count))
	return count
}

func openMigrationDatabase(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("pgx", migrationDSN)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(t.Context()))
	return db
}
