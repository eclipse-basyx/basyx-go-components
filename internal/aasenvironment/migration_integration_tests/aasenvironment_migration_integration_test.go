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
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strings"
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

type migrationBinaryEndpoints struct {
	file      string
	untouched string
	thumbnail string
}

type legacyBinaryState struct {
	modelPath   string
	contentType string
	fileName    string
	oid         int64
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
	longIdentifier := variedCJKIdentifier(880)
	fixtures := []migrationFixture{
		{endpoint: "/submodels", path: "testdata/submodel.json", id: "urn:basyx:migration:submodel:1"},
		{endpoint: "/submodels", path: "testdata/binary_submodel.json", id: "urn:basyx:migration:binary-submodel:1"},
		{endpoint: "/shells", path: "testdata/shell.json", id: "urn:basyx:migration:shell:1"},
		{endpoint: "/shell-descriptors", path: "testdata/shell_descriptor.json", id: "urn:basyx:migration:shell-descriptor:1"},
		{endpoint: "/submodel-descriptors", path: "testdata/submodel_descriptor.json", id: "urn:basyx:migration:submodel-descriptor:1"},
	}

	require.False(t, databaseTableExists(t, "submodel_supplemental_semantic_id_reference"))
	require.False(t, databaseTableExists(t, "submodel_element_supplemental_semantic_id_reference"))

	for _, fixture := range fixtures {
		postFixture(t, fixture)
	}
	postGeneratedSubmodel(t, longIdentifier)
	assertCollectionsContainFixtures(t, fixtures)

	endpoints := binaryMigrationEndpoints()
	legacyFilePayload := []byte("legacy-file-payload")
	untouchedFilePayload := []byte("legacy-file-payload-that-must-remain-untouched")
	legacyThumbnailPayload := []byte("GIF89a-legacy-thumbnail")
	uploadMultipartBinary(t, endpoints.file, "legacy-file.bin", legacyFilePayload)
	uploadMultipartBinary(t, endpoints.untouched, "legacy-untouched.bin", untouchedFilePayload)
	uploadMultipartBinary(t, endpoints.thumbnail, "legacy-thumbnail.gif", legacyThumbnailPayload)

	legacyFile := readLegacyFileState(t, "LegacyFile")
	legacyUntouched := readLegacyFileState(t, "LegacyFileUntouched")
	legacyThumbnail := readLegacyThumbnailState(t)
	assertBinaryDownload(t, endpoints.file, legacyFilePayload, legacyFile.contentType)
	assertBinaryDownload(t, endpoints.untouched, untouchedFilePayload, legacyUntouched.contentType)
	assertBinaryDownload(t, endpoints.thumbnail, legacyThumbnailPayload, legacyThumbnail.contentType)

	require.NoError(t, runCompose("stop", "old_aas_environment"))
	require.NoError(t, runCompose("rm", "-f", "old_aas_environment", "old_configuration"))
	require.NoError(t, runCompose("up", "-d", "--build", "new_configuration", "new_aas_environment"))
	require.NoError(t, testenv.WaitHealthyURL(migrationBaseURL+"/health", 5*time.Minute))

	assertCollectionsContainFixtures(t, fixtures)
	assertSchemaVersion(t, "v1.1.8")
	assertLongIdentifierEvidenceCatalogAccepts(t, longIdentifier)
	assertLegacyBinaryStateUnchanged(t, legacyFile, readLegacyFileState(t, "LegacyFile"))
	assertLegacyBinaryStateUnchanged(t, legacyUntouched, readLegacyFileState(t, "LegacyFileUntouched"))
	assertLegacyBinaryStateUnchanged(t, legacyThumbnail, readLegacyThumbnailState(t))
	require.Equal(t, int64(0), tableRowCount(t, "binary_content"))
	require.Equal(t, int64(0), tableRowCount(t, "file_binary_reference"))
	require.Equal(t, int64(0), tableRowCount(t, "thumbnail_binary_reference"))
	require.Equal(t, int64(0), tableRowCount(t, "binary_evidence_receipt"))
	assertBinaryDownload(t, endpoints.file, legacyFilePayload, legacyFile.contentType)
	assertBinaryDownload(t, endpoints.untouched, untouchedFilePayload, legacyUntouched.contentType)
	assertBinaryDownload(t, endpoints.thumbnail, legacyThumbnailPayload, legacyThumbnail.contentType)

	replacementPayload := []byte{0x00, 0x01, 0x02, 0x03, 0xff, 0xfe, 0xfd, 0xfc}
	uploadMultipartBinary(t, endpoints.file, "replacement.bin", replacementPayload)
	uploadMultipartBinary(t, endpoints.thumbnail, "replacement.bin", replacementPayload)
	assertCanonicalReplacementState(t, legacyFile, legacyUntouched, legacyThumbnail)
	assertBinaryDownload(t, endpoints.file, replacementPayload, "")
	assertBinaryDownload(t, endpoints.thumbnail, replacementPayload, "")
	assertBinaryDownload(t, endpoints.untouched, untouchedFilePayload, legacyUntouched.contentType)
	require.Equal(t, int64(0), tableRowCount(t, "binary_evidence_receipt"))

	require.Equal(t, int64(2), tableRowCount(t, "submodel_supplemental_semantic_id_reference"))
	require.Equal(t, int64(2), tableRowCount(t, "submodel_element_supplemental_semantic_id_reference"))
	assertMigratedSupplementalSemanticIDsAreQueryable(t)
}

