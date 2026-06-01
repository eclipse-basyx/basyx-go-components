package aasenvironment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	submodelrepositoryapi "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/api"
)

// CustomSubmodelRepositoryService is a pass-through stub for future combined logic.
type CustomSubmodelRepositoryService struct {
	*submodelrepositoryapi.SubmodelRepositoryAPIAPIService
	persistence                                 *Persistence
	syncConfig                                  RegistrySyncConfig
	enableReferencingAASDescriptorEmbeddingSync bool
}

// NewCustomSubmodelRepositoryService creates a new pass-through submodel repository decorator.
func NewCustomSubmodelRepositoryService(
	base *submodelrepositoryapi.SubmodelRepositoryAPIAPIService,
	persistence *Persistence,
	syncConfig RegistrySyncConfig,
) *CustomSubmodelRepositoryService {
	return NewCustomSubmodelRepositoryServiceWithAASDescriptorEmbeddingSync(
		base,
		persistence,
		syncConfig,
		syncConfig.AASRegistryIntegration,
	)
}

// NewCustomSubmodelRepositoryServiceWithAASDescriptorEmbeddingSync creates a submodel repository decorator with explicit AAS descriptor embedding sync behavior.
func NewCustomSubmodelRepositoryServiceWithAASDescriptorEmbeddingSync(
	base *submodelrepositoryapi.SubmodelRepositoryAPIAPIService,
	persistence *Persistence,
	syncConfig RegistrySyncConfig,
	enableReferencingAASDescriptorEmbeddingSync bool,
) *CustomSubmodelRepositoryService {
	return &CustomSubmodelRepositoryService{
		SubmodelRepositoryAPIAPIService: base,
		persistence:                     persistence,
		syncConfig:                      syncConfig,
		enableReferencingAASDescriptorEmbeddingSync: enableReferencingAASDescriptorEmbeddingSync,
	}
}

// ExecuteInTransaction exposes shared transaction execution for future endpoint customizations.
func (s *CustomSubmodelRepositoryService) ExecuteInTransaction(fn func(tx *sql.Tx) error) error {
	if s == nil || s.persistence == nil {
		return common.NewErrBadRequest("AASENV-SMREPO-TX-NILSERVICE service must not be nil")
	}
	return s.persistence.ExecuteInTransaction("AASENV-SMREPO-STARTTX", "AASENV-SMREPO-COMMITTX", fn)
}

func (s *CustomSubmodelRepositoryService) validateSyncDependencies(requireAASRepository bool, requireAASRegistry bool) error {
	if s == nil {
		return common.NewInternalServerError("AASENV-SMREPO-CHECKDEPS-NILSERVICE service must not be nil")
	}
	if s.persistence == nil {
		return common.NewInternalServerError("AASENV-SMREPO-CHECKDEPS-NILPERSISTENCE persistence bundle must not be nil")
	}
	if s.persistence.SubmodelRepository == nil {
		return common.NewInternalServerError("AASENV-SMREPO-CHECKDEPS-NILSMREPO Submodel repository backend must not be nil")
	}
	if s.persistence.SubmodelRegistry == nil {
		return common.NewInternalServerError("AASENV-SMREPO-CHECKDEPS-NILSMREGISTRY Submodel registry backend must not be nil")
	}
	if requireAASRepository && s.persistence.AASRepository == nil {
		return common.NewInternalServerError("AASENV-SMREPO-CHECKDEPS-NILAASREPO AAS repository backend must not be nil")
	}
	if requireAASRegistry && s.persistence.AASRegistry == nil {
		return common.NewInternalServerError("AASENV-SMREPO-CHECKDEPS-NILAASREGISTRY AAS registry backend must not be nil")
	}

	return nil
}

