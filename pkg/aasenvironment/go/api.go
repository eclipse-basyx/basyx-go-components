// Package openapi AAS Environment API
package openapi

import (
	"context"
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// DescriptionAPIAPIRouter defines the required methods for binding the api requests to a responses for the DescriptionAPIAPI
// The DescriptionAPIAPIRouter implementation should parse necessary information from the http request,
// pass the data to a DescriptionAPIAPIServicer to perform the required actions, then write the service results to the http response.
type DescriptionAPIAPIRouter interface {
	GetSelfDescription(http.ResponseWriter, *http.Request)
}

// SerializationAPIAPIRouter defines the required methods for binding the api requests to a responses for the SerializationAPIAPI
// The SerializationAPIAPIRouter implementation should parse necessary information from the http request,
// pass the data to a SerializationAPIAPIServicer to perform the required actions, then write the service results to the http response.
type SerializationAPIAPIRouter interface {
	GenerateSerializationByIDs(http.ResponseWriter, *http.Request)
}

// DescriptionAPIAPIServicer defines the api actions for the DescriptionAPIAPI service
// This interface intended to stay up to date with the openapi yaml used to generate it,
// while the service implementation can be ignored with the .openapi-generator-ignore file
// and updated with the logic required for the API.
type DescriptionAPIAPIServicer interface {
	GetSelfDescription(context.Context) (model.ImplResponse, error)
}

// SerializationAPIAPIServicer defines the api actions for the SerializationAPIAPI service
// This interface intended to stay up to date with the openapi yaml used to generate it,
// while the service implementation can be ignored with the .openapi-generator-ignore file
// and updated with the logic required for the API.
type SerializationAPIAPIServicer interface {
	GenerateSerializationByIDs(context.Context, []string, []string, bool) (model.ImplResponse, error)
}
