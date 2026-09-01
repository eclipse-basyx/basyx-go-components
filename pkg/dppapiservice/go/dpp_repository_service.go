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
	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment"
	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	smregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/persistence"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
)

const (
	defaultDPPPageLimit        int32 = 100
	maxDPPProductIDSearchItems       = 100
	maxDPPRequestBodyBytes     int64 = 10 << 20
)

// DPPRepositoryService persists and retrieves Digital Product Passport documents.
//
// Fields:
//   - aasRepo: Persistence repository for Asset Administration Shell records
//   - submodelRepo: Persistence repository for DPP metadata and content submodels
type DPPRepositoryService struct {
	aasRepo            *aasrepositorydb.AssetAdministrationShellDatabase
	submodelRepo       *submodelrepositorydb.SubmodelDatabase
	aasRegistry        *aasregistrydb.PostgreSQLAASRegistryDatabase
	submodelRegistry   *smregistrydb.PostgreSQLSMDatabase
	registrySyncConfig aasenvironment.RegistrySyncConfig
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

// NewDPPRepositoryServiceWithRegistrySync creates a DPP repository service with atomic registry synchronization.
func NewDPPRepositoryServiceWithRegistrySync(
	aasRepo *aasrepositorydb.AssetAdministrationShellDatabase,
	submodelRepo *submodelrepositorydb.SubmodelDatabase,
	aasRegistry *aasregistrydb.PostgreSQLAASRegistryDatabase,
	submodelRegistry *smregistrydb.PostgreSQLSMDatabase,
	registrySyncConfig aasenvironment.RegistrySyncConfig,
) (*DPPRepositoryService, error) {
	if registrySyncConfig.AASRegistryIntegration && aasRegistry == nil {
		return nil, common.NewInternalServerError("DPP-REGSYNC-NILAASREGISTRY AAS registry backend must not be nil")
	}
	if registrySyncConfig.SubmodelRegistryIntegration && submodelRegistry == nil {
		return nil, common.NewInternalServerError("DPP-REGSYNC-NILSMREGISTRY Submodel registry backend must not be nil")
	}
	return &DPPRepositoryService{
		aasRepo:            aasRepo,
		submodelRepo:       submodelRepo,
		aasRegistry:        aasRegistry,
		submodelRegistry:   submodelRegistry,
		registrySyncConfig: registrySyncConfig,
	}, nil
}

func (s *DPPRepositoryService) syncCreatedDescriptors(ctx context.Context, tx *sql.Tx, aas types.IAssetAdministrationShell, submodels []types.ISubmodel) error {
	if s.registrySyncConfig.SubmodelRegistryIntegration {
		for _, submodel := range submodels {
			descriptor, err := s.registrySyncConfig.BuildSubmodelDescriptor(submodel)
			if err != nil {
				return fmt.Errorf("DPP-REGSYNC-CREATE-BUILDSMDESC build submodel descriptor %s: %w", submodel.ID(), err)
			}
			if _, err = s.submodelRegistry.InsertSubmodelDescriptorInTransaction(
				aasenvironment.WithSubmodelRegistrySyncUpsertAudit(ctx), tx, descriptor,
			); err != nil {
				return fmt.Errorf("DPP-REGSYNC-CREATE-INSERTSMDESC insert submodel descriptor %s: %w", submodel.ID(), err)
			}
		}
	}
	if !s.registrySyncConfig.AASRegistryIntegration {
		return nil
	}
	descriptor, err := s.registrySyncConfig.BuildAASDescriptor(aas)
	if err != nil {
		return fmt.Errorf("DPP-REGSYNC-CREATE-BUILDAASDESC build AAS descriptor: %w", err)
	}
	if err = s.aasRegistry.InsertAdministrationShellDescriptorInTransaction(
		aasenvironment.WithAASRegistrySyncUpsertAudit(ctx), tx, descriptor,
	); err != nil {
		return fmt.Errorf("DPP-REGSYNC-CREATE-INSERTAASDESC insert AAS descriptor: %w", err)
	}
	return nil
}

func (s *DPPRepositoryService) syncUpdatedDescriptors(
	ctx context.Context,
	tx *sql.Tx,
	aas types.IAssetAdministrationShell,
	submodels []types.ISubmodel,
	staleSubmodelIDs []string,
) error {
	if s.registrySyncConfig.SubmodelRegistryIntegration {
		if err := s.upsertSubmodelDescriptors(ctx, tx, submodels); err != nil {
			return err
		}
		for _, submodelID := range staleSubmodelIDs {
			err := s.submodelRegistry.DeleteSubmodelDescriptorByIDInTransaction(
				aasenvironment.WithSubmodelRegistrySyncDeleteAudit(ctx), tx, submodelID,
			)
			if err != nil && !common.IsErrNotFound(err) {
				return fmt.Errorf("DPP-REGSYNC-UPDATE-DELETESMDESC delete stale submodel descriptor %s: %w", submodelID, err)
			}
		}
	}
	if !s.registrySyncConfig.AASRegistryIntegration {
		return nil
	}
	descriptor, err := s.registrySyncConfig.BuildAASDescriptor(aas)
	if err != nil {
		return fmt.Errorf("DPP-REGSYNC-UPDATE-BUILDAASDESC build AAS descriptor: %w", err)
	}
	if err = s.aasRegistry.UpsertAdministrationShellDescriptorInTransaction(
		aasenvironment.WithAASRegistrySyncUpsertAudit(ctx), tx, descriptor,
	); err != nil {
		return fmt.Errorf("DPP-REGSYNC-UPDATE-UPSERTAASDESC upsert AAS descriptor: %w", err)
	}
	return nil
}

func (s *DPPRepositoryService) upsertSubmodelDescriptors(ctx context.Context, tx *sql.Tx, submodels []types.ISubmodel) error {
	for _, submodel := range submodels {
		descriptor, err := s.registrySyncConfig.BuildSubmodelDescriptor(submodel)
		if err != nil {
			return fmt.Errorf("DPP-REGSYNC-UPDATE-BUILDSMDESC build submodel descriptor %s: %w", submodel.ID(), err)
		}
		if err = s.submodelRegistry.UpsertSubmodelDescriptorInTransaction(
			aasenvironment.WithSubmodelRegistrySyncUpsertAudit(ctx), tx, descriptor,
		); err != nil {
			return fmt.Errorf("DPP-REGSYNC-UPDATE-UPSERTSMDESC upsert submodel descriptor %s: %w", submodel.ID(), err)
		}
	}
	return nil
}

func (s *DPPRepositoryService) deleteDescriptors(ctx context.Context, tx *sql.Tx, dppID string, submodels []types.ISubmodel) error {
	if s.registrySyncConfig.SubmodelRegistryIntegration {
		for _, submodel := range submodels {
			err := s.submodelRegistry.DeleteSubmodelDescriptorByIDInTransaction(
				aasenvironment.WithSubmodelRegistrySyncDeleteAudit(ctx), tx, submodel.ID(),
			)
			if err != nil && !common.IsErrNotFound(err) {
				return fmt.Errorf("DPP-REGSYNC-DELETE-DELETESMDESC delete submodel descriptor %s: %w", submodel.ID(), err)
			}
		}
	}
	if !s.registrySyncConfig.AASRegistryIntegration {
		return nil
	}
	err := s.aasRegistry.DeleteAssetAdministrationShellDescriptorByIDInTransaction(
		aasenvironment.WithAASRegistrySyncDeleteAudit(ctx), tx, dppID,
	)
	if err != nil && !common.IsErrNotFound(err) {
		return fmt.Errorf("DPP-REGSYNC-DELETE-DELETEAASDESC delete AAS descriptor: %w", err)
	}
	return nil
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
	submodels, refs, err := s.buildSubmodels(header, sections, metadataSubmodelID(header.DigitalProductPassportID))
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
		return s.syncCreatedDescriptors(ctx, tx, aas, submodels)
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
	ctx = common.WithWriterPostgresReads(ctx)
	patch, _, err := decodeDPPDocument(data, false)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}
	currentResolved, err := s.resolveSubmodels(ctx, dppID, time.Time{})
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	currentContentSubmodels, err := selectedResolvedContentSubmodels(currentResolved)
	if err != nil {
		return mapPersistenceError(err, http.StatusInternalServerError), nil
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
	updatedAt, err := nextDPPUpdateTimestamp(time.Now().UTC(), current, currentContentSubmodels)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, err), nil
	}
	merged[headerDigitalProductPassportID] = dppID
	merged[headerLastUpdate] = updatedAt.Format(time.RFC3339Nano)

	raw, err := json.Marshal(merged)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, fmt.Errorf("DPP-UPDDPP-MARSHAL marshal merged DPP: %w", err)), nil
	}
	_, header, err := decodeDPPDocument(raw, true)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}

	sections := contentSections(merged)
	submodels, refs, err := s.buildSubmodels(header, sections, currentResolved.metadata.ID())
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}
	applyDPPUpdateAdministration(submodels, currentResolved.metadata, currentContentSubmodels, header.LastUpdate)
	if err = s.preserveManagedAttachments(ctx, submodels, currentContentSubmodels); err != nil {
		return errorResponse(http.StatusInternalServerError, err), nil
	}
	refs = appendUnselectedContentSubmodelReferences(refs, currentResolved, currentContentSubmodels)
	aas := buildAAS(header, refs)
	staleSubmodelIDs := staleContentSubmodelIDs(currentContentSubmodels, submodels)

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
		return s.syncUpdatedDescriptors(ctx, tx, aas, submodels, staleSubmodelIDs)
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

