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

package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

const actionUploadAASXMultipart = "UPLOAD_AASX_MULTIPART"
const actionUploadMultipart = "UPLOAD_MULTIPART"
const actionVerifyAASXAttachments = "VERIFY_AASX_ATTACHMENTS"
const actionVerifyAASXThumbnail = "VERIFY_AASX_THUMBNAIL"
const actionVerifyEndpointSnapshot = "VERIFY_ENDPOINT_SNAPSHOT"
const expectationRequired = "required"
const expectationAbsent = "absent"
const expectationOptional = "optional"
const uploadHeaderPartContentType = "X-Upload-Part-ContentType"
const uploadHeaderPartFileName = "X-Upload-Part-FileName"
const uploadHeaderPartOmitFileName = "X-Upload-Part-OmitFileName"
const uploadHeaderRequestFileName = "X-Upload-FileName"
const uploadHeaderExpectedErrorContains = "X-Upload-Expected-Error-Contains"

var uploadIntegrationDSN = testenv.PostgresKeywordDSNFromEnv("BASYX_IT_DB_PORT", 6432, "basyxTestDB")
var uploadSyncDisabledIntegrationDSN = testenv.PostgresKeywordDSNFromEnv("BASYX_IT_SYNC_OFF_DB_PORT", 6433, "basyxTestDBSyncOff")

const uploadHeaderExpectedErrorContainsSecondary = "X-Upload-Expected-Error-Contains-Secondary"

var dynamicBinaryReferencePattern = regexp.MustCompile(`^(?:\d+|/aasx/files/[A-Za-z0-9_-]{32}/[^/]+)$`)

type storedAttachment struct {
	SubmodelIdentifier string
	IDShortPath        string
	FileReference      string
	ContentType        string
}

func TestUploadAASXIntegration(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	runUploadJSONSuite(t, "upload_it_config.json")
}

func TestUploadAASXIntegrationProductionPlan(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	runUploadJSONSuite(t, "upload_productionplan_it_config.json")
}

func TestUploadAASXIntegrationSupportsLegacyHARTINGNamespace(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	runUploadJSONSuite(t, "upload_harting_it_config.json")
}

func TestUploadJSONAndXMLIntegration(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	runUploadJSONSuite(t, "upload_json_xml_config.json")
}

func TestRegistrySyncIntegration(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	runUploadJSONSuite(t, "registry_sync_it_config.json")
}

func TestRegistrySyncDisabledIntegration(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadSyncDisabledIntegrationDSN)
	waitForIntegrationHealth(t, aasEnvSyncOffBaseURL+"/health", 2*time.Minute)
	runUploadJSONSuite(t, "registry_sync_disabled_it_config.json")
}

func runUploadJSONSuite(t *testing.T, configPath string) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ConfigPath: configPath,
		ActionHandlers: map[string]testenv.JSONStepAction{
			actionUploadAASXMultipart: func(t *testing.T, _ *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, _ int) {
				runMultipartUploadAction(t, step)
			},
			actionUploadMultipart: func(t *testing.T, _ *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, _ int) {
				runMultipartUploadAction(t, step)
			},
			actionVerifyAASXAttachments: func(t *testing.T, _ *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, _ int) {
				verifyStoredAttachments(t, step)
			},
			actionVerifyAASXThumbnail: func(t *testing.T, _ *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, _ int) {
				verifyThumbnailEndpoints(t, step)
			},
			actionVerifyEndpointSnapshot: func(t *testing.T, _ *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, _ int) {
				verifyEndpointSnapshot(t, step)
			},
		},
	})
}

func waitForIntegrationHealth(t *testing.T, endpoint string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 3 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(endpoint)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("integration endpoint not healthy within timeout: %s", endpoint)
}

