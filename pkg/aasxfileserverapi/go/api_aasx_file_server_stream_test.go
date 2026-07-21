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
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
* IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
* CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
* TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
* SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

// Package openapi tests the generated AASX File Server transport adapters.
package openapi

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

type controllerMemoryStage struct{ *bytes.Reader }

func (stage *controllerMemoryStage) Size() int64  { return stage.Reader.Size() }
func (stage *controllerMemoryStage) Close() error { return nil }
func (stage *controllerMemoryStage) Promote(context.Context, func(context.Context, *sql.Tx, int64, int64) error) error {
	return nil
}

func controllerMemoryStager(_ context.Context, input io.Reader, maximum int64) (common.StagedUpload, error) {
	content, err := io.ReadAll(io.LimitReader(input, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maximum {
		return nil, common.NewErrPayloadTooLarge("AASXFILES-TESTSTAGE-TOOLARGE")
	}
	return &controllerMemoryStage{Reader: bytes.NewReader(content)}, nil
}

type captureAASXFileServerService struct {
	fileName string
	aasIDs   []string
	content  []byte
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

type failingResponseWriter struct {
	header http.Header
}

func (writer *failingResponseWriter) Header() http.Header {
	return writer.header
}

func (*failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("client disconnected")
}

func (*failingResponseWriter) WriteHeader(int) {}

func (reader *trackingReadCloser) Close() error {
	reader.closed = true
	return nil
}

func (service *captureAASXFileServerService) GetAllAASXPackageIds(context.Context, string, int32, string) (ImplResponse, error) {
	return Response(http.StatusOK, nil), nil
}

func (service *captureAASXFileServerService) PostAASXPackage(_ context.Context, file StagedUpload, aasIDs []string, fileName string) (ImplResponse, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return ImplResponse{}, err
	}
	service.fileName, service.aasIDs, service.content = fileName, aasIDs, content
	return Response(http.StatusCreated, PackageDescription{PackageId: "created"}), nil
}

func (service *captureAASXFileServerService) GetAASXByPackageId(context.Context, string) (ImplResponse, error) {
	return Response(http.StatusNotFound, nil), nil
}

func (service *captureAASXFileServerService) PutAASXByPackageId(context.Context, string, StagedUpload, []string, string) (ImplResponse, error) {
	return Response(http.StatusNoContent, nil), nil
}

func (service *captureAASXFileServerService) DeleteAASXByPackageId(context.Context, string) (ImplResponse, error) {
	return Response(http.StatusNoContent, nil), nil
}

func TestPostAASXPackageStreamsFileAndReadsTrailingMetadata(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir()+"/missing")
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "fallback.aasx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write([]byte("package-content")); err != nil {
		t.Fatal(err)
	}
	if err = writer.WriteField("aasIds", "one,two"); err != nil {
		t.Fatal(err)
	}
	if err = writer.WriteField("fileName", "selected.aasx"); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	service := &captureAASXFileServerService{}
	controller := NewAASXFileServerAPIAPIController(service, "", WithAASXFileServerUploadStager(controllerMemoryStager, 4096))
	request := httptest.NewRequest(http.MethodPost, "/packages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	controller.PostAASXPackage(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", response.Code, response.Body.String())
	}
	if service.fileName != "selected.aasx" || len(service.aasIDs) != 2 || string(service.content) != "package-content" {
		t.Fatalf("unexpected captured upload: filename=%q aasIDs=%v content=%q", service.fileName, service.aasIDs, service.content)
	}
}

func TestPostAASXPackageRejectsOversizedRequest(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "large.aasx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(bytes.Repeat([]byte("x"), 1024)); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	controller := NewAASXFileServerAPIAPIController(&captureAASXFileServerService{}, "", WithAASXFileServerUploadStager(controllerMemoryStager, 128))
	request := httptest.NewRequest(http.MethodPost, "/packages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	controller.PostAASXPackage(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413, got %d: %s", response.Code, response.Body.String())
	}
}

func TestPostAASXPackageReportsStagingFailureAsInternalError(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "package.aasx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write([]byte("package")); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	failedStager := func(context.Context, io.Reader, int64) (common.StagedUpload, error) {
		return nil, common.NewInternalServerError("AASXFILES-TESTSTAGE-DATABASE unavailable")
	}
	controller := NewAASXFileServerAPIAPIController(&captureAASXFileServerService{}, "", WithAASXFileServerUploadStager(failedStager, 4096))
	request := httptest.NewRequest(http.MethodPost, "/packages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	controller.PostAASXPackage(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", response.Code, response.Body.String())
	}
}

func TestEncodeJSONResponseStreamsAndClosesDownload(t *testing.T) {
	stream := &trackingReadCloser{Reader: bytes.NewReader(bytes.Repeat([]byte("chunk"), 20000))}
	response := httptest.NewRecorder()
	status := http.StatusOK
	if err := EncodeJSONResponse(FileDownload{
		Content: stream, ContentType: "application/aasx+json", Filename: "package.aasx",
	}, &status, response); err != nil {
		t.Fatalf("unexpected download error: %v", err)
	}
	if !stream.closed {
		t.Fatal("expected download stream to be closed")
	}
	if response.Body.Len() != len("chunk")*20000 {
		t.Fatalf("unexpected streamed byte count: %d", response.Body.Len())
	}
}

func TestEncodeJSONResponseClosesDownloadAfterCopyFailure(t *testing.T) {
	stream := &trackingReadCloser{Reader: bytes.NewReader([]byte("package"))}
	status := http.StatusOK
	err := EncodeJSONResponse(FileDownload{
		Content: stream, ContentType: "application/aasx+json", Filename: "package.aasx",
	}, &status, &failingResponseWriter{header: make(http.Header)})
	if err == nil {
		t.Fatal("expected response copy error")
	}
	if !stream.closed {
		t.Fatal("expected download stream to be closed after copy failure")
	}
}
