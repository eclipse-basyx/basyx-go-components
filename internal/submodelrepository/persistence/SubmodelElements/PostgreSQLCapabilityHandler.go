package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLCapabilityHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLCapabilityHandler(db *sql.DB) (*PostgreSQLCapabilityHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLCapabilityHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLCapabilityHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	capability, ok := submodelElement.(*gen.Capability)
	if !ok {
		return 0, errors.New("submodelElement is not of type Capability")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// Capability-specific database insertion
	err = insertCapability(capability, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLCapabilityHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	capability, ok := submodelElement.(*gen.Capability)
	if !ok {
		return 0, errors.New("submodelElement is not of type Capability")
	}

	// Create the nested capability with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// Capability-specific database insertion for nested element
	err = insertCapability(capability, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLCapabilityHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	// First, get the base submodel element
	var baseSME gen.SubmodelElement
	_, err := p.decorated.Read(tx, submodelId, idShortOrPath, &baseSME)
	if err != nil {
		return nil, err
	}

	// Check if it's a capability
	_, ok := baseSME.(*gen.Capability)
	if !ok {
		return nil, errors.New("submodelElement is not of type Capability")
	}

	// Capability has no additional data, just return the base
	return baseSME, nil
}
func (p PostgreSQLCapabilityHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLCapabilityHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertCapability(capability *gen.Capability, tx *sql.Tx, id int) error {
	_, err := tx.Exec(`INSERT INTO capability_element (id) VALUES ($1)`, id)
	return err
}