func staleContentSubmodelIDs(currentContent []types.ISubmodel, replacement []types.ISubmodel) []string {
	replacementIDs := make(map[string]struct{}, len(replacement))
	for _, submodel := range replacement {
		replacementIDs[submodel.ID()] = struct{}{}
	}
	stale := make([]string, 0)
	for _, submodel := range currentContent {
		if _, stillPresent := replacementIDs[submodel.ID()]; !stillPresent {
			stale = append(stale, submodel.ID())
		}
	}
	sort.Strings(stale)
	return stale
}

func appendUnselectedContentSubmodelReferences(refs []types.IReference, resolved resolvedDPP, selectedContent []types.ISubmodel) []types.IReference {
	includedIDs := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		includedIDs[referenceLastValue(ref)] = struct{}{}
	}
	selectedIDs := map[string]struct{}{resolved.metadata.ID(): {}}
	for _, submodel := range selectedContent {
		selectedIDs[submodel.ID()] = struct{}{}
	}
	for _, submodel := range resolved.submodels {
		if _, selected := selectedIDs[submodel.ID()]; selected {
			continue
		}
		if _, included := includedIDs[submodel.ID()]; included {
			continue
		}
		refs = append(refs, submodelReference(submodel.ID()))
		includedIDs[submodel.ID()] = struct{}{}
	}
	return refs
}

