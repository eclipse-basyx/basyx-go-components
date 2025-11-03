/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

// Author: Prajwala Prabhakar Adiga ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )
package persistence_postgresql

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	_ "github.com/lib/pq" // PostgreSQL Treiber

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	submodel_persistence "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel"
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

func NewPostgreSQLSubmodelBackend(dsn string, maxOpenConns, maxIdleConns int, connMaxLifetimeMinutes int, cacheEnabled bool, databaseSchema string) (*PostgreSQLSubmodelDatabase, error) {
	db, err := common.InitializeDatabase(dsn, databaseSchema)
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
	if limit == 0 {
		limit = 100
	}
	sm, cursor, err := submodel_persistence.GetAllSubmodels(p.db, int64(limit), cursor, nil)
	if err != nil {
		return nil, "", err
	}
	result := []gen.Submodel{}

	for _, s := range sm {
		if s != nil {
			result = append(result, *s)
		}
	}

	return result, cursor, nil
}

// get submodel metadata
func (p *PostgreSQLSubmodelDatabase) GetAllSubmodelsMetadata(
	limit int32,
	cursor string,
	idShort string,
	semanticID string,
) ([]gen.Submodel, string, error) {

	tx, err := p.db.Begin()
	if limit <= 0 {
		limit = 100
	}

	if err != nil {
		return nil, "", beginTransactionErrorSubmodelRepo
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	query := `
		SELECT 
			s.id, 
			s.id_short, 
			s.category, 
			s.kind, 
			s.model_type, 
			r.type AS semantic_reference_type,
			rk.type AS key_type,
   			rk.value AS key_value
		FROM submodel s
		LEFT JOIN reference r ON s.semantic_id = r.id
		LEFT JOIN reference_key rk ON r.id = rk.reference_id
		WHERE ($1 = '' OR s.id_short ILIKE '%' || $1 || '%')
		ORDER BY s.id
		LIMIT $2;
	`

	rows, err := p.db.Query(query, idShort, limit)
	if err != nil {
		tx.Rollback()
		fmt.Println("Error querying submodel metadata:", err)
		return nil, "", err
	}
	defer rows.Close()

	var submodels []gen.Submodel
	for rows.Next() {
		var sm gen.Submodel
		var refType, keyType, keyValue sql.NullString

		err := rows.Scan(
			&sm.ID,
			&sm.IdShort,
			&sm.Category,
			&sm.Kind,
			&sm.ModelType,
			&refType,
			&keyType,
			&keyValue,
		)
		if err != nil {
			fmt.Println("Error scanning metadata row:", err)
			return nil, "", err
		}
		if refType.Valid {
			ref := gen.Reference{
				Type: gen.ReferenceTypes(refType.String),
			}
			// Only add keys if both type and value are valid
			if keyType.Valid && keyValue.Valid {
				ref.Keys = []gen.Key{{
					Type:  gen.KeyTypes(keyType.String),
					Value: keyValue.String,
				}}
			}
			sm.SemanticID = &ref
		}
		submodels = append(submodels, sm)
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return nil, "", failedPostgresTransactionSubmodelRepo
	}

	return submodels, "", nil
}

// GetSubmodel returns one Submodel by id
func (p *PostgreSQLSubmodelDatabase) GetSubmodel(id string) (gen.Submodel, error) {
	sm, err := submodel_persistence.GetSubmodelByID(p.db, id)
	if err != nil {
		return gen.Submodel{}, err
	}

	return *sm, nil
}

// DeleteSubmodel deletes a Submodel by id
func (p *PostgreSQLSubmodelDatabase) DeleteSubmodel(id string) error {
	// Check cache first
	if p.cacheEnabled {
		delete(submodelCache, id)
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

	var semanticIdDbID, displayNameID, descriptionID, administrationID sql.NullInt64

	semanticIdDbID, err = persistence_utils.CreateReference(tx, sm.SemanticID, sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create SemanticID - no changes applied - see console for details")
	}

	displayNameID, err = persistence_utils.CreateLangStringNameTypes(tx, sm.DisplayName)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}

	// Handle possibly nil Description
	var convertedDescription []gen.LangStringText
	for _, desc := range sm.Description {
		convertedDescription = append(convertedDescription, desc)
	}
	descriptionID, err = persistence_utils.CreateLangStringTextTypes(tx, convertedDescription)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}

	administrationID, err = persistence_utils.CreateAdministrativeInformation(tx, sm.Administration)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}

	const q = `
        INSERT INTO submodel (id, id_short, category, kind, model_type, semantic_id, displayname_id, description_id, administration_id)
        VALUES ($1, $2, $3, $4, 'Submodel', $5, $6, $7, $8)
        ON CONFLICT (id) DO NOTHING
    `

	_, err = tx.Exec(q, sm.ID, sm.IdShort, sm.Category, sm.Kind, semanticIdDbID, displayNameID, descriptionID, administrationID)
	if err != nil {
		return err
	}

	if sm.SupplementalSemanticIds != nil {
		err = persistence_utils.InsertSupplementalSemanticIDsSubmodel(tx, sm.ID, sm.SupplementalSemanticIds)
		if err != nil {
			return err
		}
	}

	if sm.EmbeddedDataSpecifications != nil {
		for _, eds := range sm.EmbeddedDataSpecifications {
			edsDbID, err := persistence_utils.CreateEmbeddedDataSpecification(tx, eds)
			if err != nil {
				return err
			}
			_, err = tx.Exec("INSERT INTO submodel_embedded_data_specification(submodel_id, embedded_data_specification_id) VALUES ($1, $2)", sm.ID, edsDbID)
			if err != nil {
				return err
			}
		}
	}

	if len(sm.SubmodelElements) > 0 {
		for _, element := range sm.SubmodelElements {
			err = p.AddSubmodelElementWithTransaction(tx, sm.ID, element)
			if err != nil {
				return err
			}
		}
	}

	if len(sm.Qualifier) > 0 {
		for _, qualifier := range sm.Qualifier {
			qualifierID, err := persistence_utils.CreateQualifier(tx, qualifier)
			if err != nil {
				return err
			}
			_, err = tx.Exec(`INSERT INTO submodel_qualifier(submodel_id, qualifier_id) VALUES($1, $2)`, sm.ID, qualifierID)
			if err != nil {
				fmt.Println(err)
				return common.NewInternalServerError("Failed to Create Qualifier for Submodel with ID '" + sm.ID + "'. See console for details.")
			}
		}
	}

	if len(sm.Extension) > 0 {
		for _, extension := range sm.Extension {
			qualifierID, err := persistence_utils.CreateExtension(tx, extension)
			if err != nil {
				return err
			}
			_, err = tx.Exec(`INSERT INTO submodel_extension(submodel_id, extension_id) VALUES($1, $2)`, sm.ID, qualifierID)
			if err != nil {
				fmt.Println(err)
				return common.NewInternalServerError("Failed to Create Extension for Submodel with ID '" + sm.ID + "'. See console for details.")
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
			submodelCache[sm.ID] = sm
		}
	}
	return nil
}

