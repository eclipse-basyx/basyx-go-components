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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
)

const (
	defaultDPPPageLimit        int32 = 100
	dppProductIDSearchPageSize int32 = 500
	maxDPPProductIDSearchItems       = 100
	maxDPPRequestBodyBytes     int64 = 10 << 20
)

// DPPRepositoryService persists and retrieves Digital Product Passport documents.
//
// Fields:
//   - aasRepo: Persistence repository for Asset Administration Shell records
//   - submodelRepo: Persistence repository for DPP metadata and content submodels
type DPPRepositoryService struct {
	aasRepo      *aasrepositorydb.AssetAdministrationShellDatabase
	submodelRepo *submodelrepositorydb.SubmodelDatabase
}

// NewDPPRepositoryService creates a DPP repository service backed by AAS and submodel repositories.
//
// Parameters:
//   - aasRepo: Persistence repository for Asset Administration Shell records
//   - submodelRepo: Persistence repository for DPP metadata and content submodels
//
// Returns:
//   - *DPPRepositoryService: Configured DPP repository service
func NewDPPRepositoryService(aasRepo *aasrepositorydb.AssetAdministrationShellDatabase, submodelRepo *submodelrepositorydb.SubmodelDatabase) *DPPRepositoryService {
	return &DPPRepositoryService{aasRepo: aasRepo, submodelRepo: submodelRepo}
}

// CreateDPPFromJSON creates a DPP from a compressed JSON document.
//
// Parameters:
//   - ctx: Request context used for repository persistence calls
//   - data: Compressed DPP JSON document bytes
//
// Returns:
//   - ImplResponse: HTTP-style response containing the created DPP identifier or validation error
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) CreateDPPFromJSON(ctx context.Context, data []byte) (ImplResponse, error) {
	doc, header, err := decodeDPPDocument(data, true)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}

	sections := contentSections(doc)
	submodels, refs, err := s.buildSubmodels(header, sections)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}
	aas := buildAAS(header, refs)

	err = s.aasRepo.ExecuteInTransaction("DPP-CREATEDPP-STARTTX", "DPP-CREATEDPP-COMMITTX", func(tx *sql.Tx) error {
		if err := s.aasRepo.CreateAssetAdministrationShellInTransaction(ctx, tx, aas); err != nil {
			return fmt.Errorf("DPP-CREATEDPP-CREATEAAS create AAS: %w", err)
		}
		for _, submodel := range submodels {
			if err := s.submodelRepo.CreateSubmodelInTransaction(ctx, tx, submodel); err != nil {
				return fmt.Errorf("DPP-CREATEDPP-CREATESUBMODEL create submodel %s: %w", submodel.ID(), err)
			}
		}
		return nil
	})
	if err != nil {
		return mapPersistenceError(err, http.StatusConflict), nil
	}

	return Response(http.StatusCreated, CreateDppResponse{DigitalProductPassportId: header.DigitalProductPassportID}), nil
}

