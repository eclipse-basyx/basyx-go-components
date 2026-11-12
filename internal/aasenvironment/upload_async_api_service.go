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
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncupload"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

const (
	asyncUploadJobKind          = "aasenvironment-upload"
	asyncUploadExecutionTime    = 15 * time.Minute
	asyncUploadOperation        = "UploadAsync"
	asyncUploadStatusOperation  = "GetUploadAsyncStatus"
	asyncUploadResultOperation  = "GetUploadAsyncResult"
	asyncUploadPath             = "/upload-async"
	asyncUploadFailureText      = "AASENV-UPLOADASYNC-FAILED asynchronous upload processing failed, nothing from the uploaded environment was stored"
	asyncUploadStateReadText    = "AASENV-UPLOADASYNC-READHANDLE asynchronous upload state could not be read"
	asyncUploadAcceptFailedText = "AASENV-UPLOADASYNC-ACCEPT asynchronous upload could not be accepted"
	asyncUploadJobManagerKey    = "AASENV-UPLOAD"
	asyncUploadPoolShare        = 5
)

// UploadOperationHandle identifies one accepted asynchronous upload.
type UploadOperationHandle struct {
	HandleID string `json:"handleId"`
}

// UploadOperationResult is the BaseOperationResult of an asynchronous upload.
type UploadOperationResult struct {
	Messages       []commonmodel.Message      `json:"messages,omitempty"`
	ExecutionState commonmodel.ExecutionState `json:"executionState"`
	Success        bool                       `json:"success"`
}

// rejectedUpload carries the error response of an upload whose import was
// rejected, so the job records the same messages the synchronous upload returns.
type rejectedUpload struct {
	response commonmodel.ImplResponse
}

func (rejection *rejectedUpload) Error() string {
	if text := commonmodel.ErrorText(rejection.response.Body); text != "" {
		return text
	}
	return http.StatusText(rejection.response.Code)
}

// NewAsyncUploads creates the PostgreSQL-backed job manager and durable staging
// store for asynchronous uploads. Each running upload holds one database
// connection for its import transaction, so at most a fifth of the pool
// executes uploads at the same time; an unlimited pool uses the manager default.
//
// Parameters:
//   - ctx: Service lifecycle context bounding job execution and maintenance.
//   - db: Shared writer pool that stores jobs, staging, and imports.
//
// Returns:
//   - UploadServiceOption: Option enabling asynchronous uploads on NewUploadAPIService.
//   - ExecutionSlotAdmission: Capacity reservation for RegisterAsyncUploadAPI.
//   - error: Manager or staging store initialization error.
func NewAsyncUploads(ctx context.Context, db *sql.DB) (UploadServiceOption, ExecutionSlotAdmission, error) {
	if db == nil {
		return nil, nil, common.NewInternalServerError("AASENV-UPLOADASYNC-NILDB database handle must not be nil")
	}
	manager, err := newAsyncUploadManager(ctx, db)
	if err != nil {
		return nil, nil, fmt.Errorf("AASENV-UPLOADASYNC-NEWMANAGER %w", err)
	}
	store, err := asyncupload.NewStore(db)
	if err != nil {
		return nil, nil, fmt.Errorf("AASENV-UPLOADASYNC-NEWSTORE %w", err)
	}
	return WithAsyncUploads(manager, store), manager.TryAcquireExecutionSlotLease, nil
}

func newAsyncUploadManager(ctx context.Context, db *sql.DB) (*asyncjob.Manager, error) {
	maximumOpenConnections := db.Stats().MaxOpenConnections
	if maximumOpenConnections <= 0 {
		return asyncjob.NewPostgresManager(ctx, db, asyncUploadJobManagerKey, asyncUploadExecutionTime)
	}
	capacity := max(1, maximumOpenConnections/asyncUploadPoolShare)
	return asyncjob.NewPostgresManagerWithExecutionCapacity(ctx, db, asyncUploadJobManagerKey, asyncUploadExecutionTime, capacity)
}

