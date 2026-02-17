/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
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
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"

	// PostgreSQL Treiber
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // Postgres Driver for Goqu
	"gopkg.in/go-jose/go-jose.v2"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	smrepoconfig "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/config"
	smrepoerrors "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/errors"
	submodelpersistence "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel/submodelElements"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

// PostgreSQLSubmodelDatabase represents a PostgreSQL-based implementation of the submodel repository database.
// It provides methods for CRUD operations on submodels and their elements with optional caching support.
type PostgreSQLSubmodelDatabase struct {
	db         *sql.DB
	privateKey *rsa.PrivateKey // RSA private key for JWS signing
}

// Transaction error variables moved to smrepoerrors package for centralized error handling
var failedPostgresTransactionSubmodelRepo = smrepoerrors.ErrTransactionCommitFailed
var beginTransactionErrorSubmodelRepo = smrepoerrors.ErrTransactionBeginFailed

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
func NewPostgreSQLSubmodelBackend(dsn string, maxOpenConns int32, maxIdleConns int, connMaxLifetimeMinutes int, databaseSchema string, privateKey *rsa.PrivateKey) (*PostgreSQLSubmodelDatabase, error) {
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}

	if maxOpenConns > 0 {
		db.SetMaxOpenConns(int(maxOpenConns))
	}
	if maxIdleConns > 0 {
		db.SetMaxIdleConns(maxIdleConns)
	}
	if connMaxLifetimeMinutes > 0 {
		db.SetConnMaxLifetime(time.Duration(connMaxLifetimeMinutes) * time.Minute)
	}

	return &PostgreSQLSubmodelDatabase{db: db, privateKey: privateKey}, nil
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
func (p *PostgreSQLSubmodelDatabase) GetAllSubmodels(limit int32, cursor string, _ /* idShort */ string, valueOnly bool) ([]types.ISubmodel, string, error) {
	if limit == 0 {
		limit = smrepoconfig.DefaultPageLimit
	}

	newHandler := submodelpersistence.NewSubmodelHandler(p.db)
	submodels, nextCursor, err := newHandler.GetAllSubmodels(int64(limit), cursor)
	if err != nil {
		return nil, "", err
	}

	resultSubmodels := make([]types.ISubmodel, 0, len(submodels))
	for _, sm := range submodels {
		if sm == nil {
			continue
		}
		elements, _, elementsErr := newHandler.GetSubmodelElements(sm.ID(), -1, "", valueOnly)
		if elementsErr != nil {
			return nil, "", elementsErr
		}
		sm.SetSubmodelElements(elements)
		resultSubmodels = append(resultSubmodels, sm)
	}

	return resultSubmodels, nextCursor, nil
}