// UpdateDPPFromJSON applies a JSON merge patch to an existing DPP.
//
// Parameters:
//   - ctx: Request context used for repository persistence calls
//   - dppID: Identifier of the DPP to update
//   - data: Compressed JSON merge patch document bytes
//
// Returns:
//   - ImplResponse: HTTP-style response containing the updated DPP or mapped error payload
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) UpdateDPPFromJSON(ctx context.Context, dppID string, data []byte) (ImplResponse, error) {
	patch, _, err := decodeDPPDocument(data, false)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}
	currentResolved, err := s.resolveSubmodels(ctx, dppID, time.Time{})
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	current, err := s.composeDPP(ctx, dppID, REPRESENTATION_COMPRESSED, time.Time{})
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	mergedAny := applyMergePatch(current, patch)
	merged := dppObjectFromAny(mergedAny)
	if merged == nil {
		return errorResponse(http.StatusBadRequest, errors.New("DPP-UPDDPP-MERGE merged DPP must be a JSON object")), nil
	}
	merged[headerDigitalProductPassportID] = dppID
	merged[headerLastUpdate] = time.Now().UTC().Format(time.RFC3339Nano)

	raw, err := json.Marshal(merged)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, fmt.Errorf("DPP-UPDDPP-MARSHAL marshal merged DPP: %w", err)), nil
	}
	_, header, err := decodeDPPDocument(raw, true)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}

	sections := contentSections(merged)
	submodels, refs, err := s.buildSubmodels(header, sections)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}
	aas := buildAAS(header, refs)
	staleSubmodelIDs := staleContentSubmodelIDs(currentResolved, submodels)

	err = s.aasRepo.ExecuteInTransaction("DPP-UPDDPP-STARTTX", "DPP-UPDDPP-COMMITTX", func(tx *sql.Tx) error {
		if _, err := s.aasRepo.PutAssetAdministrationShellByIDInTransaction(ctx, tx, dppID, aas); err != nil {
			return fmt.Errorf("DPP-UPDDPP-PUTAAS put AAS: %w", err)
		}
		for _, submodel := range submodels {
			if _, err := s.submodelRepo.PutSubmodelInTransaction(ctx, tx, submodel.ID(), submodel); err != nil {
				return fmt.Errorf("DPP-UPDDPP-PUTSUBMODEL put submodel %s: %w", submodel.ID(), err)
			}
		}
		for _, submodelID := range staleSubmodelIDs {
			if err := s.submodelRepo.DeleteSubmodelInTransaction(ctx, tx, submodelID); err != nil {
				return fmt.Errorf("DPP-UPDDPP-DELETESUBMODEL delete stale submodel %s: %w", submodelID, err)
			}
		}
		return nil
	})
	if err != nil {
		return mapPersistenceError(err, http.StatusConflict), nil
	}

	updated, err := s.composeDPP(ctx, dppID, REPRESENTATION_COMPRESSED, time.Time{})
	if err != nil {
		return mapPersistenceError(err, http.StatusInternalServerError), nil
	}
	return Response(http.StatusOK, updated), nil
}

func staleContentSubmodelIDs(current resolvedDPP, replacement []types.ISubmodel) []string {
	replacementIDs := make(map[string]struct{}, len(replacement))
	for _, submodel := range replacement {
		replacementIDs[submodel.ID()] = struct{}{}
	}
	stale := make([]string, 0)
	for _, submodel := range current.submodels {
		if submodel.ID() == current.metadata.ID() {
			continue
		}
		if _, stillPresent := replacementIDs[submodel.ID()]; !stillPresent {
			stale = append(stale, submodel.ID())
		}
	}
	sort.Strings(stale)
	return stale
}

func dppObjectFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case dppDocument:
		return map[string]any(typed)
	default:
		return nil
	}
}

// ReadDPPById reads a DPP by its identifier.
//
// Parameters:
//   - ctx: Request context used for repository read calls
//   - dppID: Identifier of the DPP to read
//   - representation: Requested compressed or full DPP representation
//
// Returns:
//   - ImplResponse: HTTP-style response containing the DPP document or mapped error payload
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) ReadDPPById(ctx context.Context, dppID string, representation Representation) (ImplResponse, error) {
	doc, err := s.composeDPP(ctx, dppID, normalizeRepresentation(representation), time.Time{})
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	return Response(http.StatusOK, doc), nil
}

// DeleteDPPById deletes a DPP and its currently referenced submodels.
//
// Parameters:
//   - ctx: Request context used for repository persistence calls
//   - dppID: Identifier of the DPP to delete
//
// Returns:
//   - ImplResponse: HTTP-style response with no body on success or a mapped error payload
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) DeleteDPPById(ctx context.Context, dppID string) (ImplResponse, error) {
	resolved, err := s.resolveSubmodels(ctx, dppID, time.Time{})
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}

	err = s.aasRepo.ExecuteInTransaction("DPP-DELDPP-STARTTX", "DPP-DELDPP-COMMITTX", func(tx *sql.Tx) error {
		for _, submodel := range resolved.submodels {
			if err := s.submodelRepo.DeleteSubmodelInTransaction(ctx, tx, submodel.ID()); err != nil {
				return fmt.Errorf("DPP-DELDPP-DELETESUBMODEL delete submodel %s: %w", submodel.ID(), err)
			}
		}
		if err := s.aasRepo.DeleteAssetAdministrationShellByIDInTransaction(ctx, tx, dppID); err != nil {
			return fmt.Errorf("DPP-DELDPP-DELETEAAS delete AAS: %w", err)
		}
		return nil
	})
	if err != nil {
		return mapPersistenceError(err, http.StatusConflict), nil
	}
	return Response(http.StatusNoContent, nil), nil
}

