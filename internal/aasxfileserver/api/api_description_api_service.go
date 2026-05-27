package api

import (
	"context"
	"net/http"

	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasxfileserverapi/go"
)

// DescriptionAPIAPIService provides the static self-description response.
type DescriptionAPIAPIService struct{}

// NewDescriptionAPIAPIService creates a new description service.
func NewDescriptionAPIAPIService() *DescriptionAPIAPIService {
	return &DescriptionAPIAPIService{}
}

// GetSelfDescription returns the supported profile for the AASX file server.
func (s *DescriptionAPIAPIService) GetSelfDescription(ctx context.Context) (openapi.ImplResponse, error) {
	_ = ctx
	return openapi.Response(http.StatusOK, openapi.ServiceDescription{
		Profiles: []string{
			"https://admin-shell.io/aas/API/3/2/AasxFileServerServiceSpecification/SSP-001",
		},
	}), nil
}
