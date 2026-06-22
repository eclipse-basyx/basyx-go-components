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
	if len(descriptors) == 0 {
		return successfulAtomicResult(0)
	}
	failure := validateBulkCreateSubmodelDescriptors(descriptors)
	if failure.StatusCode != 0 {
		return failedAtomicResult(descriptorIDsFromSubmodelDescriptors(descriptors), failure)
	}

	err := s.smRegistryBackend.ExecuteInTransaction(
		"SMR-BULK-CREATE-STARTTX",
		"SMR-BULK-CREATE-COMMITTX",
		func(tx *sql.Tx) error {
			return s.executeBulkCreateSubmodelDescriptorsTx(ctx, tx, descriptors, &failure)
		},
	)
	if err != nil {
		if failure.StatusCode == 0 {
			failure = asyncbulk.ItemFailure{
				Index:      0,
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}
		return failedAtomicResult(descriptorIDsFromSubmodelDescriptors(descriptors), failure)
	}
	return successfulAtomicResult(len(descriptors))
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
		return failedAtomicResult(descriptorIDsFromSubmodelDescriptors(descriptors), failure)
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
		return s.executeBulkDeleteSubmodelIdentifiersTx(ctx, tx, submodelIdentifiers, &failure)
	})
	if err != nil {
		if failure.StatusCode == 0 {
			failure = asyncbulk.ItemFailure{
				Index:      0,
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}
		return failedAtomicResult(normalizeSubmodelIdentifiers(submodelIdentifiers), failure)
	}
	return successfulAtomicResult(len(submodelIdentifiers))
}

func (s *SubmodelRegistryAPIAPIService) executeBulkCreateSubmodelDescriptorsTx(
	ctx context.Context,
	tx *sql.Tx,
	descriptors []model.SubmodelDescriptor,
	failure *asyncbulk.ItemFailure,
) error {
	identifiers := descriptorIDsFromSubmodelDescriptors(descriptors)
	if err := s.ensureSubmodelDescriptorsDoNotExist(ctx, tx, identifiers, failure); err != nil {
		return err
	}
	if err := validateBulkCreateSubmodelDescriptorGraphs(descriptors, failure); err != nil {
		return err
	}
	failedIndex, err := s.smRegistryBackend.InsertSubmodelDescriptorsInTransaction(ctx, tx, descriptors)
	if err == nil {
		return nil
	}
	if failedIndex < 0 || failedIndex >= len(descriptors) {
		failedIndex = 0
	}
	*failure = asyncbulk.ItemFailure{
		Index:      failedIndex,
		Identifier: descriptors[failedIndex].Id,
		StatusCode: smBulkCreateErrorStatusCode(err),
		Message:    err.Error(),
	}
	return err
}

func (s *SubmodelRegistryAPIAPIService) ensureSubmodelDescriptorsDoNotExist(
	ctx context.Context,
	tx *sql.Tx,
	identifiers []string,
	failure *asyncbulk.ItemFailure,
) error {
	if auth.GetQueryFilter(ctx) != nil {
		return s.ensureVisibleSubmodelDescriptorsDoNotExist(ctx, tx, identifiers, failure)
	}

	existing, err := s.smRegistryBackend.ExistingSubmodelDescriptorIDsInTransaction(ctx, tx, identifiers)
	if err != nil {
		*failure = asyncbulk.ItemFailure{Index: 0, Identifier: firstIdentifier(identifiers), StatusCode: http.StatusInternalServerError, Message: err.Error()}
		return err
	}
	for index, identifier := range identifiers {
		if _, found := existing[identifier]; found {
			err := common.NewErrConflict("Submodel with given id already exists")
			*failure = asyncbulk.ItemFailure{Index: index, Identifier: identifier, StatusCode: http.StatusConflict, Message: err.Error()}
			return err
		}
	}
	return nil
}

func (s *SubmodelRegistryAPIAPIService) ensureVisibleSubmodelDescriptorsDoNotExist(
	ctx context.Context,
	tx *sql.Tx,
	identifiers []string,
	failure *asyncbulk.ItemFailure,
) error {
	for index, identifier := range identifiers {
		_, err := s.smRegistryBackend.GetSubmodelDescriptorByIDInTransaction(ctx, tx, identifier)
		if err == nil {
			err = common.NewErrConflict("Submodel with given id already exists")
			*failure = asyncbulk.ItemFailure{Index: index, Identifier: identifier, StatusCode: http.StatusConflict, Message: err.Error()}
			return err
		}
		if common.IsErrNotFound(err) {
			continue
		}
		*failure = asyncbulk.ItemFailure{Index: index, Identifier: identifier, StatusCode: smBulkCreateErrorStatusCode(err), Message: err.Error()}
		return err
	}
	return nil
}

