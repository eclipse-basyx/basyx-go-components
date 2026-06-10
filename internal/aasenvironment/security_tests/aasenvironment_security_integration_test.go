//nolint:all
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const actionUploadMultipart = "UPLOAD_MULTIPART"
const testBaseURL = "http://localhost:6004"
const testKeycloakTokenURL = "http://localhost:18080/realms/basyx/protocol/openid-connect/token"

func TestIntegration(t *testing.T) {
	tokenProvider := testenv.NewPasswordGrantTokenProvider(
		testKeycloakTokenURL,
		"basyx-ui",
		10*time.Second,
	)

	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		DefaultExpectedStatus: http.StatusOK,
		TokenProvider:         tokenProvider,
		ActionHandlers: map[string]testenv.JSONStepAction{
			actionUploadMultipart: func(t *testing.T, _ *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, _ int) {
				runMultipartUploadAction(t, step, tokenProvider)
			},
		},
	})
}

func TestIntegrationSerializationSecurity(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ConfigPath:            "serialization_it_config.json",
		DefaultExpectedStatus: http.StatusOK,
		ShouldCompareResponse: testenv.CompareMethods(http.MethodGet),
		TokenProvider: testenv.NewPasswordGrantTokenProvider(
			testKeycloakTokenURL,
			"basyx-ui",
			10*time.Second,
		),
	})
}

