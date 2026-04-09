package aasenvironment

import (
	"database/sql"

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
