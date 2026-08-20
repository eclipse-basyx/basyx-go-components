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

// Package api implements HTTP service behavior for the AASX file server.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/aasxfileserver/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasxfileserverapi/go"
)

const (
	componentName             = "AASXFS"
	asyncPackageJobKind       = "aasx-package-upload"
	asyncPackageExecutionTime = 15 * time.Minute
)

// AASXFileServerAPIAPIService implements the generated AASX file server API service interface.
type AASXFileServerAPIAPIService struct {
	backend        *persistence.AASXFileServerDatabase
	packageCreator aasxPackageCreator
	asyncJobs      *asyncjob.Manager
	asyncUploads   *persistence.AsyncUploadStore
}

// AASXFileServerServiceOption configures optional service capabilities.
type AASXFileServerServiceOption func(*AASXFileServerAPIAPIService)

// WithAsyncPackageUploads enables PostgreSQL-backed SSP-002 processing.
func WithAsyncPackageUploads(manager *asyncjob.Manager, uploads *persistence.AsyncUploadStore) AASXFileServerServiceOption {
	return func(service *AASXFileServerAPIAPIService) {
		service.asyncJobs = manager
		service.asyncUploads = uploads
	}
}

type aasxPackageCreator interface {
	CreatePackage(context.Context, string, common.StagedUpload, []string, string) (*persistence.PackageRecord, error)
}

// NewAASXFileServerAPIAPIService constructs an AASX file server service.
//
// Parameters:
//   - backend: PostgreSQL persistence backend used by all service operations.
//
// Returns:
//   - *AASXFileServerAPIAPIService: Configured service instance.
func NewAASXFileServerAPIAPIService(backend *persistence.AASXFileServerDatabase, options ...AASXFileServerServiceOption) *AASXFileServerAPIAPIService {
	service := &AASXFileServerAPIAPIService{backend: backend, packageCreator: backend}
	for _, option := range options {
		option(service)
	}
	return service
}

// GetAllAASXPackageIds lists available package descriptors with optional AAS filter and paging.
func (s *AASXFileServerAPIAPIService) GetAllAASXPackageIds(ctx context.Context, aasID string, limit int32, cursor string) (openapi.ImplResponse, error) {
	const operation = "GetAllAASXPackageIds"

	decodedCursor := ""
	if strings.TrimSpace(cursor) != "" {
		var decodeErr error
		decodedCursor, decodeErr = common.DecodeString(cursor)
		if decodeErr != nil {
			return newAPIErrorResponse(decodeErr, http.StatusBadRequest, operation, "BadCursor"), nil
		}
	}

	cursorID, parseErr := persistence.ParseCursorID(decodedCursor)
	if parseErr != nil {
		return newAPIErrorResponse(parseErr, http.StatusBadRequest, operation, "BadCursorValue"), nil
	}

	decodedAASID := ""
	if strings.TrimSpace(aasID) != "" {
		var decodeErr error
		decodedAASID, decodeErr = common.DecodeString(aasID)
		if decodeErr != nil {
			return newAPIErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedAasId"), nil
		}
	}

	records, nextCursorID, err := s.backend.ListPackages(ctx, limit, cursorID, decodedAASID)
	if err != nil {
		if common.IsErrBadRequest(err) {
			return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "ListPackages"), nil
	}

	result := make([]openapi.PackageDescription, 0, len(records))
	for _, record := range records {
		result = append(result, toPackageDescription(record))
	}

	nextCursor := ""
	if nextCursorID > 0 {
		nextCursor = common.EncodeString(strconv.FormatInt(nextCursorID, 10))
	}

	return openapi.Response(http.StatusOK, openapi.GetPackageDescriptionsResult{
		PagingMetadata: openapi.PagedResultPagingMetadata{Cursor: nextCursor},
		Result:         result,
	}), nil
}

