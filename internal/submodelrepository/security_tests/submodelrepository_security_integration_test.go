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
package main

import (
	"bytes"
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

const actionUploadAttachmentMultipart = "UPLOAD_ATTACHMENT_MULTIPART"

func TestIntegration(t *testing.T) {
	tokenProvider := testenv.NewPasswordGrantTokenProvider(
		"http://localhost:8080/realms/basyx/protocol/openid-connect/token",
		"basyx-ui",
		10*time.Second,
	)

	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		DefaultExpectedStatus: http.StatusOK,
		ShouldCompareResponse: testenv.CompareMethods(http.MethodGet, http.MethodPost),
		TokenProvider:         tokenProvider,
		ActionHandlers: map[string]testenv.JSONStepAction{
			actionUploadAttachmentMultipart: func(t *testing.T, _ *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, _ int) {
				runAttachmentUploadAction(t, step, tokenProvider)
			},
		},
	})
}

func runAttachmentUploadAction(t *testing.T, step testenv.JSONSuiteStep, tokenProvider testenv.JSONTokenProvider) {
	file, err := os.Open(step.Data)
	if err != nil {
		require.NoError(t, err)
		return
	}
	defer func() { _ = file.Close() }()

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	filePart, err := writer.CreateFormFile("file", filepath.Base(step.Data))
	if err != nil {
		require.NoError(t, err)
		return
	}
	if _, err := io.Copy(filePart, file); err != nil {
		require.NoError(t, err)
		return
	}
	if fileName := strings.TrimSpace(step.Headers["X-Upload-FileName"]); fileName != "" {
		if err := writer.WriteField("fileName", fileName); err != nil {
			require.NoError(t, err)
			return
		}
	}
	if err := writer.Close(); err != nil {
		require.NoError(t, err)
		return
	}

	method := strings.ToUpper(step.Method)
	if method == "" {
		method = http.MethodPut
	}
	req, err := http.NewRequest(method, step.Endpoint, payload)
	if err != nil {
		require.NoError(t, err)
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for key, value := range step.Headers {
		if strings.EqualFold(key, "X-Upload-FileName") {
			continue
		}
		req.Header.Set(key, value)
	}

	if step.Token != nil {
		token, tokenErr := tokenProvider.GetAccessToken(step.Token)
		require.NoError(t, tokenErr)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		require.NoError(t, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		require.NoError(t, err)
		return
	}

	expectedStatus := step.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusOK
	}
	require.Equalf(t, expectedStatus, resp.StatusCode, "attachment upload failed: %s", string(respBody))
}

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     "docker_compose/docker_compose.yml",
		PreDownBeforeUp: true,
		HealthURL:       "http://localhost:6004/health",
		HealthTimeout:   150 * time.Second,
	}))
}
