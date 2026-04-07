/*
 * DotAAS Part 2 | HTTP/REST | Asset Administration Shell Repository Service Specification
 *
 * The Full Profile of the Asset Administration Shell Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](https://industrialdigitaltwin.org/en/content-hub/aasspecifications).   Copyright: Industrial Digital Twin Association (IDTA) 2025
 *
 * API version: V3.1.1_SSP-001
 * Contact: info@idtwin.org
 */

// Package openapi provides the generated HTTP controller and routing bindings
// for the Asset Administration Shell Repository API.
package openapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	aasjsonization "github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/go-chi/chi/v5"
)

// AssetAdministrationShellRepositoryAPIAPIController binds http requests to an api service and writes the service results to the http response
type AssetAdministrationShellRepositoryAPIAPIController struct {
	service            AssetAdministrationShellRepositoryAPIAPIServicer
	errorHandler       ErrorHandler
	contextPath        string
	strictVerification bool
}

// AssetAdministrationShellRepositoryAPIAPIOption for how the controller is set up.
type AssetAdministrationShellRepositoryAPIAPIOption func(*AssetAdministrationShellRepositoryAPIAPIController)

// WithAssetAdministrationShellRepositoryAPIAPIErrorHandler inject ErrorHandler into controller
func WithAssetAdministrationShellRepositoryAPIAPIErrorHandler(h ErrorHandler) AssetAdministrationShellRepositoryAPIAPIOption {
	return func(c *AssetAdministrationShellRepositoryAPIAPIController) {
		c.errorHandler = h
	}
}

// NewAssetAdministrationShellRepositoryAPIAPIController creates a default api controller
func NewAssetAdministrationShellRepositoryAPIAPIController(s AssetAdministrationShellRepositoryAPIAPIServicer, contextPath string, strictVerification bool, opts ...AssetAdministrationShellRepositoryAPIAPIOption) *AssetAdministrationShellRepositoryAPIAPIController {
	controller := &AssetAdministrationShellRepositoryAPIAPIController{
		service:            s,
		errorHandler:       DefaultErrorHandler,
		contextPath:        contextPath,
		strictVerification: strictVerification,
	}

	for _, opt := range opts {
		opt(controller)
	}

	return controller
}