// StartAsyncUpload validates the upload format, durably accepts the staged file
// as an asynchronous job, and imports it in the background.
//
// Parameters:
//   - ctx: Request context carrying authorization, audit data, and the execution slot.
//   - fileName: Sanitized logical file name used for format detection.
//   - contentType: Declared content type of the file part.
//   - file: Request-scoped staged upload; ownership moves to the job on acceptance.
//
// Returns:
//   - commonmodel.ImplResponse: 202 with the operation handle, or an error response.
//   - error: Always nil; failures are expressed as responses.
func (s *UploadAPIService) StartAsyncUpload(ctx context.Context, fileName string, contentType string, file common.StagedUpload) (commonmodel.ImplResponse, error) {
	if file == nil {
		return newAsyncUploadErrorResponse(common.NewErrBadRequest("AASENV-UPLOADASYNC-MISSINGFILE uploaded file is required"), http.StatusBadRequest, asyncUploadOperation, "MissingFile"), nil
	}
	if s.asyncJobs == nil || s.asyncUploads == nil {
		return newAsyncUploadErrorResponse(common.NewInternalServerError("AASENV-UPLOADASYNC-NOTCONFIGURED asynchronous uploads are not configured"), http.StatusInternalServerError, asyncUploadOperation, "NotConfigured"), nil
	}
	if response, rejected := rejectUnsupportedUploadFormat(fileName, contentType, file); rejected {
		return response, nil
	}
	releaseExecutionSlot, acquired := s.asyncJobs.AcquireExecutionSlot(ctx)
	if !acquired {
		return newAsyncUploadErrorResponse(errors.New("AASENV-UPLOADASYNC-CAPACITY asynchronous execution capacity is exhausted"), http.StatusTooManyRequests, asyncUploadOperation, "ExecutionCapacityExhausted"), nil
	}

	executionCtx, cancelExecution := s.asyncJobs.NewExecutionContext(ctx, asyncUploadExecutionTime)
	handleID, durableUpload, err := s.asyncUploads.Accept(
		executionCtx,
		s.asyncJobs,
		file,
		auth.OwnerKeyFromContext(ctx),
		asyncjob.StartOptions{JobKind: asyncUploadJobKind, ExecutionDeadline: time.Now().UTC().Add(asyncUploadExecutionTime)},
		nil,
	)
	if err != nil {
		releaseExecutionSlot()
		cancelExecution()
		return mapAsyncUploadAcceptanceError(ctx, err), nil
	}

	job := asyncUploadJob{handleID: handleID, upload: durableUpload, fileName: fileName, contentType: contentType}
	go s.runAsyncUpload(executionCtx, job, cancelExecution, releaseExecutionSlot)
	return commonmodel.Response(http.StatusAccepted, UploadOperationHandle{HandleID: handleID}), nil
}

func rejectUnsupportedUploadFormat(fileName string, contentType string, file io.ReadSeeker) (commonmodel.ImplResponse, bool) {
	signature, err := readUploadSignature(file)
	if err != nil {
		return newAsyncUploadErrorResponse(common.NewInternalServerError("AASENV-UPLOADASYNC-READSIGNATURE "+err.Error()), http.StatusInternalServerError, asyncUploadOperation, "ReadSignature"), true
	}
	if _, _, err = detectUploadFormat(fileName, contentType, signature); err != nil {
		return newAsyncUploadErrorResponse(err, http.StatusUnsupportedMediaType, asyncUploadOperation, "UnsupportedFormat"), true
	}
	return commonmodel.ImplResponse{}, false
}

func mapAsyncUploadAcceptanceError(ctx context.Context, err error) commonmodel.ImplResponse {
	switch {
	case common.IsErrPayloadTooLarge(err):
		return newAsyncUploadErrorResponse(err, http.StatusRequestEntityTooLarge, asyncUploadOperation, "PayloadTooLarge")
	case common.IsErrBadRequest(err):
		return newAsyncUploadErrorResponse(err, http.StatusBadRequest, asyncUploadOperation, "BadRequest")
	default:
		slog.ErrorContext(ctx, "asynchronous upload acceptance failed", "error.code", "AASENV-UPLOADASYNC-ACCEPT", "error", err)
		return newAsyncUploadErrorResponse(common.NewInternalServerError(asyncUploadAcceptFailedText), http.StatusInternalServerError, asyncUploadOperation, "DurableAcceptance")
	}
}

type asyncUploadJob struct {
	handleID    string
	upload      *asyncupload.DurableUpload
	fileName    string
	contentType string
}

