package aasenvironment

import (
	"context"
	"database/sql"

	"github.com/aas-core-works/aas-core3.1-golang/types"
	aasrepositoryapi "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// CustomAASRepositoryService is a pass-through stub for future combined logic.
type CustomAASRepositoryService struct {
	*aasrepositoryapi.AssetAdministrationShellRepositoryAPIAPIService
	persistence *Persistence
}

// NewCustomAASRepositoryService creates a new pass-through AAS repository decorator.
func NewCustomAASRepositoryService(
	base *aasrepositoryapi.AssetAdministrationShellRepositoryAPIAPIService,
	persistence *Persistence,
) *CustomAASRepositoryService {
	return &CustomAASRepositoryService{
		AssetAdministrationShellRepositoryAPIAPIService: base,
		persistence: persistence,
	}
}

// ExecuteInTransaction exposes shared transaction execution for future endpoint customizations.
func (s *CustomAASRepositoryService) ExecuteInTransaction(fn func(tx *sql.Tx) error) error {
	if s == nil || s.persistence == nil {
		return common.NewErrBadRequest("AASENV-AASREPO-TX-NILSERVICE service must not be nil")
	}
	return s.persistence.ExecuteInTransaction("AASENV-AASREPO-STARTTX", "AASENV-AASREPO-COMMITTX", fn)
}

// PutAssetAdministrationShellByIDWithTxForEnvironment upserts an AAS inside an existing transaction.
func (s *CustomAASRepositoryService) PutAssetAdministrationShellByIDWithTxForEnvironment(
	ctx context.Context,
	tx *sql.Tx,
	aasIdentifier string,
	aas types.IAssetAdministrationShell,
) (bool, error) {
	if s == nil || s.persistence == nil || s.persistence.AASRepository == nil {
		return false, common.NewErrBadRequest("AASENV-AASREPO-PUTAAS-NILSERVICE service must not be nil")
	}

	// TODO: Future custom logic can be added here to create AAS descriptors automatically
	return s.persistence.AASRepository.PutAssetAdministrationShellByIDWithTx(ctx, tx, aasIdentifier, aas)
}

// GetAssetAdministrationShellsForEnvironment lists AAS payloads for environment serialization.
func (s *CustomAASRepositoryService) GetAssetAdministrationShellsForEnvironment(
	ctx context.Context,
	limit int32,
	cursor string,
	idShort string,
	assetIDs []string,
) ([]map[string]any, string, error) {
	if s == nil || s.persistence == nil || s.persistence.AASRepository == nil {
		return nil, "", common.NewErrBadRequest("AASENV-AASREPO-LISTAAS-NILSERVICE service must not be nil")
	}
	return s.persistence.AASRepository.GetAssetAdministrationShells(ctx, limit, cursor, idShort, assetIDs)
}

// GetAssetAdministrationShellByIDForEnvironment resolves a single AAS payload by identifier.
func (s *CustomAASRepositoryService) GetAssetAdministrationShellByIDForEnvironment(
	ctx context.Context,
	aasIdentifier string,
) (map[string]any, error) {
	if s == nil || s.persistence == nil || s.persistence.AASRepository == nil {
		return nil, common.NewErrBadRequest("AASENV-AASREPO-GETAAS-NILSERVICE service must not be nil")
	}
	return s.persistence.AASRepository.GetAssetAdministrationShellByID(ctx, aasIdentifier)
}
