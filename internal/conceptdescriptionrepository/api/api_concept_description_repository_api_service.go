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
	"errors"
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
	// TODO - update GetAllConceptDescriptions with the required logic for this service method.
	// Add api_concept_description_repository_api_service.go to the .openapi-generator-ignore to avoid overwriting this service implementation when updating open api generation.

	// TODO: Uncomment the next line to return model.Response Response(200, GetConceptDescriptionsResult{}) or use other options such as http.Ok ...
	// return model.Response(200, GetConceptDescriptionsResult{}), nil

	// TODO: Uncomment the next line to return model.Response Response(400, Result{}) or use other options such as http.Ok ...
	// return model.Response(400, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(403, Result{}) or use other options such as http.Ok ...
	// return model.Response(403, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(500, Result{}) or use other options such as http.Ok ...
	// return model.Response(500, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(0, Result{}) or use other options such as http.Ok ...
	// return model.Response(0, Result{}), nil

	return model.Response(http.StatusNotImplemented, nil), errors.New("GetAllConceptDescriptions method not implemented")
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
	// TODO - update GetConceptDescriptionById with the required logic for this service method.
	// Add api_concept_description_repository_api_service.go to the .openapi-generator-ignore to avoid overwriting this service implementation when updating open api generation.

	// TODO: Uncomment the next line to return model.Response Response(200, ConceptDescription{}) or use other options such as http.Ok ...
	// return model.Response(200, ConceptDescription{}), nil

	// TODO: Uncomment the next line to return model.Response Response(400, Result{}) or use other options such as http.Ok ...
	// return model.Response(400, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(403, Result{}) or use other options such as http.Ok ...
	// return model.Response(403, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(404, Result{}) or use other options such as http.Ok ...
	// return model.Response(404, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(500, Result{}) or use other options such as http.Ok ...
	// return model.Response(500, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(0, Result{}) or use other options such as http.Ok ...
	// return model.Response(0, Result{}), nil

	return model.Response(http.StatusNotImplemented, nil), errors.New("GetConceptDescriptionById method not implemented")
}

// PutConceptDescriptionById - Creates or updates an existing Concept Description
func (s *ConceptDescriptionRepositoryAPIAPIService) PutConceptDescriptionById(ctx context.Context, cdIdentifier string, conceptDescription types.IConceptDescription) (model.ImplResponse, error) {
	// TODO - update PutConceptDescriptionById with the required logic for this service method.
	// Add api_concept_description_repository_api_service.go to the .openapi-generator-ignore to avoid overwriting this service implementation when updating open api generation.

	// TODO: Uncomment the next line to return model.Response Response(201, ConceptDescription{}) or use other options such as http.Ok ...
	// return model.Response(201, ConceptDescription{}), nil

	// TODO: Uncomment the next line to return model.Response Response(204, {}) or use other options such as http.Ok ...
	// return model.Response(204, nil),nil

	// TODO: Uncomment the next line to return model.Response Response(400, Result{}) or use other options such as http.Ok ...
	// return model.Response(400, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(403, Result{}) or use other options such as http.Ok ...
	// return model.Response(403, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(500, Result{}) or use other options such as http.Ok ...
	// return model.Response(500, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(0, Result{}) or use other options such as http.Ok ...
	// return model.Response(0, Result{}), nil

	return model.Response(http.StatusNotImplemented, nil), errors.New("PutConceptDescriptionById method not implemented")
}

// DeleteConceptDescriptionById - Deletes a Concept Description
func (s *ConceptDescriptionRepositoryAPIAPIService) DeleteConceptDescriptionById(ctx context.Context, cdIdentifier string) (model.ImplResponse, error) {
	// TODO - update DeleteConceptDescriptionById with the required logic for this service method.
	// Add api_concept_description_repository_api_service.go to the .openapi-generator-ignore to avoid overwriting this service implementation when updating open api generation.

	// TODO: Uncomment the next line to return model.Response Response(204, {}) or use other options such as http.Ok ...
	// return model.Response(204, nil),nil

	// TODO: Uncomment the next line to return model.Response Response(400, Result{}) or use other options such as http.Ok ...
	// return model.Response(400, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(403, Result{}) or use other options such as http.Ok ...
	// return model.Response(403, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(404, Result{}) or use other options such as http.Ok ...
	// return model.Response(404, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(500, Result{}) or use other options such as http.Ok ...
	// return model.Response(500, Result{}), nil

	// TODO: Uncomment the next line to return model.Response Response(0, Result{}) or use other options such as http.Ok ...
	// return model.Response(0, Result{}), nil

	return model.Response(http.StatusNotImplemented, nil), errors.New("DeleteConceptDescriptionById method not implemented")
}