// QuerySubmodels retrieves a paginated list of submodels from the database that match the given query.
// This method supports the AAS Query Language for filtering submodels based on conditions.
//
// Parameters:
//   - limit: Maximum number of submodels to return (defaults to 100 if 0)
//   - cursor: Pagination cursor for retrieving next page (empty string for first page)
//   - query: Query wrapper containing the filter conditions
//   - valueOnly: Whether to return only values without metadata
//
// Returns:
//   - []gen.Submodel: List of submodels matching the query
//   - string: Next cursor for pagination (empty if no more pages)
//   - error: Error if retrieval fails
func (p *PostgreSQLSubmodelDatabase) QuerySubmodels(limit int32, cursor string, query *grammar.QueryWrapper, valueOnly bool) ([]types.ISubmodel, string, error) {
	if query == nil {
		return p.GetAllSubmodels(limit, cursor, "", valueOnly)
	}

	if limit <= 0 {
		limit = smrepoconfig.DefaultPageLimit
	}

	queriedSubmodels, _, nextCursor, err := submodelpersistence.GetAllSubmodels(p.db, int64(limit), cursor, query)
	if err != nil {
		return nil, "", err
	}

	newHandler := submodelpersistence.NewSubmodelHandler(p.db)
	resultSubmodels := make([]types.ISubmodel, 0, len(queriedSubmodels))
	for _, sm := range queriedSubmodels {
		if sm == nil {
			continue
		}

		elements, _, elementsErr := newHandler.GetSubmodelElements(sm.ID(), -1, "", valueOnly)
		if elementsErr != nil {
			return nil, "", elementsErr
		}
		sm.SetSubmodelElements(elements)
		resultSubmodels = append(resultSubmodels, sm)
	}

	return resultSubmodels, nextCursor, nil
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
	cursor string,
	idShort string,
	_ /* semanticID */ string,
) ([]types.Submodel, string, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit < 0 {
		return nil, "", common.NewErrBadRequest("Limit has to be higher than 0")
	}
	h := submodelpersistence.NewSubmodelHandler(p.db)
	allMetadata, nextCursor, err := h.GetAllSubmodels(int64(limit), cursor)
	if err != nil {
		return nil, "", err
	}

	result := make([]types.Submodel, 0, len(allMetadata))
	idShortFilter := strings.ToLower(idShort)
	for _, sm := range allMetadata {
		if sm == nil {
			continue
		}
		if idShortFilter != "" {
			if sm.IDShort() == nil || !strings.Contains(strings.ToLower(*sm.IDShort()), idShortFilter) {
				continue
			}
		}
		result = append(result, *sm)
	}

	return result, nextCursor, nil
}