func applyDPPUpdateAdministration(replacements []types.ISubmodel, currentMetadata types.ISubmodel, currentContent []types.ISubmodel, timestamp time.Time) {
	currentByID, currentBySemanticID := indexCurrentDPPSubmodels(currentMetadata, currentContent)

	for _, replacement := range replacements {
		current := matchingCurrentDPPSubmodel(replacement, currentByID, currentBySemanticID)
		replacement.SetAdministration(updatedDPPAdministration(current, timestamp))
	}
}

func indexCurrentDPPSubmodels(
	metadata types.ISubmodel,
	content []types.ISubmodel,
) (map[string]types.ISubmodel, map[string]types.ISubmodel) {
	currentByID := make(map[string]types.ISubmodel, len(content)+1)
	if metadata != nil {
		currentByID[metadata.ID()] = metadata
	}
	currentBySemanticID := make(map[string]types.ISubmodel, len(content))
	for _, submodel := range content {
		currentByID[submodel.ID()] = submodel
		if semanticID := referenceToString(submodel.SemanticID()); semanticID != "" {
			currentBySemanticID[semanticID] = submodel
		}
	}
	return currentByID, currentBySemanticID
}

func matchingCurrentDPPSubmodel(
	replacement types.ISubmodel,
	currentByID map[string]types.ISubmodel,
	currentBySemanticID map[string]types.ISubmodel,
) types.ISubmodel {
	if current := currentByID[replacement.ID()]; current != nil {
		return current
	}
	return currentBySemanticID[referenceToString(replacement.SemanticID())]
}

func (s *DPPRepositoryService) preserveManagedAttachments(
	ctx context.Context,
	replacements []types.ISubmodel,
	currentContent []types.ISubmodel,
) error {
	currentByID, currentBySemanticID := indexCurrentDPPSubmodels(nil, currentContent)
	for _, replacement := range replacements {
		current := matchingCurrentDPPSubmodel(replacement, currentByID, currentBySemanticID)
		if current == nil {
			continue
		}
		serializationContext, err := s.serializationContext(ctx, current.ID(), false)
		if err != nil {
			return fmt.Errorf("DPP-UPDDPP-PRESERVEFILES-LOADPATHS load managed attachment paths: %w", err)
		}
		preserveManagedFileValues(current, replacement, serializationContext)
	}
	return nil
}

func preserveManagedFileValues(current types.ISubmodel, replacement types.ISubmodel, serializationContext dppSerializationContext) {
	currentFiles := make(map[string]*types.File)
	replacementFiles := make(map[string]*types.File)
	collectFilesByPath(current.SubmodelElements(), "", currentFiles)
	collectFilesByPath(replacement.SubmodelElements(), "", replacementFiles)
	preserveManagedFiles(currentFiles, replacementFiles, serializationContext)
}

func preserveManagedFiles(
	currentFiles map[string]*types.File,
	replacementFiles map[string]*types.File,
	serializationContext dppSerializationContext,
) {
	for path := range serializationContext.managedAttachmentPaths {
		currentFile := currentFiles[path]
		replacementFile := replacementFiles[path]
		if currentFile == nil || replacementFile == nil {
			continue
		}
		expectedURL, err := relatedResourceURL(currentFile, path, serializationContext)
		if err == nil && dereferenceString(replacementFile.Value()) == expectedURL {
			replacementFile.SetValue(currentFile.Value())
		}
	}
}

func collectFilesByPath(elements []types.ISubmodelElement, parentPath string, files map[string]*types.File) {
	for _, element := range elements {
		path := nestedIDShortPath(parentPath, idShortValue(element))
		collectFileByPath(element, path, files)
	}
}

func collectFileByPath(element types.ISubmodelElement, path string, files map[string]*types.File) {
	switch typed := element.(type) {
	case *types.File:
		files[path] = typed
	case *types.SubmodelElementCollection:
		collectFilesByPath(typed.Value(), path, files)
	case *types.SubmodelElementList:
		for index, child := range typed.Value() {
			collectFileByPath(child, fmt.Sprintf("%s[%d]", path, index), files)
		}
	}
}

