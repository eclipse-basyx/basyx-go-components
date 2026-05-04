package aasenvironment

import (
	"database/sql"

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
