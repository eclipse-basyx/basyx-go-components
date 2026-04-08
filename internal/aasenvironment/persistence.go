package aasenvironment

import (
	"database/sql"

	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	cdrdb "github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/persistence"
	discoverydb "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/persistence"
	smregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/persistence"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
)

// Persistence bundles all component persistence backends and the shared DB pool.
type Persistence struct {
	DB *sql.DB

	AASRegistry                  *aasregistrydb.PostgreSQLAASRegistryDatabase
	SubmodelRegistry             *smregistrydb.PostgreSQLSMDatabase
	AASRepository                *aasrepositorydb.AssetAdministrationShellDatabase
	SubmodelRepository           *submodelrepositorydb.SubmodelDatabase
	ConceptDescriptionRepository *cdrdb.ConceptDescriptionBackend
	Discovery                    *discoverydb.PostgreSQLDiscoveryDatabase
}

// ExecuteInTransaction runs fn in a single shared DB transaction.
func (p *Persistence) ExecuteInTransaction(startErrorCode string, commitErrorCode string, fn func(tx *sql.Tx) error) error {
	if p == nil {
		return common.NewErrBadRequest("AASENV-TX-NILPERSISTENCE persistence bundle must not be nil")
	}
	if p.DB == nil {
		return common.NewErrBadRequest("AASENV-TX-NILDB shared DB pool must not be nil")
	}
	return common.ExecuteInTransaction(p.DB, startErrorCode, commitErrorCode, fn)
}
