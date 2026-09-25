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
	"fmt"

	aastypes "github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func (s *uploadAPIService) processReBACEnvironment(ctx context.Context, environment aastypes.IEnvironment) (err error) {
	ctx, finish := auth.BeginReBACMutationAudit(ctx)
	defer func() { err = finish(err) }()
	return s.persistence.ExecuteInTransaction(
		"AASENV-REBACIMPORT-STARTTX",
		"AASENV-REBACIMPORT-COMMITTX",
		func(tx *sql.Tx) error {
			_, err := s.processReBACEnvironmentInTransaction(ctx, tx, environment)
			return err
		},
	)
}

func (s *uploadAPIService) processReBACEnvironmentInTransaction(ctx context.Context, tx *sql.Tx, environment aastypes.IEnvironment) (map[string]context.Context, error) {
	if tx == nil {
		return nil, common.NewInternalServerError("AASENV-REBACIMPORT-NILTX transaction must not be nil")
	}
	preparedContexts := make(map[string]context.Context)
	if err := s.processReBACConceptDescriptionsInTransaction(ctx, tx, environment, preparedContexts); err != nil {
		return nil, err
	}
	if err := s.processReBACSubmodelsInTransaction(ctx, tx, environment, preparedContexts); err != nil {
		return nil, err
	}
	if err := s.processReBACAASInTransaction(ctx, tx, environment, preparedContexts); err != nil {
		return nil, err
	}
	return preparedContexts, nil
}

func (s *uploadAPIService) processReBACConceptDescriptionsInTransaction(ctx context.Context, tx *sql.Tx, environment aastypes.IEnvironment, preparedContexts map[string]context.Context) error {
	for _, conceptDescription := range environment.ConceptDescriptions() {
		resourceContext, err := auth.AuthorizeReBACResource(ctx, tx, "concept_description", conceptDescription.ID(), "PUT")
		if err != nil {
			return fmt.Errorf("AASENV-REBACIMPORT-AUTHCD failed to authorize concept description %q: %w", conceptDescription.ID(), err)
		}
		preparedContexts[rebacEnvironmentResourceContextKey("concept_description", conceptDescription.ID())] = resourceContext
		if _, err = s.persistence.ConceptDescriptionRepository.PutConceptDescriptionInTransaction(resourceContext, tx, conceptDescription.ID(), conceptDescription); err != nil {
			return fmt.Errorf("AASENV-REBACIMPORT-PUTCD failed to store concept description %q: %w", conceptDescription.ID(), err)
		}
	}
	return nil
}

func (s *uploadAPIService) processReBACSubmodelsInTransaction(ctx context.Context, tx *sql.Tx, environment aastypes.IEnvironment, preparedContexts map[string]context.Context) error {
	for _, submodel := range environment.Submodels() {
		resourceContext, err := auth.AuthorizeReBACResource(ctx, tx, "submodel", submodel.ID(), "PUT")
		if err != nil {
			return fmt.Errorf("AASENV-REBACIMPORT-AUTHSM failed to authorize submodel %q: %w", submodel.ID(), err)
		}
		preparedContexts[rebacEnvironmentResourceContextKey("submodel", submodel.ID())] = resourceContext
		putResult, err := s.persistence.SubmodelRepository.PutSubmodelInTransactionWithResult(resourceContext, tx, submodel.ID(), submodel)
		if err != nil {
			return fmt.Errorf("AASENV-REBACIMPORT-PUTSM failed to store submodel %q: %w", submodel.ID(), err)
		}
		if err = s.syncReBACSubmodelRegistryInTransaction(resourceContext, tx, submodel, putResult.Changed, putResult.Previous); err != nil {
			return err
		}
	}
	return nil
}

