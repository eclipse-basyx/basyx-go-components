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

package api

import (
	"context"
	"net/http"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/persistence"
)

// ConceptDescriptionRepositoryAPIAPIService is a service that implements the logic for the ConceptDescriptionRepositoryAPIAPIServicer
// This service should implement the business logic for every endpoint for the ConceptDescriptionRepositoryAPIAPI API.
// Include any external packages or services that will be required by this service.
type ConceptDescriptionRepositoryAPIAPIService struct {
	d *persistence.ConceptDescriptionBackend
}

const componentName = "CDREPO"

// NewConceptDescriptionRepositoryAPIAPIService creates a default api service
func NewConceptDescriptionRepositoryAPIAPIService(database *persistence.ConceptDescriptionBackend) *ConceptDescriptionRepositoryAPIAPIService {
	return &ConceptDescriptionRepositoryAPIAPIService{
		d: database,
	}
}

// GetAllConceptDescriptions - Returns all Concept Descriptions
func (s *ConceptDescriptionRepositoryAPIAPIService) GetAllConceptDescriptions(ctx context.Context, idShort string, isCaseOf string, dataSpecificationRef string, limit int32, cursor string) (model.ImplResponse, error) {
	cds, err := s.d.GetConceptDescriptions(&idShort, &isCaseOf, &dataSpecificationRef, int(limit), &cursor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "GetAllConceptDescriptions", "BadRequest"), nil
		case common.IsErrDenied(err):
			return common.NewErrorResponse(err, http.StatusForbidden, componentName, "GetAllConceptDescriptions", "Denied"), nil
		default:
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "GetAllConceptDescriptions", "Unhandled"), err
		}
	}

	var jsonable []map[string]any
	for _, cd := range cds {
		jsonObj, err := jsonization.ToJsonable(cd)
		if err != nil {
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "GetAllConceptDescriptions", "ToJsonable"), err
		}
		jsonable = append(jsonable, jsonObj)
	}

	return model.Response(http.StatusOK, jsonable), nil
}

// PostConceptDescription - Creates a new Concept Description
func (s *ConceptDescriptionRepositoryAPIAPIService) PostConceptDescription(ctx context.Context, conceptDescription types.IConceptDescription) (model.ImplResponse, error) {
	err := s.d.CreateConceptDescription(conceptDescription)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "PostConceptDescription", "BadRequest"), nil
		case common.IsErrConflict(err):
			return common.NewErrorResponse(err, http.StatusConflict, componentName, "PostConceptDescription", "Conflict"), nil
		case common.IsErrDenied(err):
			return common.NewErrorResponse(err, http.StatusForbidden, componentName, "PostConceptDescription", "Denied"), nil
		default:
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "PostConceptDescription", "Unhandled"), err
		}
	}

	jsonable, toJsonErr := jsonization.ToJsonable(conceptDescription)
	if toJsonErr != nil {
		return common.NewErrorResponse(toJsonErr, http.StatusInternalServerError, componentName, "PostConceptDescription", "ToJsonable"), toJsonErr
	}

	return model.Response(http.StatusCreated, jsonable), nil
}

// GetConceptDescriptionById - Returns a specific Concept Description
func (s *ConceptDescriptionRepositoryAPIAPIService) GetConceptDescriptionById(ctx context.Context, cdIdentifier string) (model.ImplResponse, error) {
	decodedIdentifier, err := common.Decode(cdIdentifier)
	if err != nil {
		return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "GetConceptDescriptionById", "URLDecode"), nil
	}
	cd, err := s.d.GetConceptDescriptionByID(string(decodedIdentifier))
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "GetConceptDescriptionById", "BadRequest"), nil
		case common.IsErrDenied(err):
			return common.NewErrorResponse(err, http.StatusForbidden, componentName, "GetConceptDescriptionById", "Denied"), nil
		case common.IsErrNotFound(err):
			return common.NewErrorResponse(err, http.StatusNotFound, componentName, "GetConceptDescriptionById", "NotFound"), nil
		default:
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "GetConceptDescriptionById", "Unhandled"), err
		}
	}

	var jsonable map[string]any
	jsonable, err = jsonization.ToJsonable(cd)
	if err != nil {
		return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "GetConceptDescriptionById", "ToJsonable"), err
	}

	return model.Response(http.StatusOK, jsonable), nil
}

// PutConceptDescriptionById - Creates or updates an existing Concept Description
func (s *ConceptDescriptionRepositoryAPIAPIService) PutConceptDescriptionById(ctx context.Context, cdIdentifier string, conceptDescription types.IConceptDescription) (model.ImplResponse, error) {
	decodedIdentifier, err := common.Decode(cdIdentifier)
	if err != nil {
		return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "PutConceptDescriptionById", "URLDecode"), nil
	}
	err = s.d.PutConceptDescription(string(decodedIdentifier), conceptDescription)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "PutConceptDescriptionById", "BadRequest"), nil
		case common.IsErrDenied(err):
			return common.NewErrorResponse(err, http.StatusForbidden, componentName, "PutConceptDescriptionById", "Denied"), nil
		default:
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "PutConceptDescriptionById", "Unhandled"), err
		}
	}

	jsonable, toJsonErr := jsonization.ToJsonable(conceptDescription)
	if toJsonErr != nil {
		return common.NewErrorResponse(toJsonErr, http.StatusInternalServerError, componentName, "PutConceptDescriptionById", "ToJsonable"), toJsonErr
	}

	return model.Response(http.StatusOK, jsonable), nil
}

// DeleteConceptDescriptionById - Deletes a Concept Description
func (s *ConceptDescriptionRepositoryAPIAPIService) DeleteConceptDescriptionById(ctx context.Context, cdIdentifier string) (model.ImplResponse, error) {
	decodedIdentifier, err := common.Decode(cdIdentifier)
	if err != nil {
		return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "DeleteConceptDescriptionById", "URLDecode"), nil
	}
	err = s.d.DeleteConceptDescription(string(decodedIdentifier))
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(err, http.StatusBadRequest, componentName, "DeleteConceptDescriptionById", "BadRequest"), nil
		case common.IsErrDenied(err):
			return common.NewErrorResponse(err, http.StatusForbidden, componentName, "DeleteConceptDescriptionById", "Denied"), nil
		default:
			return common.NewErrorResponse(err, http.StatusInternalServerError, componentName, "DeleteConceptDescriptionById", "Unhandled"), err
		}
	}

	return model.Response(http.StatusNoContent, nil), nil
}
