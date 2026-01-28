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
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/go-chi/chi/v5"
)

// SubmodelRepositoryAPIAPIController binds http requests to an api service and writes the service results to the http response
type SubmodelRepositoryAPIAPIController struct {
	service      SubmodelRepositoryAPIAPIServicer
	errorHandler ErrorHandler
	contextPath  string
}

// SubmodelRepositoryAPIAPIOption for how the controller is set up.
type SubmodelRepositoryAPIAPIOption func(*SubmodelRepositoryAPIAPIController)

// WithSubmodelRepositoryAPIAPIErrorHandler inject ErrorHandler into controller
func WithSubmodelRepositoryAPIAPIErrorHandler(h ErrorHandler) SubmodelRepositoryAPIAPIOption {
	return func(c *SubmodelRepositoryAPIAPIController) {
		c.errorHandler = h
	}
}

// NewSubmodelRepositoryAPIAPIController creates a default api controller
func NewSubmodelRepositoryAPIAPIController(s SubmodelRepositoryAPIAPIServicer, contextPath string, opts ...SubmodelRepositoryAPIAPIOption) *SubmodelRepositoryAPIAPIController {
	controller := &SubmodelRepositoryAPIAPIController{
		service:      s,
		errorHandler: DefaultErrorHandler,
		contextPath:  contextPath,
	}

	for _, opt := range opts {
		opt(controller)
	}

	return controller
}

