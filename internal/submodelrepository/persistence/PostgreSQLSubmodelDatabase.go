package persistence_postgresql

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strconv"

	_ "github.com/lib/pq" // PostgreSQL Treiber

	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/SubmodelElements"
	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
)

type PostgreSQLSubmodelDatabase struct {
	db *sql.DB
}

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

// GetAllSubmodels returns a page of Submodels and a next cursor ("" if no more pages).
func (p *PostgreSQLSubmodelDatabase) GetAllSubmodels(limit int32, cursor string, idShort string) ([]gen.Submodel, string, error) {
	if limit <= 0 || limit > 1000 {
		limit = 25
	}

	// Keyset pagination: start after the cursor (last seen id).
	// Simple filter by idShort; leave semanticId/level/extent for later.
	const q = `
        SELECT id, id_short, category, kind, 'Submodel' AS model_type
        FROM submodel
        WHERE ($1 = '' OR id_short = $1)
          AND ($2 = '' OR id > $2)
        ORDER BY id
        LIMIT $3
    `
	rows, err := p.db.Query(q, idShort, cursor, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	list := make([]gen.Submodel, 0, limit)
	var lastID string

	for rows.Next() {
		var (
			id, idShortDB, category, modelType string
			kind                               sql.NullString
		)
		if err := rows.Scan(&id, &idShortDB, &category, &kind, &modelType); err != nil {
			return nil, "", err
		}

		sm := gen.Submodel{
			Id:        id,
			IdShort:   idShortDB,
			Category:  category,
			ModelType: modelType, // "Submodel"
		}
		if kind.Valid {
			sm.Kind = gen.ModellingKind(kind.String) // enum stored as text
		}
		list = append(list, sm)
		lastID = id
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if int32(len(list)) == limit {
		nextCursor = lastID
	}
	return list, nextCursor, nil
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
	handler, err := getSMEHandler(submodelElement, p)
	if err != nil {
		return err
	}

	// Start a database transaction
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	// Defer rollback in case of error
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Create the top-level element
	parentId, err := handler.Create(tx, submodelId, submodelElement)
	if err != nil {
		return err
	}

	// Handle nested elements for collections and lists
	switch string(submodelElement.GetModelType()) {
	case "SubmodelElementCollection":
		submodelElementCollection, ok := submodelElement.(*gen.SubmodelElementCollection)
		if !ok {
			return errors.New("submodelElement is not of type SubmodelElementCollection")
		}
		// Recursively add nested elements
		for _, nestedElement := range submodelElementCollection.Value {
			if err := p.AddNestedSubmodelElementRecursively(tx, submodelId, parentId, submodelElementCollection.IdShort, nestedElement); err != nil {
				return err
			}
		}
	case "SubmodelElementList":
		submodelElementList, ok := submodelElement.(*gen.SubmodelElementList)
		if !ok {
			return errors.New("submodelElement is not of type SubmodelElementList")
		}
		// Recursively add nested elements with index-based paths
		for index, nestedElement := range submodelElementList.Value {
			idShortPath := submodelElementList.IdShort + "[" + strconv.Itoa(index) + "]"
			if err := p.AddNestedSubmodelElementRecursively(tx, submodelId, parentId, idShortPath, nestedElement); err != nil {
				return err
			}
		}
	}

	// Commit the transaction only if everything succeeded
	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (p *PostgreSQLSubmodelDatabase) AddNestedSubmodelElementRecursively(tx *sql.Tx, submodelId string, parentId int, currentIdShortPath string, submodelElement gen.SubmodelElement) error {
	handler, err := getSMEHandler(submodelElement, p)
	if err != nil {
		return err
	}

	// Create the nested element with the proper idShortPath
	// According to IDTA AAS grammar: <idShortPath> ::= <idShort> {[ "." <idShort> | "["<Index>"]" ]}*
	var idShortPath string
	if currentIdShortPath == "" {
		idShortPath = submodelElement.GetIdShort()
	} else {
		// For SubmodelElementList, currentIdShortPath already contains the [index] format
		if string(submodelElement.GetModelType()) == "SubmodelElementList" ||
			(len(currentIdShortPath) > 0 && currentIdShortPath[len(currentIdShortPath)-1] == ']') {
			idShortPath = currentIdShortPath
		} else {
			// For SubmodelElementCollection, use dot notation
			idShortPath = currentIdShortPath + "." + submodelElement.GetIdShort()
		}
	}

	// Create the nested element using CreateNested
	parentId, err = handler.CreateNested(tx, submodelId, parentId, idShortPath, submodelElement)
	if err != nil {
		return err
	}

	// Handle recursive nesting for collections and lists
	switch string(submodelElement.GetModelType()) {
	case "SubmodelElementCollection":
		submodelElementCollection, ok := submodelElement.(*gen.SubmodelElementCollection)
		if !ok {
			return errors.New("submodelElement is not of type SubmodelElementCollection")
		}
		// Recursively add nested elements with dot notation
		for _, nestedElement := range submodelElementCollection.Value {
			if err := p.AddNestedSubmodelElementRecursively(tx, submodelId, parentId, idShortPath, nestedElement); err != nil {
				return err
			}
		}
	case "SubmodelElementList":
		submodelElementList, ok := submodelElement.(*gen.SubmodelElementList)
		if !ok {
			return errors.New("submodelElement is not of type SubmodelElementList")
		}
		// Recursively add nested elements with index-based paths
		for index, nestedElement := range submodelElementList.Value {
			nestedIdShortPath := idShortPath + "[" + strconv.Itoa(index) + "]"
			if err := p.AddNestedSubmodelElementRecursively(tx, submodelId, parentId, nestedIdShortPath, nestedElement); err != nil {
				return err
			}
		}
	}

	return nil
}

func getSMEHandler(submodelElement gen.SubmodelElement, p *PostgreSQLSubmodelDatabase) (submodelelements.PostgreSQLSMECrudInterface, error) {
	var handler submodelelements.PostgreSQLSMECrudInterface

	switch string(submodelElement.GetModelType()) {
	case "AnnotatedRelationshipElement":
		areHandler, err := submodelelements.NewPostgreSQLAnnotatedRelationshipElementHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = areHandler
	case "BasicEventElement":
		beeHandler, err := submodelelements.NewPostgreSQLBasicEventElementHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = beeHandler
	case "Blob":
		blobHandler, err := submodelelements.NewPostgreSQLBlobHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = blobHandler
	case "Capability":
		capHandler, err := submodelelements.NewPostgreSQLCapabilityHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = capHandler
	case "DataElement":
		deHandler, err := submodelelements.NewPostgreSQLDataElementHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = deHandler
	case "Entity":
		entityHandler, err := submodelelements.NewPostgreSQLEntityHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = entityHandler
	case "EventElement":
		eventElemHandler, err := submodelelements.NewPostgreSQLEventElementHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = eventElemHandler
	case "File":
		fileHandler, err := submodelelements.NewPostgreSQLFileHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = fileHandler
	case "MultiLanguageProperty":
		mlpHandler, err := submodelelements.NewPostgreSQLMultiLanguagePropertyHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = mlpHandler
	case "Operation":
		opHandler, err := submodelelements.NewPostgreSQLOperationHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = opHandler
	case "Property":
		propHandler, err := submodelelements.NewPostgreSQLPropertyHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = propHandler
	case "Range":
		rangeHandler, err := submodelelements.NewPostgreSQLRangeHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = rangeHandler
	case "ReferenceElement":
		refElemHandler, err := submodelelements.NewPostgreSQLReferenceElementHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = refElemHandler
	case "RelationshipElement":
		relElemHandler, err := submodelelements.NewPostgreSQLRelationshipElementHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = relElemHandler
	case "SubmodelElementCollection":
		smeColHandler, err := submodelelements.NewPostgreSQLSubmodelElementCollectionHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = smeColHandler
	case "SubmodelElementList":
		smeListHandler, err := submodelelements.NewPostgreSQLSubmodelElementListHandler(p.db)
		if err != nil {
			return nil, err
		}
		handler = smeListHandler
	default:
		return nil, errors.New("ModelType " + string(submodelElement.GetModelType()) + " unsupported.")
	}
	return handler, nil
}