// PostSubmodel creates a new submodel and synchronizes descriptor writes in the same transaction.
func (s *CustomSubmodelRepositoryService) PostSubmodel(ctx context.Context, submodel types.ISubmodel) (commonmodel.ImplResponse, error) {
	const operation = "PostSubmodel"
	if !s.syncConfig.SubmodelRegistryIntegration {
		return s.SubmodelRepositoryAPIAPIService.PostSubmodel(ctx, submodel)
	}
	if dependencyErr := s.validateSyncDependencies(s.enableReferencingAASDescriptorEmbeddingSync, s.enableReferencingAASDescriptorEmbeddingSync); dependencyErr != nil {
		return newSubmodelRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		if createErr := s.persistence.SubmodelRepository.CreateSubmodelInTransaction(ctx, tx, submodel); createErr != nil {
			return createErr
		}

		descriptor, descriptorErr := s.syncConfig.buildSubmodelDescriptor(submodel)
		if descriptorErr != nil {
			return descriptorErr
		}
		if upsertErr := s.persistence.SubmodelRegistry.UpsertSubmodelDescriptorInTransaction(ctx, tx, descriptor); upsertErr != nil {
			return upsertErr
		}

		return s.syncReferencingAASDescriptorsInTransaction(ctx, tx, descriptor, nil, false)
	})
	if err != nil {
		if common.IsErrDenied(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusForbidden, operation, "Denied"), nil
		}
		if common.IsErrConflict(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusConflict, operation, "IdConflict"), nil
		}
		if common.IsErrBadRequest(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusBadRequest, operation, "InvalidSubmodelData"), nil
		}
		return newSubmodelRepoErrorResponse(err, http.StatusInternalServerError, operation, "CreateSubmodel"), nil
	}

	submodelJSON, jsonErr := jsonization.ToJsonable(submodel)
	if jsonErr != nil {
		return newSubmodelRepoErrorResponse(jsonErr, http.StatusBadRequest, operation, "InvalidSubmodelData"), nil
	}
	return commonmodel.Response(http.StatusCreated, submodelJSON), nil
}

// PutSubmodelByID upserts a submodel and synchronizes descriptor writes in the same transaction.
func (s *CustomSubmodelRepositoryService) PutSubmodelByID(ctx context.Context, submodelIdentifier string, submodel types.ISubmodel) (commonmodel.ImplResponse, error) {
	const operation = "PutSubmodelByID"
	if !s.syncConfig.SubmodelRegistryIntegration {
		return s.SubmodelRepositoryAPIAPIService.PutSubmodelByID(ctx, submodelIdentifier, submodel)
	}
	if dependencyErr := s.validateSyncDependencies(s.enableReferencingAASDescriptorEmbeddingSync, s.enableReferencingAASDescriptorEmbeddingSync); dependencyErr != nil {
		return newSubmodelRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	decodedIdentifier, decodeErr := common.DecodeString(submodelIdentifier)
	if decodeErr != nil {
		return newSubmodelRepoErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedSubmodelIdentifier"), nil
	}
	if decodedIdentifier != submodel.ID() {
		return newSubmodelRepoErrorResponse(errors.New("submodel ID in path and body do not match"), http.StatusBadRequest, operation, "IdMismatch"), nil
	}

	isUpdate := false
	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		updated, putErr := s.persistence.SubmodelRepository.PutSubmodelInTransaction(ctx, tx, decodedIdentifier, submodel)
		if putErr != nil {
			return putErr
		}
		isUpdate = updated

		descriptor, descriptorErr := s.syncConfig.buildSubmodelDescriptor(submodel)
		if descriptorErr != nil {
			return descriptorErr
		}
		if upsertErr := s.persistence.SubmodelRegistry.UpsertSubmodelDescriptorInTransaction(ctx, tx, descriptor); upsertErr != nil {
			return upsertErr
		}

		return s.syncReferencingAASDescriptorsInTransaction(ctx, tx, descriptor, nil, false)
	})
	if err != nil {
		if common.IsErrDenied(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusForbidden, operation, "Denied"), nil
		}
		if common.IsErrBadRequest(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		if common.IsErrConflict(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusConflict, operation, "Conflict"), nil
		}
		if common.IsErrNotFound(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusNotFound, operation, "SubmodelNotFound"), nil
		}
		return newSubmodelRepoErrorResponse(err, http.StatusInternalServerError, operation, "InternalServerError"), nil
	}

	if isUpdate {
		return commonmodel.Response(http.StatusNoContent, nil), nil
	}

	jsonSubmodel, jsonErr := jsonization.ToJsonable(submodel)
	if jsonErr != nil {
		return newSubmodelRepoErrorResponse(jsonErr, http.StatusBadRequest, operation, "InvalidSubmodelData"), nil
	}
	return commonmodel.Response(http.StatusCreated, jsonSubmodel), nil
}

