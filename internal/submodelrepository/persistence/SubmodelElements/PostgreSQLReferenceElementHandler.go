package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLReferenceElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLReferenceElementHandler(db *sql.DB) (*PostgreSQLReferenceElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLReferenceElementHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLReferenceElementHandler) Create(submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	_, ok := submodelElement.(*gen.ReferenceElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type ReferenceElement")
	}

	// Start a database transaction at the ReferenceElement level
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

	// ReferenceElement-specific database insertion
	// Determine which column to use based on valueType

	// Then, perform ReferenceElement-specific operations within the same transaction

	// Commit the transaction only if everything succeeded
	err = tx.Commit()
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLReferenceElementHandler) CreateNested(submodelId string, idShortPath string, submodelElement gen.SubmodelElement) (int, error) {
	return 0, errors.New("not implemented")
}

func (p PostgreSQLReferenceElementHandler) Read(idShortOrPath string) error {
	if dErr := p.decorated.Read(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLReferenceElementHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLReferenceElementHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