func TestSuperpathEndpointsSecurity(t *testing.T) {
	tokenProvider := testenv.NewPasswordGrantTokenProvider(
		testKeycloakTokenURL,
		"basyx-ui",
		10*time.Second,
	)

	adminToken, err := tokenProvider.GetAccessToken(&testenv.TokenCredentials{
		User:     "admin",
		Password: "pwd",
	})
	require.NoError(t, err)
	require.NotEmpty(t, adminToken)

	userXToken, err := tokenProvider.GetAccessToken(&testenv.TokenCredentials{
		User:     "userx",
		Password: "pwd",
	})
	require.NoError(t, err)
	require.NotEmpty(t, userXToken)

	encodedAAS := base64.RawURLEncoding.EncodeToString([]byte("urn:test:aas:security:missing"))
	encodedSubmodel := base64.RawURLEncoding.EncodeToString([]byte("urn:test:submodel:security:missing"))
	basePath := "/shells/" + encodedAAS + "/submodels/" + encodedSubmodel

	type endpointCase struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
	}

	testCases := []endpointCase{
		{name: "GET submodel", method: http.MethodGet, path: basePath},
		{name: "PUT submodel", method: http.MethodPut, path: basePath, body: `{"id":"urn:test:submodel:security:missing","idShort":"Missing","modelType":"Submodel","kind":"Instance","submodelElements":[]}`},
		{name: "DELETE submodel", method: http.MethodDelete, path: basePath},
		{name: "PATCH submodel", method: http.MethodPatch, path: basePath, body: `{"idShort":"UpdatedByPatch"}`},
		{name: "GET submodel metadata", method: http.MethodGet, path: basePath + "/$metadata"},
		{name: "PATCH submodel metadata", method: http.MethodPatch, path: basePath + "/$metadata", body: `{"idShort":"UpdatedMetadata"}`},
		{name: "GET submodel value", method: http.MethodGet, path: basePath + "/$value"},
		{name: "PATCH submodel value", method: http.MethodPatch, path: basePath + "/$value", body: `{"submodelElements":[]}`},
		{name: "GET submodel reference", method: http.MethodGet, path: basePath + "/$reference"},
		{name: "GET submodel path", method: http.MethodGet, path: basePath + "/$path"},
		{name: "GET submodel elements", method: http.MethodGet, path: basePath + "/submodel-elements"},
		{name: "POST submodel elements", method: http.MethodPost, path: basePath + "/submodel-elements", body: `{"idShort":"ElementX","modelType":"Property","valueType":"xs:string","value":"x"}`},
		{name: "GET submodel elements metadata", method: http.MethodGet, path: basePath + "/submodel-elements/$metadata"},
		{name: "GET submodel elements value", method: http.MethodGet, path: basePath + "/submodel-elements/$value"},
		{name: "GET submodel elements reference", method: http.MethodGet, path: basePath + "/submodel-elements/$reference"},
		{name: "GET submodel elements path", method: http.MethodGet, path: basePath + "/submodel-elements/$path"},
		{name: "GET submodel element", method: http.MethodGet, path: basePath + "/submodel-elements/NoSuchElement"},
		{name: "PUT submodel element", method: http.MethodPut, path: basePath + "/submodel-elements/NoSuchElement", body: `{"idShort":"NoSuchElement","modelType":"Property","valueType":"xs:string","value":"x"}`},
		{name: "POST submodel element", method: http.MethodPost, path: basePath + "/submodel-elements/NoSuchElement", body: `{"idShort":"NestedElement","modelType":"Property","valueType":"xs:string","value":"x"}`},
		{name: "DELETE submodel element", method: http.MethodDelete, path: basePath + "/submodel-elements/NoSuchElement"},
		{name: "PATCH submodel element", method: http.MethodPatch, path: basePath + "/submodel-elements/NoSuchElement", body: `{"value":"patched"}`},
		{name: "GET submodel element metadata", method: http.MethodGet, path: basePath + "/submodel-elements/NoSuchElement/$metadata"},
		{name: "PATCH submodel element metadata", method: http.MethodPatch, path: basePath + "/submodel-elements/NoSuchElement/$metadata", body: `{"idShort":"NoSuchElementPatched"}`},
		{name: "GET submodel element value", method: http.MethodGet, path: basePath + "/submodel-elements/NoSuchElement/$value"},
		{name: "PATCH submodel element value", method: http.MethodPatch, path: basePath + "/submodel-elements/NoSuchElement/$value", body: `{"value":"valueOnlyPatch"}`},
		{name: "GET submodel element reference", method: http.MethodGet, path: basePath + "/submodel-elements/NoSuchElement/$reference"},
		{name: "GET submodel element path", method: http.MethodGet, path: basePath + "/submodel-elements/NoSuchElement/$path"},
		{name: "GET attachment", method: http.MethodGet, path: basePath + "/submodel-elements/NoSuchElement/attachment"},
		{name: "PUT attachment", method: http.MethodPut, path: basePath + "/submodel-elements/NoSuchElement/attachment", body: `{"not":"multipart"}`},
		{name: "DELETE attachment", method: http.MethodDelete, path: basePath + "/submodel-elements/NoSuchElement/attachment"},
		{name: "POST invoke", method: http.MethodPost, path: basePath + "/submodel-elements/NoSuchElement/invoke", body: `{"inputArguments":[]}`},
		{name: "POST invoke value", method: http.MethodPost, path: basePath + "/submodel-elements/NoSuchElement/invoke/$value", body: `{"inputArguments":[]}`},
		{name: "POST invoke async", method: http.MethodPost, path: basePath + "/submodel-elements/NoSuchElement/invoke-async", body: `{"inputArguments":[]}`},
		{name: "POST invoke async value", method: http.MethodPost, path: basePath + "/submodel-elements/NoSuchElement/invoke-async/$value", body: `{"inputArguments":[]}`},
		{name: "GET operation status", method: http.MethodGet, path: basePath + "/submodel-elements/NoSuchElement/operation-status/dummyHandle"},
		{name: "GET operation results", method: http.MethodGet, path: basePath + "/submodel-elements/NoSuchElement/operation-results/dummyHandle"},
		{name: "GET operation results value", method: http.MethodGet, path: basePath + "/submodel-elements/NoSuchElement/operation-results/dummyHandle/$value"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			adminStatus, adminBody := runSuperpathRequest(t, tc.method, tc.path, tc.body, tc.contentType, adminToken)
			require.NotEqualf(t, http.StatusForbidden, adminStatus, "admin must not be forbidden for %s %s; body=%s", tc.method, tc.path, adminBody)
			require.NotEqualf(t, http.StatusUnauthorized, adminStatus, "admin must be authenticated for %s %s; body=%s", tc.method, tc.path, adminBody)

			userStatus, userBody := runSuperpathRequest(t, tc.method, tc.path, tc.body, tc.contentType, userXToken)
			require.Equalf(t, http.StatusForbidden, userStatus, "userx must be forbidden for %s %s; body=%s", tc.method, tc.path, userBody)
		})
	}
}

