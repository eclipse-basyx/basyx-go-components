package digitaltwinregistry

import (
	"context"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const (
	discoveryProfile = "https://admin-shell.io/aas/API/3/1/DiscoveryServiceSpecification/SSP-001"
	registryProfile  = "https://admin-shell.io/aas/API/3/1/AssetAdministrationShellRegistryServiceSpecification/SSP-001"
)

// DescriptionService provides the combined service description for the Digital Twin Registry.
type DescriptionService struct{}

// NewDescriptionService constructs the description service.
func NewDescriptionService() *DescriptionService {
	return &DescriptionService{}
}

// GetDescription - Returns the self-describing information of the Digital Twin Registry.
func (s *DescriptionService) GetDescription(ctx context.Context) (model.ImplResponse, error) {
	_ = ctx
	return model.Response(200, model.ServiceDescription{
		Profiles: []string{registryProfile, discoveryProfile},
	}), nil
}
