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
	"net/http"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	smregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/persistence"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	"github.com/stretchr/testify/require"
)

func TestNewRegistrySyncConfigAllowsBlankExternalURLWhenEnabled(t *testing.T) {
	config, err := NewRegistrySyncConfig(true, false, "")
	require.NoError(t, err)
	require.True(t, config.AASRegistryIntegration)
	require.Empty(t, config.ExternalBaseURLs)
}

func TestNewRegistrySyncConfigParsesAndNormalizesExternalURLs(t *testing.T) {
	config, err := NewRegistrySyncConfig(
		true,
		true,
		" https://public.example/api/v3/ , https://internal.example/api/v3,https://public.example/api/v3 ",
	)
	require.NoError(t, err)
	require.Equal(
		t,
		[]string{
			"https://public.example/api/v3",
			"https://internal.example/api/v3",
		},
		config.ExternalBaseURLs,
	)
}

func TestNewRegistrySyncConfigRejectsUnsupportedScheme(t *testing.T) {
	_, err := NewRegistrySyncConfig(true, true, "ftp://public.example/api/v3")
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err))
	require.Contains(t, err.Error(), "http or https")
}

func TestRegistrySyncContextOverridesHTTPAuditButKeepsPreconfigurationAudit(t *testing.T) {
	httpAudit := history.AuditContext{
		ActorSubject:        "user-1",
		ActorIssuer:         "https://issuer.example/realms/basyx",
		ClientID:            "basyx-ui",
		AuthorizationResult: "ALLOW",
		RequestID:           "req-1",
		CorrelationID:       "corr-1",
		Operation:           "PutAssetAdministrationShellById",
		Endpoint:            "/shells/{aasIdentifier}",
		HTTPMethod:          http.MethodPut,
	}
	registryAudit := history.FromContext(aasRegistryAddAuditMetadataIfNotAvailable(
		history.ContextWithAudit(context.TODO(), httpAudit),
		aasRegistrySyncUpsertOperation,
	))
	require.Equal(t, httpAudit.ActorSubject, registryAudit.ActorSubject)
	require.Equal(t, httpAudit.RequestID, registryAudit.RequestID)
	require.Equal(t, httpAudit.CorrelationID, registryAudit.CorrelationID)
	require.Equal(t, httpAudit.HTTPMethod, registryAudit.HTTPMethod)
	require.Equal(t, aasRegistrySyncUpsertOperation, registryAudit.Operation)
	require.Equal(t, aasRegistrySyncEndpoint, registryAudit.Endpoint)

	preconfigurationCtx := ContextWithAASPreconfigurationAudit(context.TODO())
	preconfigurationAudit := history.FromContext(preconfigurationCtx)
	require.NotEmpty(t, preconfigurationAudit.RequestID)
	require.NotEmpty(t, preconfigurationAudit.CorrelationID)
	require.Equal(t, history.AuthorizationResultSystemInternal, preconfigurationAudit.AuthorizationResult)
	require.Equal(t, history.AuditHTTPMethodSystem, preconfigurationAudit.HTTPMethod)

	preconfigurationRegistryAudit := history.FromContext(aasRegistryAddAuditMetadataIfNotAvailable(
		preconfigurationCtx,
		aasRegistrySyncUpsertOperation,
	))
	require.Equal(t, preconfigurationAudit, preconfigurationRegistryAudit)
}

func TestRegistrySyncConfigBuildsDeterministicEndpoints(t *testing.T) {
	config := RegistrySyncConfig{
		ExternalBaseURLs: []string{
			"https://public.example/api/v3",
			"https://internal.example/api/v3",
		},
	}

	aasEndpoints := config.buildAASDescriptorEndpoints("urn:example:aas:001")
	require.Len(t, aasEndpoints, 2)
	require.Equal(t, "https://public.example/api/v3/shells/dXJuOmV4YW1wbGU6YWFzOjAwMQ", aasEndpoints[0].ProtocolInformation.Href)
	require.Equal(t, "https://internal.example/api/v3/shells/dXJuOmV4YW1wbGU6YWFzOjAwMQ", aasEndpoints[1].ProtocolInformation.Href)
	require.Equal(t, "https", aasEndpoints[0].ProtocolInformation.EndpointProtocol)

	submodelEndpoints := config.buildSubmodelDescriptorEndpoints("urn:example:sm:001")
	require.Len(t, submodelEndpoints, 2)
	require.Equal(t, "https://public.example/api/v3/submodels/dXJuOmV4YW1wbGU6c206MDAx", submodelEndpoints[0].ProtocolInformation.Href)
	require.Equal(t, "https://internal.example/api/v3/submodels/dXJuOmV4YW1wbGU6c206MDAx", submodelEndpoints[1].ProtocolInformation.Href)
	require.Equal(t, "https", submodelEndpoints[0].ProtocolInformation.EndpointProtocol)
}