func updatedDPPAdministration(current types.ISubmodel, timestamp time.Time) types.IAdministrativeInformation {
	administration := types.NewAdministrativeInformation()
	if current != nil && current.Administration() != nil {
		existing := current.Administration()
		administration.SetEmbeddedDataSpecifications(existing.EmbeddedDataSpecifications())
		administration.SetVersion(existing.Version())
		administration.SetRevision(existing.Revision())
		administration.SetCreator(existing.Creator())
		administration.SetCreatedAt(existing.CreatedAt())
		administration.SetTemplateID(existing.TemplateID())
	}

	formatted := timestamp.UTC().Format(time.RFC3339Nano)
	if administration.CreatedAt() == nil || strings.TrimSpace(*administration.CreatedAt()) == "" {
		administration.SetCreatedAt(&formatted)
	}
	administration.SetUpdatedAt(&formatted)
	return administration
}

func nextDPPUpdateTimestamp(now time.Time, current dppDocument, currentContent []types.ISubmodel) (time.Time, error) {
	latest := now.UTC()
	if rawLastUpdate, ok := current[headerLastUpdate].(string); ok && strings.TrimSpace(rawLastUpdate) != "" {
		lastUpdate, err := common.ParseISO8601DateTime(rawLastUpdate)
		if err != nil {
			return time.Time{}, fmt.Errorf("DPP-UPDDPP-LASTUPDATE parse current lastUpdate: %w", err)
		}
		latest = timestampAfter(latest, lastUpdate)
	}
	for _, submodel := range currentContent {
		contentTimestamp, err := dppContentSubmodelTimestamp(submodel)
		if err != nil {
			return time.Time{}, err
		}
		latest = timestampAfter(latest, contentTimestamp)
	}
	return latest, nil
}

func timestampAfter(candidate time.Time, current time.Time) time.Time {
	if candidate.After(current) {
		return candidate
	}
	return current.Add(time.Nanosecond)
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
	ctx = common.WithWriterPostgresReads(ctx)
	resolved, err := s.resolveSubmodels(ctx, dppID, time.Time{})
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}

	err = s.aasRepo.ExecuteInTransaction("DPP-DELDPP-STARTTX", "DPP-DELDPP-COMMITTX", func(tx *sql.Tx) error {
		if err := s.deleteDescriptors(ctx, tx, dppID, resolved.submodels); err != nil {
			return err
		}
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
	ids, _, err := s.aasRepo.GetAssetAdministrationShellIDsByAssetAndSubmodelSemanticIDs(
		ctx,
		[]string{productID},
		dppMetadataSemanticIDValues(),
		2,
		"",
	)
	if err != nil {
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
	ids, nextCursor, err := s.aasRepo.GetAssetAdministrationShellIDsByAssetAndSubmodelSemanticIDs(
		ctx,
		request.ProductIds,
		dppMetadataSemanticIDValues(),
		limitOrDefault(limit),
		cursor,
	)
	if err != nil {
		return mapPersistenceError(err, http.StatusInternalServerError), nil
	}
	return Response(http.StatusOK, DppidSearchResult{Items: ids, Cursor: nextCursor}), nil
}

// ReadDataElement reads one DPP data element by elementIdPath.
//
// Parameters:
//   - ctx: Request context used for repository read calls
//   - dppID: Identifier of the DPP that owns the element
//   - elementIDPath: RFC 9535 Normalized Path selecting a single DPP data element
//   - representation: Requested compressed or full element representation
//
// Returns:
//   - ImplResponse: HTTP-style response containing the DPP data element or mapped error payload
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) ReadDataElement(ctx context.Context, dppID string, elementIDPath string, representation Representation) (ImplResponse, error) {
	submodelID, idShortPath, _, err := s.resolveElementPath(ctx, dppID, elementIDPath)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}
	element, err := s.submodelRepo.GetSubmodelElement(ctx, submodelID, idShortPath, true, "deep")
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	serializationContext, err := s.serializationContext(ctx, submodelID, false)
	if err != nil {
		return mapPersistenceError(err, http.StatusInternalServerError), nil
	}
	body, err := elementResponseWithContext(
		element, normalizeRepresentation(representation), elementIDPath, idShortPath, serializationContext,
	)
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
//   - elementIDPath: RFC 9535 Normalized Path selecting a single DPP data element
//   - data: Compressed JSON value used as the replacement element content
//
// Returns:
//   - ImplResponse: HTTP-style response containing the updated DPP data element or mapped error payload
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) UpdateDataElementFromJSON(ctx context.Context, dppID string, elementIDPath string, data []byte) (ImplResponse, error) {
	ctx = common.WithWriterPostgresReads(ctx)
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-UPDELEM-DECODE decode element body: %w", err)), nil
	}
	if err := rejectExpandedDataElementShape(elementIDPath, value); err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}
	submodelID, idShortPath, metadata, err := s.resolveElementPath(ctx, dppID, elementIDPath)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}
	existing, err := s.submodelRepo.GetSubmodelElement(ctx, submodelID, idShortPath, true, "deep")
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	idShortParts := strings.Split(idShortPath, ".")
	element, err := inferElement(idShortParts[len(idShortParts)-1], value)
	if err != nil {
		return errorResponse(http.StatusBadRequest, err), nil
	}
	preserveElementMetadata(existing, element)
	if err = s.preserveManagedElementAttachments(ctx, submodelID, idShortPath, existing, element); err != nil {
		return errorResponse(http.StatusInternalServerError, err), nil
	}
	metadata, err = updatedMetadata(metadata)
	if err != nil {
		return mapPersistenceError(err, http.StatusInternalServerError), nil
	}
	aas, descriptorSubmodels, err := s.elementUpdateSyncResources(ctx, dppID, submodelID, metadata)
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
		return s.syncUpdatedDescriptors(ctx, tx, aas, descriptorSubmodels, nil)
	})
	if err != nil {
		return mapPersistenceError(err, http.StatusConflict), nil
	}

	return s.ReadDataElement(ctx, dppID, elementIDPath, REPRESENTATION_COMPRESSED)
}

