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
// Author: Christian Koort ( Fraunhofer IESE )

//nolint:all
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
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const actionDeleteAllAAS = "DELETE_ALL_AAS"

var aasRepositoryBaseURL = testenv.LocalURLFromEnv("BASYX_IT_API_PORT", 6004)
var aasRepositoryInvalidBaseURL = testenv.LocalhostURLFromEnv("BASYX_IT_INVALID_API_PORT", 6006)
var integrationTestDSN = getIntegrationTestDSN()

func getIntegrationTestDSN() string {
	if dsn := os.Getenv("AASREPOSITORY_INTEGRATION_TEST_DSN"); dsn != "" {
		return dsn
	}

	return testenv.PostgresURLFromEnv("BASYX_IT_DB_PORT", 6432, "basyxTestDB")
}

func deleteAllAAS(t *testing.T, runner *testenv.JSONSuiteRunner, stepNumber int) {
	for {
		response, err := runner.RunStep(testenv.JSONSuiteStep{
			Method:         http.MethodGet,
			Endpoint:       aasRepositoryBaseURL + "/shells",
			ExpectedStatus: http.StatusOK,
		}, stepNumber)
		require.NoError(t, err)

		var list struct {
			Result []struct {
				ID string `json:"id"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal([]byte(response.Body), &list))

		if len(list.Result) == 0 {
			return
		}

		for _, item := range list.Result {
			encodedIdentifier := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
			_, err = runner.RunStep(testenv.JSONSuiteStep{
				Method:         http.MethodDelete,
				Endpoint:       fmt.Sprintf("%s/shells/%s", aasRepositoryBaseURL, encodedIdentifier),
				ExpectedStatus: http.StatusNoContent,
			}, stepNumber)
			require.NoError(t, err)
		}
	}
}

func createAASForThumbnailTest(baseURL string, aasID string) (int, error) {
	body := fmt.Sprintf(`{"id":"%s","modelType":"AssetAdministrationShell","assetInformation":{"assetKind":"Instance"}}`, aasID)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/shells", strings.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

func createAASForThumbnailTestWithDeclaredContentType(baseURL string, aasID string, thumbnailPath string, contentType string) (int, error) {
	body := fmt.Sprintf(`{"id":"%s","modelType":"AssetAdministrationShell","assetInformation":{"assetKind":"Instance","defaultThumbnail":{"path":"%s","contentType":"%s"}}}`,
		aasID,
		thumbnailPath,
		contentType,
	)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/shells", strings.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

func createTemporaryBinaryTestFile(t *testing.T, fileName string, payload []byte) string {
	t.Helper()

	filePath := filepath.Join(t.TempDir(), fileName)
	err := os.WriteFile(filePath, payload, 0o600)
	require.NoError(t, err, "failed to create temporary test file")

	return filePath
}

func uploadThumbnail(endpoint string, filePath string, fileName string) (int, error) {
	file, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return 0, fmt.Errorf("failed to create form file: %v", err)
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return 0, fmt.Errorf("failed to copy file: %v", err)
	}

	if fileName != "" {
		if err = writer.WriteField("fileName", fileName); err != nil {
			return 0, fmt.Errorf("failed to write fileName field: %v", err)
		}
	}

	err = writer.Close()
	if err != nil {
		return 0, fmt.Errorf("failed to close writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPut, endpoint, body)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

func downloadThumbnail(endpoint string) ([]byte, string, int, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to create request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", resp.StatusCode, fmt.Errorf("failed to read response: %v", err)
	}

	return body, resp.Header.Get("Content-Type"), resp.StatusCode, nil
}

func getJSONResponse(endpoint string) (map[string]any, int, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %v", err)
	}

	var payload map[string]any
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return payload, resp.StatusCode, nil
}

func getJSONArrayResponse(endpoint string) ([]string, int, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %v", err)
	}

	var payload []string
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return payload, resp.StatusCode, nil
}

func putJSONResponse(endpoint string, body string) (map[string]any, int, http.Header, error) {
	req, err := http.NewRequest(http.MethodPut, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, resp.StatusCode, resp.Header.Clone(), fmt.Errorf("failed to read response body: %v", readErr)
	}

	if strings.TrimSpace(string(responseBody)) == "" {
		return nil, resp.StatusCode, resp.Header.Clone(), nil
	}

	var payload map[string]any
	if unmarshalErr := json.Unmarshal(responseBody, &payload); unmarshalErr != nil {
		return nil, resp.StatusCode, resp.Header.Clone(), fmt.Errorf("failed to unmarshal response body with status %d: %v; body: %s", resp.StatusCode, unmarshalErr, string(responseBody))
	}

	return payload, resp.StatusCode, resp.Header.Clone(), nil
}

func postJSONResponse(endpoint string, body string) (map[string]any, int, http.Header, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, resp.StatusCode, resp.Header.Clone(), fmt.Errorf("failed to read response body: %v", readErr)
	}

	if strings.TrimSpace(string(responseBody)) == "" {
		return nil, resp.StatusCode, resp.Header.Clone(), nil
	}

	var payload map[string]any
	if unmarshalErr := json.Unmarshal(responseBody, &payload); unmarshalErr != nil {
		return nil, resp.StatusCode, resp.Header.Clone(), fmt.Errorf("failed to unmarshal response body with status %d: %v; body: %s", resp.StatusCode, unmarshalErr, string(responseBody))
	}

	return payload, resp.StatusCode, resp.Header.Clone(), nil
}

func postResponseStatus(endpoint string, body string) (int, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

func deleteResponseStatus(endpoint string) (int, error) {
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

func getResponseStatus(endpoint string) (int, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

func assertLocationHeaderMatches(t *testing.T, expectedLocation string, actualLocation string) {
	t.Helper()

	require.NotEmpty(t, actualLocation)

	expectedURL, err := url.Parse(expectedLocation)
	require.NoError(t, err)

	actualURL, err := url.Parse(actualLocation)
	require.NoError(t, err)

	assert.Equal(t, expectedURL.Scheme, actualURL.Scheme)
	assert.Equal(t, expectedURL.Path, actualURL.Path)
	assert.Equal(t, expectedURL.RawQuery, actualURL.RawQuery)
	assert.Equal(t, expectedURL.Port(), actualURL.Port())

	expectedHost := strings.ToLower(expectedURL.Hostname())
	actualHost := strings.ToLower(actualURL.Hostname())
	if expectedHost == actualHost {
		return
	}

	allowedLoopbackHosts := []string{"localhost", "127.0.0.1"}
	assert.Contains(t, allowedLoopbackHosts, expectedHost)
	assert.Contains(t, allowedLoopbackHosts, actualHost)
}

func assertServiceNeverHealthy(t *testing.T, endpoint string, observationWindow time.Duration) {
	t.Helper()

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(observationWindow)

	for time.Now().Before(deadline) {
		resp, err := client.Get(endpoint)
		if err == nil {
			if resp.Body != nil {
				_ = resp.Body.Close()
			}
			if resp.StatusCode == http.StatusOK {
				t.Fatalf("service unexpectedly became healthy at %s", endpoint)
			}
		}

		time.Sleep(300 * time.Millisecond)
	}
}

func getThumbnailWithoutFollowingRedirect(endpoint string) (int, string, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, "", fmt.Errorf("failed to create request: %v", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, resp.Header.Get("Location"), nil
}

func setExternalThumbnailForAAS(aasID string, externalURL string) error {
	db, err := sql.Open("pgx", integrationTestDSN)
	if err != nil {
		return fmt.Errorf("failed to open db connection: %v", err)
	}
	defer func() { _ = db.Close() }()

	dialect := goqu.Dialect("postgres")

	selectAASDBIDSQL, selectAASDBIDArgs, selectAASDBIDBuildErr := dialect.
		From(goqu.T("aas")).
		Select(goqu.I("id")).
		Where(goqu.I("aas_id").Eq(aasID)).
		Limit(1).
		ToSQL()
	if selectAASDBIDBuildErr != nil {
		return fmt.Errorf("failed to build aas id query: %v", selectAASDBIDBuildErr)
	}

	var aasDBID int64
	if queryErr := db.QueryRow(selectAASDBIDSQL, selectAASDBIDArgs...).Scan(&aasDBID); queryErr != nil {
		return fmt.Errorf("failed to query aas db id: %v", queryErr)
	}

	upsertThumbnailSQL, upsertThumbnailArgs, upsertThumbnailBuildErr := dialect.
		Insert(goqu.T("thumbnail_file_element")).
		Rows(goqu.Record{
			"id":           aasDBID,
			"content_type": "application/octet-stream",
			"file_name":    "external-thumbnail",
			"value":        externalURL,
		}).
		OnConflict(goqu.DoUpdate("id", goqu.Record{
			"content_type": "application/octet-stream",
			"file_name":    "external-thumbnail",
			"value":        externalURL,
		})).
		ToSQL()
	if upsertThumbnailBuildErr != nil {
		return fmt.Errorf("failed to build thumbnail upsert query: %v", upsertThumbnailBuildErr)
	}

	if _, execErr := db.Exec(upsertThumbnailSQL, upsertThumbnailArgs...); execErr != nil {
		return fmt.Errorf("failed to upsert thumbnail element: %v", execErr)
	}

	return nil
}

func getAASDatabaseID(db *sql.DB, aasID string) (int64, error) {
	dialect := goqu.Dialect("postgres")
	selectAASDBIDSQL, selectAASDBIDArgs, selectAASDBIDBuildErr := dialect.
		From(goqu.T("aas")).
		Select(goqu.I("id")).
		Where(goqu.I("aas_id").Eq(aasID)).
		Limit(1).
		ToSQL()
	if selectAASDBIDBuildErr != nil {
		return 0, fmt.Errorf("failed to build aas id query: %v", selectAASDBIDBuildErr)
	}

	var aasDBID int64
	if queryErr := db.QueryRow(selectAASDBIDSQL, selectAASDBIDArgs...).Scan(&aasDBID); queryErr != nil {
		return 0, fmt.Errorf("failed to query aas db id: %v", queryErr)
	}

	return aasDBID, nil
}

func sqlLiteral(input string) string {
	return strings.ReplaceAll(input, "'", "''")
}

func loadBodyFixture(t *testing.T, path string, replacements map[string]string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err, "failed to read body fixture")

	result := string(body)
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

func installDeleteFailureTriggers(t *testing.T, db *sql.DB, aasDBID int64, submodelID string) {
	t.Helper()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	deleteFunctionName := fmt.Sprintf("it_fail_submodel_delete_fn_%s", suffix)
	deleteTriggerName := fmt.Sprintf("it_fail_submodel_delete_trg_%s", suffix)
	restoreFunctionName := fmt.Sprintf("it_fail_submodel_ref_restore_fn_%s", suffix)
	restoreTriggerName := fmt.Sprintf("it_fail_submodel_ref_restore_trg_%s", suffix)
	safeSubmodelID := sqlLiteral(submodelID)

	createDeleteFunctionSQL := fmt.Sprintf(`
CREATE FUNCTION %s() RETURNS trigger AS $$
BEGIN
  IF OLD.submodel_identifier = '%s' THEN
    RAISE EXCEPTION 'forced submodel delete failure';
  END IF;
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;`, deleteFunctionName, safeSubmodelID)
	_, err := db.Exec(createDeleteFunctionSQL)
	require.NoError(t, err, "failed to create delete failure trigger function")

	createDeleteTriggerSQL := fmt.Sprintf(`
CREATE TRIGGER %s
BEFORE DELETE ON submodel
FOR EACH ROW
EXECUTE FUNCTION %s();`, deleteTriggerName, deleteFunctionName)
	_, err = db.Exec(createDeleteTriggerSQL)
	require.NoError(t, err, "failed to create delete failure trigger")

	createRestoreFunctionSQL := fmt.Sprintf(`
CREATE FUNCTION %s() RETURNS trigger AS $$
BEGIN
  IF NEW.aas_id = %d THEN
    RAISE EXCEPTION 'forced submodel reference restore failure';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;`, restoreFunctionName, aasDBID)
	_, err = db.Exec(createRestoreFunctionSQL)
	require.NoError(t, err, "failed to create reference restore failure trigger function")

	createRestoreTriggerSQL := fmt.Sprintf(`
CREATE TRIGGER %s
BEFORE INSERT ON aas_submodel_reference
FOR EACH ROW
EXECUTE FUNCTION %s();`, restoreTriggerName, restoreFunctionName)
	_, err = db.Exec(createRestoreTriggerSQL)
	require.NoError(t, err, "failed to create reference restore failure trigger")

	t.Cleanup(func() {
		_, _ = db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON aas_submodel_reference`, restoreTriggerName))
		_, _ = db.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, restoreFunctionName))
		_, _ = db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON submodel`, deleteTriggerName))
		_, _ = db.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, deleteFunctionName))
	})
}

// IntegrationTest runs the integration tests based on the config file
func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ActionHandlers: map[string]testenv.JSONStepAction{
			actionDeleteAllAAS: func(t *testing.T, runner *testenv.JSONSuiteRunner, _ testenv.JSONSuiteStep, stepNumber int) {
				deleteAllAAS(t, runner, stepNumber)
			},
			testenv.ActionAssertSubmodelAbsent: testenv.NewCheckSubmodelAbsentAction(testenv.CheckSubmodelAbsentOptions{
				Driver: "pgx",
				DSN:    integrationTestDSN,
			}),
		},
		StepName: func(step testenv.JSONSuiteStep, stepNumber int) string {
			context := "Not Provided"
			if step.Context != "" {
				context = step.Context
			}
			return fmt.Sprintf("Step_(%s)_%d_%s_%s", context, stepNumber, step.Method, step.Endpoint)
		},
		ShouldMatchJSON: shouldMatchImportedDescriptionResponseForAASEnvironment,
	})
}

func shouldMatchImportedDescriptionResponseForAASEnvironment(step testenv.JSONSuiteStep) bool {
	if os.Getenv("BASYX_AASENVIRONMENT_SKIP_IMPORTED_DESCRIPTION") != "1" {
		return true
	}
	if !strings.EqualFold(step.Method, http.MethodGet) {
		return true
	}
	parsedEndpoint, err := url.Parse(step.Endpoint)
	if err != nil {
		return true
	}
	return parsedEndpoint.Path != "/description"
}

func TestQueryAssetAdministrationShellFalseFragmentFiltersKeepRootAAS(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/query-fragments-%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))

	requestBody := loadBodyFixture(t, "bodies/post/postAssetAdministrationShellQueryFragments.json", map[string]string{
		"{{AAS_ID}}": aasID,
	})

	statusCode, err := postResponseStatus(baseURL+"/shells", requestBody)
	require.NoError(t, err, "AAS creation failed")
	require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")
	t.Cleanup(func() {
		_, _ = deleteResponseStatus(fmt.Sprintf("%s/shells/%s", baseURL, aasIdentifier))
	})

	tests := []struct {
		name     string
		fragment string
		assert   func(t *testing.T, shell map[string]any)
	}{
		{
			name:     "idShort",
			fragment: "$aas#idShort",
			assert: func(t *testing.T, shell map[string]any) {
				_, ok := shell["idShort"]
				assert.False(t, ok, "idShort should be filtered")
			},
		},
		{
			name:     "assetType",
			fragment: "$aas#assetInformation.assetType",
			assert: func(t *testing.T, shell map[string]any) {
				assetInformation := requireAssetInformation(t, shell)
				_, ok := assetInformation["assetType"]
				assert.False(t, ok, "assetInformation.assetType should be filtered")
			},
		},
		{
			name:     "globalAssetId",
			fragment: "$aas#assetInformation.globalAssetId",
			assert: func(t *testing.T, shell map[string]any) {
				assetInformation := requireAssetInformation(t, shell)
				_, ok := assetInformation["globalAssetId"]
				assert.False(t, ok, "assetInformation.globalAssetId should be filtered")
			},
		},
		{
			name:     "specificAssetIds",
			fragment: "$aas#assetInformation.specificAssetIds[0]",
			assert: func(t *testing.T, shell map[string]any) {
				assetInformation := requireAssetInformation(t, shell)
				_, ok := assetInformation["specificAssetIds"]
				assert.False(t, ok, "assetInformation.specificAssetIds should be filtered")
			},
		},
		{
			name:     "specificAssetExternalSubjectId",
			fragment: "$aas#assetInformation.specificAssetIds[0].externalSubjectId",
			assert: func(t *testing.T, shell map[string]any) {
				specificAssetID := requireFirstSpecificAssetID(t, shell)
				_, ok := specificAssetID["externalSubjectId"]
				assert.False(t, ok, "specificAssetIds[].externalSubjectId should be filtered")
			},
		},
		{
			name:     "specificAssetExternalSubjectIdKeys",
			fragment: "$aas#assetInformation.specificAssetIds[0].externalSubjectId.keys[0]",
			assert: func(t *testing.T, shell map[string]any) {
				specificAssetID := requireFirstSpecificAssetID(t, shell)
				_, ok := specificAssetID["externalSubjectId"]
				assert.False(t, ok, "specificAssetIds[].externalSubjectId.keys[] should be filtered")
			},
		},
		{
			name:     "submodels",
			fragment: "$aas#submodels[0]",
			assert: func(t *testing.T, shell map[string]any) {
				_, ok := shell["submodels"]
				assert.False(t, ok, "submodels should be filtered")
			},
		},
		{
			name:     "submodelKeys",
			fragment: "$aas#submodels[0].keys[0]",
			assert: func(t *testing.T, shell map[string]any) {
				_, ok := shell["submodels"]
				assert.False(t, ok, "submodels[].keys[] should be filtered")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shell := querySingleAASWithFalseFragment(t, baseURL, aasID, tt.fragment)
			assert.Equal(t, aasID, shell["id"], "root AAS should still be returned")
			tt.assert(t, shell)
		})
	}
}

func querySingleAASWithFalseFragment(t *testing.T, baseURL string, aasID string, fragment string) map[string]any {
	t.Helper()

	query := map[string]any{
		"$condition": map[string]any{
			"$eq": []any{
				map[string]any{"$field": "$aas#id"},
				map[string]any{"$strVal": aasID},
			},
		},
		"$filters": []any{
			map[string]any{
				"$fragment":  fragment,
				"$condition": map[string]any{"$boolean": false},
			},
		},
	}

	body, err := json.Marshal(query)
	require.NoError(t, err, "failed to build query body")

	payload, statusCode, _, postErr := postJSONResponse(baseURL+"/query/shells", string(body))
	require.NoError(t, postErr, "query request failed")
	require.Equal(t, http.StatusOK, statusCode, "Expected 200 OK for query")

	result, ok := payload["result"].([]any)
	require.True(t, ok, "query result should be an array")
	require.Len(t, result, 1, "query should return exactly the created AAS")

	shell, ok := result[0].(map[string]any)
	require.True(t, ok, "query result item should be an object")
	return shell
}

func requireAssetInformation(t *testing.T, shell map[string]any) map[string]any {
	t.Helper()

	assetInformation, ok := shell["assetInformation"].(map[string]any)
	require.True(t, ok, "assetInformation should be present")
	return assetInformation
}

func requireFirstSpecificAssetID(t *testing.T, shell map[string]any) map[string]any {
	t.Helper()

	assetInformation := requireAssetInformation(t, shell)
	specificAssetIDs, ok := assetInformation["specificAssetIds"].([]any)
	require.True(t, ok, "specificAssetIds should be present")
	require.Len(t, specificAssetIDs, 1, "expected exactly one specificAssetId")

	specificAssetID, ok := specificAssetIDs[0].(map[string]any)
	require.True(t, ok, "specificAssetId should be an object")
	return specificAssetID
}

func TestPostAssetAdministrationShellAcceptsNullSubmodels(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/null-submodels-%d", time.Now().UnixNano())

	requestBody := fmt.Sprintf(`{
		"id":"%s",
		"idShort":"NullSubmodelsShell",
		"modelType":"AssetAdministrationShell",
		"assetInformation":{"assetKind":"Instance"},
		"submodels":null
	}`, aasID)

	statusCode, err := postResponseStatus(baseURL+"/shells", requestBody)
	require.NoError(t, err, "POST AAS request failed")
	require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS payload with submodels=null")

	encodedAASIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	t.Cleanup(func() {
		deleteStatusCode, deleteErr := deleteResponseStatus(fmt.Sprintf("%s/shells/%s", baseURL, encodedAASIdentifier))
		if deleteErr != nil {
			t.Logf("cleanup delete failed for AAS %s: %v", aasID, deleteErr)
			return
		}
		if deleteStatusCode != http.StatusNoContent && deleteStatusCode != http.StatusNotFound {
			t.Logf("cleanup delete returned unexpected status=%d for AAS %s", deleteStatusCode, aasID)
		}
	})

	payload, getStatusCode, getErr := getJSONResponse(fmt.Sprintf("%s/shells/%s", baseURL, encodedAASIdentifier))
	require.NoError(t, getErr, "GET AAS request failed")
	require.Equal(t, http.StatusOK, getStatusCode, "Expected 200 OK after creating AAS with submodels=null")
	assert.Equal(t, aasID, payload["id"], "Expected created AAS id in GET response")
}

func TestPutSubmodelByIdAasRepositoryReturnsCreatedSubmodelAnd201(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/put-submodel-create-%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))

	statusCode, err := createAASForThumbnailTest(baseURL, aasID)
	require.NoError(t, err, "AAS creation failed")
	require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")

	submodelID := fmt.Sprintf("https://example.com/ids/sm/put-submodel-create-%d", time.Now().UnixNano())
	submodelIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	endpoint := fmt.Sprintf("%s/shells/%s/submodels/%s", baseURL, aasIdentifier, submodelIdentifier)

	body := fmt.Sprintf(
		`{"id":"%s","idShort":"PutCreateSubmodel","modelType":"Submodel","kind":"Instance","submodelElements":[{"idShort":"prop1","modelType":"Property","valueType":"xs:string","value":"hello"}]}`,
		submodelID,
	)

	payload, putStatusCode, headers, putErr := putJSONResponse(endpoint, body)
	require.NoError(t, putErr, "PUT submodel request failed")
	require.Equal(t, http.StatusCreated, putStatusCode, "Expected 201 Created when creating new submodel via AAS endpoint")
	require.NotNil(t, payload, "Expected response payload for created submodel")
	assert.Equal(t, submodelID, payload["id"], "Expected response body to contain created submodel id")
	assert.Equal(t, "PutCreateSubmodel", payload["idShort"], "Expected response body to contain created submodel idShort")
	assert.Equal(t, "Submodel", payload["modelType"], "Expected response body to contain created submodel modelType")
	assertLocationHeaderMatches(t, endpoint, headers.Get("Location"))
}

func TestPutSubmodelByIdAasRepositoryReturnsNoContentOnUpdate(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/put-submodel-update-%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))

	statusCode, err := createAASForThumbnailTest(baseURL, aasID)
	require.NoError(t, err, "AAS creation failed")
	require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")

	submodelID := fmt.Sprintf("https://example.com/ids/sm/put-submodel-update-%d", time.Now().UnixNano())
	submodelIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	endpoint := fmt.Sprintf("%s/shells/%s/submodels/%s", baseURL, aasIdentifier, submodelIdentifier)

	initialBody := fmt.Sprintf(
		`{"id":"%s","idShort":"PutUpdateSubmodel","modelType":"Submodel","kind":"Instance","submodelElements":[{"idShort":"prop1","modelType":"Property","valueType":"xs:string","value":"before"}]}`,
		submodelID,
	)

	_, initialStatusCode, _, initialErr := putJSONResponse(endpoint, initialBody)
	require.NoError(t, initialErr, "Initial PUT submodel request failed")
	require.Equal(t, http.StatusCreated, initialStatusCode, "Expected 201 Created for initial PUT")

	updatedBody := fmt.Sprintf(
		`{"id":"%s","idShort":"PutUpdateSubmodel","modelType":"Submodel","kind":"Instance","submodelElements":[{"idShort":"prop1","modelType":"Property","valueType":"xs:string","value":"after"}]}`,
		submodelID,
	)

	updatePayload, updateStatusCode, updateHeaders, updateErr := putJSONResponse(endpoint, updatedBody)
	require.NoError(t, updateErr, "Update PUT submodel request failed")
	require.Equal(t, http.StatusNoContent, updateStatusCode, "Expected 204 No Content for update PUT")
	assert.Nil(t, updatePayload, "Expected empty response body for update PUT")
	assert.Empty(t, updateHeaders.Get("Location"), "Expected no Location header for update PUT")

	getPayload, getStatusCode, getErr := getJSONResponse(endpoint)
	require.NoError(t, getErr, "GET submodel after update failed")
	require.Equal(t, http.StatusOK, getStatusCode, "Expected 200 OK for GET after update PUT")

	submodelElements, ok := getPayload["submodelElements"].([]any)
	require.True(t, ok, "Expected submodelElements in GET response")
	require.NotEmpty(t, submodelElements, "Expected submodelElements to contain updated property")

	firstElement, ok := submodelElements[0].(map[string]any)
	require.True(t, ok, "Expected first submodel element as object")
	assert.Equal(t, "after", firstElement["value"], "Expected updated property value to be persisted")
}

func TestLocationHeadersForSubmodelElementCreateEndpointsAasRepository(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/location-headers-submodel-elements-%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))

	statusCode, err := createAASForThumbnailTest(baseURL, aasID)
	require.NoError(t, err, "AAS creation failed")
	require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")

	submodelID := fmt.Sprintf("https://example.com/ids/sm/location-headers-submodel-elements-%d", time.Now().UnixNano())
	submodelIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	submodelEndpoint := fmt.Sprintf("%s/shells/%s/submodels/%s", baseURL, aasIdentifier, submodelIdentifier)

	submodelBody := fmt.Sprintf(
		`{"id":"%s","idShort":"LocationHeadersSM","modelType":"Submodel","kind":"Instance","submodelElements":[{"idShort":"MainCollection","modelType":"SubmodelElementCollection","value":[]}]}`,
		submodelID,
	)

	_, putSubmodelStatusCode, _, putSubmodelErr := putJSONResponse(submodelEndpoint, submodelBody)
	require.NoError(t, putSubmodelErr, "PUT submodel request failed")
	require.Equal(t, http.StatusCreated, putSubmodelStatusCode, "Expected 201 Created when creating submodel")

	t.Run("PostSubmodelElementAasRepositorySetsLocation", func(t *testing.T) {
		endpoint := submodelEndpoint + "/submodel-elements"
		body := `{"idShort":"PostCreated","modelType":"Property","valueType":"xs:string","value":"post-value"}`

		payload, postStatusCode, headers, postErr := postJSONResponse(endpoint, body)
		require.NoError(t, postErr, "POST submodel element request failed")
		require.Equal(t, http.StatusCreated, postStatusCode, "Expected 201 Created when creating submodel element")
		require.NotNil(t, payload, "Expected response body for created submodel element")
		assert.Equal(t, "PostCreated", payload["idShort"], "Expected response body to contain created submodel element idShort")
		assertLocationHeaderMatches(t, endpoint+"/PostCreated", headers.Get("Location"))
	})

	t.Run("PutSubmodelElementByPathAasRepositorySetsLocationOnlyOnCreate", func(t *testing.T) {
		endpoint := submodelEndpoint + "/submodel-elements/PutCreated"
		initialBody := `{"idShort":"PutCreated","modelType":"Property","valueType":"xs:string","value":"before"}`

		_, createStatusCode, createHeaders, createErr := putJSONResponse(endpoint, initialBody)
		require.NoError(t, createErr, "Initial PUT submodel element request failed")
		require.Equal(t, http.StatusCreated, createStatusCode, "Expected 201 Created when creating submodel element by path")
		assertLocationHeaderMatches(t, endpoint, createHeaders.Get("Location"))

		updatedBody := `{"idShort":"PutCreated","modelType":"Property","valueType":"xs:string","value":"after"}`
		_, updateStatusCode, updateHeaders, updateErr := putJSONResponse(endpoint, updatedBody)
		require.NoError(t, updateErr, "Update PUT submodel element request failed")
		require.Equal(t, http.StatusNoContent, updateStatusCode, "Expected 204 No Content when updating existing submodel element")
		assert.Empty(t, updateHeaders.Get("Location"), "Expected no Location header for update PUT")
	})

	t.Run("PostSubmodelElementByPathAasRepositorySetsChildLocation", func(t *testing.T) {
		endpoint := submodelEndpoint + "/submodel-elements/MainCollection"
		body := `{"idShort":"ChildCreated","modelType":"Property","valueType":"xs:string","value":"child"}`

		payload, postStatusCode, headers, postErr := postJSONResponse(endpoint, body)
		require.NoError(t, postErr, "POST submodel element by path request failed")
		require.Equal(t, http.StatusCreated, postStatusCode, "Expected 201 Created when creating child submodel element")
		require.NotNil(t, payload, "Expected response body for created child submodel element")
		assert.Equal(t, "ChildCreated", payload["idShort"], "Expected response body to contain created child idShort")
		assertLocationHeaderMatches(t, endpoint+".ChildCreated", headers.Get("Location"))
	})
}

func TestGetSubmodelByIdAasRepositoryReturnsSubmodel(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/get-submodel-%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))

	statusCode, err := createAASForThumbnailTest(baseURL, aasID)
	require.NoError(t, err, "AAS creation failed")
	require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")

	submodelID := fmt.Sprintf("https://example.com/ids/sm/get-submodel-%d", time.Now().UnixNano())
	submodelIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	endpoint := fmt.Sprintf("%s/shells/%s/submodels/%s", baseURL, aasIdentifier, submodelIdentifier)

	body := fmt.Sprintf(
		`{"id":"%s","idShort":"GetSubmodelViaAAS","modelType":"Submodel","kind":"Instance","submodelElements":[{"idShort":"prop1","modelType":"Property","valueType":"xs:string","value":"hello"}]}`,
		submodelID,
	)
	_, putStatusCode, _, putErr := putJSONResponse(endpoint, body)
	require.NoError(t, putErr, "PUT submodel request failed")
	require.Equal(t, http.StatusCreated, putStatusCode, "Expected 201 Created when creating submodel via AAS endpoint")

	payload, getStatusCode, getErr := getJSONResponse(endpoint + "?level=core")
	require.NoError(t, getErr, "GET submodel request failed")
	require.Equal(t, http.StatusOK, getStatusCode, "Expected 200 OK for GET submodel via AAS endpoint")
	assert.Equal(t, submodelID, payload["id"], "Expected submodel id in GET response")
	assert.Equal(t, "GetSubmodelViaAAS", payload["idShort"], "Expected submodel idShort in GET response")
	assert.Equal(t, "Submodel", payload["modelType"], "Expected submodel modelType in GET response")
}

func TestPostSubmodelReferenceAasRepositoryReturnsConflictOnDuplicate(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/submodel-ref-duplicate-%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))

	statusCode, err := createAASForThumbnailTest(baseURL, aasID)
	require.NoError(t, err, "AAS creation failed")
	require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")

	submodelID := fmt.Sprintf("https://example.com/ids/sm/submodel-ref-duplicate-%d", time.Now().UnixNano())
	endpoint := fmt.Sprintf("%s/shells/%s/submodel-refs", baseURL, aasIdentifier)
	requestBody := fmt.Sprintf(`{"type":"ModelReference","keys":[{"type":"Submodel","value":"%s"}]}`, submodelID)

	firstStatusCode, firstErr := postResponseStatus(endpoint, requestBody)
	require.NoError(t, firstErr, "First POST submodel reference request failed")
	require.Equal(t, http.StatusCreated, firstStatusCode, "Expected 201 Created for first POST submodel reference")

	secondStatusCode, secondErr := postResponseStatus(endpoint, requestBody)
	require.NoError(t, secondErr, "Second POST submodel reference request failed")
	require.Equal(t, http.StatusConflict, secondStatusCode, "Expected 409 Conflict when posting duplicate submodel reference")
}

func TestSubmodelSuperPathEndpointsAasRepository(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/superpath-%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))

	statusCode, err := createAASForThumbnailTest(baseURL, aasID)
	require.NoError(t, err, "AAS creation failed")
	require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")

	submodelID := fmt.Sprintf("https://example.com/ids/sm/superpath-%d", time.Now().UnixNano())
	submodelIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	topBlobValue := base64.StdEncoding.EncodeToString([]byte("aas-superpath-top-blob"))
	nestedBlobValue := base64.StdEncoding.EncodeToString([]byte("aas-superpath-nested-blob"))

	submodelEndpoint := fmt.Sprintf("%s/shells/%s/submodels/%s", baseURL, aasIdentifier, submodelIdentifier)
	body := fmt.Sprintf(
		`{"id":"%s","idShort":"SuperpathSubmodel","modelType":"Submodel","kind":"Instance","submodelElements":[{"idShort":"TopProperty","modelType":"Property","valueType":"xs:string","value":"hello"},{"idShort":"TopBlob","modelType":"Blob","contentType":"text/plain","value":"%s"},{"idShort":"MainCollection","modelType":"SubmodelElementCollection","value":[{"idShort":"NestedProperty","modelType":"Property","valueType":"xs:string","value":"nested"},{"idShort":"NestedBlob","modelType":"Blob","contentType":"application/octet-stream","value":"%s"}]}]}`,
		submodelID,
		topBlobValue,
		nestedBlobValue,
	)

	_, putStatusCode, _, putErr := putJSONResponse(submodelEndpoint, body)
	require.NoError(t, putErr, "PUT submodel request failed")
	require.Equal(t, http.StatusCreated, putStatusCode, "Expected 201 Created for PUT submodel")

	t.Run("GetSubmodelByIdPathDeepReturnsNestedPaths", func(t *testing.T) {
		paths, pathStatusCode, pathErr := getJSONArrayResponse(fmt.Sprintf("%s/$path?level=deep", submodelEndpoint))
		require.NoError(t, pathErr, "GET submodel $path request failed")
		require.Equal(t, http.StatusOK, pathStatusCode, "Expected 200 OK for GET submodel $path")

		assert.Contains(t, paths, "TopProperty")
		assert.Contains(t, paths, "TopBlob")
		assert.Contains(t, paths, "MainCollection")
		assert.Contains(t, paths, "MainCollection.NestedProperty")
		assert.Contains(t, paths, "MainCollection.NestedBlob")
	})

	t.Run("GetSubmodelByIdHonorsExtent", func(t *testing.T) {
		payload, statusCode, err := getJSONResponse(submodelEndpoint)
		require.NoError(t, err, "GET submodel request failed")
		require.Equal(t, http.StatusOK, statusCode, "Expected 200 OK for GET submodel")
		requireAASSubmodelBlobValueState(t, payload, "TopBlob", "text/plain", "", false)
		requireAASSubmodelBlobValueState(t, payload, "NestedBlob", "application/octet-stream", "", false)

		payload, statusCode, err = getJSONResponse(submodelEndpoint + "?extent=withBlobValue")
		require.NoError(t, err, "GET submodel with extent request failed")
		require.Equal(t, http.StatusOK, statusCode, "Expected 200 OK for GET submodel with extent")
		requireAASSubmodelBlobValueState(t, payload, "TopBlob", "text/plain", topBlobValue, true)
		requireAASSubmodelBlobValueState(t, payload, "NestedBlob", "application/octet-stream", nestedBlobValue, true)
	})

	t.Run("GetAllSubmodelElementsHonorsExtent", func(t *testing.T) {
		payload, statusCode, err := getJSONResponse(submodelEndpoint + "/submodel-elements?level=deep&extent=withBlobValue")
		require.NoError(t, err, "GET submodel elements request failed")
		require.Equal(t, http.StatusOK, statusCode, "Expected 200 OK for GET submodel elements")
		result, ok := payload["result"].([]any)
		require.True(t, ok, "Expected result array")
		requireAASBlobValueState(t, findAASSubmodelElementInList(t, result, "TopBlob"), "text/plain", topBlobValue, true)
		requireAASBlobValueState(t, findAASSubmodelElementInList(t, result, "NestedBlob"), "application/octet-stream", nestedBlobValue, true)
	})

	t.Run("GetSubmodelElementByPathValueOnlyHonorsExtent", func(t *testing.T) {
		payload, statusCode, err := getJSONResponse(submodelEndpoint + "/submodel-elements/TopBlob/$value")
		require.NoError(t, err, "GET value-only blob request failed")
		require.Equal(t, http.StatusOK, statusCode, "Expected 200 OK for GET value-only blob")
		requireAASValueOnlyBlobValueState(t, payload, "text/plain", "", false)

		payload, statusCode, err = getJSONResponse(submodelEndpoint + "/submodel-elements/TopBlob/$value?extent=withBlobValue")
		require.NoError(t, err, "GET value-only blob with extent request failed")
		require.Equal(t, http.StatusOK, statusCode, "Expected 200 OK for GET value-only blob with extent")
		requireAASValueOnlyBlobValueState(t, payload, "text/plain", topBlobValue, true)
	})

	t.Run("GetSubmodelElementByPathPathCoreReturnsRequestedPath", func(t *testing.T) {
		paths, pathStatusCode, pathErr := getJSONArrayResponse(fmt.Sprintf("%s/submodel-elements/MainCollection/$path?level=core", submodelEndpoint))
		require.NoError(t, pathErr, "GET submodel element $path request failed")
		require.Equal(t, http.StatusOK, pathStatusCode, "Expected 200 OK for GET submodel element $path")

		assert.Equal(t, []string{"MainCollection"}, paths)
	})

	t.Run("GetSubmodelByIdPathReturnsNotFoundIfSubmodelNotReferencedInAAS", func(t *testing.T) {
		otherAASID := fmt.Sprintf("https://example.com/ids/aas/superpath-other-%d", time.Now().UnixNano())
		otherAASIdentifier := base64.RawURLEncoding.EncodeToString([]byte(otherAASID))

		createStatus, createErr := createAASForThumbnailTest(baseURL, otherAASID)
		require.NoError(t, createErr, "Second AAS creation failed")
		require.Equal(t, http.StatusCreated, createStatus, "Expected 201 Created for second AAS creation")

		getStatusCode, getErr := getResponseStatus(fmt.Sprintf("%s/shells/%s/submodels/%s/$path?level=deep", baseURL, otherAASIdentifier, submodelIdentifier))
		require.NoError(t, getErr, "GET submodel $path with unlinked AAS request failed")
		require.Equal(t, http.StatusNotFound, getStatusCode, "Expected 404 for submodel not referenced in selected AAS")
	})
}

func requireAASSubmodelBlobValueState(t *testing.T, submodel map[string]any, idShort string, contentType string, expectedValue string, expectValue bool) {
	t.Helper()
	rawElements, ok := submodel["submodelElements"].([]any)
	require.True(t, ok, "submodelElements must be an array")
	requireAASBlobValueState(t, findAASSubmodelElementInList(t, rawElements, idShort), contentType, expectedValue, expectValue)
}

func requireAASBlobValueState(t *testing.T, element map[string]any, contentType string, expectedValue string, expectValue bool) {
	t.Helper()
	require.Equal(t, "Blob", element["modelType"])
	require.Equal(t, contentType, element["contentType"])
	actualValue, hasValue := element["value"]
	require.Equal(t, expectValue, hasValue, "blob value presence mismatch in element: %#v", element)
	if expectValue {
		require.Equal(t, expectedValue, actualValue)
	}
}

func requireAASValueOnlyBlobValueState(t *testing.T, blobValue map[string]any, contentType string, expectedValue string, expectValue bool) {
	t.Helper()
	require.Equal(t, contentType, blobValue["contentType"])
	actualValue, hasValue := blobValue["value"]
	require.Equal(t, expectValue, hasValue, "blob value presence mismatch in value-only payload: %#v", blobValue)
	if expectValue {
		require.Equal(t, expectedValue, actualValue)
	}
}

func findAASSubmodelElementInList(t *testing.T, rawElements []any, idShort string) map[string]any {
	t.Helper()
	for _, rawElement := range rawElements {
		element, ok := rawElement.(map[string]any)
		require.True(t, ok, "submodel element must be an object: %#v", rawElement)
		if element["idShort"] == idShort {
			return element
		}
		if rawValue, ok := element["value"].([]any); ok {
			if nested := findAASSubmodelElementInListOptional(t, rawValue, idShort); nested != nil {
				return nested
			}
		}
	}
	t.Fatalf("expected submodel element idShort=%s in payload: %#v", idShort, rawElements)
	return nil
}

func findAASSubmodelElementInListOptional(t *testing.T, rawElements []any, idShort string) map[string]any {
	t.Helper()
	for _, rawElement := range rawElements {
		element, ok := rawElement.(map[string]any)
		require.True(t, ok, "submodel element must be an object: %#v", rawElement)
		if element["idShort"] == idShort {
			return element
		}
		if rawValue, ok := element["value"].([]any); ok {
			if nested := findAASSubmodelElementInListOptional(t, rawValue, idShort); nested != nil {
				return nested
			}
		}
	}
	return nil
}

func TestDeleteSubmodelByIdAasRepositoryDeletesSubmodelAndReference(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/delete-submodel-%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))

	statusCode, err := createAASForThumbnailTest(baseURL, aasID)
	require.NoError(t, err, "AAS creation failed")
	require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")

	submodelID := fmt.Sprintf("https://example.com/ids/sm/delete-submodel-%d", time.Now().UnixNano())
	submodelIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	endpoint := fmt.Sprintf("%s/shells/%s/submodels/%s", baseURL, aasIdentifier, submodelIdentifier)

	body := fmt.Sprintf(
		`{"id":"%s","idShort":"DeleteSubmodelViaAAS","modelType":"Submodel","kind":"Instance","submodelElements":[{"idShort":"prop1","modelType":"Property","valueType":"xs:string","value":"hello"}]}`,
		submodelID,
	)
	_, putStatusCode, _, putErr := putJSONResponse(endpoint, body)
	require.NoError(t, putErr, "PUT submodel request failed")
	require.Equal(t, http.StatusCreated, putStatusCode, "Expected 201 Created when creating submodel via AAS endpoint")

	deleteStatusCode, deleteErr := deleteResponseStatus(endpoint)
	require.NoError(t, deleteErr, "DELETE submodel request failed")
	require.Equal(t, http.StatusNoContent, deleteStatusCode, "Expected 204 No Content for DELETE submodel via AAS endpoint")

	referencesPayload, referencesStatusCode, referencesErr := getJSONResponse(fmt.Sprintf("%s/shells/%s/submodel-refs", baseURL, aasIdentifier))
	require.NoError(t, referencesErr, "GET submodel references failed")
	require.Equal(t, http.StatusOK, referencesStatusCode, "Expected 200 OK for GET submodel references")
	result, ok := referencesPayload["result"].([]any)
	require.True(t, ok, "Expected result array in submodel references response")
	assert.Len(t, result, 0, "Expected no submodel references after deletion")

	getDeletedStatusCode, getDeletedErr := getResponseStatus(endpoint)
	require.NoError(t, getDeletedErr, "GET deleted submodel request failed")
	require.Equal(t, http.StatusNotFound, getDeletedStatusCode, "Expected 404 Not Found for deleted submodel")
}

func TestDeleteSubmodelByIdAasRepositoryRollsBackReferenceDeleteOnSubmodelDeleteFailure(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/delete-submodel-tx-%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))

	statusCode, err := createAASForThumbnailTest(baseURL, aasID)
	require.NoError(t, err, "AAS creation failed")
	require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")

	submodelID := fmt.Sprintf("https://example.com/ids/sm/delete-submodel-tx-%d", time.Now().UnixNano())
	submodelIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	endpoint := fmt.Sprintf("%s/shells/%s/submodels/%s", baseURL, aasIdentifier, submodelIdentifier)

	body := fmt.Sprintf(
		`{"id":"%s","idShort":"DeleteSubmodelTx","modelType":"Submodel","kind":"Instance","submodelElements":[{"idShort":"prop1","modelType":"Property","valueType":"xs:string","value":"hello"}]}`,
		submodelID,
	)
	_, putStatusCode, _, putErr := putJSONResponse(endpoint, body)
	require.NoError(t, putErr, "PUT submodel request failed")
	require.Equal(t, http.StatusCreated, putStatusCode, "Expected 201 Created when creating submodel via AAS endpoint")

	db, openErr := sql.Open("pgx", integrationTestDSN)
	require.NoError(t, openErr, "failed to open db connection")
	t.Cleanup(func() { _ = db.Close() })

	aasDBID, aasDBIDErr := getAASDatabaseID(db, aasID)
	require.NoError(t, aasDBIDErr, "failed to resolve aas db id")
	installDeleteFailureTriggers(t, db, aasDBID, submodelID)

	deleteStatusCode, deleteErr := deleteResponseStatus(endpoint)
	require.NoError(t, deleteErr, "DELETE submodel request failed")
	require.Equal(t, http.StatusInternalServerError, deleteStatusCode, "Expected 500 when submodel delete is forced to fail")

	referencesPayload, referencesStatusCode, referencesErr := getJSONResponse(fmt.Sprintf("%s/shells/%s/submodel-refs", baseURL, aasIdentifier))
	require.NoError(t, referencesErr, "GET submodel references failed")
	require.Equal(t, http.StatusOK, referencesStatusCode, "Expected 200 OK for GET submodel references")
	result, ok := referencesPayload["result"].([]any)
	require.True(t, ok, "Expected result array in submodel references response")
	require.Len(t, result, 1, "Expected submodel reference to be restored by transaction rollback")

	getExistingStatusCode, getExistingErr := getResponseStatus(endpoint)
	require.NoError(t, getExistingErr, "GET submodel request failed")
	require.Equal(t, http.StatusOK, getExistingStatusCode, "Expected submodel to remain when delete transaction fails")
}

func TestThumbnailAttachmentOperations(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/thumbnail_test_%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	thumbnailEndpoint := fmt.Sprintf("%s/shells/%s/asset-information/thumbnail", baseURL, aasIdentifier)
	testFilePath := "testFiles/marcus.gif"

	statusCode, err := createAASForThumbnailTest(baseURL, aasID)
	require.NoError(t, err, "AAS creation failed")
	assert.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")

	originalContent, err := os.ReadFile(testFilePath)
	require.NoError(t, err, "Failed to read thumbnail test file")

	t.Run("0_Filename_UTF8_Byte_Boundaries", func(t *testing.T) {
		maximumASCIIName := strings.Repeat("a", 255)
		uploadStatus, uploadErr := uploadThumbnail(thumbnailEndpoint, testFilePath, maximumASCIIName)
		require.NoError(t, uploadErr, "255-byte thumbnail filename upload failed")
		require.Equal(t, http.StatusNoContent, uploadStatus, "Expected a 255-byte thumbnail filename to be accepted")

		uploadStatus, uploadErr = uploadThumbnail(thumbnailEndpoint, testFilePath, strings.Repeat("a", 256))
		require.NoError(t, uploadErr, "256-byte thumbnail filename request failed")
		require.Equal(t, http.StatusBadRequest, uploadStatus, "Expected a 256-byte thumbnail filename to be rejected")

		maximumMultibyteName := strings.Repeat("ü", 127) + "a"
		uploadStatus, uploadErr = uploadThumbnail(thumbnailEndpoint, testFilePath, maximumMultibyteName)
		require.NoError(t, uploadErr, "255-byte multibyte thumbnail filename upload failed")
		require.Equal(t, http.StatusNoContent, uploadStatus, "Expected a 255-byte multibyte thumbnail filename to be accepted")
	})

	t.Run("1_Upload_Thumbnail", func(t *testing.T) {
		uploadStatus, uploadErr := uploadThumbnail(thumbnailEndpoint, testFilePath, "marcus.gif")
		require.NoError(t, uploadErr, "Thumbnail upload failed")
		assert.Equal(t, http.StatusNoContent, uploadStatus, "Expected 204 No Content for thumbnail upload")
	})

	t.Run("2_Download_Thumbnail_And_Verify", func(t *testing.T) {
		time.Sleep(2 * time.Second)
		content, contentType, getStatus, getErr := downloadThumbnail(thumbnailEndpoint)
		require.NoError(t, getErr, "Thumbnail download failed")
		assert.Equal(t, http.StatusOK, getStatus, "Expected 200 OK for thumbnail download")
		assert.NotEmpty(t, contentType, "Content-Type should be set")
		t.Logf("Downloaded thumbnail Content-Type: %s", contentType)
		assert.Equal(t, originalContent, content, "Downloaded thumbnail content should match uploaded content")
		t.Logf("Thumbnail content verified: %d bytes", len(content))
	})

	t.Run("3_Get_AAS_By_ID_Includes_DefaultThumbnail_In_AssetInformation", func(t *testing.T) {
		aasEndpoint := fmt.Sprintf("%s/shells/%s", baseURL, aasIdentifier)
		payload, getStatus, getErr := getJSONResponse(aasEndpoint)
		require.NoError(t, getErr, "AAS retrieval failed")
		assert.Equal(t, http.StatusOK, getStatus, "Expected 200 OK for AAS retrieval")

		assetInformation, ok := payload["assetInformation"].(map[string]any)
		require.True(t, ok, "assetInformation should be present")

		thumbnail, ok := assetInformation["defaultThumbnail"].(map[string]any)
		require.True(t, ok, "assetInformation.defaultThumbnail should be present")

		thumbnailPath, ok := thumbnail["path"].(string)
		require.True(t, ok, "thumbnail.path should be a string")
		assert.True(t, strings.HasPrefix(thumbnailPath, "/aasx/files/"), "thumbnail.path should use a managed AASX part path")
		assert.True(t, strings.HasSuffix(thumbnailPath, "/marcus.gif"), "thumbnail.path should preserve the safe filename")

		thumbnailContentType, ok := thumbnail["contentType"].(string)
		require.True(t, ok, "thumbnail.contentType should be a string")
		assert.Equal(t, "image/gif", thumbnailContentType, "thumbnail.contentType should match uploaded file")
	})

	t.Run("4_Get_AAS_List_Includes_DefaultThumbnail_In_AssetInformation", func(t *testing.T) {
		listEndpoint := fmt.Sprintf("%s/shells", baseURL)
		payload, getStatus, getErr := getJSONResponse(listEndpoint)
		require.NoError(t, getErr, "AAS list retrieval failed")
		assert.Equal(t, http.StatusOK, getStatus, "Expected 200 OK for AAS list retrieval")

		result, ok := payload["result"].([]any)
		require.True(t, ok, "result should be an array")

		foundAAS := false
		for _, entry := range result {
			aasMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if aasMap["id"] != aasID {
				continue
			}

			foundAAS = true
			assetInformation, ok := aasMap["assetInformation"].(map[string]any)
			require.True(t, ok, "assetInformation should be present in listed AAS")

			thumbnail, ok := assetInformation["defaultThumbnail"].(map[string]any)
			require.True(t, ok, "assetInformation.defaultThumbnail should be present in listed AAS")

			thumbnailPath, ok := thumbnail["path"].(string)
			require.True(t, ok, "thumbnail.path should be a string in listed AAS")
			assert.True(t, strings.HasPrefix(thumbnailPath, "/aasx/files/"), "listed thumbnail.path should use a managed AASX part path")

			thumbnailContentType, ok := thumbnail["contentType"].(string)
			require.True(t, ok, "thumbnail.contentType should be a string in listed AAS")
			assert.Equal(t, "image/gif", thumbnailContentType, "thumbnail.contentType should match uploaded file in listed AAS")
			break
		}

		assert.True(t, foundAAS, "Expected uploaded AAS to be present in list response")
	})

	t.Run("5_Get_Thumbnail_Redirects_For_External_URL", func(t *testing.T) {
		externalThumbnailURL := "https://example.com/assets/thumbs/thumbnail-external.gif"
		setErr := setExternalThumbnailForAAS(aasID, externalThumbnailURL)
		require.NoError(t, setErr, "Failed to set external thumbnail URL")

		statusCode, locationHeader, requestErr := getThumbnailWithoutFollowingRedirect(thumbnailEndpoint)
		require.NoError(t, requestErr, "GET thumbnail request failed")
		assert.Equal(t, http.StatusFound, statusCode, "Expected 302 Found for external thumbnail URL")
		assert.Equal(t, externalThumbnailURL, locationHeader, "Expected redirect Location header for external thumbnail URL")
	})

	t.Run("6_Delete_Thumbnail", func(t *testing.T) {
		req, reqErr := http.NewRequest(http.MethodDelete, thumbnailEndpoint, nil)
		require.NoError(t, reqErr, "Failed to create DELETE request")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, doErr := client.Do(req)
		require.NoError(t, doErr, "DELETE request failed")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "Expected 204 No Content for thumbnail deletion")
	})

	t.Run("7_Verify_Thumbnail_Deleted", func(t *testing.T) {
		_, _, getStatus, getErr := downloadThumbnail(thumbnailEndpoint)
		require.NoError(t, getErr, "Thumbnail download after delete should not fail at HTTP level")
		assert.Equal(t, http.StatusNotFound, getStatus, "Expected 404 Not Found after thumbnail deletion")
	})

	t.Run("8_Upload_Thumbnail_For_NonExisting_AAS", func(t *testing.T) {
		nonExistingID := base64.RawStdEncoding.EncodeToString([]byte("https://example.com/ids/aas/non_existing_thumbnail_test"))
		nonExistingEndpoint := fmt.Sprintf("%s/shells/%s/asset-information/thumbnail", baseURL, nonExistingID)

		uploadStatus, uploadErr := uploadThumbnail(nonExistingEndpoint, testFilePath, "marcus.gif")
		require.NoError(t, uploadErr, "Upload request for non-existing AAS should complete")
		assert.Equal(t, http.StatusNotFound, uploadStatus, "Expected 404 Not Found for non-existing AAS")
	})
}

func TestDeleteAssetAdministrationShellUnlinksThumbnailLargeObject(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/thumbnail_delete_cleanup_%d", time.Now().UnixNano())
	encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	aasEndpoint := fmt.Sprintf("%s/shells/%s", baseURL, encodedAASID)
	thumbnailEndpoint := fmt.Sprintf("%s/asset-information/thumbnail", aasEndpoint)
	baselineCount := countPostgresLargeObjects(t, integrationTestDSN)

	createAASForLargeObjectCleanupTest(t, baseURL, aasID)
	defer deleteAASForLargeObjectCleanupTest(t, aasEndpoint)

	uploadStatus, uploadErr := uploadThumbnail(thumbnailEndpoint, "testFiles/marcus.gif", "marcus.gif")
	require.NoError(t, uploadErr)
	require.Equal(t, http.StatusNoContent, uploadStatus)
	require.Greater(t, countPostgresLargeObjects(t, integrationTestDSN), baselineCount)

	deleteStatus, deleteErr := deleteResponseStatus(aasEndpoint)
	require.NoError(t, deleteErr)
	require.Equal(t, http.StatusNoContent, deleteStatus)
	require.Equal(t, baselineCount, countPostgresLargeObjects(t, integrationTestDSN))
}

func TestPutAssetAdministrationShellUnlinksReplacedThumbnailLargeObject(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/thumbnail_put_cleanup_%d", time.Now().UnixNano())
	encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	aasEndpoint := fmt.Sprintf("%s/shells/%s", baseURL, encodedAASID)
	thumbnailEndpoint := fmt.Sprintf("%s/asset-information/thumbnail", aasEndpoint)
	baselineCount := countPostgresLargeObjects(t, integrationTestDSN)

	createAASForLargeObjectCleanupTest(t, baseURL, aasID)
	defer deleteAASForLargeObjectCleanupTest(t, aasEndpoint)

	uploadStatus, uploadErr := uploadThumbnail(thumbnailEndpoint, "testFiles/marcus.gif", "marcus.gif")
	require.NoError(t, uploadErr)
	require.Equal(t, http.StatusNoContent, uploadStatus)
	require.Greater(t, countPostgresLargeObjects(t, integrationTestDSN), baselineCount)

	_, putStatus, _, putErr := putJSONResponse(aasEndpoint, aasLargeObjectCleanupPayload(aasID))
	require.NoError(t, putErr)
	require.Equal(t, http.StatusNoContent, putStatus)
	require.Equal(t, baselineCount, countPostgresLargeObjects(t, integrationTestDSN))
}

func TestFullAssetAdministrationShellPutPreservesOwnedManagedThumbnail(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/thumbnail_put_preserve_%d", time.Now().UnixNano())
	encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	aasEndpoint := fmt.Sprintf("%s/shells/%s", baseURL, encodedAASID)
	thumbnailEndpoint := fmt.Sprintf("%s/asset-information/thumbnail", aasEndpoint)

	createAASForLargeObjectCleanupTest(t, baseURL, aasID)
	defer deleteAASForLargeObjectCleanupTest(t, aasEndpoint)
	uploadStatus, uploadErr := uploadThumbnail(thumbnailEndpoint, "testFiles/marcus.gif", "marcus.gif")
	require.NoError(t, uploadErr)
	require.Equal(t, http.StatusNoContent, uploadStatus)

	shell, getStatus, getErr := getJSONResponse(aasEndpoint)
	require.NoError(t, getErr)
	require.Equal(t, http.StatusOK, getStatus)
	managedPath := requireThumbnailPath(t, shell)
	shellBody, marshalErr := json.Marshal(shell)
	require.NoError(t, marshalErr)
	_, putStatus, _, putErr := putJSONResponse(aasEndpoint, string(shellBody))
	require.NoError(t, putErr)
	require.Equal(t, http.StatusNoContent, putStatus)

	content, _, downloadStatus, downloadErr := downloadThumbnail(thumbnailEndpoint)
	require.NoError(t, downloadErr)
	require.Equal(t, http.StatusOK, downloadStatus)
	expectedContent, readErr := os.ReadFile("testFiles/marcus.gif")
	require.NoError(t, readErr)
	require.Equal(t, expectedContent, content)
	updatedShell, updatedStatus, updatedErr := getJSONResponse(aasEndpoint)
	require.NoError(t, updatedErr)
	require.Equal(t, http.StatusOK, updatedStatus)
	require.Equal(t, managedPath, requireThumbnailPath(t, updatedShell))
}

func requireThumbnailPath(t *testing.T, shell map[string]any) string {
	t.Helper()
	assetInformation, ok := shell["assetInformation"].(map[string]any)
	require.True(t, ok)
	thumbnail, ok := assetInformation["defaultThumbnail"].(map[string]any)
	require.True(t, ok)
	path, ok := thumbnail["path"].(string)
	require.True(t, ok)
	return path
}

func createAASForLargeObjectCleanupTest(t *testing.T, baseURL string, aasID string) {
	t.Helper()

	statusCode, err := postResponseStatus(baseURL+"/shells", aasLargeObjectCleanupPayload(aasID))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode)
}

func aasLargeObjectCleanupPayload(aasID string) string {
	return fmt.Sprintf(
		`{"id":"%s","modelType":"AssetAdministrationShell","assetInformation":{"assetKind":"Instance","globalAssetId":"%s-global-asset"}}`,
		aasID,
		aasID,
	)
}

func deleteAASForLargeObjectCleanupTest(t *testing.T, aasEndpoint string) {
	t.Helper()

	statusCode, err := deleteResponseStatus(aasEndpoint)
	if err != nil {
		t.Logf("cleanup delete AAS failed: %v", err)
		return
	}
	if statusCode != http.StatusNoContent && statusCode != http.StatusNotFound {
		t.Logf("cleanup delete AAS returned status=%d", statusCode)
	}
}

func countPostgresLargeObjects(t *testing.T, dsn string) int64 {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	query, args, err := goqu.Dialect("postgres").
		From(goqu.T("pg_largeobject_metadata")).
		Select(goqu.COUNT("*")).
		ToSQL()
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.QueryRow(query, args...).Scan(&count))
	return count
}

func TestContractThumbnailGetReturnsDetectedContentType(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/thumbnail_contract_%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	thumbnailEndpoint := fmt.Sprintf("%s/shells/%s/asset-information/thumbnail", baseURL, aasIdentifier)

	statusCode, err := createAASForThumbnailTest(baseURL, aasID)
	require.NoError(t, err, "AAS creation failed")
	require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")

	testFilePath := "testFiles/marcus.gif"
	expectedContentType := "image/gif"
	expectedContent, readErr := os.ReadFile(testFilePath)
	require.NoError(t, readErr, "Failed to read thumbnail test file")

	uploadStatusCode, uploadErr := uploadThumbnail(thumbnailEndpoint, testFilePath, "contract-thumbnail.gif")
	require.NoError(t, uploadErr, "Thumbnail upload failed")
	require.Equal(t, http.StatusNoContent, uploadStatusCode, "Expected 204 No Content for thumbnail upload")

	content, contentType, getStatusCode, getErr := downloadThumbnail(thumbnailEndpoint)
	require.NoError(t, getErr, "Thumbnail download failed")
	require.Equal(t, http.StatusOK, getStatusCode, "Expected 200 OK for thumbnail download")
	assert.Equal(t, expectedContent, content, "Downloaded thumbnail content should match uploaded payload")
	assert.Equal(t, expectedContentType, contentType, "Thumbnail GET content type should match detected uploaded content type")
}

func TestThumbnailUploadUsesDeclaredContentTypeFallback(t *testing.T) {
	baseURL := aasRepositoryBaseURL
	aasID := fmt.Sprintf("https://example.com/ids/aas/thumbnail_declared_fallback_%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	thumbnailEndpoint := fmt.Sprintf("%s/shells/%s/asset-information/thumbnail", baseURL, aasIdentifier)

	statusCode, err := createAASForThumbnailTestWithDeclaredContentType(baseURL, aasID, "declared-thumbnail", "image/tiff")
	require.NoError(t, err, "AAS creation failed")
	require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")

	weakPayload := []byte{0x01, 0x02, 0x03, 0x04}
	weakFilePath := createTemporaryBinaryTestFile(t, "thumbnail-weak", weakPayload)

	uploadStatusCode, uploadErr := uploadThumbnail(thumbnailEndpoint, weakFilePath, "blue_tiff_jpeg_comp.tif")
	require.NoError(t, uploadErr, "Thumbnail upload failed")
	require.Equal(t, http.StatusNoContent, uploadStatusCode, "Expected 204 No Content for thumbnail upload")

	content, contentType, getStatusCode, getErr := downloadThumbnail(thumbnailEndpoint)
	require.NoError(t, getErr, "Thumbnail download failed")
	require.Equal(t, http.StatusOK, getStatusCode, "Expected 200 OK for thumbnail download")
	assert.Equal(t, weakPayload, content, "Downloaded thumbnail content should match uploaded payload")
	assert.Equal(t, "image/tiff", contentType, "Weak MIME detection should fall back to TIFF content type")

	payload, aasStatusCode, aasErr := getJSONResponse(fmt.Sprintf("%s/shells/%s", baseURL, aasIdentifier))
	require.NoError(t, aasErr, "AAS retrieval failed")
	require.Equal(t, http.StatusOK, aasStatusCode, "Expected 200 OK for AAS retrieval")

	assetInformation, ok := payload["assetInformation"].(map[string]any)
	require.True(t, ok, "assetInformation should be present")

	thumbnail, ok := assetInformation["defaultThumbnail"].(map[string]any)
	require.True(t, ok, "assetInformation.defaultThumbnail should be present")

	thumbnailContentType, ok := thumbnail["contentType"].(string)
	require.True(t, ok, "thumbnail.contentType should be a string")
	assert.Equal(t, "image/tiff", thumbnailContentType, "AAS payload should expose fallback-resolved thumbnail contentType")
}

func TestStandaloneStartupRejectsUnsupportedSubmodelRegistryToggle(t *testing.T) {
	if os.Getenv("BASYX_EXTERNAL_COMPOSE") == "1" {
		t.Skip("requires bundled integration docker compose setup")
	}

	assertServiceNeverHealthy(t, aasRepositoryInvalidBaseURL+"/health", 20*time.Second)
}

// TestMain handles setup and teardown
func TestMain(m *testing.M) {
	if os.Getenv("BASYX_EXTERNAL_COMPOSE") == "1" {
		testenv.SetEnvDefaultsOrExit(map[string]string{
			"BASYX_IT_API_URL":         aasRepositoryBaseURL,
			"BASYX_IT_INVALID_API_URL": aasRepositoryInvalidBaseURL,
		})
		os.Exit(m.Run())
	}

	runtime := testenv.NewComposeRuntimeOrExit("aasrepository-it", []testenv.PortBinding{
		{Name: "api", EnvVar: "BASYX_IT_API_PORT"},
		{Name: "db", EnvVar: "BASYX_IT_DB_PORT"},
		{Name: "invalid-api", EnvVar: "BASYX_IT_INVALID_API_PORT"},
	})
	aasRepositoryBaseURL = runtime.LocalURL("api")
	aasRepositoryInvalidBaseURL = runtime.LocalhostURL("invalid-api")
	integrationTestDSN = runtime.PostgresURL("db", "basyxTestDB")

	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     "docker_compose/docker_compose.yml",
		ProjectName:     runtime.ProjectName,
		Env:             runtime.Env(),
		PreDownBeforeUp: true,
		HealthURL:       aasRepositoryBaseURL + "/health",
		HealthTimeout:   150 * time.Second,
	}))
}
