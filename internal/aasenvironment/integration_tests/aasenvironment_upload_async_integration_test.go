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
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/require"
)

const asyncUploadPollTimeout = 60 * time.Second

type asyncUploadOperationResult struct {
	ExecutionState string `json:"executionState"`
	Success        bool   `json:"success"`
	Messages       []struct {
		MessageType string `json:"messageType"`
		Text        string `json:"text"`
	} `json:"messages"`
}

func TestUploadAsyncAASXStoresCompleteEnvironment(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)

	result := runAsyncUpload(t, buildAtomicUploadPackage(t, "Shell", false), "atomic-upload.aasx")
	require.Equal(t, "Completed", result.ExecutionState)
	require.True(t, result.Success)
	require.NotEmpty(t, result.Messages)
	require.Contains(t, result.Messages[0].Text, "AASENV-UPLOADASYNC-COMPLETED")

	requireAtomicUploadResourceStatus(t, http.StatusOK)
	requireUploadStatus(t, "/submodels/"+encodeAtomicUploadID(atomicUploadSubmodelID)+"/submodel-elements/Manual/attachment", http.StatusOK)
	requireUploadStatus(t, "/shells/"+encodeAtomicUploadID(atomicUploadShellID)+"/asset-information/thumbnail", http.StatusOK)
	require.Zero(t, countAsyncUploadStagingRows(t), "a completed upload must release its durable staging")
}

func TestUploadAsyncAASXWithRejectedLastAttachmentStoresNothing(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	before := readAtomicUploadPersistedState(t)

	result := runAsyncUpload(t, buildAtomicUploadPackage(t, "Shell", true), "atomic-upload-invalid-attachment.aasx")
	require.Equal(t, "Failed", result.ExecutionState)
	require.False(t, result.Success)
	requireAsyncUploadRejection(t, result, "BINARYCONTENT-FILENAME-ESCAPE", atomicUploadSubmodelID)

	requireAtomicUploadResourceStatus(t, http.StatusNotFound)
	require.Equal(t, before, readAtomicUploadPersistedState(t), "a failed asynchronous upload must not leave resources, descriptors, binaries or staging behind")
	require.Zero(t, countAsyncUploadStagingRows(t), "a failed upload must release its durable staging")
}

func TestUploadAsyncJSONWithRejectedLastShellStoresNothing(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	before := readAtomicUploadPersistedState(t)

	result := runAsyncUpload(t, atomicUploadSpecification(t, "Shell;1"), "atomic-upload-invalid-shell.json")
	require.Equal(t, "Failed", result.ExecutionState)
	requireAsyncUploadRejection(t, result, "-INVALIDIDSHORT", atomicUploadSecondShellID)

	requireAtomicUploadResourceStatus(t, http.StatusNotFound)
	require.Equal(t, before, readAtomicUploadPersistedState(t))
}

func TestUploadAsyncRejectsUnsupportedFormatBeforeAcceptance(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)

	response, body := postAsyncUpload(t, []byte("plain text"), "environment.txt")
	require.Equal(t, http.StatusUnsupportedMediaType, response.StatusCode, string(body))
	require.Empty(t, response.Header.Get("Location"))
	require.Zero(t, countAsyncUploadJobs(t), "a rejected request must not create a job")
}

func TestUploadAsyncUnknownAndMalformedHandles(t *testing.T) {
	unknownHandle := encodeAtomicUploadID("AASENV-UPLOAD-unknown")
	for _, path := range []string{"/upload-async/status/" + unknownHandle, "/upload-async/result/" + unknownHandle} {
		status, body := getAsyncUploadPath(t, path)
		require.Equal(t, http.StatusNotFound, status, "%s: %s", path, string(body))
	}
	status, body := getAsyncUploadPath(t, "/upload-async/status/not*base64")
	require.Equal(t, http.StatusBadRequest, status, string(body))
}

