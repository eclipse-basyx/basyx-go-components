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
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncupload"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

type acceptingAsyncUploadService struct {
	fileName string
	content  []byte
}

func (s *acceptingAsyncUploadService) StartAsyncUpload(_ context.Context, fileName string, _ string, file common.StagedUpload) (commonmodel.ImplResponse, error) {
	s.fileName = fileName
	content := make([]byte, file.Size())
	_, err := file.Read(content)
	s.content = content
	return commonmodel.Response(http.StatusAccepted, UploadOperationHandle{HandleID: "AASENV-UPLOAD-handle"}), err
}

func (s *acceptingAsyncUploadService) GetAsyncUploadStatus(context.Context, string) (commonmodel.ImplResponse, error) {
	return commonmodel.Response(http.StatusFound, commonmodel.Redirect{Location: asyncUploadPath + "/result/handle"}), nil
}

func (s *acceptingAsyncUploadService) GetAsyncUploadResult(context.Context, string) (commonmodel.ImplResponse, error) {
	return commonmodel.Response(http.StatusOK, UploadOperationResult{ExecutionState: commonmodel.EXECUTIONSTATE_COMPLETED, Success: true}), nil
}

func asyncUploadRequest(t *testing.T, path string, fileName string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func asyncUploadRouter(service AsyncUploadService, admit ExecutionSlotAdmission) *chi.Mux {
	router := chi.NewRouter()
	api := chi.NewRouter()
	RegisterAsyncUploadAPI(api, service, 1024, memoryUploadStager, admit)
	router.Mount("/api", api)
	return router
}

func TestAsyncUploadAcceptsWithContextualizedLocation(t *testing.T) {
	manager := asyncjob.NewManager("TEST", time.Minute)
	service := &acceptingAsyncUploadService{}
	router := asyncUploadRouter(service, manager.TryAcquireExecutionSlotLease)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, asyncUploadRequest(t, "/api/upload-async", "../environment.json", []byte(`{}`)))

	require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	require.Equal(t, "http://example.com/api/upload-async/status/"+common.EncodeString("AASENV-UPLOAD-handle"), response.Header().Get("Location"))
	require.Equal(t, "environment.json", service.fileName)
	require.Equal(t, []byte(`{}`), service.content)
	var handle UploadOperationHandle
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &handle))
	require.Equal(t, "AASENV-UPLOAD-handle", handle.HandleID)

	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/upload-async/status/handle", nil))
	require.Equal(t, http.StatusFound, status.Code)
	require.Equal(t, "http://example.com/api/upload-async/result/handle", status.Header().Get("Location"))
}

func TestAsyncUploadRejectsExhaustedCapacityBeforeStaging(t *testing.T) {
	service := &acceptingAsyncUploadService{}
	router := asyncUploadRouter(service, func() (*asyncjob.ExecutionSlotLease, bool) { return nil, false })

	response := httptest.NewRecorder()
	router.ServeHTTP(response, asyncUploadRequest(t, "/api/upload-async", "environment.json", []byte(`{}`)))

	require.Equal(t, http.StatusTooManyRequests, response.Code, response.Body.String())
	require.Nil(t, service.content, "a request without capacity must not be staged")
}

func TestStartAsyncUploadValidatesBeforeAcceptance(t *testing.T) {
	configured := NewUploadAPIService(nil, nil, nil, WithAsyncUploads(asyncjob.NewManager("TEST", time.Minute), &asyncupload.Store{}))
	unconfigured := NewUploadAPIService(nil, nil, nil)
	plainText := &memoryStagedUpload{Reader: bytes.NewReader([]byte("plain text"))}
	environment := &memoryStagedUpload{Reader: bytes.NewReader([]byte(`{}`))}

	for _, test := range []struct {
		name    string
		service *UploadAPIService
		file    common.StagedUpload
		status  int
	}{
		{name: "missing file", service: configured, file: nil, status: http.StatusBadRequest},
		{name: "not configured", service: unconfigured, file: environment, status: http.StatusInternalServerError},
		{name: "unsupported format", service: configured, file: plainText, status: http.StatusUnsupportedMediaType},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := test.service.StartAsyncUpload(t.Context(), "environment.txt", "", test.file)
			require.NoError(t, err)
			require.Equal(t, test.status, response.Code)
		})
	}
}

func TestAsyncUploadHandlesRequireOwnerAndKind(t *testing.T) {
	manager := asyncjob.NewManager("TEST", time.Minute)
	service := NewUploadAPIService(nil, nil, nil, WithAsyncUploads(manager, &asyncupload.Store{}))
	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	foreign, err := manager.Start(ctx, "", asyncjob.StartOptions{JobKind: "other-kind"})
	require.NoError(t, err)
	running, err := manager.Start(ctx, "", asyncjob.StartOptions{JobKind: asyncUploadJobKind})
	require.NoError(t, err)

	response, err := service.GetAsyncUploadStatus(ctx, common.EncodeString(foreign))
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, response.Code)

	response, err = service.GetAsyncUploadStatus(ctx, common.EncodeString(running))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, commonmodel.EXECUTIONSTATE_RUNNING, response.Body.(UploadOperationResult).ExecutionState)

	response, err = service.GetAsyncUploadResult(ctx, common.EncodeString(running))
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, response.Code)

	failure := failedUploadOperationResult([]commonmodel.Message{{Text: "rejected"}})
	require.NoError(t, manager.Fail(ctx, running, http.StatusBadRequest, failure))
	response, err = service.GetAsyncUploadStatus(ctx, common.EncodeString(running))
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, response.Code)
	require.Equal(t, asyncUploadPath+"/result/"+common.EncodeString(running), response.Body.(commonmodel.Redirect).Location)

	response, err = service.GetAsyncUploadResult(ctx, common.EncodeString(running))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, failure, response.Body)

	response, err = service.GetAsyncUploadResult(ctx, "not*base64")
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestFailedAsyncUploadResultKeepsRejectionMessages(t *testing.T) {
	rejection := &rejectedUpload{response: common.NewErrorResponse(common.NewErrDenied("denied"), http.StatusForbidden, uploadComponent, uploadOperation, "Process")}
	status, result := failedAsyncUploadResult(t.Context(), "handle", rejection)
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, commonmodel.EXECUTIONSTATE_FAILED, result.ExecutionState)
	require.False(t, result.Success)
	require.Equal(t, "403 Denied: denied", result.Messages[0].Text)

	status, result = failedAsyncUploadResult(t.Context(), "handle", context.DeadlineExceeded)
	require.Equal(t, http.StatusInternalServerError, status)
	require.Equal(t, asyncUploadFailureText, result.Messages[0].Text)
}
