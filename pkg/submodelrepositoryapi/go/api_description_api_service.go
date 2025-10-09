/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

import (
	"context"
	"errors"
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

// GetDescription - Returns the self-describing information of a network resource (ServiceDescription)
func (s *DescriptionAPIAPIService) GetDescription(ctx context.Context) (model.ImplResponse, error) {
	// TODO - update GetDescription with the required logic for this service method.
	// Add api_description_api_service.go to the .openapi-generator-ignore to avoid overwriting this service implementation when updating open api generation.

	// TODO: Uncomment the next line to return response Response(200, ServiceDescription{}) or use other options such as http.Ok ...
	// return Response(200, ServiceDescription{}), nil

	// TODO: Uncomment the next line to return response Response(401, Result{}) or use other options such as http.Ok ...
	// return Response(401, Result{}), nil

	// TODO: Uncomment the next line to return response Response(403, Result{}) or use other options such as http.Ok ...
	// return Response(403, Result{}), nil

	return model.Response(http.StatusNotImplemented, nil), errors.New("GetDescription method not implemented")
}
