package aasenvironment

import (
	"testing"

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
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