func resetDatabaseForUploadIT(t *testing.T, dsn string) {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	rows, err := db.Query("SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	tables := make([]string, 0, 64)
	for rows.Next() {
		var table string
		require.NoError(t, rows.Scan(&table))
		tables = append(tables, table)
	}
	require.NoError(t, rows.Err())

	for _, table := range tables {
		truncate := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", quoteIdentifierUploadIT(table))
		_, execErr := db.Exec(truncate)
		require.NoErrorf(t, execErr, "failed to truncate table %s", table)
	}
}

func quoteIdentifierUploadIT(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func runMultipartUploadAction(t *testing.T, step testenv.JSONSuiteStep) {
	file, err := os.Open(step.Data)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)

	partFileName := filepath.Base(step.Data)
	if step.Headers != nil {
		if value, ok := step.Headers[uploadHeaderPartFileName]; ok && strings.TrimSpace(value) != "" {
			partFileName = strings.TrimSpace(value)
		}
	}

	partContentType := ""
	if step.Headers != nil {
		if value, ok := step.Headers[uploadHeaderPartContentType]; ok {
			partContentType = strings.TrimSpace(value)
		}
	}

	omitPartFileName := false
	if step.Headers != nil {
		if value, ok := step.Headers[uploadHeaderPartOmitFileName]; ok {
			omitPartFileName = strings.EqualFold(strings.TrimSpace(value), "true")
		}
	}

	var filePart io.Writer
	if partContentType == "" && !omitPartFileName {
		filePart, err = writer.CreateFormFile("file", partFileName)
	} else {
		partHeader := textproto.MIMEHeader{}
		if omitPartFileName {
			partHeader.Set("Content-Disposition", fmt.Sprintf("form-data; name=%q", "file"))
		} else {
			partHeader.Set("Content-Disposition", fmt.Sprintf("form-data; name=%q; filename=%q", "file", partFileName))
		}
		if partContentType != "" {
			partHeader.Set("Content-Type", partContentType)
		}
		filePart, err = writer.CreatePart(partHeader)
	}
	require.NoError(t, err)

	_, err = io.Copy(filePart, file)
	require.NoError(t, err)

	if step.Headers != nil {
		if value, ok := step.Headers[uploadHeaderRequestFileName]; ok && strings.TrimSpace(value) != "" {
			require.NoError(t, writer.WriteField("fileName", strings.TrimSpace(value)))
		}
	}

	if err = writer.Close(); err != nil {
		require.NoError(t, err)
		return
	}

	method := strings.ToUpper(step.Method)
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequest(method, step.Endpoint, payload)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for key, value := range step.Headers {
		if strings.HasPrefix(key, "X-Upload-") {
			continue
		}
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp := doHTTPIntegrationRequest(t, client, req)
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	expectedStatus := step.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusOK
	}
	require.Equalf(t, expectedStatus, resp.StatusCode, "AASX upload failed: %s", string(responseBody))

	if step.Headers != nil {
		if expectedErrorContains, ok := step.Headers[uploadHeaderExpectedErrorContains]; ok {
			expectedErrorContains = strings.TrimSpace(expectedErrorContains)
			if expectedErrorContains != "" {
				require.Containsf(
					t,
					string(responseBody),
					expectedErrorContains,
					"AASX upload error body does not contain expected fragment",
				)
			}
		}

		if secondaryExpected, ok := step.Headers[uploadHeaderExpectedErrorContainsSecondary]; ok {
			secondaryExpected = strings.TrimSpace(secondaryExpected)
			if secondaryExpected != "" {
				require.Containsf(
					t,
					string(responseBody),
					secondaryExpected,
					"AASX upload error body does not contain expected secondary fragment",
				)
			}
		}
	}
}

func verifyStoredAttachments(t *testing.T, step testenv.JSONSuiteStep) {
	attachments := readStoredAttachmentsFromDB(t)
	attachmentExpectation := readExpectationMode(step.Headers, "X-Attachments-Expectation", expectationRequired)
	switch attachmentExpectation {
	case expectationRequired:
		require.NotEmpty(t, attachments, "no internal AASX file attachments found in DB after upload")
	case expectationAbsent:
		require.Empty(t, attachments, "attachments are not expected for this AASX, but file attachments exist in DB")
		return
	case expectationOptional:
		if len(attachments) == 0 {
			t.Log("no internal AASX file attachments found in DB after upload; accepted because expectation is optional")
			return
		}
	default:
		t.Fatalf("unsupported attachments expectation %q", attachmentExpectation)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(step.Endpoint), "/")
	require.NotEmpty(t, baseURL)

	client := &http.Client{Timeout: 60 * time.Second}

	for _, attachment := range attachments {
		encodedSubmodel := base64.RawURLEncoding.EncodeToString([]byte(attachment.SubmodelIdentifier))
		attachmentURL := baseURL +
			"/submodels/" + encodedSubmodel +
			"/submodel-elements/" + url.PathEscape(attachment.IDShortPath) +
			"/attachment"

		req, err := http.NewRequest(http.MethodGet, attachmentURL, nil)
		require.NoError(t, err)

		resp := doHTTPIntegrationRequest(t, client, req)
		func() {
			defer func() { _ = resp.Body.Close() }()
			body, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)
			require.Equalf(
				t,
				http.StatusOK,
				resp.StatusCode,
				"attachment endpoint failed for submodel '%s' path '%s' reference '%s': %s",
				attachment.SubmodelIdentifier,
				attachment.IDShortPath,
				attachment.FileReference,
				string(body),
			)
			require.NotEmptyf(
				t,
				body,
				"attachment endpoint returned empty content for submodel '%s' path '%s' reference '%s'",
				attachment.SubmodelIdentifier,
				attachment.IDShortPath,
				attachment.FileReference,
			)
			if strings.TrimSpace(attachment.ContentType) != "" {
				responseContentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
				require.Equalf(
					t,
					attachment.ContentType,
					responseContentType,
					"content-type mismatch for submodel '%s' path '%s' reference '%s'",
					attachment.SubmodelIdentifier,
					attachment.IDShortPath,
					attachment.FileReference,
				)
			}
		}()
	}
}