// PostAASXPackage stores a staged package under a server-generated identifier.
//
// Parameters:
//   - ctx: Request context containing cancellation and configured AASX limits.
//   - file: Seekable staged package owned by the HTTP request.
//   - aasIDs: AAS identifiers associated with the package.
//   - fileName: Preferred download filename.
//
// Returns:
//   - openapi.ImplResponse: Created package description or mapped API error.
//   - error: Reserved for failures not represented by an API response.
func (s *AASXFileServerAPIAPIService) PostAASXPackage(ctx context.Context, file openapi.StagedUpload, aasIDs []string, fileName string) (openapi.ImplResponse, error) {
	const operation = "PostAASXPackage"

	if file == nil {
		return newAPIErrorResponse(errors.New("multipart form field 'file' is required"), http.StatusBadRequest, operation, "MissingFile"), nil
	}

	rawPackageID := generatePackageID()
	record, err := s.packageCreator.CreatePackage(ctx, rawPackageID, file, aasIDs, fileName)
	if err != nil {
		if common.IsErrPayloadTooLarge(err) {
			return newAPIErrorResponse(err, http.StatusRequestEntityTooLarge, operation, "PayloadTooLarge"), nil
		}
		if common.IsErrConflict(err) {
			return newAPIErrorResponse(err, http.StatusConflict, operation, "PackageIdConflict"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "CreatePackage"), nil
	}

	return openapi.Response(http.StatusCreated, toPackageDescription(*record)), nil
}

// PostAsyncAASXPackage durably accepts a staged package and processes it independently.
func (s *AASXFileServerAPIAPIService) PostAsyncAASXPackage(ctx context.Context, file openapi.StagedUpload, aasIDs []string, fileName string) (openapi.ImplResponse, error) {
	const operation = "PostAsyncAASXPackage"
	if file == nil {
		return newAPIErrorResponse(errors.New("multipart form field 'file' is required"), http.StatusBadRequest, operation, "MissingFile"), nil
	}

	if s.asyncJobs == nil || s.asyncUploads == nil {
		return newAPIErrorResponse(errors.New("asynchronous package persistence is not configured"), http.StatusInternalServerError, operation, "NotConfigured"), nil
	}
	releaseExecutionSlot, acquired := acquireAsyncExecutionSlot(ctx, s.asyncJobs)
	if !acquired {
		capacityErr := errors.New("AASXFS-ASYNC-CAPACITY asynchronous execution capacity is exhausted")
		return newAPIErrorResponse(capacityErr, http.StatusTooManyRequests, operation, "ExecutionCapacityExhausted"), nil
	}

	executionCtx, cancelExecution := s.asyncJobs.NewExecutionContext(ctx, asyncPackageExecutionTime)
	successResult := openapi.BaseOperationResult{
		ExecutionState: openapi.EXECUTIONSTATE_COMPLETED,
		Success:        true,
	}
	handleID, durableUpload, err := s.asyncUploads.Accept(
		executionCtx,
		s.asyncJobs,
		file,
		auth.OwnerKeyFromContext(ctx),
		asyncjob.StartOptions{JobKind: asyncPackageJobKind, ExecutionDeadline: time.Now().UTC().Add(asyncPackageExecutionTime)},
		successResult,
	)
	if err != nil {
		releaseExecutionSlot()
		cancelExecution()
		return mapAsyncAcceptanceError(err, operation), nil
	}

	go s.processAsyncPackage(executionCtx, s.asyncJobs, handleID, durableUpload, aasIDs, fileName, cancelExecution, releaseExecutionSlot)
	return openapi.Response(http.StatusAccepted, openapi.OperationHandle{HandleId: handleID}), nil
}

func acquireAsyncExecutionSlot(ctx context.Context, manager *asyncjob.Manager) (func(), bool) {
	if executionSlot, found := asyncjob.ExecutionSlotLeaseFromContext(ctx); found {
		return executionSlot.Claim()
	}
	return manager.TryAcquireExecutionSlot()
}

// GetAasxAsyncStatus returns a running result or redirects terminal operations.
func (s *AASXFileServerAPIAPIService) GetAasxAsyncStatus(ctx context.Context, handleID string) (openapi.ImplResponse, error) {
	const operation = "GetAasxAsyncStatus"
	record, response, ok := s.asyncRecord(ctx, handleID, operation)
	if !ok {
		return response, nil
	}
	if record.ExecutionState == "Running" {
		return openapi.Response(http.StatusOK, openapi.BaseOperationResult{
			ExecutionState: openapi.EXECUTIONSTATE_RUNNING,
			Success:        true,
		}), nil
	}
	return openapi.Response(http.StatusFound, openapi.Redirect{
		Location: "/packages-async/result/" + url.PathEscape(handleID),
	}), nil
}

// GetAasxAsyncResult returns the retained terminal operation result.
func (s *AASXFileServerAPIAPIService) GetAasxAsyncResult(ctx context.Context, handleID string) (openapi.ImplResponse, error) {
	const operation = "GetAasxAsyncResult"
	record, response, ok := s.asyncRecord(ctx, handleID, operation)
	if !ok {
		return response, nil
	}
	if record.ExecutionState == "Running" {
		err := errors.New("operation is still running")
		return newAPIErrorResponse(err, http.StatusBadRequest, operation, "OperationStillRunning"), nil
	}
	if record.ExecutionState == "Completed" && record.Payload != nil {
		return openapi.Response(http.StatusOK, record.Payload), nil
	}
	if record.ExecutionState == "Failed" {
		if payload, found := record.ErrorBody.(map[string]any); found {
			if _, hasState := payload["executionState"]; hasState {
				return openapi.Response(http.StatusOK, payload), nil
			}
		}
		return openapi.Response(http.StatusOK, failedOperationResult(fmt.Sprint(record.ErrorBody))), nil
	}
	return openapi.Response(http.StatusOK, openapi.BaseOperationResult{
		ExecutionState: openapi.ExecutionState(record.ExecutionState),
		Success:        record.ExecutionState == "Completed",
	}), nil
}

func (s *AASXFileServerAPIAPIService) asyncRecord(ctx context.Context, handleID string, operation string) (asyncjob.Record, openapi.ImplResponse, bool) {
	if s.asyncJobs == nil {
		err := errors.New("asynchronous package persistence is not configured")
		return asyncjob.Record{}, newAPIErrorResponse(err, http.StatusInternalServerError, operation, "NotConfigured"), false
	}
	record, found, err := s.asyncJobs.GetForOwner(ctx, handleID, auth.OwnerKeyFromContext(ctx))
	if err != nil {
		return asyncjob.Record{}, newAPIErrorResponse(err, http.StatusInternalServerError, operation, "ReadHandle"), false
	}
	if !found || record.JobKind != asyncPackageJobKind {
		err = common.NewErrNotFound("operation handle")
		return asyncjob.Record{}, newAPIErrorResponse(err, http.StatusNotFound, operation, "HandleNotFound"), false
	}
	return record, openapi.ImplResponse{}, true
}

func (s *AASXFileServerAPIAPIService) processAsyncPackage(
	ctx context.Context,
	manager *asyncjob.Manager,
	handleID string,
	file common.StagedUpload,
	aasIDs []string,
	fileName string,
	cancel context.CancelFunc,
	releaseExecutionSlot func(),
) {
	defer releaseExecutionSlot()
	defer cancel()
	defer func() { _ = file.Close() }()
	stopHeartbeat := manager.KeepAlive(ctx, handleID)
	defer stopHeartbeat()

	_, err := s.packageCreator.CreatePackage(ctx, generatePackageID(), file, aasIDs, fileName)
	if err == nil {
		return
	}
	if closeErr := file.Close(); closeErr != nil {
		slog.ErrorContext(ctx, "asynchronous AASX staging cleanup failed", "error.code", "AASXFS-ASYNCWORKER-CLEANUP", "error", closeErr, "async_job.handle_id", handleID)
	}
	persistenceCtx, cancelPersistence := asyncjob.NewPersistenceContext(ctx)
	defer cancelPersistence()
	result := failedOperationResult(err.Error())
	if persistErr := manager.Fail(persistenceCtx, handleID, http.StatusInternalServerError, result); persistErr != nil {
		slog.ErrorContext(persistenceCtx, "asynchronous AASX failure persistence failed", "error.code", "AASXFS-ASYNCWORKER-PERSISTFAILURE", "error", persistErr, "async_job.handle_id", handleID)
	}
}

func failedOperationResult(message string) openapi.BaseOperationResult {
	if strings.TrimSpace(message) == "" {
		message = "asynchronous package processing failed"
	}
	return openapi.BaseOperationResult{
		ExecutionState: openapi.EXECUTIONSTATE_FAILED,
		Success:        false,
		Messages: []map[string]any{{
			"code":        "AASXFS-ASYNCWORKER-FAILED",
			"messageType": "Error",
			"text":        message,
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		}},
	}
}

func mapAsyncAcceptanceError(err error, operation string) openapi.ImplResponse {
	switch {
	case common.IsErrPayloadTooLarge(err):
		return newAPIErrorResponse(err, http.StatusRequestEntityTooLarge, operation, "PayloadTooLarge")
	case common.IsErrBadRequest(err):
		return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest")
	case common.IsErrConflict(err):
		return newAPIErrorResponse(err, http.StatusConflict, operation, "Conflict")
	default:
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "DurableAcceptance")
	}
}