func (s *UploadAPIService) runAsyncUpload(ctx context.Context, job asyncUploadJob, cancel context.CancelFunc, releaseExecutionSlot func()) {
	defer releaseExecutionSlot()
	defer cancel()
	stopHeartbeat := s.asyncJobs.KeepAlive(ctx, job.handleID)
	defer stopHeartbeat()
	defer func() {
		if recover() == nil {
			return
		}
		slog.ErrorContext(ctx, "asynchronous upload worker panicked", "error.code", "AASENV-UPLOADASYNC-PANIC", "async_job.handle_id", job.handleID)
		s.failAsyncUpload(ctx, job, errors.New("AASENV-UPLOADASYNC-PANIC asynchronous upload worker panicked"))
	}()

	err := job.upload.Consume(ctx, func(ctx context.Context, tx *sql.Tx, file io.ReadSeeker) (any, error) {
		return s.importAsyncUpload(ctx, tx, job, file)
	})
	if err != nil {
		s.failAsyncUpload(ctx, job, err)
	}
}

func (s *UploadAPIService) importAsyncUpload(ctx context.Context, tx *sql.Tx, job asyncUploadJob, file io.ReadSeeker) (any, error) {
	response, err := s.boundTo(tx).HandleUpload(ctx, job.fileName, job.contentType, file)
	if err != nil {
		return nil, err
	}
	if response.Code < http.StatusOK || response.Code >= http.StatusMultipleChoices {
		return nil, &rejectedUpload{response: response}
	}
	return UploadOperationResult{
		ExecutionState: commonmodel.EXECUTIONSTATE_COMPLETED,
		Success:        true,
		Messages:       []commonmodel.Message{completedUploadMessage(job.fileName)},
	}, nil
}

