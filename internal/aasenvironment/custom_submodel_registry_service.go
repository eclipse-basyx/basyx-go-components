package aasenvironment

import (
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	smregistryapi "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/api"
)

// CustomSubmodelRegistryService is a pass-through stub for future combined logic.
type CustomSubmodelRegistryService struct {
	*smregistryapi.SubmodelRegistryAPIAPIService
	persistence *Persistence
}

// NewCustomSubmodelRegistryService creates a new pass-through submodel registry decorator.
func NewCustomSubmodelRegistryService(
	base *smregistryapi.SubmodelRegistryAPIAPIService,
	persistence *Persistence,
) *CustomSubmodelRegistryService {
	return &CustomSubmodelRegistryService{
		SubmodelRegistryAPIAPIService: base,
		persistence:                   persistence,
	}
}

// ExecuteInTransaction exposes shared transaction execution for future endpoint customizations.
func (s *CustomSubmodelRegistryService) ExecuteInTransaction(fn func(tx *sql.Tx) error) error {
	if s == nil || s.persistence == nil {
		return common.NewErrBadRequest("AASENV-SMREG-TX-NILSERVICE service must not be nil")
	}
	return s.persistence.ExecuteInTransaction("AASENV-SMREG-STARTTX", "AASENV-SMREG-COMMITTX", fn)
}
