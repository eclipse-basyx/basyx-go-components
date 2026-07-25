/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/
// Author: Christian Koort ( Fraunhofer IESE )

// Package api implements the HTTP-facing service logic for the
// Company Lookup service.
//
// This file provides an implementation of the API service
// interface and contains the business logic glue between HTTP input and the
// persistence backend (see `internal/companylookupservice/persistence`).
//
// The service is responsible for common tasks such as:
//   - decoding/validating request path and query parameters
//   - invoking the backend for CRUD operations on CompanyDescriptor objects
//   - mapping backend errors to appropriate HTTP error responses
//   - encoding paged results and response payloads
//
// Exported functionality includes the `CompanyLookupAPIService`
// type, which exposes methods for listing, creating, reading, updating and
// deleting Company Descriptors. The service expects a backend implementing
// `companylookuppostgresql.PostgreSQLCompanyLookupDatabase` that
// provides the actual persistence logic.
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	companylookuppostgresql "github.com/eclipse-basyx/basyx-go-components/internal/companylookupservice/persistence"
)

const (
	componentName = "ComLookup"
)

// CompanyLookupAPIService is a service that implements the logic for the CompanyLookup API.
// This service should implement the business logic for every endpoint for the CompanyLookup API.
// Include any external packages or services that will be required by this service.
type CompanyLookupAPIService struct {
	companyLookupBackend companylookuppostgresql.PostgreSQLCompanyLookupDatabase
}

// NewCompanyLookupAPIService creates a default api service.
func NewCompanyLookupAPIService(companyLookupBackend companylookuppostgresql.PostgreSQLCompanyLookupDatabase) *CompanyLookupAPIService {
	return &CompanyLookupAPIService{
		companyLookupBackend: companyLookupBackend,
	}
}

// GetAllCompanyDescriptors returns all company descriptors.
func (s *CompanyLookupAPIService) GetAllCompanyDescriptors(ctx context.Context, limit int32, cursor string, name string, assetId string) (model.ImplResponse, error) {
	var internalCursor string
	if strings.TrimSpace(cursor) != "" {
		dec, decErr := common.DecodeString(cursor)
		if decErr != nil {
			slog.ErrorContext(ctx, "Error in GetAllCompanyDescriptors: decode cursor limit name assetId", "error.code", "API-GETALLCOMPANYDESCRIPTORS-DECODE", "error", decErr, "component", componentName, "cursor", cursor, "limit", limit, "name", name, "asset_id", assetId)
			return common.NewErrorResponse(
				decErr, http.StatusBadRequest, componentName, "GetAllCompanyDescriptors", "BadCursor",
			), nil
		}
		internalCursor = dec
	}

	var internalName string
	if strings.TrimSpace(name) != "" {
		dec, decErr := common.DecodeString(name)
		if decErr != nil {
			slog.ErrorContext(ctx, "Error in GetAllCompanyDescriptors: decode name limit cursor assetId", "error.code", "API-GETALLCOMPANYDESCRIPTORS-EXECUTE", "error", decErr, "component", componentName, "name", name, "limit", limit, "internal_cursor", internalCursor, "asset_id", assetId)
			return common.NewErrorResponse(
				decErr, http.StatusBadRequest, componentName, "GetAllCompanyDescriptors", "BadName",
			), nil
		}
		internalName = dec
	}

	var internalAssetID string
	if strings.TrimSpace(assetId) != "" {
		dec, decErr := common.DecodeString(assetId)
		if decErr != nil {
			slog.ErrorContext(ctx, "Error in GetAllCompanyDescriptors: decode assetId limit cursor name", "error.code", "API-GETALLCOMPANYDESCRIPTORS-EXECUTE", "error", decErr, "component", componentName, "asset_id", assetId, "limit", limit, "internal_cursor", internalCursor, "internal_name", internalName)
			return common.NewErrorResponse(
				decErr, http.StatusBadRequest, componentName, "GetAllCompanyDescriptors", "BadAssetId",
			), nil
		}
		internalAssetID = dec
	}

	companyDescriptors, nextCursor, err := s.companyLookupBackend.ListCompanyDescriptors(ctx, limit, internalCursor, internalName, internalAssetID)
	if err != nil {
		slog.ErrorContext(ctx, "Error in GetAllCompanyDescriptors: list failed", "error.code", "API-GETALLCOMPANYDESCRIPTORS-EXECUTE", "error", err, "component", componentName, "limit", limit, "internal_cursor", internalCursor, "internal_name", internalName, "internal_asset_id", internalAssetID)
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "GetAllCompanyDescriptors", "BadRequest",
			), nil
		default:
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "GetAllCompanyDescriptors", "InternalServerError",
			), err
		}
	}

	jsonable := make([]map[string]any, 0, len(companyDescriptors))
	for _, companyDescriptor := range companyDescriptors {
		j, toJsonErr := companyDescriptor.ToJsonable()
		if toJsonErr != nil {
			slog.ErrorContext(ctx, "Error in GetAllCompanyDescriptors: ToJsonable failed", "error.code", "API-GETALLCOMPANYDESCRIPTORS-SERIALIZERESPONSE", "error", toJsonErr, "component", componentName)
			return common.NewErrorResponse(
				toJsonErr, http.StatusInternalServerError, componentName, "GetAllCompanyDescriptors", "Unhandled-ToJsonable",
			), toJsonErr
		}
		jsonable = append(jsonable, j)
	}

	pm := model.PagedResultPagingMetadata{}
	if nextCursor != "" {
		pm.Cursor = common.EncodeString(nextCursor)
	}

	res := struct {
		PagingMetadata model.PagedResultPagingMetadata `json:"paging_metadata"`
		Result         []map[string]any                `json:"result"`
	}{
		PagingMetadata: pm,
		Result:         jsonable,
	}

	return model.Response(http.StatusOK, res), nil
}