func (p *PostgreSQLSubmodelDatabase) GetSubmodelElement(submodelID string, idShortOrPath string, limit int, cursor string) (gen.SubmodelElement, error) {
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

	elements, _, err := submodelelements.GetSubmodelElementsWithPath(p.db, tx, submodelID, idShortOrPath, limit, cursor)
	if err != nil {
		return nil, err
	}

	if len(elements) == 0 {
		return nil, common.NewErrNotFound("SubmodelElement with idShort or path '" + idShortOrPath + "' not found in submodel '" + submodelID + "'")
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return nil, failedPostgresTransactionSubmodelRepo
	}

	return elements[0], nil
}

func (p *PostgreSQLSubmodelDatabase) GetSubmodelElements(submodelID string, limit int, cursor string) ([]gen.SubmodelElement, string, error) {
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

	elements, cursor, err := submodelelements.GetSubmodelElementsWithPath(p.db, tx, submodelID, "", limit, cursor)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return nil, "", failedPostgresTransactionSubmodelRepo
	}

	return elements, cursor, nil
}

func (p *PostgreSQLSubmodelDatabase) AddSubmodelElementWithPath(submodelID string, idShortPath string, submodelElement gen.SubmodelElement) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		delete(submodelCache, submodelID)
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

	parentID, err := crud.GetDatabaseID(idShortPath)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to execute PostgreSQL Query - no changes applied - see console for details.")
	}
	nextPosition, err := crud.GetNextPosition(parentID)
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
	id, err := handler.CreateNested(tx, submodelID, parentID, newIdShortPath, submodelElement, nextPosition)
	if err != nil {
		return err
	}
	err = p.AddNestedSubmodelElementsIteratively(tx, submodelID, id, submodelElement, newIdShortPath)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return failedPostgresTransactionSubmodelRepo
	}

	return nil
}
func (p *PostgreSQLSubmodelDatabase) AddSubmodelElement(submodelID string, submodelElement gen.SubmodelElement) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		delete(submodelCache, submodelID)
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

	err = p.AddSubmodelElementWithTransaction(tx, submodelID, submodelElement)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return failedPostgresTransactionSubmodelRepo
	}

	return nil
}

