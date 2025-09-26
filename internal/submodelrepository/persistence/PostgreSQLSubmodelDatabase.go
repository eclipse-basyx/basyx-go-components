package persistence_postgresql

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "github.com/lib/pq" // PostgreSQL Treiber

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/SubmodelElements"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
)

type PostgreSQLSubmodelDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

var maxCacheSize = 1000

// InMemory Cache for submodels
var submodelCache map[string]gen.Submodel = make(map[string]gen.Submodel)

func NewPostgreSQLSubmodelBackend(dsn string, maxOpenConns, maxIdleConns int, connMaxLifetimeMinutes int, cacheEnabled bool) (*PostgreSQLSubmodelDatabase, error) {
	db, err := sql.Open("postgres", dsn)
	//Set Max Connection
	db.SetMaxOpenConns(500)
	db.SetMaxIdleConns(500)
	db.SetConnMaxLifetime(time.Minute * 5)

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

	return &PostgreSQLSubmodelDatabase{db: db, cacheEnabled: cacheEnabled}, nil
}

// GetAllSubmodels and a next cursor ("" if no more pages).
func (p *PostgreSQLSubmodelDatabase) GetAllSubmodels(limit int32, cursor string, idShort string) ([]gen.Submodel, string, error) {
	if limit <= 0 {
		limit = 100
	}

	// Keyset pagination: start after the cursor (last seen id).
	// Simple filter by idShort if provided.
	// Note: This assumes 'id' is unique and can be used for pagination.
	// Adjust the query as needed based on actual requirements and schema.
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
	// Check cache first
	if p.cacheEnabled {
		if sm, found := submodelCache[id]; found {
			return sm, nil
		}
	}

	// Not in cache, fetch from DB
	tx, err := p.db.Begin()

	if err != nil {
		fmt.Println(err)
		return gen.Submodel{}, common.NewInternalServerError("Failed to begin PostgreSQL transaction - no changes applied - see console for details")
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	sm, err := submodelelements.GetSubmodelWithSubmodelElements(p.db, tx, id)
	if err != nil {
		return gen.Submodel{}, err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return gen.Submodel{}, common.NewInternalServerError("Failed to commit PostgreSQL transaction - no changes applied - see console for details")
	}

	// Store in cache
	if p.cacheEnabled {
		submodelCache[id] = *sm
	}
	return *sm, nil
}

// DeleteSubmodel deletes a Submodel by id
func (p *PostgreSQLSubmodelDatabase) DeleteSubmodel(id string) error {
	// Check cache first
	if p.cacheEnabled {
		if _, found := submodelCache[id]; found {
			delete(submodelCache, id)
		}
	}

	tx, err := p.db.Begin()

	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to begin PostgreSQL transaction - no changes applied - see console for details")
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	const q = `DELETE FROM submodel WHERE id=$1`

	res, err := tx.Exec(q, id)
	if err != nil {
		return err
	}

	// Check if a row was actually deleted
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to commit PostgreSQL transaction - no changes applied - see console for details")
	}
	return nil
}

