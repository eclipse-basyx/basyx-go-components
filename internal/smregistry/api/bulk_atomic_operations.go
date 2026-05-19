package smregistryapi

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncbulk"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// ExecuteBulkCreateAtomic performs bulk create atomically in one transaction.
func (s *SubmodelRegistryAPIAPIService) ExecuteBulkCreateAtomic(
	ctx context.Context,
	descriptors []model.SubmodelDescriptor,
) asyncbulk.OperationResult {
	return s.executeAtomicSubmodelDescriptorBulk(
		ctx,
		descriptors,
		"SMR-BULK-CREATE-STARTTX",
		"SMR-BULK-CREATE-COMMITTX",
		s.createDescriptorInTransaction,
	)
}

// ExecuteBulkPutAtomic performs bulk upsert atomically in one transaction.
func (s *SubmodelRegistryAPIAPIService) ExecuteBulkPutAtomic(
	ctx context.Context,
	descriptors []model.SubmodelDescriptor,
) asyncbulk.OperationResult {
	return s.executeAtomicSubmodelDescriptorBulk(
		ctx,
		descriptors,
		"SMR-BULK-PUT-STARTTX",
		"SMR-BULK-PUT-COMMITTX",
		s.upsertDescriptorInTransaction,
	)
}

// ExecuteBulkDeleteAtomic performs bulk delete atomically in one transaction.
func (s *SubmodelRegistryAPIAPIService) ExecuteBulkDeleteAtomic(
	ctx context.Context,
	submodelIdentifiers []string,
) asyncbulk.OperationResult {
	return s.executeAtomicSubmodelIdentifierBulk(
		ctx,
		submodelIdentifiers,
		"SMR-BULK-DELETE-STARTTX",
		"SMR-BULK-DELETE-COMMITTX",
	)
}

