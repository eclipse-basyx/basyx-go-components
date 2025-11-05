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

// Package persistencepostgresql provides PostgreSQL-based persistence implementation for the submodel repository.
// This package contains the database layer implementation for managing submodels and their elements
// using PostgreSQL as the backend storage system.
//
// Author: Prajwala Prabhakar Adiga ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )
package persistencepostgresql

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	_ "github.com/lib/pq" // PostgreSQL Treiber

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	submodelpersistence "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/SubmodelElements"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

// PostgreSQLSubmodelDatabase represents a PostgreSQL-based implementation of the submodel repository database.
// It provides methods for CRUD operations on submodels and their elements with optional caching support.
type PostgreSQLSubmodelDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

var failedPostgresTransactionSubmodelRepo = common.NewInternalServerError("Failed to commit PostgreSQL transaction - no changes applied - see console for details")
var beginTransactionErrorSubmodelRepo = common.NewInternalServerError("Failed to begin PostgreSQL transaction - no changes applied - see console for details")

var maxCacheSize = 1000

// InMemory Cache for submodels
var submodelCache map[string]gen.Submodel = make(map[string]gen.Submodel)

// NewPostgreSQLSubmodelBackend creates a new PostgreSQL submodel database backend.
// It initializes a database connection with the provided DSN and schema configuration.
//
// Parameters:
//   - dsn: Data Source Name for PostgreSQL connection
//   - maxOpenConns: Maximum number of open connections to the database
//   - maxIdleConns: Maximum number of idle connections in the pool
//   - connMaxLifetimeMinutes: Maximum lifetime of a connection in minutes
//   - cacheEnabled: Whether to enable in-memory caching for submodels
//   - databaseSchema: Database schema to use
//
// Returns:
//   - *PostgreSQLSubmodelDatabase: Configured database instance
//   - error: Error if database initialization fails
func NewPostgreSQLSubmodelBackend(dsn string, _ /* maxOpenConns */, _ /* maxIdleConns */ int, _ /* connMaxLifetimeMinutes */ int, cacheEnabled bool, databaseSchema string) (*PostgreSQLSubmodelDatabase, error) {
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLSubmodelDatabase{db: db, cacheEnabled: cacheEnabled}, nil
}

// GetDB returns the underlying SQL database connection.
// This method provides access to the raw database connection for advanced operations.
//
// Returns:
//   - *sql.DB: The PostgreSQL database connection
func (p *PostgreSQLSubmodelDatabase) GetDB() *sql.DB {
	return p.db
}

// GetAllSubmodels retrieves a paginated list of all submodels from the database.
// This method supports pagination through cursor-based navigation and optional filtering by idShort.
//
// Parameters:
//   - limit: Maximum number of submodels to return (defaults to 100 if 0)
//   - cursor: Pagination cursor for retrieving next page (empty string for first page)
//   - idShort: Optional filter by submodel idShort (not currently implemented)
//
// Returns:
//   - []gen.Submodel: List of submodels
//   - string: Next cursor for pagination (empty if no more pages)
//   - error: Error if retrieval fails
func (p *PostgreSQLSubmodelDatabase) GetAllSubmodels(limit int32, cursor string, _ /* idShort */ string) ([]gen.Submodel, string, error) {
	if limit == 0 {
		limit = 100
	}
	sm, cursor, err := submodelpersistence.GetAllSubmodels(p.db, int64(limit), cursor, nil)
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

// GetAllSubmodelsMetadata retrieves metadata for all submodels without their full content.
// This method is optimized for scenarios where only basic submodel information is needed,
// excluding the full submodel elements tree.
//
// Parameters:
//   - limit: Maximum number of submodels to return (defaults to 100 if <= 0)
//   - cursor: Pagination cursor for retrieving next page (not currently implemented)
//   - idShort: Optional filter by submodel idShort (supports partial matching with ILIKE)
//   - semanticID: Optional filter by semantic ID (not currently implemented)
//
// Returns:
//   - []gen.Submodel: List of submodels with metadata only
//   - string: Next cursor for pagination (currently returns empty string)
//   - error: Error if retrieval fails
func (p *PostgreSQLSubmodelDatabase) GetAllSubmodelsMetadata(
	limit int32,
	_ /* cursor */ string,
	idShort string,
	_ /* semanticID */ string,
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
			_ = tx.Rollback()
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
		_ = tx.Rollback()
		fmt.Println("Error querying submodel metadata:", err)
		return nil, "", err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			fmt.Println("Error closing rows:", closeErr)
		}
	}()

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

