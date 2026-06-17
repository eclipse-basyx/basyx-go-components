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
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	"github.com/go-chi/chi/v5"
)

const (
	defaultDPPPageLimit        int32 = 100
	dppProductIDSearchPageSize int32 = 500
	maxDPPProductIDSearchItems       = 100
	maxDPPRequestBodyBytes     int64 = 10 << 20
)

// DPPRepositoryService persists and retrieves Digital Product Passport documents.
type DPPRepositoryService struct {
	aasRepo      *aasrepositorydb.AssetAdministrationShellDatabase
	submodelRepo *submodelrepositorydb.SubmodelDatabase
}

// NewDPPRepositoryService creates a DPP repository service backed by AAS and submodel repositories.
func NewDPPRepositoryService(aasRepo *aasrepositorydb.AssetAdministrationShellDatabase, submodelRepo *submodelrepositorydb.SubmodelDatabase) *DPPRepositoryService {
	return &DPPRepositoryService{aasRepo: aasRepo, submodelRepo: submodelRepo}
}

// CreateDPPFromJSON creates a DPP from a compressed JSON document.
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
func (s *DPPRepositoryService) ReadDPPById(ctx context.Context, dppID string, representation Representation) (ImplResponse, error) {
	doc, err := s.composeDPP(ctx, dppID, normalizeRepresentation(representation), time.Time{})
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	return Response(http.StatusOK, doc), nil
}

// DeleteDPPById deletes a DPP and its currently referenced submodels.
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
func (s *DPPRepositoryService) ReadDPPByProductId(ctx context.Context, productID string, representation Representation) (ImplResponse, error) {
	shells, _, err := s.aasRepo.GetAssetAdministrationShells(ctx, 2, "", "", []string{productID})
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	if len(shells) == 0 {
		return errorResponse(http.StatusNotFound, fmt.Errorf("DPP-READBYPRODUCT-NOTFOUND no DPP for product %s", productID)), nil
	}
	if len(shells) > 1 {
		return errorResponse(http.StatusConflict, fmt.Errorf("DPP-READBYPRODUCT-AMBIGUOUS multiple DPPs for product %s", productID)), nil
	}
	doc, err := s.composeDPP(ctx, shells[0].ID(), normalizeRepresentation(representation), time.Time{})
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	return Response(http.StatusOK, doc), nil
}

// ReadDPPVersionByIdAndDate reads a historic DPP version at the requested timestamp.
func (s *DPPRepositoryService) ReadDPPVersionByIdAndDate(ctx context.Context, dppID string, date time.Time, representation Representation) (ImplResponse, error) {
	doc, err := s.composeDPP(ctx, dppID, normalizeRepresentation(representation), date)
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	return Response(http.StatusOK, doc), nil
}

// ReadDPPIdsByProductIds resolves product IDs to sorted, paged DPP IDs.
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

// ReadDataElement reads one DPP data element by content section and idShort path.
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
func (s *DPPRepositoryService) UpdateDataElement(ctx context.Context, dppID string, elementPath string, dataElement DataElement) (ImplResponse, error) {
	raw, err := json.Marshal(dataElement)
	if err != nil {
		return errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-UPDELEM-MARSHAL marshal generated data element: %w", err)), nil
	}
	return s.UpdateDataElementFromJSON(ctx, dppID, elementPath, raw)
}

// CreateDPP creates a DPP from the generated OpenAPI model.
func (s *DPPRepositoryService) CreateDPP(ctx context.Context, passport DigitalProductPassport) (ImplResponse, error) {
	raw, err := json.Marshal(passport)
	if err != nil {
		return errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-CREATEDPP-MARSHAL marshal generated DPP: %w", err)), nil
	}
	return s.CreateDPPFromJSON(ctx, raw)
}

// UpdateDPPById applies a generated model patch to an existing DPP.
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

	sectionNames := sortedKeys(sections)
	for _, sectionName := range sectionNames {
		semanticID, err := semanticIDForSection(sectionName, header.ContentSpecificationIDs)
		if err != nil {
			return nil, nil, err
		}
		submodel, err := buildContentSubmodel(header.DigitalProductPassportID, sectionName, semanticID, sections[sectionName])
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
		jsonable, err := jsonization.ToJsonable(element)
		if err != nil {
			return nil, fmt.Errorf("DPP-ELEM-FULL convert element normal serialization: %w", err)
		}
		return aasNormalToDPPExpanded(jsonable), nil
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

func errorResponse(status int, err error) ImplResponse {
	return Response(status, Result{Messages: []Message{{
		MessageType:   "Error",
		Text:          err.Error(),
		Code:          firstErrorCode(err.Error()),
		CorrelationId: "",
		Timestamp:     time.Now().UTC(),
	}}})
}

func mapPersistenceError(err error, fallbackStatus int) ImplResponse {
	status := fallbackStatus
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "not found") || strings.Contains(text, "no rows") {
		status = http.StatusNotFound
	}
	if strings.Contains(text, "duplicate") || strings.Contains(text, "already") {
		status = http.StatusConflict
	}
	return errorResponse(status, err)
}

