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

/*
 * DotAAS Part 2 | HTTP/REST | Concept Description Repository Service Specification
 *
 * The ConceptDescription Repository Service Specification as part of [Specification of the Asset Administration Shell: Part 2](https://industrialdigitaltwin.org/en/content-hub/aasspecifications).   Copyright: Industrial Digital Twin Association (IDTA) March 2023
 *
 * API version: V3.1.1_SSP-001
 * Contact: info@idtwin.org
 */

// Package api provides the Concept Description Repository API service implementation.
package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/persistence"
)

// ConceptDescriptionRepositoryAPIAPIService is a service that implements the logic for the ConceptDescriptionRepositoryAPIAPIServicer
// This service should implement the business logic for every endpoint for the ConceptDescriptionRepositoryAPIAPI API.
// Include any external packages or services that will be required by this service.
type ConceptDescriptionRepositoryAPIAPIService struct {
	d *persistence.ConceptDescriptionBackend
}

const componentName = "CDREPO"

func pagedResponse[T any](results T, nextCursor string) model.ImplResponse {
	pm := model.PagedResultPagingMetadata{}
	if nextCursor != "" {
		pm.Cursor = common.EncodeString(nextCursor)
	}

	res := struct {
		PagingMetadata model.PagedResultPagingMetadata `json:"paging_metadata"`
		Result         T                               `json:"result"`
	}{
		PagingMetadata: pm,
		Result:         results,
	}

	return model.Response(http.StatusOK, res)
}

// NewConceptDescriptionRepositoryAPIAPIService creates a default api service
func NewConceptDescriptionRepositoryAPIAPIService(database *persistence.ConceptDescriptionBackend) *ConceptDescriptionRepositoryAPIAPIService {
	return &ConceptDescriptionRepositoryAPIAPIService{
		d: database,
	}
}

// QueryConceptDescriptions returns Concept Descriptions that match the provided
// query expression and any ABAC query filter stored in ctx.
//
// The limit parameter bounds the number of returned Concept Descriptions. The
// cursor parameter is optional, base64-url encoded, and decoded before it is
// passed to the persistence layer. The query parameter contains the user
// supplied condition and fragment filters for the /query/concept-descriptions
// endpoint.
//
// The returned ImplResponse contains a paged result with JSON-serializable
// Concept Description objects and an encoded cursor for the next page when more
// results are available. Invalid limits, invalid cursors, denied access, or
// unsupported query expressions are returned as HTTP error responses.
func (s *ConceptDescriptionRepositoryAPIAPIService) QueryConceptDescriptions(ctx context.Context, limit int32, cursor string, query grammar.Query) (model.ImplResponse, error) {
	const operation = "QueryConceptDescriptions"

	decodedCursor := strings.TrimSpace(cursor)
	if decodedCursor != "" {
		var decodeErr error
		decodedCursor, decodeErr = common.DecodeString(decodedCursor)
		if decodeErr != nil {
			return common.NewErrorResponse(decodeErr, http.StatusBadRequest, componentName, operation, "BadCursor"), nil
		}
	}

	if limit < 0 {
		err := common.NewErrBadRequest("limit must be non-negative")
		return common.NewErrorResponse(err, http.StatusBadRequest, componentName, operation, "BadLimit"), nil
	}

	uintLimit64, convErr := strconv.ParseUint(strconv.FormatInt(int64(limit), 10), 10, 64)
	if convErr != nil {
		err := common.NewErrBadRequest("invalid limit")
		return common.NewErrorResponse(err, http.StatusBadRequest, componentName, operation, "BadLimit"), nil
	}

	queryCtx := auth.MergeQueryFilter(ctx, query)
	cds, nextCursor, err := s.d.GetConceptDescriptions(queryCtx, nil, nil, nil, uint(uintLimit64), &decodedCursor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(err, http.StatusBadRequest, componentName, operation, "BadRequest"), nil
		case common.IsErrDenied(err):
			return common.NewErrorResponse(err, http.StatusForbidden, componentName, operation, "Denied"), nil
		default:
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, operation, "Unhandled"), err
		}
	}

	jsonable := make([]map[string]any, 0, len(cds))
	for _, cd := range cds {
		jsonObj, err := jsonization.ToJsonable(cd)
		if err != nil {
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, operation, "ToJsonable"), err
		}
		jsonable = append(jsonable, jsonObj)
	}

	return pagedResponse(jsonable, nextCursor), nil
}

