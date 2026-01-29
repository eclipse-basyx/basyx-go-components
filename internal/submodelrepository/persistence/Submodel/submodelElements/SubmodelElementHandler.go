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
	"errors"
	"sort"
	"strconv"
	"sync"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // Postgres Driver for Goqu

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	submodelsubqueries "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel/queries"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
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
func GetSMEHandler(submodelElement model.SubmodelElement, db *sql.DB) (PostgreSQLSMECrudInterface, error) {
	return GetSMEHandlerByModelType(string(submodelElement.GetModelType()), db)
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
func GetSMEHandlerByModelType(modelType string, db *sql.DB) (PostgreSQLSMECrudInterface, error) {
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
		if modelType == "File" {
			// We have to check the database because File could be ambiguous between File and Blob
			actual, err := GetModelTypeByIdShortPathAndSubmodelID(db, submodelID, elem.IdShortPath)
			if err != nil {
				return err
			}
			modelType = actual
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
		modelType := elem.Element.GetModelType()
		if modelType == "File" {
			// We have to check the database because File could be ambiguous between File and Blob
			actual, err := GetModelTypeByIdShortPathAndSubmodelID(db, submodelID, elem.IdShortPath)
			if err != nil {
				return err
			}
			modelType = actual
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
func GetModelTypeByIdShortPathAndSubmodelID(db *sql.DB, submodelID string, idShortOrPath string) (string, error) {
	dialect := goqu.Dialect("postgres")

	query, args, err := dialect.From("submodel_element").
		Select("model_type").
		Where(
			goqu.C("submodel_id").Eq(submodelID),
			goqu.C("idshort_path").Eq(idShortOrPath),
		).
		ToSQL()
	if err != nil {
		return "", err
	}

	var modelType string
	err = db.QueryRow(query, args...).Scan(&modelType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", common.NewErrNotFound("Submodel-Element ID-Short: " + idShortOrPath)
		}
		return "", err
	}
	return modelType, nil
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
		return err
	}
	result, err := tx.Exec(sqlQuery, args...)
	if err != nil {
		return err
	}
	affectedRows, err := result.RowsAffected()
	if err != nil {
		return err
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
					return err
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
			return err
		}

		var parentID int
		err = tx.QueryRow(selectQuery, selectArgs...).Scan(&parentID)
		if err != nil {
			return err
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
			return err
		}
		_, err = tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			return err
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
			return err
		}
		_, err = tx.Exec(updatePathQuery, updatePathArgs...)
		if err != nil {
			return err
		}
	}
	if affectedRows == 0 {
		return common.NewErrNotFound("Submodel-Element ID-Short: " + idShortOrPath)
	}
	return nil
}

// GetSubmodelElementsForSubmodel retrieves all submodel elements for a given submodel ID.
func GetSubmodelElementsForSubmodel(db *sql.DB, submodelID string, idShortPath string, cursor string, limit int, valueOnly bool) ([]model.SubmodelElement, string, error) {
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
				sme, builder, err := builder.BuildSubmodelElement(smeRow)
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
					element:  *sme,
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

	res := make([]model.SubmodelElement, 0, len(roots))
	for _, r := range roots {
		res = append(res, r.element)
	}

	var nextCursor string
	if (len(res) > limit) && limit != -1 {
		nextCursor = res[limit].GetIdShort()
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

		switch p := parent.element.(type) {
		case *model.SubmodelElementCollection:
			for _, ch := range kids {
				p.Value = append(p.Value, ch.element)
			}
		case *model.SubmodelElementList:
			for _, ch := range kids {
				p.Value = append(p.Value, ch.element)
			}
		case *model.AnnotatedRelationshipElement:
			for _, ch := range kids {
				p.Annotations = append(p.Annotations, ch.element)
			}
		case *model.Entity:
			for _, ch := range kids {
				p.Statements = append(p.Statements, ch.element)
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
	id       int64                 // Database ID of the element
	parentID int64                 // Parent element ID for hierarchy
	path     string                // Full path for navigation
	position int                   // Position within parent for ordering
	element  model.SubmodelElement // The actual submodel element data
}
