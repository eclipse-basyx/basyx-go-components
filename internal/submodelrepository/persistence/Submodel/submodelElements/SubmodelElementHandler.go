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
// Author: Jannik Fried ( Fraunhofer IESE )

// Package submodelelements provides persistence layer functionality for managing submodel elements
// in the Eclipse BaSyx submodel repository. It implements CRUD operations for all types of
// submodel elements defined in the AAS specification, including properties, collections,
// relationships, events, and more.
//
// The package uses a factory pattern to create type-specific handlers and provides efficient
// database queries with hierarchical data retrieval for nested element structures.
package submodelelements

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // Postgres Driver for Goqu

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	submodelsubqueries "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel/queries"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	jsoniter "github.com/json-iterator/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// GetSMEHandler creates the appropriate CRUD handler for a submodel element.
//
// This function uses the Factory Pattern to instantiate the correct handler based on
// the model type of the provided submodel element. It provides a clean, type-safe way
// to obtain handlers without requiring client code to know the concrete handler types.
//
// Parameters:
//   - submodelElement: The submodel element for which to create a handler
//   - db: Database connection to be used by the handler
//
// Returns:
//   - PostgreSQLSMECrudInterface: Type-specific handler implementing CRUD operations
//   - error: An error if the model type is unsupported or handler creation fails
func GetSMEHandler(submodelElement types.ISubmodelElement, db *sql.DB) (PostgreSQLSMECrudInterface, error) {
	return GetSMEHandlerByModelType(submodelElement.ModelType(), db)
}

// GetSMEHandlerByModelType creates a handler by model type string.
//
// This function implements the Single Responsibility Principle by focusing solely on
// the logic for determining and instantiating the correct handler based on a model
// type string. It supports all AAS submodel element types defined in the specification.
//
// Supported model types:
//   - AnnotatedRelationshipElement: Relationship with annotations
//   - BasicEventElement: Event element for monitoring and notifications
//   - Blob: Binary data element
//   - Capability: Functional capability description
//   - Entity: Logical or physical entity
//   - File: File reference element
//   - MultiLanguageProperty: Property with multi-language support
//   - Operation: Invocable operation
//   - Property: Single-valued property
//   - Range: Value range element
//   - ReferenceElement: Reference to another element
//   - RelationshipElement: Relationship between elements
//   - SubmodelElementCollection: Collection of submodel elements
//   - SubmodelElementList: Ordered list of submodel elements
//
// Parameters:
//   - modelType: String representation of the submodel element type
//   - db: Database connection to be used by the handler
//
// Returns:
//   - PostgreSQLSMECrudInterface: Type-specific handler implementing CRUD operations
//   - error: An error if the model type is unsupported or handler creation fails
func GetSMEHandlerByModelType(modelType types.ModelType, db *sql.DB) (PostgreSQLSMECrudInterface, error) {
	// Use the centralized handler registry for cleaner factory pattern
	return GetHandlerFromRegistry(modelType, db)
}

// UpdateNestedElementsValueOnly updates nested submodel elements based on value-only patches.
//
// Parameters:
//   - db: Database connection
//   - elems: List of elements to process
//   - idShortOrPath: idShort or hierarchical path of the root element
//   - submodelID: ID of the parent submodel
//
// Returns:
//   - error: Error if update fails
func UpdateNestedElementsValueOnly(db *sql.DB, elems []persistenceutils.ValueOnlyElementsToProcess, idShortOrPath string, submodelID string) error {
	for _, elem := range elems {
		if elem.IdShortPath == idShortOrPath {
			continue // Skip the root element as it's already processed
		}
		modelType := elem.Element.GetModelType()
		if modelType == types.ModelTypeFile {
			// We have to check the database because File could be ambiguous between File and Blob
			actual, err := GetModelTypeByIdShortPathAndSubmodelID(db, submodelID, elem.IdShortPath)
			if err != nil {
				return err
			}
			if actual == nil {
				return common.NewErrNotFound("Submodel-Element ID-Short: " + elem.IdShortPath)
			}

			modelType = *actual
		}
		handler, err := GetSMEHandlerByModelType(modelType, db)
		if err != nil {
			return err
		}
		err = handler.UpdateValueOnly(submodelID, elem.IdShortPath, elem.Element)
		if err != nil {
			return err
		}
	}
	return nil
}