// Routes returns all the api routes for the AssetAdministrationShellRepositoryAPIAPIController
func (c *AssetAdministrationShellRepositoryAPIAPIController) Routes() Routes {
	return Routes{
		"GetAllAssetAdministrationShells": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells",
			c.GetAllAssetAdministrationShells,
		},
		"PostAssetAdministrationShell": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/shells",
			c.PostAssetAdministrationShell,
		},
		"GetAllAssetAdministrationShellsReference": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/$reference",
			c.GetAllAssetAdministrationShellsReference,
		},
		"GetAssetAdministrationShellById": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}",
			c.GetAssetAdministrationShellById,
		},
		"PutAssetAdministrationShellById": Route{
			strings.ToUpper("Put"),
			c.contextPath + "/shells/{aasIdentifier}",
			c.PutAssetAdministrationShellById,
		},
		"DeleteAssetAdministrationShellById": Route{
			strings.ToUpper("Delete"),
			c.contextPath + "/shells/{aasIdentifier}",
			c.DeleteAssetAdministrationShellById,
		},
		"GetAssetAdministrationShellByIdReferenceAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/$reference",
			c.GetAssetAdministrationShellByIdReferenceAasRepository,
		},
		"GetAssetInformationAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/asset-information",
			c.GetAssetInformationAasRepository,
		},
		"PutAssetInformationAasRepository": Route{
			strings.ToUpper("Put"),
			c.contextPath + "/shells/{aasIdentifier}/asset-information",
			c.PutAssetInformationAasRepository,
		},
		"GetThumbnailAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/asset-information/thumbnail",
			c.GetThumbnailAasRepository,
		},
		"PutThumbnailAasRepository": Route{
			strings.ToUpper("Put"),
			c.contextPath + "/shells/{aasIdentifier}/asset-information/thumbnail",
			c.PutThumbnailAasRepository,
		},
		"DeleteThumbnailAasRepository": Route{
			strings.ToUpper("Delete"),
			c.contextPath + "/shells/{aasIdentifier}/asset-information/thumbnail",
			c.DeleteThumbnailAasRepository,
		},
		"GetAllSubmodelReferencesAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodel-refs",
			c.GetAllSubmodelReferencesAasRepository,
		},
		"PostSubmodelReferenceAasRepository": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/shells/{aasIdentifier}/submodel-refs",
			c.PostSubmodelReferenceAasRepository,
		},
		"DeleteSubmodelReferenceAasRepository": Route{
			strings.ToUpper("Delete"),
			c.contextPath + "/shells/{aasIdentifier}/submodel-refs/{submodelIdentifier}",
			c.DeleteSubmodelReferenceAasRepository,
		},
		"GetSubmodelByIdAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}",
			c.GetSubmodelByIdAasRepository,
		},
		"PutSubmodelByIdAasRepository": Route{
			strings.ToUpper("Put"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}",
			c.PutSubmodelByIdAasRepository,
		},
		"DeleteSubmodelByIdAasRepository": Route{
			strings.ToUpper("Delete"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}",
			c.DeleteSubmodelByIdAasRepository,
		},
		"PatchSubmodelAasRepository": Route{
			strings.ToUpper("Patch"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}",
			c.PatchSubmodelAasRepository,
		},
		"GetSubmodelByIdMetadataAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$metadata",
			c.GetSubmodelByIdMetadataAasRepository,
		},
		"PatchSubmodelByIdMetadataAasRepository": Route{
			strings.ToUpper("Patch"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$metadata",
			c.PatchSubmodelByIdMetadataAasRepository,
		},
		"GetSubmodelByIdValueOnlyAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$value",
			c.GetSubmodelByIdValueOnlyAasRepository,
		},
		"PatchSubmodelByIdValueOnlyAasRepository": Route{
			strings.ToUpper("Patch"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$value",
			c.PatchSubmodelByIdValueOnlyAasRepository,
		},
		"GetSubmodelByIdReferenceAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$reference",
			c.GetSubmodelByIdReferenceAasRepository,
		},
		"GetSubmodelByIdPathAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$path",
			c.GetSubmodelByIdPathAasRepository,
		},
		"GetAllSubmodelElementsAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements",
			c.GetAllSubmodelElementsAasRepository,
		},
		"PostSubmodelElementAasRepository": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements",
			c.PostSubmodelElementAasRepository,
		},
		"GetAllSubmodelElementsMetadataAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/$metadata",
			c.GetAllSubmodelElementsMetadataAasRepository,
		},
		"GetAllSubmodelElementsValueOnlyAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/$value",
			c.GetAllSubmodelElementsValueOnlyAasRepository,
		},
		"GetAllSubmodelElementsReferenceAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/$reference",
			c.GetAllSubmodelElementsReferenceAasRepository,
		},
		"GetAllSubmodelElementsPathAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/$path",
			c.GetAllSubmodelElementsPathAasRepository,
		},
		"GetSubmodelElementByPathAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}",
			c.GetSubmodelElementByPathAasRepository,
		},
		"PutSubmodelElementByPathAasRepository": Route{
			strings.ToUpper("Put"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}",
			c.PutSubmodelElementByPathAasRepository,
		},
		"PostSubmodelElementByPathAasRepository": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}",
			c.PostSubmodelElementByPathAasRepository,
		},
		"DeleteSubmodelElementByPathAasRepository": Route{
			strings.ToUpper("Delete"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}",
			c.DeleteSubmodelElementByPathAasRepository,
		},
		"PatchSubmodelElementValueByPathAasRepository": Route{
			strings.ToUpper("Patch"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}",
			c.PatchSubmodelElementValueByPathAasRepository,
		},
		"GetSubmodelElementByPathMetadataAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$metadata",
			c.GetSubmodelElementByPathMetadataAasRepository,
		},
		"PatchSubmodelElementValueByPathMetadata": Route{
			strings.ToUpper("Patch"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$metadata",
			c.PatchSubmodelElementValueByPathMetadata,
		},
		"GetSubmodelElementByPathValueOnlyAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$value",
			c.GetSubmodelElementByPathValueOnlyAasRepository,
		},
		"PatchSubmodelElementValueByPathValueOnly": Route{
			strings.ToUpper("Patch"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$value",
			c.PatchSubmodelElementValueByPathValueOnly,
		},
		"GetSubmodelElementByPathReferenceAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$reference",
			c.GetSubmodelElementByPathReferenceAasRepository,
		},
		"GetSubmodelElementByPathPathAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$path",
			c.GetSubmodelElementByPathPathAasRepository,
		},
		"GetFileByPathAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment",
			c.GetFileByPathAasRepository,
		},
		"PutFileByPathAasRepository": Route{
			strings.ToUpper("Put"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment",
			c.PutFileByPathAasRepository,
		},
		"DeleteFileByPathAasRepository": Route{
			strings.ToUpper("Delete"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment",
			c.DeleteFileByPathAasRepository,
		},
		"InvokeOperationAasRepository": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke",
			c.InvokeOperationAasRepository,
		},
		"InvokeOperationValueOnlyAasRepository": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke/$value",
			c.InvokeOperationValueOnlyAasRepository,
		},
		"InvokeOperationAsyncAasRepository": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke-async",
			c.InvokeOperationAsyncAasRepository,
		},
		"InvokeOperationAsyncValueOnlyAasRepository": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke-async/$value",
			c.InvokeOperationAsyncValueOnlyAasRepository,
		},
		"GetOperationAsyncStatusAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-status/{handleId}",
			c.GetOperationAsyncStatusAasRepository,
		},
		"GetOperationAsyncResultAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-results/{handleId}",
			c.GetOperationAsyncResultAasRepository,
		},
		"GetOperationAsyncResultValueOnlyAasRepository": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-results/{handleId}/$value",
			c.GetOperationAsyncResultValueOnlyAasRepository,
		},
	}
}

