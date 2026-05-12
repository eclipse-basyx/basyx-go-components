package aasenvironment

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/types"
	aasrepositoryapi "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// CustomAASRepositoryService is a pass-through stub for future combined logic.
type CustomAASRepositoryService struct {
	*aasrepositoryapi.AssetAdministrationShellRepositoryAPIAPIService
	persistence *Persistence
	syncConfig  RegistrySyncConfig
}

// NewCustomAASRepositoryService creates a new pass-through AAS repository decorator.
func NewCustomAASRepositoryService(
	base *aasrepositoryapi.AssetAdministrationShellRepositoryAPIAPIService,
	persistence *Persistence,
	syncConfig RegistrySyncConfig,
) *CustomAASRepositoryService {
	return &CustomAASRepositoryService{
		AssetAdministrationShellRepositoryAPIAPIService: base,
		persistence: persistence,
		syncConfig:  syncConfig,
	}
}

// ExecuteInTransaction exposes shared transaction execution for future endpoint customizations.
func (s *CustomAASRepositoryService) ExecuteInTransaction(fn func(tx *sql.Tx) error) error {
	if s == nil || s.persistence == nil {
		return common.NewErrBadRequest("AASENV-AASREPO-TX-NILSERVICE service must not be nil")
	}
	return s.persistence.ExecuteInTransaction("AASENV-AASREPO-STARTTX", "AASENV-AASREPO-COMMITTX", fn)
}

func (s *CustomAASRepositoryService) validateSyncDependencies(requireAASRegistry bool, requireSubmodelRepository bool, requireSubmodelRegistry bool) error {
	if s == nil {
		return common.NewInternalServerError("AASENV-AASREPO-CHECKDEPS-NILSERVICE service must not be nil")
	}
	if s.persistence == nil {
		return common.NewInternalServerError("AASENV-AASREPO-CHECKDEPS-NILPERSISTENCE persistence bundle must not be nil")
	}
	if s.persistence.AASRepository == nil {
		return common.NewInternalServerError("AASENV-AASREPO-CHECKDEPS-NILAASREPO AAS repository backend must not be nil")
	}
	if requireAASRegistry && s.persistence.AASRegistry == nil {
		return common.NewInternalServerError("AASENV-AASREPO-CHECKDEPS-NILAASREGISTRY AAS registry backend must not be nil")
	}
	if requireSubmodelRepository && s.persistence.SubmodelRepository == nil {
		return common.NewInternalServerError("AASENV-AASREPO-CHECKDEPS-NILSMREPO Submodel repository backend must not be nil")
	}
	if requireSubmodelRegistry && s.persistence.SubmodelRegistry == nil {
		return common.NewInternalServerError("AASENV-AASREPO-CHECKDEPS-NILSMREGISTRY Submodel registry backend must not be nil")
	}

	return nil
}

// PostAssetAdministrationShell creates a new AAS and synchronizes descriptor writes in the same transaction.
func (s *CustomAASRepositoryService) PostAssetAdministrationShell(ctx context.Context, aas types.IAssetAdministrationShell) (commonmodel.ImplResponse, error) {
	const operation = "PostAssetAdministrationShell"
	if !s.syncConfig.AASRegistryIntegration {
		return s.AssetAdministrationShellRepositoryAPIAPIService.PostAssetAdministrationShell(ctx, aas)
	}
	if dependencyErr := s.validateSyncDependencies(true, false, false); dependencyErr != nil {
		return newAASRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		if err := s.persistence.AASRepository.CreateAssetAdministrationShellInTransaction(ctx, tx, aas); err != nil {
			return err
		}

		descriptor, descriptorErr := s.syncConfig.buildAASDescriptor(aas)
		if descriptorErr != nil {
			return descriptorErr
		}

		return s.persistence.AASRegistry.UpsertAdministrationShellDescriptorInTransaction(ctx, tx, descriptor)
	})
	if err != nil {
		if common.IsErrDenied(err) {
			return newAASRepoErrorResponse(err, http.StatusForbidden, operation, "Forbidden"), nil
		}
		if common.IsErrConflict(err) {
			return newAASRepoErrorResponse(err, http.StatusConflict, operation, "IdConflict"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAASRepoErrorResponse(err, http.StatusBadRequest, operation, "InvalidAssetAdministrationShellData"), nil
		}
		return newAASRepoErrorResponse(err, http.StatusInternalServerError, operation, "CreateAssetAdministrationShell"), nil
	}

	aasJSON, jsonErr := jsonization.ToJsonable(aas)
	if jsonErr != nil {
		return newAASRepoErrorResponse(jsonErr, http.StatusBadRequest, operation, "InvalidAssetAdministrationShellData"), nil
	}
	return commonmodel.Response(http.StatusCreated, aasJSON), nil
}

