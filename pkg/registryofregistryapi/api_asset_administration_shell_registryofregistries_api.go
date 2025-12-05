package registryofregistriesapi

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

const (
	componentName = "DISC_VAL"
)

// AssetAdministrationShellRegistryOfRegistriesAPIAPIController binds http requests to an api service and writes the service results to the http response
type AssetAdministrationShellRegistryOfRegistriesAPIAPIController struct {
	service      AssetAdministrationShellRegistryOfRegistriesAPIAPIServicer
	errorHandler model.ErrorHandler
}

// AssetAdministrationShellRegistryOfRegistriesAPIAPIOption for how the controller is set up.
type AssetAdministrationShellRegistryOfRegistriesAPIAPIOption func(*AssetAdministrationShellRegistryOfRegistriesAPIAPIController)

// WithAssetAdministrationShellRegistryOfRegistriesAPIAPIErrorHandler inject ErrorHandler into controller
func WithAssetAdministrationShellRegistryOfRegistriesAPIAPIErrorHandler(h model.ErrorHandler) AssetAdministrationShellRegistryOfRegistriesAPIAPIOption {
	return func(c *AssetAdministrationShellRegistryOfRegistriesAPIAPIController) {
		c.errorHandler = h
	}
}

// NewAssetAdministrationShellRegistryOfRegistriesAPIAPIController creates a default api controller
func NewAssetAdministrationShellRegistryOfRegistriesAPIAPIController(s AssetAdministrationShellRegistryOfRegistriesAPIAPIServicer, opts ...AssetAdministrationShellRegistryOfRegistriesAPIAPIOption) *AssetAdministrationShellRegistryOfRegistriesAPIAPIController {
	controller := &AssetAdministrationShellRegistryOfRegistriesAPIAPIController{
		service:      s,
		errorHandler: model.DefaultErrorHandler,
	}

	for _, opt := range opts {
		opt(controller)
	}

	return controller
}

// Routes returns all the api routes for the AssetAdministrationShellRegistryOfRegistriesAPIAPIController
func (c *AssetAdministrationShellRegistryOfRegistriesAPIAPIController) Routes() Routes {
	return Routes{
		"GetAllRegistryDescriptors": Route{
			strings.ToUpper("Get"),
			"/registry-descriptors",
			c.GetAllRegistryDescriptors,
		},
		"PostRegistryDescriptor": Route{
			strings.ToUpper("Post"),
			"/registry-descriptors",
			c.PostRegistryDescriptor,
		},
		"GetRegistryDescriptorById": Route{
			strings.ToUpper("Get"),
			"/registry-descriptors/{registryIdentifier}",
			c.GetRegistryDescriptorById,
		},
		"PutRegistryDescriptorById": Route{
			strings.ToUpper("Put"),
			"/registry-descriptors/{registryIdentifier}",
			c.PutRegistryDescriptorById,
		},
		"DeleteRegistryDescriptorById": Route{
			strings.ToUpper("Delete"),
			"/registry-descriptors/{registryIdentifier}",
			c.DeleteRegistryById,
		},
	}
}