// ReadDPPByProductId resolves a unique product ID to its DPP.
//
// Parameters:
//   - ctx: Request context used for repository read calls
//   - productID: Unique product identifier used to find matching DPP shells
//   - representation: Requested compressed or full DPP representation
//
// Returns:
//   - ImplResponse: HTTP-style response containing the resolved DPP or mapped error payload
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) ReadDPPByProductId(ctx context.Context, productID string, representation Representation) (ImplResponse, error) {
	ids := make([]string, 0)
	seen := make(map[string]struct{})
	if err := s.collectDPPIDsForProduct(ctx, productID, seen, &ids); err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	if len(ids) == 0 {
		return errorResponse(http.StatusNotFound, fmt.Errorf("DPP-READBYPRODUCT-NOTFOUND no DPP for product %s", productID)), nil
	}
	if len(ids) > 1 {
		return errorResponse(http.StatusConflict, fmt.Errorf("DPP-READBYPRODUCT-AMBIGUOUS multiple DPPs for product %s", productID)), nil
	}
	doc, err := s.composeDPP(ctx, ids[0], normalizeRepresentation(representation), time.Time{})
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	return Response(http.StatusOK, doc), nil
}

// ReadDPPVersionByIdAndDate reads a historic DPP version at the requested timestamp.
//
// Parameters:
//   - ctx: Request context used for repository read calls
//   - dppID: Identifier of the DPP to read
//   - date: Historical timestamp to resolve
//   - representation: Requested compressed or full DPP representation
//
// Returns:
//   - ImplResponse: HTTP-style response containing the historic DPP or mapped error payload
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) ReadDPPVersionByIdAndDate(ctx context.Context, dppID string, date time.Time, representation Representation) (ImplResponse, error) {
	doc, err := s.composeDPP(ctx, dppID, normalizeRepresentation(representation), date)
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	return Response(http.StatusOK, doc), nil
}

// ReadDPPIdsByProductIds resolves product IDs to sorted, paged DPP IDs.
//
// Parameters:
//   - ctx: Request context used for repository read calls
//   - request: Product ID search request
//   - limit: Maximum number of DPP IDs to return
//   - cursor: Cursor after which the next page starts
//
// Returns:
//   - ImplResponse: HTTP-style response containing a paged DPP ID search result
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) ReadDPPIdsByProductIds(ctx context.Context, request ReadDppIdsByProductIdsRequest, limit int32, cursor string) (ImplResponse, error) {
	ids := make([]string, 0)
	seen := make(map[string]struct{})
	for _, productID := range request.ProductIds {
		if err := s.collectDPPIDsForProduct(ctx, productID, seen, &ids); err != nil {
			return mapPersistenceError(err, http.StatusInternalServerError), nil
		}
	}
	sort.Strings(ids)
	paged, nextCursor := pageStrings(ids, limitOrDefault(limit), cursor)
	return Response(http.StatusOK, DppidSearchResult{Items: paged, Cursor: nextCursor}), nil
}

func (s *DPPRepositoryService) collectDPPIDsForProduct(ctx context.Context, productID string, seen map[string]struct{}, ids *[]string) error {
	cursor := ""
	for {
		shells, nextCursor, err := s.aasRepo.GetAssetAdministrationShells(ctx, dppProductIDSearchPageSize, cursor, "", []string{productID})
		if err != nil {
			return fmt.Errorf("DPP-READIDS-GETAAS get AAS for product %s: %w", productID, err)
		}
		for _, shell := range shells {
			if _, ok := seen[shell.ID()]; ok {
				continue
			}
			if !s.isDPPShell(ctx, shell.ID()) {
				continue
			}
			seen[shell.ID()] = struct{}{}
			*ids = append(*ids, shell.ID())
		}
		if nextCursor == "" {
			return nil
		}
		if nextCursor == cursor {
			return fmt.Errorf("DPP-READIDS-CURSOR repository returned repeated cursor %q", cursor)
		}
		cursor = nextCursor
	}
}

func (s *DPPRepositoryService) isDPPShell(ctx context.Context, shellID string) bool {
	if _, err := s.resolveSubmodels(ctx, shellID, time.Time{}); err != nil {
		return false
	}
	return true
}

