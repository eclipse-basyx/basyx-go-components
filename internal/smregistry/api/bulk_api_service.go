package smregistryapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncbulk"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const (
	bulkOperationRetryAfterSeconds = 2
)

type smBulkDescriptorService interface {
	ExecuteBulkCreateAtomic(ctx context.Context, descriptors []model.SubmodelDescriptor) asyncbulk.OperationResult
	ExecuteBulkPutAtomic(ctx context.Context, descriptors []model.SubmodelDescriptor) asyncbulk.OperationResult
	ExecuteBulkDeleteAtomic(ctx context.Context, submodelIdentifiers []string) asyncbulk.OperationResult
}

// BulkService provides SSP-003 async bulk operations for submodel descriptors.
type BulkService struct {
	descriptorService smBulkDescriptorService
	manager           *asyncbulk.Manager
}

// NewBulkService creates a new submodel registry bulk service instance.
func NewBulkService(descriptorService smBulkDescriptorService, manager *asyncbulk.Manager) *BulkService {
	if manager == nil {
		manager = asyncbulk.NewManager("SMR-BULK", 0)
	}
	return &BulkService{
		descriptorService: descriptorService,
		manager:           manager,
	}
}

// StartCreate starts an async bulk create operation.
func (s *BulkService) StartCreate(ctx context.Context, descriptors []model.SubmodelDescriptor) model.ImplResponse {
	handleID, handleErr := s.manager.Start(asyncbulk.OwnerKeyFromContext(ctx))
	if handleErr != nil {
		return common.NewErrorResponse(handleErr, http.StatusInternalServerError, componentName, "CreateBulkSubmodelDescriptors", "CreateHandle")
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
func (s *BulkService) StartPut(ctx context.Context, descriptors []model.SubmodelDescriptor) model.ImplResponse {
	handleID, handleErr := s.manager.Start(asyncbulk.OwnerKeyFromContext(ctx))
	if handleErr != nil {
		return common.NewErrorResponse(handleErr, http.StatusInternalServerError, componentName, "PutBulkSubmodelDescriptorsById", "CreateHandle")
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
func (s *BulkService) StartDelete(ctx context.Context, submodelIdentifiers []string) model.ImplResponse {
	handleID, handleErr := s.manager.Start(asyncbulk.OwnerKeyFromContext(ctx))
	if handleErr != nil {
		return common.NewErrorResponse(handleErr, http.StatusInternalServerError, componentName, "DeleteBulkSubmodelDescriptorsById", "CreateHandle")
	}
	asyncCtx := context.WithoutCancel(ctx)

	go func() {
		s.manager.Complete(handleID, s.descriptorService.ExecuteBulkDeleteAtomic(asyncCtx, submodelIdentifiers))
	}()

	return model.ResponseWithHeaders(http.StatusAccepted, nil, map[string]string{
		"Location": fmt.Sprintf("/bulk/status/%s", url.PathEscape(handleID)),
	})
}

// GetStatus returns async bulk execution status by handle id.
func (s *BulkService) GetStatus(ctx context.Context, handleID string) model.ImplResponse {
	record, found := s.manager.GetForOwner(handleID, asyncbulk.OwnerKeyFromContext(ctx))
	if !found {
		return common.NewErrorResponse(common.NewErrNotFound(handleID), http.StatusNotFound, componentName, "GetBulkAsyncStatus", "HandleNotFound")
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
	record, found := s.manager.GetForOwner(handleID, asyncbulk.OwnerKeyFromContext(ctx))
	if !found {
		return common.NewErrorResponse(common.NewErrNotFound(handleID), http.StatusNotFound, componentName, "GetBulkAsyncResult", "HandleNotFound")
	}

	if record.ExecutionState == "Running" {
		runningErr := errors.New("SMR-BULK-GETRESULT-RUNNING bulk operation is still running")
		return common.NewErrorResponse(runningErr, http.StatusBadRequest, componentName, "GetBulkAsyncResult", "OperationStillRunning")
	}

	s.manager.Delete(handleID)

	if record.Result.Success {
		return model.Response(http.StatusNoContent, nil)
	}

	baseErr := common.NewErrorResponse(
		errors.New("SMR-BULK-GETRESULT-FAILEDDESCRIPTORS at least one descriptor operation failed and the transaction was rolled back"),
		http.StatusBadRequest,
		componentName,
		"GetBulkAsyncResult",
		"DescriptorFailures",
	)

	return model.Response(http.StatusBadRequest, map[string]any{
		"messages":        baseErr.Body,
		"executionState":  "Completed",
		"success":         false,
		"processedCount":  record.Result.ProcessedCount,
		"successfulCount": record.Result.SuccessfulCount,
		"failedCount":     record.Result.FailedCount,
		"details":         record.Result.Failures,
	})
}
