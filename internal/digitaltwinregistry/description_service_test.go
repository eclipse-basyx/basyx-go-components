package digitaltwinregistry

import (
	"context"
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestDescriptionContainsSSP003Profile(t *testing.T) {
	svc := NewDescriptionService()
	resp, err := svc.GetDescription(context.Background())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Code)

	description, ok := resp.Body.(model.ServiceDescription)
	require.True(t, ok)
	require.Contains(t, description.Profiles, "https://admin-shell.io/aas/API/3/1/AssetAdministrationShellRegistryServiceSpecification/SSP-003")
}
