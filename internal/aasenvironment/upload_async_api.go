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
	"errors"
	"net/http"
	"net/url"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

// AsyncUploadService defines asynchronous upload business logic without HTTP dependencies.
type AsyncUploadService interface {
	StartAsyncUpload(ctx context.Context, fileName string, contentType string, file common.StagedUpload) (commonmodel.ImplResponse, error)
	GetAsyncUploadStatus(ctx context.Context, encodedHandleID string) (commonmodel.ImplResponse, error)
	GetAsyncUploadResult(ctx context.Context, encodedHandleID string) (commonmodel.ImplResponse, error)
}

// ExecutionSlotAdmission reserves asynchronous execution capacity before a
// request body is staged.
type ExecutionSlotAdmission func() (*asyncjob.ExecutionSlotLease, bool)

type asyncUploadAPI struct {
	service            AsyncUploadService
	maxUploadSizeBytes int64
	stager             common.UploadStager
	admit              ExecutionSlotAdmission
}

// RegisterAsyncUploadAPI registers the asynchronous upload endpoints:
// POST /upload-async, GET /upload-async/status/{handleId} and
// GET /upload-async/result/{handleId}.
//
// Parameters:
//   - r: Router receiving the endpoints.
//   - service: Asynchronous upload business logic.
//   - maxUploadSizeBytes: Maximum complete multipart request size.
//   - stager: External storage provider for the request body.
//   - admit: Execution capacity reservation taken before the body is staged.
func RegisterAsyncUploadAPI(r chi.Router, service AsyncUploadService, maxUploadSizeBytes int64, stager common.UploadStager, admit ExecutionSlotAdmission) {
	if maxUploadSizeBytes <= 0 {
		maxUploadSizeBytes = defaultUploadMaxSizeBytes
	}
	api := &asyncUploadAPI{service: service, maxUploadSizeBytes: maxUploadSizeBytes, stager: stager, admit: admit}
	r.Post(asyncUploadPath, api.startUpload)
	r.Get(asyncUploadPath+"/status/{handleId}", api.getStatus)
	r.Get(asyncUploadPath+"/result/{handleId}", api.getResult)
}

func (a *asyncUploadAPI) startUpload(w http.ResponseWriter, r *http.Request) {
	if a.admit == nil {
		writeUploadError(w, http.StatusInternalServerError, common.NewInternalServerError("AASENV-UPLOADASYNC-NOADMISSION execution slot admission is not configured"), "AASENV-UPLOADASYNC-ADMISSION")
		return
	}
	executionSlot, acquired := a.admit()
	if !acquired || executionSlot == nil {
		writeUploadError(w, http.StatusTooManyRequests, errors.New("AASENV-UPLOADASYNC-CAPACITY asynchronous execution capacity is exhausted"), "AASENV-UPLOADASYNC-CAPACITY")
		return
	}
	defer executionSlot.ReleaseIfUnclaimed()
	r = r.WithContext(asyncjob.WithExecutionSlotLease(r.Context(), executionSlot))

	upload, err := common.ReadMultipartUpload(w, r, a.maxUploadSizeBytes, "file", a.stager)
	if err != nil {
		writeUploadError(w, uploadErrorStatus(err), err, "AASENV-UPLOADASYNC-PARSEMULTIPART")
		return
	}
	defer func() { _ = upload.Close() }()

	result, err := a.service.StartAsyncUpload(r.Context(), uploadFileName(upload), upload.FileContentType, upload.File)
	if err != nil {
		writeUploadError(w, http.StatusInternalServerError, err, "AASENV-UPLOADASYNC-HANDLER")
		return
	}
	if handle, ok := result.Body.(UploadOperationHandle); ok && result.Code == http.StatusAccepted {
		location := asyncUploadPath + "/status/" + url.PathEscape(common.EncodeString(handle.HandleID))
		w.Header().Set("Location", common.ContextualizeAPIResourceLocation(r, location, asyncUploadPath))
	}
	writeAsyncUploadResponse(w, r, result)
}

func (a *asyncUploadAPI) getStatus(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.GetAsyncUploadStatus(r.Context(), chi.URLParam(r, "handleId"))
	if err != nil {
		writeUploadError(w, http.StatusInternalServerError, err, "AASENV-UPLOADASYNC-STATUS")
		return
	}
	writeAsyncUploadResponse(w, r, result)
}

func (a *asyncUploadAPI) getResult(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.GetAsyncUploadResult(r.Context(), chi.URLParam(r, "handleId"))
	if err != nil {
		writeUploadError(w, http.StatusInternalServerError, err, "AASENV-UPLOADASYNC-RESULT")
		return
	}
	writeAsyncUploadResponse(w, r, result)
}

func writeAsyncUploadResponse(w http.ResponseWriter, r *http.Request, result commonmodel.ImplResponse) {
	if redirect, ok := result.Body.(commonmodel.Redirect); ok {
		redirect.Location = common.ContextualizeAPIResourceLocation(r, redirect.Location, asyncUploadPath)
		result.Body = redirect
	}
	if err := commonmodel.EncodeJSONResponse(result.Body, &result.Code, w); err != nil {
		writeUploadError(w, http.StatusInternalServerError, err, "AASENV-UPLOADASYNC-ENCODERESPONSE")
	}
}
