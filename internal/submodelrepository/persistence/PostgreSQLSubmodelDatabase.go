package persistence_postgresql

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	_ "github.com/lib/pq" // PostgreSQL Treiber

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/SubmodelElements"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

type PostgreSQLSubmodelDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

var failedPostgresTransactionSubmodelRepo = common.NewInternalServerError("Failed to commit PostgreSQL transaction - no changes applied - see console for details")
var beginTransactionErrorSubmodelRepo = common.NewInternalServerError("Failed to begin PostgreSQL transaction - no changes applied - see console for details")

var maxCacheSize = 1000

// InMemory Cache for submodels
var submodelCache map[string]gen.Submodel = make(map[string]gen.Submodel)

func NewPostgreSQLSubmodelBackend(dsn string, maxOpenConns, maxIdleConns int, connMaxLifetimeMinutes int, cacheEnabled bool) (*PostgreSQLSubmodelDatabase, error) {
	db, err := common.InitializeDatabase(dsn, "submodelrepositoryschema.sql")
	if err != nil {
		return nil, err
	}
	return &PostgreSQLSubmodelDatabase{db: db, cacheEnabled: cacheEnabled}, nil
}

func (p *PostgreSQLSubmodelDatabase) GetDB() *sql.DB {
	return p.db
}

// GetAllSubmodels and a next cursor ("" if no more pages).
func (p *PostgreSQLSubmodelDatabase) GetAllSubmodels(limit int32, cursor string, idShort string) ([]gen.Submodel, string, error) {
	tx, err := p.db.Begin()
	if limit <= 0 {
		limit = 100
	}

	if err != nil {
		fmt.Println(err)
		return nil, "", beginTransactionErrorSubmodelRepo
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	sm, err := submodelelements.GetSubmodelWithSubmodelElementsOrAll(p.db, tx)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return nil, "", failedPostgresTransactionSubmodelRepo
	}

	return sm, "", nil
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
		return gen.Submodel{}, beginTransactionErrorSubmodelRepo
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
		return gen.Submodel{}, failedPostgresTransactionSubmodelRepo
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
		return beginTransactionErrorSubmodelRepo
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
		return failedPostgresTransactionSubmodelRepo
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
		return beginTransactionErrorSubmodelRepo
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var semanticIdDbId, displayNameId, descriptionId, administrationId sql.NullInt64

	semanticIdDbId, err = persistence_utils.CreateReference(tx, sm.SemanticId, sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create SemanticId - no changes applied - see console for details")
	}

	// if sm.SemanticId.ReferredSemanticId != nil {
	// 	lastParentId := semanticIdDbId
	// 	stack := []*gen.Reference{sm.SemanticId.ReferredSemanticId}
	// 	for len(stack) > 0 {
	// 		currentElement := stack[len(stack)-1]
	// 		if currentElement != nil {
	// 			// Pop current from stack
	// 			stack = stack[:len(stack)-1]
	// 			// First push next element to stack if exists
	// 			if currentElement.ReferredSemanticId != nil {
	// 				stack = append(stack, currentElement.ReferredSemanticId)
	// 			}

	// 			// Save current element to DB using currentElement instead of sm.SemanticId
	// 			insertedId, err := persistence_utils.CreateReference(tx, currentElement, lastParentId, semanticIdDbId)
	// 			if err != nil {
	// 				return err
	// 			}
	// 			lastParentId = insertedId

	// 		}
	// 	}
	// }

	displayNameId, err = persistence_utils.CreateLangStringNameTypes(tx, sm.DisplayName)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}

	// Handle possibly nil Description
	descriptionId, err = persistence_utils.CreateLangStringTextTypes(tx, sm.Description)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}

	administrationId, err = persistence_utils.CreateAdministrativeInformation(tx, sm.Administration)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}

	const q = `
        INSERT INTO submodel (id, id_short, category, kind, model_type, semantic_id, displayname_id, description_id, administration_id)
        VALUES ($1, $2, $3, $4, 'Submodel', $5, $6, $7, $8)
        ON CONFLICT (id) DO NOTHING
    `

	_, err = tx.Exec(q, sm.Id, sm.IdShort, sm.Category, sm.Kind, semanticIdDbId, displayNameId, descriptionId, administrationId)
	if err != nil {
		return err
	}

	if sm.SupplementalSemanticIds != nil {
		err = persistence_utils.InsertSupplementalSemanticIds(tx, sm.Id, sm.SupplementalSemanticIds)
		if err != nil {
			return err
		}
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
		return failedPostgresTransactionSubmodelRepo
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
		return nil, beginTransactionErrorSubmodelRepo
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
		return nil, failedPostgresTransactionSubmodelRepo
	}

	return elements[0], nil
}

func (p *PostgreSQLSubmodelDatabase) GetSubmodelElements(submodelId string, limit int, cursor string) ([]gen.SubmodelElement, string, error) {
	tx, err := p.db.Begin()
	if err != nil {
		fmt.Println(err)
		return nil, "", beginTransactionErrorSubmodelRepo
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
		return nil, "", failedPostgresTransactionSubmodelRepo
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
		return beginTransactionErrorSubmodelRepo
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
		return failedPostgresTransactionSubmodelRepo
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
		return beginTransactionErrorSubmodelRepo
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
		return failedPostgresTransactionSubmodelRepo
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