// ReadDataElement reads one DPP data element by content section and idShort path.
//
// Parameters:
//   - ctx: Request context used for repository read calls
//   - dppID: Identifier of the DPP that owns the element
//   - elementPath: Content section and idShort path in <section>/<path> form
//   - representation: Requested compressed or full element representation
//
// Returns:
//   - ImplResponse: HTTP-style response containing the DPP data element or mapped error payload
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) ReadDataElement(ctx context.Context, dppID string, elementPath string, representation Representation) (ImplResponse, error) {
	submodelID, idShortPath, err := s.resolveElementPath(ctx, dppID, elementPath)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}
	element, err := s.submodelRepo.GetSubmodelElement(ctx, submodelID, idShortPath, false, "deep")
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	body, err := elementResponse(element, normalizeRepresentation(representation))
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err), nil
	}
	return Response(http.StatusOK, body), nil
}

// UpdateDataElementFromJSON replaces one DPP data element from its compressed JSON value.
//
// Parameters:
//   - ctx: Request context used for repository persistence calls
//   - dppID: Identifier of the DPP that owns the element
//   - elementPath: Content section and idShort path in <section>/<path> form
//   - data: Compressed JSON value used as the replacement element content
//
// Returns:
//   - ImplResponse: HTTP-style response containing the updated DPP data element or mapped error payload
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) UpdateDataElementFromJSON(ctx context.Context, dppID string, elementPath string, data []byte) (ImplResponse, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-UPDELEM-DECODE decode element body: %w", err)), nil
	}
	submodelID, idShortPath, err := s.resolveElementPath(ctx, dppID, elementPath)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}
	existing, err := s.submodelRepo.GetSubmodelElement(ctx, submodelID, idShortPath, false, "deep")
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	idShortParts := strings.Split(idShortPath, ".")
	element, err := inferElement(idShortParts[len(idShortParts)-1], value)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}
	preserveElementMetadata(existing, element)
	metadata, err := s.updatedMetadata(ctx, dppID)
	if err != nil {
		return mapPersistenceError(err, http.StatusInternalServerError), nil
	}

	err = s.aasRepo.ExecuteInTransaction("DPP-UPDELEM-STARTTX", "DPP-UPDELEM-COMMITTX", func(tx *sql.Tx) error {
		if _, err := s.submodelRepo.PutSubmodelElementInTransaction(ctx, tx, submodelID, idShortPath, element); err != nil {
			return fmt.Errorf("DPP-UPDELEM-PUTELEMENT put element %s: %w", idShortPath, err)
		}
		if _, err := s.submodelRepo.PutSubmodelInTransaction(ctx, tx, metadata.ID(), metadata); err != nil {
			return fmt.Errorf("DPP-UPDELEM-PUTMETADATA put metadata: %w", err)
		}
		return nil
	})
	if err != nil {
		return mapPersistenceError(err, http.StatusConflict), nil
	}

	return s.ReadDataElement(ctx, dppID, elementPath, REPRESENTATION_COMPRESSED)
}

func preserveElementMetadata(existing types.ISubmodelElement, replacement types.ISubmodelElement) {
	replacement.SetExtensions(existing.Extensions())
	replacement.SetCategory(existing.Category())
	replacement.SetDisplayName(existing.DisplayName())
	replacement.SetDescription(existing.Description())
	replacement.SetSemanticID(existing.SemanticID())
	replacement.SetSupplementalSemanticIDs(existing.SupplementalSemanticIDs())
	replacement.SetQualifiers(existing.Qualifiers())
	replacement.SetEmbeddedDataSpecifications(existing.EmbeddedDataSpecifications())
}

// UpdateDataElement replaces one DPP data element from a generated model value.
//
// Parameters:
//   - ctx: Request context used for repository persistence calls
//   - dppID: Identifier of the DPP that owns the element
//   - elementPath: Content section and idShort path in <section>/<path> form
//   - dataElement: Generated DPP data element model used as replacement content
//
// Returns:
//   - ImplResponse: HTTP-style response containing the updated DPP data element or mapped error payload
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) UpdateDataElement(ctx context.Context, dppID string, elementPath string, dataElement DataElement) (ImplResponse, error) {
	raw, err := json.Marshal(dataElement)
	if err != nil {
		return errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-UPDELEM-MARSHAL marshal generated data element: %w", err)), nil
	}
	return s.UpdateDataElementFromJSON(ctx, dppID, elementPath, raw)
}