// GetAllConceptDescriptions - Returns all Concept Descriptions
func (s *ConceptDescriptionRepositoryAPIAPIService) GetAllConceptDescriptions(ctx context.Context, idShort string, isCaseOf string, dataSpecificationRef string, limit int32, cursor string) (model.ImplResponse, error) {
	decodedCursor := strings.TrimSpace(cursor)
	if decodedCursor != "" {
		var decodeErr error
		decodedCursor, decodeErr = common.DecodeString(decodedCursor)
		if decodeErr != nil {
			return common.NewErrorResponse(decodeErr, http.StatusBadRequest, componentName, "GetAllConceptDescriptions", "BadCursor"), nil
		}
	}

	if limit < 0 {
		err := common.NewErrBadRequest("limit must be non-negative")
		return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "GetAllConceptDescriptions", "BadLimit"), nil
	}

	uintLimit64, convErr := strconv.ParseUint(strconv.FormatInt(int64(limit), 10), 10, 64)
	if convErr != nil {
		err := common.NewErrBadRequest("invalid limit")
		return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "GetAllConceptDescriptions", "BadLimit"), nil
	}
	uintLimit := uint(uintLimit64)
	cds, nextCursor, err := s.d.GetConceptDescriptions(ctx, &idShort, &isCaseOf, &dataSpecificationRef, uintLimit, &decodedCursor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "GetAllConceptDescriptions", "BadRequest"), nil
		case common.IsErrDenied(err):
			return common.NewErrorResponse(err, http.StatusForbidden, componentName, "GetAllConceptDescriptions", "Denied"), nil
		default:
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "GetAllConceptDescriptions", "Unhandled"), nil
		}
	}

	jsonable := make([]map[string]any, 0, len(cds))
	for _, cd := range cds {
		jsonObj, err := jsonization.ToJsonable(cd)
		if err != nil {
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "GetAllConceptDescriptions", "ToJsonable"), nil
		}
		jsonable = append(jsonable, jsonObj)
	}

	return pagedResponse(jsonable, nextCursor), nil
}

// GetAllConceptDescriptionRecentChanges returns changed Concept Descriptions.
func (s *ConceptDescriptionRepositoryAPIAPIService) GetAllConceptDescriptionRecentChanges(ctx context.Context, createdFrom time.Time, updatedFrom time.Time, limit int32, cursor string) (model.ImplResponse, error) {
	decodedCursor := strings.TrimSpace(cursor)
	if decodedCursor != "" {
		var decodeErr error
		decodedCursor, decodeErr = common.DecodeString(decodedCursor)
		if decodeErr != nil {
			return common.NewErrorResponse(decodeErr, http.StatusBadRequest, componentName, "GetAllConceptDescriptionRecentChanges", "BadCursor"), nil
		}
	}

	rows, nextCursor, err := s.d.GetConceptDescriptionRecentChanges(ctx, limit, decodedCursor, createdFrom, updatedFrom)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "GetAllConceptDescriptionRecentChanges", "BadRequest"), nil
		case common.IsErrDenied(err):
			return common.NewErrorResponse(err, http.StatusForbidden, componentName, "GetAllConceptDescriptionRecentChanges", "Denied"), nil
		default:
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "GetAllConceptDescriptionRecentChanges", "Unhandled"), nil
		}
	}

	changes := make([]model.ConceptDescriptionRecentChange, 0, len(rows))
	for _, row := range rows {
		changes = append(changes, model.ConceptDescriptionRecentChange{
			RecentChange: model.RecentChange{
				Type:      row.ChangeType,
				CreatedAt: row.CreatedAt,
				UpdatedAt: row.UpdatedAt,
			},
			Id: row.Identifier,
		})
	}
	return pagedResponse(changes, nextCursor), nil
}

