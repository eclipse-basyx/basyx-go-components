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
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"

	_ "github.com/lib/pq" // PostgreSQL Treiber

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // Postgres Driver for Goqu
	"golang.org/x/sync/errgroup"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	submodelpersistence "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel/submodelElements"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

// PostgreSQLSubmodelDatabase represents a PostgreSQL-based implementation of the submodel repository database.
// It provides methods for CRUD operations on submodels and their elements with optional caching support.
type PostgreSQLSubmodelDatabase struct {
	db *sql.DB
}

var failedPostgresTransactionSubmodelRepo = common.NewInternalServerError("Failed to commit PostgreSQL transaction - no changes applied - see console for details")
var beginTransactionErrorSubmodelRepo = common.NewInternalServerError("Failed to begin PostgreSQL transaction - no changes applied - see console for details")

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
func NewPostgreSQLSubmodelBackend(dsn string, _ int32 /* maxOpenConns */, _ /* maxIdleConns */ int, _ /* connMaxLifetimeMinutes */ int, databaseSchema string) (*PostgreSQLSubmodelDatabase, error) {
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLSubmodelDatabase{db: db}, nil
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
func (p *PostgreSQLSubmodelDatabase) GetAllSubmodels(limit int32, cursor string, _ /* idShort */ string, valueOnly bool) ([]gen.Submodel, string, error) {
	if limit == 0 {
		limit = 100
	}

	submodelIDs := []string{}
	rows, err := submodelpersistence.GetSubmodelDataFromDbWithJSONQuery(p.db, "", int64(limit), cursor, nil, true)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_, _ = fmt.Println("Error closing rows:", closeErr)
		}
	}()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, "", err
		}
		submodelIDs = append(submodelIDs, id)
	}

	var wg errgroup.Group
	resultChan := make(chan result, 1)

	wg.Go(func() error {
		sm, smMap, cursor, err := submodelpersistence.GetAllSubmodels(p.db, int64(limit), cursor, nil)
		resultChan <- result{sm: sm, smMap: smMap, cursor: cursor, err: err}
		return err
	})

	submodelElements := make(map[string][]gen.SubmodelElement)
	var errSme error
	var errSmeMutex sync.Mutex

	numWorkers := 10
	jobs := make(chan smeJob, len(submodelIDs))
	results := make(chan smeResult, len(submodelIDs))

	for range numWorkers {
		go func() {
			for job := range jobs {
				smes, _, err := submodelelements.GetSubmodelElementsForSubmodel(p.db, job.id, "", "", -1, valueOnly)
				results <- smeResult{id: job.id, smes: smes, err: err}
			}
		}()
	}

	for _, id := range submodelIDs {
		jobs <- smeJob{id: id}
	}
	close(jobs)

	for i := 0; i < len(submodelIDs); i++ {
		res := <-results
		if res.err != nil {
			errSmeMutex.Lock()
			if errSme == nil {
				errSme = res.err
			}
			errSmeMutex.Unlock()
		} else {
			submodelElements[res.id] = res.smes
		}
	}
	if err := wg.Wait(); err != nil {
		return nil, "", err
	}
	res := <-resultChan

	if res.err != nil {
		return nil, "", res.err
	}

	if errSme != nil {
		return nil, "", errSme
	}

	submodels := []gen.Submodel{}

	for _, s := range res.sm {
		if s != nil {
			// Add corresponding submodel elements BEFORE copying
			if smes, exists := submodelElements[s.ID]; exists {
				s.SubmodelElements = smes
			}
			submodels = append(submodels, *s)
		}
	}

	return submodels, res.cursor, nil
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

	selectQuery := goqu.Select(
		"s.id",
		"s.id_short",
		"s.category",
		"s.kind",
		"s.model_type",
		goqu.I("r.type").As("semantic_reference_type"),
		goqu.I("rk.type").As("key_type"),
		goqu.I("rk.value").As("key_value"),
	).From(goqu.T("submodel").As("s")).LeftJoin(
		goqu.T("reference").As("r"),
		goqu.On(goqu.I("s.semantic_id").Eq(goqu.I("r.id"))),
	).LeftJoin(
		goqu.T("reference_key").As("rk"),
		goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
	)

	if idShort != "" {
		selectQuery = selectQuery.Where(goqu.I("s.id_short").ILike("%" + idShort + "%"))
	}
	if limit < 0 {
		return nil, "", common.NewErrBadRequest("Limit has to be higher than 0")
	}
	selectQuery = selectQuery.Order(goqu.I("s.id").Asc()).Limit(uint(limit))

	query, args, err := selectQuery.ToSQL()
	if err != nil {
		_ = tx.Rollback()
		_, _ = fmt.Println("Error building query:", err)
		return nil, "", err
	}

	rows, err := p.db.Query(query, args...)
	if err != nil {
		_ = tx.Rollback()
		_, _ = fmt.Println("Error querying submodel metadata:", err)
		return nil, "", err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_, _ = fmt.Println("Error closing rows:", closeErr)
		}
	}()

	var submodels []gen.Submodel
	for rows.Next() {
		var sm gen.Submodel
		var refType, keyType, keyValue, category sql.NullString

		err := rows.Scan(
			&sm.ID,
			&sm.IdShort,
			&category,
			&sm.Kind,
			&sm.ModelType,
			&refType,
			&keyType,
			&keyValue,
		)
		if err != nil {
			_, _ = fmt.Println("Error scanning metadata row:", err)
			return nil, "", err
		}
		if category.Valid {
			sm.Category = category.String
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
		_, _ = fmt.Println(err)
		return nil, "", failedPostgresTransactionSubmodelRepo
	}

	return submodels, "", nil
}