// CreateDPP creates a DPP from the generated OpenAPI model.
//
// Parameters:
//   - ctx: Request context used for repository persistence calls
//   - passport: Generated OpenAPI DPP model to persist
//
// Returns:
//   - ImplResponse: HTTP-style response containing the created DPP identifier or validation error
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) CreateDPP(ctx context.Context, passport DigitalProductPassport) (ImplResponse, error) {
	raw, err := json.Marshal(passport)
	if err != nil {
		return errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-CREATEDPP-MARSHAL marshal generated DPP: %w", err)), nil
	}
	return s.CreateDPPFromJSON(ctx, raw)
}

// UpdateDPPById applies a generated model patch to an existing DPP.
//
// Parameters:
//   - ctx: Request context used for repository persistence calls
//   - dppID: Identifier of the DPP to update
//   - patch: Generated OpenAPI DPP patch model
//
// Returns:
//   - ImplResponse: HTTP-style response containing the updated DPP or mapped error payload
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) UpdateDPPById(ctx context.Context, dppID string, patch DigitalProductPassportPatch) (ImplResponse, error) {
	raw, err := json.Marshal(patch)
	if err != nil {
		return errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-UPDDPP-MARSHAL marshal generated patch: %w", err)), nil
	}
	return s.UpdateDPPFromJSON(ctx, dppID, raw)
}

type resolvedDPP struct {
	metadata  types.ISubmodel
	submodels []types.ISubmodel
}

func (s *DPPRepositoryService) composeDPP(ctx context.Context, dppID string, representation Representation, at time.Time) (dppDocument, error) {
	resolved, err := s.resolveSubmodels(ctx, dppID, at)
	if err != nil {
		return nil, err
	}
	doc, err := composeHeader(resolved.metadata)
	if err != nil {
		return nil, err
	}
	for _, submodel := range resolved.submodels {
		if submodel.ID() == resolved.metadata.ID() {
			continue
		}
		sectionName := lowerFirst(idShortOrID(submodel))
		var content any
		if representation == REPRESENTATION_FULL {
			content, err = fullContent(submodel)
		} else {
			content, err = compressedContent(submodel)
		}
		if err != nil {
			return nil, err
		}
		doc[sectionName] = content
	}
	return doc, nil
}

func (s *DPPRepositoryService) resolveSubmodels(ctx context.Context, dppID string, at time.Time) (resolvedDPP, error) {
	var aas types.IAssetAdministrationShell
	var err error
	if at.IsZero() {
		aas, err = s.aasRepo.GetAssetAdministrationShellByID(ctx, dppID)
	} else {
		aas, err = s.aasRepo.GetAssetAdministrationShellByIDAndDate(ctx, dppID, at)
	}
	if err != nil {
		return resolvedDPP{}, fmt.Errorf("DPP-RESOLVE-GETAAS get AAS %s: %w", dppID, err)
	}

	submodels := make([]types.ISubmodel, 0, len(aas.Submodels()))
	var metadata types.ISubmodel
	for _, ref := range aas.Submodels() {
		submodelID := referenceLastValue(ref)
		if submodelID == "" {
			continue
		}
		var submodel types.ISubmodel
		if at.IsZero() {
			submodel, err = s.submodelRepo.GetSubmodelByID(ctx, submodelID, "deep", false)
		} else {
			submodel, err = s.submodelRepo.GetSubmodelByIDAndDate(ctx, submodelID, at)
		}
		if err != nil {
			return resolvedDPP{}, fmt.Errorf("DPP-RESOLVE-GETSUBMODEL get submodel %s: %w", submodelID, err)
		}
		if hasDPPMetadataSemanticID(submodel) || idShortOrID(submodel) == dppMetadataIDShort {
			metadata = submodel
		}
		submodels = append(submodels, submodel)
	}
	if metadata == nil {
		return resolvedDPP{}, fmt.Errorf("DPP-RESOLVE-METADATA DppMetadata submodel not found for %s", dppID)
	}
	return resolvedDPP{metadata: metadata, submodels: submodels}, nil
}