func (s *DPPRepositoryService) preserveManagedElementAttachments(
	ctx context.Context,
	submodelID string,
	idShortPath string,
	current types.ISubmodelElement,
	replacement types.ISubmodelElement,
) error {
	currentFiles := make(map[string]*types.File)
	replacementFiles := make(map[string]*types.File)
	collectFileByPath(current, idShortPath, currentFiles)
	collectFileByPath(replacement, idShortPath, replacementFiles)
	if len(currentFiles) == 0 || len(replacementFiles) == 0 {
		return nil
	}
	serializationContext, err := s.serializationContext(ctx, submodelID, false)
	if err != nil {
		return fmt.Errorf("DPP-UPDELEM-PRESERVEFILES-LOADPATHS load managed attachment paths: %w", err)
	}
	preserveManagedFiles(currentFiles, replacementFiles, serializationContext)
	return nil
}

func (s *DPPRepositoryService) elementUpdateSyncResources(
	ctx context.Context,
	dppID string,
	contentSubmodelID string,
	metadata types.ISubmodel,
) (types.IAssetAdministrationShell, []types.ISubmodel, error) {
	var aas types.IAssetAdministrationShell
	var err error
	if s.registrySyncConfig.AASRegistryIntegration {
		aas, err = s.aasRepo.GetAssetAdministrationShellByID(ctx, dppID)
		if err != nil {
			return nil, nil, fmt.Errorf("DPP-UPDELEM-GETAAS get AAS for descriptor synchronization: %w", err)
		}
	}
	if !s.registrySyncConfig.SubmodelRegistryIntegration {
		return aas, nil, nil
	}
	submodels := []types.ISubmodel{metadata}
	if contentSubmodelID == metadata.ID() {
		return aas, submodels, nil
	}
	contentSubmodel, err := s.submodelRepo.GetSubmodelByID(ctx, contentSubmodelID, "deep", false, false)
	if err != nil {
		return nil, nil, fmt.Errorf("DPP-UPDELEM-GETSUBMODEL get content submodel for descriptor synchronization: %w", err)
	}
	return aas, append(submodels, contentSubmodel), nil
}

func preserveElementMetadata(existing types.ISubmodelElement, replacement types.ISubmodelElement) {
	replacement.SetIDShort(existing.IDShort())
	replacement.SetExtensions(mergeElementExtensions(existing.Extensions(), replacement.Extensions()))
	replacement.SetCategory(existing.Category())
	replacement.SetDisplayName(existing.DisplayName())
	replacement.SetDescription(existing.Description())
	replacement.SetSemanticID(existing.SemanticID())
	replacement.SetSupplementalSemanticIDs(existing.SupplementalSemanticIDs())
	replacement.SetQualifiers(existing.Qualifiers())
	replacement.SetEmbeddedDataSpecifications(existing.EmbeddedDataSpecifications())
}

func mergeElementExtensions(existing []types.IExtension, replacement []types.IExtension) []types.IExtension {
	if len(existing) == 0 {
		return replacement
	}
	if len(replacement) == 0 {
		return existing
	}
	merged := make([]types.IExtension, 0, len(existing)+len(replacement))
	replacementByName := make(map[string]types.IExtension, len(replacement))
	for _, extension := range replacement {
		replacementByName[extension.Name()] = extension
	}
	for _, extension := range existing {
		if replacementExtension, ok := replacementByName[extension.Name()]; ok {
			merged = append(merged, replacementExtension)
			delete(replacementByName, extension.Name())
			continue
		}
		merged = append(merged, extension)
	}
	for _, extension := range replacement {
		if _, ok := replacementByName[extension.Name()]; ok {
			merged = append(merged, extension)
		}
	}
	return merged
}

