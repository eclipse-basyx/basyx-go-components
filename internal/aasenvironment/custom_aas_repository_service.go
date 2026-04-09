package aasenvironment

import (
	"database/sql"

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