// GetAllRegistryDescriptors - Returns all Registry Descriptors
func (c *AssetAdministrationShellRegistryOfRegistriesAPIAPIController) GetAllRegistryDescriptors(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		log.Printf("🧩 [%s] Error in GetAllRegistryDescriptors: parse query raw=%q: %v", componentName, r.URL.RawQuery, err)
		result := common.NewErrorResponse(
			err,
			http.StatusBadRequest,
			componentName,
			"GetAllRegistryDescriptors",
			"query",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}
	var limitParam int32
	if query.Has("limit") {
		param, err := parseNumericParameter[int32](
			query.Get("limit"),
			WithParse[int32](parseInt32),
			WithMinimum[int32](1),
		)
		if err != nil {
			log.Printf("🧩 [%s] Error in GetAllRegistryDescriptors: parse limit=%q: %v", componentName, query.Get("limit"), err)
			result := common.NewErrorResponse(
				err,
				http.StatusBadRequest,
				componentName,
				"GetAllRegistryDescriptors",
				"limit",
			)
			EncodeJSONResponse(result.Body, &result.Code, w)
			return
		}
		limitParam = param
	}
	var cursorParam string
	if query.Has("cursor") {
		cursorParam = query.Get("cursor")
	}
	var registryTypeParam string
	if query.Has("registryType") {
		registryTypeParam = query.Get("registryType")
	}
	var companyParam string
	if query.Has("company") {
		companyParam = query.Get("company")
	}

	result, err := c.service.GetAllRegistryDescriptors(r.Context(), limitParam, cursorParam, registryTypeParam, companyParam)
	if err != nil {
		log.Printf("🧩 [%s] Error in GetAllRegistryDescriptors: service failure (limit=%d cursor=%q registryType=%q companyParam=%q): %v", componentName, limitParam, cursorParam, registryTypeParam, companyParam, err)
		c.errorHandler(w, r, err, &result)
		return
	}
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PostRegistryDescriptor - Creates a new Registry Descriptor, i.e. registers a registry
func (c *AssetAdministrationShellRegistryOfRegistriesAPIAPIController) PostRegistryDescriptor(w http.ResponseWriter, r *http.Request) {
	var registryDescriptorParam model.RegistryDescriptor
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&registryDescriptorParam); err != nil {
		log.Printf("🧩 [%s] Error in PostRegistryDescriptor: decode body: %v", componentName, err)
		result := common.NewErrorResponse(
			err,
			http.StatusBadRequest,
			componentName,
			"PostRegistryDescriptor",
			"RequestBody",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}

	if err := model.AssertRegistryDescriptorRequired(registryDescriptorParam); err != nil {
		log.Printf("🧩 [%s] Error in PostRegistryDescriptor: required validation failed: %v", componentName, err)
		result := common.NewErrorResponse(
			err,
			http.StatusBadRequest,
			componentName,
			"PostRegistryDescriptor",
			"RequestBody",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}
	if err := model.AssertRegistryDescriptorConstraints(registryDescriptorParam); err != nil {
		log.Printf("🧩 [%s] Error in PostRegistryDescriptor: constraints validation failed: %v", componentName, err)
		result := common.NewErrorResponse(
			err,
			http.StatusBadRequest,
			componentName,
			"PostRegistryDescriptor",
			"RequestBody",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}
	result, err := c.service.PostRegistryDescriptor(r.Context(), registryDescriptorParam)
	if err != nil {
		log.Printf("🧩 [%s] Error in PostRegistryDescriptor: service failure (bodyId=%q): %v", componentName, registryDescriptorParam.Id, err)
		c.errorHandler(w, r, err, &result)
		return
	}
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetRegistryDescriptorById - Returns a specific Registry Descriptor
func (c *AssetAdministrationShellRegistryOfRegistriesAPIAPIController) GetRegistryDescriptorById(w http.ResponseWriter, r *http.Request) {

	registryIdentifierParam := chi.URLParam(r, "registryIdentifier")
	if registryIdentifierParam == "" {
		log.Printf("🧩 [%s] Error in GetRegistryDescriptorById: missing path parameter registryIdentifier", componentName)
		result := common.NewErrorResponse(
			common.NewErrBadRequest("Missing path parameter 'registryIdentifier'"),
			http.StatusBadRequest,
			componentName,
			"GetRegistryDescriptorById",
			"registryIdentifier",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}
	result, err := c.service.GetRegistryDescriptorById(r.Context(), registryIdentifierParam)
	if err != nil {
		log.Printf("🧩 [%s] Error in GetRegistryDescriptorById: service failure (registryIdentifier=%q): %v", componentName, registryIdentifierParam, err)
		c.errorHandler(w, r, err, &result)
		return
	}
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PutRegistryDescriptorById - Creates or updates an existing Registry Descriptor
func (c *AssetAdministrationShellRegistryOfRegistriesAPIAPIController) PutRegistryDescriptorById(w http.ResponseWriter, r *http.Request) {
	registryIdentifierParam := chi.URLParam(r, "registryIdentifier")
	if registryIdentifierParam == "" {
		log.Printf("🧩 [%s] Error in PutRegistryDescriptorById: missing path parameter registryIdentifier", componentName)
		result := common.NewErrorResponse(
			common.NewErrBadRequest("Missing path parameter 'registryIdentifier'"),
			http.StatusBadRequest,
			componentName,
			"PutRegistryDescriptorById",
			"registryIdentifier",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}
	var registryDescriptorParam model.RegistryDescriptor
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&registryDescriptorParam); err != nil {
		log.Printf("🧩 [%s] Error in PutRegistryDescriptorById: decode body: %v", componentName, err)
		result := common.NewErrorResponse(
			err,
			http.StatusBadRequest,
			componentName,
			"PutRegistryDescriptorById",
			"RequestBody",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}
	if err := model.AssertRegistryDescriptorRequired(registryDescriptorParam); err != nil {
		log.Printf("🧩 [%s] Error in PutRegistryDescriptorById: required validation failed: %v", componentName, err)
		result := common.NewErrorResponse(
			err,
			http.StatusBadRequest,
			componentName,
			"PutRegistryDescriptorById",
			"RequestBody",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}
	if err := model.AssertRegistryDescriptorConstraints(registryDescriptorParam); err != nil {
		log.Printf("🧩 [%s] Error in PutRegistryDescriptorById: constraints validation failed: %v", componentName, err)
		result := common.NewErrorResponse(
			err,
			http.StatusBadRequest,
			componentName,
			"PutRegistryDescriptorById",
			"RequestBody",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}
	result, err := c.service.PutRegistryDescriptorById(r.Context(), registryIdentifierParam, registryDescriptorParam)
	if err != nil {
		log.Printf("🧩 [%s] Error in PutRegistryDescriptorById: service failure (registryIdentifier=%q bodyId=%q): %v", componentName, registryIdentifierParam, registryDescriptorParam.Id, err)
		c.errorHandler(w, r, err, &result)
		return
	}
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// DeleteRegistryById - Deletes a Registry Descriptor, i.e. de-registers a registry
func (c *AssetAdministrationShellRegistryOfRegistriesAPIAPIController) DeleteRegistryById(w http.ResponseWriter, r *http.Request) {
	registryIdentifierParam := chi.URLParam(r, "registryIdentifier")
	if registryIdentifierParam == "" {
		log.Printf("Reg [%s] Error in DeleteRegistryByID: missing path parameter registryIdentifier", componentName)
		result := common.NewErrorResponse(
			common.NewErrBadRequest("Missing path parameter 'registryIdentifier'"),
			http.StatusBadRequest,
			componentName,
			"DeleteRegistryByID",
			"registryIdentifier",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}
	result, err := c.service.DeleteRegistryDescriptorById(r.Context(), registryIdentifierParam)
	if err != nil {
		log.Printf("🧩 [%s] Error in DeleteRegistryDescriptorById: service failure (registryIdentifier=%q): %v", componentName, registryIdentifierParam, err)
		c.errorHandler(w, r, err, &result)
		return
	}
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}
