/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// Package assregistryapi implements Asset Administration Shell Registry Service
/*
 * DotAAS Part 2 | HTTP/REST | Asset Administration Shell Registry Service Specification
 *
 * The Full Profile of the Asset Administration Shell Registry Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](https://industrialdigitaltwin.org/en/content-hub/aasspecifications).   Copyright: Industrial Digital Twin Association (IDTA) 2025
 *
 * API version: V3.1.1_SSP-001
 * Contact: info@idtwin.org
 */
// Author: Martin Stemmer ( Fraunhofer IESE )
package assregistryapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	persistence_postgresql "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

const (
	componentName = "AASR"
)

// AssetAdministrationShellRegistryAPIAPIService is a service that implements the logic for the AssetAdministrationShellRegistryAPIAPIServicer
// This service should implement the business logic for every endpoint for the AssetAdministrationShellRegistryAPIAPI API.
// Include any external packages or services that will be required by this service.
type AssetAdministrationShellRegistryAPIAPIService struct {
	aasRegistryBackend persistence_postgresql.PostgreSQLAASRegistryDatabase
}

// NewAssetAdministrationShellRegistryAPIAPIService creates a default api service
func NewAssetAdministrationShellRegistryAPIAPIService(databaseBackend persistence_postgresql.PostgreSQLAASRegistryDatabase) *AssetAdministrationShellRegistryAPIAPIService {
	return &AssetAdministrationShellRegistryAPIAPIService{
		aasRegistryBackend: databaseBackend,
	}
}

// GetAllAssetAdministrationShellDescriptors - Returns all Asset Administration Shell Descriptors
func (s *AssetAdministrationShellRegistryAPIAPIService) GetAllAssetAdministrationShellDescriptors(ctx context.Context, limit int32, cursor string, assetKind model.AssetKind, assetType string) (model.ImplResponse, error) {

	internalCursor, resp, err := decodeCursor(strings.TrimSpace(cursor), "GetAllAssetAdministrationShellDescriptors")
	if resp != nil || err != nil {
		return *resp, err
	}
	aasds, nextCursor, err := s.aasRegistryBackend.ListAssetAdministrationShellDescriptors(ctx, limit, internalCursor, assetKind, assetType)
	if err != nil {
		log.Printf("🧩 [%s] Error in GetAllAssetAdministrationShellDescriptors: list failed (limit=%d cursor=%q assetKind=%q assetType=%q): %v", componentName, limit, internalCursor, string(assetKind), assetType, err)
		switch {
		case common.IsErrBadRequest(err):
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "GetAllAssetAdministrationShellDescriptors", "BadRequest",
			), nil
		default:
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "GetAllAssetAdministrationShellDescriptors", "InternalServerError",
			), err
		}
	}

	return pagedResponse(aasds, nextCursor), nil
}

// PostAssetAdministrationShellDescriptor - Creates a new Asset Administration Shell Descriptor, i.e. registers an AAS
func (s *AssetAdministrationShellRegistryAPIAPIService) PostAssetAdministrationShellDescriptor(ctx context.Context, assetAdministrationShellDescriptor model.AssetAdministrationShellDescriptor) (model.ImplResponse, error) {

	if resp, err := enforceAccessForAAS(ctx, "PostAssetAdministrationShellDescriptor", assetAdministrationShellDescriptor); resp != nil || err != nil {
		return *resp, err
	}
	// Existence check: AAS with same Id should not already exist (lightweight)
	if strings.TrimSpace(assetAdministrationShellDescriptor.Id) != "" {
		if exists, chkErr := s.aasRegistryBackend.ExistsAASByID(ctx, assetAdministrationShellDescriptor.Id); chkErr != nil {
			log.Printf("🧩 [%s] Error in PostAssetAdministrationShellDescriptor: existence check failed (aasId=%q): %v", componentName, assetAdministrationShellDescriptor.Id, chkErr)
			return common.NewErrorResponse(
				chkErr, http.StatusInternalServerError, componentName, "PostAssetAdministrationShellDescriptor", "Unhandled-Precheck",
			), chkErr
		} else if exists {
			e := common.NewErrConflict("AAS with given id already exists")
			log.Printf("🧩 [%s] Error in PostAssetAdministrationShellDescriptor: conflict (aasId=%q): %v", componentName, assetAdministrationShellDescriptor.Id, e)
			return common.NewErrorResponse(
				e, http.StatusConflict, componentName, "PostAssetAdministrationShellDescriptor", "Conflict-Exists",
			), nil
		}
	}

	err := s.aasRegistryBackend.InsertAdministrationShellDescriptor(ctx, assetAdministrationShellDescriptor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			log.Printf("🧩 [%s] Error in InsertAdministrationShellDescriptor: bad request (aasId=%q): %v", componentName, assetAdministrationShellDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "InsertAdministrationShellDescriptor", "BadRequest",
			), nil
		case common.IsErrConflict(err):
			log.Printf("🧩 [%s] Error in InsertAdministrationShellDescriptor: conflict (aasId=%q): %v", componentName, assetAdministrationShellDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusConflict, componentName, "InsertAdministrationShellDescriptor", "Conflict",
			), nil
		default:
			log.Printf("🧩 [%s] Error in InsertAdministrationShellDescriptor: internal (aasId=%q): %v", componentName, assetAdministrationShellDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "InsertAdministrationShellDescriptor", "Unhandled",
			), err
		}
	}

	return model.Response(http.StatusCreated, assetAdministrationShellDescriptor), nil
}