// DeleteSubmodelByID deletes a submodel and synchronizes descriptor deletion in the same transaction.
func (s *CustomSubmodelRepositoryService) DeleteSubmodelByID(ctx context.Context, id string) (commonmodel.ImplResponse, error) {
	const operation = "DeleteSubmodelByID"
	if !s.syncConfig.SubmodelRegistryIntegration {
		return s.SubmodelRepositoryAPIAPIService.DeleteSubmodelByID(ctx, id)
	}
	if dependencyErr := s.validateSyncDependencies(s.enableReferencingAASDescriptorEmbeddingSync, s.enableReferencingAASDescriptorEmbeddingSync); dependencyErr != nil {
		return newSubmodelRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	decodedSubmodelIdentifier, decodeErr := common.DecodeString(id)
	if decodeErr != nil {
		return newSubmodelRepoErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedSubmodelIdentifier"), nil
	}

	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		if deleteErr := s.persistence.SubmodelRepository.DeleteSubmodelInTransaction(ctx, tx, decodedSubmodelIdentifier); deleteErr != nil {
			return deleteErr
		}
		if deleteDescriptorErr := s.persistence.SubmodelRegistry.DeleteSubmodelDescriptorByIDInTransaction(ctx, tx, decodedSubmodelIdentifier); deleteDescriptorErr != nil {
			return deleteDescriptorErr
		}

		return s.syncReferencingAASDescriptorsInTransaction(ctx, tx, commonmodel.SubmodelDescriptor{Id: decodedSubmodelIdentifier}, nil, true)
	})
	if err != nil {
		if common.IsErrDenied(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusForbidden, operation, "Denied"), nil
		}
		if common.IsErrNotFound(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusNotFound, operation, "SubmodelNotFound"), nil
		}
		if common.IsErrBadRequest(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newSubmodelRepoErrorResponse(err, http.StatusInternalServerError, operation, "InternalServerError"), nil
	}

	return commonmodel.Response(http.StatusNoContent, nil), nil
}