// DoesSubmodelExist checks if a submodel with the given identifier exists in the database.
//
// Parameters:
//   - submodelIdentifier: Unique identifier of the submodel to check
//
// Returns:
//   - bool: True if the submodel exists, false otherwise
//   - error: Error if the query fails
func (p *PostgreSQLSubmodelDatabase) DoesSubmodelExist(submodelIdentifier string, tx *sql.Tx) (bool, error) {
	var localTX *sql.Tx
	if tx != nil {
		localTX = tx
	} else {
		startedTX, cu, err := common.StartTransaction(p.db)
		if err != nil {
			_, _ = fmt.Println("SMREPO-DSE-STARTTX " + err.Error())
			return false, beginTransactionErrorSubmodelRepo
		}
		defer cu(&err)
		localTX = startedTX
	}
	var count int
	sqlQuery, args, err := goqu.Select(goqu.COUNT("*")).
		From("submodel").
		Where(goqu.I("submodel_identifier").Eq(submodelIdentifier)).
		ToSQL()
	if err != nil {
		return false, common.NewInternalServerError("SMREPO-DSE-QUERYERROR Error while building SQL query for submodel existence check - see console for details: " + err.Error())
	}
	err = localTX.QueryRow(sqlQuery, args...).Scan(&count)
	if err != nil {
		return false, common.NewInternalServerError("SMREPO-DSE-QUERYEXECERROR Error while executing SQL query for submodel existence check - see console for details: " + err.Error())
	}

	if tx == nil {
		if err := localTX.Commit(); err != nil {
			_, _ = fmt.Println("SMREPO-DSE-COMMIT " + err.Error())
			return false, common.NewInternalServerError("SMREPO-DSE-COMMIT Error while committing transaction for submodel existence check - see console for details: " + err.Error())
		}
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
func (p *PostgreSQLSubmodelDatabase) GetSubmodel(id string, valueOnly bool) (types.Submodel, error) {
	newHandler := submodelpersistence.NewSubmodelHandler(p.db)
	sm, err := newHandler.GetSubmodelByID(id)
	if err != nil {
		return *types.NewSubmodel("Error"), err
	}

	elements, _, elementsErr := newHandler.GetSubmodelElements(id, -1, "", valueOnly)
	if elementsErr != nil {
		return *types.NewSubmodel("Error"), elementsErr
	}

	sm.SetSubmodelElements(elements)
	return *sm, nil
}

// GetSignedSubmodel retrieves a submodel by its ID and returns it as a JWS-signed compact serialization.
//
// Parameters:
//   - id: Unique identifier of the submodel to retrieve and sign
//   - valueOnly: If true, returns only the value representation
//
// Returns:
//   - string: JWS compact serialization of the signed submodel JSON
//   - error: Error if submodel not found, private key not configured, or signing fails
func (p *PostgreSQLSubmodelDatabase) GetSignedSubmodel(id string, valueOnly bool) (string, error) {
	// Get the submodel from database
	submodel, err := p.GetSubmodel(id, valueOnly)
	if err != nil {
		return "", err
	}

	// Check if private key is configured
	if p.privateKey == nil {
		return "", errors.New("JWS signing not configured: private key not loaded")
	}

	// Marshal submodel to JSON
	jsonSubmodel, err := jsonization.ToJsonable(&submodel)
	if err != nil {
		return "", fmt.Errorf("failed to convert submodel to jsonable: %w", err)
	}
	payload, err := json.Marshal(jsonSubmodel)
	if err != nil {
		return "", fmt.Errorf("failed to marshal submodel: %w", err)
	}

	// Create JWS signer with RS256
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: p.privateKey}, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create JWS signer: %w", err)
	}

	// Sign the payload
	jws, err := signer.Sign(payload)
	if err != nil {
		return "", fmt.Errorf("failed to sign submodel: %w", err)
	}

	// Get compact serialization
	compactSerialized, err := jws.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize JWS: %w", err)
	}

	return compactSerialized, nil
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
func (p *PostgreSQLSubmodelDatabase) DeleteSubmodel(id string, optionalTX *sql.Tx) error {
	var tx *sql.Tx
	var shouldCommit bool
	if optionalTX == nil {
		startedTX, cu, err := common.StartTransaction(p.db)
		if err != nil {
			_, _ = fmt.Println("SMREPO-DSM-STARTTX " + err.Error())
			return beginTransactionErrorSubmodelRepo
		}
		defer cu(&err)
		tx = startedTX
		shouldCommit = true
	} else {
		tx = optionalTX
	}

	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return err
	}

	delSME := goqu.Delete("submodel_element").Where(goqu.I("submodel_id").Eq(submodelDatabaseID))
	querySME, argsSME, err := delSME.ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(querySME, argsSME...)
	if err != nil {
		return err
	}

	del := goqu.Delete("submodel").Where(goqu.I("id").Eq(submodelDatabaseID))
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
	if shouldCommit {
		if err := tx.Commit(); err != nil {
			_, _ = fmt.Println("SMREPO-DSM-COMMIT " + err.Error())
			return failedPostgresTransactionSubmodelRepo
		}
	}
	return nil
}

