package digitaltwinregistry

import (
	"context"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	discoveryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
)

// CustomDiscoveryService wraps the default discovery service to allow custom logic.
type CustomDiscoveryService struct {
	*discoveryapiinternal.AssetAdministrationShellBasicDiscoveryAPIAPIService
}

// NewCustomDiscoveryService constructs a custom discovery service wrapper.
func NewCustomDiscoveryService(base *discoveryapiinternal.AssetAdministrationShellBasicDiscoveryAPIAPIService) *CustomDiscoveryService {
	return &CustomDiscoveryService{AssetAdministrationShellBasicDiscoveryAPIAPIService: base}
}

// Example override: add custom logic before delegating.
func (s *CustomDiscoveryService) SearchAllAssetAdministrationShellIdsByAssetLink(
	ctx context.Context,
	limit int32,
	cursor string,
	assetLink []model.AssetLink,
) (model.ImplResponse, error) {
	// TODO: add custom logic here for /lookup/shellsByAssetLink (aka shellsByAssetIds)
	// Example: enforce a max limit, audit, or modify assetLink before delegating.
	return s.AssetAdministrationShellBasicDiscoveryAPIAPIService.SearchAllAssetAdministrationShellIdsByAssetLink(ctx, limit, cursor, assetLink)
}

// Custom logic for /lookup/shells/{aasIdentifier}
func (s *CustomDiscoveryService) GetAllAssetLinksByID(
	ctx context.Context,
	aasIdentifier string,
) (model.ImplResponse, error) {
	// TODO: add custom logic here (validation, enrichment, access checks, etc.)
	return s.AssetAdministrationShellBasicDiscoveryAPIAPIService.GetAllAssetLinksByID(ctx, aasIdentifier)
}