// UpdateNestedElements updates nested submodel elements based on value-only patches.
//
// Parameters:
//   - db: Database connection
//   - elems: List of elements to process
//   - idShortOrPath: idShort or hierarchical path of the root element
//   - submodelID: ID of the parent submodel
//
// Returns:
//   - error: Error if update fails
func UpdateNestedElements(db *sql.DB, elems []persistenceutils.SubmodelElementToProcess, idShortOrPath string, submodelID string, tx *sql.Tx, isPut bool) error {
	localTx := tx
	var err error
	if tx == nil {
		var cu func(*error)
		localTx, cu, err = common.StartTransaction(db)
		if err != nil {
			return err
		}

		defer cu(&err)
	}
	for _, elem := range elems {
		if elem.IdShortPath == idShortOrPath {
			continue // Skip the root element as it's already processed
		}
		modelType := elem.Element.ModelType()
		if modelType == types.ModelTypeFile {
			// We have to check the database because File could be ambiguous between File and Blob
			actual, err := GetModelTypeByIdShortPathAndSubmodelID(db, submodelID, elem.IdShortPath)
			if err != nil {
				return err
			}
			if actual == nil {
				return common.NewErrNotFound("SMREPO-UPDNESTED-NOTFOUND Submodel-Element ID-Short: " + elem.IdShortPath)
			}

			modelType = *actual
		}
		handler, err := GetSMEHandlerByModelType(modelType, db)
		if err != nil {
			return err
		}
		err = handler.Update(submodelID, elem.IdShortPath, elem.Element, localTx, isPut)
		if err != nil {
			return err
		}
	}

	if tx == nil {
		if err = localTx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// GetModelTypeByIdShortPathAndSubmodelID retrieves the model type of a submodel element
//
// Parameters:
// - db: Database connection
// - submodelID: ID of the parent submodel
//
// - idShortOrPath: idShort or hierarchical path of the submodel element
// Returns:
// - string: Model type of the submodel element
// - error: Error if retrieval fails or element is not found
func GetModelTypeByIdShortPathAndSubmodelID(db *sql.DB, submodelID string, idShortOrPath string) (*types.ModelType, error) {
	dialect := goqu.Dialect("postgres")

	query, args, err := dialect.From("submodel_element").
		Select("model_type").
		Where(
			goqu.C("submodel_id").Eq(submodelID),
			goqu.C("idshort_path").Eq(idShortOrPath),
		).
		ToSQL()
	if err != nil {
		return nil, err
	}

	var modelType types.ModelType
	err = db.QueryRow(query, args...).Scan(&modelType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.NewErrNotFound("SMREPO-GETMODELTYPE-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
		}
		return nil, err
	}
	return &modelType, nil
}

// DeleteSubmodelElementByPath removes a submodel element by its idShort or path including all nested elements.
//
// This function performs cascading deletion of a submodel element and its entire subtree.
// If the deleted element is part of a SubmodelElementList, it automatically adjusts the
// position indices of remaining elements to maintain consistency.
//
// The function handles:
//   - Direct deletion of the element and its subtree (using path pattern matching)
//   - Index recalculation for SubmodelElementList elements after deletion
//   - Path updates for remaining list elements to reflect new indices
//
// Parameters:
//   - tx: Transaction context for atomic deletion operations
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: Path to the element to delete (e.g., "prop1" or "collection.list[2]")
//
// Returns:
//   - error: An error if the element is not found or database operations fail
//
// Example:
//
//	// Delete a simple property
//	err := DeleteSubmodelElementByPath(tx, "submodel123", "temperature")
//
//	// Delete an element in a list (adjusts indices of elements after it)
//	err := DeleteSubmodelElementByPath(tx, "submodel123", "sensors[1]")
//
//	// Delete a nested collection and all its children
//	err := DeleteSubmodelElementByPath(tx, "submodel123", "properties.metadata")
func DeleteSubmodelElementByPath(tx *sql.Tx, submodelID string, idShortOrPath string) error {
	del := goqu.Delete("submodel_element").Where(
		goqu.And(
			goqu.I("submodel_id").Eq(submodelID),
			goqu.Or(
				goqu.I("idshort_path").Eq(idShortOrPath),
				goqu.I("idshort_path").Like(idShortOrPath+".%"),
				goqu.I("idshort_path").Like(idShortOrPath+"[%"),
			),
		),
	)
	sqlQuery, args, err := del.ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSMEBPATH-TOSQL Failed to build delete query: " + err.Error())
	}
	result, err := tx.Exec(sqlQuery, args...)
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSMEBPATH-EXEC Failed to execute delete query: " + err.Error())
	}
	affectedRows, err := result.RowsAffected()
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSMEBPATH-ROWSAFFECTED Failed to get affected rows: " + err.Error())
	}
	// if idShortPath ends with ] it is part of a SubmodelElementList and we need to update the indices of the remaining elements
	if idShortOrPath[len(idShortOrPath)-1] == ']' {
		// extract the parent path and the index of the deleted element
		var parentPath string
		var deletedIndex int
		for i := len(idShortOrPath) - 1; i >= 0; i-- {
			if idShortOrPath[i] == '[' {
				parentPath = idShortOrPath[:i]
				indexStr := idShortOrPath[i+1 : len(idShortOrPath)-1]
				var err error
				deletedIndex, err = strconv.Atoi(indexStr)
				if err != nil {
					return common.NewInternalServerError("SMREPO-DELSMEBPATH-PARSEINDEX Failed to parse index: " + err.Error())
				}
				break
			}
		}

		// get the id of the parent SubmodelElementList
		dialect := goqu.Dialect("postgres")
		selectQuery, selectArgs, err := dialect.From("submodel_element").
			Select("id").
			Where(goqu.And(
				goqu.C("submodel_id").Eq(submodelID),
				goqu.C("idshort_path").Eq(parentPath),
			)).
			ToSQL()
		if err != nil {
			return common.NewInternalServerError("SMREPO-DELSMEBPATH-SELECTPARENT-TOSQL Failed to build select query: " + err.Error())
		}

		var parentID int
		err = tx.QueryRow(selectQuery, selectArgs...).Scan(&parentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return common.NewErrNotFound("SMREPO-DELSMEBPATH-SELECTPARENT-NOTFOUND Parent ID-Short: " + parentPath)
			}
			return common.NewInternalServerError("SMREPO-DELSMEBPATH-SELECTPARENT-EXEC Failed to execute select query: " + err.Error())
		}

		// update the indices of the remaining elements in the SubmodelElementList
		updateQuery, updateArgs, err := dialect.Update("submodel_element").
			Set(goqu.Record{"position": goqu.L("position - 1")}).
			Where(goqu.And(
				goqu.C("parent_sme_id").Eq(parentID),
				goqu.C("position").Gt(deletedIndex),
			)).
			ToSQL()
		if err != nil {
			return common.NewInternalServerError("SMREPO-DELSMEBPATH-UPDATEINDICES-TOSQL Failed to build update query: " + err.Error())
		}
		_, err = tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			return common.NewInternalServerError("SMREPO-DELSMEBPATH-UPDATEINDICES-EXEC Failed to execute update query: " + err.Error())
		}
		// update their idshort_path as well
		updatePathQuery, updatePathArgs, err := dialect.Update("submodel_element").
			Set(goqu.Record{"idshort_path": goqu.L("regexp_replace(idshort_path, '\\[' || (position + 1) || '\\]', '[' || position || ']')")}).
			Where(goqu.And(
				goqu.C("parent_sme_id").Eq(parentID),
				goqu.C("position").Gte(deletedIndex),
			)).
			ToSQL()
		if err != nil {
			return common.NewInternalServerError("SMREPO-DELSMEBPATH-UPDATEPATH-TOSQL Failed to build update path query: " + err.Error())
		}
		_, err = tx.Exec(updatePathQuery, updatePathArgs...)
		if err != nil {
			return common.NewInternalServerError("SMREPO-DELSMEBPATH-UPDATEPATH-EXEC Failed to execute update path query: " + err.Error())
		}
	}
	if affectedRows == 0 {
		return common.NewErrNotFound("SMREPO-DELSMEBPATH-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
	}
	return nil
}

