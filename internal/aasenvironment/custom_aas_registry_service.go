// Package aasenvironment provides composition services for the AAS Environment APIs.
package aasenvironment

import (
	"database/sql"

	aasregistryapi "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// CustomAASRegistryService is a pass-through stub for future combined logic.
type CustomAASRegistryService struct {
	*aasregistryapi.AssetAdministrationShellRegistryAPIAPIService
	persistence *Persistence
}

// NewCustomAASRegistryService creates a new pass-through registry decorator.
func NewCustomAASRegistryService(
	base *aasregistryapi.AssetAdministrationShellRegistryAPIAPIService,
	persistence *Persistence,
) *CustomAASRegistryService {
	return &CustomAASRegistryService{
		AssetAdministrationShellRegistryAPIAPIService: base,
		persistence: persistence,
	}
}

// ExecuteInTransaction exposes shared transaction execution for future endpoint customizations.
func (s *CustomAASRegistryService) ExecuteInTransaction(fn func(tx *sql.Tx) error) error {
	if s == nil || s.persistence == nil {
		return common.NewErrBadRequest("AASENV-AASREG-TX-NILSERVICE service must not be nil")
	}
	return s.persistence.ExecuteInTransaction("AASENV-AASREG-STARTTX", "AASENV-AASREG-COMMITTX", fn)
}