func TestRegistrySyncConfigBuildsDynamicEndpointsFromRequestContext(t *testing.T) {
	config := RegistrySyncConfig{}
	ctx := common.ContextWithRequestExternalBaseURL(context.Background(), "https://public.example/api/v3")

	aasEndpoints := config.buildAASDescriptorEndpointsForContext(ctx, "urn:example:aas:001")
	require.Len(t, aasEndpoints, 1)
	require.Equal(t, "https://public.example/api/v3/shells/dXJuOmV4YW1wbGU6YWFzOjAwMQ", aasEndpoints[0].ProtocolInformation.Href)
	require.Equal(t, "https", aasEndpoints[0].ProtocolInformation.EndpointProtocol)

	submodelEndpoints := config.buildSubmodelDescriptorEndpointsForContext(ctx, "urn:example:sm:001")
	require.Len(t, submodelEndpoints, 1)
	require.Equal(t, "https://public.example/api/v3/submodels/dXJuOmV4YW1wbGU6c206MDAx", submodelEndpoints[0].ProtocolInformation.Href)
}

func TestRegistrySyncConfigDoesNotEmitBlankDynamicEndpoints(t *testing.T) {
	config := RegistrySyncConfig{}

	require.Empty(t, config.buildAASDescriptorEndpointsForContext(context.Background(), "urn:example:aas:001"))
	require.Empty(t, config.buildSubmodelDescriptorEndpointsForContext(context.Background(), "urn:example:sm:001"))
	require.False(t, config.hasEndpointBaseURL(context.Background()))
}

func TestRegistrySyncConfigPrefersStaticExternalURLsOverDynamicContext(t *testing.T) {
	config := RegistrySyncConfig{ExternalBaseURLs: []string{"https://static.example/api/v3"}}
	ctx := common.ContextWithRequestExternalBaseURL(context.Background(), "https://dynamic.example/api/v3")

	endpoints := config.buildAASDescriptorEndpointsForContext(ctx, "urn:example:aas:001")
	require.Len(t, endpoints, 1)
	require.Equal(t, "https://static.example/api/v3/shells/dXJuOmV4YW1wbGU6YWFzOjAwMQ", endpoints[0].ProtocolInformation.Href)
}

func TestDynamicBlankModeSubmodelDescriptorRequiresEndpointBase(t *testing.T) {
	config := RegistrySyncConfig{
		AASRegistryIntegration:      true,
		SubmodelRegistryIntegration: true,
	}
	submodel, err := jsonization.SubmodelFromJsonable(map[string]any{
		"id":        "urn:example:sm:superpath",
		"idShort":   "SuperpathSubmodel",
		"modelType": "Submodel",
	})
	require.NoError(t, err)

	descriptor, err := config.buildSubmodelDescriptorForContext(context.Background(), submodel)
	require.NoError(t, err)
	require.Empty(t, descriptor.Endpoints)
	require.False(t, config.hasEndpointBaseURL(context.Background()))

	ctx := common.ContextWithRequestExternalBaseURL(context.Background(), "https://public.example/api/v3")
	descriptor, err = config.buildSubmodelDescriptorForContext(ctx, submodel)
	require.NoError(t, err)
	require.Len(t, descriptor.Endpoints, 1)
	require.Equal(
		t,
		"https://public.example/api/v3/submodels/"+common.EncodeString("urn:example:sm:superpath"),
		descriptor.Endpoints[0].ProtocolInformation.Href,
	)
	require.True(t, config.hasEndpointBaseURL(ctx))
}

