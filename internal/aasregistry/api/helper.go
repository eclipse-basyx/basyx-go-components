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
// Author: Martin Stemmer ( Fraunhofer IESE )

package aasregistryapi

import (
	"context"
	"log"
	"net/http"

	persistence_postgresql "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// decodePathParam decodes an URL path component and builds a consistent error response.
func decodePathParam(raw, paramName, operation, errorDetail string) (string, *model.ImplResponse, error) {
	decoded, err := common.DecodeString(raw)
	if err != nil {
		log.Printf("🧩 [%s] Error in %s: decode %s=%q: %v", componentName, operation, paramName, raw, err)
		resp := common.NewErrorResponse(
			err, http.StatusBadRequest, componentName, operation, errorDetail,
		)
		return "", &resp, nil
	}
	return decoded, nil, nil
}

// decodeCursor wraps cursor decoding with shared logging + error response.
func decodeCursor(raw, operation string) (string, *model.ImplResponse, error) {
	if raw == "" {
		return "", nil, nil
	}
	return decodePathParam(raw, "cursor", operation, "BadCursor")
}

type accessEvaluator func(formula *auth.QueryFilter) (bool, error)

// enforceAccess evaluates the query filter formula (when present) and returns a ready-to-send response on denial or error.
func enforceAccess(ctx context.Context, operation, targetID string, evaluate accessEvaluator) (*model.ImplResponse, error) {
	qf := auth.GetQueryFilter(ctx)
	if qf == nil || qf.Formula == nil {
		return nil, nil
	}

	ok, err := evaluate(qf)
	if err != nil {
		log.Printf("🧩 [%s] Error in %s: evaluation failed (id=%q): %v", componentName, operation, targetID, err)
		resp := common.NewErrorResponse(
			err, http.StatusInternalServerError, componentName, operation, "Unhandled",
		)
		return &resp, err
	}
	if !ok {
		log.Printf("🧩 [%s] Access denied in %s (id=%q)", componentName, operation, targetID)
		resp := common.NewAccessDeniedResponse()
		return &resp, nil
	}
	return nil, nil
}

// enforceAccessForAAS wraps enforceAccess for AAS descriptors.
func enforceAccessForAAS(ctx context.Context, operation string, aas model.AssetAdministrationShellDescriptor) (*model.ImplResponse, error) {
	return enforceAccess(ctx, operation, aas.Id, func(qf *auth.QueryFilter) (bool, error) {
		return qf.Formula.EvaluateAssetAdministrationShellDescriptor(aas)
	})
}

// enforceAccessForSubmodel wraps enforceAccess for submodel descriptors.
func enforceAccessForSubmodel(ctx context.Context, operation string, smd model.SubmodelDescriptor) (*model.ImplResponse, error) {
	return enforceAccess(ctx, operation, smd.Id, func(qf *auth.QueryFilter) (bool, error) {
		return qf.Formula.EvaluateSubmodelDescriptor(smd)
	})
}

// pagedResponse builds the common paged envelope used across list endpoints.
func pagedResponse(results interface{}, nextCursor string) model.ImplResponse {
	pm := model.PagedResultPagingMetadata{}
	if nextCursor != "" {
		pm.Cursor = common.EncodeString(nextCursor)
	}
	res := struct {
		PagingMetadata interface{} `json:"pagingMetadata"`
		Result         interface{} `json:"result"`
	}{
		PagingMetadata: pm,
		Result:         results,
	}
	return model.Response(http.StatusOK, res)
}

// loadAASForAuth retrieves an AAS descriptor and returns a ready-to-send response on handled errors.
func loadAASForAuth(ctx context.Context, backend persistence_postgresql.PostgreSQLAASRegistryDatabase, aasID string, operation string) (model.AssetAdministrationShellDescriptor, *model.ImplResponse, error) {
	result, err := backend.GetAssetAdministrationShellDescriptorByID(ctx, aasID)
	if err != nil {
		switch {
		case common.IsErrNotFound(err):
			log.Printf("🧩 [%s] Error in %s: not found (aasId=%q): %v", componentName, operation, aasID, err)
			resp := common.NewErrorResponse(
				err, http.StatusNotFound, componentName, operation, "NotFound",
			)
			return model.AssetAdministrationShellDescriptor{}, &resp, nil
		case common.IsErrBadRequest(err):
			log.Printf("🧩 [%s] Error in %s: bad request (aasId=%q): %v", componentName, aasID, operation, err)
			resp := common.NewErrorResponse(
				err, http.StatusBadRequest, componentName, operation, "BadRequest",
			)
			return model.AssetAdministrationShellDescriptor{}, &resp, nil
		default:
			log.Printf("🧩 [%s] Error in %s: internal (aasId=%q): %v", componentName, operation, aasID, err)
			resp := common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, operation, "Unhandled",
			)
			return model.AssetAdministrationShellDescriptor{}, &resp, err
		}
	}
	return result, nil, nil
}

// loadSubmodelForAuth retrieves a submodel descriptor and returns a ready-to-send response on handled errors.
func loadSubmodelForAuth(ctx context.Context, backend persistence_postgresql.PostgreSQLAASRegistryDatabase, aasID, smdID, operation string) (model.SubmodelDescriptor, *model.ImplResponse, error) {
	result, err := backend.GetSubmodelDescriptorForAASByID(ctx, aasID, smdID)
	if err != nil {
		switch {
		case common.IsErrNotFound(err):
			log.Printf("🧩 [%s] Error in %s: not found (aasId=%q submodelId=%q): %v", componentName, operation, aasID, smdID, err)
			resp := common.NewErrorResponse(
				err, http.StatusNotFound, componentName, operation, "NotFound",
			)
			return model.SubmodelDescriptor{}, &resp, nil
		default:
			log.Printf("🧩 [%s] Error in %s: internal (aasId=%q submodelId=%q): %v", componentName, operation, aasID, smdID, err)
			resp := common.NewErrorResponse(
				err, http.StatusInternalServerError, componentName, operation, "Unhandled",
			)
			return model.SubmodelDescriptor{}, &resp, err
		}
	}
	return result, nil, nil
}
