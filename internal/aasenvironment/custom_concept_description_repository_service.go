package aasenvironment

import (
	"context"
	"database/sql"

	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	cdrapi "github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/api"
)

// CustomConceptDescriptionRepositoryService is a pass-through stub for future combined logic.
type CustomConceptDescriptionRepositoryService struct {
	*cdrapi.ConceptDescriptionRepositoryAPIAPIService
	persistence *Persistence
}

// NewCustomConceptDescriptionRepositoryService creates a new pass-through concept description repository decorator.
func NewCustomConceptDescriptionRepositoryService(
	base *cdrapi.ConceptDescriptionRepositoryAPIAPIService,
	persistence *Persistence,
) *CustomConceptDescriptionRepositoryService {
	return &CustomConceptDescriptionRepositoryService{
		ConceptDescriptionRepositoryAPIAPIService: base,
		persistence: persistence,
	}
}

// ExecuteInTransaction exposes shared transaction execution for future endpoint customizations.
func (s *CustomConceptDescriptionRepositoryService) ExecuteInTransaction(fn func(tx *sql.Tx) error) error {
	if s == nil || s.persistence == nil {
		return common.NewErrBadRequest("AASENV-CDREPO-TX-NILSERVICE service must not be nil")
	}
	return s.persistence.ExecuteInTransaction("AASENV-CDREPO-STARTTX", "AASENV-CDREPO-COMMITTX", fn)
}

// PutConceptDescriptionWithTxForEnvironment upserts a concept description inside an existing transaction.
func (s *CustomConceptDescriptionRepositoryService) PutConceptDescriptionWithTxForEnvironment(
	ctx context.Context,
	tx *sql.Tx,
	id string,
	cd types.IConceptDescription,
) error {
	if s == nil || s.persistence == nil || s.persistence.ConceptDescriptionRepository == nil {
		return common.NewErrBadRequest("AASENV-CDREPO-PUTCD-NILSERVICE service must not be nil")
	}
	return s.persistence.ConceptDescriptionRepository.PutConceptDescriptionWithTx(ctx, tx, id, cd)
}

// GetConceptDescriptionsForEnvironment lists concept descriptions for environment serialization.
func (s *CustomConceptDescriptionRepositoryService) GetConceptDescriptionsForEnvironment(
	ctx context.Context,
	idShort *string,
	isCaseOf *string,
	dataSpecificationRef *string,
	limit uint,
	cursor *string,
) ([]types.IConceptDescription, string, error) {
	if s == nil || s.persistence == nil || s.persistence.ConceptDescriptionRepository == nil {
		return nil, "", common.NewErrBadRequest("AASENV-CDREPO-LISTCD-NILSERVICE service must not be nil")
	}
	return s.persistence.ConceptDescriptionRepository.GetConceptDescriptions(ctx, idShort, isCaseOf, dataSpecificationRef, limit, cursor)
}
