package api

import (
	"context"
	"errors"
	"net/http"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasrepositoryapi/go"
)

// AASRepositoryService implements the logic for AssetAdministrationShellRepositoryAPIAPIServicer
type AASRepositoryService struct{}

// NewAASRepositoryService creates a new service instance
func NewAASRepositoryService() *AASRepositoryService {
	return &AASRepositoryService{}
}

// === Basic implementation examples ===

// GetDescription - returns static description for now
func (s *AASRepositoryService) GetDescription(_ context.Context) (gen.ImplResponse, error) {
	desc := gen.ServiceDescription{
		Profiles: []string{"AAS Repository V3.1.1"},
	}
	return gen.Response(http.StatusOK, desc), nil
}

// The rest of the AAS Repository endpoints are required by the generated interface.
// These are stub methods to make the project compile and run cleanly.
// You can fill these out later once persistence and real logic are added.

func (s *AASRepositoryService) DeleteAssetAdministrationShellById(ctx context.Context, aasIdentifier string) (openapi.ImplResponse, error) {
	return openapi.Response(http.StatusNotImplemented, nil), errors.New("not implemented")
}

func (s *AASRepositoryService) GetAllAssetAdministrationShells(ctx context.Context, limit int32, cursor string, idShort string) (openapi.ImplResponse, error) {
	return openapi.Response(http.StatusNotImplemented, nil), errors.New("not implemented")
}

func (s *AASRepositoryService) GetAssetAdministrationShellById(ctx context.Context, aasIdentifier string) (openapi.ImplResponse, error) {
	return openapi.Response(http.StatusNotImplemented, nil), errors.New("not implemented")
}

func (s *AASRepositoryService) PostAssetAdministrationShell(ctx context.Context, aas openapi.AssetAdministrationShell) (openapi.ImplResponse, error) {
	return openapi.Response(http.StatusNotImplemented, nil), errors.New("not implemented")
}

func (s *AASRepositoryService) PutAssetAdministrationShellById(ctx context.Context, aasIdentifier string, aas openapi.AssetAdministrationShell) (openapi.ImplResponse, error) {
	return openapi.Response(http.StatusNotImplemented, nil), errors.New("not implemented")
}
