package submodelelements

import (
	"database/sql"
	"fmt"
	"reflect"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLSMECrudHandler struct {
	db *sql.DB
}

// isEmptyReference checks if a Reference is empty (zero value)
func isEmptyReference(ref gen.Reference) bool {
	return reflect.DeepEqual(ref, gen.Reference{})
}

// Konstruktor
func NewPostgreSQLSMECrudHandler(db *sql.DB) (*PostgreSQLSMECrudHandler, error) {
	return &PostgreSQLSMECrudHandler{db: db}, nil
}

func (p *PostgreSQLSMECrudHandler) Create(submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	// Start a database transaction
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

	id, err := p.CreateWithTx(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	err = tx.Commit()
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateWithTx performs the base SubmodelElement operations within an existing transaction
func (p *PostgreSQLSMECrudHandler) CreateWithTx(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	var referenceID sql.NullInt64

	if !isEmptyReference(submodelElement.GetSemanticId()) {
		var id int
		err := tx.QueryRow(`INSERT INTO reference (type) VALUES ($1) RETURNING id`, submodelElement.GetSemanticId().Type).Scan(&id)
		if err != nil {
			return 0, err
		}
		referenceID = sql.NullInt64{Int64: int64(id), Valid: true}
		println("Inserted Reference for SubmodelElement with idShort: " + submodelElement.GetIdShort())

		references := submodelElement.GetSemanticId().Keys
		for i := range references {
			_, err = tx.Exec(`INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`,
				id, i, references[i].Type, references[i].Value)
			if err != nil {
				return 0, err
			}
			println("Inserted Reference Key for SubmodelElement with idShort: " + submodelElement.GetIdShort())
		}
	}
	// If no semantic ID is provided, referenceID remains sql.NullInt64{Valid: false} which represents NULL

	// Check if a SubmodelElement with the same submodelId and idshort_path already exists
	var exists bool
	err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM submodel_element WHERE submodel_id = $1 AND idshort_path = $2)`,
		submodelId, submodelElement.GetIdShort()).Scan(&exists)
	if err != nil {
		return 0, err
	}

	if exists {
		return 0, fmt.Errorf("SubmodelElement with submodelId '%s' and idshort_path '%s' already exists",
			submodelId, submodelElement.GetIdShort())
	}
	var id int
	err = tx.QueryRow(`	INSERT INTO
	 					submodel_element(submodel_id, parent_sme_id, position, id_short, category, model_type, semantic_id, idshort_path)
						VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		submodelId,
		nil, //TODO
		0,   //TODO
		submodelElement.GetIdShort(),
		submodelElement.GetCategory(),
		submodelElement.GetModelType(),
		referenceID,                  // This will be NULL if no semantic ID was provided
		submodelElement.GetIdShort(), //TODO
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	println("Inserted SubmodelElement with idShort: " + submodelElement.GetIdShort())

	return id, nil
}

func (p *PostgreSQLSMECrudHandler) Read(idShortOrPath string) error {
	return nil
}

func (p *PostgreSQLSMECrudHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	return nil
}

func (p *PostgreSQLSMECrudHandler) Delete(idShortOrPath string) error {
	return nil
}