// CreateSubmodel inserts a new Submodel
// If a Submodel with the same id already exists, it does nothing and returns nil
// we might want ON CONFLICT DO UPDATE for upserts, but spec-wise POST usually means create new
// model_type is hardcoded to "Submodel"
func (p *PostgreSQLSubmodelDatabase) CreateSubmodel(sm gen.Submodel) error {
	tx, err := p.db.Begin()

	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to begin PostgreSQL transaction - no changes applied - see console for details")
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	referenceID, err := persistence_utils.CreateSemanticId(tx, sm.SemanticId)

	const q = `
        INSERT INTO submodel (id, id_short, category, kind, model_type, semantic_id)
        VALUES ($1, $2, $3, $4, 'Submodel', $5)
        ON CONFLICT (id) DO NOTHING
    `

	_, err = tx.Exec(q, sm.Id, sm.IdShort, sm.Category, sm.Kind, referenceID)
	if err != nil {
		return err
	}

	if len(sm.SubmodelElements) > 0 {
		for _, element := range sm.SubmodelElements {
			err = p.AddSubmodelElementWithTransaction(tx, sm.Id, element)
			if err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to commit PostgreSQL transaction - no changes applied - see console for details")
	}
	// Store in cache if enough space
	if p.cacheEnabled {
		if len(submodelCache) < maxCacheSize {
			submodelCache[sm.Id] = sm
		}
	}
	return nil
}

func (p *PostgreSQLSubmodelDatabase) GetSubmodelElement(submodelId string, idShortOrPath string, limit int, cursor string) (gen.SubmodelElement, error) {
	tx, err := p.db.Begin()
	if err != nil {
		fmt.Println(err)
		return nil, common.NewInternalServerError("Failed to begin PostgreSQL transaction - no changes applied - see console for details")
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	elements, _, err := submodelelements.GetSubmodelElementsWithPath(p.db, tx, submodelId, idShortOrPath, limit, cursor)
	if err != nil {
		return nil, err
	}

	if len(elements) == 0 {
		return nil, common.NewErrNotFound("SubmodelElement with idShort or path '" + idShortOrPath + "' not found in submodel '" + submodelId + "'")
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return nil, common.NewInternalServerError("Failed to commit PostgreSQL transaction - no changes applied - see console for details")
	}

	return elements[0], nil
}

func (p *PostgreSQLSubmodelDatabase) GetSubmodelElements(submodelId string, limit int, cursor string) ([]gen.SubmodelElement, string, error) {
	tx, err := p.db.Begin()
	if err != nil {
		fmt.Println(err)
		return nil, "", common.NewInternalServerError("Failed to begin PostgreSQL transaction - no changes applied - see console for details")
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	elements, cursor, err := submodelelements.GetSubmodelElementsWithPath(p.db, tx, submodelId, "", limit, cursor)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return nil, "", common.NewInternalServerError("Failed to commit PostgreSQL transaction - no changes applied - see console for details")
	}

	return elements, cursor, nil
}

func (p *PostgreSQLSubmodelDatabase) AddSubmodelElementWithPath(submodelId string, idShortPath string, submodelElement gen.SubmodelElement) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		if _, found := submodelCache[submodelId]; found {
			delete(submodelCache, submodelId)
		}
	}
	handler, err := submodelelements.GetSMEHandler(submodelElement, p.db)
	if err != nil {
		return err
	}

	crud, err := submodelelements.NewPostgreSQLSMECrudHandler(p.db)
	if err != nil {
		return err
	}

	tx, err := p.db.Begin()
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to begin PostgreSQL transaction - no changes applied - see console for details")
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	parentId, err := crud.GetDatabaseId(idShortPath)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to execute PostgreSQL Query - no changes applied - see console for details.")
	}
	nextPosition, err := crud.GetNextPosition(parentId)
	if err != nil {
		return err
	}

	modelType, err := crud.GetSubmodelElementType(idShortPath)
	if err != nil {
		return err
	}
	if modelType != "SubmodelElementCollection" && modelType != "SubmodelElementList" {
		return errors.New("cannot add nested element to non-collection/list element")
	}
	var newIdShortPath string
	if modelType == "SubmodelElementList" {
		newIdShortPath = idShortPath + "[" + strconv.Itoa(nextPosition) + "]"
	} else {
		newIdShortPath = idShortPath + "." + submodelElement.GetIdShort()
	}
	id, err := handler.CreateNested(tx, submodelId, parentId, newIdShortPath, submodelElement, nextPosition)
	if err != nil {
		return err
	}
	err = p.AddNestedSubmodelElementsIteratively(tx, submodelId, id, submodelElement, newIdShortPath)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to commit PostgreSQL transaction - no changes applied - see console for details")
	}

	return nil
}
func (p *PostgreSQLSubmodelDatabase) AddSubmodelElement(submodelId string, submodelElement gen.SubmodelElement) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		if _, found := submodelCache[submodelId]; found {
			delete(submodelCache, submodelId)
		}
	}
	tx, err := p.db.Begin()
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to begin PostgreSQL transaction - no changes applied - see console for details")
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	err = p.AddSubmodelElementWithTransaction(tx, submodelId, submodelElement)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to commit PostgreSQL transaction - no changes applied - see console for details")
	}

	return nil
}

func (p *PostgreSQLSubmodelDatabase) AddSubmodelElementWithTransaction(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		if _, found := submodelCache[submodelId]; found {
			delete(submodelCache, submodelId)
		}
	}
	handler, err := submodelelements.GetSMEHandler(submodelElement, p.db)
	if err != nil {
		return err
	}
	parentId, err := handler.Create(tx, submodelId, submodelElement)
	if err != nil {
		return err
	}

	err = p.AddNestedSubmodelElementsIteratively(tx, submodelId, parentId, submodelElement, "")
	if err != nil {
		return err
	}
	return nil
}

type ElementToProcess struct {
	element                   gen.SubmodelElement
	parentId                  int
	currentIdShortPath        string
	isFromSubmodelElementList bool // Indicates if the current element is from a SubmodelElementList
	position                  int  // Position/index within the parent collection or list
}