func TestDynamicRegistryReconciliationStateSkipsDuplicateRuns(t *testing.T) {
	var state dynamicRegistryReconciliationState

	require.True(t, state.reserve("https://public.example/api/v3"))
	require.False(t, state.reserve("https://public.example/api/v3"))

	state.complete("https://public.example/api/v3", false)
	require.True(t, state.reserve("https://public.example/api/v3"))

	state.complete("https://public.example/api/v3", true)
	require.False(t, state.reserve("https://public.example/api/v3"))
}

func TestBuildAASDescriptorDerivesFromIdentifiable(t *testing.T) {
	config := RegistrySyncConfig{ExternalBaseURLs: []string{"https://public.example/api/v3"}}
	aas, err := jsonization.AssetAdministrationShellFromJsonable(map[string]any{
		"id":        "urn:example:aas:derived",
		"idShort":   "DerivedAAS",
		"modelType": "AssetAdministrationShell",
		"submodels": []any{
			map[string]any{
				"type": "ModelReference",
				"keys": []any{
					map[string]any{
						"type":  "Submodel",
						"value": "urn:example:sm:derived-embedded",
					},
				},
			},
		},
		"administration": map[string]any{
			"version":  "1",
			"revision": "2",
		},
		"assetInformation": map[string]any{
			"assetKind":     "Instance",
			"globalAssetId": "urn:example:asset:derived",
			"specificAssetIds": []any{
				map[string]any{
					"name":  "serialNumber",
					"value": "SN-001",
				},
			},
		},
	})
	require.NoError(t, err)

	descriptor, err := config.buildAASDescriptor(aas)
	require.NoError(t, err)
	require.Equal(t, "urn:example:aas:derived", descriptor.Id)
	require.Equal(t, "DerivedAAS", descriptor.IdShort)
	require.Equal(t, "urn:example:asset:derived", descriptor.GlobalAssetId)
	adminInfo, ok := descriptor.Administration.(*types.AdministrativeInformation)
	require.True(t, ok)
	require.Equal(t, "1", *adminInfo.Version())
	require.Equal(t, "2", *adminInfo.Revision())
	require.NotNil(t, descriptor.AssetKind)
	require.Equal(t, aas.AssetInformation().AssetKind(), *descriptor.AssetKind)
	require.Len(t, descriptor.SpecificAssetIds, 1)
	require.Equal(t, "serialNumber", descriptor.SpecificAssetIds[0].Name())
	require.Equal(t, "SN-001", descriptor.SpecificAssetIds[0].Value())
	require.Len(t, descriptor.SubmodelDescriptors, 1)
	require.Equal(t, "urn:example:sm:derived-embedded", descriptor.SubmodelDescriptors[0].Id)
	require.Len(t, descriptor.SubmodelDescriptors[0].Endpoints, 1)
	require.Equal(
		t,
		"https://public.example/api/v3/submodels/"+common.EncodeString("urn:example:sm:derived-embedded"),
		descriptor.SubmodelDescriptors[0].Endpoints[0].ProtocolInformation.Href,
	)
	require.Len(t, descriptor.Endpoints, 1)
	require.Equal(
		t,
		"https://public.example/api/v3/shells/"+common.EncodeString("urn:example:aas:derived"),
		descriptor.Endpoints[0].ProtocolInformation.Href,
	)
	require.Equal(t, "https", descriptor.Endpoints[0].ProtocolInformation.EndpointProtocol)
}

func TestBuildSubmodelDescriptorDerivesFromIdentifiable(t *testing.T) {
	config := RegistrySyncConfig{ExternalBaseURLs: []string{"http://public.example/api/v3"}}
	submodel, err := jsonization.SubmodelFromJsonable(map[string]any{
		"id":        "urn:example:sm:derived",
		"idShort":   "DerivedSubmodel",
		"kind":      "Instance",
		"modelType": "Submodel",
		"administration": map[string]any{
			"version":  "7",
			"revision": "8",
		},
		"semanticId": map[string]any{
			"type": "ModelReference",
			"keys": []any{
				map[string]any{
					"type":  "Submodel",
					"value": "urn:example:semantic:derived",
				},
			},
		},
		"submodelElements": []any{},
	})
	require.NoError(t, err)

	descriptor, err := config.buildSubmodelDescriptor(submodel)
	require.NoError(t, err)
	require.Equal(t, "urn:example:sm:derived", descriptor.Id)
	require.Equal(t, "DerivedSubmodel", descriptor.IdShort)
	smAdminInfo, ok := descriptor.Administration.(*types.AdministrativeInformation)
	require.True(t, ok)
	require.Equal(t, "7", *smAdminInfo.Version())
	require.Equal(t, "8", *smAdminInfo.Revision())
	require.NotNil(t, descriptor.SemanticId)
	require.Len(t, descriptor.Endpoints, 1)
	require.Equal(
		t,
		"http://public.example/api/v3/submodels/"+common.EncodeString("urn:example:sm:derived"),
		descriptor.Endpoints[0].ProtocolInformation.Href,
	)
	require.Equal(t, "http", descriptor.Endpoints[0].ProtocolInformation.EndpointProtocol)
}