// UpdateDataElement replaces one DPP data element from a generated model value.
//
// Parameters:
//   - ctx: Request context used for repository persistence calls
//   - dppID: Identifier of the DPP that owns the element
//   - elementIDPath: RFC 9535 Normalized Path selecting a single DPP data element
//   - dataElement: Generated DPP data element model used as replacement content
//
// Returns:
//   - ImplResponse: HTTP-style response containing the updated DPP data element or mapped error payload
//   - error: Unexpected service error, if one occurs outside normal response mapping
func (s *DPPRepositoryService) UpdateDataElement(ctx context.Context, dppID string, elementIDPath string, dataElement DataElement) (ImplResponse, error) {
	raw, err := json.Marshal(dataElement)
	if err != nil {
		return errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-UPDELEM-MARSHAL marshal generated data element: %w", err)), nil
	}
	return s.UpdateDataElementFromJSON(ctx, dppID, elementIDPath, raw)
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
	return composeResolvedDPPWithContext(resolved, representation, func(submodel types.ISubmodel) (dppSerializationContext, error) {
		return s.serializationContext(ctx, submodel.ID(), !at.IsZero())
	})
}

func composeResolvedDPP(resolved resolvedDPP, representation Representation) (dppDocument, error) {
	return composeResolvedDPPWithContext(resolved, representation, nil)
}

type dppSerializationContextLoader func(types.ISubmodel) (dppSerializationContext, error)

func composeResolvedDPPWithContext(
	resolved resolvedDPP,
	representation Representation,
	contextLoader dppSerializationContextLoader,
) (dppDocument, error) {
	doc, err := composeHeader(resolved.metadata)
	if err != nil {
		return nil, err
	}
	specificationSet, err := contentSpecificationSetFromHeader(doc)
	if err != nil {
		return nil, err
	}
	contentSubmodels, err := selectedContentSubmodelsForHeader(doc, resolved.metadata.ID(), resolved.submodels)
	if err != nil {
		return nil, err
	}
	if representation == REPRESENTATION_FULL {
		elements := make([]map[string]any, 0, len(contentSubmodels))
		for _, submodel := range contentSubmodels {
			serializationContext, err := loadDPPSerializationContext(contextLoader, submodel)
			if err != nil {
				return nil, err
			}
			content, err := fullContentWithContext(submodel, serializationContext)
			if err != nil {
				return nil, err
			}
			element, ok := content.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("DPP-COMPOSE-FULLTYPE full content for %s is not a DataElement object", submodel.ID())
			}
			elements = append(elements, element)
		}
		doc["elements"] = elements
		return doc, nil
	}
	for _, submodel := range contentSubmodels {
		serializationContext, err := loadDPPSerializationContext(contextLoader, submodel)
		if err != nil {
			return nil, err
		}
		sectionName := compressedContentSectionName(submodel, specificationSet)
		content, err := compressedContentWithContext(submodel, serializationContext)
		if err != nil {
			return nil, err
		}
		doc[sectionName] = content
	}
	return doc, nil
}

func loadDPPSerializationContext(loader dppSerializationContextLoader, submodel types.ISubmodel) (dppSerializationContext, error) {
	if loader == nil {
		return dppSerializationContext{}, nil
	}
	return loader(submodel)
}

func (s *DPPRepositoryService) serializationContext(
	ctx context.Context,
	submodelID string,
	historical bool,
) (dppSerializationContext, error) {
	managedPaths, err := s.submodelRepo.ManagedFileAttachmentPaths(ctx, submodelID)
	if err != nil {
		return dppSerializationContext{}, fmt.Errorf(
			"DPP-FILEURL-LOADMANAGEDPATHS load managed attachment paths for %s: %w", submodelID, err,
		)
	}
	return dppSerializationContext{
		submodelID:               submodelID,
		externalBaseURL:          common.ExternalBaseURLFromContext(ctx),
		managedAttachmentPaths:   managedPaths,
		managedPathHistoryLookup: historical,
	}, nil
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
			submodel, err = s.submodelRepo.GetSubmodelByID(ctx, submodelID, "deep", false, true)
		} else {
			submodel, err = s.submodelRepo.GetSubmodelByIDAndDate(ctx, submodelID, at)
		}
		if err != nil {
			return resolvedDPP{}, fmt.Errorf("DPP-RESOLVE-GETSUBMODEL get submodel %s: %w", submodelID, err)
		}
		if hasDPPMetadataSemanticID(submodel) {
			metadata = submodel
		}
		submodels = append(submodels, submodel)
	}
	if metadata == nil {
		return resolvedDPP{}, fmt.Errorf("DPP-RESOLVE-METADATA DppMetadata submodel not found for %s", dppID)
	}
	return resolvedDPP{metadata: metadata, submodels: submodels}, nil
}

func selectedResolvedContentSubmodels(resolved resolvedDPP) ([]types.ISubmodel, error) {
	header, err := composeHeader(resolved.metadata)
	if err != nil {
		return nil, err
	}
	return selectedContentSubmodelsForHeader(header, resolved.metadata.ID(), resolved.submodels)
}

