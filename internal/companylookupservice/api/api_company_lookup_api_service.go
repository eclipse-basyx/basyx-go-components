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
	"log"
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
			log.Printf("📍 [%s] Error in GetAllCompanyDescriptors: decode cursor=%q limit=%d name=%q assetId=%q: %v", componentName, cursor, limit, name, assetId, decErr)
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
			log.Printf("📍 [%s] Error in GetAllCompanyDescriptors: decode name=%q limit=%d cursor=%q assetId=%q: %v", componentName, name, limit, internalCursor, assetId, decErr)
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
			log.Printf("📍 [%s] Error in GetAllCompanyDescriptors: decode assetId=%q limit=%d cursor=%q name=%q: %v", componentName, assetId, limit, internalCursor, internalName, decErr)
			return common.NewErrorResponse(
				decErr, http.StatusBadRequest, componentName, "GetAllCompanyDescriptors", "BadAssetId",
			), nil
		}
		internalAssetID = dec
	}

	companyDescriptors, nextCursor, err := s.companyLookupBackend.ListCompanyDescriptors(ctx, limit, internalCursor, internalName, internalAssetID)
	if err != nil {
		log.Printf("📍 [%s] Error in GetAllCompanyDescriptors: list failed (limit=%d cursor=%q name=%q assetId=%q): %v", componentName, limit, internalCursor, internalName, internalAssetID, err)
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
			log.Printf("📍 [%s] Error in GetAllCompanyDescriptors: ToJsonable failed (companyDomain=%q): %v", componentName, companyDescriptor.Domain, toJsonErr)
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
		Data           []map[string]any                `json:"data"`
	}{
		PagingMetadata: pm,
		Data:           jsonable,
	}

	return model.Response(http.StatusOK, res), nil
}

// PostCompanyDescriptor creates a new company descriptor.
func (s *CompanyLookupAPIService) PostCompanyDescriptor(ctx context.Context, companyDescriptor model.CompanyDescriptor) (model.ImplResponse, error) {
	if strings.TrimSpace(companyDescriptor.Domain) != "" && !model.IsStrictCompanyDomain(companyDescriptor.Domain) {
		invalidDomainErr := common.NewErrBadRequest("COMLOOKUP-POSTCOMPANYDESCRIPTOR-VALIDATEDOMAIN provided domain is not a syntactically valid domain")
		log.Printf("📍 [%s] Error in PostCompanyDescriptor: invalid domain syntax in body (companyDomain=%q)", componentName, companyDescriptor.Domain)
		return common.NewErrorResponse(
			invalidDomainErr, http.StatusBadRequest, componentName, "PostCompanyDescriptor", "BadRequest-InvalidDomainSyntax",
		), nil
	}

	result, err := s.companyLookupBackend.InsertCompanyDescriptor(ctx, companyDescriptor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			log.Printf("📍 [%s] Error in InsertCompanyDescriptor: bad request (companyDomain=%q): %v", componentName, companyDescriptor.Domain, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "InsertCompanyDescriptor", "BadRequest",
			), nil
		case common.IsErrConflict(err):
			log.Printf("📍 [%s] Error in InsertCompanyDescriptor: conflict (companyDomain=%q): %v", componentName, companyDescriptor.Domain, err)
			return common.NewErrorResponse(
				err, http.StatusConflict, componentName, "InsertCompanyDescriptor", "Conflict",
			), nil
		default:
			log.Printf("📍 [%s] Error in InsertCompanyDescriptor: internal (companyDomain=%q): %v", componentName, companyDescriptor.Domain, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "InsertCompanyDescriptor", "Unhandled",
			), err
		}
	}

	j, toJsonErr := result.ToJsonable()
	if toJsonErr != nil {
		log.Printf("📍 [%s] Error in PostCompanyDescriptor: ToJsonable failed (companyDomain=%q): %v", componentName, result.Domain, toJsonErr)
		return common.NewErrorResponse(
			toJsonErr, http.StatusInternalServerError, componentName, "PostCompanyDescriptor", "Unhandled-ToJsonable",
		), toJsonErr
	}

	return model.Response(http.StatusCreated, map[string]any{"data": j}), nil
}