// PutSubmodel creates or updates a submodel in the database.
// If the submodel already exists, it is deleted and recreated with the new data.
// This method ensures that the submodel is fully replaced with the provided data.
//
// Parameters:
//   - submodelID: Unique identifier of the submodel to create or update
//   - submodel: The submodel data to store
//
// Returns:
//   - bool: True if the submodel was updated (existed before), false if created new
//   - error: Error if creation or update fails
func (p *PostgreSQLSubmodelDatabase) PutSubmodel(submodelID string, submodel types.ISubmodel) (bool, error) {
	tx, cu, err := common.StartTransaction(p.db)
	if err != nil {
		_, _ = fmt.Println("SMREPO-PUTSM-STARTTX " + err.Error())
		return false, beginTransactionErrorSubmodelRepo
	}
	defer cu(&err)
	exists, err := p.DoesSubmodelExist(submodelID, tx)
	if err != nil {
		_, _ = fmt.Println("SMREPO-PUTSM-DOESSMEXIST " + err.Error())
		return false, common.NewInternalServerError("Error while checking for submodel Existence - see console for details.")
	}
	if exists {
		err = p.DeleteSubmodel(submodelID, tx)
		if err != nil {
			return false, err
		}
	}

	err = p.CreateSubmodel(submodel, tx)
	if err != nil {
		return false, err
	}
	err = tx.Commit()
	return exists, err
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
func (p *PostgreSQLSubmodelDatabase) CreateSubmodel(smInt types.ISubmodel, optionalTX *sql.Tx) error {
	newHandler := submodelpersistence.NewSubmodelHandler(p.db)
	return newHandler.CreateSubmodel(smInt, optionalTX)
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
//   - types.ISubmodelElement: The requested submodel element
//   - error: Error if element not found or retrieval fails
func (p *PostgreSQLSubmodelDatabase) GetSubmodelElement(submodelID string, idShortOrPath string, valueOnly bool) (types.ISubmodelElement, error) {
	_ = valueOnly
	newHandler := submodelpersistence.NewSubmodelHandler(p.db)
	return newHandler.GetSubmodelElementByIdShortOrPath(submodelID, idShortOrPath)
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
//   - []types.ISubmodelElement: List of submodel elements
//   - string: Next cursor for pagination
//   - error: Error if retrieval fails
func (p *PostgreSQLSubmodelDatabase) GetSubmodelElements(submodelID string, limit int, cursor string, valueOnly bool) ([]types.ISubmodelElement, string, error) {
	newHandler := submodelpersistence.NewSubmodelHandler(p.db)
	return newHandler.GetSubmodelElements(submodelID, limit, cursor, valueOnly)
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
func (p *PostgreSQLSubmodelDatabase) AddSubmodelElementWithPath(submodelID string, idShortPath string, submodelElement types.ISubmodelElement) error {
	crud, err := submodelelements.NewPostgreSQLSMECrudHandler(p.db)
	if err != nil {
		return err
	}

	tx, cu, err := common.StartTransaction(p.db)
	if err != nil {
		_, _ = fmt.Println(err)
		return beginTransactionErrorSubmodelRepo
	}

	defer cu(&err)

	smDbID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to execute PostgreSQL Query - no changes applied - see console for details.")
	}

	parentID, err := crud.GetDatabaseID(smDbID, idShortPath)
	if err != nil {
		_, _ = fmt.Println(err)
		// if is no rows error, then the specified path does not exist
		if errors.Is(err, sql.ErrNoRows) {
			return common.NewErrNotFound("Parent element with path '" + idShortPath + "' not found in submodel '" + submodelID + "'")
		}
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
	if *modelType != types.ModelTypeSubmodelElementCollection && *modelType != types.ModelTypeSubmodelElementList && *modelType != types.ModelTypeEntity && *modelType != types.ModelTypeAnnotatedRelationshipElement {
		mt, ok := stringification.ModelTypeToString(*modelType)
		if !ok {
			mt = "unknown"
		}
		return common.NewErrBadRequest("cannot add nested element to non-collection/list element. Tried to add to element of type '" + mt + "' at path '" + idShortPath + "'")
	}

	isFromList := *modelType == types.ModelTypeSubmodelElementList
	var newIDShortPath string
	if isFromList {
		newIDShortPath = idShortPath + "[" + strconv.Itoa(nextPosition) + "]"
		// For lists, check if an element with the same idShort already exists within the list
		checkQuery, checkArgs, err := goqu.Select(goqu.COUNT("id")).From("submodel_element").
			Where(
				goqu.I("submodel_id").Eq(smDbID),
				goqu.I("parent_sme_id").Eq(parentID),
				goqu.I("id_short").Eq(submodelElement.IDShort()),
			).ToSQL()
		if err != nil {
			_, _ = fmt.Println(err)
			return common.NewInternalServerError("Failed to check for duplicate idShort in list - no changes applied - see console for details.")
		}
		var count int
		err = tx.QueryRow(checkQuery, checkArgs...).Scan(&count)
		if err != nil {
			_, _ = fmt.Println(err)
			return common.NewInternalServerError("Failed to check for duplicate idShort in list - no changes applied - see console for details.")
		}
		if count > 0 {
			return common.NewErrConflict("SubmodelElement with idShort '" + *submodelElement.IDShort() + "' already exists in submodel '" + submodelID + "'")
		}
	} else {
		newIDShortPath = idShortPath + "." + *submodelElement.IDShort()
	}

	exists, err := doesSubmodelElementExist(tx, submodelID, newIDShortPath)
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to check for existing SubmodelElement - no changes applied - see console for details.")
	}
	if exists {
		return common.NewErrConflict("SubmodelElement with idShort '" + *submodelElement.IDShort() + "' already exists in submodel '" + submodelID + "'")
	}

	var rootSmeID int
	sqlQuery, args, err := goqu.Select("root_sme_id").From("submodel_element").Where(goqu.I("idshort_path").Eq(idShortPath)).ToSQL()
	if err != nil {
		return err
	}
	err = tx.QueryRow(sqlQuery, args...).Scan(&rootSmeID)
	if err != nil {
		return err
	}

	// Use BatchInsert with proper context for nested insertion
	ctx := &submodelelements.BatchInsertContext{
		ParentID:      parentID,
		ParentPath:    idShortPath,
		RootSmeID:     rootSmeID,
		IsFromList:    isFromList,
		StartPosition: nextPosition,
	}
	_, err = submodelelements.BatchInsert(p.db, submodelID, []types.ISubmodelElement{submodelElement}, tx, ctx)
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
func (p *PostgreSQLSubmodelDatabase) AddSubmodelElement(submodelID string, submodelElement types.ISubmodelElement) error {
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
func (p *PostgreSQLSubmodelDatabase) AddSubmodelElementWithTransaction(tx *sql.Tx, submodelID string, submodelElement types.ISubmodelElement) error {
	exists, err := doesSubmodelElementExist(tx, submodelID, *submodelElement.IDShort())
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to check for existing SubmodelElement - no changes applied - see console for details.")
	}
	if exists {
		return common.NewErrConflict("SubmodelElement with idShort '" + *submodelElement.IDShort() + "' already exists in submodel '" + submodelID + "'")
	}

	// Use BatchInsert with a single element - it handles all nested elements automatically
	_, err = submodelelements.BatchInsert(p.db, submodelID, []types.ISubmodelElement{submodelElement}, tx, nil)
	if err != nil {
		return err
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
// This method determines the appropriate handler based on the element's model type
// and delegates the update operation to that handler.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortPath: idShort or hierarchical path to the element
//   - submodelElement: The updated submodel element
//   - isPut: Flag indicating if this is a full replacement (PUT) or partial update (PATCH)
//
// Returns:
//   - error: Error if the update operation fails
func (p *PostgreSQLSubmodelDatabase) UpdateSubmodelElement(submodelID string, idShortPath string, submodelElement types.ISubmodelElement, isPut bool) error {
	tx, cu, err := common.StartTransaction(p.db)
	if err != nil {
		_, _ = fmt.Println(err)
		return beginTransactionErrorSubmodelRepo
	}
	defer cu(&err)
	// Get the model type to determine which handler to use
	modelType, err := getSubmodelElementModelTypeByIDShortPathAndSubmodelID(p.db, submodelID, idShortPath)
	if err != nil {
		return err
	}

	if modelType == nil {
		return common.NewErrNotFound("SubmodelElement with idShort or path '" + idShortPath + "' not found in submodel '" + submodelID + "'")
	}

	// Get the appropriate handler for this model type
	handler, err := submodelelements.GetSMEHandlerByModelType(*modelType, p.db)
	if err != nil {
		stringModelType, ok := stringification.ModelTypeToString(*modelType)
		if !ok {
			stringModelType = "unknown"
		}
		return fmt.Errorf("failed to get handler for model type %s: %w", stringModelType, err)
	}
	err = handler.Update(submodelID, idShortPath, submodelElement, nil, isPut)

	if err != nil {
		return err
	}

	// If the idShort changed during a PUT, update the idShortPath of this element and all descendants
	effectivePath := idShortPath
	if isPut && submodelElement.IDShort() != nil {
		smeHandler := submodelelements.PostgreSQLSMECrudHandler{Db: p.db}
		newPath, pathErr := smeHandler.UpdateIdShortPaths(tx, submodelID, idShortPath, *submodelElement.IDShort())
		if pathErr != nil {
			err = pathErr
			return err
		}
		effectivePath = newPath
	}

	if isPut {
		err = handleNestedElementsAfterPut(p, effectivePath, *modelType, tx, submodelID, submodelElement)
		if err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		_, _ = fmt.Println(err)
		return failedPostgresTransactionSubmodelRepo
	}
	return nil
}

func handleNestedElementsAfterPut(p *PostgreSQLSubmodelDatabase, idShortPath string, modelType types.ModelType, tx *sql.Tx, submodelID string, submodelElement types.ISubmodelElement) error {
	if !isModelTypeWithNestedElements(modelType) {
		return nil
	}
	smDbID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to execute PostgreSQL Query - no changes applied - see console for details.")
	}

	smeHandler := submodelelements.PostgreSQLSMECrudHandler{Db: p.db}
	elementID, err := smeHandler.GetDatabaseID(smDbID, idShortPath)
	if err != nil {
		return err
	}

	// Get child elements based on model type
	var children []types.ISubmodelElement
	isFromList := false

	switch modelType {
	case types.ModelTypeSubmodelElementCollection:
		if coll, ok := submodelElement.(*types.SubmodelElementCollection); ok {
			children = coll.Value()
		}
	case types.ModelTypeSubmodelElementList:
		if list, ok := submodelElement.(*types.SubmodelElementList); ok {
			children = list.Value()
			isFromList = true
		}
	case types.ModelTypeAnnotatedRelationshipElement:
		if rel, ok := submodelElement.(*types.AnnotatedRelationshipElement); ok {
			for _, ann := range rel.Annotations() {
				children = append(children, ann)
			}
		}
	case types.ModelTypeEntity:
		if ent, ok := submodelElement.(*types.Entity); ok {
			children = ent.Statements()
		}
	}

	if len(children) == 0 {
		return nil
	}

	// Use BatchInsert with context for nested elements
	ctx := &submodelelements.BatchInsertContext{
		ParentID:   elementID,
		ParentPath: idShortPath,
		RootSmeID:  elementID, // For PUT, the updated element is the root
		IsFromList: isFromList,
	}

	_, err = submodelelements.BatchInsert(p.db, submodelID, children, tx, ctx)
	return err
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
	if modelType == nil {
		return common.NewErrNotFound("SubmodelElement with idShort or path '" + idShortOrPath + "' not found in submodel '" + submodelID + "'")
	}

	// Get the appropriate handler for this model type
	handler, err := submodelelements.GetSMEHandlerByModelType(*modelType, p.db)
	if err != nil {
		stringModelType, ok := stringification.ModelTypeToString(*modelType)
		if !ok {
			stringModelType = "unknown"
		}
		return fmt.Errorf("failed to get handler for model type %s: %w", stringModelType, err)
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
	exists, err := p.DoesSubmodelExist(submodelID, nil)
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

func isModelTypeWithNestedElements(modelType types.ModelType) bool {
	return modelType == types.ModelTypeAnnotatedRelationshipElement || modelType == types.ModelTypeSubmodelElementCollection || modelType == types.ModelTypeSubmodelElementList || modelType == types.ModelTypeEntity
}

// doesSubmodelElementExist checks if a submodel element exists within a transaction context
func doesSubmodelElementExist(tx *sql.Tx, submodelID string, idShortOrPath string) (bool, error) {
	dialect := goqu.Dialect("postgres")
	smDbID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		_, _ = fmt.Println(err)
		return false, common.NewInternalServerError("Failed to execute PostgreSQL Query - no changes applied - see console for details.")
	}
	selectQuery := dialect.From("submodel_element").Select(goqu.COUNT("*")).Where(
		goqu.I("submodel_id").Eq(smDbID),
		goqu.I("idshort_path").Eq(idShortOrPath),
	)

	query, args, err := selectQuery.ToSQL()
	if err != nil {
		return false, err
	}

	var count int
	err = tx.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetPrivateKey returns the RSA private key for JWS signing.
func (p *PostgreSQLSubmodelDatabase) GetPrivateKey() *rsa.PrivateKey {
	return p.privateKey
}

func getAdministrationJSONStringFromSubmodel(sm *types.Submodel) (sql.NullString, error) {
	if sm.Administration() == nil {
		return sql.NullString{Valid: false}, nil
	}
	jsonable, err := jsonization.ToJsonable(sm.Administration())
	if err != nil {
		return sql.NullString{}, fmt.Errorf("failed to convert administration to jsonable: %w", err)
	}

	adminJSON, err := json.Marshal(jsonable)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("failed to marshal administration: %w", err)
	}
	return sql.NullString{String: string(adminJSON), Valid: true}, nil
}

func getDescriptionJSONStringFromSubmodel(sm *types.Submodel) (sql.NullString, error) {
	if sm.Description() == nil {
		return sql.NullString{Valid: false}, nil
	}
	var jsonable []map[string]interface{}
	for i, desc := range sm.Description() {
		jsonableSlice, err := jsonization.ToJsonable(desc)
		if err != nil {
			return sql.NullString{}, fmt.Errorf("failed to convert description at index %d to jsonable: %w", i, err)
		}
		jsonable = append(jsonable, jsonableSlice)
	}
	descJSON, err := json.Marshal(jsonable)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("failed to marshal description: %w", err)
	}
	return sql.NullString{String: string(descJSON), Valid: true}, nil
}

func getDisplayNameJSONStringFromSubmodel(sm *types.Submodel) (sql.NullString, error) {
	if sm.DisplayName() == nil {
		return sql.NullString{Valid: false}, nil
	}
	var jsonable []map[string]interface{}
	for i, disp := range sm.DisplayName() {
		jsonableSlice, err := jsonization.ToJsonable(disp)
		if err != nil {
			return sql.NullString{}, fmt.Errorf("failed to convert display name at index %d to jsonable: %w", i, err)
		}
		jsonable = append(jsonable, jsonableSlice)
	}
	dispJSON, err := json.Marshal(jsonable)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("failed to marshal display name: %w", err)
	}
	return sql.NullString{String: string(dispJSON), Valid: true}, nil
}

func getQualifierJSONStringFromSubmodel(sm *types.Submodel) (sql.NullString, error) {
	if sm.Qualifiers() == nil {
		return sql.NullString{Valid: false}, nil
	}
	var jsonable []map[string]interface{}
	for i, qual := range sm.Qualifiers() {
		jsonableSlice, err := jsonization.ToJsonable(qual)
		if err != nil {
			return sql.NullString{}, fmt.Errorf("failed to convert qualifier at index %d to jsonable: %w", i, err)
		}
		jsonable = append(jsonable, jsonableSlice)
	}
	qualJSON, err := json.Marshal(jsonable)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("failed to marshal qualifier: %w", err)
	}
	return sql.NullString{String: string(qualJSON), Valid: true}, nil
}

func createSemanticIDForSubmodel(tx *sql.Tx, submodelDatabaseID int64, sm *types.Submodel) (sql.NullInt64, error) {
	if sm == nil {
		return sql.NullInt64{Valid: false}, nil
	}
	return persistenceutils.CreateContextReferenceByOwnerID(tx, submodelDatabaseID, "submodel_semantic_id", sm.SemanticID())
}