// PutAssetAdministrationShellById upserts an AAS and synchronizes descriptor writes in the same transaction.
func (s *CustomAASRepositoryService) PutAssetAdministrationShellById(ctx context.Context, aasIdentifier string, assetAdministrationShell types.IAssetAdministrationShell) (commonmodel.ImplResponse, error) {
	const operation = "PutAssetAdministrationShellById"
	if !s.syncConfig.AASRegistryIntegration {
		return s.AssetAdministrationShellRepositoryAPIAPIService.PutAssetAdministrationShellById(ctx, aasIdentifier, assetAdministrationShell)
	}
	if dependencyErr := s.validateSyncDependencies(true, false, false); dependencyErr != nil {
		return newAASRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	decodedIdentifier, decodeErr := common.DecodeString(aasIdentifier)
	if decodeErr != nil {
		return newAASRepoErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedAssetAdministrationShellIdentifier"), nil
	}

	isUpdate := false
	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		updated, putErr := s.persistence.AASRepository.PutAssetAdministrationShellByIDInTransaction(ctx, tx, decodedIdentifier, assetAdministrationShell)
		if putErr != nil {
			return putErr
		}
		isUpdate = updated

		descriptor, descriptorErr := s.syncConfig.buildAASDescriptor(assetAdministrationShell)
		if descriptorErr != nil {
			return descriptorErr
		}
		return s.persistence.AASRegistry.UpsertAdministrationShellDescriptorInTransaction(ctx, tx, descriptor)
	})
	if err != nil {
		if common.IsErrDenied(err) {
			return newAASRepoErrorResponse(err, http.StatusForbidden, operation, "Forbidden"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAASRepoErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		if common.IsErrConflict(err) {
			return newAASRepoErrorResponse(err, http.StatusConflict, operation, "Conflict"), nil
		}
		return newAASRepoErrorResponse(err, http.StatusInternalServerError, operation, "PutAssetAdministrationShellByID"), nil
	}

	if isUpdate {
		return commonmodel.Response(http.StatusNoContent, nil), nil
	}

	aasJSON, jsonErr := jsonization.ToJsonable(assetAdministrationShell)
	if jsonErr != nil {
		return newAASRepoErrorResponse(jsonErr, http.StatusBadRequest, operation, "InvalidAssetAdministrationShellData"), nil
	}
	return commonmodel.Response(http.StatusCreated, aasJSON), nil
}

// DeleteAssetAdministrationShellById deletes an AAS and synchronizes descriptor deletion in the same transaction.
func (s *CustomAASRepositoryService) DeleteAssetAdministrationShellById(ctx context.Context, aasIdentifier string) (commonmodel.ImplResponse, error) {
	const operation = "DeleteAssetAdministrationShellById"
	if !s.syncConfig.AASRegistryIntegration {
		return s.AssetAdministrationShellRepositoryAPIAPIService.DeleteAssetAdministrationShellById(ctx, aasIdentifier)
	}
	if dependencyErr := s.validateSyncDependencies(true, false, false); dependencyErr != nil {
		return newAASRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	decodedIdentifier, decodeErr := common.DecodeString(aasIdentifier)
	if decodeErr != nil {
		return newAASRepoErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedAssetAdministrationShellIdentifier"), nil
	}

	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		if deleteErr := s.persistence.AASRepository.DeleteAssetAdministrationShellByIDInTransaction(ctx, tx, decodedIdentifier); deleteErr != nil {
			return deleteErr
		}
		return s.persistence.AASRegistry.DeleteAssetAdministrationShellDescriptorByIDInTransaction(ctx, tx, decodedIdentifier)
	})
	if err != nil {
		if common.IsErrDenied(err) {
			return newAASRepoErrorResponse(err, http.StatusForbidden, operation, "Forbidden"), nil
		}
		if common.IsErrNotFound(err) {
			return newAASRepoErrorResponse(err, http.StatusNotFound, operation, "AssetAdministrationShellNotFound"), nil
		}
		return newAASRepoErrorResponse(err, http.StatusInternalServerError, operation, "DeleteAssetAdministrationShellByID"), nil
	}

	return commonmodel.Response(http.StatusNoContent, nil), nil
}

