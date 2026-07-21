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

package aasenvironment

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

type captureUploadService struct {
	fileName    string
	contentType string
	content     []byte
}

type memoryStagedUpload struct {
	*bytes.Reader
}

func (upload *memoryStagedUpload) Size() int64  { return upload.Reader.Size() }
func (upload *memoryStagedUpload) Close() error { return nil }
func (upload *memoryStagedUpload) Promote(context.Context, func(context.Context, *sql.Tx, int64, int64) error) error {
	return nil
}

func memoryUploadStager(_ context.Context, input io.Reader, maximum int64) (common.StagedUpload, error) {
	content, err := io.ReadAll(io.LimitReader(input, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maximum {
		return nil, common.NewErrPayloadTooLarge("TEST-UPLOAD-TOOLARGE")
	}
	return &memoryStagedUpload{Reader: bytes.NewReader(content)}, nil
}

func (s *captureUploadService) HandleUpload(_ context.Context, fileName string, contentType string, file io.ReadSeeker) (commonmodel.ImplResponse, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return commonmodel.ImplResponse{}, err
	}

	s.fileName = fileName
	s.contentType = contentType
	s.content = content

	return commonmodel.Response(http.StatusOK, map[string]string{"status": "ok"}), nil
}

func TestUploadDoesNotRequireWritableTempDirectory(t *testing.T) {
	tempRoot := t.TempDir()
	noWriteTemp := filepath.Join(tempRoot, "no-write")
	if err := os.Mkdir(noWriteTemp, 0o500); err != nil {
		t.Fatalf("failed to create non-writable temp dir: %v", err)
	}
	t.Cleanup(func() {
		// #nosec G302 -- test cleanup restores owner access on a temporary directory created by this test.
		_ = os.Chmod(noWriteTemp, 0o700)
	})
	t.Setenv("TMPDIR", noWriteTemp)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("fileName", "../environment.json"); err != nil {
		t.Fatalf("failed to write fileName field: %v", err)
	}
	part, err := writer.CreateFormFile("file", "environment.json")
	if err != nil {
		t.Fatalf("failed to create file part: %v", err)
	}
	if _, err = part.Write([]byte(`{"assetAdministrationShells":[],"submodels":[],"conceptDescriptions":[]}`)); err != nil {
		t.Fatalf("failed to write file part: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	service := &captureUploadService{}
	router := chi.NewRouter()
	RegisterUploadAPI(router, service, 1024, memoryUploadStager)

	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected upload to succeed without writable TMPDIR, got status %d body %s", response.Code, response.Body.String())
	}
	if service.fileName != "environment.json" {
		t.Fatalf("expected sanitized file name environment.json, got %q", service.fileName)
	}
	if service.contentType != "application/octet-stream" {
		t.Fatalf("expected default multipart file content type, got %q", service.contentType)
	}
	if !bytes.Contains(service.content, []byte(`"assetAdministrationShells"`)) {
		t.Fatalf("expected service to receive uploaded content, got %q", string(service.content))
	}
}

func TestUploadErrorStatusPreservesStagingFailures(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{err: common.NewErrPayloadTooLarge("TEST-UPLOAD-TOOLARGE"), status: http.StatusRequestEntityTooLarge},
		{err: common.NewErrServiceUnavailable("TEST-UPLOAD-UNAVAILABLE"), status: http.StatusServiceUnavailable},
		{err: common.NewInternalServerError("TEST-UPLOAD-STAGEFAILED"), status: http.StatusInternalServerError},
		{err: common.NewErrBadRequest("TEST-UPLOAD-BADREQUEST"), status: http.StatusBadRequest},
	}

	for _, test := range tests {
		if status := uploadErrorStatus(test.err); status != test.status {
			t.Fatalf("expected status %d for %v, got %d", test.status, test.err, status)
		}
	}
}
