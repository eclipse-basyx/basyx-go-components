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
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// AssetAdministrationShellBasicDiscoveryAPIAPIRouter defines the required methods for binding the api requests to a responses for the AssetAdministrationShellBasicDiscoveryAPIAPI
// The AssetAdministrationShellBasicDiscoveryAPIAPIRouter implementation should parse necessary information from the http request,
// pass the data to a AssetAdministrationShellBasicDiscoveryAPIAPIServicer to perform the required actions, then write the service results to the http response.
type AssetAdministrationShellBasicDiscoveryAPIAPIRouter interface {
	GetAllAssetAdministrationShellIdsByAssetLink(http.ResponseWriter, *http.Request)
	GetAllAssetLinksById(http.ResponseWriter, *http.Request)
	PostAllAssetLinksById(http.ResponseWriter, *http.Request)
	DeleteAllAssetLinksById(http.ResponseWriter, *http.Request)
}

// DescriptionAPIAPIRouter defines the required methods for binding the api requests to a responses for the DescriptionAPIAPI
// The DescriptionAPIAPIRouter implementation should parse necessary information from the http request,
// pass the data to a DescriptionAPIAPIServicer to perform the required actions, then write the service results to the http response.
type DescriptionAPIAPIRouter interface {
	GetDescription(http.ResponseWriter, *http.Request)
}

// AssetAdministrationShellBasicDiscoveryAPIAPIServicer defines the api actions for the AssetAdministrationShellBasicDiscoveryAPIAPI service
// This interface intended to stay up to date with the openapi yaml used to generate it,
// while the service implementation can be ignored with the .openapi-generator-ignore file
// and updated with the logic required for the API.
type AssetAdministrationShellBasicDiscoveryAPIAPIServicer interface {
	GetAllAssetAdministrationShellIdsByAssetLink(context.Context, []string, int32, string) (model.ImplResponse, error)
	GetAllAssetLinksById(context.Context, string) (model.ImplResponse, error)
	PostAllAssetLinksById(context.Context, string, []SpecificAssetId) (model.ImplResponse, error)
	DeleteAllAssetLinksById(context.Context, string) (model.ImplResponse, error)
}

// DescriptionAPIAPIServicer defines the api actions for the DescriptionAPIAPI service
// This interface intended to stay up to date with the openapi yaml used to generate it,
// while the service implementation can be ignored with the .openapi-generator-ignore file
// and updated with the logic required for the API.
type DescriptionAPIAPIServicer interface {
	GetDescription(context.Context) (model.ImplResponse, error)
}