// DoesSubmodelExist checks if a submodel with the given identifier exists in the database.
//
// Parameters:
//   - submodelIdentifier: Unique identifier of the submodel to check
//
// Returns:
//   - bool: True if the submodel exists, false otherwise
//   - error: Error if the query fails
func (p *PostgreSQLSubmodelDatabase) DoesSubmodelExist(submodelIdentifier string) (bool, error) {
	var count int
	sqlQuery, args, err := goqu.Select(goqu.COUNT("id")).
		From("submodel").
		Where(goqu.I("id").Eq(submodelIdentifier)).
		Limit(1).
		ToSQL()
	if err != nil {
		return false, err
	}
	err = p.db.QueryRow(sqlQuery, args...).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
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
func (p *PostgreSQLSubmodelDatabase) GetSubmodel(id string, valueOnly bool) (gen.Submodel, error) {
	type result struct {
		sm  *gen.Submodel
		err error
	}

	type resultSME struct {
		smes []gen.SubmodelElement
		err  error
	}

	var wg sync.WaitGroup
	resultChan := make(chan result, 1)
	resultChanSME := make(chan resultSME, 1)

	wg.Add(2)
	go func() {
		defer wg.Done()
		sm, err := submodelpersistence.GetSubmodelByID(p.db, id)
		resultChan <- result{sm: sm, err: err}
	}()

	go func() {
		defer wg.Done()
		smes, _, err := submodelelements.GetSubmodelElementsForSubmodel(p.db, id, "", "", -1, valueOnly)
		resultChanSME <- resultSME{smes: smes, err: err}
	}()

	wg.Wait()
	res := <-resultChan

	resSME := <-resultChanSME
	if resSME.err != nil {
		return gen.Submodel{}, resSME.err
	}

	if res.sm != nil {
		res.sm.SubmodelElements = resSME.smes
	}

	if res.err != nil {
		return gen.Submodel{}, res.err
	}

	return *res.sm, nil
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
	tx, cu, err := common.StartTransaction(p.db)

	if err != nil {
		_, _ = fmt.Println(err)
		return beginTransactionErrorSubmodelRepo
	}
	defer cu()

	del := goqu.Delete("submodel").Where(goqu.I("id").Eq(id))
	query, args, err := del.ToSQL()
	if err != nil {
		return err
	}
	res, err := tx.Exec(query, args...)
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
		_, _ = fmt.Println(err)
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
//
//nolint:revive // This function is already refactored in smaller parts, but further splitting it would reduce readability.
func (p *PostgreSQLSubmodelDatabase) CreateSubmodel(sm gen.Submodel) error {
	tx, cu, err := common.StartTransaction(p.db)

	if err != nil {
		_, _ = fmt.Println(err)
		return beginTransactionErrorSubmodelRepo
	}

	defer cu()

	var semanticIDDbID, displayNameID, descriptionID, administrationID sql.NullInt64

	semanticIDDbID, err = persistenceutils.CreateReference(tx, sm.SemanticID, sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to create SemanticID - no changes applied - see console for details")
	}

	displayNameID, err = persistenceutils.CreateLangStringNameTypes(tx, sm.DisplayName)
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}

	// Handle possibly nil Description
	var convertedDescription []gen.LangStringText
	for _, desc := range sm.Description {
		convertedDescription = append(convertedDescription, desc)
	}
	descriptionID, err = persistenceutils.CreateLangStringTextTypes(tx, convertedDescription)
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}

	administrationID, err = persistenceutils.CreateAdministrativeInformation(tx, sm.Administration)
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}

	edsJSONString, err := getEDSJSONStringFromSubmodel(&sm)
	if err != nil {
		return err
	}

	extensionJSONString, err := getExtensionJSONStringFromSubmodel(&sm)
	if err != nil {
		return err
	}

	supplementalSemanticIDs, err := getSupplementalSemanticIDsJSONStringFromSubmodel(&sm)
	if err != nil {
		return err
	}

	insert := goqu.Insert("submodel").Rows(goqu.Record{
		"id":                          sm.ID,
		"id_short":                    sm.IdShort,
		"category":                    sm.Category,
		"kind":                        sm.Kind,
		"model_type":                  "Submodel",
		"semantic_id":                 semanticIDDbID,
		"displayname_id":              displayNameID,
		"description_id":              descriptionID,
		"administration_id":           administrationID,
		"embedded_data_specification": edsJSONString,
		"extensions":                  extensionJSONString,
		"supplemental_semantic_ids":   supplementalSemanticIDs,
	}).OnConflict(goqu.DoNothing())
	sqlQuery, args, err := insert.ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlQuery, args...)
	if err != nil {
		return err
	}

	if len(sm.SubmodelElements) > 0 {
		for _, element := range sm.SubmodelElements {
			err = p.AddSubmodelElementWithTransaction(tx, sm.ID, element)
			if err != nil {
				return err
			}
		}
	}

	if len(sm.Qualifiers) > 0 {
		for i, qualifier := range sm.Qualifiers {
			qualifierID, err := persistenceutils.CreateQualifier(tx, qualifier, i)
			if err != nil {
				return err
			}
			insert := goqu.Insert("submodel_qualifier").Rows(goqu.Record{
				"submodel_id":  sm.ID,
				"qualifier_id": qualifierID,
			})
			query, args, err := insert.ToSQL()
			if err != nil {
				_, _ = fmt.Println(err)
				return common.NewInternalServerError("Failed to Create Qualifier for Submodel with ID '" + sm.ID + "'. See console for details.")
			}
			_, err = tx.Exec(query, args...)
			if err != nil {
				_, _ = fmt.Println(err)
				return common.NewInternalServerError("Failed to Create Qualifier for Submodel with ID '" + sm.ID + "'. See console for details.")
			}
		}
	}

	if err := tx.Commit(); err != nil {
		_, _ = fmt.Println(err)
		return failedPostgresTransactionSubmodelRepo
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
func (p *PostgreSQLSubmodelDatabase) GetSubmodelElement(submodelID string, idShortOrPath string, valueOnly bool) (gen.SubmodelElement, error) {
	tx, cu, err := common.StartTransaction(p.db)
	if err != nil {
		_, _ = fmt.Println(err)
		return nil, beginTransactionErrorSubmodelRepo
	}
	defer cu()

	elements, _, err := submodelelements.GetSubmodelElementsForSubmodel(p.db, submodelID, idShortOrPath, "", -1, valueOnly)
	if err != nil {
		return nil, err
	}

	if len(elements) == 0 {
		return nil, common.NewErrNotFound("SubmodelElement with idShort or path '" + idShortOrPath + "' not found in submodel '" + submodelID + "'")
	}

	if err := tx.Commit(); err != nil {
		_, _ = fmt.Println(err)
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
func (p *PostgreSQLSubmodelDatabase) GetSubmodelElements(submodelID string, limit int, cursor string, valueOnly bool) ([]gen.SubmodelElement, string, error) {
	tx, cu, err := common.StartTransaction(p.db)
	if err != nil {
		_, _ = fmt.Println(err)
		return nil, "", beginTransactionErrorSubmodelRepo
	}
	defer cu()

	if limit <= 0 {
		limit = 100
	}

	var count int
	sqlQuery, args, err := goqu.Select(goqu.COUNT("id")).From("submodel").Where(goqu.I("id").Eq(submodelID)).ToSQL()
	if err != nil {
		return nil, "", err
	}
	err = p.db.QueryRow(sqlQuery, args...).Scan(&count)
	if err != nil {
		return nil, "", err
	}
	if count == 0 {
		return nil, "", common.NewErrNotFound("Submodel with ID '" + submodelID + "' not found")
	}

	elements, cursor, err := submodelelements.GetSubmodelElementsForSubmodel(p.db, submodelID, "", cursor, limit, valueOnly)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(); err != nil {
		_, _ = fmt.Println(err)
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
	handler, err := submodelelements.GetSMEHandler(submodelElement, p.db)
	if err != nil {
		return err
	}

	crud, err := submodelelements.NewPostgreSQLSMECrudHandler(p.db)
	if err != nil {
		return err
	}

	tx, cu, err := common.StartTransaction(p.db)
	if err != nil {
		_, _ = fmt.Println(err)
		return beginTransactionErrorSubmodelRepo
	}

	defer cu()

	parentID, err := crud.GetDatabaseID(idShortPath)
	if err != nil {
		_, _ = fmt.Println(err)
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

	var rootSmeID int
	sqlQuery, args, err := goqu.Select("root_sme_id").From("submodel_element").Where(goqu.I("idshort_path").Eq(idShortPath)).ToSQL()
	if err != nil {
		return err
	}
	err = p.db.QueryRow(sqlQuery, args...).Scan(&rootSmeID)
	if err != nil {
		return err
	}

	id, err := handler.CreateNested(tx, submodelID, parentID, newIDShortPath, submodelElement, nextPosition, rootSmeID)
	if err != nil {
		return err
	}
	err = p.AddNestedSubmodelElementsIteratively(tx, submodelID, id, submodelElement, newIDShortPath, rootSmeID)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		_, _ = fmt.Println(err)
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
	tx, err := p.db.Begin()
	if err != nil {
		_, _ = fmt.Println(err)
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
		_, _ = fmt.Println(err)
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
	handler, err := submodelelements.GetSMEHandler(submodelElement, p.db)
	if err != nil {
		return err
	}
	rootID, err := handler.Create(tx, submodelID, submodelElement)
	if err != nil {
		return err
	}

	err = p.AddNestedSubmodelElementsIteratively(tx, submodelID, rootID, submodelElement, "", rootID)
	if err != nil {
		return err
	}
	return nil
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
func (p *PostgreSQLSubmodelDatabase) AddNestedSubmodelElementsIteratively(tx *sql.Tx, submodelID string, parentID int, topLevelElement gen.SubmodelElement, startPath string, rootSubmodelElementID int) error {
	stack, err := getElementsToProcess(topLevelElement, parentID, startPath)
	if err != nil {
		return err
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

		newParentID, err := handler.CreateNested(tx, submodelID, current.parentID, idShortPath, current.element, current.position, rootSubmodelElementID)
		if err != nil {
			return err
		}
		stack, err = processByModelType(current.element.GetModelType(), newParentID, idShortPath, current, stack)
		if err != nil {
			return err
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

// UploadFileAttachment uploads a file to PostgreSQL's Large Object system for a File submodel element.
// This method delegates to the FileHandler to handle the upload process.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortPath: Path to the file element within the submodel
//   - file: The file to upload
//
// Returns:
//   - error: Error if the upload operation fails
func (p *PostgreSQLSubmodelDatabase) UploadFileAttachment(submodelID string, idShortPath string, file *os.File, fileName string) error {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(p.db)
	if err != nil {
		return fmt.Errorf("failed to create file handler: %w", err)
	}
	return fileHandler.UploadFileAttachment(submodelID, idShortPath, file, fileName)
}

// DownloadFileAttachment retrieves a file from PostgreSQL Large Object system.
// Returns the file content and content type.
func (p *PostgreSQLSubmodelDatabase) DownloadFileAttachment(submodelID string, idShortPath string) ([]byte, string, string, error) {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(p.db)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to create file handler: %w", err)
	}
	return fileHandler.DownloadFileAttachment(submodelID, idShortPath)
}

// UpdateSubmodelElement updates an existing submodel element by its idShortPath.
func (p *PostgreSQLSubmodelDatabase) UpdateSubmodelElement(submodelID string, idShortPath string, submodelElement gen.SubmodelElement) error {
	// Get the model type to determine which handler to use
	modelType, err := getSubmodelElementModelTypeByIDShortPathAndSubmodelID(p.db, submodelID, idShortPath)
	if err != nil {
		return err
	}

	// Get the appropriate handler for this model type
	handler, err := submodelelements.GetSMEHandlerByModelType(modelType, p.db)
	if err != nil {
		return fmt.Errorf("failed to get handler for model type %s: %w", modelType, err)
	}

	// Update the element
	return handler.Update(submodelID, idShortPath, submodelElement)
}

// DeleteFileAttachment deletes a file attachment from PostgreSQL Large Object system.
func (p *PostgreSQLSubmodelDatabase) DeleteFileAttachment(submodelID string, idShortPath string) error {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(p.db)
	if err != nil {
		return fmt.Errorf("failed to create file handler: %w", err)
	}
	return fileHandler.DeleteFileAttachment(submodelID, idShortPath)
}

// UpdateSubmodelElementValueOnly updates only the value of a submodel element identified by its idShort or path.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: idShort or hierarchical path to the element
//   - valueOnly: The new value to set for the submodel element
//
// Returns:
//   - error: Error if the update operation fails
func (p *PostgreSQLSubmodelDatabase) UpdateSubmodelElementValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	// Get the model type to determine which handler to use
	modelType, err := getSubmodelElementModelTypeByIDShortPathAndSubmodelID(p.db, submodelID, idShortOrPath)
	if err != nil {
		return err
	}

	// Get the appropriate handler for this model type
	handler, err := submodelelements.GetSMEHandlerByModelType(modelType, p.db)
	if err != nil {
		return fmt.Errorf("failed to get handler for model type %s: %w", modelType, err)
	}

	// Update the value only
	return handler.UpdateValueOnly(submodelID, idShortOrPath, valueOnly)
}

// UpdateSubmodelValueOnly updates only the values of multiple submodel elements within a submodel.
//
// Parameters:
//   - submodelID: ID of the submodel to update
//   - valueOnly: Map of idShorts to their new values
//
// Returns:
//   - error: Error if the update operation fails
func (p *PostgreSQLSubmodelDatabase) UpdateSubmodelValueOnly(submodelID string, valueOnly gen.SubmodelValue) error {
	exists, err := p.DoesSubmodelExist(submodelID)
	if err != nil {
		_, _ = fmt.Println(err)
		return err
	}

	if !exists {
		return common.NewErrNotFound(fmt.Sprintf("Submodel with ID %s does not exist", submodelID))
	}

	for idShort, submodelElementValue := range valueOnly {
		err = p.UpdateSubmodelElementValueOnly(submodelID, idShort, submodelElementValue)
		if err != nil {
			return err
		}
	}

	return nil
}
