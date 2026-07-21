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

package common

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

type multipartMemoryStage struct {
	*bytes.Reader
}

func (stage *multipartMemoryStage) Size() int64  { return stage.Reader.Size() }
func (stage *multipartMemoryStage) Close() error { return nil }
func (stage *multipartMemoryStage) Promote(context.Context, func(context.Context, *sql.Tx, int64, int64) error) error {
	return nil
}

func multipartMemoryStager(_ context.Context, input io.Reader, maximum int64) (StagedUpload, error) {
	content, err := io.ReadAll(io.LimitReader(input, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maximum {
		return nil, NewErrPayloadTooLarge("COMMON-MULTIPARTTEST-TOOLARGE payload exceeds limit")
	}
	return &multipartMemoryStage{Reader: bytes.NewReader(content)}, nil
}

func TestReadMultipartUploadAcceptsMetadataAfterFile(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "fallback.aasx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write([]byte("package")); err != nil {
		t.Fatal(err)
	}
	if err = writer.WriteField("fileName", "selected.aasx"); err != nil {
		t.Fatal(err)
	}
	if err = writer.WriteField("aasIds", "one,two"); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/packages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	upload, err := ReadMultipartUpload(httptest.NewRecorder(), request, 4096, "file", multipartMemoryStager)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	defer func() { _ = upload.Close() }()
	if upload.FirstField("fileName") != "selected.aasx" || upload.MultipartFileName != "fallback.aasx" {
		t.Fatalf("unexpected upload metadata: %+v", upload)
	}
	content, err := io.ReadAll(upload.File)
	if err != nil || string(content) != "package" {
		t.Fatalf("unexpected staged content %q: %v", string(content), err)
	}
}

func TestReadMultipartUploadAcceptsMetadataBeforeFile(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("fileName", "selected.aasx"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("aasIds", "one,two"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("file", "fallback.aasx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write([]byte("package")); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/packages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	upload, err := ReadMultipartUpload(httptest.NewRecorder(), request, 4096, "file", multipartMemoryStager)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	defer func() { _ = upload.Close() }()
	if upload.FirstField("fileName") != "selected.aasx" || upload.FirstField("aasIds") != "one,two" {
		t.Fatalf("unexpected upload metadata: %+v", upload)
	}
}

func TestReadMultipartUploadRejectsMissingFile(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("fileName", "selected.aasx"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/packages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if _, err := ReadMultipartUpload(httptest.NewRecorder(), request, 4096, "file", multipartMemoryStager); !IsErrBadRequest(err) {
		t.Fatalf("expected bad request for missing file, got %v", err)
	}
}

func TestReadMultipartUploadRejectsDuplicateFiles(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, name := range []string{"first.aasx", "second.aasx"} {
		part, err := writer.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write([]byte(name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/packages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if _, err := ReadMultipartUpload(httptest.NewRecorder(), request, 4096, "file", multipartMemoryStager); !IsErrBadRequest(err) {
		t.Fatalf("expected bad request for duplicate files, got %v", err)
	}
}

func TestReadMultipartUploadRejectsOversizedRequest(t *testing.T) {
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
	request := httptest.NewRequest(http.MethodPost, "/packages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if _, err = ReadMultipartUpload(httptest.NewRecorder(), request, 128, "file", multipartMemoryStager); !IsErrPayloadTooLarge(err) {
		t.Fatalf("expected payload-too-large error, got %v", err)
	}
}