// GetAssetAdministrationShellDescriptorById - Returns a specific Asset Administration Shell Descriptor
// nolint:revive // defined by standard
func (s *AssetAdministrationShellRegistryAPIAPIService) GetAssetAdministrationShellDescriptorById(ctx context.Context, aasIdentifier string) (model.ImplResponse, error) {

	decoded, resp, err := decodePathParam(aasIdentifier, "aasIdentifier", "GetAssetAdministrationShellDescriptorById", "BadRequest-Decode")
	if resp != nil || err != nil {
		return *resp, err
	}

	result, err := s.aasRegistryBackend.GetAssetAdministrationShellDescriptorByID(ctx, decoded)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			log.Printf("🧩 [%s] Error in GetAssetAdministrationShellDescriptorById: bad request (aasId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "GetAssetAdministrationShellDescriptorById", "BadRequest",
			), nil
		case common.IsErrNotFound(err):
			log.Printf("🧩 [%s] Error in GetAssetAdministrationShellDescriptorById: not found (aasId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "GetAssetAdministrationShellDescriptorById", "NotFound",
			), nil
		default:
			log.Printf("🧩 [%s] Error in GetAssetAdministrationShellDescriptorById: internal (aasId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "GetAssetAdministrationShellDescriptorById", "Unhandled",
			), err
		}
	}

	if resp, err := enforceAccessForAAS(ctx, "GetAssetAdministrationShellDescriptorById", result); resp != nil || err != nil {
		return *resp, err
	}

	return model.Response(http.StatusOK, result), nil
}

// PutAssetAdministrationShellDescriptorById - Creates or updates an existing Asset Administration Shell Descriptor
// nolint:revive // defined by standard
func (s *AssetAdministrationShellRegistryAPIAPIService) PutAssetAdministrationShellDescriptorById(ctx context.Context, aasIdentifier string, assetAdministrationShellDescriptor model.AssetAdministrationShellDescriptor) (model.ImplResponse, error) {
	if resp, err := enforceAccessForAAS(ctx, "PutAssetAdministrationShellDescriptorById", assetAdministrationShellDescriptor); resp != nil || err != nil {
		return *resp, err
	}

	// Decode path AAS id
	decodedAAS, resp, err := decodePathParam(aasIdentifier, "aasIdentifier", "PutAssetAdministrationShellDescriptorById", "BadRequest-Decode")
	if resp != nil || err != nil {
		return *resp, err
	}

	// Enforce id consistency with path
	if strings.TrimSpace(assetAdministrationShellDescriptor.Id) != "" && assetAdministrationShellDescriptor.Id != decodedAAS {
		log.Printf("🧩 [%s] Error in PutAssetAdministrationShellDescriptorById: body id does not match path id (body=%q path=%q)", componentName, assetAdministrationShellDescriptor.Id, decodedAAS)
		return common.NewErrorResponse(
			errors.New("body id does not match path id"), http.StatusBadRequest, componentName, "PutAssetAdministrationShellDescriptorById", "BadRequest-IdMismatch",
		), nil
	}
	assetAdministrationShellDescriptor.Id = decodedAAS

	existed, err := s.aasRegistryBackend.ReplaceAdministrationShellDescriptor(ctx, assetAdministrationShellDescriptor)
	if err != nil {
		switch {
		case common.IsErrBadRequest(err):
			log.Printf("🧩 [%s] Error in PutAssetAdministrationShellDescriptorById: bad request (aasId=%q): %v", componentName, decodedAAS, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "PutAssetAdministrationShellDescriptorById", "BadRequest",
			), nil
		case common.IsErrConflict(err):
			log.Printf("🧩 [%s] Error in PutAssetAdministrationShellDescriptorById: conflict (aasId=%q): %v", componentName, decodedAAS, err)
			return common.NewErrorResponse(
				err, http.StatusConflict, componentName, "PutAssetAdministrationShellDescriptorById", "Conflict",
			), nil
		default:
			log.Printf("🧩 [%s] Error in PutAssetAdministrationShellDescriptorById: internal (aasId=%q): %v", componentName, decodedAAS, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "PutAssetAdministrationShellDescriptorById", "Unhandled-Insert",
			), err
		}
	}

	if existed {
		return model.Response(http.StatusNoContent, nil), nil
	}
	return model.Response(http.StatusCreated, assetAdministrationShellDescriptor), nil
}