// PostCompanyDescriptor creates a new company descriptor.
func (s *CompanyLookupAPIService) PostCompanyDescriptor(ctx context.Context, companyDescriptor model.CompanyDescriptor) (model.ImplResponse, error) {
	if strings.TrimSpace(companyDescriptor.Domain) != "" && !model.IsStrictCompanyDomain(companyDescriptor.Domain) {
		invalidDomainErr := common.NewErrBadRequest("COMLOOKUP-POSTCOMPANYDESCRIPTOR-VALIDATEDOMAIN provided domain is not a syntactically valid domain")
		slog.ErrorContext(ctx, "Error in PostCompanyDescriptor: invalid domain syntax in body", "error.code", "API-POSTCOMPANYDESCRIPTOR-VALIDATE", "component", componentName)
		return common.NewErrorResponse(
			invalidDomainErr, http.StatusBadRequest, componentName, "PostCompanyDescriptor", "BadRequest-InvalidDomainSyntax",
		), nil
	}

	result, err := s.companyLookupBackend.InsertCompanyDescriptor(ctx, companyDescriptor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			slog.ErrorContext(ctx, "Error in InsertCompanyDescriptor: bad request", "error.code", "API-POSTCOMPANYDESCRIPTOR-VALIDATE", "error", err, "component", componentName)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "InsertCompanyDescriptor", "BadRequest",
			), nil
		case common.IsErrConflict(err):
			slog.ErrorContext(ctx, "Error in InsertCompanyDescriptor: conflict", "error.code", "API-POSTCOMPANYDESCRIPTOR-CHECKCONFLICT", "error", err, "component", componentName)
			return common.NewErrorResponse(
				err, http.StatusConflict, componentName, "InsertCompanyDescriptor", "Conflict",
			), nil
		default:
			slog.ErrorContext(ctx, "Error in InsertCompanyDescriptor: internal", "error.code", "API-POSTCOMPANYDESCRIPTOR-EXECUTE", "error", err, "component", componentName)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "InsertCompanyDescriptor", "Unhandled",
			), err
		}
	}

	jsonable, toJsonErr := result.ToJsonable()
	if toJsonErr != nil {
		slog.ErrorContext(ctx, "Error in PostCompanyDescriptor: ToJsonable failed", "error.code", "API-POSTCOMPANYDESCRIPTOR-SERIALIZERESPONSE", "error", toJsonErr, "component", componentName, "result_domain", result.Domain)
		return common.NewErrorResponse(
			toJsonErr, http.StatusInternalServerError, componentName, "PostCompanyDescriptor", "Unhandled-ToJsonable",
		), toJsonErr
	}

	return model.Response(http.StatusCreated, jsonable), nil
}