// Routes returns all the api routes for the SubmodelRepositoryAPIAPIController
func (c *SubmodelRepositoryAPIAPIController) Routes() Routes {
	return Routes{
		"QuerySubmodels": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/query/submodels",
			c.QuerySubmodels,
		},
		"GetAllSubmodels": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels",
			c.GetAllSubmodels,
		},
		"PostSubmodel": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/submodels",
			c.PostSubmodel,
		},
		"GetAllSubmodelsMetadata": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/$metadata",
			c.GetAllSubmodelsMetadata,
		},
		"GetAllSubmodelsValueOnly": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/$value",
			c.GetAllSubmodelsValueOnly,
		},
		"GetAllSubmodelsReference": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/$reference",
			c.GetAllSubmodelsReference,
		},
		"GetAllSubmodelsPath": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/$path",
			c.GetAllSubmodelsPath,
		},
		"GetSubmodelById": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}",
			c.GetSubmodelByID,
		},
		"PutSubmodelById": Route{
			strings.ToUpper("Put"),
			c.contextPath + "/submodels/{submodelIdentifier}",
			c.PutSubmodelByID,
		},
		"DeleteSubmodelById": Route{
			strings.ToUpper("Delete"),
			c.contextPath + "/submodels/{submodelIdentifier}",
			c.DeleteSubmodelByID,
		},
		"PatchSubmodelById": Route{
			strings.ToUpper("Patch"),
			c.contextPath + "/submodels/{submodelIdentifier}",
			c.PatchSubmodelByID,
		},
		"GetSubmodelByIDMetadata": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/$metadata",
			c.GetSubmodelByIDMetadata,
		},
		"PatchSubmodelByIDMetadata": Route{
			strings.ToUpper("Patch"),
			c.contextPath + "/submodels/{submodelIdentifier}/$metadata",
			c.PatchSubmodelByIDMetadata,
		},
		"GetSubmodelByIDValueOnly": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/$value",
			c.GetSubmodelByIDValueOnly,
		},
		"PatchSubmodelByIDValueOnly": Route{
			strings.ToUpper("Patch"),
			c.contextPath + "/submodels/{submodelIdentifier}/$value",
			c.PatchSubmodelByIDValueOnly,
		},
		"GetSubmodelByIDReference": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/$reference",
			c.GetSubmodelByIDReference,
		},
		"GetSubmodelByIDPath": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/$path",
			c.GetSubmodelByIDPath,
		},
		"GetAllSubmodelElements": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements",
			c.GetAllSubmodelElements,
		},
		"PostSubmodelElementSubmodelRepo": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements",
			c.PostSubmodelElementSubmodelRepo,
		},
		"GetAllSubmodelElementsMetadataSubmodelRepo": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/$metadata",
			c.GetAllSubmodelElementsMetadataSubmodelRepo,
		},
		"GetAllSubmodelElementsValueOnlySubmodelRepo": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/$value",
			c.GetAllSubmodelElementsValueOnlySubmodelRepo,
		},
		"GetAllSubmodelElementsReferenceSubmodelRepo": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/$reference",
			c.GetAllSubmodelElementsReferenceSubmodelRepo,
		},
		"GetAllSubmodelElementsPathSubmodelRepo": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/$path",
			c.GetAllSubmodelElementsPathSubmodelRepo,
		},
		"GetSubmodelElementByPathSubmodelRepo": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}",
			c.GetSubmodelElementByPathSubmodelRepo,
		},
		"PutSubmodelElementByPathSubmodelRepo": Route{
			strings.ToUpper("Put"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}",
			c.PutSubmodelElementByPathSubmodelRepo,
		},
		"PostSubmodelElementByPathSubmodelRepo": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}",
			c.PostSubmodelElementByPathSubmodelRepo,
		},
		"DeleteSubmodelElementByPathSubmodelRepo": Route{
			strings.ToUpper("Delete"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}",
			c.DeleteSubmodelElementByPathSubmodelRepo,
		},
		"PatchSubmodelElementByPathSubmodelRepo": Route{
			strings.ToUpper("Patch"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}",
			c.PatchSubmodelElementByPathSubmodelRepo,
		},
		"GetSubmodelElementByPathMetadataSubmodelRepo": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$metadata",
			c.GetSubmodelElementByPathMetadataSubmodelRepo,
		},
		"PatchSubmodelElementByPathMetadataSubmodelRepo": Route{
			strings.ToUpper("Patch"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$metadata",
			c.PatchSubmodelElementByPathMetadataSubmodelRepo,
		},
		"GetSubmodelElementByPathValueOnlySubmodelRepo": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$value",
			c.GetSubmodelElementByPathValueOnlySubmodelRepo,
		},
		"PatchSubmodelElementByPathValueOnlySubmodelRepo": Route{
			strings.ToUpper("Patch"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$value",
			c.PatchSubmodelElementByPathValueOnlySubmodelRepo,
		},
		"GetSubmodelElementByPathReferenceSubmodelRepo": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$reference",
			c.GetSubmodelElementByPathReferenceSubmodelRepo,
		},
		"GetSubmodelElementByPathPathSubmodelRepo": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$path",
			c.GetSubmodelElementByPathPathSubmodelRepo,
		},
		"GetFileByPathSubmodelRepo": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment",
			c.GetFileByPathSubmodelRepo,
		},
		"PutFileByPathSubmodelRepo": Route{
			strings.ToUpper("Put"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment",
			c.PutFileByPathSubmodelRepo,
		},
		"DeleteFileByPathSubmodelRepo": Route{
			strings.ToUpper("Delete"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment",
			c.DeleteFileByPathSubmodelRepo,
		},
		"InvokeOperationSubmodelRepo": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke",
			c.InvokeOperationSubmodelRepo,
		},
		"InvokeOperationValueOnly": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke/$value",
			c.InvokeOperationValueOnly,
		},
		"InvokeOperationAsync": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke-async",
			c.InvokeOperationAsync,
		},
		"InvokeOperationAsyncValueOnly": Route{
			strings.ToUpper("Post"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke-async/$value",
			c.InvokeOperationAsyncValueOnly,
		},
		"GetOperationAsyncStatus": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-status/{handleId}",
			c.GetOperationAsyncStatus,
		},
		"GetOperationAsyncResult": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-results/{handleId}",
			c.GetOperationAsyncResult,
		},
		"GetOperationAsyncResultValueOnly": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-results/{handleId}/$value",
			c.GetOperationAsyncResultValueOnly,
		},
		"GetSignedSubmodelByID": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/$signed",
			c.GetSignedSubmodelByID,
		},
		"GetSignedSubmodelByIDValueOnly": Route{
			strings.ToUpper("Get"),
			c.contextPath + "/submodels/{submodelIdentifier}/$value/$signed",
			c.GetSignedSubmodelByIDValueOnly,
		},
	}
}