// GetSubmodel retrieves a complete submodel by its ID.
// This method returns the full submodel including all its submodel elements and metadata.
//
// Parameters:
//   - id: Unique identifier of the submodel to retrieve
//
// Returns:
//   - gen.Submodel: The complete submodel with all its elements
//   - error: Error if submodel not found or retrieval fails
func (p *PostgreSQLSubmodelDatabase) GetSubmodel(id string) (gen.Submodel, error) {
	sm, err := submodelpersistence.GetSubmodelByID(p.db, id)
	if err != nil {
		return gen.Submodel{}, err
	}

	return *sm, nil
}

// DeleteSubmodel removes a submodel and all its associated data from the database.
// This operation also removes the submodel from the cache if caching is enabled.
// The deletion cascades to remove all related submodel elements and references.
//
// Parameters:
//   - id: Unique identifier of the submodel to delete
//
// Returns:
//   - error: Error if deletion fails or submodel not found (sql.ErrNoRows)
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
			_ = tx.Rollback()
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

// CreateSubmodel inserts a new submodel into the database.
// If a submodel with the same ID already exists, the operation is ignored (ON CONFLICT DO NOTHING).
// This method creates all associated elements including submodel elements, qualifiers, extensions,
// and embedded data specifications. The model_type is automatically set to "Submodel".
//
// Parameters:
//   - sm: The submodel to create with all its properties and elements
//
// Returns:
//   - error: Error if creation fails, nil if successful or if submodel already exists
func (p *PostgreSQLSubmodelDatabase) CreateSubmodel(sm gen.Submodel) error {
	tx, err := p.db.Begin()

	if err != nil {
		fmt.Println(err)
		return beginTransactionErrorSubmodelRepo
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var semanticIDDbID, displayNameID, descriptionID, administrationID sql.NullInt64

	semanticIDDbID, err = persistenceutils.CreateReference(tx, sm.SemanticID, sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create SemanticID - no changes applied - see console for details")
	}

	displayNameID, err = persistenceutils.CreateLangStringNameTypes(tx, sm.DisplayName)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}

	// Handle possibly nil Description
	var convertedDescription []gen.LangStringText
	for _, desc := range sm.Description {
		convertedDescription = append(convertedDescription, desc)
	}
	descriptionID, err = persistenceutils.CreateLangStringTextTypes(tx, convertedDescription)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}

	administrationID, err = persistenceutils.CreateAdministrativeInformation(tx, sm.Administration)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}

	edsJSONString := "[]"
	if sm.EmbeddedDataSpecifications != nil {
		edsBytes, err := json.Marshal(sm.EmbeddedDataSpecifications)
		if err != nil {
			fmt.Println(err)
			return common.NewInternalServerError("Failed to marshal EmbeddedDataSpecifications - no changes applied - see console for details")
		}
		if edsBytes != nil {
			edsJSONString = string(edsBytes)
		}
	}

	const q = `
        INSERT INTO submodel (id, id_short, category, kind, model_type, semantic_id, displayname_id, description_id, administration_id, embedded_data_specification)
        VALUES ($1, $2, $3, $4, 'Submodel', $5, $6, $7, $8, $9)
        ON CONFLICT (id) DO NOTHING
    `

	_, err = tx.Exec(q, sm.ID, sm.IdShort, sm.Category, sm.Kind, semanticIDDbID, displayNameID, descriptionID, administrationID, edsJSONString)
	if err != nil {
		return err
	}

	if sm.SupplementalSemanticIds != nil {
		err = persistenceutils.InsertSupplementalSemanticIDsSubmodel(tx, sm.ID, sm.SupplementalSemanticIds)
		if err != nil {
			return err
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
		for i, qualifier := range sm.Qualifier {
			qualifierID, err := persistenceutils.CreateQualifier(tx, qualifier, i)
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
		for i, extension := range sm.Extension {
			qualifierID, err := persistenceutils.CreateExtension(tx, extension, i)
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

// GetSubmodelElement retrieves a single submodel element by its idShort or path.
// The path can be a simple idShort for top-level elements or a hierarchical path
// using dot notation for nested elements (e.g., "collection.property").
//
// Parameters:
//   - submodelID: ID of the submodel containing the element
//   - idShortOrPath: idShort or hierarchical path to the element
//   - limit: Maximum number of elements to return (used for pagination in collections)
//   - cursor: Pagination cursor (not currently implemented)
//
// Returns:
//   - gen.SubmodelElement: The requested submodel element
//   - error: Error if element not found or retrieval fails
func (p *PostgreSQLSubmodelDatabase) GetSubmodelElement(submodelID string, idShortOrPath string, limit int, cursor string) (gen.SubmodelElement, error) {
	tx, err := p.db.Begin()
	if err != nil {
		fmt.Println(err)
		return nil, beginTransactionErrorSubmodelRepo
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
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

// GetSubmodelElements retrieves all top-level submodel elements for a given submodel.
// This method supports pagination through cursor-based navigation.
//
// Parameters:
//   - submodelID: ID of the submodel whose elements to retrieve
//   - limit: Maximum number of elements to return
//   - cursor: Pagination cursor for retrieving next page
//
// Returns:
//   - []gen.SubmodelElement: List of submodel elements
//   - string: Next cursor for pagination
//   - error: Error if retrieval fails
func (p *PostgreSQLSubmodelDatabase) GetSubmodelElements(submodelID string, limit int, cursor string) ([]gen.SubmodelElement, string, error) {
	tx, err := p.db.Begin()
	if err != nil {
		fmt.Println(err)
		return nil, "", beginTransactionErrorSubmodelRepo
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
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

// AddSubmodelElementWithPath adds a new submodel element at a specific path within the hierarchy.
// This method allows adding elements to collections or lists at specific positions.
// For SubmodelElementList, the path uses index notation (e.g., "list[0]").
// For SubmodelElementCollection, the path uses dot notation (e.g., "collection.element").
//
// Parameters:
//   - submodelID: ID of the submodel to add the element to
//   - idShortPath: Hierarchical path where to add the element
//   - submodelElement: The element to add
//
// Returns:
//   - error: Error if addition fails or target path is invalid
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
			_ = tx.Rollback()
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
	var newIDShortPath string
	if modelType == "SubmodelElementList" {
		newIDShortPath = idShortPath + "[" + strconv.Itoa(nextPosition) + "]"
	} else {
		newIDShortPath = idShortPath + "." + submodelElement.GetIdShort()
	}
	id, err := handler.CreateNested(tx, submodelID, parentID, newIDShortPath, submodelElement, nextPosition)
	if err != nil {
		return err
	}
	err = p.AddNestedSubmodelElementsIteratively(tx, submodelID, id, submodelElement, newIDShortPath)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return failedPostgresTransactionSubmodelRepo
	}

	return nil
}

// AddSubmodelElement adds a new submodel element as a top-level element to the specified submodel.
// This method invalidates the submodel cache if caching is enabled and handles all nested elements recursively.
//
// Parameters:
//   - submodelID: ID of the submodel to add the element to
//   - submodelElement: The element to add at the top level
//
// Returns:
//   - error: Error if addition fails
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
			_ = tx.Rollback()
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

// AddSubmodelElementWithTransaction adds a submodel element within an existing database transaction.
// This method is used internally when creating submodels or when multiple operations need to be atomic.
// It invalidates the submodel cache if caching is enabled.
//
// Parameters:
//   - tx: Active database transaction
//   - submodelID: ID of the submodel to add the element to
//   - submodelElement: The element to add
//
// Returns:
//   - error: Error if addition fails
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

// ElementToProcess represents a submodel element to be processed during iterative creation.
// This struct is used internally by the AddNestedSubmodelElementsIteratively method to manage
// the stack-based processing of nested elements in collections and lists.
type ElementToProcess struct {
	element                   gen.SubmodelElement
	parentID                  int
	currentIDShortPath        string
	isFromSubmodelElementList bool // Indicates if the current element is from a SubmodelElementList
	position                  int  // Position/index within the parent collection or list
}

// AddNestedSubmodelElementsIteratively processes and creates nested submodel elements using a stack-based approach.
// This method handles both SubmodelElementCollection and SubmodelElementList types, ensuring proper
// hierarchical path construction and position management. It invalidates the submodel cache if caching is enabled.
//
// Parameters:
//   - tx: Active database transaction
//   - submodelID: ID of the parent submodel
//   - topLevelParentID: Database ID of the top-level parent element
//   - topLevelElement: The top-level element containing nested elements
//   - startPath: Starting path for nested element hierarchy (empty for top-level)
//
// Returns:
//   - error: Error if processing fails
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
				currentIDShortPath:        currentPath,
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
				currentIDShortPath:        idShortPath,
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
		idShortPath := buildCurrentIDShortPath(current)

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

// DeleteSubmodelElementByPath removes a submodel element and all its nested elements by path.
// This method supports hierarchical paths using dot notation for collections and index notation for lists.
// For SubmodelElementList, the indices of remaining elements are automatically adjusted after deletion.
// The submodel cache is invalidated if caching is enabled.
//
// Parameters:
//   - submodelID: ID of the submodel containing the element
//   - idShortOrPath: idShort or hierarchical path to the element to delete
//
// Returns:
//   - error: Error if deletion fails or element not found
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
			_ = tx.Rollback()
		}
	}()
	err = submodelelements.DeleteSubmodelElementByPath(tx, submodelID, idShortOrPath)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func buildCurrentIDShortPath(current ElementToProcess) string {
	var idShortPath string
	if current.currentIDShortPath == "" {
		idShortPath = current.element.GetIdShort()
	} else {
		// If element comes from a SubmodelElementList, use the path as-is (includes [index])
		if current.isFromSubmodelElementList {
			idShortPath = current.currentIDShortPath
		} else {
			// For SubmodelElementCollection, append element's idShort with dot notation
			idShortPath = current.currentIDShortPath + "." + current.element.GetIdShort()
		}
	}
	return idShortPath
}

func addNestedElementToStackWithNormalPath(submodelElementCollection *gen.SubmodelElementCollection, i int, stack []ElementToProcess, newParentID int, idShortPath string) []ElementToProcess {
	nestedElement := submodelElementCollection.Value[i]
	stack = append(stack, ElementToProcess{
		element:                   nestedElement,
		parentID:                  newParentID,
		currentIDShortPath:        idShortPath,
		isFromSubmodelElementList: false, // Children of collection are not from list
		position:                  i,
	})
	return stack
}

func addNestedElementToStackWithIndexPath(submodelElementList *gen.SubmodelElementList, index int, idShortPath string, stack []ElementToProcess, newParentID int) []ElementToProcess {
	nestedElement := submodelElementList.Value[index]
	nestedIDShortPath := idShortPath + "[" + strconv.Itoa(index) + "]"
	stack = append(stack, ElementToProcess{
		element:                   nestedElement,
		parentID:                  newParentID,
		currentIDShortPath:        nestedIDShortPath,
		isFromSubmodelElementList: true,  // Children of list are from list
		position:                  index, // For lists, position is the actual index
	})
	return stack
}