func (s *uploadAPIService) processReBACAASInTransaction(ctx context.Context, tx *sql.Tx, environment aastypes.IEnvironment, preparedContexts map[string]context.Context) error {
	for _, aas := range environment.AssetAdministrationShells() {
		resourceContext, err := auth.AuthorizeReBACResource(ctx, tx, "aas", aas.ID(), "PUT")
		if err != nil {
			return fmt.Errorf("AASENV-REBACIMPORT-AUTHAAS failed to authorize AAS %q: %w", aas.ID(), err)
		}
		preparedContexts[rebacEnvironmentResourceContextKey("aas", aas.ID())] = resourceContext
		putResult, err := s.persistence.AASRepository.PutAssetAdministrationShellByIDInTransactionWithResult(resourceContext, tx, aas.ID(), aas)
		if err != nil {
			return fmt.Errorf("AASENV-REBACIMPORT-PUTAAS failed to store AAS %q: %w", aas.ID(), err)
		}
		if err = s.syncReBACAASRegistryInTransaction(resourceContext, tx, aas, putResult.Changed, putResult.Previous); err != nil {
			return err
		}
	}
	return nil
}

func rebacEnvironmentResourceContextKey(kind string, id string) string {
	return kind + ":" + id
}

func (s *uploadAPIService) syncReBACSubmodelRegistryInTransaction(ctx context.Context, tx *sql.Tx, submodel aastypes.ISubmodel, changed bool, previous aastypes.ISubmodel) error {
	service, ok := s.submodelRepositoryService.(*CustomSubmodelRepositoryService)
	if !ok || !service.syncConfig.SubmodelRegistryIntegration || !changed {
		return nil
	}
	if err := service.validateSyncDependencies(service.enableReferencingAASDescriptorEmbeddingSync, service.enableReferencingAASDescriptorEmbeddingSync); err != nil {
		return err
	}
	descriptor, descriptorChanged, err := service.syncConfig.ChangedSubmodelDescriptor(previous, submodel)
	if err != nil {
		return err
	}
	if !descriptorChanged {
		return nil
	}
	registryContext := submodelRegistryAddAuditMetadataIfNotAvailable(
		auth.ContextWithReBACDerivedTarget(ctx, "submodel", submodel.ID(), "submodel_descriptor", descriptor.Id),
		submodelRegistrySyncUpsertOperation,
	)
	if err = s.persistence.SubmodelRegistry.UpsertSubmodelDescriptorInTransaction(registryContext, tx, descriptor); err != nil {
		return fmt.Errorf("AASENV-REBACIMPORT-UPDATESMDESCRIPTOR failed to synchronize submodel descriptor %q: %w", descriptor.Id, err)
	}
	if err = service.syncReferencingAASDescriptorsInTransaction(ctx, tx, descriptor, nil, false); err != nil {
		return fmt.Errorf("AASENV-REBACIMPORT-UPDATEREFERENCINGAAS failed to synchronize AAS descriptors for submodel %q: %w", submodel.ID(), err)
	}
	return nil
}

func (s *uploadAPIService) syncReBACAASRegistryInTransaction(ctx context.Context, tx *sql.Tx, aas aastypes.IAssetAdministrationShell, changed bool, previous aastypes.IAssetAdministrationShell) error {
	service, ok := s.aasRepositoryService.(*CustomAASRepositoryService)
	if !ok || !service.syncConfig.AASRegistryIntegration || !changed {
		return nil
	}
	if err := service.validateSyncDependencies(true, false, false); err != nil {
		return err
	}
	descriptor, descriptorChanged, err := service.syncConfig.ChangedAASDescriptor(previous, aas)
	if err != nil {
		return err
	}
	if !descriptorChanged {
		return nil
	}
	registryContext := aasRegistryAddAuditMetadataIfNotAvailable(
		generatedAASDescriptorContext(ctx, descriptor),
		aasRegistrySyncUpsertOperation,
	)
	if err = s.persistence.AASRegistry.UpsertAdministrationShellDescriptorInTransaction(registryContext, tx, descriptor); err != nil {
		return fmt.Errorf("AASENV-REBACIMPORT-UPDATEAASDESCRIPTOR failed to synchronize AAS descriptor %q: %w", descriptor.Id, err)
	}
	return nil
}
