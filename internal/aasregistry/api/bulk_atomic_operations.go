package aasregistryapi

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
func (s *AssetAdministrationShellRegistryAPIAPIService) ExecuteBulkCreateAtomic(
	ctx context.Context,
	descriptors []model.AssetAdministrationShellDescriptor,
) asyncbulk.OperationResult {
	return s.executeAtomicAASDescriptorBulk(
		ctx,
		descriptors,
		"AASR-BULK-CREATE-STARTTX",
		"AASR-BULK-CREATE-COMMITTX",
		s.createDescriptorInTransaction,
	)
}

// ExecuteBulkPutAtomic performs bulk upsert atomically in one transaction.
func (s *AssetAdministrationShellRegistryAPIAPIService) ExecuteBulkPutAtomic(
	ctx context.Context,
	descriptors []model.AssetAdministrationShellDescriptor,
) asyncbulk.OperationResult {
	return s.executeAtomicAASDescriptorBulk(
		ctx,
		descriptors,
		"AASR-BULK-PUT-STARTTX",
		"AASR-BULK-PUT-COMMITTX",
		s.upsertDescriptorInTransaction,
	)
}

// ExecuteBulkDeleteAtomic performs bulk delete atomically in one transaction.
func (s *AssetAdministrationShellRegistryAPIAPIService) ExecuteBulkDeleteAtomic(
	ctx context.Context,
	aasIdentifiers []string,
) asyncbulk.OperationResult {
	return s.executeAtomicAASIdentifierBulk(
		ctx,
		aasIdentifiers,
		"AASR-BULK-DELETE-STARTTX",
		"AASR-BULK-DELETE-COMMITTX",
	)
}

func (s *AssetAdministrationShellRegistryAPIAPIService) executeAtomicAASDescriptorBulk(
	ctx context.Context,
	descriptors []model.AssetAdministrationShellDescriptor,
	startErrorCode string,
	commitErrorCode string,
	execute func(context.Context, *sql.Tx, model.AssetAdministrationShellDescriptor) (int, error),
) asyncbulk.OperationResult {
	failure := asyncbulk.ItemFailure{}
	err := s.aasRegistryBackend.ExecuteInTransaction(startErrorCode, commitErrorCode, func(tx *sql.Tx) error {
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
		return failedAtomicResult(descriptorIDsFromAASDescriptors(descriptors), failure)
	}
	return successfulAtomicResult(len(descriptors))
}

func (s *AssetAdministrationShellRegistryAPIAPIService) executeAtomicAASIdentifierBulk(
	ctx context.Context,
	aasIdentifiers []string,
	startErrorCode string,
	commitErrorCode string,
) asyncbulk.OperationResult {
	failure := asyncbulk.ItemFailure{}
	err := s.aasRegistryBackend.ExecuteInTransaction(startErrorCode, commitErrorCode, func(tx *sql.Tx) error {
		for idx, rawID := range aasIdentifiers {
			descriptorID := strings.TrimSpace(rawID)
			if descriptorID == "" {
				failure = asyncbulk.ItemFailure{
					Index:      idx,
					Identifier: rawID,
					StatusCode: http.StatusBadRequest,
					Message:    "AASR-BULK-DELAASDESC-MISSINGID descriptor id must not be empty",
				}
				return common.NewErrBadRequest(failure.Message)
			}

			if err := s.aasRegistryBackend.DeleteAssetAdministrationShellDescriptorByIDInTransaction(ctx, tx, descriptorID); err != nil {
				failure = asyncbulk.ItemFailure{
					Index:      idx,
					Identifier: descriptorID,
					StatusCode: aasBulkDeleteErrorStatusCode(err),
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
		return failedAtomicResult(normalizeAASIdentifiers(aasIdentifiers), failure)
	}
	return successfulAtomicResult(len(aasIdentifiers))
}

func (s *AssetAdministrationShellRegistryAPIAPIService) createDescriptorInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	descriptor model.AssetAdministrationShellDescriptor,
) (int, error) {
	descriptorID := strings.TrimSpace(descriptor.Id)
	if descriptorID == "" {
		return http.StatusBadRequest, common.NewErrBadRequest("AASR-BULK-CREATE-MISSINGID descriptor id must not be empty")
	}

	_, err := s.aasRegistryBackend.GetAssetAdministrationShellDescriptorByIDInTransaction(ctx, tx, descriptorID)
	if err == nil {
		return http.StatusConflict, common.NewErrConflict("AAS with given id already exists")
	}
	if !common.IsErrNotFound(err) {
		return http.StatusInternalServerError, err
	}

	if err = s.aasRegistryBackend.InsertAdministrationShellDescriptorInTransaction(ctx, tx, descriptor); err != nil {
		return aasBulkCreateErrorStatusCode(err), err
	}
	return http.StatusCreated, nil
}

func (s *AssetAdministrationShellRegistryAPIAPIService) upsertDescriptorInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	descriptor model.AssetAdministrationShellDescriptor,
) (int, error) {
	descriptorID := strings.TrimSpace(descriptor.Id)
	if descriptorID == "" {
		return http.StatusBadRequest, common.NewErrBadRequest("AASR-BULK-PUTAASDESC-MISSINGID descriptor id must not be empty")
	}

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return http.StatusInternalServerError, enforceErr
	}

	_, getErr := s.aasRegistryBackend.GetAssetAdministrationShellDescriptorByIDInTransaction(ctx, tx, descriptorID)
	exists := getErr == nil
	if getErr != nil && !common.IsErrNotFound(getErr) {
		return http.StatusInternalServerError, getErr
	}
	if shouldEnforceFormula {
		ctx = auth.SelectPutFormulaByExistence(ctx, exists)
	}

	if err := s.aasRegistryBackend.UpsertAdministrationShellDescriptorInTransaction(ctx, tx, descriptor); err != nil {
		return aasBulkPutErrorStatusCode(err), err
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

func failedAtomicResult(itemIdentifiers []string, failure asyncbulk.ItemFailure) asyncbulk.OperationResult {
	failures := asyncbulk.ExpandAtomicFailures(itemIdentifiers, failure)
	itemCount := len(itemIdentifiers)
	return asyncbulk.OperationResult{
		Success:         false,
		ProcessedCount:  itemCount,
		SuccessfulCount: 0,
		FailedCount:     itemCount,
		Failures:        failures,
	}
}

func descriptorIDsFromAASDescriptors(descriptors []model.AssetAdministrationShellDescriptor) []string {
	ids := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		ids = append(ids, strings.TrimSpace(descriptor.Id))
	}
	return ids
}

func normalizeAASIdentifiers(rawIdentifiers []string) []string {
	identifiers := make([]string, 0, len(rawIdentifiers))
	for _, rawID := range rawIdentifiers {
		identifiers = append(identifiers, strings.TrimSpace(rawID))
	}
	return identifiers
}

func aasBulkCreateErrorStatusCode(err error) int {
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

func aasBulkPutErrorStatusCode(err error) int {
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

func aasBulkDeleteErrorStatusCode(err error) int {
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