// GetAllAssetAdministrationShells - Returns all Asset Administration Shells
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllAssetAdministrationShells(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	assetIdsParam := query["assetIds"]
	idShortParam := query.Get("idShort")

	var limitParam int32
	if limit := query.Get("limit"); limit != "" {
		parsed, err := strconv.ParseInt(limit, 10, 32)
		if err != nil {
			c.errorHandler(w, r, &ParsingError{Err: err}, nil)
			return
		}
		limitParam = int32(parsed)
	}

	cursorParam := query.Get("cursor")

	result, err := c.service.GetAllAssetAdministrationShells(r.Context(), assetIdsParam, idShortParam, limitParam, cursorParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PostAssetAdministrationShell - Creates a new Asset Administration Shell
func (c *AssetAdministrationShellRepositoryAPIAPIController) PostAssetAdministrationShell(w http.ResponseWriter, r *http.Request) {
	// Read and unmarshal JSON to interface{} first
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	var jsonable interface{}
	if err := json.Unmarshal(bodyBytes, &jsonable); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	aasParam, err := aasjsonization.AssetAdministrationShellFromJsonable(jsonable)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	result, err := c.service.PostAssetAdministrationShell(r.Context(), aasParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	if result.Code == http.StatusCreated {
		shellID := aasParam.ID()
		if shellID != "" {
			encodedShellID := encodeIdentifierForPath(shellID)
			location := c.buildShellLocation(r, encodedShellID)
			if location != "" {
				w.Header().Set("Location", location)
			}
		}
	}

	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllAssetAdministrationShellsReference - Returns References to all Asset Administration Shells
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllAssetAdministrationShellsReference(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	assetIdsParam := query["assetIds"]
	idShortParam := query.Get("idShort")

	var limitParam int32
	if limit := query.Get("limit"); limit != "" {
		parsed, err := strconv.ParseInt(limit, 10, 32)
		if err != nil {
			c.errorHandler(w, r, &ParsingError{Err: err}, nil)
			return
		}
		limitParam = int32(parsed)
	}

	cursorParam := query.Get("cursor")

	result, err := c.service.GetAllAssetAdministrationShellsReference(r.Context(), assetIdsParam, idShortParam, limitParam, cursorParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAssetAdministrationShellById - Returns a specific Asset Administration Shell
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAssetAdministrationShellById(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}

	result, err := c.service.GetAssetAdministrationShellById(r.Context(), aasIdentifierParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PutAssetAdministrationShellById - Creates or updates an existing Asset Administration Shell
func (c *AssetAdministrationShellRepositoryAPIAPIController) PutAssetAdministrationShellById(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	var jsonable interface{}
	if err := json.Unmarshal(bodyBytes, &jsonable); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	aasParam, err := aasjsonization.AssetAdministrationShellFromJsonable(jsonable)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	result, err := c.service.PutAssetAdministrationShellById(r.Context(), aasIdentifierParam, aasParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	if result.Code == http.StatusCreated {
		location := c.buildShellLocation(r, aasIdentifierParam)
		if location != "" {
			w.Header().Set("Location", location)
		}
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// DeleteAssetAdministrationShellById - Deletes an Asset Administration Shell
func (c *AssetAdministrationShellRepositoryAPIAPIController) DeleteAssetAdministrationShellById(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}

	result, err := c.service.DeleteAssetAdministrationShellById(r.Context(), aasIdentifierParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAssetAdministrationShellByIdReferenceAasRepository - Returns a specific Asset Administration Shell as a Reference
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAssetAdministrationShellByIdReferenceAasRepository(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}

	result, err := c.service.GetAssetAdministrationShellByIdReferenceAasRepository(r.Context(), aasIdentifierParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAssetInformationAasRepository - Returns the Asset Information
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAssetInformationAasRepository(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}

	result, err := c.service.GetAssetInformationAasRepository(r.Context(), aasIdentifierParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PutAssetInformationAasRepository - Updates the Asset Information
func (c *AssetAdministrationShellRepositoryAPIAPIController) PutAssetInformationAasRepository(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	var jsonable interface{}
	if err := json.Unmarshal(bodyBytes, &jsonable); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	assetInformationParam, err := aasjsonization.AssetInformationFromJsonable(jsonable)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	result, err := c.service.PutAssetInformationAasRepository(r.Context(), aasIdentifierParam, assetInformationParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetThumbnailAasRepository -
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetThumbnailAasRepository(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}

	result, err := c.service.GetThumbnailAasRepository(r.Context(), aasIdentifierParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PutThumbnailAasRepository -
func (c *AssetAdministrationShellRepositoryAPIAPIController) PutThumbnailAasRepository(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}

	fileNameParam := r.FormValue("fileName")
	fileParam, fileErr := ReadFormFileToTempFile(r, "file")
	if fileErr != nil {
		c.errorHandler(w, r, &ParsingError{Param: "file", Err: fileErr}, nil)
		return
	}

	result, err := c.service.PutThumbnailAasRepository(r.Context(), aasIdentifierParam, fileNameParam, fileParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// DeleteThumbnailAasRepository -
func (c *AssetAdministrationShellRepositoryAPIAPIController) DeleteThumbnailAasRepository(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}

	result, err := c.service.DeleteThumbnailAasRepository(r.Context(), aasIdentifierParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelReferencesAasRepository - Returns all submodel references
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllSubmodelReferencesAasRepository(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}

	query := r.URL.Query()
	var limitParam int32
	if limit := query.Get("limit"); limit != "" {
		parsed, err := strconv.ParseInt(limit, 10, 32)
		if err != nil {
			c.errorHandler(w, r, &ParsingError{Err: err}, nil)
			return
		}
		limitParam = int32(parsed)
	}
	cursorParam := query.Get("cursor")

	result, err := c.service.GetAllSubmodelReferencesAasRepository(r.Context(), aasIdentifierParam, limitParam, cursorParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PostSubmodelReferenceAasRepository - Creates a submodel reference at the Asset Administration Shell
func (c *AssetAdministrationShellRepositoryAPIAPIController) PostSubmodelReferenceAasRepository(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}

	// Read and unmarshal JSON to interface{} first
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	var jsonable interface{}
	if err := json.Unmarshal(bodyBytes, &jsonable); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	refParam, err := aasjsonization.ReferenceFromJsonable(jsonable)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	result, err := c.service.PostSubmodelReferenceAasRepository(r.Context(), aasIdentifierParam, refParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	if result.Code == http.StatusCreated {
		location := c.buildSubmodelReferencesLocation(r, aasIdentifierParam)
		if location != "" {
			w.Header().Set("Location", location)
		}
	}

	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// DeleteSubmodelReferenceAasRepository - Deletes the submodel reference from the Asset Administration Shell. Does not delete the submodel itself!
func (c *AssetAdministrationShellRepositoryAPIAPIController) DeleteSubmodelReferenceAasRepository(w http.ResponseWriter, r *http.Request) {
	aasIdentifierParam := chi.URLParam(r, "aasIdentifier")
	if aasIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"aasIdentifier"}, nil)
		return
	}

	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}

	result, err := c.service.DeleteSubmodelReferenceAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelByIdAasRepository - Returns the Submodel
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelByIdAasRepository(w http.ResponseWriter, r *http.Request) {
}

// PutSubmodelByIdAasRepository - Creates or updates the Submodel
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PutSubmodelByIdAasRepository(w http.ResponseWriter, r *http.Request) {
}

// DeleteSubmodelByIdAasRepository - Deletes the submodel from the Asset Administration Shell and the Repository.
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) DeleteSubmodelByIdAasRepository(w http.ResponseWriter, r *http.Request) {
}

// PatchSubmodelAasRepository - Updates the Submodel
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PatchSubmodelAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetSubmodelByIdMetadataAasRepository - Returns the Submodel's metadata elements
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelByIdMetadataAasRepository(w http.ResponseWriter, r *http.Request) {
}

// PatchSubmodelByIdMetadataAasRepository - Updates the metadata attributes of the Submodel
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PatchSubmodelByIdMetadataAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetSubmodelByIdValueOnlyAasRepository - Returns the Submodel's ValueOnly representation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelByIdValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
}

// PatchSubmodelByIdValueOnlyAasRepository - Updates the values of the Submodel
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PatchSubmodelByIdValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetSubmodelByIdReferenceAasRepository - Returns the Submodel as a Reference
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelByIdReferenceAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetSubmodelByIdPathAasRepository - Returns the elements of this submodel in path notation.
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelByIdPathAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetAllSubmodelElementsAasRepository - Returns all submodel elements including their hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllSubmodelElementsAasRepository(w http.ResponseWriter, r *http.Request) {
}

// PostSubmodelElementAasRepository - Creates a new submodel element
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PostSubmodelElementAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetAllSubmodelElementsMetadataAasRepository - Returns all submodel elements including their hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllSubmodelElementsMetadataAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetAllSubmodelElementsValueOnlyAasRepository - Returns all submodel elements including their hierarchy in the ValueOnly representation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllSubmodelElementsValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetAllSubmodelElementsReferenceAasRepository - Returns all submodel elements as a list of References
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllSubmodelElementsReferenceAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetAllSubmodelElementsPathAasRepository - Returns all submodel elements including their hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllSubmodelElementsPathAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetSubmodelElementByPathAasRepository - Returns a specific submodel element from the Submodel at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelElementByPathAasRepository(w http.ResponseWriter, r *http.Request) {
}

// PutSubmodelElementByPathAasRepository - Creates or updates an existing submodel element at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PutSubmodelElementByPathAasRepository(w http.ResponseWriter, r *http.Request) {
}

// PostSubmodelElementByPathAasRepository - Creates a new submodel element at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PostSubmodelElementByPathAasRepository(w http.ResponseWriter, r *http.Request) {
}

// DeleteSubmodelElementByPathAasRepository - Deletes a submodel element at a specified path within the submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) DeleteSubmodelElementByPathAasRepository(w http.ResponseWriter, r *http.Request) {
}

// PatchSubmodelElementValueByPathAasRepository - Updates an existing submodel element value at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PatchSubmodelElementValueByPathAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetSubmodelElementByPathMetadataAasRepository - Returns the metadata attributes if a specific submodel element from the Submodel at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelElementByPathMetadataAasRepository(w http.ResponseWriter, r *http.Request) {
}

// PatchSubmodelElementValueByPathMetadata - Updates the metadata attributes of an existing submodel element value at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PatchSubmodelElementValueByPathMetadata(w http.ResponseWriter, r *http.Request) {
}

// GetSubmodelElementByPathValueOnlyAasRepository - Returns a specific submodel element from the Submodel at a specified path in the ValueOnly representation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelElementByPathValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
}

// PatchSubmodelElementValueByPathValueOnly - Updates the value of an existing submodel element value at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PatchSubmodelElementValueByPathValueOnly(w http.ResponseWriter, r *http.Request) {
}

// GetSubmodelElementByPathReferenceAasRepository - Returns the Reference of a specific submodel element from the Submodel at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelElementByPathReferenceAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetSubmodelElementByPathPathAasRepository - Returns a specific submodel element from the Submodel at a specified path in the Path notation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelElementByPathPathAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetFileByPathAasRepository - Downloads file content from a specific submodel element from the Submodel at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetFileByPathAasRepository(w http.ResponseWriter, r *http.Request) {
}

// PutFileByPathAasRepository - Uploads file content to an existing submodel element at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PutFileByPathAasRepository(w http.ResponseWriter, r *http.Request) {
}

// DeleteFileByPathAasRepository - Deletes file content of an existing submodel element at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) DeleteFileByPathAasRepository(w http.ResponseWriter, r *http.Request) {
}

// InvokeOperationAasRepository - Synchronously invokes an Operation at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) InvokeOperationAasRepository(w http.ResponseWriter, r *http.Request) {
}

// InvokeOperationValueOnlyAasRepository - Synchronously invokes an Operation at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) InvokeOperationValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
}

// InvokeOperationAsyncAasRepository - Asynchronously invokes an Operation at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) InvokeOperationAsyncAasRepository(w http.ResponseWriter, r *http.Request) {
}

// InvokeOperationAsyncValueOnlyAasRepository - Asynchronously invokes an Operation at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) InvokeOperationAsyncValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetOperationAsyncStatusAasRepository - Returns the Operation status of an asynchronous invoked Operation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetOperationAsyncStatusAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetOperationAsyncResultAasRepository - Returns the Operation result of an asynchronous invoked Operation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetOperationAsyncResultAasRepository(w http.ResponseWriter, r *http.Request) {
}

// GetOperationAsyncResultValueOnlyAasRepository - Returns the ValueOnly notation of the Operation result of an asynchronous invoked Operation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetOperationAsyncResultValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
}