// DeleteAssetAdministrationShellDescriptorById - Deletes an Asset Administration Shell Descriptor, i.e. de-registers an AAS
// nolint:revive // defined by standard
func (s *AssetAdministrationShellRegistryAPIAPIService) DeleteAssetAdministrationShellDescriptorById(ctx context.Context, aasIdentifier string) (model.ImplResponse, error) {

	decoded, resp, err := decodePathParam(aasIdentifier, "aasIdentifier", "DeleteAssetAdministrationShellDescriptorById", "BadRequest-Decode")
	if resp != nil || err != nil {
		return *resp, err
	}

	if qf := auth.GetQueryFilter(ctx); qf != nil && qf.Formula != nil {
		// retrieve asset shell for security and existence check
		result, resp, err := loadAASForAuth(ctx, s.aasRegistryBackend, decoded, "DeleteAssetAdministrationShellDescriptorById")
		if resp != nil || err != nil {
			return *resp, err
		}

		if resp, err := enforceAccessForAAS(ctx, "DeleteAssetAdministrationShellDescriptorById", result); resp != nil || err != nil {
			return *resp, err
		}
	}
	if err := s.aasRegistryBackend.DeleteAssetAdministrationShellDescriptorByID(ctx, decoded); err != nil {
		switch {
		case common.IsErrNotFound(err):
			log.Printf("🧩 [%s] Error in DeleteAssetAdministrationShellDescriptorById: not found (aasId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "DeleteAssetAdministrationShellDescriptorById", "NotFound",
			), nil
		case common.IsErrBadRequest(err):
			log.Printf("🧩 [%s] Error in DeleteAssetAdministrationShellDescriptorById: bad request (aasId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "DeleteAssetAdministrationShellDescriptorById", "BadRequest",
			), nil
		default:
			log.Printf("🧩 [%s] Error in DeleteAssetAdministrationShellDescriptorById: internal (aasId=%q): %v", componentName, decoded, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "DeleteAssetAdministrationShellDescriptorById", "Unhandled",
			), err
		}
	}

	return model.Response(http.StatusNoContent, nil), nil
}

// GetAllSubmodelDescriptorsThroughSuperpath - Returns all Submodel Descriptors
func (s *AssetAdministrationShellRegistryAPIAPIService) GetAllSubmodelDescriptorsThroughSuperpath(ctx context.Context, aasIdentifier string, limit int32, cursor string) (model.ImplResponse, error) {
	// Decode AAS identifier from path
	decodedAAS, resp, err := decodePathParam(aasIdentifier, "aasIdentifier", "GetAllSubmodelDescriptorsThroughSuperpath", "BadRequest-Decode")
	if resp != nil || err != nil {
		return *resp, err
	}

	// Check AAS existence
	if exists, chkErr := s.aasRegistryBackend.ExistsAASByID(ctx, decodedAAS); chkErr != nil {
		log.Printf("🧩 [%s] Error in GetAllSubmodelDescriptorsThroughSuperpath: existence check failed (aasId=%q): %v", componentName, decodedAAS, chkErr)
		return common.NewErrorResponse(
			chkErr, http.StatusInternalServerError, componentName, "GetAllSubmodelDescriptorsThroughSuperpath", "Unhandled-ExistenceCheck",
		), chkErr
	} else if !exists {
		e := common.NewErrNotFound("AAS not found")
		log.Printf("🧩 [%s] Error in GetAllSubmodelDescriptorsThroughSuperpath: not found (aasId=%q): %v", componentName, decodedAAS, e)
		return common.NewErrorResponse(
			e, http.StatusNotFound, componentName, "GetAllSubmodelDescriptorsThroughSuperpath", "NotFound",
		), nil
	}

	// Decode cursor if provided
	internalCursor, resp, err := decodeCursor(strings.TrimSpace(cursor), "GetAllSubmodelDescriptorsThroughSuperpath")
	if resp != nil || err != nil {
		return *resp, err
	}

	// Read submodel descriptors via persistence layer
	smds, nextCursor, err := s.aasRegistryBackend.ListSubmodelDescriptorsForAAS(ctx, decodedAAS, limit, internalCursor)
	if err != nil {
		log.Printf("🧩 [%s] Error in GetAllSubmodelDescriptorsThroughSuperpath: list failed (aasId=%q limit=%d cursor=%q): %v", componentName, decodedAAS, limit, internalCursor, err)
		return common.NewErrorResponse(
			err, http.StatusInternalServerError, componentName, "GetAllSubmodelDescriptorsThroughSuperpath", "InternalServerError",
		), err
	}

	// Paging metadata and response envelope
	return pagedResponse(smds, nextCursor), nil
}

