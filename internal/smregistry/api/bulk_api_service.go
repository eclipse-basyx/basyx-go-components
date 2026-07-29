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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package smregistryapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

const (
	bulkJobRetryAfterSeconds = 2
	bulkJobExecutionTimeout  = 15 * time.Minute
)

type smBulkDescriptorService interface {
	ExecuteBulkCreateAtomic(ctx context.Context, descriptors []model.SubmodelDescriptor) asyncjob.BulkResult
	ExecuteBulkPutAtomic(ctx context.Context, descriptors []model.SubmodelDescriptor) asyncjob.BulkResult
	ExecuteBulkDeleteAtomic(ctx context.Context, submodelIdentifiers []string) asyncjob.BulkResult
}

// BulkService provides SSP-003 bulk jobs for submodel descriptors.
type BulkService struct {
	descriptorService smBulkDescriptorService
	manager           *asyncjob.Manager
}

// NewBulkService creates a new submodel registry bulk service instance.
func NewBulkService(descriptorService smBulkDescriptorService, manager *asyncjob.Manager) *BulkService {
	if manager == nil {
		manager = asyncjob.NewManager("SMR-BULK", 0)
	}
	return &BulkService{
		descriptorService: descriptorService,
		manager:           manager,
	}
}

// StartCreate starts a bulk create job.
func (s *BulkService) StartCreate(ctx context.Context, descriptors []model.SubmodelDescriptor) model.ImplResponse {
	return s.start(ctx, "submodel-registry.bulk.create", "CreateBulkSubmodelDescriptors", "SMR-BULK-CREATE-COMPLETE", func(executionContext context.Context) asyncjob.BulkResult {
		return s.descriptorService.ExecuteBulkCreateAtomic(executionContext, descriptors)
	})
}

// StartPut starts a bulk upsert job.
func (s *BulkService) StartPut(ctx context.Context, descriptors []model.SubmodelDescriptor) model.ImplResponse {
	return s.start(ctx, "submodel-registry.bulk.put", "PutBulkSubmodelDescriptorsById", "SMR-BULK-PUT-COMPLETE", func(executionContext context.Context) asyncjob.BulkResult {
		return s.descriptorService.ExecuteBulkPutAtomic(executionContext, descriptors)
	})
}

// StartDelete starts a bulk delete job.
func (s *BulkService) StartDelete(ctx context.Context, submodelIdentifiers []string) model.ImplResponse {
	return s.start(ctx, "submodel-registry.bulk.delete", "DeleteBulkSubmodelDescriptorsById", "SMR-BULK-DELETE-COMPLETE", func(executionContext context.Context) asyncjob.BulkResult {
		return s.descriptorService.ExecuteBulkDeleteAtomic(executionContext, submodelIdentifiers)
	})
}

func (s *BulkService) start(
	ctx context.Context,
	jobKind string,
	operationName string,
	completionErrorCode string,
	execute func(context.Context) asyncjob.BulkResult,
) model.ImplResponse {
	releaseExecutionSlot, acquired := s.manager.TryAcquireExecutionSlot()
	if !acquired {
		capacityErr := errors.New("SMR-BULK-START-CAPACITY asynchronous execution capacity is exhausted")
		return common.NewErrorResponse(capacityErr, http.StatusTooManyRequests, componentName, operationName, "ExecutionCapacityExhausted")
	}

	executionContext, cancelExecution := s.manager.NewExecutionContext(ctx, bulkJobExecutionTimeout)
	executionDeadline, _ := executionContext.Deadline()
	handleID, err := s.manager.Start(ctx, auth.OwnerKeyFromContext(ctx), asyncjob.StartOptions{
		JobKind:           jobKind,
		ExecutionDeadline: executionDeadline,
	})
	if err != nil {
		releaseExecutionSlot()
		cancelExecution()
		return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, operationName, "CreateHandle")
	}

	go s.execute(executionContext, handleID, cancelExecution, releaseExecutionSlot, completionErrorCode, execute)

	return model.ResponseWithHeaders(http.StatusAccepted, nil, map[string]string{
		"Location": fmt.Sprintf("/bulk/status/%s", url.PathEscape(handleID)),
	})
}

func (s *BulkService) execute(
	executionContext context.Context,
	handleID string,
	cancelExecution context.CancelFunc,
	releaseExecutionSlot func(),
	completionErrorCode string,
	execute func(context.Context) asyncjob.BulkResult,
) {
	defer releaseExecutionSlot()
	defer cancelExecution()
	stopHeartbeat := s.manager.KeepAlive(executionContext, handleID)
	defer stopHeartbeat()

	result := execute(executionContext)
	persistenceContext, cancelPersistence := asyncjob.NewPersistenceContext(executionContext)
	defer cancelPersistence()
	if err := s.manager.Complete(persistenceContext, handleID, result); err != nil {
		slog.ErrorContext(persistenceContext, "submodel registry bulk job completion failed", "error.code", completionErrorCode, "error", err, "async_job.handle_id", handleID)
	}
}

// GetStatus returns bulk job execution status by handle id.
func (s *BulkService) GetStatus(ctx context.Context, handleID string) model.ImplResponse {
	record, found, err := s.manager.GetForOwner(ctx, handleID, auth.OwnerKeyFromContext(ctx))
	if err != nil {
		return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "GetBulkAsyncStatus", "ReadHandle")
	}
	if !found {
		return common.NewErrorResponse(common.NewErrNotFound(handleID), http.StatusNotFound, componentName, "GetBulkAsyncStatus", "HandleNotFound")
	}

	if record.ExecutionState == "Running" {
		return model.Response(http.StatusOK, map[string]any{
			"executionState": "Running",
			"success":        true,
			"retryAfter":     bulkJobRetryAfterSeconds,
		})
	}

	location := fmt.Sprintf("/bulk/result/%s", url.PathEscape(handleID))
	return model.ResponseWithHeaders(http.StatusFound, nil, map[string]string{"Location": location})
}

// GetResult returns a bulk job result by handle id.
func (s *BulkService) GetResult(ctx context.Context, handleID string) model.ImplResponse {
	record, found, err := s.manager.GetForOwner(ctx, handleID, auth.OwnerKeyFromContext(ctx))
	if err != nil {
		return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "GetBulkAsyncResult", "ReadHandle")
	}
	if !found {
		return common.NewErrorResponse(common.NewErrNotFound(handleID), http.StatusNotFound, componentName, "GetBulkAsyncResult", "HandleNotFound")
	}

	if record.ExecutionState == "Running" {
		runningErr := errors.New("SMR-BULK-GETRESULT-RUNNING bulk operation is still running")
		return common.NewErrorResponse(runningErr, http.StatusBadRequest, componentName, "GetBulkAsyncResult", "OperationStillRunning")
	}

	if err := s.manager.DeleteForOwner(ctx, handleID, auth.OwnerKeyFromContext(ctx)); err != nil {
		return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "GetBulkAsyncResult", "DeleteHandle")
	}

	if record.ExecutionState == "Failed" {
		status := record.ErrorStatus
		if status <= 0 {
			status = http.StatusInternalServerError
		}
		return model.Response(status, record.ErrorBody)
	}

	if record.Result.Success {
		return model.Response(http.StatusNoContent, nil)
	}

	return model.Response(http.StatusBadRequest, map[string]any{
		"messages":        asyncjob.ToMessages(record.Result.Failures),
		"executionState":  "Completed",
		"success":         false,
		"processedCount":  record.Result.ProcessedCount,
		"successfulCount": record.Result.SuccessfulCount,
		"failedCount":     record.Result.FailedCount,
		"details":         record.Result.Failures,
	})
}
