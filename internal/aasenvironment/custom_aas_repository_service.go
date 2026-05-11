package aasenvironment

import (
	"context"
	"database/sql"
	"net/http"

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

// PostAssetAdministrationShell creates a new AAS and synchronizes descriptor writes in the same transaction.
func (s *CustomAASRepositoryService) PostAssetAdministrationShell(ctx context.Context, aas types.IAssetAdministrationShell) (commonmodel.ImplResponse, error) {
	const operation = "PostAssetAdministrationShell"
	if !s.syncConfig.AASRegistryIntegration {
		return s.AssetAdministrationShellRepositoryAPIAPIService.PostAssetAdministrationShell(ctx, aas)
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
		return newAASRepoErrorResponse(err, http.StatusInternalServerError, operation, "CreateAssetAdministrationShell"), err
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
		return newAASRepoErrorResponse(err, http.StatusInternalServerError, operation, "PutAssetAdministrationShellByID"), err
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
		return newAASRepoErrorResponse(err, http.StatusInternalServerError, operation, "DeleteAssetAdministrationShellByID"), err
	}

	return commonmodel.Response(http.StatusNoContent, nil), nil
}

func newAASRepoErrorResponse(err error, status int, operation string, info string) commonmodel.ImplResponse {
	return common.NewErrorResponse(err, status, "AASREPO", operation, info)
}
