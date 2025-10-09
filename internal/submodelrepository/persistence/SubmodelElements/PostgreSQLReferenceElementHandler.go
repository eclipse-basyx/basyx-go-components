package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
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

func (p PostgreSQLReferenceElementHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	refElem, ok := submodelElement.(*gen.ReferenceElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type ReferenceElement")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// ReferenceElement-specific database insertion
	err = insertReferenceElement(refElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLReferenceElementHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	refElem, ok := submodelElement.(*gen.ReferenceElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type ReferenceElement")
	}

	// Create the nested refElem with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// ReferenceElement-specific database insertion for nested element
	err = insertReferenceElement(refElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLReferenceElementHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	var sme gen.SubmodelElement = &gen.ReferenceElement{}
	var valueRef sql.NullInt64
	id, err := p.decorated.Read(tx, submodelId, idShortOrPath, &sme)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRow(`SELECT value_ref FROM reference_element WHERE id = $1`, id).Scan(&valueRef)
	if err != nil {
		return sme, nil
	}
	if valueRef.Valid {
		// Read the reference
		var refType string
		err = tx.QueryRow(`SELECT type FROM reference WHERE id = $1`, valueRef.Int64).Scan(&refType)
		if err != nil {
			return nil, err
		}
		rows, err := tx.Query(`SELECT type, value FROM reference_key WHERE reference_id = $1 ORDER BY position`, valueRef.Int64)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var keys []gen.Key
		for rows.Next() {
			var kType, kValue string
			if err := rows.Scan(&kType, &kValue); err != nil {
				return nil, err
			}
			keys = append(keys, gen.Key{Type: gen.KeyTypes(kType), Value: kValue})
		}
		refElem := sme.(*gen.ReferenceElement)
		refElem.Value = &gen.Reference{Type: gen.ReferenceTypes(refType), Keys: keys}
	}
	return sme, nil
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

func insertReferenceElement(refElem *gen.ReferenceElement, tx *sql.Tx, id int) error {
	if isEmptyReference(*refElem.Value) {
		// Insert with NULL
		_, err := tx.Exec(`INSERT INTO reference_element (id, value_ref) VALUES ($1, $2)`, id, nil)
		return err
	}

	// Insert the reference
	var refId int
	err := tx.QueryRow(`INSERT INTO reference (type) VALUES ($1) RETURNING id`, refElem.Value.Type).Scan(&refId)
	if err != nil {
		return err
	}

	// Insert reference keys
	for i, key := range refElem.Value.Keys {
		_, err = tx.Exec(`INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`,
			refId, i, key.Type, key.Value)
		if err != nil {
			return err
		}
	}

	// Insert reference_element
	_, err = tx.Exec(`INSERT INTO reference_element (id, value_ref) VALUES ($1, $2)`, id, refId)
	return err
}