// GetCompanyDescriptorById returns a specific company descriptor.
func (s *CompanyLookupAPIService) GetCompanyDescriptorById(ctx context.Context, companyIdentifier string) (model.ImplResponse, error) {
	decoded, decodeErr := common.DecodeString(companyIdentifier)
	if decodeErr != nil {
		slog.ErrorContext(ctx, "Error in GetCompanyDescriptorById: decode companyIdentifier", "error.code", "API-GETCOMPANYDESCRIPTORBYID-DECODE", "error", decodeErr, "component", componentName, "company_identifier", companyIdentifier)
		return common.NewErrorResponse(
			decodeErr, http.StatusBadRequest, componentName, "GetCompanyDescriptorById", "BadRequest-Decode",
		), nil
	}
	if !model.IsStrictCompanyDomain(decoded) {
		invalidDomainErr := common.NewErrBadRequest("COMLOOKUP-GETCOMPANYDESCRIPTORBYID-VALIDATEDOMAIN decoded identifier is not a syntactically valid domain")
		slog.ErrorContext(ctx, "Error in GetCompanyDescriptorById: invalid decoded domain syntax", "error.code", "API-GETCOMPANYDESCRIPTORBYID-VALIDATE", "component", componentName, "company_identifier", companyIdentifier, "decoded", decoded)
		return common.NewErrorResponse(
			invalidDomainErr, http.StatusBadRequest, componentName, "GetCompanyDescriptorById", "BadRequest-InvalidDomainSyntax",
		), nil
	}

	result, err := s.companyLookupBackend.GetCompanyDescriptorByID(ctx, decoded)

	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			slog.ErrorContext(ctx, "Error in GetCompanyDescriptorById: bad request", "error.code", "API-GETCOMPANYDESCRIPTORBYID-VALIDATE", "error", err, "component", componentName, "decoded", string(decoded))
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "GetCompanyDescriptorById", "BadRequest",
			), nil
		case common.IsErrNotFound(err):
			slog.ErrorContext(ctx, "Error in GetCompanyDescriptorById: not found", "error.code", "API-GETCOMPANYDESCRIPTORBYID-FIND", "error", err, "component", componentName, "decoded", string(decoded))
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "GetCompanyDescriptorById", "NotFound",
			), nil
		default:
			slog.ErrorContext(ctx, "Error in GetCompanyDescriptorById: internal", "error.code", "API-GETCOMPANYDESCRIPTORBYID-EXECUTE", "error", err, "component", componentName, "decoded", string(decoded))
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "GetCompanyDescriptorById", "Unhandled",
			), err
		}
	}

	jsonable, toJsonErr := result.ToJsonable()
	if toJsonErr != nil {
		return common.NewErrorResponse(
			toJsonErr, http.StatusInternalServerError, componentName, "GetCompanyDescriptorById", "Unhandled-ToJsonable",
		), toJsonErr
	}

	return model.Response(http.StatusOK, jsonable), nil
}

