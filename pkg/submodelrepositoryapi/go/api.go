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
	"net/http"
	"os"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// DescriptionAPIAPIRouter defines the required methods for binding the api requests to a responses for the DescriptionAPIAPI
// The DescriptionAPIAPIRouter implementation should parse necessary information from the http request,
// pass the data to a DescriptionAPIAPIServicer to perform the required actions, then write the service results to the http response.
type DescriptionAPIAPIRouter interface {
	GetDescription(http.ResponseWriter, *http.Request)
}

// SerializationAPIAPIRouter defines the required methods for binding the api requests to a responses for the SerializationAPIAPI
// The SerializationAPIAPIRouter implementation should parse necessary information from the http request,
// pass the data to a SerializationAPIAPIServicer to perform the required actions, then write the service results to the http response.
type SerializationAPIAPIRouter interface {
	GenerateSerializationByIds(http.ResponseWriter, *http.Request)
}

// SubmodelRepositoryAPIAPIRouter defines the required methods for binding the api requests to a responses for the SubmodelRepositoryAPIAPI
// The SubmodelRepositoryAPIAPIRouter implementation should parse necessary information from the http request,
// pass the data to a SubmodelRepositoryAPIAPIServicer to perform the required actions, then write the service results to the http response.
type SubmodelRepositoryAPIAPIRouter interface {
	GetAllSubmodels(http.ResponseWriter, *http.Request)
	PostSubmodel(http.ResponseWriter, *http.Request)
	GetAllSubmodelsMetadata(http.ResponseWriter, *http.Request)
	GetAllSubmodelsValueOnly(http.ResponseWriter, *http.Request)
	GetAllSubmodelsReference(http.ResponseWriter, *http.Request)
	GetAllSubmodelsPath(http.ResponseWriter, *http.Request)
	GetSubmodelById(http.ResponseWriter, *http.Request)
	PutSubmodelById(http.ResponseWriter, *http.Request)
	DeleteSubmodelById(http.ResponseWriter, *http.Request)
	PatchSubmodelById(http.ResponseWriter, *http.Request)
	GetSubmodelByIdMetadata(http.ResponseWriter, *http.Request)
	PatchSubmodelByIdMetadata(http.ResponseWriter, *http.Request)
	GetSubmodelByIdValueOnly(http.ResponseWriter, *http.Request)
	PatchSubmodelByIdValueOnly(http.ResponseWriter, *http.Request)
	GetSubmodelByIdReference(http.ResponseWriter, *http.Request)
	GetSubmodelByIdPath(http.ResponseWriter, *http.Request)
	GetAllSubmodelElements(http.ResponseWriter, *http.Request)
	PostSubmodelElementSubmodelRepo(http.ResponseWriter, *http.Request)
	GetAllSubmodelElementsMetadataSubmodelRepo(http.ResponseWriter, *http.Request)
	GetAllSubmodelElementsValueOnlySubmodelRepo(http.ResponseWriter, *http.Request)
	GetAllSubmodelElementsReferenceSubmodelRepo(http.ResponseWriter, *http.Request)
	GetAllSubmodelElementsPathSubmodelRepo(http.ResponseWriter, *http.Request)
	GetSubmodelElementByPathSubmodelRepo(http.ResponseWriter, *http.Request)
	PutSubmodelElementByPathSubmodelRepo(http.ResponseWriter, *http.Request)
	PostSubmodelElementByPathSubmodelRepo(http.ResponseWriter, *http.Request)
	DeleteSubmodelElementByPathSubmodelRepo(http.ResponseWriter, *http.Request)
	PatchSubmodelElementByPathSubmodelRepo(http.ResponseWriter, *http.Request)
	GetSubmodelElementByPathMetadataSubmodelRepo(http.ResponseWriter, *http.Request)
	PatchSubmodelElementByPathMetadataSubmodelRepo(http.ResponseWriter, *http.Request)
	GetSubmodelElementByPathValueOnlySubmodelRepo(http.ResponseWriter, *http.Request)
	PatchSubmodelElementByPathValueOnlySubmodelRepo(http.ResponseWriter, *http.Request)
	GetSubmodelElementByPathReferenceSubmodelRepo(http.ResponseWriter, *http.Request)
	GetSubmodelElementByPathPathSubmodelRepo(http.ResponseWriter, *http.Request)
	GetFileByPathSubmodelRepo(http.ResponseWriter, *http.Request)
	PutFileByPathSubmodelRepo(http.ResponseWriter, *http.Request)
	DeleteFileByPathSubmodelRepo(http.ResponseWriter, *http.Request)
	InvokeOperationSubmodelRepo(http.ResponseWriter, *http.Request)
	InvokeOperationValueOnly(http.ResponseWriter, *http.Request)
	InvokeOperationAsync(http.ResponseWriter, *http.Request)
	InvokeOperationAsyncValueOnly(http.ResponseWriter, *http.Request)
	GetOperationAsyncStatus(http.ResponseWriter, *http.Request)
	GetOperationAsyncResult(http.ResponseWriter, *http.Request)
	GetOperationAsyncResultValueOnly(http.ResponseWriter, *http.Request)
}

// DescriptionAPIAPIServicer defines the api actions for the DescriptionAPIAPI service
// This interface intended to stay up to date with the openapi yaml used to generate it,
// while the service implementation can be ignored with the .openapi-generator-ignore file
// and updated with the logic required for the API.
type DescriptionAPIAPIServicer interface {
	GetDescription(context.Context) (model.ImplResponse, error)
}

