package persistence_postgresql

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"

	_ "github.com/lib/pq" // PostgreSQL Treiber

	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/SubmodelElements"
	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
)

type PostgreSQLSubmodelDatabase struct {
	db *sql.DB
}

// Konstruktor
func NewPostgreSQLSubmodelBackend(dsn string) (*PostgreSQLSubmodelDatabase, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	dir, osErr := os.Getwd()

	if osErr != nil {
		return nil, osErr
	}

	queryString, fileError := os.ReadFile(dir + "/resources/sql/submodelrepositoryschema.sql")

	if fileError != nil {
		return nil, fileError
	}

	_, dbError := db.Exec(string(queryString))

	if dbError != nil {
		return nil, dbError
	}

	return &PostgreSQLSubmodelDatabase{db: db}, nil
}

// GetAllSubmodels holt alle Submodelle aus der DB
func (p *PostgreSQLSubmodelDatabase) GetAllSubmodels() ([]gen.Submodel, error) {
	rows, err := p.db.Query(`SELECT payload FROM submodels ORDER BY id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []gen.Submodel{}
	for rows.Next() {
		var js []byte
		if err := rows.Scan(&js); err != nil {
			return nil, err
		}
		var m gen.Submodel
		if err := json.Unmarshal(js, &m); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

// GetSubmodel returns one Submodel by id
func (p *PostgreSQLSubmodelDatabase) GetSubmodel(id string) (gen.Submodel, error) {
	var js []byte
	err := p.db.QueryRow(`SELECT payload FROM submodels WHERE id=$1`, id).Scan(&js)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return gen.Submodel{}, sql.ErrNoRows
		}
		return gen.Submodel{}, err
	}
	var m gen.Submodel
	if err := json.Unmarshal(js, &m); err != nil {
		return gen.Submodel{}, err
	}
	return m, nil
}

// DeleteSubmodel deletes a Submodel by id
func (p *PostgreSQLSubmodelDatabase) DeleteSubmodel(id string) error {
	_, err := p.db.Exec(`DELETE FROM submodels WHERE id=$1`, id)
	return err
}

// CreateSubmodel inserts a new Submodel
func (p *PostgreSQLSubmodelDatabase) CreateSubmodel(m gen.Submodel) (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	_, err = p.db.Exec(
		`INSERT INTO submodels(id, payload) VALUES ($1, $2::jsonb) ON CONFLICT (id) DO UPDATE SET payload = EXCLUDED.payload`,
		m.Id, string(b),
	)
	return m.Id, err
}

func (p *PostgreSQLSubmodelDatabase) AddSubmodelElement(submodelId string, submodelElement gen.SubmodelElement) error {
	var handler submodelelements.PostgreSQLSMECrudInterface
	switch submodelElement.ModelType {
	case "Property":
		propHandler, err := submodelelements.NewPostgreSQLPropertyHandler(p.db)
		if err != nil {
			return err
		}
		handler = propHandler
	default:
		return errors.New("ModelType " + string(submodelElement.ModelType) + " unsupported.")
	}
	if err := handler.Create(submodelId, submodelElement); err != nil {
		return err
	}
	return nil
}
