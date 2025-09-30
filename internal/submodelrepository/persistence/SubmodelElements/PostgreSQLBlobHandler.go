package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLBlobHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLBlobHandler(db *sql.DB) (*PostgreSQLBlobHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLBlobHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLBlobHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	blob, ok := submodelElement.(*gen.Blob)
	if !ok {
		return 0, errors.New("submodelElement is not of type Blob")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// Blob-specific database insertion
	_, err = tx.Exec(`INSERT INTO blob_element (id, content_type, value) VALUES ($1, $2, $3)`,
		id, blob.ContentType, []byte(blob.Value))
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLBlobHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	blob, ok := submodelElement.(*gen.Blob)
	if !ok {
		return 0, errors.New("submodelElement is not of type Blob")
	}

	// Create the nested blob with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// Blob-specific database insertion for nested element
	_, err = tx.Exec(`INSERT INTO blob_element (id, content_type, value) VALUES ($1, $2, $3)`,
		id, blob.ContentType, []byte(blob.Value))
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLBlobHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	var sme gen.SubmodelElement = &gen.Blob{}
	var contentType string
	var value []byte
	id, err := p.decorated.Read(tx, submodelId, idShortOrPath, &sme)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRow(`
		SELECT content_type, value
		FROM blob_element
		WHERE id = $1
	`, id).Scan(&contentType, &value)
	if err != nil {
		return sme, nil // Return base if no specific data
	}
	blob := sme.(*gen.Blob)
	blob.ContentType = contentType
	blob.Value = string(value)
	return sme, nil
}
func (p PostgreSQLBlobHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLBlobHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