// GetCompanyDescriptorById returns a specific company descriptor.
func (s *CompanyLookupAPIService) GetCompanyDescriptorById(ctx context.Context, companyIdentifier string) (model.ImplResponse, error) {
	decoded, decodeErr := common.DecodeString(companyIdentifier)
	if decodeErr != nil {
		log.Printf("📍 [%s] Error in GetCompanyDescriptorById: decode companyIdentifier=%q: %v", componentName, companyIdentifier, decodeErr)
		return common.NewErrorResponse(
			decodeErr, http.StatusBadRequest, componentName, "GetCompanyDescriptorById", "BadRequest-Decode",
		), nil
	}
	if !model.IsStrictCompanyDomain(decoded) {
		invalidDomainErr := common.NewErrBadRequest("COMLOOKUP-GETCOMPANYDESCRIPTORBYID-VALIDATEDOMAIN decoded identifier is not a syntactically valid domain")
		log.Printf("📍 [%s] Error in GetCompanyDescriptorById: invalid decoded domain syntax (companyIdentifier=%q decoded=%q)", componentName, companyIdentifier, decoded)
		return common.NewErrorResponse(
			invalidDomainErr, http.StatusBadRequest, componentName, "GetCompanyDescriptorById", "BadRequest-InvalidDomainSyntax",
		), nil
	}

	result, err := s.companyLookupBackend.GetCompanyDescriptorByID(ctx, decoded)

	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			log.Printf("📍 [%s] Error in GetCompanyDescriptorById: bad request (companyId=%q): %v", componentName, string(decoded), err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "GetCompanyDescriptorById", "BadRequest",
			), nil
		case common.IsErrNotFound(err):
			log.Printf("📍 [%s] Error in GetCompanyDescriptorById: not found (companyId=%q): %v", componentName, string(decoded), err)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "GetCompanyDescriptorById", "NotFound",
			), nil
		default:
			log.Printf("📍 [%s] Error in GetCompanyDescriptorById: internal (companyId=%q): %v", componentName, string(decoded), err)
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

	return model.Response(http.StatusOK, map[string]any{"data": jsonable}), nil
}

// PutCompanyDescriptorById updates an existing company descriptor.
func (s *CompanyLookupAPIService) PutCompanyDescriptorById(ctx context.Context, companyIdentifier string, companyDescriptor model.CompanyDescriptor) (model.ImplResponse, error) {
	// Decode path AAS id
	decodedCompany, decErr := common.DecodeString(companyIdentifier)
	if decErr != nil {
		log.Printf("📍 [%s] Error in PutCompanyDescriptorById: decode companyIdentifier=%q: %v", componentName, companyIdentifier, decErr)
		return common.NewErrorResponse(
			decErr, http.StatusBadRequest, componentName, "PutCompanyDescriptorById", "BadRequest-Decode",
		), nil
	}
	if !model.IsStrictCompanyDomain(decodedCompany) {
		invalidDomainErr := common.NewErrBadRequest("COMLOOKUP-PUTCOMPANYDESCRIPTORBYID-VALIDATEDOMAIN decoded identifier is not a syntactically valid domain")
		log.Printf("📍 [%s] Error in PutCompanyDescriptorById: invalid decoded domain syntax (companyIdentifier=%q decoded=%q)", componentName, companyIdentifier, decodedCompany)
		return common.NewErrorResponse(
			invalidDomainErr, http.StatusBadRequest, componentName, "PutCompanyDescriptorById", "BadRequest-InvalidDomainSyntax",
		), nil
	}

	// Enforce domain consistency with path.
	if strings.TrimSpace(companyDescriptor.Domain) == "" {
		companyDescriptor.Domain = decodedCompany
	} else if companyDescriptor.Domain != decodedCompany {
		log.Printf("📍 [%s] Error in PutCompanyDescriptorById: body domain does not match path domain (body=%q path=%q)", componentName, companyDescriptor.Domain, decodedCompany)
		return common.NewErrorResponse(
			errors.New("body domain does not match path domain"), http.StatusBadRequest, componentName, "PutCompanyDescriptorById", "BadRequest-DomainMismatch",
		), nil
	}

	if exists, chkErr := s.companyLookupBackend.ExistsCompanyDescriptorByID(ctx, companyDescriptor.Domain); chkErr != nil {
		log.Printf("📍 [%s] Error in PutCompanyDescriptorById: existence check failed (companyDomain=%q): %v", componentName, companyDescriptor.Domain, chkErr)
		return common.NewErrorResponse(
			chkErr, http.StatusInternalServerError, componentName, "PutCompanyDescriptorById", "Unhandled-Precheck",
		), chkErr
	} else if !exists {
		notFoundErr := common.NewErrNotFound("Company Descriptor not found")
		log.Printf("📍 [%s] Error in PutCompanyDescriptorById: not found (companyDomain=%q)", componentName, companyDescriptor.Domain)
		return common.NewErrorResponse(
			notFoundErr, http.StatusNotFound, componentName, "PutCompanyDescriptorById", "NotFound",
		), nil
	}

	result, err := s.companyLookupBackend.ReplaceCompanyDescriptor(ctx, companyDescriptor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			log.Printf("📍 [%s] Error in PutCompanyDescriptorById: bad request (companyId=%q): %v", componentName, decodedCompany, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "PutCompanyDescriptorById", "BadRequest",
			), nil
		case common.IsErrConflict(err):
			log.Printf("📍 [%s] Error in PutCompanyDescriptorById: conflict (companyId=%q): %v", componentName, decodedCompany, err)
			return common.NewErrorResponse(
				err, http.StatusConflict, componentName, "PutCompanyDescriptorById", "Conflict",
			), nil
		default:
			log.Printf("📍 [%s] Error in PutCompanyDescriptorById: internal (companyId=%q): %v", componentName, decodedCompany, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "PutCompanyDescriptorById", "Unhandled-Insert",
			), err
		}
	}

	jsonable, toJsonErr := result.ToJsonable()
	if toJsonErr != nil {
		log.Printf("📍 [%s] Error in PutCompanyDescriptorById: ToJsonable failed (companyDomain=%q): %v", componentName, result.Domain, toJsonErr)
		return common.NewErrorResponse(
			toJsonErr, http.StatusInternalServerError, componentName, "PutCompanyDescriptorById", "Unhandled-ToJsonable",
		), toJsonErr
	}

	return model.Response(http.StatusOK, map[string]any{"data": jsonable}), nil
}