// PatchSubmodelByID updates a submodel and synchronizes descriptor writes in the same transaction.
func (s *CustomSubmodelRepositoryService) PatchSubmodelByID(ctx context.Context, submodelIdentifier string, submodel types.ISubmodel, level string) (commonmodel.ImplResponse, error) {
	_ = level
	const operation = "PatchSubmodelByID"
	if !s.syncConfig.SubmodelRegistryIntegration {
		return s.SubmodelRepositoryAPIAPIService.PatchSubmodelByID(ctx, submodelIdentifier, submodel, level)
	}
	if dependencyErr := s.validateSyncDependencies(s.enableReferencingAASDescriptorEmbeddingSync, s.enableReferencingAASDescriptorEmbeddingSync); dependencyErr != nil {
		return newSubmodelRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	decodedIdentifier, decodeErr := common.DecodeString(submodelIdentifier)
	if decodeErr != nil {
		return newSubmodelRepoErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedSubmodelIdentifier"), nil
	}
	if submodel == nil {
		return newSubmodelRepoErrorResponse(errors.New("submodel payload is required"), http.StatusBadRequest, operation, "MissingSubmodelPayload"), nil
	}
	if submodel.ID() != "" && decodedIdentifier != submodel.ID() {
		return newSubmodelRepoErrorResponse(errors.New("submodel ID in path and body do not match"), http.StatusBadRequest, operation, "IdMismatch"), nil
	}

	patchJSON, patchJSONErr := jsonization.ToJsonable(submodel)
	if patchJSONErr != nil {
		return newSubmodelRepoErrorResponse(patchJSONErr, http.StatusBadRequest, operation, "InvalidSubmodelData"), nil
	}

	_, patchIncludesSubmodelElements := patchJSON["submodelElements"]
	existingSubmodels, _, getErr := s.persistence.SubmodelRepository.GetSubmodels(ctx, 1, "", decodedIdentifier)
	if getErr != nil {
		if common.IsErrNotFound(getErr) || errors.Is(getErr, sql.ErrNoRows) {
			return newSubmodelRepoErrorResponse(getErr, http.StatusNotFound, operation, "SubmodelNotFound"), nil
		}
		return newSubmodelRepoErrorResponse(getErr, http.StatusInternalServerError, operation, "GetSubmodelByID"), nil
	}
	if len(existingSubmodels) == 0 {
		return newSubmodelRepoErrorResponse(common.NewErrNotFound(decodedIdentifier), http.StatusNotFound, operation, "SubmodelNotFound"), nil
	}

	existingSubmodel := existingSubmodels[0]
	if existingSubmodel == nil {
		nilErr := common.NewInternalServerError("SMREPO-PATCHSM-EXISTINGNIL Existing submodel is nil")
		return newSubmodelRepoErrorResponse(nilErr, http.StatusInternalServerError, operation, "GetSubmodelByID"), nil
	}

	existingJSON, existingJSONErr := jsonization.ToJsonable(existingSubmodel)
	if existingJSONErr != nil {
		return newSubmodelRepoErrorResponse(existingJSONErr, http.StatusInternalServerError, operation, "ToJsonableCurrentSubmodel"), nil
	}
	patchJSON["id"] = decodedIdentifier

	mergedJSON := mergeSubmodelJSON(existingJSON, patchJSON)
	mergedSubmodel, mergedErr := jsonization.SubmodelFromJsonable(mergedJSON)
	if mergedErr != nil {
		return newSubmodelRepoErrorResponse(mergedErr, http.StatusBadRequest, operation, "InvalidPatchedSubmodel"), nil
	}

	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		if patchIncludesSubmodelElements {
			if patchErr := s.persistence.SubmodelRepository.PatchSubmodelInTransaction(ctx, decodedIdentifier, tx, mergedSubmodel); patchErr != nil {
				return patchErr
			}
		} else {
			if patchErr := s.persistence.SubmodelRepository.PatchSubmodelMetadataInTransaction(ctx, decodedIdentifier, tx, mergedSubmodel); patchErr != nil {
				return patchErr
			}
		}

		descriptor, descriptorErr := s.syncConfig.buildSubmodelDescriptor(mergedSubmodel)
		if descriptorErr != nil {
			return descriptorErr
		}
		if upsertErr := s.persistence.SubmodelRegistry.UpsertSubmodelDescriptorInTransaction(ctx, tx, descriptor); upsertErr != nil {
			return upsertErr
		}

		return s.syncReferencingAASDescriptorsInTransaction(ctx, tx, descriptor, nil, false)
	})
	if err != nil {
		if common.IsErrBadRequest(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		if common.IsErrNotFound(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusNotFound, operation, "SubmodelNotFound"), nil
		}
		return newSubmodelRepoErrorResponse(err, http.StatusInternalServerError, operation, "InternalServerError"), nil
	}

	return commonmodel.Response(http.StatusNoContent, nil), nil
}

