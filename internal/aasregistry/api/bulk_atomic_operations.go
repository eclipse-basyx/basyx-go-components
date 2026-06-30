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
	if len(descriptors) == 0 {
		return successfulAtomicResult(0)
	}
	failure := validateBulkCreateDescriptors(descriptors)
	if failure.StatusCode != 0 {
		return failedAtomicResult(descriptorIDsFromAASDescriptors(descriptors), failure)
	}

	err := s.aasRegistryBackend.ExecuteInTransaction(
		"AASR-BULK-CREATE-STARTTX",
		"AASR-BULK-CREATE-COMMITTX",
		func(tx *sql.Tx) error {
			identifiers := descriptorIDsFromAASDescriptors(descriptors)
			if existsErr := s.ensureAASDescriptorsDoNotExist(ctx, tx, identifiers, &failure); existsErr != nil {
				return existsErr
			}

			failedIndex, insertErr := s.aasRegistryBackend.InsertAdministrationShellDescriptorsInTransaction(ctx, tx, descriptors)
			if insertErr != nil {
				if failedIndex < 0 || failedIndex >= len(descriptors) {
					failedIndex = 0
				}
				failure = asyncbulk.ItemFailure{
					Index:      failedIndex,
					Identifier: descriptors[failedIndex].Id,
					StatusCode: aasBulkCreateErrorStatusCode(insertErr),
					Message:    insertErr.Error(),
				}
				return insertErr
			}
			return nil
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
		return failedAtomicResult(descriptorIDsFromAASDescriptors(descriptors), failure)
	}
	return successfulAtomicResult(len(descriptors))
}

func (s *AssetAdministrationShellRegistryAPIAPIService) ensureAASDescriptorsDoNotExist(
	ctx context.Context,
	tx *sql.Tx,
	identifiers []string,
	failure *asyncbulk.ItemFailure,
) error {
	if auth.GetQueryFilter(ctx) != nil {
		return s.ensureVisibleAASDescriptorsDoNotExist(ctx, tx, identifiers, failure)
	}

	existing, err := s.aasRegistryBackend.ExistingAASDescriptorIDsInTransaction(ctx, tx, identifiers)
	if err != nil {
		*failure = asyncbulk.ItemFailure{Index: 0, Identifier: firstIdentifier(identifiers), StatusCode: http.StatusInternalServerError, Message: err.Error()}
		return err
	}
	for index, identifier := range identifiers {
		if _, found := existing[identifier]; found {
			err := common.NewErrConflict("AAS with given id already exists")
			*failure = asyncbulk.ItemFailure{Index: index, Identifier: identifier, StatusCode: http.StatusConflict, Message: err.Error()}
			return err
		}
	}
	return nil
}

func (s *AssetAdministrationShellRegistryAPIAPIService) ensureVisibleAASDescriptorsDoNotExist(
	ctx context.Context,
	tx *sql.Tx,
	identifiers []string,
	failure *asyncbulk.ItemFailure,
) error {
	for index, identifier := range identifiers {
		_, err := s.aasRegistryBackend.GetAssetAdministrationShellDescriptorByIDInTransaction(ctx, tx, identifier)
		if err == nil {
			err = common.NewErrConflict("AAS with given id already exists")
			*failure = asyncbulk.ItemFailure{Index: index, Identifier: identifier, StatusCode: http.StatusConflict, Message: err.Error()}
			return err
		}
		if common.IsErrNotFound(err) {
			continue
		}
		*failure = asyncbulk.ItemFailure{Index: index, Identifier: identifier, StatusCode: aasBulkCreateErrorStatusCode(err), Message: err.Error()}
		return err
	}
	return nil
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
		return s.executeBulkDeleteAASIdentifiersTx(ctx, tx, aasIdentifiers, &failure)
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

func (s *AssetAdministrationShellRegistryAPIAPIService) executeBulkDeleteAASIdentifiersTx(
	ctx context.Context,
	tx *sql.Tx,
	rawIdentifiers []string,
	failure *asyncbulk.ItemFailure,
) error {
	identifiers, err := s.validateBulkDeleteAASIdentifiersTx(ctx, tx, rawIdentifiers, failure)
	if err != nil {
		return err
	}
	failedIndex, err := s.aasRegistryBackend.DeleteAssetAdministrationShellDescriptorsByIDsInTransaction(ctx, tx, identifiers)
	if err == nil {
		return nil
	}
	if failedIndex < 0 || failedIndex >= len(identifiers) {
		failedIndex = 0
	}
	*failure = asyncbulk.ItemFailure{
		Index:      failedIndex,
		Identifier: identifiers[failedIndex],
		StatusCode: aasBulkDeleteErrorStatusCode(err),
		Message:    err.Error(),
	}
	return err
}

func (s *AssetAdministrationShellRegistryAPIAPIService) validateBulkDeleteAASIdentifiersTx(
	ctx context.Context,
	tx *sql.Tx,
	rawIdentifiers []string,
	failure *asyncbulk.ItemFailure,
) ([]string, error) {
	identifiers := normalizeAASIdentifiers(rawIdentifiers)
	existing, err := s.aasRegistryBackend.ExistingAASDescriptorIDsInTransaction(ctx, tx, identifiers)
	if err != nil {
		*failure = asyncbulk.ItemFailure{Index: 0, Identifier: firstIdentifier(identifiers), StatusCode: http.StatusInternalServerError, Message: err.Error()}
		return nil, err
	}
	return identifiers, validateExistingAASDeleteIdentifiers(rawIdentifiers, identifiers, existing, failure)
}

func validateExistingAASDeleteIdentifiers(
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
				Message:    "AASR-BULK-DELAASDESC-MISSINGID descriptor id must not be empty",
			}
			return common.NewErrBadRequest(failure.Message)
		}
		if _, found := existing[identifier]; !found {
			return aasBulkDeleteNotFound(idx, identifier, failure)
		}
		if _, duplicate := seen[identifier]; duplicate {
			return aasBulkDeleteNotFound(idx, identifier, failure)
		}
		seen[identifier] = struct{}{}
	}
	return nil
}

func aasBulkDeleteNotFound(index int, identifier string, failure *asyncbulk.ItemFailure) error {
	err := common.NewErrNotFound("AAS Descriptor not found")
	*failure = asyncbulk.ItemFailure{
		Index:      index,
		Identifier: identifier,
		StatusCode: aasBulkDeleteErrorStatusCode(err),
		Message:    err.Error(),
	}
	return err
}

func validateBulkCreateDescriptors(descriptors []model.AssetAdministrationShellDescriptor) asyncbulk.ItemFailure {
	seen := make(map[string]struct{}, len(descriptors))
	for index, descriptor := range descriptors {
		identifier := strings.TrimSpace(descriptor.Id)
		descriptors[index].Id = identifier
		if identifier == "" {
			err := common.NewErrBadRequest("AASR-BULK-CREATE-MISSINGID descriptor id must not be empty")
			return asyncbulk.ItemFailure{
				Index:      index,
				Identifier: identifier,
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			}
		}
		if _, found := seen[identifier]; found {
			err := common.NewErrConflict("AAS with given id occurs multiple times in bulk request")
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

	_, getErr := s.aasRegistryBackend.GetAssetAdministrationShellDescriptorByIDInTransaction(auth.WithoutQueryFilter(ctx), tx, descriptorID)
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

func firstIdentifier(identifiers []string) string {
	if len(identifiers) == 0 {
		return ""
	}
	return identifiers[0]
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