// GetAASXByPackageId returns a streamed package and its download metadata.
//
// Parameters:
//   - ctx: Request context used for lookup, streaming, and cancellation.
//   - packageID: Base64URL-encoded package identifier from the request path.
//
// Returns:
//   - openapi.ImplResponse: FileDownload owning the response stream, or a mapped API error.
//   - error: Reserved for failures not represented by an API response.
func (s *AASXFileServerAPIAPIService) GetAASXByPackageId(ctx context.Context, packageID string) (openapi.ImplResponse, error) {
	const operation = "GetAASXByPackageId"

	decodedPackageID, decodeErr := common.DecodeString(packageID)
	if decodeErr != nil {
		return newAPIErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedPackageId"), nil
	}

	pkg, err := s.backend.GetPackageByID(ctx, decodedPackageID)
	if err != nil {
		if common.IsErrNotFound(err) {
			return newAPIErrorResponse(err, http.StatusNotFound, operation, "PackageNotFound"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "GetPackageByID"), nil
	}

	return openapi.Response(http.StatusOK, openapi.FileDownload{
		Content:     pkg.Content,
		ContentType: pkg.ContentType,
		Filename:    pkg.FileName,
		Headers: map[string]string{
			"X-FileName": pkg.FileName,
		},
	}), nil
}

// PutAASXByPackageId creates or replaces a staged package.
//
// Parameters:
//   - ctx: Request context containing cancellation and configured AASX limits.
//   - packageID: Base64URL-encoded package identifier from the request path.
//   - file: Seekable staged replacement package owned by the HTTP request.
//   - aasIDs: Replacement AAS identifiers associated with the package.
//   - fileName: Preferred replacement download filename.
//
// Returns:
//   - openapi.ImplResponse: HTTP 204 for replacement, HTTP 201 for creation, or a mapped API error.
//   - error: Reserved for failures not represented by an API response.
func (s *AASXFileServerAPIAPIService) PutAASXByPackageId(ctx context.Context, packageID string, file openapi.StagedUpload, aasIDs []string, fileName string) (openapi.ImplResponse, error) {
	const operation = "PutAASXByPackageId"

	if file == nil {
		return newAPIErrorResponse(errors.New("multipart form field 'file' is required"), http.StatusBadRequest, operation, "MissingFile"), nil
	}

	decodedPackageID, decodeErr := common.DecodeString(packageID)
	if decodeErr != nil {
		return newAPIErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedPackageId"), nil
	}

	updated, record, err := s.backend.PutPackage(ctx, decodedPackageID, file, aasIDs, fileName)
	if err != nil {
		if common.IsErrPayloadTooLarge(err) {
			return newAPIErrorResponse(err, http.StatusRequestEntityTooLarge, operation, "PayloadTooLarge"), nil
		}
		if common.IsErrConflict(err) {
			return newAPIErrorResponse(err, http.StatusConflict, operation, "PackageIdConflict"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "PutPackage"), nil
	}

	if updated {
		return openapi.Response(http.StatusNoContent, nil), nil
	}

	return openapi.Response(http.StatusCreated, toPackageDescription(*record)), nil
}

// DeleteAASXByPackageId removes a package and its associated large object content.
func (s *AASXFileServerAPIAPIService) DeleteAASXByPackageId(ctx context.Context, packageID string) (openapi.ImplResponse, error) {
	const operation = "DeleteAASXByPackageId"

	decodedPackageID, decodeErr := common.DecodeString(packageID)
	if decodeErr != nil {
		return newAPIErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedPackageId"), nil
	}

	err := s.backend.DeletePackageByID(ctx, decodedPackageID)
	if err != nil {
		if common.IsErrNotFound(err) {
			return newAPIErrorResponse(err, http.StatusNotFound, operation, "PackageNotFound"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "DeletePackage"), nil
	}

	return openapi.Response(http.StatusNoContent, nil), nil
}

func toPackageDescription(record persistence.PackageRecord) openapi.PackageDescription {
	aasIDs := make([]string, 0, len(record.AASIDs))
	for _, aasID := range record.AASIDs {
		aasIDs = append(aasIDs, common.EncodeString(aasID))
	}

	return openapi.PackageDescription{
		PackageId:   common.EncodeString(record.PackageID),
		AasIds:      aasIDs,
		FileName:    record.FileName,
		ContentType: record.ContentType,
	}
}

func generatePackageID() string {
	return fmt.Sprintf("pkg-%d", time.Now().UnixNano())
}

func newAPIErrorResponse(err error, status int, operation string, info string) openapi.ImplResponse {
	if err == nil {
		err = errors.New(http.StatusText(status))
	}

	response := common.NewErrorResponse(err, status, componentName, operation, info)
	return openapi.ImplResponse{Code: response.Code, Body: response.Body}
}