// PutAssetInformationAasRepository updates AAS asset information and synchronizes descriptor writes in the same transaction.
func (s *CustomAASRepositoryService) PutAssetInformationAasRepository(ctx context.Context, aasIdentifier string, assetInformation types.IAssetInformation) (commonmodel.ImplResponse, error) {
	const operation = "PutAssetInformationAasRepository"
	if !s.syncConfig.AASRegistryIntegration {
		return s.AssetAdministrationShellRepositoryAPIAPIService.PutAssetInformationAasRepository(ctx, aasIdentifier, assetInformation)
	}
	if dependencyErr := s.validateSyncDependencies(true, false, false); dependencyErr != nil {
		return newAASRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	decodedIdentifier, decodeErr := common.DecodeString(aasIdentifier)
	if decodeErr != nil {
		return newAASRepoErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedAssetAdministrationShellIdentifier"), nil
	}

	existingAASJSON, getErr := s.persistence.AASRepository.GetAssetAdministrationShellByID(ctx, decodedIdentifier)
	if getErr != nil {
		if common.IsErrNotFound(getErr) {
			return newAASRepoErrorResponse(getErr, http.StatusNotFound, operation, "AssetAdministrationShellNotFound"), nil
		}
		return newAASRepoErrorResponse(getErr, http.StatusInternalServerError, operation, "GetAssetAdministrationShellByID"), nil
	}

	descriptor, descriptorGetErr := s.persistence.AASRegistry.GetAssetAdministrationShellDescriptorByID(ctx, decodedIdentifier)
	if descriptorGetErr != nil && !common.IsErrNotFound(descriptorGetErr) {
		return newAASRepoErrorResponse(descriptorGetErr, http.StatusInternalServerError, operation, "GetAssetAdministrationShellDescriptorByID"), nil
	}
	if common.IsErrNotFound(descriptorGetErr) {
		descriptor = commonmodel.AssetAdministrationShellDescriptor{
			Id:        decodedIdentifier,
			Endpoints: s.syncConfig.buildAASDescriptorEndpoints(decodedIdentifier),
		}
		if idShort, ok := existingAASJSON["idShort"].(string); ok {
			descriptor.IdShort = idShort
		}
	}
	descriptor.AssetKind = assetKindPointer(assetInformation.AssetKind())
	descriptor.AssetType = readOptionalString(assetInformation.AssetType())
	descriptor.GlobalAssetId = readOptionalString(assetInformation.GlobalAssetID())
	descriptor.SpecificAssetIds = assetInformation.SpecificAssetIDs()
	if len(descriptor.Endpoints) == 0 {
		descriptor.Endpoints = s.syncConfig.buildAASDescriptorEndpoints(decodedIdentifier)
	}

	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		if err := s.persistence.AASRepository.PutAssetInformationByAASIDInTransaction(ctx, tx, decodedIdentifier, assetInformation); err != nil {
			return err
		}
		return s.persistence.AASRegistry.UpsertAdministrationShellDescriptorInTransaction(ctx, tx, descriptor)
	})
	if err != nil {
		if common.IsErrDenied(err) {
			return newAASRepoErrorResponse(err, http.StatusForbidden, operation, "Forbidden"), nil
		}
		if common.IsErrNotFound(err) {
			return newAASRepoErrorResponse(err, http.StatusNotFound, operation, "AssetAdministrationShellNotFound"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAASRepoErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAASRepoErrorResponse(err, http.StatusInternalServerError, operation, "PutAssetInformationByAASID"), nil
	}

	return commonmodel.Response(http.StatusNoContent, nil), nil
}

// PostSubmodelReferenceAasRepository creates a submodel reference and synchronizes embedded descriptors.
func (s *CustomAASRepositoryService) PostSubmodelReferenceAasRepository(ctx context.Context, aasIdentifier string, reference types.IReference) (commonmodel.ImplResponse, error) {
	const operation = "PostSubmodelReferenceAasRepository"
	if !s.syncConfig.AASRegistryIntegration {
		return s.AssetAdministrationShellRepositoryAPIAPIService.PostSubmodelReferenceAasRepository(ctx, aasIdentifier, reference)
	}
	if dependencyErr := s.validateSyncDependencies(true, true, false); dependencyErr != nil {
		return newAASRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	decodedAASIdentifier, decodeErr := common.DecodeString(aasIdentifier)
	if decodeErr != nil {
		return newAASRepoErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedAssetAdministrationShellIdentifier"), nil
	}

	embeddedDescriptor, hasSubmodelReference, descriptorErr := s.buildSubmodelDescriptorForReference(ctx, reference)
	if descriptorErr != nil {
		return newAASRepoErrorResponse(descriptorErr, http.StatusInternalServerError, operation, "BuildSubmodelDescriptor"), nil
	}

	aasDescriptor, hasAASDescriptor, getDescriptorErr := s.ensureAASDescriptorForSubmodelSync(ctx, decodedAASIdentifier)
	if getDescriptorErr != nil && !common.IsErrNotFound(getDescriptorErr) {
		return newAASRepoErrorResponse(getDescriptorErr, http.StatusInternalServerError, operation, "GetAssetAdministrationShellDescriptorByID"), nil
	}

	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		if createErr := s.persistence.AASRepository.CreateSubmodelReferenceInAssetAdministrationShellInTransaction(ctx, tx, decodedAASIdentifier, reference); createErr != nil {
			return createErr
		}

		if hasAASDescriptor && hasSubmodelReference {
			aasDescriptor.SubmodelDescriptors = addOrUpdateEmbeddedSubmodelDescriptor(aasDescriptor.SubmodelDescriptors, embeddedDescriptor)
			return s.persistence.AASRegistry.UpsertAdministrationShellDescriptorInTransaction(ctx, tx, aasDescriptor)
		}

		return nil
	})
	if err != nil {
		if common.IsErrDenied(err) {
			return newAASRepoErrorResponse(err, http.StatusForbidden, operation, "Forbidden"), nil
		}
		if common.IsErrNotFound(err) {
			return newAASRepoErrorResponse(err, http.StatusNotFound, operation, "AssetAdministrationShellNotFound"), nil
		}
		if common.IsErrConflict(err) {
			return newAASRepoErrorResponse(err, http.StatusConflict, operation, "Conflict"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAASRepoErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAASRepoErrorResponse(err, http.StatusInternalServerError, operation, "CreateSubmodelReferenceInAssetAdministrationShell"), nil
	}

	referenceJSON, jsonErr := jsonization.ToJsonable(reference)
	if jsonErr != nil {
		return newAASRepoErrorResponse(jsonErr, http.StatusBadRequest, operation, "InvalidReferenceData"), nil
	}

	return commonmodel.Response(http.StatusCreated, referenceJSON), nil
}

// DeleteSubmodelReferenceAasRepository deletes a submodel reference and synchronizes embedded descriptors.
func (s *CustomAASRepositoryService) DeleteSubmodelReferenceAasRepository(ctx context.Context, aasIdentifier string, submodelIdentifier string) (commonmodel.ImplResponse, error) {
	const operation = "DeleteSubmodelReferenceAasRepository"
	if !s.syncConfig.AASRegistryIntegration {
		return s.AssetAdministrationShellRepositoryAPIAPIService.DeleteSubmodelReferenceAasRepository(ctx, aasIdentifier, submodelIdentifier)
	}
	if dependencyErr := s.validateSyncDependencies(true, false, false); dependencyErr != nil {
		return newAASRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	decodedAASIdentifier, decodeAASErr := common.DecodeString(aasIdentifier)
	if decodeAASErr != nil {
		return newAASRepoErrorResponse(decodeAASErr, http.StatusBadRequest, operation, "MalformedAssetAdministrationShellIdentifier"), nil
	}

	decodedSubmodelIdentifier, decodeSubmodelErr := common.DecodeString(submodelIdentifier)
	if decodeSubmodelErr != nil {
		return newAASRepoErrorResponse(decodeSubmodelErr, http.StatusBadRequest, operation, "MalformedSubmodelIdentifier"), nil
	}

	aasDescriptor, hasAASDescriptor, getDescriptorErr := s.ensureAASDescriptorForSubmodelSync(ctx, decodedAASIdentifier)
	if getDescriptorErr != nil && !common.IsErrNotFound(getDescriptorErr) {
		return newAASRepoErrorResponse(getDescriptorErr, http.StatusInternalServerError, operation, "GetAssetAdministrationShellDescriptorByID"), nil
	}

	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		if deleteErr := s.persistence.AASRepository.DeleteSubmodelReferenceInAssetAdministrationShellInTransaction(ctx, tx, decodedAASIdentifier, decodedSubmodelIdentifier); deleteErr != nil {
			return deleteErr
		}

		if hasAASDescriptor {
			aasDescriptor.SubmodelDescriptors = removeEmbeddedSubmodelDescriptor(aasDescriptor.SubmodelDescriptors, decodedSubmodelIdentifier)
			return s.persistence.AASRegistry.UpsertAdministrationShellDescriptorInTransaction(ctx, tx, aasDescriptor)
		}

		return nil
	})
	if err != nil {
		if common.IsErrDenied(err) {
			return newAASRepoErrorResponse(err, http.StatusForbidden, operation, "Forbidden"), nil
		}
		if common.IsErrNotFound(err) {
			return newAASRepoErrorResponse(err, http.StatusNotFound, operation, "SubmodelReferenceNotFound"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAASRepoErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAASRepoErrorResponse(err, http.StatusInternalServerError, operation, "DeleteSubmodelReferenceInAssetAdministrationShell"), nil
	}

	return commonmodel.Response(http.StatusNoContent, nil), nil
}

// PutSubmodelByIdAasRepository creates or updates a submodel through the superpath and synchronizes descriptors.
func (s *CustomAASRepositoryService) PutSubmodelByIdAasRepository(ctx context.Context, aasIdentifier string, submodelIdentifier string, submodel types.ISubmodel) (commonmodel.ImplResponse, error) {
	const operation = "PutSubmodelByIdAasRepository"
	if !s.syncConfig.AASRegistryIntegration && !s.syncConfig.SubmodelRegistryIntegration {
		return s.AssetAdministrationShellRepositoryAPIAPIService.PutSubmodelByIdAasRepository(ctx, aasIdentifier, submodelIdentifier, submodel)
	}
	if dependencyErr := s.validateSyncDependencies(s.syncConfig.AASRegistryIntegration, true, s.syncConfig.SubmodelRegistryIntegration); dependencyErr != nil {
		return newAASRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	decodedAASIdentifier, decodeAASErr := common.DecodeString(aasIdentifier)
	if decodeAASErr != nil {
		return newAASRepoErrorResponse(decodeAASErr, http.StatusBadRequest, operation, "MalformedAssetAdministrationShellIdentifier"), nil
	}

	decodedSubmodelIdentifier, decodeSubmodelErr := common.DecodeString(submodelIdentifier)
	if decodeSubmodelErr != nil {
		return newAASRepoErrorResponse(decodeSubmodelErr, http.StatusBadRequest, operation, "MalformedSubmodelIdentifier"), nil
	}

	if decodedSubmodelIdentifier != submodel.ID() {
		return newAASRepoErrorResponse(errors.New("submodel ID in path and body do not match"), http.StatusBadRequest, operation, "IdMismatch"), nil
	}

	if _, aasLookupErr := s.persistence.AASRepository.GetAssetAdministrationShellByID(ctx, decodedAASIdentifier); aasLookupErr != nil {
		if common.IsErrDenied(aasLookupErr) {
			return newAASRepoErrorResponse(aasLookupErr, http.StatusForbidden, operation, "Forbidden"), nil
		}
		if common.IsErrNotFound(aasLookupErr) {
			return newAASRepoErrorResponse(aasLookupErr, http.StatusNotFound, operation, "AssetAdministrationShellNotFound"), nil
		}
		if common.IsErrBadRequest(aasLookupErr) {
			return newAASRepoErrorResponse(aasLookupErr, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAASRepoErrorResponse(aasLookupErr, http.StatusInternalServerError, operation, "GetAssetAdministrationShellByID"), nil
	}

	submodelDescriptor, descriptorErr := s.syncConfig.buildSubmodelDescriptor(submodel)
	if descriptorErr != nil {
		return newAASRepoErrorResponse(descriptorErr, http.StatusInternalServerError, operation, "BuildSubmodelDescriptor"), nil
	}

	aasDescriptor, hasAASDescriptor, getDescriptorErr := s.ensureAASDescriptorForSubmodelSync(ctx, decodedAASIdentifier)
	if getDescriptorErr != nil && !common.IsErrNotFound(getDescriptorErr) {
		return newAASRepoErrorResponse(getDescriptorErr, http.StatusInternalServerError, operation, "GetAssetAdministrationShellDescriptorByID"), nil
	}
	if hasAASDescriptor {
		aasDescriptor.SubmodelDescriptors = addOrUpdateEmbeddedSubmodelDescriptor(aasDescriptor.SubmodelDescriptors, submodelDescriptor)
	}

	isUpdate := false
	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		updated, putErr := s.persistence.SubmodelRepository.PutSubmodelInTransaction(ctx, tx, decodedSubmodelIdentifier, submodel)
		if putErr != nil {
			return putErr
		}
		isUpdate = updated

		submodelReference := types.NewReference(
			types.ReferenceTypesModelReference,
			[]types.IKey{types.NewKey(types.KeyTypesSubmodel, decodedSubmodelIdentifier)},
		)
		createReferenceErr := s.persistence.AASRepository.CreateSubmodelReferenceInAssetAdministrationShellInTransaction(ctx, tx, decodedAASIdentifier, submodelReference)
		if createReferenceErr != nil && !common.IsErrConflict(createReferenceErr) {
			return createReferenceErr
		}

		if s.syncConfig.SubmodelRegistryIntegration {
			if upsertErr := s.persistence.SubmodelRegistry.UpsertSubmodelDescriptorInTransaction(ctx, tx, submodelDescriptor); upsertErr != nil {
				return upsertErr
			}
		}

		if s.syncConfig.AASRegistryIntegration && hasAASDescriptor {
			return s.persistence.AASRegistry.UpsertAdministrationShellDescriptorInTransaction(ctx, tx, aasDescriptor)
		}

		return nil
	})
	if err != nil {
		if common.IsErrDenied(err) {
			return newAASRepoErrorResponse(err, http.StatusForbidden, operation, "Forbidden"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAASRepoErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		if common.IsErrConflict(err) {
			return newAASRepoErrorResponse(err, http.StatusConflict, operation, "Conflict"), nil
		}
		if common.IsErrNotFound(err) {
			return newAASRepoErrorResponse(err, http.StatusNotFound, operation, "SubmodelNotFound"), nil
		}
		return newAASRepoErrorResponse(err, http.StatusInternalServerError, operation, "PutSubmodel"), nil
	}

	if isUpdate {
		return commonmodel.Response(http.StatusNoContent, nil), nil
	}

	submodelJSON, jsonErr := jsonization.ToJsonable(submodel)
	if jsonErr != nil {
		return newAASRepoErrorResponse(jsonErr, http.StatusBadRequest, operation, "InvalidSubmodelData"), nil
	}

	return commonmodel.Response(http.StatusCreated, submodelJSON), nil
}

// DeleteSubmodelByIdAasRepository deletes a submodel through the superpath and synchronizes descriptors.
func (s *CustomAASRepositoryService) DeleteSubmodelByIdAasRepository(ctx context.Context, aasIdentifier string, submodelIdentifier string) (commonmodel.ImplResponse, error) {
	const operation = "DeleteSubmodelByIdAasRepository"
	if !s.syncConfig.AASRegistryIntegration && !s.syncConfig.SubmodelRegistryIntegration {
		return s.AssetAdministrationShellRepositoryAPIAPIService.DeleteSubmodelByIdAasRepository(ctx, aasIdentifier, submodelIdentifier)
	}
	if dependencyErr := s.validateSyncDependencies(s.syncConfig.AASRegistryIntegration, true, s.syncConfig.SubmodelRegistryIntegration); dependencyErr != nil {
		return newAASRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	decodedAASIdentifier, decodeAASErr := common.DecodeString(aasIdentifier)
	if decodeAASErr != nil {
		return newAASRepoErrorResponse(decodeAASErr, http.StatusBadRequest, operation, "MalformedAssetAdministrationShellIdentifier"), nil
	}

	decodedSubmodelIdentifier, decodeSubmodelErr := common.DecodeString(submodelIdentifier)
	if decodeSubmodelErr != nil {
		return newAASRepoErrorResponse(decodeSubmodelErr, http.StatusBadRequest, operation, "MalformedSubmodelIdentifier"), nil
	}

	if _, aasLookupErr := s.persistence.AASRepository.GetAssetAdministrationShellByID(ctx, decodedAASIdentifier); aasLookupErr != nil {
		if common.IsErrDenied(aasLookupErr) {
			return newAASRepoErrorResponse(aasLookupErr, http.StatusForbidden, operation, "Forbidden"), nil
		}
		if common.IsErrNotFound(aasLookupErr) {
			return newAASRepoErrorResponse(aasLookupErr, http.StatusNotFound, operation, "AssetAdministrationShellNotFound"), nil
		}
		if common.IsErrBadRequest(aasLookupErr) {
			return newAASRepoErrorResponse(aasLookupErr, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAASRepoErrorResponse(aasLookupErr, http.StatusInternalServerError, operation, "GetAssetAdministrationShellByID"), nil
	}

	aasDescriptor, hasAASDescriptor, getDescriptorErr := s.ensureAASDescriptorForSubmodelSync(ctx, decodedAASIdentifier)
	if getDescriptorErr != nil && !common.IsErrNotFound(getDescriptorErr) {
		return newAASRepoErrorResponse(getDescriptorErr, http.StatusInternalServerError, operation, "GetAssetAdministrationShellDescriptorByID"), nil
	}
	if hasAASDescriptor {
		aasDescriptor.SubmodelDescriptors = removeEmbeddedSubmodelDescriptor(aasDescriptor.SubmodelDescriptors, decodedSubmodelIdentifier)
	}

	err := s.ExecuteInTransaction(func(tx *sql.Tx) error {
		if checkErr := s.persistence.AASRepository.CheckIfSubmodelReferenceExistsInAssetAdministrationShellInTransaction(tx, decodedAASIdentifier, decodedSubmodelIdentifier); checkErr != nil {
			return checkErr
		}

		if deleteRefErr := s.persistence.AASRepository.DeleteSubmodelReferenceInAssetAdministrationShellInTransaction(ctx, tx, decodedAASIdentifier, decodedSubmodelIdentifier); deleteRefErr != nil {
			return deleteRefErr
		}

		if deleteSubmodelErr := s.persistence.SubmodelRepository.DeleteSubmodelInTransaction(ctx, tx, decodedSubmodelIdentifier); deleteSubmodelErr != nil {
			return deleteSubmodelErr
		}

		if s.syncConfig.SubmodelRegistryIntegration {
			if deleteDescriptorErr := s.persistence.SubmodelRegistry.DeleteSubmodelDescriptorByIDInTransaction(ctx, tx, decodedSubmodelIdentifier); deleteDescriptorErr != nil && !common.IsErrNotFound(deleteDescriptorErr) {
				return deleteDescriptorErr
			}
		}

		if s.syncConfig.AASRegistryIntegration && hasAASDescriptor {
			return s.persistence.AASRegistry.UpsertAdministrationShellDescriptorInTransaction(ctx, tx, aasDescriptor)
		}

		return nil
	})
	if err != nil {
		if common.IsErrDenied(err) {
			return newAASRepoErrorResponse(err, http.StatusForbidden, operation, "Forbidden"), nil
		}
		if common.IsErrNotFound(err) || errors.Is(err, sql.ErrNoRows) {
			return newAASRepoErrorResponse(err, http.StatusNotFound, operation, "SubmodelNotFound"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAASRepoErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAASRepoErrorResponse(err, http.StatusInternalServerError, operation, "DeleteSubmodel"), nil
	}

	return commonmodel.Response(http.StatusNoContent, nil), nil
}

// PatchSubmodelAasRepository updates a submodel through the superpath and synchronizes descriptors.
func (s *CustomAASRepositoryService) PatchSubmodelAasRepository(ctx context.Context, aasIdentifier string, submodelIdentifier string, submodel types.ISubmodel, level string) (commonmodel.ImplResponse, error) {
	const operation = "PatchSubmodelAasRepository"
	if !s.syncConfig.AASRegistryIntegration && !s.syncConfig.SubmodelRegistryIntegration {
		return s.AssetAdministrationShellRepositoryAPIAPIService.PatchSubmodelAasRepository(ctx, aasIdentifier, submodelIdentifier, submodel, level)
	}
	if dependencyErr := s.validateSyncDependencies(s.syncConfig.AASRegistryIntegration, true, s.syncConfig.SubmodelRegistryIntegration); dependencyErr != nil {
		return newAASRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	response, patchErr := s.AssetAdministrationShellRepositoryAPIAPIService.PatchSubmodelAasRepository(ctx, aasIdentifier, submodelIdentifier, submodel, level)
	if patchErr != nil {
		return response, patchErr
	}
	if response.Code != http.StatusNoContent {
		return response, nil
	}

	decodedAASIdentifier, decodeAASErr := common.DecodeString(aasIdentifier)
	if decodeAASErr != nil {
		return newAASRepoErrorResponse(decodeAASErr, http.StatusBadRequest, operation, "MalformedAssetAdministrationShellIdentifier"), nil
	}
	decodedSubmodelIdentifier, decodeSubmodelErr := common.DecodeString(submodelIdentifier)
	if decodeSubmodelErr != nil {
		return newAASRepoErrorResponse(decodeSubmodelErr, http.StatusBadRequest, operation, "MalformedSubmodelIdentifier"), nil
	}

	if syncErr := s.syncUpdatedSubmodelDescriptors(ctx, decodedAASIdentifier, decodedSubmodelIdentifier); syncErr != nil {
		if common.IsErrNotFound(syncErr) {
			return newAASRepoErrorResponse(syncErr, http.StatusNotFound, operation, "SubmodelNotFound"), nil
		}
		if common.IsErrBadRequest(syncErr) {
			return newAASRepoErrorResponse(syncErr, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAASRepoErrorResponse(syncErr, http.StatusInternalServerError, operation, "SyncDescriptor"), nil
	}

	return response, nil
}

// PatchSubmodelByIdMetadataAasRepository updates submodel metadata through the superpath and synchronizes descriptors.
func (s *CustomAASRepositoryService) PatchSubmodelByIdMetadataAasRepository(ctx context.Context, aasIdentifier string, submodelIdentifier string, submodelMetadata commonmodel.SubmodelMetadata) (commonmodel.ImplResponse, error) {
	const operation = "PatchSubmodelByIdMetadataAasRepository"
	if !s.syncConfig.AASRegistryIntegration && !s.syncConfig.SubmodelRegistryIntegration {
		return s.AssetAdministrationShellRepositoryAPIAPIService.PatchSubmodelByIdMetadataAasRepository(ctx, aasIdentifier, submodelIdentifier, submodelMetadata)
	}
	if dependencyErr := s.validateSyncDependencies(s.syncConfig.AASRegistryIntegration, true, s.syncConfig.SubmodelRegistryIntegration); dependencyErr != nil {
		return newAASRepoErrorResponse(dependencyErr, http.StatusInternalServerError, operation, "ValidateDependencies"), nil
	}

	response, patchErr := s.AssetAdministrationShellRepositoryAPIAPIService.PatchSubmodelByIdMetadataAasRepository(ctx, aasIdentifier, submodelIdentifier, submodelMetadata)
	if patchErr != nil {
		return response, patchErr
	}
	if response.Code != http.StatusNoContent {
		return response, nil
	}

	decodedAASIdentifier, decodeAASErr := common.DecodeString(aasIdentifier)
	if decodeAASErr != nil {
		return newAASRepoErrorResponse(decodeAASErr, http.StatusBadRequest, operation, "MalformedAssetAdministrationShellIdentifier"), nil
	}
	decodedSubmodelIdentifier, decodeSubmodelErr := common.DecodeString(submodelIdentifier)
	if decodeSubmodelErr != nil {
		return newAASRepoErrorResponse(decodeSubmodelErr, http.StatusBadRequest, operation, "MalformedSubmodelIdentifier"), nil
	}

	if syncErr := s.syncUpdatedSubmodelDescriptors(ctx, decodedAASIdentifier, decodedSubmodelIdentifier); syncErr != nil {
		if common.IsErrNotFound(syncErr) {
			return newAASRepoErrorResponse(syncErr, http.StatusNotFound, operation, "SubmodelNotFound"), nil
		}
		if common.IsErrBadRequest(syncErr) {
			return newAASRepoErrorResponse(syncErr, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAASRepoErrorResponse(syncErr, http.StatusInternalServerError, operation, "SyncDescriptor"), nil
	}

	return response, nil
}

func (s *CustomAASRepositoryService) syncUpdatedSubmodelDescriptors(ctx context.Context, aasID string, submodelID string) error {
	if dependencyErr := s.validateSyncDependencies(s.syncConfig.AASRegistryIntegration, true, s.syncConfig.SubmodelRegistryIntegration); dependencyErr != nil {
		return dependencyErr
	}

	submodel, getSubmodelErr := s.persistence.SubmodelRepository.GetSubmodelByID(ctx, submodelID, "core", true)
	if getSubmodelErr != nil {
		return getSubmodelErr
	}

	submodelDescriptor, descriptorErr := s.syncConfig.buildSubmodelDescriptor(submodel)
	if descriptorErr != nil {
		return descriptorErr
	}

	aasDescriptor, hasAASDescriptor, getDescriptorErr := s.ensureAASDescriptorForSubmodelSync(ctx, aasID)
	if getDescriptorErr != nil {
		if common.IsErrNotFound(getDescriptorErr) {
			return nil
		}
		return getDescriptorErr
	}
	if hasAASDescriptor {
		aasDescriptor.SubmodelDescriptors = addOrUpdateEmbeddedSubmodelDescriptor(aasDescriptor.SubmodelDescriptors, submodelDescriptor)
	}

	return s.ExecuteInTransaction(func(tx *sql.Tx) error {
		if s.syncConfig.SubmodelRegistryIntegration {
			if upsertErr := s.persistence.SubmodelRegistry.UpsertSubmodelDescriptorInTransaction(ctx, tx, submodelDescriptor); upsertErr != nil {
				return upsertErr
			}
		}

		if s.syncConfig.AASRegistryIntegration && hasAASDescriptor {
			return s.persistence.AASRegistry.UpsertAdministrationShellDescriptorInTransaction(ctx, tx, aasDescriptor)
		}

		return nil
	})
}

func (s *CustomAASRepositoryService) buildSubmodelDescriptorForReference(ctx context.Context, reference types.IReference) (commonmodel.SubmodelDescriptor, bool, error) {
	if dependencyErr := s.validateSyncDependencies(false, true, false); dependencyErr != nil {
		return commonmodel.SubmodelDescriptor{}, false, dependencyErr
	}

	submodelID, hasSubmodelReference := extractReferencedSubmodelIdentifier(reference)
	if !hasSubmodelReference {
		return commonmodel.SubmodelDescriptor{}, false, nil
	}

	submodel, getSubmodelErr := s.persistence.SubmodelRepository.GetSubmodelByID(ctx, submodelID, "core", true)
	if getSubmodelErr == nil {
		descriptor, descriptorErr := s.syncConfig.buildSubmodelDescriptor(submodel)
		return descriptor, true, descriptorErr
	}
	if !common.IsErrNotFound(getSubmodelErr) && !errors.Is(getSubmodelErr, sql.ErrNoRows) {
		return commonmodel.SubmodelDescriptor{}, false, getSubmodelErr
	}

	return commonmodel.SubmodelDescriptor{
		Id:        submodelID,
		Endpoints: s.syncConfig.buildSubmodelDescriptorEndpoints(submodelID),
	}, true, nil
}

func (s *CustomAASRepositoryService) ensureAASDescriptorForSubmodelSync(ctx context.Context, aasID string) (commonmodel.AssetAdministrationShellDescriptor, bool, error) {
	if dependencyErr := s.validateSyncDependencies(true, false, false); dependencyErr != nil {
		return commonmodel.AssetAdministrationShellDescriptor{}, false, dependencyErr
	}

	descriptor, getDescriptorErr := s.persistence.AASRegistry.GetAssetAdministrationShellDescriptorByID(ctx, aasID)
	if getDescriptorErr == nil {
		if len(descriptor.Endpoints) == 0 {
			descriptor.Endpoints = s.syncConfig.buildAASDescriptorEndpoints(aasID)
		}
		return descriptor, true, nil
	}
	if !common.IsErrNotFound(getDescriptorErr) {
		return commonmodel.AssetAdministrationShellDescriptor{}, false, getDescriptorErr
	}

	existingAASJSON, getAASErr := s.persistence.AASRepository.GetAssetAdministrationShellByID(ctx, aasID)
	if getAASErr != nil {
		return commonmodel.AssetAdministrationShellDescriptor{}, false, getAASErr
	}

	descriptor = commonmodel.AssetAdministrationShellDescriptor{
		Id:        aasID,
		Endpoints: s.syncConfig.buildAASDescriptorEndpoints(aasID),
	}
	if idShort, ok := existingAASJSON["idShort"].(string); ok {
		descriptor.IdShort = idShort
	}

	return descriptor, true, nil
}

func extractReferencedSubmodelIdentifier(reference types.IReference) (string, bool) {
	if reference == nil {
		return "", false
	}

	for _, key := range reference.Keys() {
		if key == nil {
			continue
		}

		if key.Type() != types.KeyTypesSubmodel {
			continue
		}

		submodelID := strings.TrimSpace(key.Value())
		if submodelID == "" {
			continue
		}

		return submodelID, true
	}

	return "", false
}

func addOrUpdateEmbeddedSubmodelDescriptor(descriptors []commonmodel.SubmodelDescriptor, descriptor commonmodel.SubmodelDescriptor) []commonmodel.SubmodelDescriptor {
	if len(descriptors) == 0 {
		return []commonmodel.SubmodelDescriptor{descriptor}
	}

	for index, current := range descriptors {
		if current.Id != descriptor.Id {
			continue
		}

		descriptors[index] = descriptor
		return descriptors
	}

	return append(descriptors, descriptor)
}

func removeEmbeddedSubmodelDescriptor(descriptors []commonmodel.SubmodelDescriptor, submodelID string) []commonmodel.SubmodelDescriptor {
	if len(descriptors) == 0 {
		return descriptors
	}

	filtered := make([]commonmodel.SubmodelDescriptor, 0, len(descriptors))
	for _, current := range descriptors {
		if current.Id == submodelID {
			continue
		}
		filtered = append(filtered, current)
	}

	return filtered
}

func newAASRepoErrorResponse(err error, status int, operation string, info string) commonmodel.ImplResponse {
	return common.NewErrorResponse(err, status, "AASREPO", operation, info)
}
