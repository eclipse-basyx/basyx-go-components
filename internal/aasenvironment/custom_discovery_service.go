package aasenvironment

import (
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	discoveryapi "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
)

// CustomDiscoveryService is a pass-through stub for future combined logic.
type CustomDiscoveryService struct {
	*discoveryapi.AssetAdministrationShellBasicDiscoveryAPIAPIService
	persistence *Persistence
}

// NewCustomDiscoveryService creates a new pass-through discovery decorator.
func NewCustomDiscoveryService(
	base *discoveryapi.AssetAdministrationShellBasicDiscoveryAPIAPIService,
	persistence *Persistence,
) *CustomDiscoveryService {
	return &CustomDiscoveryService{
		AssetAdministrationShellBasicDiscoveryAPIAPIService: base,
		persistence: persistence,
	}
}

// ExecuteInTransaction exposes shared transaction execution for future endpoint customizations.
func (s *CustomDiscoveryService) ExecuteInTransaction(fn func(tx *sql.Tx) error) error {
	if s == nil || s.persistence == nil {
		return common.NewErrBadRequest("AASENV-DISC-TX-NILSERVICE service must not be nil")
	}
	return s.persistence.ExecuteInTransaction("AASENV-DISC-STARTTX", "AASENV-DISC-COMMITTX", fn)
}