func firstErrorCode(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "DPP-ERROR-UNKNOWN"
	}
	if strings.HasPrefix(fields[0], "DPP-") {
		return fields[0]
	}
	return "DPP-ERROR-UNKNOWN"
}

// DPPRepositoryRouter exposes DPP repository service operations as HTTP handlers.
type DPPRepositoryRouter struct {
	service *DPPRepositoryService
}

// NewDPPRepositoryRouter creates an HTTP router adapter for the DPP repository service.
func NewDPPRepositoryRouter(service *DPPRepositoryService) *DPPRepositoryRouter {
	return &DPPRepositoryRouter{service: service}
}

// OrderedRoutes returns DPP routes in registration order.
func (r *DPPRepositoryRouter) OrderedRoutes() []Route {
	return []Route{
		{"ReadDPPById", http.MethodGet, "/v1/dpps/{dppId}", r.ReadDPPById},
		{"DeleteDPPById", http.MethodDelete, "/v1/dpps/{dppId}", r.DeleteDPPById},
		{"UpdateDPPById", http.MethodPatch, "/v1/dpps/{dppId}", r.UpdateDPPById},
		{"CreateDPP", http.MethodPost, "/v1/dpps", r.CreateDPP},
		{"ReadDPPByProductId", http.MethodGet, "/v1/dppsByProductId/{productId}", r.ReadDPPByProductId},
		{"ReadDPPVersionByIdAndDate", http.MethodGet, "/v1/dppsByIdAndDate/{dppId}", r.ReadDPPVersionByIdAndDate},
		{"ReadDPPIdsByProductIds", http.MethodPost, "/v1/dppsByProductIds", r.ReadDPPIdsByProductIds},
		{"ReadDataElement", http.MethodGet, "/v1/dpps/{dppId}/elements/*", r.ReadDataElement},
		{"UpdateDataElement", http.MethodPut, "/v1/dpps/{dppId}/elements/*", r.UpdateDataElement},
	}
}

// Routes returns DPP routes keyed by operation name.
func (r *DPPRepositoryRouter) Routes() Routes {
	routes := make(Routes)
	for _, route := range r.OrderedRoutes() {
		routes[route.Name] = route
	}
	return routes
}

// ReadDPPById handles GET /v1/dpps/{dppId}.
func (r *DPPRepositoryRouter) ReadDPPById(w http.ResponseWriter, req *http.Request) {
	representation, invalid := queryRepresentation(req)
	if invalid != nil {
		r.write(w, *invalid, nil)
		return
	}
	response, err := r.service.ReadDPPById(req.Context(), pathParam(req, "dppId"), representation)
	r.write(w, response, err)
}

// DeleteDPPById handles DELETE /v1/dpps/{dppId}.
func (r *DPPRepositoryRouter) DeleteDPPById(w http.ResponseWriter, req *http.Request) {
	response, err := r.service.DeleteDPPById(req.Context(), pathParam(req, "dppId"))
	r.write(w, response, err)
}

// UpdateDPPById handles PATCH /v1/dpps/{dppId}.
func (r *DPPRepositoryRouter) UpdateDPPById(w http.ResponseWriter, req *http.Request) {
	body, err := readRequestBody(w, req)
	if err != nil {
		r.write(w, requestBodyErrorResponse("UPDDPP", err), nil)
		return
	}
	response, err := r.service.UpdateDPPFromJSON(req.Context(), pathParam(req, "dppId"), body)
	r.write(w, response, err)
}

// CreateDPP handles POST /v1/dpps.
func (r *DPPRepositoryRouter) CreateDPP(w http.ResponseWriter, req *http.Request) {
	body, err := readRequestBody(w, req)
	if err != nil {
		r.write(w, requestBodyErrorResponse("CREATEDPP", err), nil)
		return
	}
	response, err := r.service.CreateDPPFromJSON(req.Context(), body)
	r.write(w, response, err)
}

// ReadDPPByProductId handles GET /v1/dppsByProductId/{productId}.
func (r *DPPRepositoryRouter) ReadDPPByProductId(w http.ResponseWriter, req *http.Request) {
	representation, invalid := queryRepresentation(req)
	if invalid != nil {
		r.write(w, *invalid, nil)
		return
	}
	response, err := r.service.ReadDPPByProductId(req.Context(), pathParam(req, "productId"), representation)
	r.write(w, response, err)
}