func TestBuildEmbeddedSubmodelDescriptorsSkipsNilReferencesAndKeys(t *testing.T) {
	config := RegistrySyncConfig{ExternalBaseURLs: []string{"https://public.example/api/v3"}}

	references := []types.IReference{
		nil,
		types.NewReference(
			types.ReferenceTypesModelReference,
			[]types.IKey{
				nil,
				types.NewKey(types.KeyTypesAssetAdministrationShell, "urn:example:aas:ignored"),
				types.NewKey(types.KeyTypesSubmodel, " "),
				types.NewKey(types.KeyTypesSubmodel, "urn:example:sm:nil-guard"),
			},
		),
	}

	descriptors := config.buildEmbeddedSubmodelDescriptors(references)
	require.Len(t, descriptors, 1)
	require.Equal(t, "urn:example:sm:nil-guard", descriptors[0].Id)
	require.Len(t, descriptors[0].Endpoints, 1)
	require.Equal(
		t,
		"https://public.example/api/v3/submodels/"+common.EncodeString("urn:example:sm:nil-guard"),
		descriptors[0].Endpoints[0].ProtocolInformation.Href,
	)
}

func TestValidateStandaloneAASRepositoryRegistrySyncConfig(t *testing.T) {
	err := ValidateStandaloneAASRepositoryRegistrySyncConfig(nil)
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err))

	cfg := &common.Config{}
	err = ValidateStandaloneAASRepositoryRegistrySyncConfig(cfg)
	require.NoError(t, err)

	cfg.General.SubmodelRegistryIntegration = true
	err = ValidateStandaloneAASRepositoryRegistrySyncConfig(cfg)
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err))
	require.Contains(t, err.Error(), "general.submodelRegistryIntegration")
}

func TestValidateStandaloneSubmodelRepositoryRegistrySyncConfig(t *testing.T) {
	err := ValidateStandaloneSubmodelRepositoryRegistrySyncConfig(nil)
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err))

	cfg := &common.Config{}
	err = ValidateStandaloneSubmodelRepositoryRegistrySyncConfig(cfg)
	require.NoError(t, err)

	cfg.General.AASRegistryIntegration = true
	err = ValidateStandaloneSubmodelRepositoryRegistrySyncConfig(cfg)
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err))
	require.Contains(t, err.Error(), "general.aasRegistryIntegration")
}

