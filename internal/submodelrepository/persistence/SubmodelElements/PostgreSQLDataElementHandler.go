package submodelelements

import (
	"database/sql"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLDataElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLDataElementHandler(db *sql.DB) (*PostgreSQLDataElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLDataElementHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLDataElementHandler) Create(submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	id, dErr := p.decorated.Create(submodelId, submodelElement)
	if dErr != nil {
		return 0, dErr
	}
	return id, nil
}

func (p PostgreSQLDataElementHandler) Read(idShortOrPath string) error {
	if dErr := p.decorated.Read(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLDataElementHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLDataElementHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