// SerializationAPIAPIServicer defines the api actions for the SerializationAPIAPI service
// This interface intended to stay up to date with the openapi yaml used to generate it,
// while the service implementation can be ignored with the .openapi-generator-ignore file
// and updated with the logic required for the API.
type SerializationAPIAPIServicer interface {
	GenerateSerializationByIds(context.Context, []string, []string, bool) (model.ImplResponse, error)
}

// SubmodelRepositoryAPIAPIServicer defines the api actions for the SubmodelRepositoryAPIAPI service
// This interface intended to stay up to date with the openapi yaml used to generate it,
// while the service implementation can be ignored with the .openapi-generator-ignore file
// and updated with the logic required for the API.
type SubmodelRepositoryAPIAPIServicer interface {
	GetAllSubmodels(context.Context, string, string, int32, string, string, string) (model.ImplResponse, error)
	PostSubmodel(context.Context, model.Submodel) (model.ImplResponse, error)
	GetAllSubmodelsMetadata(context.Context, string, string, int32, string) (model.ImplResponse, error)
	GetAllSubmodelsValueOnly(context.Context, string, string, int32, string, string, string) (model.ImplResponse, error)
	GetAllSubmodelsReference(context.Context, string, string, int32, string, string) (model.ImplResponse, error)
	GetAllSubmodelsPath(context.Context, string, string, int32, string, string) (model.ImplResponse, error)
	GetSubmodelById(context.Context, string, string, string) (model.ImplResponse, error)
	PutSubmodelById(context.Context, string, model.Submodel) (model.ImplResponse, error)
	DeleteSubmodelById(context.Context, string) (model.ImplResponse, error)
	PatchSubmodelById(context.Context, string, model.Submodel, string) (model.ImplResponse, error)
	GetSubmodelByIdMetadata(context.Context, string) (model.ImplResponse, error)
	PatchSubmodelByIdMetadata(context.Context, string, model.SubmodelMetadata) (model.ImplResponse, error)
	GetSubmodelByIdValueOnly(context.Context, string, string, string) (model.ImplResponse, error)
	PatchSubmodelByIdValueOnly(context.Context, string, map[string]interface{}, string) (model.ImplResponse, error)
	GetSubmodelByIdReference(context.Context, string) (model.ImplResponse, error)
	GetSubmodelByIdPath(context.Context, string, string) (model.ImplResponse, error)
	GetAllSubmodelElements(context.Context, string, int32, string, string, string) (model.ImplResponse, error)
	PostSubmodelElementSubmodelRepo(context.Context, string, model.SubmodelElement) (model.ImplResponse, error)
	GetAllSubmodelElementsMetadataSubmodelRepo(context.Context, string, int32, string) (model.ImplResponse, error)
	GetAllSubmodelElementsValueOnlySubmodelRepo(context.Context, string, int32, string, string, string) (model.ImplResponse, error)
	GetAllSubmodelElementsReferenceSubmodelRepo(context.Context, string, int32, string, string) (model.ImplResponse, error)
	GetAllSubmodelElementsPathSubmodelRepo(context.Context, string, int32, string, string) (model.ImplResponse, error)
	GetSubmodelElementByPathSubmodelRepo(context.Context, string, string, string, string) (model.ImplResponse, error)
	PutSubmodelElementByPathSubmodelRepo(context.Context, string, string, model.SubmodelElement, string) (model.ImplResponse, error)
	PostSubmodelElementByPathSubmodelRepo(context.Context, string, string, model.SubmodelElement) (model.ImplResponse, error)
	DeleteSubmodelElementByPathSubmodelRepo(context.Context, string, string) (model.ImplResponse, error)
	PatchSubmodelElementByPathSubmodelRepo(context.Context, string, string, model.SubmodelElement, string) (model.ImplResponse, error)
	GetSubmodelElementByPathMetadataSubmodelRepo(context.Context, string, string) (model.ImplResponse, error)
	PatchSubmodelElementByPathMetadataSubmodelRepo(context.Context, string, string, model.SubmodelElementMetadata) (model.ImplResponse, error)
	GetSubmodelElementByPathValueOnlySubmodelRepo(context.Context, string, string, string, string) (model.ImplResponse, error)
	PatchSubmodelElementByPathValueOnlySubmodelRepo(context.Context, string, string, model.SubmodelElementValue, string) (model.ImplResponse, error)
	GetSubmodelElementByPathReferenceSubmodelRepo(context.Context, string, string) (model.ImplResponse, error)
	GetSubmodelElementByPathPathSubmodelRepo(context.Context, string, string, string) (model.ImplResponse, error)
	GetFileByPathSubmodelRepo(context.Context, string, string) (model.ImplResponse, error)
	PutFileByPathSubmodelRepo(context.Context, string, string, string, *os.File) (model.ImplResponse, error)
	DeleteFileByPathSubmodelRepo(context.Context, string, string) (model.ImplResponse, error)
	InvokeOperationSubmodelRepo(context.Context, string, string, model.OperationRequest, bool) (model.ImplResponse, error)
	InvokeOperationValueOnly(context.Context, string, string, string, model.OperationRequestValueOnly, bool) (model.ImplResponse, error)
	InvokeOperationAsync(context.Context, string, string, model.OperationRequest) (model.ImplResponse, error)
	InvokeOperationAsyncValueOnly(context.Context, string, string, string, model.OperationRequestValueOnly) (model.ImplResponse, error)
	GetOperationAsyncStatus(context.Context, string, string, string) (model.ImplResponse, error)
	GetOperationAsyncResult(context.Context, string, string, string) (model.ImplResponse, error)
	GetOperationAsyncResultValueOnly(context.Context, string, string, string) (model.ImplResponse, error)
}