func TestCustomAASRepositoryServiceValidateSyncDependencies(t *testing.T) {
	var nilService *CustomAASRepositoryService
	err := nilService.validateSyncDependencies(false, false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AASENV-AASREPO-CHECKDEPS-NILSERVICE")

	service := &CustomAASRepositoryService{}
	err = service.validateSyncDependencies(false, false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AASENV-AASREPO-CHECKDEPS-NILPERSISTENCE")

	service.persistence = &Persistence{}
	err = service.validateSyncDependencies(false, false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AASENV-AASREPO-CHECKDEPS-NILAASREPO")

	db := &sql.DB{}
	aasRepository, repoErr := aasrepositorydb.NewAssetAdministrationShellDatabaseFromDB(db, string(commonmodel.VerificationModeOff))
	require.NoError(t, repoErr)
	aasRegistry, aasRegistryErr := aasregistrydb.NewPostgreSQLAASRegistryDatabaseFromDB(db, false)
	require.NoError(t, aasRegistryErr)
	submodelRepository, submodelRepoErr := submodelrepositorydb.NewSubmodelDatabaseFromDB(db, nil, string(commonmodel.VerificationModeOff))
	require.NoError(t, submodelRepoErr)
	submodelRegistry, submodelRegistryErr := smregistrydb.NewPostgreSQLSMBackendFromDB(db)
	require.NoError(t, submodelRegistryErr)

	service.persistence = &Persistence{
		AASRepository:      aasRepository,
		AASRegistry:        aasRegistry,
		SubmodelRepository: submodelRepository,
		SubmodelRegistry:   submodelRegistry,
	}

	err = service.validateSyncDependencies(true, true, true)
	require.NoError(t, err)
}

func TestCustomSubmodelRepositoryServiceValidateSyncDependencies(t *testing.T) {
	var nilService *CustomSubmodelRepositoryService
	err := nilService.validateSyncDependencies(false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AASENV-SMREPO-CHECKDEPS-NILSERVICE")

	service := &CustomSubmodelRepositoryService{}
	err = service.validateSyncDependencies(false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AASENV-SMREPO-CHECKDEPS-NILPERSISTENCE")

	service.persistence = &Persistence{}
	err = service.validateSyncDependencies(false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AASENV-SMREPO-CHECKDEPS-NILSMREPO")

	db := &sql.DB{}
	aasRepository, aasRepoErr := aasrepositorydb.NewAssetAdministrationShellDatabaseFromDB(db, string(commonmodel.VerificationModeOff))
	require.NoError(t, aasRepoErr)
	aasRegistry, aasRegistryErr := aasregistrydb.NewPostgreSQLAASRegistryDatabaseFromDB(db, false)
	require.NoError(t, aasRegistryErr)
	submodelRepository, submodelRepoErr := submodelrepositorydb.NewSubmodelDatabaseFromDB(db, nil, string(commonmodel.VerificationModeOff))
	require.NoError(t, submodelRepoErr)
	submodelRegistry, submodelRegistryErr := smregistrydb.NewPostgreSQLSMBackendFromDB(db)
	require.NoError(t, submodelRegistryErr)

	service.persistence = &Persistence{SubmodelRepository: submodelRepository}
	err = service.validateSyncDependencies(false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AASENV-SMREPO-CHECKDEPS-NILSMREGISTRY")

	service.persistence = &Persistence{
		SubmodelRepository: submodelRepository,
		SubmodelRegistry:   submodelRegistry,
	}
	err = service.validateSyncDependencies(true, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AASENV-SMREPO-CHECKDEPS-NILAASREPO")

	service.persistence = &Persistence{
		AASRepository:      aasRepository,
		SubmodelRepository: submodelRepository,
		SubmodelRegistry:   submodelRegistry,
	}
	err = service.validateSyncDependencies(true, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AASENV-SMREPO-CHECKDEPS-NILAASREGISTRY")

	service.persistence = &Persistence{
		AASRepository:      aasRepository,
		AASRegistry:        aasRegistry,
		SubmodelRepository: submodelRepository,
		SubmodelRegistry:   submodelRegistry,
	}
	err = service.validateSyncDependencies(true, true)
	require.NoError(t, err)
}

func TestCustomSubmodelRepositoryServiceSyncReferencingAASDescriptorsGuardsMissingDependencies(t *testing.T) {
	service := &CustomSubmodelRepositoryService{enableReferencingAASDescriptorEmbeddingSync: true}
	err := service.syncReferencingAASDescriptorsInTransaction(
		context.Background(),
		nil,
		commonmodel.SubmodelDescriptor{Id: "urn:example:submodel:guard"},
		nil,
		false,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AASENV-SMREPO-SYNCAAS-NILPERSISTENCE")

	db := &sql.DB{}
	aasRepository, repoErr := aasrepositorydb.NewAssetAdministrationShellDatabaseFromDB(db, string(commonmodel.VerificationModeOff))
	require.NoError(t, repoErr)

	service.persistence = &Persistence{AASRepository: aasRepository}
	err = service.syncReferencingAASDescriptorsInTransaction(
		context.Background(),
		nil,
		commonmodel.SubmodelDescriptor{Id: "urn:example:submodel:guard"},
		nil,
		false,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AASENV-SMREPO-SYNCAAS-NILAASREGISTRY")
}