func TestABACPolicyManagementActivationChangesSecuredSubmodelAccess(t *testing.T) {
	tokenProvider := testenv.NewPasswordGrantTokenProvider(
		testKeycloakTokenURL,
		"basyx-ui",
		10*time.Second,
	)

	adminToken, err := tokenProvider.GetAccessToken(&testenv.TokenCredentials{User: "admin", Password: "pwd"})
	require.NoError(t, err)
	editorToken, err := tokenProvider.GetAccessToken(&testenv.TokenCredentials{User: "userx", Password: "pwd"})
	require.NoError(t, err)

	aasID := "urn:test:aas:abac-policy-story"
	submodelID := "urn:test:sm:abac-policy-story"
	encodedSubmodelID := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	submodelURL := testBaseURL + "/submodels/" + encodedSubmodelID

	createAASBody := fmt.Sprintf(`{
		"id":"%s",
		"idShort":"ABACPolicyStoryAAS",
		"modelType":"AssetAdministrationShell",
		"assetInformation":{"assetKind":"Instance","globalAssetId":"urn:test:asset:abac-policy-story"}
	}`, aasID)
	createSubmodelBody := fmt.Sprintf(`{
		"id":"%s",
		"idShort":"ABACPolicyStorySubmodel",
		"modelType":"Submodel",
		"kind":"Instance",
		"submodelElements":[]
	}`, submodelID)

	assertStatus(t, http.MethodPost, testBaseURL+"/shells", createAASBody, adminToken, http.StatusCreated)
	assertStatus(t, http.MethodPost, testBaseURL+"/submodels", createSubmodelBody, adminToken, http.StatusCreated)
	assertStatus(t, http.MethodGet, submodelURL, "", editorToken, http.StatusForbidden)

	activeVersionID := activePolicyVersionID(t, adminToken)
	draftVersionID := clonePolicyVersion(t, activeVersionID, adminToken)
	createEditorSubmodelReadRule(t, draftVersionID, adminToken)
	assertStatus(t, http.MethodGet, submodelURL, "", editorToken, http.StatusForbidden)
	validatePolicyVersion(t, draftVersionID, adminToken)
	activatePolicyVersion(t, draftVersionID, adminToken)
	assertStatus(t, http.MethodGet, submodelURL, "", editorToken, http.StatusOK)
}

func runSuperpathRequest(t *testing.T, method string, path string, body string, contentType string, bearerToken string) (int, string) {
	t.Helper()

	var bodyReader io.Reader
	if strings.TrimSpace(body) != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, testBaseURL+path, bodyReader)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	if bodyReader != nil {
		if strings.TrimSpace(contentType) == "" {
			if method == http.MethodPatch {
				contentType = "application/merge-patch+json"
			} else {
				contentType = "application/json"
			}
		}
		req.Header.Set("Content-Type", contentType)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(respBody)
}

func activePolicyVersionID(t *testing.T, bearerToken string) int64 {
	t.Helper()

	status, body := doAuthorizedRequest(t, http.MethodGet, testBaseURL+"/security/abac/policy-versions", "", bearerToken)
	require.Equalf(t, http.StatusOK, status, "list policy versions failed: %s", body)
	var versions []struct {
		VersionID int64  `json:"version_id"`
		Status    string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &versions))
	for _, version := range versions {
		if version.Status == "active" {
			return version.VersionID
		}
	}
	t.Fatalf("no active ABAC policy version found: %s", body)
	return 0
}

