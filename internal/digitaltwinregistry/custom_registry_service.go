package digitaltwinregistry

import (
	"context"

	registryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// CustomRegistryService wraps the default registry service to allow custom logic.
type CustomRegistryService struct {
	*registryapiinternal.AssetAdministrationShellRegistryAPIAPIService
}

// NewCustomRegistryService constructs a custom registry service wrapper.
func NewCustomRegistryService(base *registryapiinternal.AssetAdministrationShellRegistryAPIAPIService) *CustomRegistryService {
	return &CustomRegistryService{AssetAdministrationShellRegistryAPIAPIService: base}
}

// Example override: add custom logic before delegating.
func (s *CustomRegistryService) PostAssetAdministrationShellDescriptor(
	ctx context.Context,
	desc model.AssetAdministrationShellDescriptor,
) (model.ImplResponse, error) {
	// TODO: add custom logic here (validation, audit, enrichment, etc.)
	return s.AssetAdministrationShellRegistryAPIAPIService.PostAssetAdministrationShellDescriptor(ctx, desc)
}
