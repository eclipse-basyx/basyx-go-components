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
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// AssetAdministrationShellBasicDiscoveryAPIAPIController binds http requests to an api service and writes the service results to the http response
type AssetAdministrationShellBasicDiscoveryAPIAPIController struct {
	service      AssetAdministrationShellBasicDiscoveryAPIAPIServicer
	errorHandler ErrorHandler
}

// AssetAdministrationShellBasicDiscoveryAPIAPIOption for how the controller is set up.
type AssetAdministrationShellBasicDiscoveryAPIAPIOption func(*AssetAdministrationShellBasicDiscoveryAPIAPIController)

// WithAssetAdministrationShellBasicDiscoveryAPIAPIErrorHandler inject ErrorHandler into controller
func WithAssetAdministrationShellBasicDiscoveryAPIAPIErrorHandler(h ErrorHandler) AssetAdministrationShellBasicDiscoveryAPIAPIOption {
	return func(c *AssetAdministrationShellBasicDiscoveryAPIAPIController) {
		c.errorHandler = h
	}
}

// NewAssetAdministrationShellBasicDiscoveryAPIAPIController creates a default api controller
func NewAssetAdministrationShellBasicDiscoveryAPIAPIController(s AssetAdministrationShellBasicDiscoveryAPIAPIServicer, opts ...AssetAdministrationShellBasicDiscoveryAPIAPIOption) *AssetAdministrationShellBasicDiscoveryAPIAPIController {
	controller := &AssetAdministrationShellBasicDiscoveryAPIAPIController{
		service:      s,
		errorHandler: DefaultErrorHandler,
	}

	for _, opt := range opts {
		opt(controller)
	}

	return controller
}

// Routes returns all the api routes for the AssetAdministrationShellBasicDiscoveryAPIAPIController
func (c *AssetAdministrationShellBasicDiscoveryAPIAPIController) Routes() Routes {
	return Routes{
		"GetAllAssetAdministrationShellIdsByAssetLink": Route{
			strings.ToUpper("Get"),
			"/lookup/shells",
			c.GetAllAssetAdministrationShellIdsByAssetLink,
		},
		"GetAllAssetLinksById": Route{
			strings.ToUpper("Get"),
			"/lookup/shells/{aasIdentifier}",
			c.GetAllAssetLinksById,
		},
		"PostAllAssetLinksById": Route{
			strings.ToUpper("Post"),
			"/lookup/shells/{aasIdentifier}",
			c.PostAllAssetLinksById,
		},
		"DeleteAllAssetLinksById": Route{
			strings.ToUpper("Delete"),
			"/lookup/shells/{aasIdentifier}",
			c.DeleteAllAssetLinksById,
		},
	}
}

// GetAllAssetAdministrationShellIdsByAssetLink - Returns a list of Asset Administration Shell ids linked to specific Asset identifiers
func (c *AssetAdministrationShellBasicDiscoveryAPIAPIController) GetAllAssetAdministrationShellIdsByAssetLink(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	var assetIdsParam []string
	if query.Has("assetIds") {
		assetIdsParam = strings.Split(query.Get("assetIds"), ",")
	}
	var limitParam int32
	if query.Has("limit") {
		param, err := parseNumericParameter[int32](
			query.Get("limit"),
			WithParse[int32](parseInt32),
			WithMinimum[int32](1),
		)
		if err != nil {
			c.errorHandler(w, r, &ParsingError{Param: "limit", Err: err}, nil)
			return
		}

		limitParam = param
	} else {
	}
	var cursorParam string
	if query.Has("cursor") {
		param := query.Get("cursor")

		cursorParam = param
	} else {
	}
	result, err := c.service.GetAllAssetAdministrationShellIdsByAssetLink(r.Context(), assetIdsParam, limitParam, cursorParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllAssetLinksById - Returns a list of specific Asset identifiers based on an Asset Administration Shell id to edit discoverable content
func (c *AssetAdministrationShellBasicDiscoveryAPIAPIController) GetAllAssetLinksById(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}
	result, err := c.service.GetAllAssetLinksById(r.Context(), aasIdentifierParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PostAllAssetLinksById - Creates specific Asset identifiers linked to an Asset Administration Shell to edit discoverable content
func (c *AssetAdministrationShellBasicDiscoveryAPIAPIController) PostAllAssetLinksById(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}
	var specificAssetIdParam []SpecificAssetId
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&specificAssetIdParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	for _, el := range specificAssetIdParam {
		if err := AssertSpecificAssetIdRequired(el); err != nil {
			c.errorHandler(w, r, err, nil)
			return
		}
	}
	result, err := c.service.PostAllAssetLinksById(r.Context(), aasIdentifierParam, specificAssetIdParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// DeleteAllAssetLinksById - Deletes all specific Asset identifiers linked to an Asset Administration Shell to edit discoverable content
func (c *AssetAdministrationShellBasicDiscoveryAPIAPIController) DeleteAllAssetLinksById(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}
	result, err := c.service.DeleteAllAssetLinksById(r.Context(), aasIdentifierParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}
