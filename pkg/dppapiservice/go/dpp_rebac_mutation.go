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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/eclipse-basyx/basyx-go-components/internal/registrysync"
)

type dppMutationContextsKey struct{}
type dppMutationContexts map[string]context.Context

func (s *DPPRepositoryService) executeDPPMutation(ctx context.Context, start, commit string, run func(context.Context, *sql.Tx) error) error {
	ctx, finish := auth.BeginReBACMutationAudit(ctx)
	ctx = context.WithValue(ctx, dppMutationContextsKey{}, dppMutationContexts{})
	err := s.aasRepo.ExecuteInTransaction(start, commit, func(tx *sql.Tx) error { return run(ctx, tx) })
	return finish(err)
}

func authorizeDPPMutation(ctx context.Context, tx *sql.Tx, kind, id, method string) (context.Context, error) {
	authorized, err := auth.AuthorizeReBACResource(ctx, tx, kind, id, method)
	if err != nil {
		return ctx, err
	}
	if contexts, ok := ctx.Value(dppMutationContextsKey{}).(dppMutationContexts); ok {
		contexts[kind+":"+id] = authorized
	}
	return authorized, nil
}

func dppDerivedMutationContext(ctx context.Context, tx *sql.Tx, kind, id, targetKind string) (context.Context, error) {
	contexts, _ := ctx.Value(dppMutationContextsKey{}).(dppMutationContexts)
	source, ok := contexts[kind+":"+id]
	if !ok {
		var err error
		source, err = authorizeDPPMutation(ctx, tx, kind, id, http.MethodPut)
		if err != nil {
			return ctx, err
		}
	}
	return auth.ContextWithReBACDerivedTarget(source, kind, id, targetKind, id), nil
}

func (s *DPPRepositoryService) createDPPInTransaction(ctx context.Context, tx *sql.Tx, aas types.IAssetAdministrationShell, submodels []types.ISubmodel) error {
	aasContext, err := authorizeDPPMutation(ctx, tx, "aas", aas.ID(), http.MethodPut)
	if err != nil {
		return err
	}
	if err = s.aasRepo.CreateAssetAdministrationShellInTransaction(aasContext, tx, aas); err != nil {
		return fmt.Errorf("DPP-CREATEDPP-CREATEAAS create AAS: %w", err)
	}
	for _, submodel := range submodels {
		smContext, err := authorizeDPPMutation(ctx, tx, "submodel", submodel.ID(), http.MethodPut)
		if err != nil {
			return err
		}
		if err = s.submodelRepo.CreateSubmodelInTransaction(smContext, tx, submodel); err != nil {
			return fmt.Errorf("DPP-CREATEDPP-CREATESUBMODEL create submodel: %w", err)
		}
	}
	return s.syncCreatedDescriptors(ctx, tx, aas, submodels)
}

func (s *DPPRepositoryService) syncCreatedReBACDescriptors(ctx context.Context, tx *sql.Tx, aas types.IAssetAdministrationShell, submodels []types.ISubmodel) error {
	if s.registrySyncConfig.SubmodelRegistryIntegration {
		for _, submodel := range submodels {
			descriptor, err := s.registrySyncConfig.BuildSubmodelDescriptor(submodel)
			if err != nil {
				return err
			}
			derived, err := dppDerivedMutationContext(ctx, tx, "submodel", submodel.ID(), "submodel_descriptor")
			if err != nil {
				return err
			}
			if _, err = s.submodelRegistry.InsertSubmodelDescriptorsInTransaction(registrysync.WithSubmodelRegistrySyncUpsertAudit(derived), tx, []commonmodel.SubmodelDescriptor{descriptor}); err != nil {
				return err
			}
		}
	}
	if !s.registrySyncConfig.AASRegistryIntegration {
		return nil
	}
	descriptor, err := s.registrySyncConfig.BuildAASDescriptor(aas)
	if err != nil {
		return err
	}
	derived, err := dppDerivedMutationContext(ctx, tx, "aas", aas.ID(), "aas_descriptor")
	if err != nil {
		return err
	}
	return s.aasRegistry.InsertAdministrationShellDescriptorInTransaction(registrysync.WithAASRegistrySyncUpsertAudit(derived), tx, descriptor)
}

func refreshDPPAfterMutation(ctx context.Context) (context.Context, error) {
	if !dppReBACEnabled(ctx) {
		return ctx, nil
	}
	cfg, _ := common.ConfigFromContext(ctx)
	timeout := time.Duration(cfg.ReBAC.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	refreshContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		refreshed, err := auth.RefreshReBACExecutionContext(ctx)
		if err == nil {
			return refreshed, nil
		}
		if !common.IsErrServiceUnavailable(err) {
			return ctx, err
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-refreshContext.Done():
			timer.Stop()
			return ctx, common.NewErrServiceUnavailable("DPP-REBAC-SYNCHRONIZATION response awaits authorization synchronization")
		case <-timer.C:
		}
	}
}

func (s *DPPRepositoryService) persistDPPElementMutation(ctx context.Context, tx *sql.Tx, submodelID, idShortPath string, element types.ISubmodelElement, metadata types.ISubmodel) error {
	elementContext, err := auth.AuthorizeReBACMutationElement(ctx, tx, submodelID, idShortPath, http.MethodPut)
	if err != nil {
		return err
	}
	metadataContext, err := authorizeDPPMutation(ctx, tx, "submodel", metadata.ID(), http.MethodPut)
	if err != nil {
		return err
	}
	if _, err := s.submodelRepo.PutSubmodelElementInTransaction(elementContext, tx, submodelID, idShortPath, element); err != nil {
		return fmt.Errorf("DPP-UPDELEM-PUTELEMENT put element %s: %w", idShortPath, err)
	}
	putResult, putErr := s.submodelRepo.PutSubmodelInTransactionWithResult(metadataContext, tx, metadata.ID(), metadata)
	if putErr != nil {
		return fmt.Errorf("DPP-UPDELEM-PUTMETADATA put metadata: %w", putErr)
	}
	return s.syncUpdatedDescriptors(ctx, tx, nil, nil, []submodelDescriptorUpdate{{
		previous: putResult.Previous, submitted: metadata,
	}})
}