// PutCompanyDescriptorById updates an existing company descriptor.
func (s *CompanyLookupAPIService) PutCompanyDescriptorById(ctx context.Context, companyIdentifier string, companyDescriptor model.CompanyDescriptor) (model.ImplResponse, error) {
	// Decode path AAS id
	decodedCompany, decErr := common.DecodeString(companyIdentifier)
	if decErr != nil {
		slog.ErrorContext(ctx, "Error in PutCompanyDescriptorById: decode companyIdentifier", "error.code", "API-PUTCOMPANYDESCRIPTORBYID-DECODE", "error", decErr, "component", componentName, "company_identifier", companyIdentifier)
		return common.NewErrorResponse(
			decErr, http.StatusBadRequest, componentName, "PutCompanyDescriptorById", "BadRequest-Decode",
		), nil
	}
	if !model.IsStrictCompanyDomain(decodedCompany) {
		invalidDomainErr := common.NewErrBadRequest("COMLOOKUP-PUTCOMPANYDESCRIPTORBYID-VALIDATEDOMAIN decoded identifier is not a syntactically valid domain")
		slog.ErrorContext(ctx, "Error in PutCompanyDescriptorById: invalid decoded domain syntax", "error.code", "API-PUTCOMPANYDESCRIPTORBYID-VALIDATE", "component", componentName, "company_identifier", companyIdentifier, "decoded_company", decodedCompany)
		return common.NewErrorResponse(
			invalidDomainErr, http.StatusBadRequest, componentName, "PutCompanyDescriptorById", "BadRequest-InvalidDomainSyntax",
		), nil
	}

	// Enforce domain consistency with path.
	if strings.TrimSpace(companyDescriptor.Domain) == "" {
		companyDescriptor.Domain = decodedCompany
	} else if companyDescriptor.Domain != decodedCompany {
		slog.ErrorContext(ctx, "Error in PutCompanyDescriptorById: body domain does not match path domain", "error.code", "API-PUTCOMPANYDESCRIPTORBYID-VALIDATEBODY", "component", componentName, "decoded_company", decodedCompany)
		return common.NewErrorResponse(
			errors.New("body domain does not match path domain"), http.StatusBadRequest, componentName, "PutCompanyDescriptorById", "BadRequest-DomainMismatch",
		), nil
	}

	if exists, chkErr := s.companyLookupBackend.ExistsCompanyDescriptorByID(ctx, companyDescriptor.Domain); chkErr != nil {
		slog.ErrorContext(ctx, "Error in PutCompanyDescriptorById: existence check failed", "error.code", "API-PUTCOMPANYDESCRIPTORBYID-CHECKEXISTS", "error", chkErr, "component", componentName)
		return common.NewErrorResponse(
			chkErr, http.StatusInternalServerError, componentName, "PutCompanyDescriptorById", "Unhandled-Precheck",
		), chkErr
	} else if !exists {
		notFoundErr := common.NewErrNotFound("Company Descriptor not found")
		slog.ErrorContext(ctx, "Error in PutCompanyDescriptorById: not found", "error.code", "API-PUTCOMPANYDESCRIPTORBYID-FIND", "component", componentName)
		return common.NewErrorResponse(
			notFoundErr, http.StatusNotFound, componentName, "PutCompanyDescriptorById", "NotFound",
		), nil
	}

	result, err := s.companyLookupBackend.ReplaceCompanyDescriptor(ctx, companyDescriptor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			slog.ErrorContext(ctx, "Error in PutCompanyDescriptorById: bad request", "error.code", "API-PUTCOMPANYDESCRIPTORBYID-VALIDATE", "error", err, "component", componentName, "decoded_company", decodedCompany)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "PutCompanyDescriptorById", "BadRequest",
			), nil
		case common.IsErrConflict(err):
			slog.ErrorContext(ctx, "Error in PutCompanyDescriptorById: conflict", "error.code", "API-PUTCOMPANYDESCRIPTORBYID-CHECKCONFLICT", "error", err, "component", componentName, "decoded_company", decodedCompany)
			return common.NewErrorResponse(
				err, http.StatusConflict, componentName, "PutCompanyDescriptorById", "Conflict",
			), nil
		default:
			slog.ErrorContext(ctx, "Error in PutCompanyDescriptorById: internal", "error.code", "API-PUTCOMPANYDESCRIPTORBYID-EXECUTE", "error", err, "component", componentName, "decoded_company", decodedCompany)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "PutCompanyDescriptorById", "Unhandled-Insert",
			), err
		}
	}

	jsonable, toJsonErr := result.ToJsonable()
	if toJsonErr != nil {
		slog.ErrorContext(ctx, "Error in PutCompanyDescriptorById: ToJsonable failed", "error.code", "API-PUTCOMPANYDESCRIPTORBYID-SERIALIZERESPONSE", "error", toJsonErr, "component", componentName, "result_domain", result.Domain)
		return common.NewErrorResponse(
			toJsonErr, http.StatusInternalServerError, componentName, "PutCompanyDescriptorById", "Unhandled-ToJsonable",
		), toJsonErr
	}

	return model.Response(http.StatusOK, jsonable), nil
}

