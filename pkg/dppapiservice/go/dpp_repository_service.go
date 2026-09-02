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
	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/registrysync"
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
	aasRegistry        aasRegistryPersistence
	submodelRegistry   submodelRegistryPersistence
	registrySyncConfig registrysync.Config
}

type aasRegistryPersistence interface {
	InsertAdministrationShellDescriptorInTransaction(context.Context, *sql.Tx, commonmodel.AssetAdministrationShellDescriptor) error
	UpsertAdministrationShellDescriptorInTransaction(context.Context, *sql.Tx, commonmodel.AssetAdministrationShellDescriptor) error
	GetAssetAdministrationShellDescriptorByIDInTransaction(context.Context, *sql.Tx, string) (commonmodel.AssetAdministrationShellDescriptor, error)
	DeleteAssetAdministrationShellDescriptorByIDInTransaction(context.Context, *sql.Tx, string) error
}

type submodelRegistryPersistence interface {
	InsertSubmodelDescriptorsInTransaction(context.Context, *sql.Tx, []commonmodel.SubmodelDescriptor) (int, error)
	UpsertSubmodelDescriptorInTransaction(context.Context, *sql.Tx, commonmodel.SubmodelDescriptor) error
	GetSubmodelDescriptorByIDInTransaction(context.Context, *sql.Tx, string) (commonmodel.SubmodelDescriptor, error)
	DeleteSubmodelDescriptorByIDInTransaction(context.Context, *sql.Tx, string) error
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
//
// Parameters:
//   - aasRepo: Persistence repository for Asset Administration Shell records
//   - submodelRepo: Persistence repository for DPP metadata and content submodels
//   - aasRegistry: Optional AAS registry persistence dependency, required when enabled
//   - submodelRegistry: Optional Submodel registry persistence dependency, required when enabled
//   - registrySyncConfig: Validated descriptor synchronization configuration
//
// Returns:
//   - *DPPRepositoryService: Configured DPP repository service
//   - error: Configuration error when an enabled registry dependency is missing
func NewDPPRepositoryServiceWithRegistrySync(
	aasRepo *aasrepositorydb.AssetAdministrationShellDatabase,
	submodelRepo *submodelrepositorydb.SubmodelDatabase,
	aasRegistry *aasregistrydb.PostgreSQLAASRegistryDatabase,
	submodelRegistry *smregistrydb.PostgreSQLSMDatabase,
	registrySyncConfig registrysync.Config,
) (*DPPRepositoryService, error) {
	if registrySyncConfig.AASRegistryIntegration && aasRegistry == nil {
		return nil, common.NewInternalServerError("DPP-REGSYNC-NILAASREGISTRY AAS registry backend must not be nil")
	}
	if registrySyncConfig.SubmodelRegistryIntegration && submodelRegistry == nil {
		return nil, common.NewInternalServerError("DPP-REGSYNC-NILSMREGISTRY Submodel registry backend must not be nil")
	}
	return newDPPRepositoryServiceWithRegistrySync(
		aasRepo, submodelRepo, aasRegistry, submodelRegistry, registrySyncConfig,
	)
}

func newDPPRepositoryServiceWithRegistrySync(
	aasRepo *aasrepositorydb.AssetAdministrationShellDatabase,
	submodelRepo *submodelrepositorydb.SubmodelDatabase,
	aasRegistry aasRegistryPersistence,
	submodelRegistry submodelRegistryPersistence,
	registrySyncConfig registrysync.Config,
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
		descriptors, err := s.buildSubmodelDescriptors(submodels)
		if err != nil {
			return err
		}
		failedIndex, err := s.submodelRegistry.InsertSubmodelDescriptorsInTransaction(
			registrysync.WithSubmodelRegistrySyncUpsertAudit(ctx), tx, descriptors,
		)
		if err != nil {
			return fmt.Errorf("DPP-REGSYNC-CREATE-INSERTSMDESCS insert submodel descriptors at index %d: %w", failedIndex, err)
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
		registrysync.WithAASRegistrySyncUpsertAudit(ctx), tx, descriptor,
	); err != nil {
		return fmt.Errorf("DPP-REGSYNC-CREATE-INSERTAASDESC insert AAS descriptor: %w", err)
	}
	return nil
}

func (s *DPPRepositoryService) buildSubmodelDescriptors(submodels []types.ISubmodel) ([]commonmodel.SubmodelDescriptor, error) {
	descriptors := make([]commonmodel.SubmodelDescriptor, 0, len(submodels))
	for _, submodel := range submodels {
		descriptor, err := s.registrySyncConfig.BuildSubmodelDescriptor(submodel)
		if err != nil {
			return nil, fmt.Errorf("DPP-REGSYNC-CREATE-BUILDSMDESC build submodel descriptor %s: %w", submodel.ID(), err)
		}
		descriptors = append(descriptors, descriptor)
	}
	return descriptors, nil
}

type submodelDescriptorUpdate struct {
	previous  types.ISubmodel
	submitted types.ISubmodel
}

func (s *DPPRepositoryService) syncUpdatedDescriptors(
	ctx context.Context,
	tx *sql.Tx,
	previousAAS types.IAssetAdministrationShell,
	aas types.IAssetAdministrationShell,
	submodels []submodelDescriptorUpdate,
	staleSubmodelIDs []string,
) error {
	if s.registrySyncConfig.SubmodelRegistryIntegration {
		if err := s.upsertSubmodelDescriptors(ctx, tx, submodels); err != nil {
			return err
		}
		for _, submodelID := range staleSubmodelIDs {
			err := s.submodelRegistry.DeleteSubmodelDescriptorByIDInTransaction(
				registrysync.WithSubmodelRegistrySyncDeleteAudit(ctx), tx, submodelID,
			)
			if err != nil && !common.IsErrNotFound(err) {
				return fmt.Errorf("DPP-REGSYNC-UPDATE-DELETESMDESC delete stale submodel descriptor %s: %w", submodelID, err)
			}
		}
	}
	if !s.registrySyncConfig.AASRegistryIntegration || aas == nil {
		return nil
	}
	descriptor, changed, err := s.registrySyncConfig.ChangedAASDescriptor(previousAAS, aas)
	if err != nil {
		return fmt.Errorf("DPP-REGSYNC-UPDATE-BUILDAASDESC build AAS descriptor: %w", err)
	}
	if !changed {
		_, err = s.aasRegistry.GetAssetAdministrationShellDescriptorByIDInTransaction(ctx, tx, aas.ID())
		if err == nil {
			return nil
		}
		if !common.IsErrNotFound(err) {
			return fmt.Errorf("DPP-REGSYNC-UPDATE-GETAASDESC get AAS descriptor: %w", err)
		}
	}
	if err = s.aasRegistry.UpsertAdministrationShellDescriptorInTransaction(
		registrysync.WithAASRegistrySyncUpsertAudit(ctx), tx, descriptor,
	); err != nil {
		return fmt.Errorf("DPP-REGSYNC-UPDATE-UPSERTAASDESC upsert AAS descriptor: %w", err)
	}
	return nil
}

func (s *DPPRepositoryService) upsertSubmodelDescriptors(ctx context.Context, tx *sql.Tx, submodels []submodelDescriptorUpdate) error {
	for _, submodel := range submodels {
		descriptor, changed, err := s.registrySyncConfig.ChangedSubmodelDescriptor(submodel.previous, submodel.submitted)
		if err != nil {
			return fmt.Errorf("DPP-REGSYNC-UPDATE-BUILDSMDESC build submodel descriptor %s: %w", submodel.submitted.ID(), err)
		}
		if !changed {
			_, err = s.submodelRegistry.GetSubmodelDescriptorByIDInTransaction(ctx, tx, submodel.submitted.ID())
			if err == nil {
				continue
			}
			if !common.IsErrNotFound(err) {
				return fmt.Errorf("DPP-REGSYNC-UPDATE-GETSMDESC get submodel descriptor %s: %w", submodel.submitted.ID(), err)
			}
		}
		if err = s.submodelRegistry.UpsertSubmodelDescriptorInTransaction(
			registrysync.WithSubmodelRegistrySyncUpsertAudit(ctx), tx, descriptor,
		); err != nil {
			return fmt.Errorf("DPP-REGSYNC-UPDATE-UPSERTSMDESC upsert submodel descriptor %s: %w", submodel.submitted.ID(), err)
		}
	}
	return nil
}

func (s *DPPRepositoryService) deleteDPPResourcesInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	dppID string,
	submodels []types.ISubmodel,
) error {
	for _, submodel := range submodels {
		if err := s.submodelRepo.DeleteSubmodelInTransaction(ctx, tx, submodel.ID()); err != nil {
			return fmt.Errorf("DPP-DELDPP-DELETESUBMODEL delete submodel %s: %w", submodel.ID(), err)
		}
		if err := s.deleteSubmodelDescriptorIfEnabled(ctx, tx, submodel.ID()); err != nil {
			return err
		}
	}
	if err := s.aasRepo.DeleteAssetAdministrationShellByIDInTransaction(ctx, tx, dppID); err != nil {
		return fmt.Errorf("DPP-DELDPP-DELETEAAS delete AAS: %w", err)
	}
	return s.deleteAASDescriptorIfEnabled(ctx, tx, dppID)
}