// ReadDPPVersionByIdAndDate handles GET /v1/dppsByIdAndDate/{dppId}.
func (r *DPPRepositoryRouter) ReadDPPVersionByIdAndDate(w http.ResponseWriter, req *http.Request) {
	date, err := time.Parse(time.RFC3339Nano, req.URL.Query().Get("date"))
	if err != nil {
		r.write(w, errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-READDATE-PARSE parse date query parameter: %w", err)), nil)
		return
	}
	representation, invalid := queryRepresentation(req)
	if invalid != nil {
		r.write(w, *invalid, nil)
		return
	}
	response, err := r.service.ReadDPPVersionByIdAndDate(req.Context(), pathParam(req, "dppId"), date, representation)
	r.write(w, response, err)
}

// ReadDPPIdsByProductIds handles POST /v1/dppsByProductIds.
func (r *DPPRepositoryRouter) ReadDPPIdsByProductIds(w http.ResponseWriter, req *http.Request) {
	var request ReadDppIdsByProductIdsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxDPPRequestBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		r.write(w, requestBodyDecodeErrorResponse("READIDS", err), nil)
		return
	}
	if err := validateReadDPPIdsRequest(request); err != nil {
		r.write(w, errorResponse(http.StatusBadRequest, err), nil)
		return
	}
	limit := defaultDPPPageLimit
	if rawLimit := req.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.ParseInt(rawLimit, 10, 32)
		if err != nil || parsed < 1 {
			r.write(w, errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-READIDS-LIMIT invalid limit")), nil)
			return
		}
		limit = int32(parsed)
	}
	response, err := r.service.ReadDPPIdsByProductIds(req.Context(), request, limit, req.URL.Query().Get("cursor"))
	r.write(w, response, err)
}

func validateReadDPPIdsRequest(request ReadDppIdsByProductIdsRequest) error {
	if len(request.ProductIds) == 0 {
		return fmt.Errorf("DPP-READIDS-MISSING productIds must contain at least one product id")
	}
	if len(request.ProductIds) > maxDPPProductIDSearchItems {
		return fmt.Errorf("DPP-READIDS-MAXITEMS productIds must contain at most %d product ids", maxDPPProductIDSearchItems)
	}
	for _, productID := range request.ProductIds {
		if strings.TrimSpace(productID) == "" {
			return fmt.Errorf("DPP-READIDS-INVALID productIds must contain only non-empty strings")
		}
	}
	return nil
}

// ReadDataElement handles GET /v1/dpps/{dppId}/elements/{elementPath}.
func (r *DPPRepositoryRouter) ReadDataElement(w http.ResponseWriter, req *http.Request) {
	representation, invalid := queryRepresentation(req)
	if invalid != nil {
		r.write(w, *invalid, nil)
		return
	}
	response, err := r.service.ReadDataElement(req.Context(), pathParam(req, "dppId"), elementPathParam(req), representation)
	r.write(w, response, err)
}

// UpdateDataElement handles PUT /v1/dpps/{dppId}/elements/{elementPath}.
func (r *DPPRepositoryRouter) UpdateDataElement(w http.ResponseWriter, req *http.Request) {
	body, err := readRequestBody(w, req)
	if err != nil {
		r.write(w, requestBodyErrorResponse("UPDELEM", err), nil)
		return
	}
	response, err := r.service.UpdateDataElementFromJSON(req.Context(), pathParam(req, "dppId"), elementPathParam(req), body)
	r.write(w, response, err)
}

func readRequestBody(w http.ResponseWriter, req *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(w, req.Body, maxDPPRequestBodyBytes))
}

func requestBodyErrorResponse(operation string, err error) ImplResponse {
	if isRequestBodyTooLarge(err) {
		return errorResponse(http.StatusRequestEntityTooLarge, fmt.Errorf("DPP-%s-BODYTOOLARGE request body exceeds %d bytes", operation, maxDPPRequestBodyBytes))
	}
	return errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-%s-READBODY read request body: %w", operation, err))
}

func requestBodyDecodeErrorResponse(operation string, err error) ImplResponse {
	if isRequestBodyTooLarge(err) {
		return errorResponse(http.StatusRequestEntityTooLarge, fmt.Errorf("DPP-%s-BODYTOOLARGE request body exceeds %d bytes", operation, maxDPPRequestBodyBytes))
	}
	return errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-%s-DECODE decode request body: %w", operation, err))
}

func isRequestBodyTooLarge(err error) bool {
	return strings.Contains(err.Error(), "request body too large")
}

func pathParam(req *http.Request, name string) string {
	return decodePathParam(chi.URLParam(req, name))
}

func elementPathParam(req *http.Request) string {
	return strings.TrimPrefix(decodePathParam(chi.URLParam(req, "*")), "/")
}

func decodePathParam(value string) string {
	decoded := value
	for range 3 {
		next, err := url.PathUnescape(decoded)
		if err != nil || next == decoded {
			return decoded
		}
		decoded = next
	}
	return decoded
}

func (r *DPPRepositoryRouter) write(w http.ResponseWriter, response ImplResponse, err error) {
	if err != nil {
		response = errorResponse(http.StatusInternalServerError, err)
	}
	_ = EncodeJSONResponse(response.Body, &response.Code, w)
}

func queryRepresentation(req *http.Request) (Representation, *ImplResponse) {
	representation := Representation(req.URL.Query().Get("representation"))
	if representation == "" {
		return REPRESENTATION_COMPRESSED, nil
	}
	if !representation.IsValid() {
		response := errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-REPRESENTATION-INVALID invalid representation %q", representation))
		return "", &response
	}
	return representation, nil
}
