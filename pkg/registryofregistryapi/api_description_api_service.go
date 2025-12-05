package registryofregistriesapi

import (
	"context"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// DescriptionAPIAPIService is a service that implements the logic for the DescriptionAPIAPIServicer
// This service should implement the business logic for every endpoint for the DescriptionAPIAPI API.
// Include any external packages or services that will be required by this service.
type DescriptionAPIAPIService struct {
}

// NewDescriptionAPIAPIService creates a default api service
func NewDescriptionAPIAPIService() *DescriptionAPIAPIService {
	return &DescriptionAPIAPIService{}
}

// GetDescription - Returns the self-describing information of a network resource (ServiceDescription)
func (s *DescriptionAPIAPIService) GetDescription(ctx context.Context) (model.ImplResponse, error) {
	return model.Response(200, model.ServiceDescription{
		Profiles: []string{"TODO"},
	}), nil
}
