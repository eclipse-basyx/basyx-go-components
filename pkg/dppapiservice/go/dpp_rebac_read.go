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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// WriterDB exposes the existing writer pool for shared authorization setup.
func (s *DPPRepositoryService) WriterDB() *sql.DB { return s.aasRepo.WriterDB() }

func dppReBACEnabled(ctx context.Context) bool {
	cfg, ok := common.ConfigFromContext(ctx)
	return ok && cfg.ReBAC.Enabled
}

func (s *DPPRepositoryService) authorizeResolvedDPP(ctx context.Context, resolved resolvedDPP) error {
	if !dppReBACEnabled(ctx) {
		return nil
	}
	aasContext, err := auth.AuthorizeReBACReadResource(ctx, "aas", resolved.aas.ID())
	if err != nil {
		return err
	}
	visibleAAS, err := s.aasRepo.GetAssetAdministrationShellByID(aasContext, resolved.aas.ID())
	if err != nil {
		return err
	}
	if err = auth.RequireCompleteReBACRepresentation(ctx, resolved.aas, visibleAAS); err != nil {
		return err
	}
	content, err := selectedResolvedContentSubmodels(resolved)
	if err != nil {
		return err
	}
	for _, submodel := range append(content, resolved.metadata) {
		if err = s.authorizeDPPSubmodel(ctx, submodel); err != nil {
			return err
		}
	}
	return nil
}

func (s *DPPRepositoryService) authorizeDPPSubmodel(ctx context.Context, submodel types.ISubmodel) error {
	authorized, err := auth.AuthorizeReBACReadResource(ctx, "submodel", submodel.ID())
	if err != nil {
		return err
	}
	visible, err := s.submodelRepo.GetSubmodelByID(authorized, submodel.ID(), "deep", false, true)
	if err != nil {
		return err
	}
	return auth.RequireCompleteReBACRepresentation(ctx, submodel, visible)
}

func (s *DPPRepositoryService) visibleDPPForProduct(ctx context.Context, identifiers aasrepositorydb.DPPAssetIdentifiers) (resolvedDPP, error) {
	resolved, err := s.resolveSubmodelsForAAS(auth.ReBACPlanningContext(ctx), identifiers.AASID, identifiers.DPPID, time.Time{})
	if err != nil {
		return resolved, err
	}
	return resolved, s.authorizeResolvedDPP(ctx, resolved)
}

func (s *DPPRepositoryService) readReBACDPPByProductID(ctx context.Context, productID string, representation Representation) (ImplResponse, error) {
	identifiers, err := s.aasRepo.GetDPPAssetIdentifiersByAssetAndMetadataSemanticIDs(auth.ReBACPlanningContext(ctx), []string{productID}, dppMetadataSemanticIDValues(), 0)
	if err != nil {
		return mapPersistenceError(err, http.StatusInternalServerError), nil
	}
	var visible []resolvedDPP
	for _, identifier := range identifiers {
		resolved, err := s.visibleDPPForProduct(ctx, identifier)
		if common.IsErrDenied(err) || common.IsErrNotFound(err) {
			continue
		}
		if err != nil {
			return mapPersistenceError(err, http.StatusInternalServerError), nil
		}
		visible = append(visible, resolved)
	}
	if len(visible) == 0 {
		return errorResponse(http.StatusNotFound, fmt.Errorf("DPP-REBAC-PRODUCT no visible DPP")), nil
	}
	if len(visible) > 1 {
		return errorResponse(http.StatusConflict, fmt.Errorf("DPP-REBAC-PRODUCT multiple visible DPPs")), nil
	}
	document, err := s.composeLoadedDPP(ctx, visible[0], normalizeRepresentation(representation), false)
	if err != nil {
		return mapPersistenceError(err, http.StatusInternalServerError), nil
	}
	return Response(http.StatusOK, document), nil
}

func (s *DPPRepositoryService) readReBACDPPIDs(ctx context.Context, request ReadDppIdsByProductIdsRequest, limit int32, cursor string) (ImplResponse, error) {
	identifiers, err := s.aasRepo.GetDPPAssetIdentifiersByAssetAndMetadataSemanticIDs(auth.ReBACPlanningContext(ctx), request.ProductIds, dppMetadataSemanticIDValues(), 0)
	if err != nil {
		return mapPersistenceError(err, http.StatusInternalServerError), nil
	}
	visible := map[string]struct{}{}
	for _, identifier := range identifiers {
		if identifier.DPPID <= cursor {
			continue
		}
		_, err := s.visibleDPPForProduct(ctx, identifier)
		if common.IsErrDenied(err) || common.IsErrNotFound(err) {
			continue
		}
		if err != nil {
			return mapPersistenceError(err, http.StatusInternalServerError), nil
		}
		visible[identifier.DPPID] = struct{}{}
	}
	return Response(http.StatusOK, pagedVisibleDPPIDs(visible, limitOrDefault(limit))), nil
}

func pagedVisibleDPPIDs(visible map[string]struct{}, limit int32) DppidSearchResult {
	ids := make([]string, 0, len(visible))
	for id := range visible {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	cursor := ""
	if limit > 0 && len(ids) > int(limit) {
		ids = ids[:limit]
		cursor = ids[len(ids)-1]
	}
	return DppidSearchResult{Items: ids, Cursor: cursor}
}

func (s *DPPRepositoryService) readReBACElement(ctx context.Context, dppID, path string, representation Representation) (ImplResponse, error) {
	planning := auth.ReBACPlanningContext(ctx)
	submodelID, idShortPath, _, err := s.resolveElementPath(planning, dppID, path)
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	authorized, err := auth.AuthorizeReBACReadElement(ctx, submodelID, idShortPath)
	if err != nil {
		return mapPersistenceError(err, http.StatusForbidden), nil
	}
	complete, err := s.submodelRepo.GetSubmodelElement(planning, submodelID, idShortPath, true, "deep")
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	visible, err := s.submodelRepo.GetSubmodelElement(authorized, submodelID, idShortPath, true, "deep")
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	if err = auth.RequireCompleteReBACRepresentation(ctx, complete, visible); err != nil {
		return mapPersistenceError(err, http.StatusForbidden), nil
	}
	serialization, err := s.serializationContext(authorized, submodelID, false)
	if err != nil {
		return mapPersistenceError(err, http.StatusInternalServerError), nil
	}
	body, err := elementResponseWithContext(visible, normalizeRepresentation(representation), path, idShortPath, serialization)
	if err != nil {
		return mapPersistenceError(err, http.StatusInternalServerError), nil
	}
	return Response(http.StatusOK, body), nil
}