func (p *PostgreSQLSubmodelDatabase) AddSubmodelElementWithTransaction(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		delete(submodelCache, submodelID)
	}
	handler, err := submodelelements.GetSMEHandler(submodelElement, p.db)
	if err != nil {
		return err
	}
	parentID, err := handler.Create(tx, submodelID, submodelElement)
	if err != nil {
		return err
	}

	err = p.AddNestedSubmodelElementsIteratively(tx, submodelID, parentID, submodelElement, "")
	if err != nil {
		return err
	}
	return nil
}

type ElementToProcess struct {
	element                   gen.SubmodelElement
	parentID                  int
	currentIdShortPath        string
	isFromSubmodelElementList bool // Indicates if the current element is from a SubmodelElementList
	position                  int  // Position/index within the parent collection or list
}

func (p *PostgreSQLSubmodelDatabase) AddNestedSubmodelElementsIteratively(tx *sql.Tx, submodelID string, topLevelParentID int, topLevelElement gen.SubmodelElement, startPath string) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		delete(submodelCache, submodelID)
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
				parentID:                  topLevelParentID,
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
				parentID:                  topLevelParentID,
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

		newParentID, err := handler.CreateNested(tx, submodelID, current.parentID, idShortPath, current.element, current.position)
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
				stack = addNestedElementToStackWithNormalPath(submodelElementCollection, i, stack, newParentID, idShortPath)
			}
		case "SubmodelElementList":
			submodelElementList, ok := current.element.(*gen.SubmodelElementList)
			if !ok {
				return common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementList' is not of type SubmodelElementList")
			}
			for index := len(submodelElementList.Value) - 1; index >= 0; index-- {
				stack = addNestedElementToStackWithIndexPath(submodelElementList, index, idShortPath, stack, newParentID)
			}
		}
	}

	return nil
}

// This method removes a SubmodelElement by its idShort or path and all its nested elements
// If the deleted Element is in a SubmodelElementList, the indices of the remaining elements are adjusted accordingly
func (p *PostgreSQLSubmodelDatabase) DeleteSubmodelElementByPath(submodelID string, idShortOrPath string) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		delete(submodelCache, submodelID)
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
	err = submodelelements.DeleteSubmodelElementByPath(tx, submodelID, idShortOrPath)
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

func addNestedElementToStackWithNormalPath(submodelElementCollection *gen.SubmodelElementCollection, i int, stack []ElementToProcess, newParentID int, idShortPath string) []ElementToProcess {
	nestedElement := submodelElementCollection.Value[i]
	stack = append(stack, ElementToProcess{
		element:                   nestedElement,
		parentID:                  newParentID,
		currentIdShortPath:        idShortPath,
		isFromSubmodelElementList: false, // Children of collection are not from list
		position:                  i,
	})
	return stack
}

func addNestedElementToStackWithIndexPath(submodelElementList *gen.SubmodelElementList, index int, idShortPath string, stack []ElementToProcess, newParentID int) []ElementToProcess {
	nestedElement := submodelElementList.Value[index]
	nestedIdShortPath := idShortPath + "[" + strconv.Itoa(index) + "]"
	stack = append(stack, ElementToProcess{
		element:                   nestedElement,
		parentID:                  newParentID,
		currentIdShortPath:        nestedIdShortPath,
		isFromSubmodelElementList: true,  // Children of list are from list
		position:                  index, // For lists, position is the actual index
	})
	return stack
}