// GetSubmodelElementsForSubmodel retrieves all submodel elements for a given submodel ID.
func GetSubmodelElementsForSubmodel(db *sql.DB, submodelID string, idShortPath string, cursor string, limit int, valueOnly bool) ([]types.ISubmodelElement, string, error) {
	filter := submodelsubqueries.SubmodelElementFilter{
		SubmodelFilter: &submodelsubqueries.SubmodelElementSubmodelFilter{
			SubmodelIDFilter: submodelID,
		},
	}

	if idShortPath != "" {
		filter.SubmodelElementIDShortPathFilter = &submodelsubqueries.SubmodelElementIDShortPathFilter{
			SubmodelElementIDShortPath: idShortPath,
		}
	}

	submodelElementQuery, err := submodelsubqueries.GetSubmodelElementsQuery(filter, cursor, limit, valueOnly)
	if err != nil {
		return nil, "", err
	}
	q, params, err := submodelElementQuery.ToSQL()

	if err != nil {
		return nil, "", err
	}

	rows, err := db.Query(q, params...)

	if err != nil {
		return nil, "", err
	}
	defer func() { _ = rows.Close() }()

	nodes := make(map[int64]*node, 256)
	children := make(map[int64][]*node, 256)
	roots := make([]*node, 0, 16)

	// RootSubmodelElements
	smeBuilderMap := make(map[int64]*builder.SubmodelElementBuilder)
	smeRows := []model.SubmodelElementRow{}
	for rows.Next() {
		smeRow := model.SubmodelElementRow{}
		if err := rows.Scan(
			&smeRow.DbID,
			&smeRow.ParentID,
			&smeRow.RootID,
			&smeRow.IDShort,
			&smeRow.IDShortPath,
			&smeRow.Category,
			&smeRow.ModelType,
			&smeRow.Position,
			&smeRow.EmbeddedDataSpecifications,
			&smeRow.SupplementalSemanticIDs,
			&smeRow.Extensions,
			&smeRow.DisplayNames,
			&smeRow.Descriptions,
			&smeRow.Value,
			&smeRow.SemanticID,
			&smeRow.SemanticIDReferred,
			&smeRow.Qualifiers,
		); err != nil {
			return nil, "", err
		}
		smeRows = append(smeRows, smeRow)
	}
	var wg sync.WaitGroup
	var buildError error
	var mu sync.Mutex
	for _, smeRow := range smeRows {
		wg.Go(func() {
			mu.Lock()
			_, exists := smeBuilderMap[smeRow.DbID.Int64]
			mu.Unlock()
			if !exists {
				sme, builder, err := builder.BuildSubmodelElement(smeRow, db)
				if err != nil {
					buildError = err
					return
				}
				mu.Lock()
				smeBuilderMap[smeRow.DbID.Int64] = builder
				mu.Unlock()
				n := &node{
					id:       smeRow.DbID.Int64,
					parentID: smeRow.ParentID.Int64,
					path:     smeRow.IDShortPath,
					position: smeRow.Position,
					element:  sme,
				}
				mu.Lock()
				nodes[smeRow.DbID.Int64] = n
				mu.Unlock()
				if smeRow.ParentID.Valid && (idShortPath != smeRow.IDShortPath) {
					mu.Lock()
					children[smeRow.ParentID.Int64] = append(children[smeRow.ParentID.Int64], n)
					mu.Unlock()
				} else {
					mu.Lock()
					roots = append(roots, n)
					mu.Unlock()
				}
			}
		})
	}
	wg.Wait()
	if buildError != nil {
		return nil, "", buildError
	}

	attachChildrenToSubmodelElements(nodes, children)

	sort.SliceStable(roots, func(i, j int) bool {
		a, b := roots[i], roots[j]
		return a.path < b.path
	})

	res := make([]types.ISubmodelElement, 0, len(roots))
	for _, r := range roots {
		res = append(res, r.element)
	}

	var nextCursor string
	if (len(res) > limit) && limit != -1 {
		nextCursor = *res[limit].IDShort()
		res = res[:limit]
	}

	return res, nextCursor, nil
}

