package openapi

import (
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestNewSubmodelRepositoryAPIAPIControllerParsesVerificationModeCaseAndWhitespace(t *testing.T) {
	ctrl := NewSubmodelRepositoryAPIAPIController(nil, "", " PerMiSsIvE ")
	if ctrl.verificationMode != model.VerificationModePermissive {
		t.Fatalf("expected permissive verification mode, got %q", ctrl.verificationMode)
	}
}

func TestSubmodelRepositoryRoutesIncludeSignedWriteOperations(t *testing.T) {
	t.Parallel()

	ctrl := NewSubmodelRepositoryAPIAPIController(nil, "/api/v3", "")
	routes := ctrl.Routes()

	require.Equal(t, "/api/v3/submodels/{submodelIdentifier}/$signed", routes["GetSignedSubmodelByID"].Pattern)
	require.Equal(t, http.MethodGet, routes["GetSignedSubmodelByID"].Method)
	require.Equal(t, "/api/v3/submodels/{submodelIdentifier}/$signed", routes["PutSubmodelByIdSigned"].Pattern)
	require.Equal(t, http.MethodPut, routes["PutSubmodelByIdSigned"].Method)
	require.Equal(t, "/api/v3/submodels/{submodelIdentifier}/$signed", routes["PatchSubmodelByIdSigned"].Pattern)
	require.Equal(t, http.MethodPatch, routes["PatchSubmodelByIdSigned"].Method)
	require.Equal(t, "/api/v3/submodels/{submodelIdentifier}/$signed", routes["DeleteSubmodelByIdSigned"].Pattern)
	require.Equal(t, http.MethodDelete, routes["DeleteSubmodelByIdSigned"].Method)
}
