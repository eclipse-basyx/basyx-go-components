/*
 * DotAAS Part 2 | HTTP/REST | Discovery Service Specification
 *
 * The entire Full Profile of the Discovery Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) April 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

import (
	"context"
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
func (s *DescriptionAPIAPIService) GetDescription(ctx context.Context) (ImplResponse, error) {
	return Response(200, ServiceDescription{
		Profiles: []string{"https://admin-shell.io/aas/API/3/0/DiscoveryServiceSpecification/SSP-001"},
	}), nil
}
