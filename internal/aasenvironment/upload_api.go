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
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"

	"github.com/go-chi/chi/v5"
)

const defaultUploadMaxSizeBytes int64 = 128 << 20

// RegisterUploadAPI registers the bounded multipart upload endpoint.
//
// Parameters:
//   - r: Router receiving POST /upload.
//   - service: Upload business logic invoked with seekable staged content.
//   - maxUploadSizeBytes: Maximum complete multipart request size.
//   - stager: External storage provider used instead of process memory or local files.
func RegisterUploadAPI(r chi.Router, service UploadService, maxUploadSizeBytes int64, stager common.UploadStager) {
	if maxUploadSizeBytes <= 0 {
		maxUploadSizeBytes = defaultUploadMaxSizeBytes
	}

	api := &uploadAPI{service: service, maxUploadSizeBytes: maxUploadSizeBytes, stager: stager}
	r.Post("/upload", api.HandleUpload)
}

// UploadService defines upload business logic without HTTP dependencies.
type UploadService interface {
	HandleUpload(ctx context.Context, fileName string, contentType string, file io.ReadSeeker) (commonmodel.ImplResponse, error)
}

type uploadAPI struct {
	service            UploadService
	maxUploadSizeBytes int64
	stager             common.UploadStager
}

func (a *uploadAPI) HandleUpload(w http.ResponseWriter, r *http.Request) {
	upload, err := common.ReadMultipartUpload(w, r, a.maxUploadSizeBytes, "file", a.stager)
	if err != nil {
		status := http.StatusBadRequest
		if common.IsErrPayloadTooLarge(err) {
			status = http.StatusRequestEntityTooLarge
		}
		writeUploadError(w, status, err, "AASENV-UPLOAD-PARSEMULTIPART")
		return
	}
	defer func() { _ = upload.Close() }()

	contentType := upload.FileContentType

	fileName := strings.TrimSpace(upload.FirstField("fileName"))
	if fileName == "" {
		fileName = upload.MultipartFileName
	}
	fileName = sanitizeUploadFileName(fileName)

	result, err := a.service.HandleUpload(r.Context(), fileName, contentType, upload.File)
	if err != nil {
		writeUploadError(w, http.StatusInternalServerError, err, "AASENV-UPLOAD-HANDLER")
		return
	}

	if encErr := commonmodel.EncodeJSONResponse(result.Body, &result.Code, w); encErr != nil {
		writeUploadError(w, http.StatusInternalServerError, encErr, "AASENV-UPLOAD-ENCODERESPONSE")
	}
}

func writeUploadError(w http.ResponseWriter, status int, err error, info string) {
	resp := common.NewErrorResponse(err, status, "AASENV", "UploadAPI", info)
	if encErr := commonmodel.EncodeJSONResponse(resp.Body, &resp.Code, w); encErr != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func sanitizeUploadFileName(fileName string) string {
	baseName := strings.TrimSpace(fileName)
	if baseName == "" {
		return ""
	}

	baseName = strings.ReplaceAll(baseName, "\\", "/")
	if lastSlash := strings.LastIndex(baseName, "/"); lastSlash >= 0 {
		baseName = baseName[lastSlash+1:]
	}

	baseName = strings.TrimSpace(baseName)
	if baseName == "" || baseName == "." || baseName == "/" {
		return ""
	}

	return baseName
}