func readStoredAttachmentsFromDB(t *testing.T) []storedAttachment {
	t.Helper()

	db, err := sql.Open("pgx", uploadIntegrationDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	query, args, err := goqu.From(goqu.T("submodel").As("sm")).
		Select(
			goqu.I("sm.submodel_identifier"),
			goqu.I("sme.idshort_path"),
			goqu.I("fe.value"),
			goqu.I("fe.content_type"),
		).
		Join(goqu.T("submodel_element").As("sme"), goqu.On(goqu.I("sme.submodel_id").Eq(goqu.I("sm.id")))).
		Join(goqu.T("file_element").As("fe"), goqu.On(goqu.I("fe.id").Eq(goqu.I("sme.id")))).
		Join(goqu.T("file_binary_reference").As("fbr"), goqu.On(goqu.I("fbr.file_element_id").Eq(goqu.I("fe.id")))).
		Join(goqu.T("binary_content").As("bc"), goqu.On(goqu.I("bc.id").Eq(goqu.I("fbr.binary_content_id")))).
		Order(goqu.I("sm.submodel_identifier").Asc(), goqu.I("sme.idshort_path").Asc()).
		ToSQL()
	require.NoError(t, err)

	rows, err := db.Query(query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	result := make([]storedAttachment, 0)
	for rows.Next() {
		var attachment storedAttachment
		scanErr := rows.Scan(
			&attachment.SubmodelIdentifier,
			&attachment.IDShortPath,
			&attachment.FileReference,
			&attachment.ContentType,
		)
		require.NoError(t, scanErr)
		t.Logf("validated attachment candidate: submodel=%s path=%s value=%s contentType=%s", attachment.SubmodelIdentifier, attachment.IDShortPath, attachment.FileReference, attachment.ContentType)
		result = append(result, attachment)
	}
	require.NoError(t, rows.Err())
	return result
}

func verifyThumbnailEndpoints(t *testing.T, step testenv.JSONSuiteStep) {
	baseURL := strings.TrimRight(strings.TrimSpace(step.Endpoint), "/")
	require.NotEmpty(t, baseURL)

	aasIDs := fetchAASIDs(t, baseURL)
	require.NotEmpty(t, aasIDs, "no AAS IDs found after upload")
	thumbnailExpectation := readExpectationMode(step.Headers, "X-Thumbnail-Expectation", expectationRequired)

	client := &http.Client{Timeout: 60 * time.Second}
	for _, aasID := range aasIDs {
		encodedID := base64.RawURLEncoding.EncodeToString([]byte(aasID))
		thumbnailURL := baseURL + "/shells/" + encodedID + "/asset-information/thumbnail"

		req, err := http.NewRequest(http.MethodGet, thumbnailURL, nil)
		require.NoError(t, err)

		resp := doHTTPIntegrationRequest(t, client, req)
		func() {
			defer func() { _ = resp.Body.Close() }()
			body, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)
			switch thumbnailExpectation {
			case expectationRequired:
				require.Equalf(t, http.StatusOK, resp.StatusCode, "thumbnail endpoint failed for AAS '%s': %s", aasID, string(body))
				require.NotEmptyf(t, body, "thumbnail endpoint returned empty content for AAS '%s'", aasID)
			case expectationAbsent:
				require.Equalf(t, http.StatusNotFound, resp.StatusCode, "thumbnail must be absent for AAS '%s': %s", aasID, string(body))
			case expectationOptional:
				if resp.StatusCode == http.StatusNotFound {
					return
				}
				require.Equalf(t, http.StatusOK, resp.StatusCode, "thumbnail endpoint failed for AAS '%s': %s", aasID, string(body))
				require.NotEmptyf(t, body, "thumbnail endpoint returned empty content for AAS '%s'", aasID)
			default:
				t.Fatalf("unsupported thumbnail expectation %q", thumbnailExpectation)
			}
		}()
	}
}

func readExpectationMode(headers map[string]string, key string, defaultValue string) string {
	value := defaultValue
	if headers != nil {
		if headerValue, ok := headers[key]; ok && strings.TrimSpace(headerValue) != "" {
			value = headerValue
		}
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func fetchAASIDs(t *testing.T, baseURL string) []string {
	t.Helper()
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, baseURL+"/shells", nil)
	require.NoError(t, err)

	resp := doHTTPIntegrationRequest(t, client, req)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "listing AAS shells failed: %s", string(body))

	var parsed struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &parsed))

	ids := make([]string, 0, len(parsed.Result))
	for _, item := range parsed.Result {
		if strings.TrimSpace(item.ID) != "" {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func verifyEndpointSnapshot(t *testing.T, step testenv.JSONSuiteStep) {
	t.Helper()

	expectedPath := strings.TrimSpace(step.Data)
	require.NotEmpty(t, expectedPath, "expected snapshot path is required in step.data")

	method := strings.ToUpper(strings.TrimSpace(step.Method))
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequest(method, strings.TrimSpace(step.Endpoint), nil)
	require.NoError(t, err)
	for key, value := range step.Headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp := doHTTPIntegrationRequest(t, client, req)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "snapshot endpoint failed: %s", string(body))

	normalizedActual, err := normalizeJSONDocument(body)
	require.NoError(t, err)

	// #nosec G304 -- expectedPath comes from controlled integration test suite data.
	expectedRaw, err := os.ReadFile(expectedPath)
	if err != nil {
		require.NoError(t, os.MkdirAll(filepath.Dir(expectedPath), 0o750))
		// #nosec G306 -- integration tests intentionally create readable snapshot fixtures.
		require.NoError(t, os.WriteFile(expectedPath, normalizedActual, 0o600))
		t.Fatalf("expected snapshot created at %s; rerun test to verify", expectedPath)
	}

	normalizedExpected, err := normalizeJSONDocument(expectedRaw)
	require.NoError(t, err)
	require.Equal(t, string(normalizedExpected), string(normalizedActual), "snapshot mismatch for %s", step.Endpoint)
}

func normalizeJSONDocument(raw []byte) ([]byte, error) {
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}

	normalizeVolatileSnapshotFields(parsed)

	return json.Marshal(parsed)
}

func normalizeVolatileSnapshotFields(node any) {
	switch typed := node.(type) {
	case map[string]any:
		if modelType, ok := typed["modelType"].(string); ok && modelType == "File" {
			if value, ok := typed["value"].(string); ok && dynamicBinaryReferencePattern.MatchString(value) {
				typed["value"] = "<dynamic-file-reference>"
			}
		}

		if defaultThumbnail, ok := typed["defaultThumbnail"].(map[string]any); ok {
			if path, ok := defaultThumbnail["path"].(string); ok && dynamicBinaryReferencePattern.MatchString(path) {
				defaultThumbnail["path"] = "<dynamic-thumbnail-reference>"
			}
		}

		for _, value := range typed {
			normalizeVolatileSnapshotFields(value)
		}
	case []any:
		for _, item := range typed {
			normalizeVolatileSnapshotFields(item)
		}
	}
}

func doHTTPIntegrationRequest(t *testing.T, client *http.Client, req *http.Request) *http.Response {
	t.Helper()
	// #nosec G704 -- integration tests call controlled test-suite endpoints, not user-provided URLs.
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}