func validateBulkCreateSubmodelDescriptorGraphs(
	descriptors []model.SubmodelDescriptor,
	failure *asyncbulk.ItemFailure,
) error {
	for index, descriptor := range descriptors {
		if len(descriptor.Endpoints) == 0 {
			err := common.NewErrBadRequest("Submodel Descriptor needs at least 1 Endpoint.")
			*failure = asyncbulk.ItemFailure{
				Index:      index,
				Identifier: descriptor.Id,
				StatusCode: smBulkCreateErrorStatusCode(err),
				Message:    err.Error(),
			}
			return err
		}
	}
	return nil
}

func (s *SubmodelRegistryAPIAPIService) executeBulkDeleteSubmodelIdentifiersTx(
	ctx context.Context,
	tx *sql.Tx,
	rawIdentifiers []string,
	failure *asyncbulk.ItemFailure,
) error {
	identifiers, err := s.validateBulkDeleteSubmodelIdentifiersTx(ctx, tx, rawIdentifiers, failure)
	if err != nil {
		return err
	}
	if err = s.smRegistryBackend.DeleteSubmodelDescriptorsByIDsInTransaction(ctx, tx, identifiers); err != nil {
		*failure = asyncbulk.ItemFailure{
			Index:      0,
			Identifier: firstIdentifier(identifiers),
			StatusCode: smBulkDeleteErrorStatusCode(err),
			Message:    err.Error(),
		}
		return err
	}
	return nil
}

func (s *SubmodelRegistryAPIAPIService) validateBulkDeleteSubmodelIdentifiersTx(
	ctx context.Context,
	tx *sql.Tx,
	rawIdentifiers []string,
	failure *asyncbulk.ItemFailure,
) ([]string, error) {
	identifiers := normalizeSubmodelIdentifiers(rawIdentifiers)
	existing, err := s.smRegistryBackend.ExistingSubmodelDescriptorIDsInTransaction(ctx, tx, identifiers)
	if err != nil {
		*failure = asyncbulk.ItemFailure{Index: 0, Identifier: firstIdentifier(identifiers), StatusCode: http.StatusInternalServerError, Message: err.Error()}
		return nil, err
	}
	return identifiers, validateExistingSubmodelDeleteIdentifiers(rawIdentifiers, identifiers, existing, failure)
}

func validateExistingSubmodelDeleteIdentifiers(
	rawIdentifiers []string,
	identifiers []string,
	existing map[string]struct{},
	failure *asyncbulk.ItemFailure,
) error {
	seen := make(map[string]struct{}, len(identifiers))
	for idx, identifier := range identifiers {
		if identifier == "" {
			*failure = asyncbulk.ItemFailure{
				Index:      idx,
				Identifier: rawIdentifiers[idx],
				StatusCode: http.StatusBadRequest,
				Message:    "SMR-BULK-DELSMDESC-MISSINGID descriptor id must not be empty",
			}
			return common.NewErrBadRequest(failure.Message)
		}
		if _, found := existing[identifier]; !found {
			return submodelBulkDeleteNotFound(idx, identifier, failure)
		}
		if _, duplicate := seen[identifier]; duplicate {
			return submodelBulkDeleteNotFound(idx, identifier, failure)
		}
		seen[identifier] = struct{}{}
	}
	return nil
}

func submodelBulkDeleteNotFound(index int, identifier string, failure *asyncbulk.ItemFailure) error {
	err := common.NewErrNotFound("Submodel Descriptor not found")
	*failure = asyncbulk.ItemFailure{
		Index:      index,
		Identifier: identifier,
		StatusCode: smBulkDeleteErrorStatusCode(err),
		Message:    err.Error(),
	}
	return err
}

func validateBulkCreateSubmodelDescriptors(descriptors []model.SubmodelDescriptor) asyncbulk.ItemFailure {
	seen := make(map[string]struct{}, len(descriptors))
	for index, descriptor := range descriptors {
		identifier := strings.TrimSpace(descriptor.Id)
		descriptors[index].Id = identifier
		if identifier == "" {
			err := common.NewErrBadRequest("SMR-BULK-CREATE-MISSINGID descriptor id must not be empty")
			return asyncbulk.ItemFailure{
				Index:      index,
				Identifier: identifier,
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			}
		}
		if _, found := seen[identifier]; found {
			err := common.NewErrConflict("Submodel with given id occurs multiple times in bulk request")
			return asyncbulk.ItemFailure{
				Index:      index,
				Identifier: identifier,
				StatusCode: http.StatusConflict,
				Message:    err.Error(),
			}
		}
		seen[identifier] = struct{}{}
	}
	return asyncbulk.ItemFailure{}
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

func descriptorIDsFromSubmodelDescriptors(descriptors []model.SubmodelDescriptor) []string {
	ids := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		ids = append(ids, strings.TrimSpace(descriptor.Id))
	}
	return ids
}

func normalizeSubmodelIdentifiers(rawIdentifiers []string) []string {
	identifiers := make([]string, 0, len(rawIdentifiers))
	for _, rawID := range rawIdentifiers {
		identifiers = append(identifiers, strings.TrimSpace(rawID))
	}
	return identifiers
}

func firstIdentifier(identifiers []string) string {
	if len(identifiers) == 0 {
		return ""
	}
	return identifiers[0]
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
