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
// Registry of Infrastructures (RoI).
//
// This file provides an implementation of the API service
// interface and contains the business logic glue between HTTP input and the
// persistence backend (see `internal/registryofinfrastructuresservice/persistence`).
//
// The service is responsible for common tasks such as:
//   - decoding/validating request path and query parameters
//   - invoking the backend for CRUD operations on InfrastructureDescriptor objects
//   - mapping backend errors to appropriate HTTP error responses
//   - encoding paged results and response payloads
//
// Exported functionality includes the `RegistryOfInfrastructuresAPIAPIService`
// type, which exposes methods for listing, creating, reading, updating and
// deleting Infrastructure Descriptors. The service expects a backend implementing
// `registryofinfrastructurespostgresql.PostgreSQLRegistryOfInfrastructuresDatabase` that
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
	registryofinfrastructurespostgresql "github.com/eclipse-basyx/basyx-go-components/internal/registryofinfrastructuresservice/persistence"
)

const (
	componentName = "ROI"
)

// RegistryOfInfrastructuresAPIAPIService is a service that implements the logic for the RegistryOfInfrastructuresAPIAPIService
// This service should implement the business logic for every endpoint for the RegistryOfInfrastructuresAPIAPI API.
// Include any external packages or services that will be required by this service.
type RegistryOfInfrastructuresAPIAPIService struct {
	registryOfInfrastructuresBackend registryofinfrastructurespostgresql.PostgreSQLRegistryOfInfrastructuresDatabase
}

// NewRegistryOfInfrastructuresAPIAPIService creates a default api service
func NewRegistryOfInfrastructuresAPIAPIService(registryOfInfrastructuresBackend registryofinfrastructurespostgresql.PostgreSQLRegistryOfInfrastructuresDatabase) *RegistryOfInfrastructuresAPIAPIService {
	return &RegistryOfInfrastructuresAPIAPIService{
		registryOfInfrastructuresBackend: registryOfInfrastructuresBackend,
	}
}

// GetAllInfrastructureDescriptors - Returns all Infrastructure Descriptors
func (s *RegistryOfInfrastructuresAPIAPIService) GetAllInfrastructureDescriptors(ctx context.Context, limit int32, cursor string, name string, assetId string) (model.ImplResponse, error) {
	var internalCursor string
	if strings.TrimSpace(cursor) != "" {
		dec, decErr := common.DecodeString(cursor)
		if decErr != nil {
			log.Printf("📍 [%s] Error in GetAllInfrastructureDescriptors: decode cursor=%q limit=%d name=%q assetId=%q: %v", componentName, cursor, limit, name, assetId, decErr)
			return common.NewErrorResponse(
				decErr, http.StatusBadRequest, componentName, "GetAllInfrastructureDescriptors", "BadCursor",
			), nil
		}
		internalCursor = dec
	}

	var internalName string
	if strings.TrimSpace(name) != "" {
		dec, decErr := common.DecodeString(name)
		if decErr != nil {
			log.Printf("📍 [%s] Error in GetAllInfrastructureDescriptors: decode name=%q limit=%d cursor=%q assetId=%q: %v", componentName, name, limit, internalCursor, assetId, decErr)
			return common.NewErrorResponse(
				decErr, http.StatusBadRequest, componentName, "GetAllInfrastructureDescriptors", "BadName",
			), nil
		}
		internalName = dec
	}

	var internalAssetID string
	if strings.TrimSpace(assetId) != "" {
		dec, decErr := common.DecodeString(assetId)
		if decErr != nil {
			log.Printf("📍 [%s] Error in GetAllInfrastructureDescriptors: decode assetId=%q limit=%d cursor=%q name=%q: %v", componentName, assetId, limit, internalCursor, internalName, decErr)
			return common.NewErrorResponse(
				decErr, http.StatusBadRequest, componentName, "GetAllInfrastructureDescriptors", "BadAssetId",
			), nil
		}
		internalAssetID = dec
	}

	infrastructureDescriptors, nextCursor, err := s.registryOfInfrastructuresBackend.ListInfrastructureDescriptors(ctx, limit, internalCursor, internalName, internalAssetID)
	if err != nil {
		log.Printf("📍 [%s] Error in GetAllInfrastructureDescriptors: list failed (limit=%d cursor=%q name=%q assetId=%q): %v", componentName, limit, internalCursor, internalName, internalAssetID, err)
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "GetAllInfrastructureDescriptors", "BadRequest",
			), nil
		default:
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "GetAllInfrastructureDescriptors", "InternalServerError",
			), err
		}
	}

	jsonable := make([]map[string]any, 0, len(infrastructureDescriptors))
	for _, infrastructureDescriptor := range infrastructureDescriptors {
		j, toJsonErr := infrastructureDescriptor.ToJsonable()
		if toJsonErr != nil {
			log.Printf("📍 [%s] Error in GetAllInfrastructureDescriptors: ToJsonable failed (infrastructureDomain=%q): %v", componentName, infrastructureDescriptor.Domain, toJsonErr)
			return common.NewErrorResponse(
				toJsonErr, http.StatusInternalServerError, componentName, "GetAllInfrastructureDescriptors", "Unhandled-ToJsonable",
			), toJsonErr
		}
		jsonable = append(jsonable, j)
	}

	pm := model.PagedResultPagingMetadata{}
	if nextCursor != "" {
		pm.Cursor = common.EncodeString(nextCursor)
	}

	res := struct {
		PagingMetadata model.PagedResultPagingMetadata `json:"pagingMetadata"`
		Data           []map[string]any                `json:"data"`
	}{
		PagingMetadata: pm,
		Data:           jsonable,
	}

	return model.Response(http.StatusOK, res), nil
}