func clonePolicyVersion(t *testing.T, versionID int64, bearerToken string) int64 {
	t.Helper()

	endpoint := fmt.Sprintf("%s/security/abac/policy-versions/%d/clone", testBaseURL, versionID)
	status, body := doAuthorizedRequest(t, http.MethodPost, endpoint, "", bearerToken)
	require.Equalf(t, http.StatusCreated, status, "clone policy failed: %s", body)
	var version struct {
		VersionID int64  `json:"version_id"`
		Status    string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &version))
	require.Equal(t, "staged", version.Status)
	return version.VersionID
}

func createEditorSubmodelReadRule(t *testing.T, versionID int64, bearerToken string) {
	t.Helper()

	body := `{
		"rule": {
			"ACL": {
				"ATTRIBUTES": [{ "CLAIM": "role" }],
				"RIGHTS": ["READ"],
				"ACCESS": "ALLOW"
			},
			"OBJECTS": [{ "ROUTE": "/submodels/*" }],
			"FORMULA": {
				"$eq": [
					{ "$attribute": { "CLAIM": "role" } },
					{ "$strVal": "editor" }
				]
			}
		}
	}`
	endpoint := fmt.Sprintf("%s/security/abac/policy-versions/%d/rules", testBaseURL, versionID)
	status, response := doAuthorizedRequest(t, http.MethodPost, endpoint, body, bearerToken)
	require.Equalf(t, http.StatusOK, status, "create policy rule failed: %s", response)
}

func validatePolicyVersion(t *testing.T, versionID int64, bearerToken string) {
	t.Helper()

	endpoint := fmt.Sprintf("%s/security/abac/policy-versions/%d/validate", testBaseURL, versionID)
	status, body := doAuthorizedRequest(t, http.MethodPost, endpoint, "", bearerToken)
	require.Equalf(t, http.StatusOK, status, "validate policy failed: %s", body)
	var result struct {
		Valid bool `json:"valid"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &result))
	require.Truef(t, result.Valid, "policy validation failed: %s", body)
}

func activatePolicyVersion(t *testing.T, versionID int64, bearerToken string) {
	t.Helper()

	endpoint := fmt.Sprintf("%s/security/abac/policy-versions/%d/activate", testBaseURL, versionID)
	status, body := doAuthorizedRequest(t, http.MethodPost, endpoint, "", bearerToken)
	require.Equalf(t, http.StatusOK, status, "activate policy failed: %s", body)
	var version struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &version))
	require.Equal(t, "active", version.Status)
}

func assertStatus(t *testing.T, method string, endpoint string, body string, bearerToken string, expectedStatus int) {
	t.Helper()

	status, response := doAuthorizedRequest(t, method, endpoint, body, bearerToken)
	require.Equalf(t, expectedStatus, status, "%s %s returned unexpected status: %s", method, endpoint, response)
}

func doAuthorizedRequest(t *testing.T, method string, endpoint string, body string, bearerToken string) (int, string) {
	t.Helper()

	var reader io.Reader
	if strings.TrimSpace(body) != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, endpoint, reader)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(respBody)
}

func runMultipartUploadAction(t *testing.T, step testenv.JSONSuiteStep, tokenProvider testenv.JSONTokenProvider) {
	file, err := os.Open(step.Data)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)

	filePart, err := writer.CreateFormFile("file", filepath.Base(step.Data))
	require.NoError(t, err)

	_, err = io.Copy(filePart, file)
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

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

	if step.Token != nil {
		token, tokenErr := tokenProvider.GetAccessToken(step.Token)
		require.NoError(t, tokenErr)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	expectedStatus := step.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusOK
	}
	require.Equalf(t, expectedStatus, resp.StatusCode, "multipart upload failed: %s", string(respBody))
}

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     "docker_compose/docker_compose.yml",
		PreDownBeforeUp: true,
		HealthURL:       "http://localhost:6004/health",
		HealthTimeout:   3 * time.Minute,
	}))
}
