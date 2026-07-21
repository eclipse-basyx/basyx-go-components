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

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestAddHealthEndpoint_UsesJSONResponseFormat(t *testing.T) {
	router := chi.NewRouter()
	cfg := &Config{Server: ServerConfig{ContextPath: "/api"}}
	AddHealthEndpoint(router, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("expected content type %q, got %q", "application/json", contentType)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["status"] != "UP" {
		t.Fatalf("expected status field %q, got %q", "UP", body["status"])
	}
}

func TestAddHealthEndpointWithProbe_ReturnsServiceUnavailableOnProbeFailure(t *testing.T) {
	router := chi.NewRouter()
	cfg := &Config{Server: ServerConfig{ContextPath: "/api"}}
	AddHealthEndpointWithProbe(router, cfg, func() (bool, string) {
		return false, "AAS preconfiguration in progress"
	})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("expected content type %q, got %q", "application/json", contentType)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["status"] != "DOWN" {
		t.Fatalf("expected status field %q, got %q", "DOWN", body["status"])
	}
	if body["details"] != "AAS preconfiguration in progress" {
		t.Fatalf("expected details field %q, got %q", "AAS preconfiguration in progress", body["details"])
	}
}

func TestVerifyPayload_RawJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(`{"assetAdministrationShells":[],"submodels":[],"conceptDescriptions":[]}`))
	req.Header.Set("Content-Type", "application/json")

	result, err := VerifyPayload(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if format, ok := result["format"].(string); !ok || format != "json" {
		t.Fatalf("expected format json, got %#v", result["format"])
	}
	if _, ok := result["valid"].(bool); !ok {
		t.Fatalf("expected valid boolean field, got %#v", result["valid"])
	}
	if _, ok := result["messages"].([]string); !ok {
		t.Fatalf("expected messages []string field, got %#v", result["messages"])
	}
}

func TestVerifyPayload_RawXML(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader("<environment xmlns=\"https://admin-shell.io/aas/3/2\"><assetAdministrationShells /><submodels /><conceptDescriptions /></environment>"))
	req.Header.Set("Content-Type", "application/xml")

	result, err := VerifyPayload(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if format, ok := result["format"].(string); !ok || format != "xml" {
		t.Fatalf("expected format xml, got %#v", result["format"])
	}
}

func TestVerifyPayload_MultipartJSON(t *testing.T) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "environment.json")
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	_, err = part.Write([]byte(`{"assetAdministrationShells":[],"submodels":[],"conceptDescriptions":[]}`))
	if err != nil {
		t.Fatalf("write form payload failed: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/verify", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	result, verifyErr := VerifyPayload(req)
	if verifyErr != nil {
		t.Fatalf("expected no error, got %v", verifyErr)
	}
	if format, ok := result["format"].(string); !ok || format != "json" {
		t.Fatalf("expected format json, got %#v", result["format"])
	}
}

func TestVerifyPayload_MultipartPayloadFieldJSON(t *testing.T) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("payload", `{"assetAdministrationShells":[],"submodels":[],"conceptDescriptions":[]}`); err != nil {
		t.Fatalf("write payload field failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/verify", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	result, verifyErr := VerifyPayload(req)
	if verifyErr != nil {
		t.Fatalf("expected no error, got %v", verifyErr)
	}
	if format, ok := result["format"].(string); !ok || format != "json" {
		t.Fatalf("expected format json, got %#v", result["format"])
	}
}

func TestVerifyPayload_RawAASX(t *testing.T) {
	aasxPath := filepath.Join("..", "aasenvironment", "integration_tests", "testdata", "ProductionPlanSFKL.aasx")
	// #nosec G304 -- path is a static test fixture under repository-controlled testdata.
	aasxBytes, err := os.ReadFile(aasxPath)
	if err != nil {
		t.Fatalf("failed to read aasx fixture %s: %v", aasxPath, err)
	}

	req := httptest.NewRequest(http.MethodPost, "/verify", bytes.NewReader(aasxBytes))
	req.Header.Set("Content-Type", "application/aasx+xml")

	result, verifyErr := VerifyPayload(req)
	if verifyErr != nil {
		t.Fatalf("expected no error, got %v", verifyErr)
	}

	if format, ok := result["format"].(string); !ok || format != "aasx" {
		t.Fatalf("expected format aasx, got %#v", result["format"])
	}
}

func TestVerifyPayload_UnsupportedContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader("plain text"))
	req.Header.Set("Content-Type", "text/plain")

	_, err := VerifyPayload(req)
	if err == nil {
		t.Fatal("expected error for unsupported content type")
	}
}