// PostInfrastructureDescriptor - Creates a new Infrastructure Descriptor, i.e. registers an Infrastructure
func (s *RegistryOfInfrastructuresAPIAPIService) PostInfrastructureDescriptor(ctx context.Context, infrastructureDescriptor model.InfrastructureDescriptor) (model.ImplResponse, error) {
	result, err := s.registryOfInfrastructuresBackend.InsertInfrastructureDescriptor(ctx, infrastructureDescriptor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			log.Printf("📍 [%s] Error in InsertInfrastructureDescriptor: bad request (infrastructureDomain=%q): %v", componentName, infrastructureDescriptor.Domain, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "InsertInfrastructureDescriptor", "BadRequest",
			), nil
		case common.IsErrConflict(err):
			log.Printf("📍 [%s] Error in InsertInfrastructureDescriptor: conflict (infrastructureDomain=%q): %v", componentName, infrastructureDescriptor.Domain, err)
			return common.NewErrorResponse(
				err, http.StatusConflict, componentName, "InsertInfrastructureDescriptor", "Conflict",
			), nil
		default:
			log.Printf("📍 [%s] Error in InsertInfrastructureDescriptor: internal (infrastructureDomain=%q): %v", componentName, infrastructureDescriptor.Domain, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "InsertInfrastructureDescriptor", "Unhandled",
			), err
		}
	}

	j, toJsonErr := result.ToJsonable()
	if toJsonErr != nil {
		log.Printf("📍 [%s] Error in PostInfrastructureDescriptor: ToJsonable failed (infrastructureDomain=%q): %v", componentName, result.Domain, toJsonErr)
		return common.NewErrorResponse(
			toJsonErr, http.StatusInternalServerError, componentName, "PostInfrastructureDescriptor", "Unhandled-ToJsonable",
		), toJsonErr
	}

	return model.Response(http.StatusCreated, j), nil
}

// GetInfrastructureDescriptorById - Returns a specific Infrastructure Descriptor
func (s *RegistryOfInfrastructuresAPIAPIService) GetInfrastructureDescriptorById(ctx context.Context, infrastructureIdentifier string) (model.ImplResponse, error) {
	decoded, decodeErr := common.DecodeString(infrastructureIdentifier)
	if decodeErr != nil {
		log.Printf("📍 [%s] Error in GetInfrastructureDescriptorById: decode infrastructureIdentifier=%q: %v", componentName, infrastructureIdentifier, decodeErr)
		return common.NewErrorResponse(
			decodeErr, http.StatusBadRequest, componentName, "GetInfrastructureDescriptorById", "BadRequest-Decode",
		), nil
	}

	result, err := s.registryOfInfrastructuresBackend.GetInfrastructureDescriptorByID(ctx, decoded)

	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			log.Printf("📍 [%s] Error in GetInfrastructureDescriptorById: bad request (infrastructureId=%q): %v", componentName, string(decoded), err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "GetInfrastructureDescriptorById", "BadRequest",
			), nil
		case common.IsErrNotFound(err):
			log.Printf("📍 [%s] Error in GetInfrastructureDescriptorById: not found (infrastructureId=%q): %v", componentName, string(decoded), err)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "GetInfrastructureDescriptorById", "NotFound",
			), nil
		default:
			log.Printf("📍 [%s] Error in GetInfrastructureDescriptorById: internal (infrastructureId=%q): %v", componentName, string(decoded), err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "GetInfrastructureDescriptorById", "Unhandled",
			), err
		}
	}

	jsonable, toJsonErr := result.ToJsonable()
	if toJsonErr != nil {
		return common.NewErrorResponse(
			toJsonErr, http.StatusInternalServerError, componentName, "GetInfrastructureDescriptorById", "Unhandled-ToJsonable",
		), toJsonErr
	}

	return model.Response(http.StatusOK, jsonable), nil
}

