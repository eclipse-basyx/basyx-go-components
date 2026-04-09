package aasenvironment

import (
	"context"
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// DescriptionService exposes a merged profile description for the AAS Environment Service.
type DescriptionService struct{}

// NewDescriptionService creates a new description service.
func NewDescriptionService() *DescriptionService {
	return &DescriptionService{}
}

// GetDescription returns merged service profile metadata for all bundled components.
func (s *DescriptionService) GetDescription(_ context.Context) (model.ImplResponse, error) {
	return model.Response(http.StatusOK, model.ServiceDescription{
		Profiles: mergedProfiles(),
	}), nil
}

func mergedProfiles() []string {
	profiles := []string{
		"https://admin-shell.io/aas/API/3/1/AssetAdministrationShellRegistryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/1/SubmodelRegistryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/1/AssetAdministrationShellRepositoryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/1/SubmodelRepositoryServiceSpecification/SSP-001",
		"https://basyx.org/aas/go-server/API/SubmodelRepositoryService/1.0",
		"https://admin-shell.io/aas/API/3/1/ConceptDescriptionRepositoryServiceSpecification/SSP-001",
		"https://basyx.org/aas/go-server/API/ConceptDescriptionRepositoryService/1.0",
		"https://admin-shell.io/aas/API/3/1/DiscoveryServiceSpecification/SSP-001",
	}

	seen := make(map[string]struct{}, len(profiles))
	result := make([]string, 0, len(profiles))
	for _, p := range profiles {
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		result = append(result, p)
	}
	return result
}