func variedCJKIdentifier(length int) string {
	identifier := make([]rune, length)
	for index := range identifier {
		identifier[index] = rune(0x4e00 + index%2000)
	}
	return string(identifier)
}

func postGeneratedSubmodel(t *testing.T, identifier string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"id": identifier, "idShort": "LongIdentifier", "kind": "Instance", "modelType": "Submodel",
	})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, migrationBaseURL+"/submodels", bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := testenv.HTTPClient().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "long-identifier submodel creation failed: %s", string(responseBody))
}

func assertLongIdentifierEvidenceCatalogAccepts(t *testing.T, identifier string) {
	t.Helper()
	db := openMigrationDatabase(t)
	defer func() { _ = db.Close() }()
	digestBytes := sha256.Sum256([]byte(identifier))
	digest := hex.EncodeToString(digestBytes[:])
	stateQuery, stateArgs, err := goqu.Insert("mutation_evidence_state").Rows(goqu.Record{
		"entity_type": "submodel_history", "identifier": identifier, "identifier_digest": digest,
		"last_sequence": 1, "last_event_hash": strings.Repeat("a", 64), "last_content_hash": strings.Repeat("b", 64),
	}).ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), stateQuery, stateArgs...)
	require.NoError(t, err)
	artifactQuery, artifactArgs, err := goqu.Insert("mutation_evidence_artifacts").Rows(goqu.Record{
		"entity_type": "submodel_history", "identifier": identifier, "identifier_digest": digest,
		"event_sequence": 1, "event_hash": strings.Repeat("a", 64), "content_hash": strings.Repeat("b", 64),
		"payload_hash": strings.Repeat("c", 64), "payload_type": "snapshot", "provider": "s3",
		"object_key": "migration-test/long-identifier", "sha256": strings.Repeat("d", 64),
		"size_bytes": 1, "content_type": "application/json",
	}).ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), artifactQuery, artifactArgs...)
	require.NoError(t, err)
}

func binaryMigrationEndpoints() migrationBinaryEndpoints {
	submodelID := base64.RawURLEncoding.EncodeToString([]byte("urn:basyx:migration:binary-submodel:1"))
	aasID := base64.RawURLEncoding.EncodeToString([]byte("urn:basyx:migration:shell:1"))
	base := strings.TrimRight(migrationBaseURL, "/")
	attachmentBase := base + "/submodels/" + submodelID + "/submodel-elements/"
	return migrationBinaryEndpoints{
		file:      attachmentBase + "LegacyFile/attachment",
		untouched: attachmentBase + "LegacyFileUntouched/attachment",
		thumbnail: base + "/shells/" + aasID + "/asset-information/thumbnail",
	}
}