// PostSubmodelDescriptorThroughSuperpath - Creates a new Submodel Descriptor, i.e. registers a submodel
func (s *AssetAdministrationShellRegistryAPIAPIService) PostSubmodelDescriptorThroughSuperpath(ctx context.Context, aasIdentifier string, submodelDescriptor model.SubmodelDescriptor) (model.ImplResponse, error) {

	if resp, err := enforceAccessForSubmodel(ctx, "PostSubmodelDescriptorThroughSuperpath", submodelDescriptor); resp != nil || err != nil {
		return *resp, err
	}
	decodedAAS, resp, err := decodePathParam(aasIdentifier, "aasIdentifier", "PostSubmodelDescriptorThroughSuperpath", "BadRequest-Decode")
	if resp != nil || err != nil {
		return *resp, err
	}

	// Conflict check: lightweight existence for submodel under this AAS
	if strings.TrimSpace(submodelDescriptor.Id) != "" {
		if exists, chkErr := s.aasRegistryBackend.ExistsSubmodelForAAS(ctx, decodedAAS, submodelDescriptor.Id); chkErr != nil {
			log.Printf("🧩 [%s] Error in PostSubmodelDescriptorThroughSuperpath: existence check failed (aasId=%q submodelId=%q): %v", componentName, decodedAAS, submodelDescriptor.Id, chkErr)
			return common.NewErrorResponse(
				chkErr, http.StatusInternalServerError, componentName, "PostSubmodelDescriptorThroughSuperpath", "Unhandled-Precheck",
			), chkErr
		} else if exists {
			e := common.NewErrConflict("Submodel with given id already exists for this AAS")
			log.Printf("🧩 [%s] Error in PostSubmodelDescriptorThroughSuperpath: conflict (aasId=%q submodelId=%q): %v", componentName, decodedAAS, submodelDescriptor.Id, e)
			return common.NewErrorResponse(
				e, http.StatusConflict, componentName, "PostSubmodelDescriptorThroughSuperpath", "Conflict-Exists",
			), nil
		}
	}

	// Persist submodel descriptor under the AAS
	if err := s.aasRegistryBackend.InsertSubmodelDescriptorForAAS(ctx, decodedAAS, submodelDescriptor); err != nil {
		switch {
		case common.IsErrNotFound(err):
			log.Printf("🧩 [%s] Error in PostSubmodelDescriptorThroughSuperpath: not found (aasId=%q submodelId=%q): %v", componentName, decodedAAS, submodelDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "PostSubmodelDescriptorThroughSuperpath", "NotFound",
			), nil
		case common.IsErrBadRequest(err):
			log.Printf("🧩 [%s] Error in PostSubmodelDescriptorThroughSuperpath: bad request (aasId=%q submodelId=%q): %v", componentName, decodedAAS, submodelDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "PostSubmodelDescriptorThroughSuperpath", "BadRequest",
			), nil
		case common.IsErrConflict(err):
			log.Printf("🧩 [%s] Error in PostSubmodelDescriptorThroughSuperpath: conflict (aasId=%q submodelId=%q): %v", componentName, decodedAAS, submodelDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusConflict, componentName, "PostSubmodelDescriptorThroughSuperpath", "Conflict",
			), nil
		default:
			log.Printf("🧩 [%s] Error in PostSubmodelDescriptorThroughSuperpath: internal (aasId=%q submodelId=%q): %v", componentName, decodedAAS, submodelDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "PostSubmodelDescriptorThroughSuperpath", "Unhandled",
			), err
		}
	}

	return model.Response(http.StatusCreated, submodelDescriptor), nil
}

