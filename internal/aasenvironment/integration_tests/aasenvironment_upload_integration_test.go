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

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

const actionUploadAASXMultipart = "UPLOAD_AASX_MULTIPART"
const actionUploadMultipart = "UPLOAD_MULTIPART"
const actionVerifyAASXAttachments = "VERIFY_AASX_ATTACHMENTS"
const actionVerifyAASXThumbnail = "VERIFY_AASX_THUMBNAIL"
const actionVerifyEndpointSnapshot = "VERIFY_ENDPOINT_SNAPSHOT"
const uploadIntegrationDSN = "host=127.0.0.1 port=6432 user=admin password=admin123 dbname=basyxTestDB sslmode=disable"
const expectationRequired = "required"
const expectationAbsent = "absent"
const expectationOptional = "optional"
const uploadHeaderPartContentType = "X-Upload-Part-ContentType"
const uploadHeaderPartFileName = "X-Upload-Part-FileName"
const uploadHeaderPartOmitFileName = "X-Upload-Part-OmitFileName"
const uploadHeaderRequestFileName = "X-Upload-FileName"

var numericValuePattern = regexp.MustCompile(`^\d+$`)

type storedAttachment struct {
	SubmodelIdentifier string
	IDShortPath        string
	FileReference      string
	ContentType        string
}

func TestUploadAASXIntegration(t *testing.T) {
	resetDatabaseForUploadIT(t)
	runUploadJSONSuite(t, "upload_it_config.json")
}

func TestUploadAASXIntegrationProductionPlan(t *testing.T) {
	resetDatabaseForUploadIT(t)
	runUploadJSONSuite(t, "upload_productionplan_it_config.json")
}

func TestUploadJSONAndXMLIntegration(t *testing.T) {
	resetDatabaseForUploadIT(t)
	runUploadJSONSuite(t, "upload_json_xml_config.json")
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

func resetDatabaseForUploadIT(t *testing.T) {
	t.Helper()

	db, err := sql.Open("postgres", uploadIntegrationDSN)
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

	db, err := sql.Open("postgres", uploadIntegrationDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	query := `
SELECT
    sm.submodel_identifier,
    sme.idshort_path,
    fe.value,
    fe.content_type
FROM submodel sm
JOIN submodel_element sme
    ON sme.submodel_id = sm.id
JOIN file_element fe
    ON fe.id = sme.id
JOIN file_data fd
    ON fd.id = fe.id
ORDER BY sm.submodel_identifier, sme.idshort_path
`

	rows, err := db.Query(query)
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
			if value, ok := typed["value"].(string); ok && numericValuePattern.MatchString(value) {
				typed["value"] = "<dynamic-file-reference>"
			}
		}

		if defaultThumbnail, ok := typed["defaultThumbnail"].(map[string]any); ok {
			if path, ok := defaultThumbnail["path"].(string); ok && numericValuePattern.MatchString(path) {
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