func selectedContentSubmodelsForHeader(header dppDocument, metadataID string, submodels []types.ISubmodel) ([]types.ISubmodel, error) {
	specificationSet, err := contentSpecificationSetFromHeader(header)
	if err != nil {
		return nil, err
	}
	selectedBySemanticID := make(map[string]types.ISubmodel)
	selectedWithoutSemanticID := make([]types.ISubmodel, 0, len(submodels))
	for _, submodel := range submodels {
		if submodel.ID() == metadataID {
			continue
		}
		semanticID := referenceToString(submodel.SemanticID())
		if len(specificationSet) > 0 {
			if _, included := specificationSet[semanticID]; !included {
				continue
			}
		}
		if semanticID == "" {
			selectedWithoutSemanticID = append(selectedWithoutSemanticID, submodel)
			continue
		}
		current, found := selectedBySemanticID[semanticID]
		if !found {
			selectedBySemanticID[semanticID] = submodel
			continue
		}
		newer, err := isNewerDPPContentSubmodel(submodel, current)
		if err != nil {
			return nil, err
		}
		if newer {
			selectedBySemanticID[semanticID] = submodel
		}
	}
	selected := make([]types.ISubmodel, 0, len(selectedBySemanticID)+len(selectedWithoutSemanticID))
	for _, submodel := range selectedBySemanticID {
		selected = append(selected, submodel)
	}
	selected = append(selected, selectedWithoutSemanticID...)
	sort.Slice(selected, func(left int, right int) bool {
		return contentSubmodelSortKey(selected[left]) < contentSubmodelSortKey(selected[right])
	})
	return selected, nil
}

func isNewerDPPContentSubmodel(candidate types.ISubmodel, current types.ISubmodel) (bool, error) {
	candidateUpdatedAt, err := dppContentSubmodelTimestamp(candidate)
	if err != nil {
		return false, err
	}
	currentUpdatedAt, err := dppContentSubmodelTimestamp(current)
	if err != nil {
		return false, err
	}
	if !candidateUpdatedAt.Equal(currentUpdatedAt) {
		return candidateUpdatedAt.After(currentUpdatedAt), nil
	}
	return candidate.ID() > current.ID(), nil
}

func dppContentSubmodelTimestamp(submodel types.ISubmodel) (time.Time, error) {
	administration := submodel.Administration()
	if administration == nil {
		return time.Time{}, nil
	}
	if updatedAt := administration.UpdatedAt(); updatedAt != nil && strings.TrimSpace(*updatedAt) != "" {
		parsed, err := common.ParseISO8601DateTime(*updatedAt)
		if err != nil {
			return time.Time{}, fmt.Errorf("DPP-FILTER-UPDATEDAT parse updatedAt for submodel %s: %w", submodel.ID(), err)
		}
		return parsed, nil
	}
	if createdAt := administration.CreatedAt(); createdAt != nil && strings.TrimSpace(*createdAt) != "" {
		parsed, err := common.ParseISO8601DateTime(*createdAt)
		if err != nil {
			return time.Time{}, fmt.Errorf("DPP-FILTER-CREATEDAT parse createdAt for submodel %s: %w", submodel.ID(), err)
		}
		return parsed, nil
	}
	return time.Time{}, nil
}

func contentSubmodelSortKey(submodel types.ISubmodel) string {
	semanticID := referenceToString(submodel.SemanticID())
	if semanticID != "" {
		return semanticID
	}
	return submodel.ID()
}

func contentSpecificationSetFromHeader(header dppDocument) (map[string]struct{}, error) {
	specificationIDs, err := contentSpecificationIDsFromHeader(header)
	if err != nil {
		return nil, err
	}
	specificationSet := make(map[string]struct{}, len(specificationIDs))
	for _, specificationID := range specificationIDs {
		specificationSet[specificationID] = struct{}{}
	}
	return specificationSet, nil
}

func contentSpecificationIDsFromHeader(header dppDocument) ([]string, error) {
	rawIDs, ok := header[headerContentSpecificationIDs]
	if !ok {
		return nil, nil
	}
	switch typedIDs := rawIDs.(type) {
	case []any:
		return contentSpecificationIDsFromValues(typedIDs)
	case []string:
		return contentSpecificationIDsFromStrings(typedIDs)
	default:
		return nil, fmt.Errorf("DPP-FILTER-SEMSPEC metadata %s must be an array of strings", headerContentSpecificationIDs)
	}
}