func TestVerifyPayload_SingleAASJSON(t *testing.T) {
	payload := `{
  "idShort": "DelegatedOperationsAAS",
  "id": "https://example.com/ids/aas/delegated-operations-example",
  "assetInformation": {
    "assetKind": "Instance",
    "globalAssetId": "https://example.com/assets/delegated-operations-demo"
  },
  "submodels": [
    {
      "type": "ModelReference",
      "keys": [
        {
          "type": "Submodel",
          "value": "https://example.com/ids/sm/delegated-operations"
        }
      ]
    }
  ],
  "modelType": "AssetAdministrationShell"
}`

	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	result, err := VerifyPayload(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count, ok := result["assetAdministrationShellCount"].(int); !ok || count != 1 {
		t.Fatalf("expected assetAdministrationShellCount=1, got %#v", result["assetAdministrationShellCount"])
	}
}

func TestVerifyPayload_SingleSubmodelJSON(t *testing.T) {
	payload := `{
  "modelType": "Submodel",
  "id": "https://example.com/ids/sm/delegated-operations",
  "idShort": "DelegatedOperationsSubmodel",
  "kind": "Instance",
  "submodelElements": []
}`

	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	result, err := VerifyPayload(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count, ok := result["submodelCount"].(int); !ok || count != 1 {
		t.Fatalf("expected submodelCount=1, got %#v", result["submodelCount"])
	}
}

func TestVerifyPayload_SingleConceptDescriptionJSON(t *testing.T) {
	payload := `{
  "id": "urn:example:cd:editor:post-allowed",
  "idShort": "EditorAllowed",
  "modelType": "ConceptDescription"
}`

	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	result, err := VerifyPayload(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count, ok := result["conceptDescriptionCount"].(int); !ok || count != 1 {
		t.Fatalf("expected conceptDescriptionCount=1, got %#v", result["conceptDescriptionCount"])
	}
}

func TestVerifyPayload_SingleSubmodelElementJSON(t *testing.T) {
	payload := `{
  "modelType": "Property",
  "idShort": "numberA",
  "valueType": "xs:int",
  "value": "0"
}`

	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	result, err := VerifyPayload(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if format, ok := result["format"].(string); !ok || format != "json" {
		t.Fatalf("expected format json, got %#v", result["format"])
	}
}

func invalidPropertyWithMultipleVerificationIssuesPayload() string {
	return `{
  "modelType": "Property",
  "idShort": "1",
  "extensions": [],
  "description": [],
  "displayName": [],
  "supplementalSemanticIds": [],
  "qualifiers": [],
  "embeddedDataSpecifications": [],
  "valueType": "xs:int",
  "value": "not-an-int"}`
}

func assertVerificationMessagesInOrder(t *testing.T, messages []string) {
	t.Helper()
	expectedMessages := []string{
		"Extensions must be either not set or have at least one item.",
		"Description must be either not set or have at least one item.",
		"Display name must be either not set or have at least one item.",
		"Supplemental semantic IDs must be either not set or have at least one item.",
		"Constraint AASd-118",
		"Qualifiers must be either not set or have at least one item.",
		"Embedded data specifications must be either not set or have at least one item.",
		"Value must be consistent with the value type.",
		"IDShort: ID-short of Referables shall consist of at least two characters",
	}

	if len(messages) != len(expectedMessages) {
		t.Fatalf("expected %d verification messages, got %d: %#v", len(expectedMessages), len(messages), messages)
	}

	for i, expectedMessage := range expectedMessages {
		if !strings.Contains(messages[i], expectedMessage) {
			t.Fatalf("expected message %d to contain %q, got %q", i, expectedMessage, messages[i])
		}
	}
}

func TestVerifyPayload_ReturnsAllVerificationMessages(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(invalidPropertyWithMultipleVerificationIssuesPayload()))
	req.Header.Set("Content-Type", "application/json")

	result, err := VerifyPayload(req)
	if err != nil {
		t.Fatalf("expected no parse error, got %v", err)
	}

	if valid, ok := result["valid"].(bool); !ok || valid {
		t.Fatalf("expected valid=false, got %#v", result["valid"])
	}

	messages, ok := result["messages"].([]string)
	if !ok {
		t.Fatalf("expected messages []string field, got %#v", result["messages"])
	}
	assertVerificationMessagesInOrder(t, messages)
}

func TestAddVerificationEndpoint_ReturnsAllVerificationMessages(t *testing.T) {
	router := chi.NewRouter()
	cfg := &Config{}
	AddVerificationEndpoint(router, cfg)

	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(invalidPropertyWithMultipleVerificationIssuesPayload()))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var body struct {
		Valid    bool     `json:"valid"`
		Messages []string `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Valid {
		t.Fatal("expected valid=false")
	}
	assertVerificationMessagesInOrder(t, body.Messages)
}

func TestAddVerificationEndpoint_RawJSON(t *testing.T) {
	router := chi.NewRouter()
	cfg := &Config{}
	AddVerificationEndpoint(router, cfg)

	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(`{"assetAdministrationShells":[],"submodels":[],"conceptDescriptions":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("expected content type %q, got %q", "application/json", contentType)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if format, ok := body["format"].(string); !ok || format != "json" {
		t.Fatalf("expected format json, got %#v", body["format"])
	}
}

func TestAddVerificationEndpointDoesNotStageJSONInPostgreSQL(t *testing.T) {
	const payload = `{"assetAdministrationShells":[],"submodels":[],"conceptDescriptions":[]}`
	neverStage := func(context.Context, io.Reader, int64) (StagedUpload, error) {
		t.Fatal("JSON verification must not use the PostgreSQL stager")
		return nil, nil
	}

	tests := []struct {
		name    string
		request func() *http.Request
	}{
		{
			name: "raw",
			request: func() *http.Request {
				request := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(payload))
				request.Header.Set("Content-Type", "application/json")
				return request
			},
		},
		{
			name: "multipart",
			request: func() *http.Request {
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				part, err := writer.CreateFormFile("file", "environment.json")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = io.WriteString(part, payload); err != nil {
					t.Fatal(err)
				}
				if err = writer.Close(); err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(http.MethodPost, "/verify", &body)
				request.Header.Set("Content-Type", writer.FormDataContentType())
				return request
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := chi.NewRouter()
			cfg := &Config{General: GeneralConfig{UploadMaxSizeBytes: 4096}}
			AddVerificationEndpoint(router, cfg, neverStage)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, test.request())
			if response.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAddVerificationEndpointUsesMountedContextPath(t *testing.T) {
	router := chi.NewRouter()
	apiRouter := chi.NewRouter()
	cfg := &Config{Server: ServerConfig{ContextPath: "/api"}}
	AddVerificationEndpoint(apiRouter, cfg)
	router.Mount(cfg.Server.ContextPath, apiRouter)

	req := httptest.NewRequest(http.MethodPost, "/api/verify", strings.NewReader(`{"assetAdministrationShells":[],"submodels":[],"conceptDescriptions":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestAddVerificationEndpoint_RejectsRawPayloadOverConfiguredLimit(t *testing.T) {
	router := chi.NewRouter()
	cfg := &Config{
		Server:  ServerConfig{ContextPath: "/api"},
		General: GeneralConfig{UploadMaxSizeBytes: 8},
	}
	AddVerificationEndpoint(router, cfg)

	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(`{"assetAdministrationShells":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
	}
	assertStandardErrorResponse(t, rec.Body.Bytes(), "413")
}

func TestAddVerificationEndpoint_RejectsMultipartPayloadOverConfiguredLimit(t *testing.T) {
	router := chi.NewRouter()
	cfg := &Config{
		Server:  ServerConfig{ContextPath: "/api"},
		General: GeneralConfig{UploadMaxSizeBytes: 64},
	}
	AddVerificationEndpoint(router, cfg)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", "environment.json")
	if err != nil {
		t.Fatalf("failed to create multipart file: %v", err)
	}
	if _, err = fileWriter.Write([]byte(`{"assetAdministrationShells":[],"submodels":[],"conceptDescriptions":[]}`)); err != nil {
		t.Fatalf("failed to write multipart file: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/verify", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
	}
	assertStandardErrorResponse(t, rec.Body.Bytes(), "413")
}

func TestAddVerificationEndpointRejectsExpandedAASXLimitWithStaging(t *testing.T) {
	aasxPath := filepath.Join("..", "aasenvironment", "integration_tests", "testdata", "ProductionPlanSFKL.aasx")
	// #nosec G304 -- the path resolves to a repository-owned test fixture.
	aasxBytes, err := os.ReadFile(aasxPath)
	if err != nil {
		t.Fatalf("failed to read AASX fixture: %v", err)
	}
	cfg := &Config{General: GeneralConfig{
		UploadMaxSizeBytes:            int64(len(aasxBytes)) + 1024,
		AASXMaxPartCount:              1,
		AASXMaxOPCMetadataSizeBytes:   16 << 20,
		AASXMaxPartExpandedSizeBytes:  128 << 20,
		AASXMaxTotalExpandedSizeBytes: 128 << 20,
		AASXMaxThumbnailSizeBytes:     16 << 20,
	}}
	router := chi.NewRouter()
	AddVerificationEndpoint(router, cfg, multipartMemoryStager)
	request := httptest.NewRequest(http.MethodPost, "/verify", bytes.NewReader(aasxBytes))
	request.Header.Set("Content-Type", "application/aasx+json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAddVerificationEndpointPreservesStagingErrorStatus(t *testing.T) {
	tests := []struct {
		name       string
		stagingErr error
		status     int
	}{
		{name: "service unavailable", stagingErr: NewErrServiceUnavailable("TEST-VERIFY-STAGEUNAVAILABLE"), status: http.StatusServiceUnavailable},
		{name: "internal error", stagingErr: NewInternalServerError("TEST-VERIFY-STAGEFAILED"), status: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stager := func(context.Context, io.Reader, int64) (StagedUpload, error) {
				return nil, test.stagingErr
			}
			router := chi.NewRouter()
			AddVerificationEndpoint(router, &Config{General: GeneralConfig{UploadMaxSizeBytes: 1024}}, stager)
			request := httptest.NewRequest(http.MethodPost, "/verify", bytes.NewReader([]byte{'P', 'K', 3, 4}))
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != test.status {
				t.Fatalf("expected status %d, got %d: %s", test.status, response.Code, response.Body.String())
			}
		})
	}
}

func assertStandardErrorResponse(t *testing.T, responseBody []byte, expectedCode string) {
	t.Helper()

	var body []ErrorHandler
	if err := json.Unmarshal(responseBody, &body); err != nil {
		t.Fatalf("failed to decode standardized error response: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("expected one error entry, got %d", len(body))
	}
	if body[0].MessageType != "Error" || body[0].Code != expectedCode {
		t.Fatalf("expected standardized error code %q, got %#v", expectedCode, body[0])
	}
	if body[0].CorrelationID == "" || body[0].Timestamp == "" {
		t.Fatalf("expected correlation ID and timestamp, got %#v", body[0])
	}
}