func uploadMultipartBinary(t *testing.T, endpoint string, fileName string, payload []byte) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("fileName", fileName))
	part, err := writer.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = part.Write(payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, endpoint, body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := testenv.HTTPClient().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusNoContent, resp.StatusCode, "binary upload failed: %s", string(responseBody))
}

func assertBinaryDownload(t *testing.T, endpoint string, expected []byte, expectedContentType string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint, nil)
	require.NoError(t, err)
	resp, err := testenv.HTTPClient().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "binary download failed: %s", string(body))
	require.Equal(t, expected, body)
	actualContentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	require.NotEmpty(t, actualContentType)
	if strings.TrimSpace(expectedContentType) != "" {
		require.Equal(t, expectedContentType, actualContentType)
	}
}

func readLegacyFileState(t *testing.T, idShortPath string) legacyBinaryState {
	t.Helper()
	db := openMigrationDatabase(t)
	defer func() { _ = db.Close() }()
	query, args, err := goqu.From(goqu.T("submodel").As("sm")).
		Join(goqu.T("submodel_element").As("sme"), goqu.On(goqu.I("sme.submodel_id").Eq(goqu.I("sm.id")))).
		Join(goqu.T("file_element").As("fe"), goqu.On(goqu.I("fe.id").Eq(goqu.I("sme.id")))).
		Join(goqu.T("file_data").As("fd"), goqu.On(goqu.I("fd.id").Eq(goqu.I("fe.id")))).
		Select(goqu.I("fe.value"), goqu.I("fe.content_type"), goqu.I("fe.file_name"), goqu.I("fd.file_oid")).
		Where(goqu.Ex{"sm.submodel_identifier": "urn:basyx:migration:binary-submodel:1", "sme.idshort_path": idShortPath}).
		ToSQL()
	require.NoError(t, err)
	return scanLegacyBinaryState(t, db.QueryRowContext(t.Context(), query, args...))
}

func readLegacyThumbnailState(t *testing.T) legacyBinaryState {
	t.Helper()
	db := openMigrationDatabase(t)
	defer func() { _ = db.Close() }()
	query, args, err := goqu.From(goqu.T("aas").As("a")).
		Join(goqu.T("thumbnail_file_element").As("tfe"), goqu.On(goqu.I("tfe.id").Eq(goqu.I("a.id")))).
		Join(goqu.T("thumbnail_file_data").As("tfd"), goqu.On(goqu.I("tfd.id").Eq(goqu.I("tfe.id")))).
		Select(goqu.I("tfe.value"), goqu.I("tfe.content_type"), goqu.I("tfe.file_name"), goqu.I("tfd.file_oid")).
		Where(goqu.I("a.aas_id").Eq("urn:basyx:migration:shell:1")).ToSQL()
	require.NoError(t, err)
	return scanLegacyBinaryState(t, db.QueryRowContext(t.Context(), query, args...))
}

func scanLegacyBinaryState(t *testing.T, row *sql.Row) legacyBinaryState {
	t.Helper()
	var state legacyBinaryState
	var modelPath, contentType, fileName sql.NullString
	require.NoError(t, row.Scan(&modelPath, &contentType, &fileName, &state.oid))
	state.modelPath = modelPath.String
	state.contentType = contentType.String
	state.fileName = fileName.String
	require.Positive(t, state.oid)
	return state
}

func assertLegacyBinaryStateUnchanged(t *testing.T, expected legacyBinaryState, actual legacyBinaryState) {
	t.Helper()
	require.Equal(t, expected, actual)
	require.True(t, largeObjectExists(t, expected.oid))
}

