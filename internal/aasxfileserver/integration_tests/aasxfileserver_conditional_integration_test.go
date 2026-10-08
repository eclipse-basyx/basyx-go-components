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
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

func multipartPackage(t *testing.T, relativePath string) ([]byte, string) {
	t.Helper()
	file, err := os.Open(filepath.Clean(relativePath))
	require.NoError(t, err)
	defer func() { _ = file.Close() }()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(relativePath))
	require.NoError(t, err)
	_, err = io.Copy(part, file)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return body.Bytes(), writer.FormDataContentType()
}

func putPackageConditionally(t *testing.T, packageURL string, relativePath string, headers map[string]string) testenv.HTTPResult {
	t.Helper()
	body, contentType := multipartPackage(t, relativePath)
	merged := map[string]string{"Content-Type": contentType}
	for key, value := range headers {
		merged[key] = value
	}
	return testenv.DoHTTP(t, http.MethodPut, packageURL, body, merged)
}

func TestConditionalPackageLifecycle(t *testing.T) {
	uploadBody, uploadStatus, _ := uploadAASXPackage(t, "../../aasenvironment/integration_tests/testdata/IESEDriveMotorDM3000.aasx")
	require.Equal(t, http.StatusCreated, uploadStatus)
	packageURL := baseURL + "/packages/" + decodePackageDescription(t, uploadBody).PackageId
	t.Cleanup(func() { testenv.DoHTTP(t, http.MethodDelete, packageURL, nil, nil) })

	download := testenv.DoHTTP(t, http.MethodGet, packageURL, nil, nil)
	require.Equal(t, http.StatusOK, download.Status)
	etag := download.Header.Get("ETag")
	require.Regexp(t, `^"\d+-[0-9a-f]{8}-[0-9a-f]{16}"$`, etag)
	notModified := testenv.DoHTTP(t, http.MethodGet, packageURL, nil, map[string]string{"If-None-Match": etag})
	require.Equal(t, http.StatusNotModified, notModified.Status)
	require.Empty(t, notModified.Body)

	replacement := "../../aasenvironment/integration_tests/testdata/ProductionPlanSFKL.aasx"
	require.Equal(t, http.StatusPreconditionFailed, putPackageConditionally(t, packageURL, replacement, map[string]string{"If-Match": `"1-00000000"`}).Status)
	require.Equal(t, http.StatusPreconditionFailed, putPackageConditionally(t, packageURL, replacement, map[string]string{"If-None-Match": "*"}).Status)
	require.Equal(t, etag, testenv.DoHTTP(t, http.MethodGet, packageURL, nil, nil).Header.Get("ETag"))
	require.Equal(t, http.StatusNoContent, putPackageConditionally(t, packageURL, replacement, map[string]string{"If-Match": etag}).Status)

	current := testenv.DoHTTP(t, http.MethodGet, packageURL, nil, nil).Header.Get("ETag")
	require.NotEqual(t, etag, current)
	require.Equal(t, http.StatusPreconditionFailed, testenv.DoHTTP(t, http.MethodDelete, packageURL, nil, map[string]string{"If-Match": etag}).Status)
	require.Equal(t, http.StatusNoContent, testenv.DoHTTP(t, http.MethodDelete, packageURL, nil, map[string]string{"If-Match": current}).Status)
}
