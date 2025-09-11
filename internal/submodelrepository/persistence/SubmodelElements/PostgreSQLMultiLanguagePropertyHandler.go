package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLMultiLanguagePropertyHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLMultiLanguagePropertyHandler(db *sql.DB) (*PostgreSQLMultiLanguagePropertyHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLMultiLanguagePropertyHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLMultiLanguagePropertyHandler) Create(submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	_, ok := submodelElement.(*gen.MultiLanguageProperty)
	if !ok {
		return 0, errors.New("submodelElement is not of type MultiLanguageProperty")
	}

	// Start a database transaction at the MultiLanguageProperty level
	tx, err := p.db.Begin()
	if err != nil {
		return 0, err
	}

	// Defer rollback in case of error
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.CreateWithTx(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// MultiLanguageProperty-specific database insertion
	// Determine which column to use based on valueType

	// Then, perform MultiLanguageProperty-specific operations within the same transaction

	// Commit the transaction only if everything succeeded
	err = tx.Commit()
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLMultiLanguagePropertyHandler) CreateNested(submodelId string, idShortPath string, submodelElement gen.SubmodelElement) (int, error) {
	return 0, errors.New("not implemented")
}

func (p PostgreSQLMultiLanguagePropertyHandler) Read(idShortOrPath string) error {
	if dErr := p.decorated.Read(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLMultiLanguagePropertyHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLMultiLanguagePropertyHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
