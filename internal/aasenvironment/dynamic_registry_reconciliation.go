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
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

const dynamicRegistryReconciliationPageSize int32 = 100

type dynamicRegistryReconciliationState struct {
	mu              sync.Mutex
	reconcilingBase string
	reconciledBase  string
}

func (s *dynamicRegistryReconciliationState) reserve(externalBaseURL string) bool {
	if externalBaseURL == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reconciledBase == externalBaseURL || s.reconcilingBase == externalBaseURL {
		return false
	}

	s.reconcilingBase = externalBaseURL
	return true
}

func (s *dynamicRegistryReconciliationState) complete(externalBaseURL string, succeeded bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if succeeded {
		s.reconciledBase = externalBaseURL
	}
	if s.reconcilingBase == externalBaseURL {
		s.reconcilingBase = ""
	}
}

type dynamicRegistryReconciler interface {
	triggerDynamicRegistryReconciliation(ctx context.Context)
}

// DynamicRegistryReconciliationMiddleware triggers dynamic registry descriptor reconciliation after a trusted request base URL is known.
func DynamicRegistryReconciliationMiddleware(reconcilers ...dynamicRegistryReconciler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if common.RequestExternalBaseURLFromContext(r.Context()) != "" {
				for _, reconciler := range reconcilers {
					if reconciler != nil {
						reconciler.triggerDynamicRegistryReconciliation(r.Context())
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *CustomAASRepositoryService) triggerDynamicRegistryReconciliation(ctx context.Context) {
	reconciliationCtx := dynamicRegistryReconciliationContext(ctx)
	if reconciliationCtx == nil {
		return
	}
	externalBaseURL := s.dynamicExternalBaseURL(reconciliationCtx)
	if !s.dynamicReconciliationState.reserve(externalBaseURL) {
		return
	}

	go func() {
		err := s.reconcileDynamicRegistryDescriptors(reconciliationCtx)
		s.dynamicReconciliationState.complete(externalBaseURL, err == nil)
		if err != nil {
			log.Printf("AASENV-DYNREGRECON-AASERR dynamic registry reconciliation failed: %v", err)
		}
	}()
}

func (s *CustomSubmodelRepositoryService) triggerDynamicRegistryReconciliation(ctx context.Context) {
	reconciliationCtx := dynamicRegistryReconciliationContext(ctx)
	if reconciliationCtx == nil {
		return
	}
	externalBaseURL := s.dynamicExternalBaseURL(reconciliationCtx)
	if !s.dynamicReconciliationState.reserve(externalBaseURL) {
		return
	}

	go func() {
		err := s.reconcileDynamicRegistryDescriptors(reconciliationCtx)
		s.dynamicReconciliationState.complete(externalBaseURL, err == nil)
		if err != nil {
			log.Printf("AASENV-DYNREGRECON-SMERR dynamic registry reconciliation failed: %v", err)
		}
	}()
}

func dynamicRegistryReconciliationContext(ctx context.Context) context.Context {
	externalBaseURL := common.RequestExternalBaseURLFromContext(ctx)
	if externalBaseURL == "" {
		return nil
	}

	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || cfg == nil {
		return nil
	}

	internalCtx := context.WithoutCancel(ctx)
	internalCtx = auth.ContextWithoutQueryFilter(internalCtx)
	internalCtx = common.ContextWithConfig(internalCtx, cfg)
	internalCtx = common.ContextWithRequestExternalBaseURL(internalCtx, externalBaseURL)
	return ContextWithAASPreconfigurationAudit(internalCtx)
}

func (s *CustomAASRepositoryService) reconcileDynamicRegistryDescriptors(ctx context.Context) error {
	externalBaseURL := s.dynamicExternalBaseURL(ctx)
	if externalBaseURL == "" {
		return nil
	}

	if s.syncConfig.AASRegistryIntegration {
		if err := s.reconcileDynamicAASDescriptors(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (s *CustomAASRepositoryService) dynamicExternalBaseURL(ctx context.Context) string {
	if len(s.syncConfig.ExternalBaseURLs) > 0 {
		return ""
	}
	return common.RequestExternalBaseURLFromContext(ctx)
}

func (s *CustomAASRepositoryService) reconcileDynamicAASDescriptors(ctx context.Context) error {
	if dependencyErr := s.validateSyncDependencies(true, false, false); dependencyErr != nil {
		return dependencyErr
	}

	cursor := ""
	for {
		aasList, nextCursor, getErr := s.persistence.AASRepository.GetAssetAdministrationShells(ctx, dynamicRegistryReconciliationPageSize, cursor, "", nil, time.Time{}, time.Time{})
		if getErr != nil {
			return getErr
		}

		for _, aas := range aasList {
			descriptor, descriptorErr := s.syncConfig.buildAASDescriptorForContext(ctx, aas)
			if descriptorErr != nil {
				return descriptorErr
			}
			if len(descriptor.Endpoints) == 0 {
				return common.NewInternalServerError("AASENV-DYNREGRECON-AASNOENDPOINTS dynamic AAS descriptor endpoints must not be empty")
			}
			if upsertErr := s.ExecuteInTransaction(func(tx *sql.Tx) error {
				return s.persistence.AASRegistry.UpsertAdministrationShellDescriptorInTransaction(
					aasRegistryAddAuditMetadataIfNotAvailable(ctx, aasRegistrySyncUpsertOperation), tx, descriptor,
				)
			}); upsertErr != nil {
				return upsertErr
			}
		}

		if nextCursor == "" {
			return nil
		}
		cursor = nextCursor
	}
}

func (s *CustomSubmodelRepositoryService) reconcileDynamicRegistryDescriptors(ctx context.Context) error {
	externalBaseURL := s.dynamicExternalBaseURL(ctx)
	if externalBaseURL == "" {
		return nil
	}

	return s.reconcileDynamicSubmodelDescriptors(ctx)
}

func (s *CustomSubmodelRepositoryService) dynamicExternalBaseURL(ctx context.Context) string {
	if len(s.syncConfig.ExternalBaseURLs) > 0 {
		return ""
	}
	return common.RequestExternalBaseURLFromContext(ctx)
}

func (s *CustomSubmodelRepositoryService) reconcileDynamicSubmodelDescriptors(ctx context.Context) error {
	if dependencyErr := s.validateSyncDependencies(s.enableReferencingAASDescriptorEmbeddingSync, s.enableReferencingAASDescriptorEmbeddingSync); dependencyErr != nil {
		return dependencyErr
	}

	cursor := ""
	for {
		submodels, nextCursor, getErr := s.persistence.SubmodelRepository.GetSubmodels(ctx, dynamicRegistryReconciliationPageSize, cursor, "", "", time.Time{}, time.Time{})
		if getErr != nil {
			return getErr
		}

		for _, submodel := range submodels {
			descriptor, descriptorErr := s.syncConfig.buildSubmodelDescriptorForContext(ctx, submodel)
			if descriptorErr != nil {
				return descriptorErr
			}
			if len(descriptor.Endpoints) == 0 {
				return common.NewInternalServerError("AASENV-DYNREGRECON-SMNOENDPOINTS dynamic submodel descriptor endpoints must not be empty")
			}
			if upsertErr := s.ExecuteInTransaction(func(tx *sql.Tx) error {
				if err := s.persistence.SubmodelRegistry.UpsertSubmodelDescriptorInTransaction(
					submodelRegistryAddAuditMetadataIfNotAvailable(ctx, submodelRegistrySyncUpsertOperation), tx, descriptor,
				); err != nil {
					return err
				}
				return s.syncReferencingAASDescriptorsInTransaction(ctx, tx, descriptor, nil, false)
			}); upsertErr != nil {
				return upsertErr
			}
		}

		if nextCursor == "" {
			return nil
		}
		cursor = nextCursor
	}
}