func (s *DPPRepositoryService) deleteSubmodelDescriptorIfEnabled(ctx context.Context, tx *sql.Tx, submodelID string) error {
	if !s.registrySyncConfig.SubmodelRegistryIntegration {
		return nil
	}
	err := s.submodelRegistry.DeleteSubmodelDescriptorByIDInTransaction(
		registrysync.WithSubmodelRegistrySyncDeleteAudit(ctx), tx, submodelID,
	)
	if err != nil && !common.IsErrNotFound(err) {
		return fmt.Errorf("DPP-REGSYNC-DELETE-DELETESMDESC delete submodel descriptor %s: %w", submodelID, err)
	}
	return nil
}

func (s *DPPRepositoryService) deleteAASDescriptorIfEnabled(ctx context.Context, tx *sql.Tx, dppID string) error {
	if !s.registrySyncConfig.AASRegistryIntegration {
		return nil
	}
	err := s.aasRegistry.DeleteAssetAdministrationShellDescriptorByIDInTransaction(
		registrysync.WithAASRegistrySyncDeleteAudit(ctx), tx, dppID,
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
	resolved, currentContent, current, err := s.loadDPPUpdateState(ctx, dppID)
	if err != nil {
		return mapPersistenceError(err, http.StatusNotFound), nil
	}
	update, err := s.prepareDPPUpdate(ctx, dppID, patch, resolved, currentContent, current)
	if err != nil {
		return mapPersistenceError(err, http.StatusBadRequest), nil
	}
	err = s.persistDPPUpdate(ctx, dppID, update)
	if err != nil {
		return mapPersistenceError(err, http.StatusConflict), nil
	}

	updated, err := s.composeDPP(ctx, dppID, REPRESENTATION_COMPRESSED, time.Time{})
	if err != nil {
		return mapPersistenceError(err, http.StatusInternalServerError), nil
	}
	return Response(http.StatusOK, updated), nil
}

type preparedDPPUpdate struct {
	aas              types.IAssetAdministrationShell
	submodels        []types.ISubmodel
	staleSubmodelIDs []string
}

func (s *DPPRepositoryService) prepareDPPUpdate(
	ctx context.Context,
	dppID string,
	patch dppDocument,
	resolved resolvedDPP,
	currentContent []types.ISubmodel,
	current dppDocument,
) (preparedDPPUpdate, error) {
	merged, header, err := mergeDPPUpdateDocument(dppID, current, currentContent, patch)
	if err != nil {
		return preparedDPPUpdate{}, err
	}
	submodels, refs, err := s.buildSubmodels(header, contentSections(merged), resolved.metadata.ID())
	if err != nil {
		return preparedDPPUpdate{}, err
	}
	applyDPPUpdateAdministration(submodels, resolved.metadata, currentContent, header.LastUpdate)
	if err = s.preserveManagedAttachments(ctx, submodels, currentContent); err != nil {
		return preparedDPPUpdate{}, err
	}
	refs = appendUnselectedContentSubmodelReferences(refs, resolved, currentContent)
	return preparedDPPUpdate{
		aas:              buildAAS(header, refs),
		submodels:        submodels,
		staleSubmodelIDs: staleContentSubmodelIDs(currentContent, submodels),
	}, nil
}

func (s *DPPRepositoryService) loadDPPUpdateState(
	ctx context.Context,
	dppID string,
) (resolvedDPP, []types.ISubmodel, dppDocument, error) {
	resolved, err := s.resolveSubmodels(ctx, dppID, time.Time{})
	if err != nil {
		return resolvedDPP{}, nil, nil, err
	}
	currentContent, err := selectedResolvedContentSubmodels(resolved)
	if err != nil {
		return resolvedDPP{}, nil, nil, fmt.Errorf(
			"DPP-UPDDPP-SELECTCONTENT %w",
			common.NewInternalServerError(err.Error()),
		)
	}
	current, err := s.composeDPP(ctx, dppID, REPRESENTATION_COMPRESSED, time.Time{})
	return resolved, currentContent, current, err
}

func mergeDPPUpdateDocument(
	dppID string,
	current dppDocument,
	currentContent []types.ISubmodel,
	patch dppDocument,
) (dppDocument, dppHeader, error) {
	merged := dppObjectFromAny(applyMergePatch(current, patch))
	if merged == nil {
		return nil, dppHeader{}, errors.New("DPP-UPDDPP-MERGE merged DPP must be a JSON object")
	}
	updatedAt, err := nextDPPUpdateTimestamp(time.Now().UTC(), current, currentContent)
	if err != nil {
		return nil, dppHeader{}, err
	}
	merged[headerDigitalProductPassportID] = dppID
	merged[headerLastUpdate] = updatedAt.Format(time.RFC3339Nano)
	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, dppHeader{}, fmt.Errorf("DPP-UPDDPP-MARSHAL marshal merged DPP: %w", err)
	}
	_, header, err := decodeDPPDocument(raw, true)
	return merged, header, err
}