// GetAllSubmodels - Returns all Submodels
func (c *SubmodelRepositoryAPIAPIController) GetAllSubmodels(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	var semanticIDParam string
	if query.Has("semanticId") {
		param := query.Get("semanticId")

		semanticIDParam = param
	}

	var idShortParam string
	if query.Has("idShort") {
		param := query.Get("idShort")

		idShortParam = param
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

	var cursorParam string
	if query.Has("cursor") {
		param := query.Get("cursor")

		cursorParam = param
	}

	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	var extentParam string
	if query.Has("extent") {
		param := query.Get("extent")

		extentParam = param
	} else {
		param := "withoutBlobValue"
		extentParam = param
	}
	result, err := c.service.GetAllSubmodels(r.Context(), semanticIDParam, idShortParam, limitParam, cursorParam, levelParam, extentParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PostSubmodel - Creates a new Submodel
func (c *SubmodelRepositoryAPIAPIController) PostSubmodel(w http.ResponseWriter, r *http.Request) {
	var submodelParam model.Submodel
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&submodelParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	if err := model.AssertSubmodelRequired(submodelParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	if err := model.AssertSubmodelConstraints(submodelParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	result, err := c.service.PostSubmodel(r.Context(), submodelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelsMetadata - Returns the metadata attributes of all Submodels
func (c *SubmodelRepositoryAPIAPIController) GetAllSubmodelsMetadata(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	var semanticIDParam string
	if query.Has("semanticId") {
		param := query.Get("semanticId")

		semanticIDParam = param
	}

	var idShortParam string
	if query.Has("idShort") {
		param := query.Get("idShort")

		idShortParam = param
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

	var cursorParam string
	if query.Has("cursor") {
		param := query.Get("cursor")

		cursorParam = param
	}

	result, err := c.service.GetAllSubmodelsMetadata(r.Context(), semanticIDParam, idShortParam, limitParam, cursorParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelsValueOnly - Returns all Submodels in their ValueOnly representation
func (c *SubmodelRepositoryAPIAPIController) GetAllSubmodelsValueOnly(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	var semanticIDParam string
	if query.Has("semanticId") {
		param := query.Get("semanticId")

		semanticIDParam = param
	}

	var idShortParam string
	if query.Has("idShort") {
		param := query.Get("idShort")

		idShortParam = param
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

	var cursorParam string
	if query.Has("cursor") {
		param := query.Get("cursor")

		cursorParam = param
	}

	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	var extentParam string
	if query.Has("extent") {
		param := query.Get("extent")

		extentParam = param
	} else {
		param := "withoutBlobValue"
		extentParam = param
	}
	result, err := c.service.GetAllSubmodelsValueOnly(r.Context(), semanticIDParam, idShortParam, limitParam, cursorParam, levelParam, extentParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelsReference - Returns the References for all Submodels
func (c *SubmodelRepositoryAPIAPIController) GetAllSubmodelsReference(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	var semanticIDParam string
	if query.Has("semanticId") {
		param := query.Get("semanticId")

		semanticIDParam = param
	}

	var idShortParam string
	if query.Has("idShort") {
		param := query.Get("idShort")

		idShortParam = param
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

	var cursorParam string
	if query.Has("cursor") {
		param := query.Get("cursor")

		cursorParam = param
	}

	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "core"
		levelParam = param
	}
	result, err := c.service.GetAllSubmodelsReference(r.Context(), semanticIDParam, idShortParam, limitParam, cursorParam, levelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelsPath - Returns all Submodels in the Path notation
func (c *SubmodelRepositoryAPIAPIController) GetAllSubmodelsPath(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	var semanticIDParam string
	if query.Has("semanticId") {
		param := query.Get("semanticId")

		semanticIDParam = param
	}

	var idShortParam string
	if query.Has("idShort") {
		param := query.Get("idShort")

		idShortParam = param
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

	var cursorParam string
	if query.Has("cursor") {
		param := query.Get("cursor")

		cursorParam = param
	}

	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	result, err := c.service.GetAllSubmodelsPath(r.Context(), semanticIDParam, idShortParam, limitParam, cursorParam, levelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelByID - Returns a specific Submodel
func (c *SubmodelRepositoryAPIAPIController) GetSubmodelByID(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	var extentParam string
	if query.Has("extent") {
		param := query.Get("extent")

		extentParam = param
	} else {
		param := "withoutBlobValue"
		extentParam = param
	}
	result, err := c.service.GetSubmodelByID(r.Context(), submodelIdentifierParam, levelParam, extentParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSignedSubmodelByID - Returns a specific Submodel
func (c *SubmodelRepositoryAPIAPIController) GetSignedSubmodelByID(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	var extentParam string
	if query.Has("extent") {
		param := query.Get("extent")

		extentParam = param
	} else {
		param := "withoutBlobValue"
		extentParam = param
	}
	result, err := c.service.GetSignedSubmodelByID(r.Context(), submodelIdentifierParam, levelParam, extentParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSignedSubmodelByIDValueOnly - Returns a specific Submodel in ValueOnly representation
func (c *SubmodelRepositoryAPIAPIController) GetSignedSubmodelByIDValueOnly(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	var extentParam string
	if query.Has("extent") {
		param := query.Get("extent")

		extentParam = param
	} else {
		param := "withoutBlobValue"
		extentParam = param
	}
	result, err := c.service.GetSignedSubmodelByIDValueOnly(r.Context(), submodelIdentifierParam, levelParam, extentParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PutSubmodelByID - Updates an existing Submodel
func (c *SubmodelRepositoryAPIAPIController) PutSubmodelByID(w http.ResponseWriter, r *http.Request) {
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}
	var submodelParam model.Submodel
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&submodelParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	if err := model.AssertSubmodelRequired(submodelParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	if err := model.AssertSubmodelConstraints(submodelParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	result, err := c.service.PutSubmodelByID(r.Context(), submodelIdentifierParam, submodelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// DeleteSubmodelByID - Deletes a Submodel
func (c *SubmodelRepositoryAPIAPIController) DeleteSubmodelByID(w http.ResponseWriter, r *http.Request) {
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}
	result, err := c.service.DeleteSubmodelByID(r.Context(), submodelIdentifierParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PatchSubmodelByID - Updates an existing Submodel
func (c *SubmodelRepositoryAPIAPIController) PatchSubmodelByID(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}
	var submodelParam model.Submodel
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&submodelParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	if err := model.AssertSubmodelRequired(submodelParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	if err := model.AssertSubmodelConstraints(submodelParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "core"
		levelParam = param
	}
	result, err := c.service.PatchSubmodelByID(r.Context(), submodelIdentifierParam, submodelParam, levelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelByIDMetadata - Returns the metadata attributes of a specific Submodel
func (c *SubmodelRepositoryAPIAPIController) GetSubmodelByIDMetadata(w http.ResponseWriter, r *http.Request) {
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}
	result, err := c.service.GetSubmodelByIDMetadata(r.Context(), submodelIdentifierParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PatchSubmodelByIDMetadata - Updates the metadata attributes of an existing Submodel
func (c *SubmodelRepositoryAPIAPIController) PatchSubmodelByIDMetadata(w http.ResponseWriter, r *http.Request) {
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}
	var submodelMetadataParam model.SubmodelMetadata
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&submodelMetadataParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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
	result, err := c.service.PatchSubmodelByIDMetadata(r.Context(), submodelIdentifierParam, submodelMetadataParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelByIDValueOnly - Returns a specific Submodel in the ValueOnly representation
func (c *SubmodelRepositoryAPIAPIController) GetSubmodelByIDValueOnly(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	var extentParam string
	if query.Has("extent") {
		param := query.Get("extent")

		extentParam = param
	} else {
		param := "withoutBlobValue"
		extentParam = param
	}
	result, err := c.service.GetSubmodelByIDValueOnly(r.Context(), submodelIdentifierParam, levelParam, extentParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PatchSubmodelByIDValueOnly - Updates the values of an existing Submodel
func (c *SubmodelRepositoryAPIAPIController) PatchSubmodelByIDValueOnly(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}
	var bodyParam model.SubmodelValue
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&bodyParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "core"
		levelParam = param
	}
	result, err := c.service.PatchSubmodelByIDValueOnly(r.Context(), submodelIdentifierParam, bodyParam, levelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelByIDReference - Returns the Reference of a specific Submodel
func (c *SubmodelRepositoryAPIAPIController) GetSubmodelByIDReference(w http.ResponseWriter, r *http.Request) {
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}
	result, err := c.service.GetSubmodelByIDReference(r.Context(), submodelIdentifierParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelByIDPath - Returns a specific Submodel in the Path notation
func (c *SubmodelRepositoryAPIAPIController) GetSubmodelByIDPath(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	result, err := c.service.GetSubmodelByIDPath(r.Context(), submodelIdentifierParam, levelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelElements - Returns all submodel elements including their hierarchy
func (c *SubmodelRepositoryAPIAPIController) GetAllSubmodelElements(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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

	var cursorParam string
	if query.Has("cursor") {
		param := query.Get("cursor")

		cursorParam = param
	}

	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	var extentParam string
	if query.Has("extent") {
		param := query.Get("extent")

		extentParam = param
	} else {
		param := "withoutBlobValue"
		extentParam = param
	}
	result, err := c.service.GetAllSubmodelElements(r.Context(), submodelIdentifierParam, limitParam, cursorParam, levelParam, extentParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PostSubmodelElementSubmodelRepo - Creates a new submodel element
func (c *SubmodelRepositoryAPIAPIController) PostSubmodelElementSubmodelRepo(w http.ResponseWriter, r *http.Request) {
	submodelIdentifierParam := chi.URLParam(r, "submodelIdentifier")
	if submodelIdentifierParam == "" {
		c.errorHandler(w, r, &RequiredError{"submodelIdentifier"}, nil)
		return
	}

	// Read the raw JSON data from the request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	// Use our custom factory function to create the appropriate concrete type
	submodelElementParam, err := model.UnmarshalSubmodelElement(bodyBytes)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	// FIXED: Use custom validation that handles optional references correctly
	if err := model.AssertSubmodelElementRequiredFixed(submodelElementParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	if err := model.AssertSubmodelElementConstraintsFixed(submodelElementParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	result, err := c.service.PostSubmodelElementSubmodelRepo(r.Context(), submodelIdentifierParam, submodelElementParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelElementsMetadataSubmodelRepo - Returns the metadata attributes of all submodel elements including their hierarchy
func (c *SubmodelRepositoryAPIAPIController) GetAllSubmodelElementsMetadataSubmodelRepo(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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

	var cursorParam string
	if query.Has("cursor") {
		param := query.Get("cursor")

		cursorParam = param
	}

	result, err := c.service.GetAllSubmodelElementsMetadataSubmodelRepo(r.Context(), submodelIdentifierParam, limitParam, cursorParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelElementsValueOnlySubmodelRepo - Returns all submodel elements including their hierarchy in the ValueOnly representation
func (c *SubmodelRepositoryAPIAPIController) GetAllSubmodelElementsValueOnlySubmodelRepo(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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

	var cursorParam string
	if query.Has("cursor") {
		param := query.Get("cursor")

		cursorParam = param
	}

	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	var extentParam string
	if query.Has("extent") {
		param := query.Get("extent")

		extentParam = param
	} else {
		param := "withoutBlobValue"
		extentParam = param
	}
	result, err := c.service.GetAllSubmodelElementsValueOnlySubmodelRepo(r.Context(), submodelIdentifierParam, limitParam, cursorParam, levelParam, extentParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelElementsReferenceSubmodelRepo - Returns the References of all submodel elements
func (c *SubmodelRepositoryAPIAPIController) GetAllSubmodelElementsReferenceSubmodelRepo(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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

	var cursorParam string
	if query.Has("cursor") {
		param := query.Get("cursor")

		cursorParam = param
	}

	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "core"
		levelParam = param
	}
	result, err := c.service.GetAllSubmodelElementsReferenceSubmodelRepo(r.Context(), submodelIdentifierParam, limitParam, cursorParam, levelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetAllSubmodelElementsPathSubmodelRepo - Returns all submodel elements including their hierarchy in the Path notation
func (c *SubmodelRepositoryAPIAPIController) GetAllSubmodelElementsPathSubmodelRepo(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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

	var cursorParam string
	if query.Has("cursor") {
		param := query.Get("cursor")

		cursorParam = param
	}

	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	result, err := c.service.GetAllSubmodelElementsPathSubmodelRepo(r.Context(), submodelIdentifierParam, limitParam, cursorParam, levelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelElementByPathSubmodelRepo - Returns a specific submodel element from the Submodel at a specified path
func (c *SubmodelRepositoryAPIAPIController) GetSubmodelElementByPathSubmodelRepo(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	var extentParam string
	if query.Has("extent") {
		param := query.Get("extent")

		extentParam = param
	} else {
		param := "withoutBlobValue"
		extentParam = param
	}
	result, err := c.service.GetSubmodelElementByPathSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam, levelParam, extentParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PutSubmodelElementByPathSubmodelRepo - Updates an existing submodel element at a specified path within submodel elements hierarchy
func (c *SubmodelRepositoryAPIAPIController) PutSubmodelElementByPathSubmodelRepo(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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

	// Read the raw JSON data from the request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	// Use our custom factory function to create the appropriate concrete type
	submodelElementParam, err := model.UnmarshalSubmodelElement(bodyBytes)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	if err := model.AssertSubmodelElementRequired(submodelElementParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	if err := model.AssertSubmodelElementConstraints(submodelElementParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	result, err := c.service.PutSubmodelElementByPathSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam, submodelElementParam, levelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PostSubmodelElementByPathSubmodelRepo - Creates a new submodel element at a specified path within submodel elements hierarchy
func (c *SubmodelRepositoryAPIAPIController) PostSubmodelElementByPathSubmodelRepo(w http.ResponseWriter, r *http.Request) {
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

	// Read the raw JSON data from the request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	// Use our custom factory function to create the appropriate concrete type
	submodelElementParam, err := model.UnmarshalSubmodelElement(bodyBytes)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	if err := model.AssertSubmodelElementRequired(submodelElementParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	if err := model.AssertSubmodelElementConstraints(submodelElementParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	result, err := c.service.PostSubmodelElementByPathSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam, submodelElementParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// DeleteSubmodelElementByPathSubmodelRepo - Deletes a submodel element at a specified path within the submodel elements hierarchy
func (c *SubmodelRepositoryAPIAPIController) DeleteSubmodelElementByPathSubmodelRepo(w http.ResponseWriter, r *http.Request) {
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
	result, err := c.service.DeleteSubmodelElementByPathSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PatchSubmodelElementByPathSubmodelRepo - Updates an existing SubmodelElement
func (c *SubmodelRepositoryAPIAPIController) PatchSubmodelElementByPathSubmodelRepo(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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

	// Read the raw JSON data from the request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}

	// Use our custom factory function to create the appropriate concrete type
	submodelElementParam, err := model.UnmarshalSubmodelElement(bodyBytes)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	if err := model.AssertSubmodelElementRequired(submodelElementParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	if err := model.AssertSubmodelElementConstraints(submodelElementParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "core"
		levelParam = param
	}
	result, err := c.service.PatchSubmodelElementByPathSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam, submodelElementParam, levelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelElementByPathMetadataSubmodelRepo - Returns the matadata attributes of a specific submodel element from the Submodel at a specified path
func (c *SubmodelRepositoryAPIAPIController) GetSubmodelElementByPathMetadataSubmodelRepo(w http.ResponseWriter, r *http.Request) {
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
	result, err := c.service.GetSubmodelElementByPathMetadataSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PatchSubmodelElementByPathMetadataSubmodelRepo - Updates the metadata attributes an existing SubmodelElement
func (c *SubmodelRepositoryAPIAPIController) PatchSubmodelElementByPathMetadataSubmodelRepo(w http.ResponseWriter, r *http.Request) {
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
	var submodelElementMetadataParam model.SubmodelElementMetadata
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&submodelElementMetadataParam); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	if err := model.AssertSubmodelElementMetadataRequired(submodelElementMetadataParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	if err := model.AssertSubmodelElementMetadataConstraints(submodelElementMetadataParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	result, err := c.service.PatchSubmodelElementByPathMetadataSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam, submodelElementMetadataParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelElementByPathValueOnlySubmodelRepo - Returns a specific submodel element from the Submodel at a specified path in the ValueOnly representation
func (c *SubmodelRepositoryAPIAPIController) GetSubmodelElementByPathValueOnlySubmodelRepo(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	var extentParam string
	if query.Has("extent") {
		param := query.Get("extent")

		extentParam = param
	} else {
		param := "withoutBlobValue"
		extentParam = param
	}
	result, err := c.service.GetSubmodelElementByPathValueOnlySubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam, levelParam, extentParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PatchSubmodelElementByPathValueOnlySubmodelRepo - Updates the value of an existing SubmodelElement
func (c *SubmodelRepositoryAPIAPIController) PatchSubmodelElementByPathValueOnlySubmodelRepo(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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
	// SubmodelElementValue is an interface, deserialize from JSON
	body, err := io.ReadAll(r.Body)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	submodelElementValueParam, err := model.UnmarshalSubmodelElementValue(body)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
		return
	}
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "core"
		levelParam = param
	}
	result, err := c.service.PatchSubmodelElementByPathValueOnlySubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam, submodelElementValueParam, levelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelElementByPathReferenceSubmodelRepo - Returns the Reference of a specific submodel element from the Submodel at a specified path
func (c *SubmodelRepositoryAPIAPIController) GetSubmodelElementByPathReferenceSubmodelRepo(w http.ResponseWriter, r *http.Request) {
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
	result, err := c.service.GetSubmodelElementByPathReferenceSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetSubmodelElementByPathPathSubmodelRepo - Returns a specific submodel element from the Submodel at a specified path in the Path notation
func (c *SubmodelRepositoryAPIAPIController) GetSubmodelElementByPathPathSubmodelRepo(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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
	var levelParam string
	if query.Has("level") {
		param := query.Get("level")

		levelParam = param
	} else {
		param := "deep"
		levelParam = param
	}
	result, err := c.service.GetSubmodelElementByPathPathSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam, levelParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetFileByPathSubmodelRepo - Downloads file content from a specific submodel element from the Submodel at a specified path
func (c *SubmodelRepositoryAPIAPIController) GetFileByPathSubmodelRepo(w http.ResponseWriter, r *http.Request) {
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
	result, err := c.service.GetFileByPathSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// PutFileByPathSubmodelRepo - Uploads file content to an existing submodel element at a specified path within submodel elements hierarchy
func (c *SubmodelRepositoryAPIAPIController) PutFileByPathSubmodelRepo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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

	result, err := c.service.PutFileByPathSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam, fileNameParam, fileParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// DeleteFileByPathSubmodelRepo - Deletes file content of an existing submodel element at a specified path within submodel elements hierarchy
func (c *SubmodelRepositoryAPIAPIController) DeleteFileByPathSubmodelRepo(w http.ResponseWriter, r *http.Request) {
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
	result, err := c.service.DeleteFileByPathSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// InvokeOperationSubmodelRepo - Synchronously or asynchronously invokes an Operation at a specified path
func (c *SubmodelRepositoryAPIAPIController) InvokeOperationSubmodelRepo(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		c.errorHandler(w, r, &ParsingError{Err: err}, nil)
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
	if err := model.AssertOperationRequestRequired(operationRequestParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	if err := model.AssertOperationRequestConstraints(operationRequestParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	var asyncParam bool
	if query.Has("async") {
		param, err := parseBoolParameter(
			query.Get("async"),
			WithParse[bool](parseBool),
		)
		if err != nil {
			c.errorHandler(w, r, &ParsingError{Param: "async", Err: err}, nil)
			return
		}

		asyncParam = param
	} else {
		var param = false
		asyncParam = param
	}
	result, err := c.service.InvokeOperationSubmodelRepo(r.Context(), submodelIdentifierParam, idShortPathParam, operationRequestParam, asyncParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// InvokeOperationValueOnly - Synchronously or asynchronously invokes an Operation at a specified path
func (c *SubmodelRepositoryAPIAPIController) InvokeOperationValueOnly(w http.ResponseWriter, r *http.Request) {
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
	var asyncParam bool
	if query.Has("async") {
		param, err := parseBoolParameter(
			query.Get("async"),
			WithParse[bool](parseBool),
		)
		if err != nil {
			c.errorHandler(w, r, &ParsingError{Param: "async", Err: err}, nil)
			return
		}

		asyncParam = param
	} else {
		var param = false
		asyncParam = param
	}
	result, err := c.service.InvokeOperationValueOnly(r.Context(), aasIdentifierParam, submodelIdentifierParam, idShortPathParam, operationRequestValueOnlyParam, asyncParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// InvokeOperationAsync - Asynchronously invokes an Operation at a specified path
func (c *SubmodelRepositoryAPIAPIController) InvokeOperationAsync(w http.ResponseWriter, r *http.Request) {
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
	if err := model.AssertOperationRequestRequired(operationRequestParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	if err := model.AssertOperationRequestConstraints(operationRequestParam); err != nil {
		c.errorHandler(w, r, err, nil)
		return
	}
	result, err := c.service.InvokeOperationAsync(r.Context(), submodelIdentifierParam, idShortPathParam, operationRequestParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// InvokeOperationAsyncValueOnly - Asynchronously invokes an Operation at a specified path
func (c *SubmodelRepositoryAPIAPIController) InvokeOperationAsyncValueOnly(w http.ResponseWriter, r *http.Request) {
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
	result, err := c.service.InvokeOperationAsyncValueOnly(r.Context(), aasIdentifierParam, submodelIdentifierParam, idShortPathParam, operationRequestValueOnlyParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetOperationAsyncStatus - Returns the status of an asynchronously invoked Operation
func (c *SubmodelRepositoryAPIAPIController) GetOperationAsyncStatus(w http.ResponseWriter, r *http.Request) {
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
	result, err := c.service.GetOperationAsyncStatus(r.Context(), submodelIdentifierParam, idShortPathParam, handleIDParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetOperationAsyncResult - Returns the Operation result of an asynchronously invoked Operation
func (c *SubmodelRepositoryAPIAPIController) GetOperationAsyncResult(w http.ResponseWriter, r *http.Request) {
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
	result, err := c.service.GetOperationAsyncResult(r.Context(), submodelIdentifierParam, idShortPathParam, handleIDParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

// GetOperationAsyncResultValueOnly - Returns the Operation result of an asynchronously invoked Operation
func (c *SubmodelRepositoryAPIAPIController) GetOperationAsyncResultValueOnly(w http.ResponseWriter, r *http.Request) {
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
	result, err := c.service.GetOperationAsyncResultValueOnly(r.Context(), submodelIdentifierParam, idShortPathParam, handleIDParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}

const componentName = "SubmodelRepository"

// QuerySubmodels - Returns all Submodels that match the input query
func (c *SubmodelRepositoryAPIAPIController) QuerySubmodels(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		log.Printf("🧩 [%s] Error in QuerySubmodels: parse query raw=%q: %v", componentName, r.URL.RawQuery, err)
		result := common.NewErrorResponse(
			err,
			http.StatusBadRequest,
			componentName,
			"QuerySubmodels",
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
			log.Printf("🧩 [%s] Error in QuerySubmodels: parse limit=%q: %v", componentName, query.Get("limit"), err)
			result := common.NewErrorResponse(
				err,
				http.StatusBadRequest,
				componentName,
				"QuerySubmodels",
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
	var queryParam grammar.Query
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&queryParam); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("🧩 [%s] Error in QuerySubmodels: decode body: %v", componentName, err)
		result := common.NewErrorResponse(
			err,
			http.StatusBadRequest,
			componentName,
			"QuerySubmodels",
			"RequestBody",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}
	if err := grammar.AssertQueryRequired(queryParam); err != nil {
		log.Printf("🧩 [%s] Error in QuerySubmodels: required validation failed: %v", componentName, err)
		result := common.NewErrorResponse(
			err,
			http.StatusBadRequest,
			componentName,
			"QuerySubmodels",
			"RequestBody",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}
	if err := grammar.AssertQueryConstraints(queryParam); err != nil {
		log.Printf("🧩 [%s] Error in QuerySubmodels: constraints validation failed: %v", componentName, err)
		result := common.NewErrorResponse(
			err,
			http.StatusBadRequest,
			componentName,
			"QuerySubmodels",
			"RequestBody",
		)
		EncodeJSONResponse(result.Body, &result.Code, w)
		return
	}
	result, err := c.service.QuerySubmodels(r.Context(), limitParam, cursorParam, queryParam)
	// If an error occurred, encode the error with the status code
	if err != nil {
		log.Printf("🧩 [%s] Error in QuerySubmodels: service failure (limit=%d cursor=%q): %v", componentName, limitParam, cursorParam, err)
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}
