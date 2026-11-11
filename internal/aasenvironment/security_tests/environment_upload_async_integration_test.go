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
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

var asyncUploadNoRedirectClient = &http.Client{
	Timeout: 60 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

type asyncEnvironmentUploadResult struct {
	ExecutionState string `json:"executionState"`
	Success        bool   `json:"success"`
	Messages       []struct {
		Code string `json:"code"`
		Text string `json:"text"`
	} `json:"messages"`
}

func checkEnvironmentAsyncUploadAuthorization(t *testing.T, admin, editor, id string) {
	t.Helper()
	asyncID := id + ":async"
	asyncRoutes := map[string]any{
		"ACL":     map[string]any{"ATTRIBUTES": []any{map[string]string{"CLAIM": "role"}}, "RIGHTS": []string{"READ"}, "ACCESS": "ALLOW"},
		"OBJECTS": []any{map[string]string{"ROUTE": "/upload-async/*"}},
		"FORMULA": map[string]bool{"$boolean": true},
	}
	installEnvironmentRules(t, admin, environmentObjectRule("cd", asyncID, "CREATE"), asyncRoutes)
	payload, err := json.Marshal(environmentFixture(asyncID, "Visible"))
	require.NoError(t, err)

	status, body, location := postAsyncEnvironmentFile(t, editor, "environment.json", payload)
	require.Equal(t, http.StatusAccepted, status, string(body))
	require.NotEmpty(t, location)

	result := waitForAsyncEnvironmentResult(t, editor, location)
	require.Equal(t, "Failed", result.ExecutionState)
	require.False(t, result.Success)
	require.NotEmpty(t, result.Messages)
	require.Equal(t, "403", result.Messages[0].Code)
	require.Contains(t, result.Messages[0].Text, "AASENV-PROCESSENV-ROLLEDBACK")
	require.Contains(t, result.Messages[0].Text, asyncID)
	for _, path := range []string{"/concept-descriptions/", "/submodels/", "/shells/"} {
		status, _ = doAuthorizedRequest(t, http.MethodGet, testBaseURL+path+common.EncodeString(asyncID), "", admin)
		require.Equal(t, http.StatusNotFound, status, "denied asynchronous upload must not leave %s behind", path)
	}

	status, _ = getAsyncEnvironmentURL(t, admin, location)
	require.Equal(t, http.StatusNotFound, status, "another principal must not read the upload handle")

	status, body, _ = postAsyncEnvironmentFile(t, "", "environment.json", payload)
	require.Equal(t, http.StatusUnauthorized, status, string(body))
}

func postAsyncEnvironmentFile(t *testing.T, token, name string, content []byte) (int, []byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/upload-async", &body)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := asyncUploadNoRedirectClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, responseBody, response.Header.Get("Location")
}

func waitForAsyncEnvironmentResult(t *testing.T, token, statusLocation string) asyncEnvironmentUploadResult {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		status, body, resultLocation := getAsyncEnvironmentResponse(t, token, statusLocation)
		if status == http.StatusFound {
			status, body, _ = getAsyncEnvironmentResponse(t, token, resultLocation)
			require.Equal(t, http.StatusOK, status, string(body))
			var result asyncEnvironmentUploadResult
			require.NoError(t, json.Unmarshal(body, &result), string(body))
			return result
		}
		require.Equal(t, http.StatusOK, status, string(body))
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("asynchronous upload did not finish in time")
	return asyncEnvironmentUploadResult{}
}

func getAsyncEnvironmentURL(t *testing.T, token, endpoint string) (int, []byte) {
	t.Helper()
	status, body, _ := getAsyncEnvironmentResponse(t, token, endpoint)
	return status, body
}

func getAsyncEnvironmentResponse(t *testing.T, token, endpoint string) (int, []byte, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := asyncUploadNoRedirectClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, body, response.Header.Get("Location")
}