// PutInfrastructureDescriptorById - Updates an existing Infrastructure Descriptor
func (s *RegistryOfInfrastructuresAPIAPIService) PutInfrastructureDescriptorById(ctx context.Context, infrastructureIdentifier string, infrastructureDescriptor model.InfrastructureDescriptor) (model.ImplResponse, error) {
	// Decode path AAS id
	decodedInfrastructure, decErr := common.DecodeString(infrastructureIdentifier)
	if decErr != nil {
		log.Printf("📍 [%s] Error in PutInfrastructureDescriptorById: decode infrastructureIdentifier=%q: %v", componentName, infrastructureIdentifier, decErr)
		return common.NewErrorResponse(
			decErr, http.StatusBadRequest, componentName, "PutInfrastructureDescriptorById", "BadRequest-Decode",
		), nil
	}

	// Enforce domain consistency with path.
	if strings.TrimSpace(infrastructureDescriptor.Domain) == "" {
		infrastructureDescriptor.Domain = decodedInfrastructure
	} else if infrastructureDescriptor.Domain != decodedInfrastructure {
		log.Printf("📍 [%s] Error in PutInfrastructureDescriptorById: body domain does not match path domain (body=%q path=%q)", componentName, infrastructureDescriptor.Domain, decodedInfrastructure)
		return common.NewErrorResponse(
			errors.New("body domain does not match path domain"), http.StatusBadRequest, componentName, "PutInfrastructureDescriptorById", "BadRequest-DomainMismatch",
		), nil
	}

	if exists, chkErr := s.registryOfInfrastructuresBackend.ExistsInfrastructureByID(ctx, infrastructureDescriptor.Domain); chkErr != nil {
		log.Printf("📍 [%s] Error in PutInfrastructureDescriptorById: existence check failed (infrastructureDomain=%q): %v", componentName, infrastructureDescriptor.Domain, chkErr)
		return common.NewErrorResponse(
			chkErr, http.StatusInternalServerError, componentName, "PutInfrastructureDescriptorById", "Unhandled-Precheck",
		), chkErr
	} else if !exists {
		notFoundErr := common.NewErrNotFound("Infrastructure Descriptor not found")
		log.Printf("📍 [%s] Error in PutInfrastructureDescriptorById: not found (infrastructureDomain=%q)", componentName, infrastructureDescriptor.Domain)
		return common.NewErrorResponse(
			notFoundErr, http.StatusNotFound, componentName, "PutInfrastructureDescriptorById", "NotFound",
		), nil
	}

	_, err := s.registryOfInfrastructuresBackend.ReplaceInfrastructureDescriptor(ctx, infrastructureDescriptor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			log.Printf("📍 [%s] Error in PutInfrastructureDescriptorById: bad request (infrastructureId=%q): %v", componentName, decodedInfrastructure, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "PutInfrastructureDescriptorById", "BadRequest",
			), nil
		case common.IsErrConflict(err):
			log.Printf("📍 [%s] Error in PutInfrastructureDescriptorById: conflict (infrastructureId=%q): %v", componentName, decodedInfrastructure, err)
			return common.NewErrorResponse(
				err, http.StatusConflict, componentName, "PutInfrastructureDescriptorById", "Conflict",
			), nil
		default:
			log.Printf("📍 [%s] Error in PutInfrastructureDescriptorById: internal (infrastructureId=%q): %v", componentName, decodedInfrastructure, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "PutInfrastructureDescriptorById", "Unhandled-Insert",
			), err
		}
	}

	return model.Response(http.StatusNoContent, nil), nil
}

// DeleteInfrastructureDescriptorById - Deletes an Infrastructure Descriptor, i.e. de-registers an infrastructure
func (s *RegistryOfInfrastructuresAPIAPIService) DeleteInfrastructureDescriptorById(ctx context.Context, infrastructureIdentifier string) (model.ImplResponse, error) {
	decoded, decodeErr := common.DecodeString(infrastructureIdentifier)
	if decodeErr != nil {
		log.Printf("📍 [%s] Error DeleteInfrastructureDescriptorById: decode infrastructureIdentifier=%q failed: %v", componentName, infrastructureIdentifier, decodeErr)
		return common.NewErrorResponse(
			decodeErr, http.StatusBadRequest, componentName, "DeleteInfrastructureDescriptorById", "BadRequest-Decode",
		), nil
	}

	if err := s.registryOfInfrastructuresBackend.DeleteInfrastructureDescriptorByID(ctx, decoded); err != nil {
		switch {
		case common.IsErrNotFound(err):
			log.Printf("📍 [%s] Error in DeleteInfrastructureDescriptorById: not found (infrastructureId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "DeleteInfrastructureDescriptorById", "NotFound",
			), nil
		case common.IsErrBadRequest(err):
			log.Printf("📍 [%s] Error in DeleteInfrastructureDescriptorById: bad request (infrastructureId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "DeleteInfrastructureDescriptorById", "BadRequest",
			), nil
		default:
			log.Printf("📍 [%s] Error in DeleteInfrastructureDescriptorById: internal (infrastructureId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "DeleteInfrastructureDescriptorById", "Unhandled",
			), err
		}
	}

	return model.Response(http.StatusNoContent, nil), nil
}
