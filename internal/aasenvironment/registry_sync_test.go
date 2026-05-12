package aasenvironment

import (
	"context"
	"database/sql"
	"testing"

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/types"
	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	smregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/persistence"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	"github.com/stretchr/testify/require"
)

func TestNewRegistrySyncConfigRequiresExternalURLWhenEnabled(t *testing.T) {
	_, err := NewRegistrySyncConfig(true, false, "")
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err))
	require.Contains(t, err.Error(), externalURLKey)
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
	aasRepository, repoErr := aasrepositorydb.NewAssetAdministrationShellDatabaseFromDB(db, false)
	require.NoError(t, repoErr)
	aasRegistry, aasRegistryErr := aasregistrydb.NewPostgreSQLAASRegistryDatabaseFromDB(db, false)
	require.NoError(t, aasRegistryErr)
	submodelRepository, submodelRepoErr := submodelrepositorydb.NewSubmodelDatabaseFromDB(db, nil, false)
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

func TestCustomSubmodelRepositoryServiceSyncReferencingAASDescriptorsGuardsMissingDependencies(t *testing.T) {
	service := &CustomSubmodelRepositoryService{enableAASDescriptorEmbeddingSync: true}
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
	aasRepository, repoErr := aasrepositorydb.NewAssetAdministrationShellDatabaseFromDB(db, false)
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