func (s *DPPRepositoryService) persistDPPUpdate(ctx context.Context, dppID string, update preparedDPPUpdate) error {
	return s.aasRepo.ExecuteInTransaction("DPP-UPDDPP-STARTTX", "DPP-UPDDPP-COMMITTX", func(tx *sql.Tx) error {
		aasResult, err := s.aasRepo.PutAssetAdministrationShellByIDInTransactionWithResult(ctx, tx, dppID, update.aas)
		if err != nil {
			return fmt.Errorf("DPP-UPDDPP-PUTAAS put AAS: %w", err)
		}
		descriptorUpdates := make([]submodelDescriptorUpdate, 0, len(update.submodels))
		for _, submodel := range update.submodels {
			putResult, putErr := s.submodelRepo.PutSubmodelInTransactionWithResult(ctx, tx, submodel.ID(), submodel)
			if putErr != nil {
				return fmt.Errorf("DPP-UPDDPP-PUTSUBMODEL put submodel %s: %w", submodel.ID(), putErr)
			}
			descriptorUpdates = append(descriptorUpdates, submodelDescriptorUpdate{previous: putResult.Previous, submitted: submodel})
		}
		for _, submodelID := range update.staleSubmodelIDs {
			if err := s.submodelRepo.DeleteSubmodelInTransaction(ctx, tx, submodelID); err != nil {
				return fmt.Errorf("DPP-UPDDPP-DELETESUBMODEL delete stale submodel %s: %w", submodelID, err)
			}
		}
		return s.syncUpdatedDescriptors(ctx, tx, aasResult.Previous, update.aas, descriptorUpdates, update.staleSubmodelIDs)
	})
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
	type attachmentPair struct {
		current     types.ISubmodel
		replacement types.ISubmodel
	}
	pairs := make([]attachmentPair, 0, len(replacements))
	currentWithFiles := make([]types.ISubmodel, 0, len(replacements))
	for _, replacement := range replacements {
		current := matchingCurrentDPPSubmodel(replacement, currentByID, currentBySemanticID)
		if current == nil || !submodelContainsFile(current) || !submodelContainsFile(replacement) {
			continue
		}
		pairs = append(pairs, attachmentPair{current: current, replacement: replacement})
		currentWithFiles = append(currentWithFiles, current)
	}
	contextLoader, err := s.serializationContextLoader(ctx, currentWithFiles, false)
	if err != nil {
		return newDPPHTTPError(
			http.StatusInternalServerError,
			"DPP-UPDDPP-PRESERVEFILES-LOADPATHS",
			"load managed attachment paths: "+err.Error(),
		)
	}
	for _, pair := range pairs {
		serializationContext, _ := contextLoader(pair.current)
		preserveManagedFileValues(pair.current, pair.replacement, serializationContext)
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

func submodelContainsFile(submodel types.ISubmodel) bool {
	return elementsContainFile(submodel.SubmodelElements())
}

func elementsContainFile(elements []types.ISubmodelElement) bool {
	for _, element := range elements {
		if elementContainsFile(element) {
			return true
		}
	}
	return false
}

func elementContainsFile(element types.ISubmodelElement) bool {
	switch typed := element.(type) {
	case *types.File:
		return true
	case *types.SubmodelElementCollection:
		return elementsContainFile(typed.Value())
	case *types.SubmodelElementList:
		return elementsContainFile(typed.Value())
	default:
		return false
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
		return s.deleteDPPResourcesInTransaction(ctx, tx, dppID, resolved.submodels)
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
	serializationContext := newDPPSerializationContext(ctx, submodelID, false, nil)
	if elementContainsFile(element) {
		serializationContext, err = s.serializationContext(ctx, submodelID, false)
		if err != nil {
			return mapPersistenceError(err, http.StatusInternalServerError), nil
		}
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
	err = s.aasRepo.ExecuteInTransaction("DPP-UPDELEM-STARTTX", "DPP-UPDELEM-COMMITTX", func(tx *sql.Tx) error {
		if _, err := s.submodelRepo.PutSubmodelElementInTransaction(ctx, tx, submodelID, idShortPath, element); err != nil {
			return fmt.Errorf("DPP-UPDELEM-PUTELEMENT put element %s: %w", idShortPath, err)
		}
		putResult, putErr := s.submodelRepo.PutSubmodelInTransactionWithResult(ctx, tx, metadata.ID(), metadata)
		if putErr != nil {
			return fmt.Errorf("DPP-UPDELEM-PUTMETADATA put metadata: %w", putErr)
		}
		return s.syncUpdatedDescriptors(ctx, tx, nil, nil, []submodelDescriptorUpdate{{
			previous: putResult.Previous, submitted: metadata,
		}}, nil)
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
	contextLoader, err := s.serializationContextLoader(ctx, resolved.submodels, !at.IsZero())
	if err != nil {
		return nil, err
	}
	return composeResolvedDPPWithContext(resolved, representation, contextLoader)
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
		return composeFullResolvedDPP(doc, contentSubmodels, contextLoader)
	}
	return composeCompressedResolvedDPP(doc, specificationSet, contentSubmodels, contextLoader)
}

func composeFullResolvedDPP(
	doc dppDocument,
	contentSubmodels []types.ISubmodel,
	contextLoader dppSerializationContextLoader,
) (dppDocument, error) {
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

func composeCompressedResolvedDPP(
	doc dppDocument,
	specificationSet map[string]struct{},
	contentSubmodels []types.ISubmodel,
	contextLoader dppSerializationContextLoader,
) (dppDocument, error) {
	for _, submodel := range contentSubmodels {
		serializationContext, err := loadDPPSerializationContext(contextLoader, submodel)
		if err != nil {
			return nil, err
		}
		content, err := compressedContentWithContext(submodel, serializationContext)
		if err != nil {
			return nil, err
		}
		doc[compressedContentSectionName(submodel, specificationSet)] = content
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
	managedPathsBySubmodel, err := s.submodelRepo.ManagedFileAttachmentPathsBySubmodelIDs(ctx, []string{submodelID})
	if err != nil {
		return dppSerializationContext{}, fmt.Errorf(
			"DPP-FILEURL-LOADMANAGEDPATHS load managed attachment paths for %s: %w", submodelID, err,
		)
	}
	return newDPPSerializationContext(ctx, submodelID, historical, managedPathsBySubmodel[submodelID]), nil
}

func (s *DPPRepositoryService) serializationContextLoader(
	ctx context.Context,
	submodels []types.ISubmodel,
	historical bool,
) (dppSerializationContextLoader, error) {
	submodelIDs := make([]string, 0, len(submodels))
	seen := make(map[string]struct{}, len(submodels))
	for _, submodel := range submodels {
		if !submodelContainsFile(submodel) {
			continue
		}
		if _, exists := seen[submodel.ID()]; exists {
			continue
		}
		seen[submodel.ID()] = struct{}{}
		submodelIDs = append(submodelIDs, submodel.ID())
	}
	if len(submodelIDs) == 0 {
		return func(submodel types.ISubmodel) (dppSerializationContext, error) {
			return newDPPSerializationContext(ctx, submodel.ID(), historical, nil), nil
		}, nil
	}
	managedPaths, err := s.submodelRepo.ManagedFileAttachmentPathsBySubmodelIDs(ctx, submodelIDs)
	if err != nil {
		return nil, fmt.Errorf("DPP-FILEURL-LOADMANAGEDPATHS load managed attachment paths: %w", err)
	}
	return func(submodel types.ISubmodel) (dppSerializationContext, error) {
		return newDPPSerializationContext(ctx, submodel.ID(), historical, managedPaths[submodel.ID()]), nil
	}, nil
}

func newDPPSerializationContext(
	ctx context.Context,
	submodelID string,
	historical bool,
	managedPaths map[string]struct{},
) dppSerializationContext {
	return dppSerializationContext{
		submodelID:               submodelID,
		externalBaseURL:          common.ExternalBaseURLFromContext(ctx),
		managedAttachmentPaths:   managedPaths,
		managedPathHistoryLookup: historical,
	}
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