// DeleteCompanyDescriptorById deletes a company descriptor.
func (s *CompanyLookupAPIService) DeleteCompanyDescriptorById(ctx context.Context, companyIdentifier string) (model.ImplResponse, error) {
	decoded, decodeErr := common.DecodeString(companyIdentifier)
	if decodeErr != nil {
		slog.ErrorContext(ctx, "Error DeleteCompanyDescriptorById: decode companyIdentifier failed", "error.code", "API-DELETECOMPANYDESCRIPTORBYID-DECODE", "error", decodeErr, "component", componentName, "company_identifier", companyIdentifier)
		return common.NewErrorResponse(
			decodeErr, http.StatusBadRequest, componentName, "DeleteCompanyDescriptorById", "BadRequest-Decode",
		), nil
	}
	if !model.IsStrictCompanyDomain(decoded) {
		invalidDomainErr := common.NewErrBadRequest("COMLOOKUP-DELETECOMPANYDESCRIPTORBYID-VALIDATEDOMAIN decoded identifier is not a syntactically valid domain")
		slog.ErrorContext(ctx, "Error in DeleteCompanyDescriptorById: invalid decoded domain syntax", "error.code", "API-DELETECOMPANYDESCRIPTORBYID-VALIDATE", "component", componentName, "company_identifier", companyIdentifier, "decoded", decoded)
		return common.NewErrorResponse(
			invalidDomainErr, http.StatusBadRequest, componentName, "DeleteCompanyDescriptorById", "BadRequest-InvalidDomainSyntax",
		), nil
	}

	if err := s.companyLookupBackend.DeleteCompanyDescriptorByID(ctx, decoded); err != nil {
		switch {
		case common.IsErrNotFound(err):
			slog.ErrorContext(ctx, "Error in DeleteCompanyDescriptorById: not found", "error.code", "API-DELETECOMPANYDESCRIPTORBYID-FIND", "error", err, "component", componentName, "decoded", decoded)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "DeleteCompanyDescriptorById", "NotFound",
			), nil
		case common.IsErrBadRequest(err):
			slog.ErrorContext(ctx, "Error in DeleteCompanyDescriptorById: bad request", "error.code", "API-DELETECOMPANYDESCRIPTORBYID-VALIDATE", "error", err, "component", componentName, "decoded", decoded)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "DeleteCompanyDescriptorById", "BadRequest",
			), nil
		default:
			slog.ErrorContext(ctx, "Error in DeleteCompanyDescriptorById: internal", "error.code", "API-DELETECOMPANYDESCRIPTORBYID-EXECUTE", "error", err, "component", componentName, "decoded", decoded)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "DeleteCompanyDescriptorById", "Unhandled",
			), err
		}
	}

	return model.Response(http.StatusNoContent, nil), nil
}