// DeleteAllChildren removes all associated children
//
// Parameters:
// - db: The database connection
// - submodelId: The Identifier of the Submodel the SubmodelElement belongs to
// - idShortPath: The parents idShortPath to delete the children from
// - tx: transaction context (will be set if nil)
func DeleteAllChildren(db *sql.DB, submodelId string, idShortPath string, tx *sql.Tx) error {
	var err error
	localTx := tx
	if tx == nil {
		var cu func(*error)
		localTx, cu, err = common.StartTransaction(db)
		if err != nil {
			return err
		}

		defer cu(&err)
	}

	// Delete All Elements that start with idShortPath + "." or with idShortPath + "["

	del := goqu.Delete("submodel_element").Where(
		goqu.And(
			goqu.I("submodel_id").Eq(submodelId),
			goqu.Or(
				goqu.I("idshort_path").Like(idShortPath+".%"),
				goqu.I("idshort_path").Like(idShortPath+"[%"),
			),
		),
	)
	sqlQuery, args, err := del.ToSQL()
	if err != nil {
		return err
	}
	_, err = localTx.Exec(sqlQuery, args...)
	if err != nil {
		return err
	}

	if tx == nil {
		if err = localTx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

// attachChildrenToSubmodelElements reconstructs the hierarchical structure of submodel elements.
//
// This function attaches child elements to their parent containers (SubmodelElementCollection
// or SubmodelElementList) in the proper order. It performs a stable sort based on position
// (if set) with path as tie-breaker, ensuring consistent ordering of children.
//
// The function operates in O(n log n) time where n is the number of children, using an
// efficient sorting algorithm and direct map lookups.
//
// Parameters:
//   - nodes: Map of all nodes indexed by their database ID
//   - children: Map of parent ID to slice of child nodes
//
// Note: Only SubmodelElementCollection and SubmodelElementList types support children.
// Other element types are silently skipped even if they have entries in the children map.
func attachChildrenToSubmodelElements(nodes map[int64]*node, children map[int64][]*node) {
	for id, parent := range nodes {
		kids := children[id]
		if len(kids) == 0 {
			continue
		}

		// Stable order: by position (if set)
		sort.SliceStable(kids, func(i, j int) bool {
			a, b := kids[i], kids[j]
			return a.position < b.position
		})

		switch parent.element.ModelType() {
		case types.ModelTypeSubmodelElementCollection:
			if p, ok := parent.element.(types.ISubmodelElementCollection); ok {
				value := p.Value()
				for _, ch := range kids {
					value = append(value, ch.element)
				}
				p.SetValue(value)
			}
		case types.ModelTypeSubmodelElementList:
			if p, ok := parent.element.(types.ISubmodelElementList); ok {
				value := p.Value()
				for _, ch := range kids {
					value = append(value, ch.element)
				}
				p.SetValue(value)
			}
		case types.ModelTypeAnnotatedRelationshipElement:
			if p, ok := parent.element.(types.IAnnotatedRelationshipElement); ok {
				annotations := p.Annotations()
				for _, ch := range kids {
					annotations = append(annotations, ch.element)
				}
				p.SetAnnotations(annotations)
			}
		case types.ModelTypeEntity:
			if p, ok := parent.element.(types.IEntity); ok {
				statements := p.Statements()
				for _, ch := range kids {
					statements = append(statements, ch.element)
				}
				p.SetStatements(statements)
			}
		}
	}
}

// node is a helper struct to build the hierarchical structure of SubmodelElements.
//
// It holds metadata such as database ID, parent ID, path, position, and the actual
// SubmodelElement data. This struct is used during the reconstruction of the
// nested structure of submodel elements from flat database rows.
type node struct {
	id       int64                  // Database ID of the element
	parentID int64                  // Parent element ID for hierarchy
	path     string                 // Full path for navigation
	position int                    // Position within parent for ordering
	element  types.ISubmodelElement // The actual submodel element data
}

// batchInsertElement holds the data needed for batch inserting a single element.
type batchInsertElement struct {
	element     types.ISubmodelElement
	handler     PostgreSQLSMECrudInterface
	position    int
	idShort     string
	idShortPath string
	isFromList  bool // Indicates if element is from a SubmodelElementList (uses index path)
}

// BatchInsertContext provides context for batch inserting submodel elements.
// It specifies where in the hierarchy the elements should be inserted.
type BatchInsertContext struct {
	ParentID      int    // Database ID of the parent element (0 for top-level elements)
	ParentPath    string // Path of the parent element (empty for top-level elements)
	RootSmeID     int    // Database ID of the root submodel element (0 for top-level elements, will be set to own ID)
	IsFromList    bool   // Whether elements are being inserted into a SubmodelElementList
	StartPosition int    // Starting position for elements (used when adding to existing containers)
}

// BatchInsert inserts multiple submodel elements and their nested children in optimized SQL operations.
//
// This function handles both top-level submodel elements (direct children of a Submodel) and
// nested elements (children of SubmodelElementCollection, SubmodelElementList, Entity, or
// AnnotatedRelationshipElement). It replaces both Create and CreateNested methods.
//
// The function optimizes database operations by:
// 1. Inserting all elements at the current level in one batch INSERT
// 2. Grouping type-specific records by table and inserting each group in one batch
// 3. Recursively processing nested children level by level
//
// Parameters:
//   - db: Database connection
//   - submodelID: ID of the parent submodel
//   - elements: Slice of submodel elements to insert
//   - tx: Transaction context (if nil, a new transaction will be created)
//   - ctx: Optional context for nested insertion (nil for top-level elements)
//
// Returns:
//   - []int: Database IDs of the inserted elements (in same order as input)
//   - error: An error if any insertion fails
//
//nolint:revive // cognitive-complexity is acceptable here due to the batch operation nature
func BatchInsert(db *sql.DB, submodelID string, elements []types.ISubmodelElement, tx *sql.Tx, ctx *BatchInsertContext) ([]int, error) {
	// Handle empty elements slice
	if len(elements) == 0 {
		return []int{}, nil
	}

	// Default context for top-level elements
	if ctx == nil {
		ctx = &BatchInsertContext{
			ParentID:   0,
			ParentPath: "",
			RootSmeID:  0,
			IsFromList: false,
		}
	}

	// Manage transaction lifecycle
	var localTx *sql.Tx
	var err error
	ownTransaction := tx == nil

	if ownTransaction {
		localTx, _, err = common.StartTransaction(db)
		if err != nil {
			return nil, common.NewInternalServerError("Failed to start transaction for batch insert: " + err.Error())
		}
		defer func() {
			if err != nil {
				_ = localTx.Rollback()
			}
		}()
	} else {
		localTx = tx
	}

	dialect := goqu.Dialect("postgres")
	jsonLib := jsoniter.ConfigCompatibleWithStandardLibrary

	// Prepare batch insert elements with their handlers and paths
	batchElements := make([]batchInsertElement, 0, len(elements))
	for i, element := range elements {
		handler, handlerErr := GetSMEHandler(element, db)
		if handlerErr != nil {
			err = handlerErr
			return nil, err
		}
		idShort := ""
		if element.IDShort() != nil {
			idShort = *element.IDShort()
		}

		// Calculate position with context offset
		position := ctx.StartPosition + i

		// Build idShortPath based on context
		var idShortPath string
		if ctx.ParentPath == "" {
			// Top-level element or first level in a container
			if ctx.IsFromList {
				idShortPath = "[" + strconv.Itoa(position) + "]"
			} else {
				idShortPath = idShort
			}
		} else {
			// Nested element
			if ctx.IsFromList {
				idShortPath = ctx.ParentPath + "[" + strconv.Itoa(position) + "]"
			} else {
				idShortPath = ctx.ParentPath + "." + idShort
			}
		}

		batchElements = append(batchElements, batchInsertElement{
			element:     element,
			handler:     handler,
			position:    position,
			idShort:     idShort,
			idShortPath: idShortPath,
			isFromList:  ctx.IsFromList,
		})
	}

	// Collect all SemanticIDs, Descriptions, and DisplayNames for batch insertion
	semanticIDs := make([]types.IReference, len(batchElements))
	descriptions := make([][]types.ILangStringTextType, len(batchElements))
	displayNames := make([][]types.ILangStringNameType, len(batchElements))
	for i, be := range batchElements {
		semanticIDs[i] = be.element.SemanticID()
		descriptions[i] = be.element.Description()
		displayNames[i] = be.element.DisplayName()
	}

	// Batch insert all SemanticIDs
	semanticIDResults, semErr := persistenceutils.BatchCreateReferences(localTx, semanticIDs)
	if semErr != nil {
		err = common.NewInternalServerError("Failed to batch create SemanticIDs: " + semErr.Error())
		return nil, err
	}

	// Batch insert all Descriptions
	descriptionIDResults, descErr := persistenceutils.BatchCreateLangStringTextTypes(localTx, descriptions)
	if descErr != nil {
		err = common.NewInternalServerError("Failed to batch create Descriptions: " + descErr.Error())
		return nil, err
	}

	// Batch insert all DisplayNames
	displayNameIDResults, dispErr := persistenceutils.BatchCreateLangStringNameTypes(localTx, displayNames)
	if dispErr != nil {
		err = common.NewInternalServerError("Failed to batch create DisplayNames: " + dispErr.Error())
		return nil, err
	}

	// Build base submodel_element records using pre-computed IDs
	baseRecords := make([]goqu.Record, 0, len(batchElements))
	for i, be := range batchElements {
		params := baseRecordParams{
			SubmodelID:    submodelID,
			Element:       be.element,
			IDShort:       be.idShort,
			IDShortPath:   be.idShortPath,
			Position:      be.position,
			ParentID:      ctx.ParentID,
			RootSmeID:     ctx.RootSmeID,
			SemanticID:    semanticIDResults[i],
			DescriptionID: descriptionIDResults[i],
			DisplayNameID: displayNameIDResults[i],
		}
		record, buildErr := buildBaseSubmodelElementRecord(params, jsonLib)
		if buildErr != nil {
			err = buildErr
			return nil, err
		}
		baseRecords = append(baseRecords, record)
	}

	// Batch insert all base submodel_element records and get their IDs
	// Convert baseRecords to []interface{} for variadic Rows() call
	rowsToInsert := make([]interface{}, len(baseRecords))
	for i, record := range baseRecords {
		rowsToInsert[i] = record
	}

	insertQuery := dialect.Insert("submodel_element").
		Cols("submodel_id", "parent_sme_id", "position", "id_short", "category", "model_type", "semantic_id", "idshort_path", "description_id", "displayname_id", "root_sme_id", "embedded_data_specification", "supplemental_semantic_ids", "extensions").
		Rows(rowsToInsert...).
		Returning("id")

	sqlQuery, args, buildErr := insertQuery.ToSQL()
	if buildErr != nil {
		err = buildErr
		return nil, err
	}

	rows, execErr := localTx.Query(sqlQuery, args...)
	if execErr != nil {
		err = execErr
		return nil, err
	}

	// Collect the returned IDs
	ids := make([]int, 0, len(batchElements))
	for rows.Next() {
		var id int
		if scanErr := rows.Scan(&id); scanErr != nil {
			_ = rows.Close()
			err = scanErr
			return nil, err
		}
		ids = append(ids, id)
	}
	if closeErr := rows.Close(); closeErr != nil {
		err = closeErr
		return nil, err
	}
	if rowErr := rows.Err(); rowErr != nil {
		err = rowErr
		return nil, err
	}

	// Verify we got back the expected number of IDs
	if len(ids) != len(batchElements) {
		err = common.NewInternalServerError(fmt.Sprintf("batch insert returned %d IDs but expected %d", len(ids), len(batchElements)))
		return nil, err
	}

	// For top-level elements (RootSmeID == 0), update root_sme_id to their own ID
	if ctx.RootSmeID == 0 && len(ids) > 0 {
		updateQuery, updateArgs, buildErr := dialect.Update("submodel_element").
			Set(goqu.Record{"root_sme_id": goqu.L("id")}).
			Where(goqu.C("id").In(ids)).
			ToSQL()
		if buildErr != nil {
			err = buildErr
			return nil, err
		}
		_, execErr := localTx.Exec(updateQuery, updateArgs...)
		if execErr != nil {
			err = execErr
			return nil, err
		}
	}

	// Group type-specific records by table name
	typeSpecificRecords := make(map[string][]goqu.Record)
	for i, be := range batchElements {
		queryPart, partErr := be.handler.GetInsertQueryPart(localTx, ids[i], be.element)
		if partErr != nil {
			err = partErr
			return nil, err
		}
		if queryPart != nil {
			typeSpecificRecords[queryPart.TableName] = append(typeSpecificRecords[queryPart.TableName], queryPart.Record)
		}
	}

	// Batch insert each type-specific table
	for tableName, records := range typeSpecificRecords {
		if len(records) == 0 {
			continue
		}
		// Convert records to []interface{} for variadic Rows() call
		typeRows := make([]interface{}, len(records))
		for i, record := range records {
			typeRows[i] = record
		}
		typeInsertQuery := dialect.Insert(tableName).Rows(typeRows...)
		typeSQLQuery, typeArgs, buildErr := typeInsertQuery.ToSQL()
		if buildErr != nil {
			err = buildErr
			return nil, err
		}
		_, execErr := localTx.Exec(typeSQLQuery, typeArgs...)
		if execErr != nil {
			err = execErr
			return nil, err
		}
	}

	// Batch insert MultiLanguageProperty values (secondary table that requires parent to exist first)
	mlpValueRows := make([]interface{}, 0)
	for i, be := range batchElements {
		if be.element.ModelType() == types.ModelTypeMultiLanguageProperty {
			mlp, ok := be.element.(*types.MultiLanguageProperty)
			if ok && len(mlp.Value()) > 0 {
				for _, val := range mlp.Value() {
					mlpValueRows = append(mlpValueRows, goqu.Record{
						"mlp_id":   ids[i],
						"language": val.Language(),
						"text":     val.Text(),
					})
				}
			}
		}
	}
	if len(mlpValueRows) > 0 {
		mlpInsertQuery := dialect.Insert("multilanguage_property_value").
			Cols("mlp_id", "language", "text").
			Rows(mlpValueRows...)
		mlpSQLQuery, mlpArgs, buildErr := mlpInsertQuery.ToSQL()
		if buildErr != nil {
			err = buildErr
			return nil, err
		}
		_, execErr := localTx.Exec(mlpSQLQuery, mlpArgs...)
		if execErr != nil {
			err = execErr
			return nil, err
		}
	}

	// Insert qualifiers for all elements
	for i, be := range batchElements {
		qualifiers := be.element.Qualifiers()
		if len(qualifiers) > 0 {
			for qi, qualifier := range qualifiers {
				qualifierID, qualErr := persistenceutils.CreateQualifier(localTx, qualifier, qi)
				if qualErr != nil {
					err = qualErr
					return nil, err
				}

				insertQualQuery, insertQualArgs, buildErr := dialect.Insert("submodel_element_qualifier").
					Rows(goqu.Record{
						"sme_id":       ids[i],
						"qualifier_id": qualifierID,
					}).
					ToSQL()
				if buildErr != nil {
					err = buildErr
					return nil, err
				}
				_, execErr := localTx.Exec(insertQualQuery, insertQualArgs...)
				if execErr != nil {
					err = execErr
					return nil, err
				}
			}
		}
	}

	// Process nested children for each element
	for i, be := range batchElements {
		children := getChildElements(be.element)
		if len(children) == 0 {
			continue
		}

		// Determine the rootSmeID for children
		childRootSmeID := ctx.RootSmeID
		if childRootSmeID == 0 {
			// This is a top-level element, so children inherit this element's ID as root
			childRootSmeID = ids[i]
		}

		// Determine if children are from a list
		isFromList := be.element.ModelType() == types.ModelTypeSubmodelElementList

		childCtx := &BatchInsertContext{
			ParentID:   ids[i],
			ParentPath: be.idShortPath,
			RootSmeID:  childRootSmeID,
			IsFromList: isFromList,
		}

		_, childErr := BatchInsert(db, submodelID, children, localTx, childCtx)
		if childErr != nil {
			err = childErr
			return nil, err
		}
	}

	// Commit if we own the transaction
	if ownTransaction {
		if commitErr := localTx.Commit(); commitErr != nil {
			err = common.NewInternalServerError("Failed to commit batch insert transaction: " + commitErr.Error())
			return nil, err
		}
	}

	return ids, nil
}

// getChildElements extracts child elements from container-type submodel elements.
// Returns an empty slice for element types that don't have children.
func getChildElements(element types.ISubmodelElement) []types.ISubmodelElement {
	switch element.ModelType() {
	case types.ModelTypeSubmodelElementCollection:
		if coll, ok := element.(*types.SubmodelElementCollection); ok {
			return coll.Value()
		}
	case types.ModelTypeSubmodelElementList:
		if list, ok := element.(*types.SubmodelElementList); ok {
			return list.Value()
		}
	case types.ModelTypeAnnotatedRelationshipElement:
		if rel, ok := element.(*types.AnnotatedRelationshipElement); ok {
			children := make([]types.ISubmodelElement, 0, len(rel.Annotations()))
			for _, ann := range rel.Annotations() {
				children = append(children, ann)
			}
			return children
		}
	case types.ModelTypeEntity:
		if ent, ok := element.(*types.Entity); ok {
			return ent.Statements()
		}
	}
	return nil
}

// baseRecordParams contains all parameters needed to build a base submodel_element record.
type baseRecordParams struct {
	SubmodelID    string
	Element       types.ISubmodelElement
	IDShort       string
	IDShortPath   string
	Position      int
	ParentID      int
	RootSmeID     int
	SemanticID    sql.NullInt64
	DescriptionID sql.NullInt64
	DisplayNameID sql.NullInt64
}

// buildBaseSubmodelElementRecord builds the base submodel_element record using pre-computed reference IDs.
// This is used for optimized batch insertion where SemanticID, Description, and DisplayName IDs
// have already been created in batch operations.
func buildBaseSubmodelElementRecord(params baseRecordParams, jsonLib jsoniter.API) (goqu.Record, error) {
	// Handle EmbeddedDataSpecifications
	edsJSONString := "[]"
	eds := params.Element.EmbeddedDataSpecifications()
	if len(eds) > 0 {
		var toJson []map[string]any
		for _, ed := range eds {
			jsonObj, jsonErr := jsonization.ToJsonable(ed)
			if jsonErr != nil {
				return nil, common.NewErrBadRequest("Failed to convert EmbeddedDataSpecification: " + jsonErr.Error())
			}
			toJson = append(toJson, jsonObj)
		}
		edsBytes, marshalErr := jsonLib.Marshal(toJson)
		if marshalErr != nil {
			return nil, marshalErr
		}
		edsJSONString = string(edsBytes)
	}

	// Handle SupplementalSemanticIDs
	supplementalSemanticIDsJSONString := "[]"
	supplementalSemanticIDs := params.Element.SupplementalSemanticIDs()
	if len(supplementalSemanticIDs) > 0 {
		var toJson []map[string]any
		for _, ref := range supplementalSemanticIDs {
			jsonObj, jsonErr := jsonization.ToJsonable(ref)
			if jsonErr != nil {
				return nil, common.NewErrBadRequest("Failed to convert SupplementalSemanticID: " + jsonErr.Error())
			}
			toJson = append(toJson, jsonObj)
		}
		supplBytes, marshalErr := json.Marshal(toJson)
		if marshalErr != nil {
			return nil, marshalErr
		}
		supplementalSemanticIDsJSONString = string(supplBytes)
	}

	// Handle Extensions
	extensionsJSONString := "[]"
	extensions := params.Element.Extensions()
	if len(extensions) > 0 {
		var toJson []map[string]any
		for _, ext := range extensions {
			jsonObj, jsonErr := jsonization.ToJsonable(ext)
			if jsonErr != nil {
				return nil, common.NewErrBadRequest("Failed to convert Extension: " + jsonErr.Error())
			}
			toJson = append(toJson, jsonObj)
		}
		extensionsBytes, marshalErr := jsonLib.Marshal(toJson)
		if marshalErr != nil {
			return nil, marshalErr
		}
		extensionsJSONString = string(extensionsBytes)
	}

	// Build parent_sme_id (NULL for top-level elements)
	var parentDBId sql.NullInt64
	if params.ParentID == 0 {
		parentDBId = sql.NullInt64{}
	} else {
		parentDBId = sql.NullInt64{Int64: int64(params.ParentID), Valid: true}
	}

	// Build root_sme_id (will be updated later for top-level elements)
	var rootDbID sql.NullInt64
	if params.RootSmeID == 0 {
		rootDbID = sql.NullInt64{}
	} else {
		rootDbID = sql.NullInt64{Int64: int64(params.RootSmeID), Valid: true}
	}

	return goqu.Record{
		"submodel_id":                 params.SubmodelID,
		"parent_sme_id":               parentDBId,
		"position":                    params.Position,
		"id_short":                    params.IDShort,
		"category":                    params.Element.Category(),
		"model_type":                  params.Element.ModelType(),
		"semantic_id":                 params.SemanticID,
		"idshort_path":                params.IDShortPath,
		"description_id":              params.DescriptionID,
		"displayname_id":              params.DisplayNameID,
		"root_sme_id":                 rootDbID,
		"embedded_data_specification": edsJSONString,
		"supplemental_semantic_ids":   supplementalSemanticIDsJSONString,
		"extensions":                  extensionsJSONString,
	}, nil
}