func (s *SubmodelRegistryAPIAPIService) executeAtomicSubmodelDescriptorBulk(
	ctx context.Context,
	descriptors []model.SubmodelDescriptor,
	startErrorCode string,
	commitErrorCode string,
	execute func(context.Context, *sql.Tx, model.SubmodelDescriptor) (int, error),
) asyncbulk.OperationResult {
	failure := asyncbulk.ItemFailure{}
	err := s.smRegistryBackend.ExecuteInTransaction(startErrorCode, commitErrorCode, func(tx *sql.Tx) error {
		for idx, descriptor := range descriptors {
			statusCode, descriptorErr := execute(ctx, tx, descriptor)
			if descriptorErr != nil {
				failure = asyncbulk.ItemFailure{
					Index:      idx,
					Identifier: descriptor.Id,
					StatusCode: statusCode,
					Message:    descriptorErr.Error(),
				}
				return descriptorErr
			}
		}
		return nil
	})
	if err != nil {
		if failure.StatusCode == 0 {
			failure = asyncbulk.ItemFailure{
				Index:      0,
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}
		return failedAtomicResult(len(descriptors), failure)
	}
	return successfulAtomicResult(len(descriptors))
}

func (s *SubmodelRegistryAPIAPIService) executeAtomicSubmodelIdentifierBulk(
	ctx context.Context,
	submodelIdentifiers []string,
	startErrorCode string,
	commitErrorCode string,
) asyncbulk.OperationResult {
	failure := asyncbulk.ItemFailure{}
	err := s.smRegistryBackend.ExecuteInTransaction(startErrorCode, commitErrorCode, func(tx *sql.Tx) error {
		for idx, rawID := range submodelIdentifiers {
			descriptorID := strings.TrimSpace(rawID)
			if descriptorID == "" {
				failure = asyncbulk.ItemFailure{
					Index:      idx,
					Identifier: rawID,
					StatusCode: http.StatusBadRequest,
					Message:    "SMR-BULK-DELSMDESC-MISSINGID descriptor id must not be empty",
				}
				return common.NewErrBadRequest(failure.Message)
			}

			if err := s.smRegistryBackend.DeleteSubmodelDescriptorByIDInTransaction(ctx, tx, descriptorID); err != nil {
				failure = asyncbulk.ItemFailure{
					Index:      idx,
					Identifier: descriptorID,
					StatusCode: smBulkDeleteErrorStatusCode(err),
					Message:    err.Error(),
				}
				return err
			}
		}
		return nil
	})
	if err != nil {
		if failure.StatusCode == 0 {
			failure = asyncbulk.ItemFailure{
				Index:      0,
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}
		return failedAtomicResult(len(submodelIdentifiers), failure)
	}
	return successfulAtomicResult(len(submodelIdentifiers))
}

func (s *SubmodelRegistryAPIAPIService) createDescriptorInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	descriptor model.SubmodelDescriptor,
) (int, error) {
	descriptorID := strings.TrimSpace(descriptor.Id)
	if descriptorID == "" {
		return http.StatusBadRequest, common.NewErrBadRequest("SMR-BULK-CREATE-MISSINGID descriptor id must not be empty")
	}

	_, err := s.smRegistryBackend.GetSubmodelDescriptorByIDInTransaction(ctx, tx, descriptorID)
	if err == nil {
		return http.StatusConflict, common.NewErrConflict("Submodel with given id already exists")
	}
	if !common.IsErrNotFound(err) {
		return http.StatusInternalServerError, err
	}

	if _, err = s.smRegistryBackend.InsertSubmodelDescriptorInTransaction(ctx, tx, descriptor); err != nil {
		return smBulkCreateErrorStatusCode(err), err
	}
	return http.StatusCreated, nil
}

func (s *SubmodelRegistryAPIAPIService) upsertDescriptorInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	descriptor model.SubmodelDescriptor,
) (int, error) {
	descriptorID := strings.TrimSpace(descriptor.Id)
	if descriptorID == "" {
		return http.StatusBadRequest, common.NewErrBadRequest("SMR-BULK-PUTSMDESC-MISSINGID descriptor id must not be empty")
	}

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return http.StatusInternalServerError, enforceErr
	}

	_, getErr := s.smRegistryBackend.GetSubmodelDescriptorByIDInTransaction(ctx, tx, descriptorID)
	exists := getErr == nil
	if getErr != nil && !common.IsErrNotFound(getErr) {
		return http.StatusInternalServerError, getErr
	}
	if shouldEnforceFormula {
		ctx = auth.SelectPutFormulaByExistence(ctx, exists)
	}

	if err := s.smRegistryBackend.UpsertSubmodelDescriptorInTransaction(ctx, tx, descriptor); err != nil {
		return smBulkPutErrorStatusCode(err), err
	}
	return http.StatusNoContent, nil
}

func successfulAtomicResult(itemCount int) asyncbulk.OperationResult {
	return asyncbulk.OperationResult{
		Success:         true,
		ProcessedCount:  itemCount,
		SuccessfulCount: itemCount,
		FailedCount:     0,
	}
}

func failedAtomicResult(itemCount int, failure asyncbulk.ItemFailure) asyncbulk.OperationResult {
	return asyncbulk.OperationResult{
		Success:         false,
		ProcessedCount:  itemCount,
		SuccessfulCount: 0,
		FailedCount:     itemCount,
		Failures:        []asyncbulk.ItemFailure{failure},
	}
}

func smBulkCreateErrorStatusCode(err error) int {
	switch {
	case common.IsErrBadRequest(err):
		return http.StatusBadRequest
	case common.IsErrConflict(err):
		return http.StatusConflict
	case common.IsErrDenied(err):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

func smBulkPutErrorStatusCode(err error) int {
	switch {
	case common.IsErrBadRequest(err):
		return http.StatusBadRequest
	case common.IsErrConflict(err):
		return http.StatusConflict
	case common.IsErrDenied(err):
		return http.StatusForbidden
	case common.IsErrNotFound(err):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func smBulkDeleteErrorStatusCode(err error) int {
	switch {
	case common.IsErrNotFound(err):
		return http.StatusNotFound
	case common.IsErrDenied(err):
		return http.StatusForbidden
	case common.IsErrBadRequest(err):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
