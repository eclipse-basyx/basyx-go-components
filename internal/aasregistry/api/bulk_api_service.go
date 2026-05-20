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

package aasregistryapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncbulk"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

const (
	bulkOperationRetryAfterSeconds = 2
)

type aasBulkDescriptorService interface {
	ExecuteBulkCreateAtomic(ctx context.Context, descriptors []model.AssetAdministrationShellDescriptor) asyncbulk.OperationResult
	ExecuteBulkPutAtomic(ctx context.Context, descriptors []model.AssetAdministrationShellDescriptor) asyncbulk.OperationResult
	ExecuteBulkDeleteAtomic(ctx context.Context, aasIdentifiers []string) asyncbulk.OperationResult
}

// BulkService provides SSP-003 async bulk operations for AAS descriptors.
type BulkService struct {
	descriptorService aasBulkDescriptorService
	manager           *asyncbulk.Manager
}

// NewBulkService creates a new bulk service instance.
func NewBulkService(descriptorService aasBulkDescriptorService, manager *asyncbulk.Manager) *BulkService {
	if manager == nil {
		manager = asyncbulk.NewManager("AASR-BULK", 0)
	}
	return &BulkService{
		descriptorService: descriptorService,
		manager:           manager,
	}
}

// StartCreate starts an async bulk create operation.
func (s *BulkService) StartCreate(ctx context.Context, descriptors []model.AssetAdministrationShellDescriptor) model.ImplResponse {
	handleID, handleErr := s.manager.Start(auth.OwnerKeyFromContext(ctx))
	if handleErr != nil {
		return common.NewErrorResponse(handleErr, http.StatusInternalServerError, componentName, "CreateBulkAssetAdministrationShellDescriptors", "CreateHandle")
	}
	asyncCtx := context.WithoutCancel(ctx)

	go func() {
		s.manager.Complete(handleID, s.descriptorService.ExecuteBulkCreateAtomic(asyncCtx, descriptors))
	}()

	return model.ResponseWithHeaders(http.StatusAccepted, nil, map[string]string{
		"Location": fmt.Sprintf("/bulk/status/%s", url.PathEscape(handleID)),
	})
}

// StartPut starts an async bulk upsert operation.
func (s *BulkService) StartPut(ctx context.Context, descriptors []model.AssetAdministrationShellDescriptor) model.ImplResponse {
	handleID, handleErr := s.manager.Start(auth.OwnerKeyFromContext(ctx))
	if handleErr != nil {
		return common.NewErrorResponse(handleErr, http.StatusInternalServerError, componentName, "PutBulkAssetAdministrationShellDescriptorsById", "CreateHandle")
	}
	asyncCtx := context.WithoutCancel(ctx)

	go func() {
		s.manager.Complete(handleID, s.descriptorService.ExecuteBulkPutAtomic(asyncCtx, descriptors))
	}()

	return model.ResponseWithHeaders(http.StatusAccepted, nil, map[string]string{
		"Location": fmt.Sprintf("/bulk/status/%s", url.PathEscape(handleID)),
	})
}

// StartDelete starts an async bulk delete operation.
func (s *BulkService) StartDelete(ctx context.Context, aasIdentifiers []string) model.ImplResponse {
	handleID, handleErr := s.manager.Start(auth.OwnerKeyFromContext(ctx))
	if handleErr != nil {
		return common.NewErrorResponse(handleErr, http.StatusInternalServerError, componentName, "DeleteBulkAssetAdministrationShellDescriptorsById", "CreateHandle")
	}
	asyncCtx := context.WithoutCancel(ctx)

	go func() {
		s.manager.Complete(handleID, s.descriptorService.ExecuteBulkDeleteAtomic(asyncCtx, aasIdentifiers))
	}()

	return model.ResponseWithHeaders(http.StatusAccepted, nil, map[string]string{
		"Location": fmt.Sprintf("/bulk/status/%s", url.PathEscape(handleID)),
	})
}

// GetStatus returns async bulk execution status by handle id.
func (s *BulkService) GetStatus(ctx context.Context, handleID string) model.ImplResponse {
	record, found := s.manager.GetForOwner(handleID, auth.OwnerKeyFromContext(ctx))
	if !found {
		return common.NewErrorResponse(common.NewErrNotFound(handleID), http.StatusNotFound, componentName, "GetAsyncBulkStatus", "HandleNotFound")
	}

	if record.ExecutionState == "Running" {
		return model.Response(http.StatusOK, map[string]any{
			"executionState": "Running",
			"success":        true,
			"retryAfter":     bulkOperationRetryAfterSeconds,
		})
	}

	location := fmt.Sprintf("/bulk/result/%s", url.PathEscape(handleID))
	return model.ResponseWithHeaders(http.StatusFound, nil, map[string]string{"Location": location})
}

// GetResult returns async bulk result by handle id.
func (s *BulkService) GetResult(ctx context.Context, handleID string) model.ImplResponse {
	record, found := s.manager.GetForOwner(handleID, auth.OwnerKeyFromContext(ctx))
	if !found {
		return common.NewErrorResponse(common.NewErrNotFound(handleID), http.StatusNotFound, componentName, "GetBulkAsyncResult", "HandleNotFound")
	}

	if record.ExecutionState == "Running" {
		runningErr := errors.New("AASR-BULK-GETRESULT-RUNNING bulk operation is still running")
		return common.NewErrorResponse(runningErr, http.StatusBadRequest, componentName, "GetBulkAsyncResult", "OperationStillRunning")
	}

	s.manager.Delete(handleID)

	if record.Result.Success {
		return model.Response(http.StatusNoContent, nil)
	}

	return model.Response(http.StatusBadRequest, map[string]any{
		"messages":        asyncbulk.ToMessages(record.Result.Failures),
		"executionState":  "Completed",
		"success":         false,
		"processedCount":  record.Result.ProcessedCount,
		"successfulCount": record.Result.SuccessfulCount,
		"failedCount":     record.Result.FailedCount,
		"details":         record.Result.Failures,
	})
}
