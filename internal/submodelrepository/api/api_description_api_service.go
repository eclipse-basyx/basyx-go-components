/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.1.1_SSP-001
 * Contact: info@idtwin.org
 */

package api

import (
	"context"
	"net/http"

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

// GetSelfDescription - Returns the self-describing information of a network resource (ServiceDescription)
func (s *DescriptionAPIAPIService) GetSelfDescription(_ context.Context) (model.ImplResponse, error) {
	sd := model.ServiceDescription{
		Profiles: []string{
			"https://admin-shell.io/aas/API/3/1/SubmodelRepositoryServiceSpecification/SSP-001",
			"https://basyx.org/aas/go-server/API/SubmodelRepositoryService/1.0",
		},
	}

	return model.Response(http.StatusOK, sd), nil
}
