package submodelelements

import (
	"database/sql"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
)

type PostgreSQLSMECrudInterface interface {
	Create(*sql.Tx, string, gen.SubmodelElement) (int, error)
	CreateNested(*sql.Tx, string, int, string, gen.SubmodelElement, int) (int, error)
	Read(string) error
	Update(string, gen.SubmodelElement) error
	Delete(string) error
}