func assertCanonicalReplacementState(t *testing.T, replacedFile legacyBinaryState, untouchedFile legacyBinaryState, replacedThumbnail legacyBinaryState) {
	t.Helper()
	db := openMigrationDatabase(t)
	defer func() { _ = db.Close() }()

	query, args, err := goqu.From(goqu.T("binary_content").As("bc")).
		Join(goqu.T("file_binary_reference").As("fbr"), goqu.On(goqu.I("fbr.binary_content_id").Eq(goqu.I("bc.id")))).
		Join(goqu.T("thumbnail_binary_reference").As("tbr"), goqu.On(goqu.I("tbr.binary_content_id").Eq(goqu.I("bc.id")))).
		Join(goqu.T("file_element").As("fe"), goqu.On(goqu.I("fe.id").Eq(goqu.I("fbr.file_element_id")))).
		Join(goqu.T("thumbnail_file_element").As("tfe"), goqu.On(goqu.I("tfe.id").Eq(goqu.I("tbr.thumbnail_element_id")))).
		Select(goqu.I("bc.reference_count"), goqu.I("bc.file_oid"), goqu.I("fe.value"), goqu.I("tfe.value")).ToSQL()
	require.NoError(t, err)
	var referenceCount, canonicalOID int64
	var filePath, thumbnailPath string
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&referenceCount, &canonicalOID, &filePath, &thumbnailPath))
	require.Equal(t, int64(2), referenceCount)
	require.Positive(t, canonicalOID)
	require.NotEqual(t, filePath, thumbnailPath)
	require.True(t, strings.HasPrefix(filePath, "/aasx/files/"))
	require.True(t, strings.HasPrefix(thumbnailPath, "/aasx/files/"))
	require.Equal(t, int64(1), tableRowCount(t, "binary_content"))
	require.Equal(t, int64(1), tableRowCount(t, "file_binary_reference"))
	require.Equal(t, int64(1), tableRowCount(t, "thumbnail_binary_reference"))
	require.False(t, largeObjectExists(t, replacedFile.oid))
	require.False(t, largeObjectExists(t, replacedThumbnail.oid))
	require.True(t, largeObjectExists(t, untouchedFile.oid))
	require.True(t, largeObjectExists(t, canonicalOID))

	legacyFileCountQuery, legacyFileCountArgs, err := goqu.From(goqu.T("file_data").As("fd")).
		Join(goqu.T("submodel_element").As("sme"), goqu.On(goqu.I("sme.id").Eq(goqu.I("fd.id")))).
		Select(goqu.COUNT("*")).Where(goqu.I("sme.idshort_path").Eq("LegacyFile")).ToSQL()
	require.NoError(t, err)
	var replacedLegacyFileRows int64
	require.NoError(t, db.QueryRowContext(t.Context(), legacyFileCountQuery, legacyFileCountArgs...).Scan(&replacedLegacyFileRows))
	require.Zero(t, replacedLegacyFileRows)
	require.Equal(t, int64(1), tableRowCount(t, "file_data"))
	require.Equal(t, int64(0), tableRowCount(t, "thumbnail_file_data"))
}

func largeObjectExists(t *testing.T, oid int64) bool {
	t.Helper()
	db := openMigrationDatabase(t)
	defer func() { _ = db.Close() }()
	query, args, err := goqu.From("pg_largeobject_metadata").Select(goqu.COUNT("*")).Where(goqu.C("oid").Eq(oid)).ToSQL()
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&count))
	return count == 1
}

func assertSchemaVersion(t *testing.T, expected string) {
	t.Helper()
	db := openMigrationDatabase(t)
	defer func() { _ = db.Close() }()
	query, args, err := goqu.From("basyxsystem").Select("schema_version").Order(goqu.C("identifier").Asc()).Limit(1).ToSQL()
	require.NoError(t, err)
	var actual string
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&actual))
	require.Equal(t, expected, strings.TrimSpace(actual))
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
		"binary_content":                                      {},
		"binary_evidence_receipt":                             {},
		"file_data":                                           {},
		"file_binary_reference":                               {},
		"thumbnail_binary_reference":                          {},
		"thumbnail_file_data":                                 {},
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
