package aasenvironment

import (
	"testing"

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
}