func runAsyncUpload(t *testing.T, content []byte, fileName string) asyncUploadOperationResult {
	t.Helper()
	response, body := postAsyncUpload(t, content, fileName)
	require.Equal(t, http.StatusAccepted, response.StatusCode, string(body))
	var handle struct {
		HandleID string `json:"handleId"`
	}
	require.NoError(t, json.Unmarshal(body, &handle))
	require.NotEmpty(t, handle.HandleID)
	location := response.Header.Get("Location")
	require.True(t, strings.HasSuffix(location, "/upload-async/status/"+encodeAtomicUploadID(handle.HandleID)), "unexpected Location %q", location)

	resultLocation := waitForAsyncUploadResultLocation(t, location)
	require.True(t, strings.HasSuffix(resultLocation, "/upload-async/result/"+encodeAtomicUploadID(handle.HandleID)), "unexpected result Location %q", resultLocation)
	status, resultBody := getAsyncUploadURL(t, resultLocation)
	require.Equal(t, http.StatusOK, status, string(resultBody))
	var result asyncUploadOperationResult
	require.NoError(t, json.Unmarshal(resultBody, &result), string(resultBody))
	return result
}

func waitForAsyncUploadResultLocation(t *testing.T, statusLocation string) string {
	t.Helper()
	deadline := time.Now().Add(asyncUploadPollTimeout)
	for time.Now().Before(deadline) {
		response := doHTTPIntegrationRequest(t, aasEnvNoRedirectClient, mustNewRequest(t, http.MethodGet, statusLocation))
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		require.NoError(t, err)
		if response.StatusCode == http.StatusFound {
			return response.Header.Get("Location")
		}
		require.Equal(t, http.StatusOK, response.StatusCode, string(body))
		var running asyncUploadOperationResult
		require.NoError(t, json.Unmarshal(body, &running))
		require.Equal(t, "Running", running.ExecutionState)
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("asynchronous upload did not finish within %s", asyncUploadPollTimeout)
	return ""
}

func requireAsyncUploadRejection(t *testing.T, result asyncUploadOperationResult, cause string, failedResourceID string) {
	t.Helper()
	require.NotEmpty(t, result.Messages)
	text := result.Messages[0].Text
	require.Equal(t, "Error", result.Messages[0].MessageType)
	require.Contains(t, text, "AASENV-PROCESSENV-ROLLEDBACK", "result must state that nothing was stored")
	require.Contains(t, text, cause, "result must name the rejection cause")
	require.Contains(t, text, failedResourceID, "result must name the rejected resource")
}

func postAsyncUpload(t *testing.T, content []byte, fileName string) (*http.Response, []byte) {
	t.Helper()
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	filePart, err := writer.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = filePart.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request, err := http.NewRequest(http.MethodPost, aasEnvBaseURL+"/upload-async", &payload)
	require.NoError(t, err)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := doHTTPIntegrationRequest(t, aasEnvNoRedirectClient, request)
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response, body
}

func getAsyncUploadPath(t *testing.T, path string) (int, []byte) {
	t.Helper()
	return getAsyncUploadURL(t, aasEnvBaseURL+path)
}

func getAsyncUploadURL(t *testing.T, endpoint string) (int, []byte) {
	t.Helper()
	response := doHTTPIntegrationRequest(t, aasEnvNoRedirectClient, mustNewRequest(t, http.MethodGet, endpoint))
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, body
}

func countAsyncUploadStagingRows(t *testing.T) int64 {
	t.Helper()
	return countAsyncUploadTableRows(t, goqu.T("aasx_async_upload"))
}

func countAsyncUploadJobs(t *testing.T) int64 {
	t.Helper()
	return countAsyncUploadTableRows(t, goqu.T("async_job"))
}

func countAsyncUploadTableRows(t *testing.T, table any) int64 {
	t.Helper()
	db, err := sql.Open("pgx", uploadIntegrationDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	return countAtomicUploadRows(t, db, table)
}
