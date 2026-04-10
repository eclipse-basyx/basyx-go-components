package aasenvironment

import (
	"context"
	"database/sql"

	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	submodelrepositoryapi "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/api"
)

// CustomSubmodelRepositoryService is a pass-through stub for future combined logic.
type CustomSubmodelRepositoryService struct {
	*submodelrepositoryapi.SubmodelRepositoryAPIAPIService
	persistence *Persistence
}

// NewCustomSubmodelRepositoryService creates a new pass-through submodel repository decorator.
func NewCustomSubmodelRepositoryService(
	base *submodelrepositoryapi.SubmodelRepositoryAPIAPIService,
	persistence *Persistence,
) *CustomSubmodelRepositoryService {
	return &CustomSubmodelRepositoryService{
		SubmodelRepositoryAPIAPIService: base,
		persistence:                     persistence,
	}
}

// ExecuteInTransaction exposes shared transaction execution for future endpoint customizations.
func (s *CustomSubmodelRepositoryService) ExecuteInTransaction(fn func(tx *sql.Tx) error) error {
	if s == nil || s.persistence == nil {
		return common.NewErrBadRequest("AASENV-SMREPO-TX-NILSERVICE service must not be nil")
	}
	return s.persistence.ExecuteInTransaction("AASENV-SMREPO-STARTTX", "AASENV-SMREPO-COMMITTX", fn)
}

// PutSubmodelWithTxForEnvironment upserts a submodel inside an existing transaction.
func (s *CustomSubmodelRepositoryService) PutSubmodelWithTxForEnvironment(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
	submodel types.ISubmodel,
) (bool, error) {
	if s == nil || s.persistence == nil || s.persistence.SubmodelRepository == nil {
		return false, common.NewErrBadRequest("AASENV-SMREPO-PUTSM-NILSERVICE service must not be nil")
	}

	// TODO: Future custom logic can be added here to create submodel descriptors automatically
	return s.persistence.SubmodelRepository.PutSubmodelWithTx(ctx, tx, submodelID, submodel)
}

// GetSubmodelsForEnvironment lists submodels for environment serialization.
func (s *CustomSubmodelRepositoryService) GetSubmodelsForEnvironment(
	ctx context.Context,
	limit int32,
	cursor string,
	submodelIdentifier string,
) ([]types.ISubmodel, string, error) {
	if s == nil || s.persistence == nil || s.persistence.SubmodelRepository == nil {
		return nil, "", common.NewErrBadRequest("AASENV-SMREPO-LISTSM-NILSERVICE service must not be nil")
	}
	return s.persistence.SubmodelRepository.GetSubmodels(ctx, limit, cursor, submodelIdentifier)
}

// GetSubmodelByIDForEnvironment resolves a submodel by identifier.
func (s *CustomSubmodelRepositoryService) GetSubmodelByIDForEnvironment(
	ctx context.Context,
	submodelIdentifier string,
	level string,
	metadataOnly bool,
) (types.ISubmodel, error) {
	if s == nil || s.persistence == nil || s.persistence.SubmodelRepository == nil {
		return nil, common.NewErrBadRequest("AASENV-SMREPO-GETSM-NILSERVICE service must not be nil")
	}
	return s.persistence.SubmodelRepository.GetSubmodelByID(ctx, submodelIdentifier, level, metadataOnly)
}