// PatchSubmodelByIDMetadata updates submodel metadata and synchronizes descriptor writes in the same transaction.
func (s *CustomSubmodelRepositoryService) PatchSubmodelByIDMetadata(ctx context.Context, submodelIdentifier string, submodelMetadata commonmodel.SubmodelMetadata) (commonmodel.ImplResponse, error) {
	const operation = "PatchSubmodelByIDMetadata"
	if !s.syncConfig.SubmodelRegistryIntegration {
		return s.SubmodelRepositoryAPIAPIService.PatchSubmodelByIDMetadata(ctx, submodelIdentifier, submodelMetadata)
	}
	if dependencyErr := s.validateSyncDependencies(s.enableReferencingAASDescriptorEmbeddingSync, s.enableReferencingAASDescriptorEmbeddingSync); dependencyErr != nil {
		return newSubmodelRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	decodedIdentifier, decodeErr := common.DecodeString(submodelIdentifier)
	if decodeErr != nil {
		return newSubmodelRepoErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedSubmodelIdentifier"), nil
	}

	if submodelMetadata.ID != "" && decodedIdentifier != submodelMetadata.ID {
		return newSubmodelRepoErrorResponse(errors.New("submodel ID in path and body do not match"), http.StatusBadRequest, operation, "IdMismatch"), nil
	}

	patchJSON, patchJSONErr := submodelMetadataToPatchJSON(submodelMetadata)
	if patchJSONErr != nil {
		return newSubmodelRepoErrorResponse(patchJSONErr, http.StatusBadRequest, operation, "InvalidSubmodelMetadata"), nil
	}
	if rawPatchJSON, hasRawPatch := common.GetSubmodelMetadataPatch(ctx); hasRawPatch {
		patchJSON = rawPatchJSON
	}
	if patchJSON["modelType"] != "Submodel" {
		return newSubmodelRepoErrorResponse(errors.New("modelType for Submodel metadata must be 'Submodel'"), http.StatusBadRequest, operation, "InvalidSubmodelMetadata"), nil
	}
	patchJSON["id"] = decodedIdentifier

	existingSubmodels, _, getErr := s.persistence.SubmodelRepository.GetSubmodels(ctx, 1, "", decodedIdentifier)
	if getErr != nil {
		if common.IsErrNotFound(getErr) || errors.Is(getErr, sql.ErrNoRows) {
			return newSubmodelRepoErrorResponse(getErr, http.StatusNotFound, operation, "SubmodelNotFound"), nil
		}
		return newSubmodelRepoErrorResponse(getErr, http.StatusInternalServerError, operation, "GetSubmodelByID"), nil
	}
	if len(existingSubmodels) == 0 {
		return newSubmodelRepoErrorResponse(common.NewErrNotFound(decodedIdentifier), http.StatusNotFound, operation, "SubmodelNotFound"), nil
	}

	existingSubmodel := existingSubmodels[0]
	if existingSubmodel == nil {
		nilErr := common.NewInternalServerError("SMREPO-PATCHSMMETA-EXISTINGNIL Existing submodel is nil")
		return newSubmodelRepoErrorResponse(nilErr, http.StatusInternalServerError, operation, "GetSubmodelByID"), nil
	}

	existingJSON, existingJSONErr := jsonization.ToJsonable(existingSubmodel)
	if existingJSONErr != nil {
		return newSubmodelRepoErrorResponse(existingJSONErr, http.StatusInternalServerError, operation, "ToJsonableCurrentSubmodel"), nil
	}

	mergedJSON := mergeSubmodelJSON(existingJSON, patchJSON)
	delete(mergedJSON, "submodelElements")
	mergedSubmodel, mergedErr := jsonization.SubmodelFromJsonable(mergedJSON)
	if mergedErr != nil {
		return newSubmodelRepoErrorResponse(mergedErr, http.StatusBadRequest, operation, "InvalidPatchedSubmodel"), nil
	}

	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		if patchErr := s.persistence.SubmodelRepository.PatchSubmodelMetadataInTransaction(ctx, decodedIdentifier, tx, mergedSubmodel); patchErr != nil {
			return patchErr
		}

		descriptor, descriptorErr := s.syncConfig.buildSubmodelDescriptor(mergedSubmodel)
		if descriptorErr != nil {
			return descriptorErr
		}
		if upsertErr := s.persistence.SubmodelRegistry.UpsertSubmodelDescriptorInTransaction(ctx, tx, descriptor); upsertErr != nil {
			return upsertErr
		}

		return s.syncReferencingAASDescriptorsInTransaction(ctx, tx, descriptor, nil, false)
	})
	if err != nil {
		if common.IsErrBadRequest(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		if common.IsErrNotFound(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusNotFound, operation, "SubmodelNotFound"), nil
		}
		if common.IsErrDenied(err) {
			return newSubmodelRepoErrorResponse(err, http.StatusForbidden, operation, "Denied"), nil
		}
		return newSubmodelRepoErrorResponse(err, http.StatusInternalServerError, operation, "PatchSubmodelMetadata"), nil
	}

	return commonmodel.Response(http.StatusNoContent, nil), nil
}

