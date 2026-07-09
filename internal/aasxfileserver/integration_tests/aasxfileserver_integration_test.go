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
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasxfileserverapi/go"
	"github.com/stretchr/testify/require"
)

const composeFilePath = "./docker_compose/docker_compose.yml"

var baseURL = testenv.LocalURLFromEnv("BASYX_IT_API_PORT", 6004)

func TestMain(m *testing.M) {
	if os.Getenv("BASYX_EXTERNAL_COMPOSE") == "1" {
		os.Exit(m.Run())
	}

	runtime := testenv.NewComposeRuntimeOrExit("aasxfileserver-it", []testenv.PortBinding{
		{Name: "api", EnvVar: "BASYX_IT_API_PORT"},
		{Name: "db", EnvVar: "BASYX_IT_DB_PORT"},
	})
	baseURL = runtime.LocalURL("api")

	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     composeFilePath,
		ProjectName:     runtime.ProjectName,
		Env:             runtime.Env(),
		PreDownBeforeUp: true,
		HealthURL:       baseURL + "/health",
		HealthTimeout:   2 * time.Minute,
	}))
}

func TestPackageLifecycle(t *testing.T) {
	uploadBody, uploadStatus, uploadHeaders := uploadAASXPackage(t, "../../aasenvironment/integration_tests/testdata/IESEDriveMotorDM3000.aasx")
	require.Equal(t, http.StatusCreated, uploadStatus)
	require.NotEmpty(t, uploadHeaders.Get("Location"))

	created := decodePackageDescription(t, uploadBody)
	require.NotEmpty(t, created.PackageId)

	listResult, listStatus := listPackages(t)
	require.Equal(t, http.StatusOK, listStatus)
	require.NotEmpty(t, listResult.Result)
	require.Contains(t, collectPackageIDs(listResult.Result), created.PackageId)

	downloadResp, downloadBody := request(t, http.MethodGet, baseURL+"/packages/"+created.PackageId, nil, "")
	require.Equal(t, http.StatusOK, downloadResp.StatusCode)
	require.NotEmpty(t, downloadResp.Header.Get("X-FileName"))
	require.NotEmpty(t, downloadBody)

	updateStatus := putAASXPackage(t, created.PackageId, "../../aasenvironment/integration_tests/testdata/ProductionPlanSFKL.aasx")
	require.Equal(t, http.StatusNoContent, updateStatus)

	deleteResp, _ := request(t, http.MethodDelete, baseURL+"/packages/"+created.PackageId, nil, "")
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	notFoundResp, _ := request(t, http.MethodGet, baseURL+"/packages/"+created.PackageId, nil, "")
	require.Equal(t, http.StatusNotFound, notFoundResp.StatusCode)
}

func uploadAASXPackage(t *testing.T, relativePath string) ([]byte, int, http.Header) {
	t.Helper()

	filePath := filepath.Clean(relativePath)
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	filePart, err := writer.CreateFormFile("file", filepath.Base(filePath))
	require.NoError(t, err)
	_, err = io.Copy(filePart, file)
	require.NoError(t, err)

	require.NoError(t, writer.Close())

	resp, responseBody := request(t, http.MethodPost, baseURL+"/packages", body, writer.FormDataContentType())
	return responseBody, resp.StatusCode, resp.Header
}

func putAASXPackage(t *testing.T, packageID string, relativePath string) int {
	t.Helper()

	filePath := filepath.Clean(relativePath)
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	filePart, err := writer.CreateFormFile("file", filepath.Base(filePath))
	require.NoError(t, err)
	_, err = io.Copy(filePart, file)
	require.NoError(t, err)

	require.NoError(t, writer.Close())

	resp, _ := request(t, http.MethodPut, baseURL+"/packages/"+packageID, body, writer.FormDataContentType())
	return resp.StatusCode
}

func listPackages(t *testing.T) (openapi.GetPackageDescriptionsResult, int) {
	t.Helper()

	resp, body := request(t, http.MethodGet, baseURL+"/packages", nil, "")
	var result openapi.GetPackageDescriptionsResult
	require.NoError(t, json.Unmarshal(body, &result))
	return result, resp.StatusCode
}

func decodePackageDescription(t *testing.T, body []byte) openapi.PackageDescription {
	t.Helper()

	var created openapi.PackageDescription
	require.NoError(t, json.Unmarshal(body, &created))
	return created
}

func collectPackageIDs(items []openapi.PackageDescription) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.PackageId)
	}
	return result
}

func request(t *testing.T, method string, url string, body io.Reader, contentType string) (*http.Response, []byte) {
	t.Helper()

	req, err := http.NewRequest(method, url, body)
	require.NoError(t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	// #nosec G704 -- Integration tests call a fixed local compose URL only.
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, responseBody
}
