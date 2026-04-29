package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

const actionUploadAASXMultipart = "UPLOAD_AASX_MULTIPART"
const actionVerifyAASXAttachments = "VERIFY_AASX_ATTACHMENTS"
const uploadIntegrationDSN = "host=127.0.0.1 port=6432 user=admin password=admin123 dbname=basyxTestDB sslmode=disable"

type storedAttachment struct {
	SubmodelIdentifier string
	IDShortPath        string
	FileReference      string
	ContentType        string
}

func TestUploadAASXIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ConfigPath: "upload_it_config.json",
		ActionHandlers: map[string]testenv.JSONStepAction{
			actionUploadAASXMultipart: func(t *testing.T, _ *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, _ int) {
				runAASXUploadAction(t, step)
			},
			actionVerifyAASXAttachments: func(t *testing.T, _ *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, _ int) {
				verifyStoredAttachments(t, step)
			},
		},
	})
}

func runAASXUploadAction(t *testing.T, step testenv.JSONSuiteStep) {
	file, err := os.Open(step.Data)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)

	filePart, err := writer.CreateFormFile("file", filepath.Base(step.Data))
	require.NoError(t, err)

	_, err = io.Copy(filePart, file)
	require.NoError(t, err)

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
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
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
	require.NotEmpty(t, attachments, "no internal AASX file attachments found in DB after upload")

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

		resp, err := client.Do(req)
		require.NoError(t, err)
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