func (s *DPPRepositoryService) buildSubmodels(header dppHeader, sections map[string]any) ([]types.ISubmodel, []types.IReference, error) {
	metadata := buildMetadataSubmodel(header.DigitalProductPassportID, header)
	submodels := []types.ISubmodel{metadata}
	refs := []types.IReference{submodelReference(metadata.ID())}

	semanticIDs, err := semanticIDsForSections(sections, header.ContentSpecificationIDs)
	if err != nil {
		return nil, nil, err
	}
	sectionNames := sortedKeys(sections)
	for _, sectionName := range sectionNames {
		submodel, err := buildContentSubmodel(header.DigitalProductPassportID, sectionName, semanticIDs[sectionName], sections[sectionName])
		if err != nil {
			return nil, nil, err
		}
		submodels = append(submodels, submodel)
		refs = append(refs, submodelReference(submodel.ID()))
	}
	return submodels, refs, nil
}

func (s *DPPRepositoryService) resolveElementPath(ctx context.Context, dppID string, elementPath string) (string, string, error) {
	parts := strings.SplitN(elementPath, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("DPP-ELEMPATH-INVALID elementPath must be <contentSection>/<idShort.path>")
	}
	resolved, err := s.resolveSubmodels(ctx, dppID, time.Time{})
	if err != nil {
		return "", "", err
	}
	for _, submodel := range resolved.submodels {
		if submodel.ID() == resolved.metadata.ID() {
			continue
		}
		if lowerFirst(idShortOrID(submodel)) == parts[0] {
			return submodel.ID(), parts[1], nil
		}
	}
	return "", "", fmt.Errorf("DPP-ELEMPATH-NOTFOUND content section %s not found", parts[0])
}

func (s *DPPRepositoryService) updatedMetadata(ctx context.Context, dppID string) (types.ISubmodel, error) {
	metadata, err := s.submodelRepo.GetSubmodelByID(ctx, metadataSubmodelID(dppID), "deep", false)
	if err != nil {
		return nil, fmt.Errorf("DPP-TOUCHMETA-GET get metadata: %w", err)
	}
	for _, element := range metadata.SubmodelElements() {
		if element.IDShort() != nil && *element.IDShort() == headerLastUpdate {
			value := time.Now().UTC().Format(time.RFC3339Nano)
			if property, ok := element.(*types.Property); ok {
				property.SetValue(&value)
			}
		}
	}
	return metadata, nil
}

func elementResponse(element types.ISubmodelElement, representation Representation) (any, error) {
	if representation == REPRESENTATION_FULL {
		response, err := dppElementFromAAS(element)
		if err != nil {
			return nil, fmt.Errorf("DPP-ELEM-FULL convert element to DPP expanded representation: %w", err)
		}
		return response, nil
	}
	idShort := element.IDShort()
	if idShort == nil || *idShort == "" {
		return nil, fmt.Errorf("DPP-ELEM-COMPRESSED element has no idShort")
	}
	submodel := types.NewSubmodel("dpp-element-response")
	submodel.SetSubmodelElements([]types.ISubmodelElement{element})
	content, err := compressedContent(submodel)
	if err != nil {
		return nil, err
	}
	object, ok := content.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("DPP-ELEM-COMPRESSED compressed element response is not an object")
	}
	return object[*idShort], nil
}

func idShortOrID(submodel types.ISubmodel) string {
	if submodel.IDShort() != nil && *submodel.IDShort() != "" {
		return *submodel.IDShort()
	}
	return submodel.ID()
}

func normalizeRepresentation(representation Representation) Representation {
	if representation == "" {
		return REPRESENTATION_COMPRESSED
	}
	return representation
}

func limitOrDefault(limit int32) int32 {
	if limit <= 0 {
		return defaultDPPPageLimit
	}
	return limit
}

func pageStrings(values []string, limit int32, cursor string) ([]string, string) {
	start := 0
	if cursor != "" {
		for index, value := range values {
			if value == cursor {
				start = index + 1
				break
			}
		}
	}
	end := start + int(limit)
	if end > len(values) {
		end = len(values)
	}
	nextCursor := ""
	if end < len(values) {
		nextCursor = values[end-1]
	}
	return values[start:end], nextCursor
}
