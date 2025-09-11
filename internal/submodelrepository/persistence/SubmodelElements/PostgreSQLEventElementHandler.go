package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLEventElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLEventElementHandler(db *sql.DB) (*PostgreSQLEventElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLEventElementHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLEventElementHandler) Create(submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	_, ok := submodelElement.(*gen.EventElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type EventElement")
	}

	// Start a database transaction at the EventElement level
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

	// EventElement-specific database insertion
	// Determine which column to use based on valueType

	// Then, perform EventElement-specific operations within the same transaction

	// Commit the transaction only if everything succeeded
	err = tx.Commit()
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLEventElementHandler) Read(idShortOrPath string) error {
	if dErr := p.decorated.Read(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLEventElementHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLEventElementHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