// GetSubmodelDescriptorByIdThroughSuperpath - Returns a specific Submodel Descriptor
// nolint:revive // defined by standard
func (s *AssetAdministrationShellRegistryAPIAPIService) GetSubmodelDescriptorByIdThroughSuperpath(ctx context.Context, aasIdentifier string, submodelIdentifier string) (model.ImplResponse, error) {
	// Decode path params
	decodedAAS, resp, err := decodePathParam(aasIdentifier, "aasIdentifier", "GetSubmodelDescriptorByIdThroughSuperpath", "BadRequest-Decode-AAS")
	if resp != nil || err != nil {
		return *resp, err
	}
	decodedSMD, resp, err := decodePathParam(submodelIdentifier, "submodelIdentifier", "GetSubmodelDescriptorByIdThroughSuperpath", "BadRequest-Decode-Submodel")
	if resp != nil || err != nil {
		return *resp, err
	}

	smd, err := s.aasRegistryBackend.GetSubmodelDescriptorForAASByID(ctx, decodedAAS, decodedSMD)

	if err != nil {
		switch {
		case common.IsErrNotFound(err):
			log.Printf("🧩 [%s] Error in GetSubmodelDescriptorByIdThroughSuperpath: not found (aasId=%q submodelId=%q): %v", componentName, decodedAAS, decodedSMD, err)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "GetSubmodelDescriptorByIdThroughSuperpath", "NotFound",
			), nil
		default:
			log.Printf("🧩 [%s] Error in GetSubmodelDescriptorByIdThroughSuperpath: internal (aasId=%q submodelId=%q): %v", componentName, decodedAAS, decodedSMD, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "GetSubmodelDescriptorByIdThroughSuperpath", "Unhandled",
			), err
		}
	}

	if resp, err := enforceAccessForSubmodel(ctx, "GetSubmodelDescriptorByIdThroughSuperpath", smd); resp != nil || err != nil {
		return *resp, err
	}

	return model.Response(http.StatusOK, smd), nil
}

// PutSubmodelDescriptorByIdThroughSuperpath - Creates or updates an existing Submodel Descriptor
// nolint:revive // defined by standard
func (s *AssetAdministrationShellRegistryAPIAPIService) PutSubmodelDescriptorByIdThroughSuperpath(ctx context.Context, aasIdentifier string, submodelIdentifier string, submodelDescriptor model.SubmodelDescriptor) (model.ImplResponse, error) {

	if resp, err := enforceAccessForSubmodel(ctx, "PutSubmodelDescriptorByIdThroughSuperpath", submodelDescriptor); resp != nil || err != nil {
		return *resp, err
	}

	// Decode path params
	decodedAAS, resp, err := decodePathParam(aasIdentifier, "aasIdentifier", "PutSubmodelDescriptorByIdThroughSuperpath", "BadRequest-Decode-AAS")
	if resp != nil || err != nil {
		return *resp, err
	}
	decodedSMD, resp, err := decodePathParam(submodelIdentifier, "submodelIdentifier", "PutSubmodelDescriptorByIdThroughSuperpath", "BadRequest-Decode-Submodel")
	if resp != nil || err != nil {
		return *resp, err
	}

	// Enforce id consistency
	if strings.TrimSpace(submodelDescriptor.Id) != "" && submodelDescriptor.Id != decodedSMD {
		log.Printf("🧩 [%s] Error in PutSubmodelDescriptorByIdThroughSuperpath: body id does not match path id (body=%q path=%q)", componentName, submodelDescriptor.Id, decodedSMD)
		return common.NewErrorResponse(
			errors.New("body id does not match path id"), http.StatusBadRequest, componentName, "PutSubmodelDescriptorByIdThroughSuperpath", "BadRequest-IdMismatch",
		), nil
	}
	submodelDescriptor.Id = decodedSMD

	// Replace in a single transaction (delete + insert)
	existed, err := s.aasRegistryBackend.ReplaceSubmodelDescriptorForAAS(ctx, decodedAAS, submodelDescriptor)
	if err != nil {
		switch {
		case common.IsErrNotFound(err):
			log.Printf("🧩 [%s] Error in PutSubmodelDescriptorByIdThroughSuperpath: not found (aasId=%q submodelId=%q): %v", componentName, decodedAAS, submodelDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "PutSubmodelDescriptorByIdThroughSuperpath", "NotFound",
			), nil
		case common.IsErrBadRequest(err):
			log.Printf("🧩 [%s] Error in PutSubmodelDescriptorByIdThroughSuperpath: bad request (aasId=%q submodelId=%q): %v", componentName, decodedAAS, submodelDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "PutSubmodelDescriptorByIdThroughSuperpath", "BadRequest",
			), nil
		case common.IsErrConflict(err):
			log.Printf("🧩 [%s] Error in PutSubmodelDescriptorByIdThroughSuperpath: conflict (aasId=%q submodelId=%q): %v", componentName, decodedAAS, submodelDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusConflict, componentName, "PutSubmodelDescriptorByIdThroughSuperpath", "Conflict",
			), nil
		default:
			log.Printf("🧩 [%s] Error in PutSubmodelDescriptorByIdThroughSuperpath: internal (aasId=%q submodelId=%q): %v", componentName, decodedAAS, submodelDescriptor.Id, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "PutSubmodelDescriptorByIdThroughSuperpath", "Unhandled-Replace",
			), err
		}
	}

	if existed {
		return model.Response(http.StatusNoContent, nil), nil
	}
	return model.Response(http.StatusCreated, submodelDescriptor), nil
}