func submodelMetadataToPatchJSON(metadata commonmodel.SubmodelMetadata) (map[string]any, error) {
	rawJSON, marshalErr := json.Marshal(metadata)
	if marshalErr != nil {
		return nil, marshalErr
	}

	result := map[string]any{}
	if unmarshalErr := json.Unmarshal(rawJSON, &result); unmarshalErr != nil {
		return nil, unmarshalErr
	}

	return result, nil
}

func newSubmodelRepoErrorResponse(err error, status int, operation string, info string) commonmodel.ImplResponse {
	return common.NewErrorResponse(err, status, "SMREPO", operation, info)
}

func mergeSubmodelJSON(base map[string]any, patch map[string]any) map[string]any {
	merged := make(map[string]any, len(base))
	for key, value := range base {
		merged[key] = value
	}

	for key, patchValue := range patch {
		if patchValue == nil {
			delete(merged, key)
			continue
		}

		baseValue, baseExists := merged[key]
		baseMap, baseIsMap := baseValue.(map[string]any)
		patchMap, patchIsMap := patchValue.(map[string]any)
		if baseExists && baseIsMap && patchIsMap {
			merged[key] = mergeSubmodelJSON(baseMap, patchMap)
			continue
		}

		merged[key] = patchValue
	}

	return merged
}

func (s *CustomSubmodelRepositoryService) syncReferencingAASDescriptorsInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	submodelDescriptor commonmodel.SubmodelDescriptor,
	referencingAASIDs []string,
	remove bool,
) error {
	if !s.enableReferencingAASDescriptorEmbeddingSync {
		return nil
	}
	if s.persistence == nil {
		return common.NewInternalServerError("AASENV-SMREPO-SYNCAAS-NILPERSISTENCE persistence bundle must not be nil")
	}
	if s.persistence.AASRepository == nil {
		return common.NewInternalServerError("AASENV-SMREPO-SYNCAAS-NILAASREPO AAS repository backend must not be nil")
	}
	if s.persistence.AASRegistry == nil {
		return common.NewInternalServerError("AASENV-SMREPO-SYNCAAS-NILAASREGISTRY AAS registry backend must not be nil")
	}

	if len(referencingAASIDs) == 0 {
		aasIDs, aasLookupErr := s.persistence.AASRepository.ListAASIdentifiersBySubmodelIDInTransaction(ctx, tx, submodelDescriptor.Id)
		if aasLookupErr != nil {
			return aasLookupErr
		}
		referencingAASIDs = aasIDs
	}

	for _, aasID := range referencingAASIDs {
		aasDescriptor, getDescriptorErr := s.persistence.AASRegistry.GetAssetAdministrationShellDescriptorByIDInTransaction(ctx, tx, aasID)
		if getDescriptorErr != nil {
			if common.IsErrNotFound(getDescriptorErr) {
				continue
			}
			return getDescriptorErr
		}

		if remove {
			aasDescriptor.SubmodelDescriptors = removeEmbeddedSubmodelDescriptor(aasDescriptor.SubmodelDescriptors, submodelDescriptor.Id)
		} else {
			aasDescriptor.SubmodelDescriptors = addOrUpdateEmbeddedSubmodelDescriptor(aasDescriptor.SubmodelDescriptors, submodelDescriptor)
		}

		if len(aasDescriptor.Endpoints) == 0 {
			aasDescriptor.Endpoints = s.syncConfig.buildAASDescriptorEndpoints(aasID)
		}

		if upsertErr := s.persistence.AASRegistry.UpsertAdministrationShellDescriptorInTransaction(ctx, tx, aasDescriptor); upsertErr != nil {
			return upsertErr
		}
	}

	return nil
}