// DeleteCompanyDescriptorById deletes a company descriptor.
func (s *CompanyLookupAPIService) DeleteCompanyDescriptorById(ctx context.Context, companyIdentifier string) (model.ImplResponse, error) {
	decoded, decodeErr := common.DecodeString(companyIdentifier)
	if decodeErr != nil {
		log.Printf("📍 [%s] Error DeleteCompanyDescriptorById: decode companyIdentifier=%q failed: %v", componentName, companyIdentifier, decodeErr)
		return common.NewErrorResponse(
			decodeErr, http.StatusBadRequest, componentName, "DeleteCompanyDescriptorById", "BadRequest-Decode",
		), nil
	}
	if !model.IsStrictCompanyDomain(decoded) {
		invalidDomainErr := common.NewErrBadRequest("COMLOOKUP-DELETECOMPANYDESCRIPTORBYID-VALIDATEDOMAIN decoded identifier is not a syntactically valid domain")
		log.Printf("📍 [%s] Error in DeleteCompanyDescriptorById: invalid decoded domain syntax (companyIdentifier=%q decoded=%q)", componentName, companyIdentifier, decoded)
		return common.NewErrorResponse(
			invalidDomainErr, http.StatusBadRequest, componentName, "DeleteCompanyDescriptorById", "BadRequest-InvalidDomainSyntax",
		), nil
	}

	if err := s.companyLookupBackend.DeleteCompanyDescriptorByID(ctx, decoded); err != nil {
		switch {
		case common.IsErrNotFound(err):
			log.Printf("📍 [%s] Error in DeleteCompanyDescriptorById: not found (companyId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "DeleteCompanyDescriptorById", "NotFound",
			), nil
		case common.IsErrBadRequest(err):
			log.Printf("📍 [%s] Error in DeleteCompanyDescriptorById: bad request (companyId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "DeleteCompanyDescriptorById", "BadRequest",
			), nil
		default:
			log.Printf("📍 [%s] Error in DeleteCompanyDescriptorById: internal (companyId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "DeleteCompanyDescriptorById", "Unhandled",
			), err
		}
	}

	return model.Response(http.StatusNoContent, nil), nil
}
