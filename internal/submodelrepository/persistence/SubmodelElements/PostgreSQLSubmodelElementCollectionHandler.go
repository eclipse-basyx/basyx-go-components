package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLSubmodelElementCollectionHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLSubmodelElementCollectionHandler(db *sql.DB) (*PostgreSQLSubmodelElementCollectionHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLSubmodelElementCollectionHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLSubmodelElementCollectionHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	_, ok := submodelElement.(*gen.SubmodelElementCollection)
	if !ok {
		return 0, errors.New("submodelElement is not of type SubmodelElementCollection")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// SubmodelElementCollection-specific database insertion
	_, err = tx.Exec(`INSERT INTO submodel_element_collection (id) VALUES ($1)`, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLSubmodelElementCollectionHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	_, ok := submodelElement.(*gen.SubmodelElementCollection)
	if !ok {
		return 0, errors.New("submodelElement is not of type SubmodelElementCollection")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// SubmodelElementCollection-specific database insertion
	_, err = tx.Exec(`INSERT INTO submodel_element_collection (id) VALUES ($1)`, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLSubmodelElementCollectionHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	return nil, nil
	// var sme gen.SubmodelElement = &gen.SubmodelElementCollection{}
	// id, err := p.decorated.Read(tx, submodelId, idShortOrPath, &sme)
	// if err != nil {
	// 	return nil, err
	// }

	// // Check if there are Children and load them if necessary
	// var idShortPath string
	// rows, err := tx.Query(`
	// 	SELECT idshort_path FROM submodel_element WHERE parent_sme_id = $1 ORDER BY position
	// `, id)
	// if err != nil {
	// 	return nil, err
	// }
	// defer rows.Close()
	// var children []gen.SubmodelElement
	// for rows.Next() {
	// 	if err := rows.Scan(&idShortPath); err != nil {
	// 		return nil, err
	// 	}
	// 	sme, err := GetSubmodelElement(p.db, submodelId, idShortPath)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	children = append(children, sme)
	// }
	// smc, ok := sme.(*gen.SubmodelElementCollection)
	// if ok {
	// 	smc.Value = children
	// }
	// return sme, nil
}
func (p PostgreSQLSubmodelElementCollectionHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLSubmodelElementCollectionHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
