package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLFileHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLFileHandler(db *sql.DB) (*PostgreSQLFileHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLFileHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLFileHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	file, ok := submodelElement.(*gen.File)
	if !ok {
		return 0, errors.New("submodelElement is not of type File")
	}
	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// File-specific database insertion
	_, err = tx.Exec(`INSERT INTO file_element (id, content_type, value) VALUES ($1, $2, $3)`,
		id, file.ContentType, file.Value)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLFileHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	file, ok := submodelElement.(*gen.File)
	if !ok {
		return 0, errors.New("submodelElement is not of type File")
	}

	// Create the nested file with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// File-specific database insertion for nested element
	_, err = tx.Exec(`INSERT INTO file_element (id, content_type, value) VALUES ($1, $2, $3)`,
		id, file.ContentType, file.Value)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLFileHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	var sme gen.SubmodelElement = &gen.File{}
	var contentType, value string
	id, err := p.decorated.Read(tx, submodelId, idShortOrPath, &sme)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRow(`
		SELECT content_type, value
		FROM file_element
		WHERE id = $1
	`, id).Scan(&contentType, &value)
	if err != nil {
		return sme, nil // Return base if no specific data
	}
	file := sme.(*gen.File)
	file.ContentType = contentType
	file.Value = value
	return sme, nil
}
func (p PostgreSQLFileHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLFileHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