func contentSpecificationIDsFromValues(rawItems []any) ([]string, error) {
	ids := make([]string, 0, len(rawItems))
	for _, rawItem := range rawItems {
		id, ok := rawItem.(string)
		if !ok || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("DPP-FILTER-SEMSPEC metadata %s must contain only non-empty strings", headerContentSpecificationIDs)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func contentSpecificationIDsFromStrings(rawItems []string) ([]string, error) {
	ids := make([]string, 0, len(rawItems))
	for _, id := range rawItems {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("DPP-FILTER-SEMSPEC metadata %s must contain only non-empty strings", headerContentSpecificationIDs)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *DPPRepositoryService) buildSubmodels(header dppHeader, sections map[string]any, metadataID string) ([]types.ISubmodel, []types.IReference, error) {
	metadata := buildMetadataSubmodelWithID(metadataID, header)
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
		setNewDPPSubmodelAdministration(submodel, header.LastUpdate)
		submodels = append(submodels, submodel)
		refs = append(refs, submodelReference(submodel.ID()))
	}
	return submodels, refs, nil
}

func (s *DPPRepositoryService) resolveElementPath(ctx context.Context, dppID string, elementIDPath string) (string, string, types.ISubmodel, error) {
	if err := validateDPPElementPath(elementIDPath); err != nil {
		return "", "", nil, err
	}
	resolved, err := s.resolveSubmodels(ctx, dppID, time.Time{})
	if err != nil {
		return "", "", nil, err
	}
	submodelID, idShortPath, err := resolveDPPElementPathParts(resolved, elementIDPath)
	return submodelID, idShortPath, resolved.metadata, err
}

func resolveDPPElementPath(resolved resolvedDPP, elementIDPath string) (string, string, error) {
	if err := validateDPPElementPath(elementIDPath); err != nil {
		return "", "", err
	}
	return resolveDPPElementPathParts(resolved, elementIDPath)
}

func validateDPPElementPath(elementIDPath string) error {
	_, err := parseDPPJSONElementPath(elementIDPath)
	return err
}

func resolveDPPElementPathParts(resolved resolvedDPP, elementIDPath string) (string, string, error) {
	header, err := composeHeader(resolved.metadata)
	if err != nil {
		return "", "", err
	}
	specificationSet, err := contentSpecificationSetFromHeader(header)
	if err != nil {
		return "", "", err
	}
	contentSubmodels, err := selectedContentSubmodelsForHeader(header, resolved.metadata.ID(), resolved.submodels)
	if err != nil {
		return "", "", err
	}
	return resolveDPPJSONElementPathParts(contentSubmodels, specificationSet, elementIDPath)
}

func resolveDPPJSONElementPathParts(contentSubmodels []types.ISubmodel, specificationSet map[string]struct{}, elementIDPath string) (string, string, error) {
	parsed, err := parseDPPJSONElementPath(elementIDPath)
	if err != nil {
		return "", "", err
	}
	for _, submodel := range contentSubmodels {
		if parsed.sectionName == compressedContentSectionName(submodel, specificationSet) {
			return submodel.ID(), parsed.idShortPath, nil
		}
	}
	return "", "", fmt.Errorf("DPP-ELEMPATH-NOTFOUND content section %s not found", parsed.sectionName)
}

func compressedContentSectionName(submodel types.ISubmodel, specificationSet map[string]struct{}) string {
	if semanticID := referenceToString(submodel.SemanticID()); semanticID != "" {
		if _, selected := specificationSet[semanticID]; selected {
			return semanticID
		}
	}
	return lowerFirst(idShortOrID(submodel))
}

func updatedMetadata(metadata types.ISubmodel) (types.ISubmodel, error) {
	header, err := composeHeader(metadata)
	if err != nil {
		return nil, fmt.Errorf("DPP-TOUCHMETA-COMPOSE compose current metadata: %w", err)
	}
	updatedAt, err := nextDPPUpdateTimestamp(time.Now().UTC(), header, []types.ISubmodel{metadata})
	if err != nil {
		return nil, fmt.Errorf("DPP-TOUCHMETA-TIMESTAMP determine next update timestamp: %w", err)
	}
	for _, element := range metadata.SubmodelElements() {
		if element.IDShort() != nil && *element.IDShort() == headerLastUpdate {
			value := updatedAt.Format(time.RFC3339Nano)
			if property, ok := element.(*types.Property); ok {
				property.SetValue(&value)
			}
		}
	}
	metadata.SetAdministration(updatedDPPAdministration(metadata, updatedAt))
	return metadata, nil
}

func elementResponse(element types.ISubmodelElement, representation Representation, elementIDPath string) (any, error) {
	return elementResponseWithContext(
		element, representation, elementIDPath, idShortValue(element), dppSerializationContext{},
	)
}

func elementResponseWithContext(
	element types.ISubmodelElement,
	representation Representation,
	elementIDPath string,
	idShortPath string,
	serializationContext dppSerializationContext,
) (any, error) {
	if representation == REPRESENTATION_FULL {
		response, err := dppElementFromAASWithContext(element, idShortPath, serializationContext)
		if err != nil {
			return nil, fmt.Errorf("DPP-ELEM-FULL convert element to DPP expanded representation: %w", err)
		}
		if response["elementId"] == "" {
			parsed, err := parseDPPJSONElementPath(elementIDPath)
			if err != nil {
				return nil, err
			}
			response["elementId"] = parsed.elementID
		}
		return response, nil
	}
	return compressedElementValueWithContext(element, idShortPath, serializationContext)
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
