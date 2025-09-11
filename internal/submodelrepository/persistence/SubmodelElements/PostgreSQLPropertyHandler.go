package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLPropertyHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// Konstruktor
func NewPostgreSQLPropertyHandler(db *sql.DB) (*PostgreSQLPropertyHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLPropertyHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLPropertyHandler) Create(submodelId string, submodelElement interface{}) error {
	_, ok := submodelElement.(gen.Property)
	genericSubmodelElement, ok := submodelElement.(gen.SubmodelElement)
	if !ok {
		return errors.New("submodelElement does not implement SubmodelElement interface")
	}
	if !ok {
		return errors.New("submodelElement is not of type Property")
	}
	// Start a database transaction at the Property level
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	// Defer rollback in case of error
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// First, perform base SubmodelElement operations within the transaction

	err = p.decorated.CreateWithTx(tx, submodelId, genericSubmodelElement)
	if err != nil {
		return err
	}

	// Then, perform Property-specific operations within the same transaction

	// Commit the transaction only if everything succeeded
	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}
func (p PostgreSQLPropertyHandler) Read(idShortOrPath string) error {
	if dErr := p.decorated.Read(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLPropertyHandler) Update(idShortOrPath string, submodelElement interface{}) error {
	genericSubmodelElement, ok := submodelElement.(gen.SubmodelElement)
	if !ok {
		return errors.New("submodelElement does not implement SubmodelElement interface")
	}
	if dErr := p.decorated.Update(idShortOrPath, genericSubmodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLPropertyHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