func completedUploadMessage(fileName string) commonmodel.Message {
	text := "AASENV-UPLOADASYNC-COMPLETED the uploaded environment was stored completely"
	if fileName != "" {
		text += " from " + fileName
	}
	return commonmodel.Message{
		MessageType: "Info",
		Text:        text,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *UploadAPIService) failAsyncUpload(ctx context.Context, job asyncUploadJob, cause error) {
	if closeErr := job.upload.Close(); closeErr != nil {
		slog.ErrorContext(ctx, "asynchronous upload staging cleanup failed", "error.code", "AASENV-UPLOADASYNC-CLEANUP", "error", closeErr, "async_job.handle_id", job.handleID)
	}
	status, result := failedAsyncUploadResult(ctx, job.handleID, cause)
	persistenceCtx, cancelPersistence := asyncjob.NewPersistenceContext(ctx)
	defer cancelPersistence()
	if err := s.asyncJobs.Fail(persistenceCtx, job.handleID, status, result); err != nil {
		slog.ErrorContext(persistenceCtx, "asynchronous upload failure could not be recorded", "error.code", "AASENV-UPLOADASYNC-PERSISTFAILURE", "error", err, "async_job.handle_id", job.handleID)
	}
}

func failedAsyncUploadResult(ctx context.Context, handleID string, cause error) (int, UploadOperationResult) {
	var rejection *rejectedUpload
	if errors.As(cause, &rejection) {
		if result, ok := rejection.response.Body.(commonmodel.Result); ok && len(result.Messages) > 0 {
			return rejection.response.Code, failedUploadOperationResult(result.Messages)
		}
	}
	slog.ErrorContext(ctx, "asynchronous upload processing failed", "error.code", "AASENV-UPLOADASYNC-PROCESS", "error", cause, "async_job.handle_id", handleID)
	message := commonmodel.NewErrorMessage(
		asyncUploadFailureText,
		strconv.Itoa(http.StatusInternalServerError),
		commonmodel.NewErrorCorrelationID(uploadComponent, http.StatusInternalServerError, asyncUploadOperation, "Process"),
	)
	return http.StatusInternalServerError, failedUploadOperationResult([]commonmodel.Message{message})
}

func failedUploadOperationResult(messages []commonmodel.Message) UploadOperationResult {
	return UploadOperationResult{ExecutionState: commonmodel.EXECUTIONSTATE_FAILED, Success: false, Messages: messages}
}

// GetAsyncUploadStatus reports a running upload or redirects to the result of
// a finished one.
//
// Parameters:
//   - ctx: Request context identifying the handle owner.
//   - encodedHandleID: Base64URL-encoded operation handle.
//
// Returns:
//   - commonmodel.ImplResponse: 200 while running, 302 to the result afterwards, or an error response.
//   - error: Always nil; failures are expressed as responses.
func (s *UploadAPIService) GetAsyncUploadStatus(ctx context.Context, encodedHandleID string) (commonmodel.ImplResponse, error) {
	handleID, record, response, ok := s.asyncUploadRecord(ctx, encodedHandleID, asyncUploadStatusOperation)
	if !ok {
		return response, nil
	}
	if record.ExecutionState == string(commonmodel.EXECUTIONSTATE_RUNNING) {
		return commonmodel.Response(http.StatusOK, UploadOperationResult{ExecutionState: commonmodel.EXECUTIONSTATE_RUNNING, Success: true}), nil
	}
	return commonmodel.Response(http.StatusFound, commonmodel.Redirect{
		Location: asyncUploadPath + "/result/" + url.PathEscape(common.EncodeString(handleID)),
	}), nil
}

// GetAsyncUploadResult returns the retained result of a finished upload.
//
// Parameters:
//   - ctx: Request context identifying the handle owner.
//   - encodedHandleID: Base64URL-encoded operation handle.
//
// Returns:
//   - commonmodel.ImplResponse: 200 with the operation result, or an error response.
//   - error: Always nil; failures are expressed as responses.
func (s *UploadAPIService) GetAsyncUploadResult(ctx context.Context, encodedHandleID string) (commonmodel.ImplResponse, error) {
	_, record, response, ok := s.asyncUploadRecord(ctx, encodedHandleID, asyncUploadResultOperation)
	if !ok {
		return response, nil
	}
	switch record.ExecutionState {
	case string(commonmodel.EXECUTIONSTATE_RUNNING):
		return newAsyncUploadErrorResponse(common.NewErrBadRequest("AASENV-UPLOADASYNC-RUNNING operation is still running"), http.StatusBadRequest, asyncUploadResultOperation, "OperationStillRunning"), nil
	case string(commonmodel.EXECUTIONSTATE_COMPLETED):
		if record.Payload != nil {
			return commonmodel.Response(http.StatusOK, record.Payload), nil
		}
	case string(commonmodel.EXECUTIONSTATE_FAILED):
		if record.ErrorBody != nil {
			return commonmodel.Response(http.StatusOK, record.ErrorBody), nil
		}
	}
	return commonmodel.Response(http.StatusOK, UploadOperationResult{
		ExecutionState: commonmodel.ExecutionState(record.ExecutionState),
		Success:        record.ExecutionState == string(commonmodel.EXECUTIONSTATE_COMPLETED),
	}), nil
}

func (s *UploadAPIService) asyncUploadRecord(ctx context.Context, encodedHandleID string, operation string) (string, asyncjob.Record, commonmodel.ImplResponse, bool) {
	handleID, err := common.DecodeString(encodedHandleID)
	if err != nil {
		return "", asyncjob.Record{}, newAsyncUploadErrorResponse(common.NewErrBadRequest("AASENV-UPLOADASYNC-MALFORMEDHANDLE "+err.Error()), http.StatusBadRequest, operation, "MalformedHandleId"), false
	}
	if s.asyncJobs == nil {
		return "", asyncjob.Record{}, newAsyncUploadErrorResponse(common.NewInternalServerError("AASENV-UPLOADASYNC-NOTCONFIGURED asynchronous uploads are not configured"), http.StatusInternalServerError, operation, "NotConfigured"), false
	}
	record, found, err := s.asyncJobs.GetForOwner(ctx, handleID, auth.OwnerKeyFromContext(ctx))
	if err != nil {
		slog.ErrorContext(ctx, "asynchronous upload state could not be read", "error.code", "AASENV-UPLOADASYNC-READHANDLE", "error", err)
		return "", asyncjob.Record{}, newAsyncUploadErrorResponse(common.NewInternalServerError(asyncUploadStateReadText), http.StatusInternalServerError, operation, "ReadHandle"), false
	}
	if !found || record.JobKind != asyncUploadJobKind {
		return "", asyncjob.Record{}, newAsyncUploadErrorResponse(common.NewErrNotFound("AASENV-UPLOADASYNC-HANDLENOTFOUND operation handle"), http.StatusNotFound, operation, "HandleNotFound"), false
	}
	return handleID, record, commonmodel.ImplResponse{}, true
}

func newAsyncUploadErrorResponse(err error, status int, operation string, info string) commonmodel.ImplResponse {
	return common.NewErrorResponse(err, status, uploadComponent, operation, info)
}