func (p *PostgreSQLSubmodelDatabase) AddNestedSubmodelElementsIteratively(tx *sql.Tx, submodelId string, topLevelParentId int, topLevelElement gen.SubmodelElement, startPath string) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		if _, found := submodelCache[submodelId]; found {
			delete(submodelCache, submodelId)
		}
	}
	stack := []ElementToProcess{}

	switch string(topLevelElement.GetModelType()) {
	case "SubmodelElementCollection":
		submodelElementCollection, ok := topLevelElement.(*gen.SubmodelElementCollection)
		if !ok {
			return common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementCollection' is not of type SubmodelElementCollection")
		}
		for index, nestedElement := range submodelElementCollection.Value {
			var currentPath string
			if startPath == "" {
				currentPath = submodelElementCollection.IdShort
			} else {
				currentPath = startPath
			}
			stack = append(stack, ElementToProcess{
				element:                   nestedElement,
				parentId:                  topLevelParentId,
				currentIdShortPath:        currentPath,
				isFromSubmodelElementList: false,
				position:                  index,
			})
		}
	case "SubmodelElementList":
		submodelElementList, ok := topLevelElement.(*gen.SubmodelElementList)
		if !ok {
			return common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementList' is not of type SubmodelElementList")
		}
		// Add nested elements to stack with index-based paths
		for index, nestedElement := range submodelElementList.Value {
			var idShortPath string
			if startPath == "" {
				idShortPath = submodelElementList.IdShort + "[" + strconv.Itoa(index) + "]"
			} else {
				idShortPath = startPath
			}
			stack = append(stack, ElementToProcess{
				element:                   nestedElement,
				parentId:                  topLevelParentId,
				currentIdShortPath:        idShortPath,
				isFromSubmodelElementList: true,
				position:                  index,
			})
		}
	}

	for len(stack) > 0 {
		// LIFO Stack
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		handler, err := submodelelements.GetSMEHandler(current.element, p.db)
		if err != nil {
			return err
		}

		// Build the idShortPath for current element
		idShortPath := buildCurrentIdShortPath(current)

		newParentId, err := handler.CreateNested(tx, submodelId, current.parentId, idShortPath, current.element, current.position)
		if err != nil {
			return err
		}

		switch string(current.element.GetModelType()) {
		case "SubmodelElementCollection":
			submodelElementCollection, ok := current.element.(*gen.SubmodelElementCollection)
			if !ok {
				return common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementCollection' is not of type SubmodelElementCollection")
			}
			for i := len(submodelElementCollection.Value) - 1; i >= 0; i-- {
				stack = addNestedElementToStackWithNormalPath(submodelElementCollection, i, stack, newParentId, idShortPath)
			}
		case "SubmodelElementList":
			submodelElementList, ok := current.element.(*gen.SubmodelElementList)
			if !ok {
				return common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementList' is not of type SubmodelElementList")
			}
			for index := len(submodelElementList.Value) - 1; index >= 0; index-- {
				stack = addNestedElementToStackWithIndexPath(submodelElementList, index, idShortPath, stack, newParentId)
			}
		}
	}

	return nil
}

// This method removes a SubmodelElement by its idShort or path and all its nested elements
// If the deleted Element is in a SubmodelElementList, the indices of the remaining elements are adjusted accordingly
func (p *PostgreSQLSubmodelDatabase) DeleteSubmodelElementByPath(submodelId string, idShortOrPath string) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		if _, found := submodelCache[submodelId]; found {
			delete(submodelCache, submodelId)
		}
	}
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	err = submodelelements.DeleteSubmodelElementByPath(tx, submodelId, idShortOrPath)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func buildCurrentIdShortPath(current ElementToProcess) string {
	var idShortPath string
	if current.currentIdShortPath == "" {
		idShortPath = current.element.GetIdShort()
	} else {
		// If element comes from a SubmodelElementList, use the path as-is (includes [index])
		if current.isFromSubmodelElementList {
			idShortPath = current.currentIdShortPath
		} else {
			// For SubmodelElementCollection, append element's idShort with dot notation
			idShortPath = current.currentIdShortPath + "." + current.element.GetIdShort()
		}
	}
	return idShortPath
}

func addNestedElementToStackWithNormalPath(submodelElementCollection *gen.SubmodelElementCollection, i int, stack []ElementToProcess, newParentId int, idShortPath string) []ElementToProcess {
	nestedElement := submodelElementCollection.Value[i]
	stack = append(stack, ElementToProcess{
		element:                   nestedElement,
		parentId:                  newParentId,
		currentIdShortPath:        idShortPath,
		isFromSubmodelElementList: false, // Children of collection are not from list
		position:                  i,
	})
	return stack
}

func addNestedElementToStackWithIndexPath(submodelElementList *gen.SubmodelElementList, index int, idShortPath string, stack []ElementToProcess, newParentId int) []ElementToProcess {
	nestedElement := submodelElementList.Value[index]
	nestedIdShortPath := idShortPath + "[" + strconv.Itoa(index) + "]"
	stack = append(stack, ElementToProcess{
		element:                   nestedElement,
		parentId:                  newParentId,
		currentIdShortPath:        nestedIdShortPath,
		isFromSubmodelElementList: true,  // Children of list are from list
		position:                  index, // For lists, position is the actual index
	})
	return stack
}