// DeleteSubmodelDescriptorByIdThroughSuperpath - Deletes a Submodel Descriptor, i.e. de-registers a submodel
// nolint:revive // defined by standard
func (s *AssetAdministrationShellRegistryAPIAPIService) DeleteSubmodelDescriptorByIdThroughSuperpath(ctx context.Context, aasIdentifier string, submodelIdentifier string) (model.ImplResponse, error) {
	decodedAAS, resp, err := decodePathParam(aasIdentifier, "aasIdentifier", "DeleteSubmodelDescriptorByIdThroughSuperpath", "BadRequest-Decode-AAS")
	if resp != nil || err != nil {
		return *resp, err
	}
	decodedSMD, resp, err := decodePathParam(submodelIdentifier, "submodelIdentifier", "DeleteSubmodelDescriptorByIdThroughSuperpath", "BadRequest-Decode-Submodel")
	if resp != nil || err != nil {
		return *resp, err
	}

	if qf := auth.GetQueryFilter(ctx); qf != nil && qf.Formula != nil {
		smd, resp, err := loadSubmodelForAuth(ctx, s.aasRegistryBackend, decodedAAS, decodedSMD, "GetSubmodelDescriptorByIdThroughSuperpath")
		if resp != nil || err != nil {
			return *resp, err
		}
		if resp, err := enforceAccessForSubmodel(ctx, "DeleteSubmodelDescriptorByIdThroughSuperpath", smd); resp != nil || err != nil {
			return *resp, err
		}

	}
	if err := s.aasRegistryBackend.DeleteSubmodelDescriptorForAASByID(ctx, decodedAAS, decodedSMD); err != nil {
		switch {
		case common.IsErrNotFound(err):
			log.Printf("🧩 [%s] Error in DeleteSubmodelDescriptorByIdThroughSuperpath: not found (aasId=%q submodelId=%q): %v", componentName, decodedAAS, decodedSMD, err)
			return common.NewErrorResponse(
				err, http.StatusNotFound, componentName, "DeleteSubmodelDescriptorByIdThroughSuperpath", "NotFound",
			), nil
		case common.IsErrBadRequest(err):
			log.Printf("🧩 [%s] Error in DeleteSubmodelDescriptorByIdThroughSuperpath: bad request (aasId=%q submodelId=%q): %v", componentName, decodedAAS, decodedSMD, err)
			return common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, "DeleteSubmodelDescriptorByIdThroughSuperpath", "BadRequest",
			), nil
		default:
			log.Printf("🧩 [%s] Error in DeleteSubmodelDescriptorByIdThroughSuperpath: internal (aasId=%q submodelId=%q): %v", componentName, decodedAAS, decodedSMD, err)
			return common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, "DeleteSubmodelDescriptorByIdThroughSuperpath", "Unhandled",
			), err
		}
	}

	return model.Response(http.StatusNoContent, nil), nil
}