// PostConceptDescription - Creates a new Concept Description
func (s *ConceptDescriptionRepositoryAPIAPIService) PostConceptDescription(ctx context.Context, conceptDescription types.IConceptDescription) (model.ImplResponse, error) {
	err := s.d.CreateConceptDescription(ctx, conceptDescription)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "PostConceptDescription", "BadRequest"), nil
		case common.IsErrConflict(err):
			return common.NewErrorResponse(err, http.StatusConflict, componentName, "PostConceptDescription", "Conflict"), nil
		case common.IsErrDenied(err):
			return common.NewErrorResponse(err, http.StatusForbidden, componentName, "PostConceptDescription", "Denied"), nil
		default:
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "PostConceptDescription", "Unhandled"), nil
		}
	}

	jsonable, toJsonErr := jsonization.ToJsonable(conceptDescription)
	if toJsonErr != nil {
		return common.NewErrorResponse(toJsonErr, http.StatusInternalServerError, componentName, "PostConceptDescription", "ToJsonable"), nil
	}

	return model.Response(http.StatusCreated, jsonable), nil
}

// GetConceptDescriptionById - Returns a specific Concept Description
func (s *ConceptDescriptionRepositoryAPIAPIService) GetConceptDescriptionById(ctx context.Context, cdIdentifier string) (model.ImplResponse, error) {
	decodedIdentifier, err := common.Decode(cdIdentifier)
	if err != nil {
		return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "GetConceptDescriptionById", "URLDecode"), nil
	}
	cd, err := s.d.GetConceptDescriptionByID(ctx, string(decodedIdentifier))
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "GetConceptDescriptionById", "BadRequest"), nil
		case common.IsErrDenied(err):
			return common.NewErrorResponse(err, http.StatusForbidden, componentName, "GetConceptDescriptionById", "Denied"), nil
		case common.IsErrNotFound(err):
			return common.NewErrorResponse(err, http.StatusNotFound, componentName, "GetConceptDescriptionById", "NotFound"), nil
		default:
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "GetConceptDescriptionById", "Unhandled"), nil
		}
	}

	var jsonable map[string]any
	jsonable, err = jsonization.ToJsonable(cd)
	if err != nil {
		return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "GetConceptDescriptionById", "ToJsonable"), nil
	}

	return model.Response(http.StatusOK, jsonable), nil
}

// PutConceptDescriptionById - Creates or updates an existing Concept Description
func (s *ConceptDescriptionRepositoryAPIAPIService) PutConceptDescriptionById(ctx context.Context, cdIdentifier string, conceptDescription types.IConceptDescription) (model.ImplResponse, error) {
	decodedIdentifier, err := common.Decode(cdIdentifier)
	if err != nil {
		return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "PutConceptDescriptionById", "URLDecode"), nil
	}
	isUpdate, err := s.d.PutConceptDescription(ctx, string(decodedIdentifier), conceptDescription)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "PutConceptDescriptionById", "BadRequest"), nil
		case common.IsErrDenied(err):
			return common.NewErrorResponse(err, http.StatusForbidden, componentName, "PutConceptDescriptionById", "Denied"), nil
		default:
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "PutConceptDescriptionById", "Unhandled"), nil
		}
	}

	if isUpdate {
		return model.Response(http.StatusNoContent, nil), nil
	}

	jsonable, toJsonErr := jsonization.ToJsonable(conceptDescription)
	if toJsonErr != nil {
		return common.NewErrorResponse(toJsonErr, http.StatusInternalServerError, componentName, "PutConceptDescriptionById", "ToJsonable"), nil
	}

	return model.Response(http.StatusCreated, jsonable), nil
}

// DeleteConceptDescriptionById - Deletes a Concept Description
func (s *ConceptDescriptionRepositoryAPIAPIService) DeleteConceptDescriptionById(ctx context.Context, cdIdentifier string) (model.ImplResponse, error) {
	decodedIdentifier, err := common.Decode(cdIdentifier)
	if err != nil {
		return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "DeleteConceptDescriptionById", "URLDecode"), nil
	}
	err = s.d.DeleteConceptDescription(ctx, string(decodedIdentifier))
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "DeleteConceptDescriptionById", "BadRequest"), nil
		case common.IsErrDenied(err):
			return common.NewErrorResponse(err, http.StatusForbidden, componentName, "DeleteConceptDescriptionById", "Denied"), nil
		case common.IsErrNotFound(err):
			return common.NewErrorResponse(err, http.StatusNotFound, componentName, "DeleteConceptDescriptionById", "NotFound"), nil
		default:
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "DeleteConceptDescriptionById", "Unhandled"), nil
		}
	}

	return model.Response(http.StatusNoContent, nil), nil
}
