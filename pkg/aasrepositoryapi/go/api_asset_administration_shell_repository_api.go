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
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	aasjsonization "github.com/aas-core-works/aas-core3.1-golang/jsonization"
	aasverification "github.com/aas-core-works/aas-core3.1-golang/verification"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
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
	r.Body = http.MaxBytesReader(w, r.Body, uploadMaxSizeFromRequestContext(r))

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

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
	defer func() {
		if fileParam != nil {
			tempFilePath := fileParam.Name()
			_ = fileParam.Close()
			// #nosec G703 -- path comes from server-generated temporary file.
			_ = os.Remove(tempFilePath)
		}
	}()

	result, err := c.service.PutThumbnailAasRepository(r.Context(), aasIdentifierParam, fileNameParam, fileParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

func uploadMaxSizeFromRequestContext(r *http.Request) int64 {
	maxBytes := int64(128 << 20)
	if r == nil {
		return maxBytes
	}

	cfg, ok := common.ConfigFromContext(r.Context())
	if ok && cfg != nil && cfg.General.UploadMaxSizeBytes > 0 {
		return cfg.General.UploadMaxSizeBytes
	}

	return maxBytes
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

	query := r.URL.Query()
	levelParam := query.Get("level")
	extentParam := query.Get("extent")

	result, err := c.service.GetSubmodelByIdAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		levelParam,
		extentParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PutSubmodelByIdAasRepository - Creates or updates the Submodel
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PutSubmodelByIdAasRepository(w http.ResponseWriter, r *http.Request) {
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
	submodelParam, err := aasjsonization.SubmodelFromJsonable(jsonable)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	result, err := c.service.PutSubmodelByIdAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		submodelParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	if result.Code == http.StatusCreated {
		location := c.buildSubmodelLocation(r, aasIdentifierParam, submodelIdentifierParam)
		if location != "" {
			w.Header().Set("Location", location)
		}
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// DeleteSubmodelByIdAasRepository - Deletes the submodel from the Asset Administration Shell and the Repository.
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) DeleteSubmodelByIdAasRepository(w http.ResponseWriter, r *http.Request) {
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

	result, err := c.service.DeleteSubmodelByIdAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PatchSubmodelAasRepository - Updates the Submodel
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PatchSubmodelAasRepository(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
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
	submodelParam, err := aasjsonization.SubmodelFromJsonable(jsonable)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	levelParam := ""
	if query.Has("level") {
		levelParam = query.Get("level")
	} else {
		levelParam = "core"
	}

	result, err := c.service.PatchSubmodelAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam, submodelParam, levelParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelByIdMetadataAasRepository - Returns the Submodel's metadata elements
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelByIdMetadataAasRepository(w http.ResponseWriter, r *http.Request) {
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

	result, err := c.service.GetSubmodelByIdMetadataAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PatchSubmodelByIdMetadataAasRepository - Updates the metadata attributes of the Submodel
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PatchSubmodelByIdMetadataAasRepository(w http.ResponseWriter, r *http.Request) {
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

	var jsonable any
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&jsonable); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	submodelMetadataParam, err := common.ParseSubmodelMetadataPayload(jsonable)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	rawMetadataPayload, ok := jsonable.(map[string]any)
	if !ok {
		c.errorHandler(w, r, &ParsingError{Err: errors.New("metadata payload must be an object")}, nil)
		return
	}
	if err := model.AssertSubmodelMetadataRequired(submodelMetadataParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	if err := model.AssertSubmodelMetadataConstraints(submodelMetadataParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}

	ctxWithRawPatch := common.WithSubmodelMetadataPatch(r.Context(), rawMetadataPayload)
	result, err := c.service.PatchSubmodelByIdMetadataAasRepository(ctxWithRawPatch, aasIdentifierParam, submodelIdentifierParam, submodelMetadataParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelByIdValueOnlyAasRepository - Returns the Submodel's ValueOnly representation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelByIdValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
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

	levelParam := ""
	if query.Has("level") {
		levelParam = query.Get("level")
	} else {
		levelParam = "deep"
	}

	extentParam := ""
	if query.Has("extent") {
		extentParam = query.Get("extent")
	} else {
		extentParam = "withoutBlobValue"
	}

	result, err := c.service.GetSubmodelByIdValueOnlyAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam, levelParam, extentParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PatchSubmodelByIdValueOnlyAasRepository - Updates the values of the Submodel
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PatchSubmodelByIdValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
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

	var bodyParam map[string]any
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&bodyParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	levelParam := ""
	if query.Has("level") {
		levelParam = query.Get("level")
	} else {
		levelParam = "core"
	}

	result, err := c.service.PatchSubmodelByIdValueOnlyAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam, bodyParam, levelParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelByIdReferenceAasRepository - Returns the Submodel as a Reference
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelByIdReferenceAasRepository(w http.ResponseWriter, r *http.Request) {
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

	result, err := c.service.GetSubmodelByIdReferenceAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelByIdPathAasRepository - Returns the elements of this submodel in path notation.
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelByIdPathAasRepository(w http.ResponseWriter, r *http.Request) {
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

	query := r.URL.Query()
	levelParam := query.Get("level")

	result, err := c.service.GetSubmodelByIdPathAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		levelParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelElementsAasRepository - Returns all submodel elements including their hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllSubmodelElementsAasRepository(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

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
	}

	cursorParam := ""
	if query.Has("cursor") {
		cursorParam = query.Get("cursor")
	}

	levelParam := "deep"
	if query.Has("level") {
		levelParam = query.Get("level")
	}

	extentParam := "withoutBlobValue"
	if query.Has("extent") {
		extentParam = query.Get("extent")
	}

	result, err := c.service.GetAllSubmodelElementsAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		limitParam,
		cursorParam,
		levelParam,
		extentParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PostSubmodelElementAasRepository - Creates a new submodel element
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PostSubmodelElementAasRepository(w http.ResponseWriter, r *http.Request) {
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

	submodelElementParam, err := aasjsonization.SubmodelElementFromJsonable(jsonable)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	if c.strictVerification {
		var validationErrors []string
		aasverification.Verify(submodelElementParam, func(verErr *aasverification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false
		})

		if len(validationErrors) > 0 {
			err := fmt.Errorf("validation failed: %s", strings.Join(validationErrors, "; "))
			c.errorHandler(w, r, &ParsingError{Err: err}, nil)
			return
		}
	}

	result, err := c.service.PostSubmodelElementAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam, submodelElementParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelElementsMetadataAasRepository - Returns all submodel elements including their hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllSubmodelElementsMetadataAasRepository(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

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
	}

	cursorParam := ""
	if query.Has("cursor") {
		cursorParam = query.Get("cursor")
	}

	result, err := c.service.GetAllSubmodelElementsMetadataAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		limitParam,
		cursorParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelElementsValueOnlyAasRepository - Returns all submodel elements including their hierarchy in the ValueOnly representation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllSubmodelElementsValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

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
	}

	cursorParam := ""
	if query.Has("cursor") {
		cursorParam = query.Get("cursor")
	}

	levelParam := "deep"
	if query.Has("level") {
		levelParam = query.Get("level")
	}

	result, err := c.service.GetAllSubmodelElementsValueOnlyAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		limitParam,
		cursorParam,
		levelParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelElementsReferenceAasRepository - Returns all submodel elements as a list of References
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllSubmodelElementsReferenceAasRepository(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

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
	}

	cursorParam := ""
	if query.Has("cursor") {
		cursorParam = query.Get("cursor")
	}

	levelParam := "core"
	if query.Has("level") {
		levelParam = query.Get("level")
	}

	result, err := c.service.GetAllSubmodelElementsReferenceAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		limitParam,
		cursorParam,
		levelParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelElementsPathAasRepository - Returns all submodel elements including their hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetAllSubmodelElementsPathAasRepository(w http.ResponseWriter, r *http.Request) {
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

	query := r.URL.Query()

	limitParam := int32(0)
	if limitParamString := query.Get("limit"); limitParamString != "" {
		limitParam64, err := strconv.ParseInt(limitParamString, 10, 32)
		if err != nil {
			c.errorHandler(w, r, &ParsingError{Err: err}, nil)
			return
		}
		limitParam = int32(limitParam64)
	}

	cursorParam := query.Get("cursor")
	levelParam := query.Get("level")
	extentParam := query.Get("extent")

	result, err := c.service.GetAllSubmodelElementsPathAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		limitParam,
		cursorParam,
		levelParam,
		extentParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelElementByPathAasRepository - Returns a specific submodel element from the Submodel at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelElementByPathAasRepository(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	levelParam := "deep"
	if query.Has("level") {
		levelParam = query.Get("level")
	}

	extentParam := "withoutBlobValue"
	if query.Has("extent") {
		extentParam = query.Get("extent")
	}

	result, err := c.service.GetSubmodelElementByPathAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		levelParam,
		extentParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PutSubmodelElementByPathAasRepository - Creates or updates an existing submodel element at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PutSubmodelElementByPathAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
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

	submodelElementParam, err := aasjsonization.SubmodelElementFromJsonable(jsonable)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	if c.strictVerification {
		var validationErrors []string
		aasverification.Verify(submodelElementParam, func(verErr *aasverification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false
		})

		if len(validationErrors) > 0 {
			err := fmt.Errorf("validation failed: %s", strings.Join(validationErrors, "; "))
			c.errorHandler(w, r, &ParsingError{Err: err}, nil)
			return
		}
	}

	result, err := c.service.PutSubmodelElementByPathAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam, idShortPathParam, submodelElementParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PostSubmodelElementByPathAasRepository - Creates a new submodel element at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PostSubmodelElementByPathAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
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

	submodelElementParam, err := aasjsonization.SubmodelElementFromJsonable(jsonable)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	if c.strictVerification {
		var validationErrors []string
		aasverification.Verify(submodelElementParam, func(verErr *aasverification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false
		})

		if len(validationErrors) > 0 {
			err := fmt.Errorf("validation failed: %s", strings.Join(validationErrors, "; "))
			c.errorHandler(w, r, &ParsingError{Err: err}, nil)
			return
		}
	}

	result, err := c.service.PostSubmodelElementByPathAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam, idShortPathParam, submodelElementParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// DeleteSubmodelElementByPathAasRepository - Deletes a submodel element at a specified path within the submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) DeleteSubmodelElementByPathAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	result, err := c.service.DeleteSubmodelElementByPathAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam, idShortPathParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PatchSubmodelElementValueByPathAasRepository - Updates an existing submodel element value at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PatchSubmodelElementValueByPathAasRepository(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
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

	submodelElementParam, err := aasjsonization.SubmodelElementFromJsonable(jsonable)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	if c.strictVerification {
		var validationErrors []string
		aasverification.Verify(submodelElementParam, func(verErr *aasverification.VerificationError) bool {
			validationErrors = append(validationErrors, verErr.Error())
			return false
		})

		if len(validationErrors) > 0 {
			err := fmt.Errorf("validation failed: %s", strings.Join(validationErrors, "; "))
			c.errorHandler(w, r, &ParsingError{Err: err}, nil)
			return
		}
	}

	levelParam := "core"
	if query.Has("level") {
		levelParam = query.Get("level")
	}

	result, err := c.service.PatchSubmodelElementValueByPathAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		submodelElementParam,
		levelParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelElementByPathMetadataAasRepository - Returns the metadata attributes if a specific submodel element from the Submodel at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelElementByPathMetadataAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	result, err := c.service.GetSubmodelElementByPathMetadataAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam, idShortPathParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PatchSubmodelElementValueByPathMetadata - Updates the metadata attributes of an existing submodel element value at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PatchSubmodelElementValueByPathMetadata(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	var jsonable any
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&jsonable); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	submodelElementMetadataParam, err := common.ParseSubmodelElementMetadataPayload(jsonable)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	rawMetadataPayload, ok := jsonable.(map[string]any)
	if !ok {
		c.errorHandler(w, r, &ParsingError{Err: errors.New("metadata payload must be an object")}, nil)
		return
	}

	ctxWithRawPatch := common.WithSubmodelElementMetadataPatch(r.Context(), rawMetadataPayload)
	result, err := c.service.PatchSubmodelElementValueByPathMetadata(
		ctxWithRawPatch,
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		submodelElementMetadataParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelElementByPathValueOnlyAasRepository - Returns a specific submodel element from the Submodel at a specified path in the ValueOnly representation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelElementByPathValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	levelParam := "deep"
	if query.Has("level") {
		levelParam = query.Get("level")
	}

	extentParam := "withoutBlobValue"
	if query.Has("extent") {
		extentParam = query.Get("extent")
	}

	result, err := c.service.GetSubmodelElementByPathValueOnlyAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		levelParam,
		extentParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PatchSubmodelElementValueByPathValueOnly - Updates the value of an existing submodel element value at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PatchSubmodelElementValueByPathValueOnly(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	submodelElementValueParam, err := model.UnmarshalSubmodelElementValue(bodyBytes)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	levelParam := "core"
	if query.Has("level") {
		levelParam = query.Get("level")
	}

	result, err := c.service.PatchSubmodelElementValueByPathValueOnly(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		submodelElementValueParam,
		levelParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelElementByPathReferenceAasRepository - Returns the Reference of a specific submodel element from the Submodel at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelElementByPathReferenceAasRepository(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	levelParam := "core"
	if query.Has("level") {
		levelParam = query.Get("level")
	}

	result, err := c.service.GetSubmodelElementByPathReferenceAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		levelParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelElementByPathPathAasRepository - Returns a specific submodel element from the Submodel at a specified path in the Path notation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetSubmodelElementByPathPathAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	query := r.URL.Query()
	levelParam := query.Get("level")

	result, err := c.service.GetSubmodelElementByPathPathAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		levelParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetFileByPathAasRepository - Downloads file content from a specific submodel element from the Submodel at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetFileByPathAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	result, err := c.service.GetFileByPathAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam, idShortPathParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PutFileByPathAasRepository - Uploads file content to an existing submodel element at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) PutFileByPathAasRepository(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, uploadMaxSizeFromRequestContext(r))

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	fileNameParam := r.FormValue("fileName")
	var fileParam *os.File
	{
		param, err := ReadFormFileToTempFile(r, "file")
		if err != nil {
			c.errorHandler(w, r, &ParsingError{Param: "file", Err: err}, nil)
			return
		}
		fileParam = param
	}
	defer func() {
		if fileParam != nil {
			tempFilePath := fileParam.Name()
			_ = fileParam.Close()
			// #nosec G703 -- path comes from server-generated temporary file.
			_ = os.Remove(tempFilePath)
		}
	}()

	result, err := c.service.PutFileByPathAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		fileNameParam,
		fileParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// DeleteFileByPathAasRepository - Deletes file content of an existing submodel element at a specified path within submodel elements hierarchy
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) DeleteFileByPathAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	result, err := c.service.DeleteFileByPathAasRepository(r.Context(), aasIdentifierParam, submodelIdentifierParam, idShortPathParam)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// InvokeOperationAasRepository - Synchronously invokes an Operation at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) InvokeOperationAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	var operationRequestParam model.OperationRequest
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&operationRequestParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	requestContext := common.WithAuthorizationHeader(r.Context(), r.Header.Get("Authorization"))
	result, err := c.service.InvokeOperationAasRepository(
		requestContext,
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		operationRequestParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// InvokeOperationValueOnlyAasRepository - Synchronously invokes an Operation at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) InvokeOperationValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	var operationRequestValueOnlyParam model.OperationRequestValueOnly
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&operationRequestValueOnlyParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	if err := model.AssertOperationRequestValueOnlyRequired(operationRequestValueOnlyParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	if err := model.AssertOperationRequestValueOnlyConstraints(operationRequestValueOnlyParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}

	requestContext := common.WithAuthorizationHeader(r.Context(), r.Header.Get("Authorization"))
	result, err := c.service.InvokeOperationValueOnlyAasRepository(
		requestContext,
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		operationRequestValueOnlyParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// InvokeOperationAsyncAasRepository - Asynchronously invokes an Operation at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) InvokeOperationAsyncAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	var operationRequestParam model.OperationRequest
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&operationRequestParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	requestContext := common.WithAuthorizationHeader(r.Context(), r.Header.Get("Authorization"))
	result, err := c.service.InvokeOperationAsyncAasRepository(
		requestContext,
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		operationRequestParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// InvokeOperationAsyncValueOnlyAasRepository - Asynchronously invokes an Operation at a specified path
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) InvokeOperationAsyncValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	var operationRequestValueOnlyParam model.OperationRequestValueOnly
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&operationRequestValueOnlyParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	if err := model.AssertOperationRequestValueOnlyRequired(operationRequestValueOnlyParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	if err := model.AssertOperationRequestValueOnlyConstraints(operationRequestValueOnlyParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}

	requestContext := common.WithAuthorizationHeader(r.Context(), r.Header.Get("Authorization"))
	result, err := c.service.InvokeOperationAsyncValueOnlyAasRepository(
		requestContext,
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		operationRequestValueOnlyParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetOperationAsyncStatusAasRepository - Returns the Operation status of an asynchronous invoked Operation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetOperationAsyncStatusAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	handleIDParam := chi.URLParam(r, "handleId")
	if handleIDParam == "" {
		c.errorHandler(w, r, &RequiredError{"handleId"}, nil)
		return
	}

	result, err := c.service.GetOperationAsyncStatusAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		handleIDParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetOperationAsyncResultAasRepository - Returns the Operation result of an asynchronous invoked Operation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetOperationAsyncResultAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	handleIDParam := chi.URLParam(r, "handleId")
	if handleIDParam == "" {
		c.errorHandler(w, r, &RequiredError{"handleId"}, nil)
		return
	}

	result, err := c.service.GetOperationAsyncResultAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		handleIDParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetOperationAsyncResultValueOnlyAasRepository - Returns the ValueOnly notation of the Operation result of an asynchronous invoked Operation
// nolint:revive
func (c *AssetAdministrationShellRepositoryAPIAPIController) GetOperationAsyncResultValueOnlyAasRepository(w http.ResponseWriter, r *http.Request) {
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

	idShortPathParam := chi.URLParam(r, "idShortPath")
	if idShortPathParam == "" {
		c.errorHandler(w, r, &RequiredError{"idShortPath"}, nil)
		return
	}

	handleIDParam := chi.URLParam(r, "handleId")
	if handleIDParam == "" {
		c.errorHandler(w, r, &RequiredError{"handleId"}, nil)
		return
	}

	result, err := c.service.GetOperationAsyncResultValueOnlyAasRepository(
		r.Context(),
		aasIdentifierParam,
		submodelIdentifierParam,
		idShortPathParam,
		handleIDParam,
	)
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}

	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}
