package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLEntityHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLEntityHandler(db *sql.DB) (*PostgreSQLEntityHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLEntityHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLEntityHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	entity, ok := submodelElement.(*gen.Entity)
	if !ok {
		return 0, errors.New("submodelElement is not of type Entity")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// Entity-specific database insertion
	err = insertEntity(entity, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLEntityHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	entity, ok := submodelElement.(*gen.Entity)
	if !ok {
		return 0, errors.New("submodelElement is not of type Entity")
	}

	// Create the nested entity with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// Entity-specific database insertion for nested element
	err = insertEntity(entity, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLEntityHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	var sme gen.SubmodelElement
	_, err := p.decorated.Read(tx, submodelId, idShortOrPath, &sme)
	if err != nil {
		return nil, err
	}
	return sme, nil
}
func (p PostgreSQLEntityHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLEntityHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertEntity(entity *gen.Entity, tx *sql.Tx, id int) error {
	_, err := tx.Exec(`INSERT INTO entity_element (id, entity_type, global_asset_id) VALUES ($1, $2, $3)`,
		id, entity.EntityType, entity.GlobalAssetId)
	if err != nil {
		return err
	}

	// Insert specific asset ids
	for _, sai := range entity.SpecificAssetIds {
		var extRef sql.NullInt64
		if !isEmptyReference(sai.ExternalSubjectId) {
			refId, err := insertReference(tx, *sai.ExternalSubjectId)
			if err != nil {
				return err
			}
			extRef = sql.NullInt64{Int64: int64(refId), Valid: true}
		}
		_, err = tx.Exec(`INSERT INTO entity_specific_asset_id (entity_id, name, value, external_subject_ref) VALUES ($1, $2, $3, $4)`,
			id, sai.Name, sai.Value, extRef)
		if err != nil {
			return err
		}
	}
	return nil
}
