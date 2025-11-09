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
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
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
func GetSMEHandler(submodelElement gen.SubmodelElement, db *sql.DB) (PostgreSQLSMECrudInterface, error) {
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
//   - EventElement: Base event element
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
	var handler PostgreSQLSMECrudInterface

	switch modelType {
	case "AnnotatedRelationshipElement":
		areHandler, err := NewPostgreSQLAnnotatedRelationshipElementHandler(db)
		if err != nil {
			fmt.Println("Error creating AnnotatedRelationshipElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create AnnotatedRelationshipElement handler. See console for details.")
		}
		handler = areHandler
	case "BasicEventElement":
		beeHandler, err := NewPostgreSQLBasicEventElementHandler(db)
		if err != nil {
			fmt.Println("Error creating BasicEventElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create BasicEventElement handler. See console for details.")
		}
		handler = beeHandler
	case "Blob":
		blobHandler, err := NewPostgreSQLBlobHandler(db)
		if err != nil {
			fmt.Println("Error creating Blob handler:", err)
			return nil, common.NewInternalServerError("Failed to create Blob handler. See console for details.")
		}
		handler = blobHandler
	case "Capability":
		capHandler, err := NewPostgreSQLCapabilityHandler(db)
		if err != nil {
			fmt.Println("Error creating Capability handler:", err)
			return nil, common.NewInternalServerError("Failed to create Capability handler. See console for details.")
		}
		handler = capHandler
	case "Entity":
		entityHandler, err := NewPostgreSQLEntityHandler(db)
		if err != nil {
			fmt.Println("Error creating Entity handler:", err)
			return nil, common.NewInternalServerError("Failed to create Entity handler. See console for details.")
		}
		handler = entityHandler
	case "EventElement":
		eventElemHandler, err := NewPostgreSQLEventElementHandler(db)
		if err != nil {
			fmt.Println("Error creating EventElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create EventElement handler. See console for details.")
		}
		handler = eventElemHandler
	case "File":
		fileHandler, err := NewPostgreSQLFileHandler(db)
		if err != nil {
			fmt.Println("Error creating File handler:", err)
			return nil, common.NewInternalServerError("Failed to create File handler. See console for details.")
		}
		handler = fileHandler
	case "MultiLanguageProperty":
		mlpHandler, err := NewPostgreSQLMultiLanguagePropertyHandler(db)
		if err != nil {
			fmt.Println("Error creating MultiLanguageProperty handler:", err)
			return nil, common.NewInternalServerError("Failed to create MultiLanguageProperty handler. See console for details.")
		}
		handler = mlpHandler
	case "Operation":
		opHandler, err := NewPostgreSQLOperationHandler(db)
		if err != nil {
			fmt.Println("Error creating Operation handler:", err)
			return nil, common.NewInternalServerError("Failed to create Operation handler. See console for details.")
		}
		handler = opHandler
	case "Property":
		propHandler, err := NewPostgreSQLPropertyHandler(db)
		if err != nil {
			fmt.Println("Error creating Property handler:", err)
			return nil, common.NewInternalServerError("Failed to create Property handler. See console for details.")
		}
		handler = propHandler
	case "Range":
		rangeHandler, err := NewPostgreSQLRangeHandler(db)
		if err != nil {
			fmt.Println("Error creating Range handler:", err)
			return nil, common.NewInternalServerError("Failed to create Range handler. See console for details.")
		}
		handler = rangeHandler
	case "ReferenceElement":
		refElemHandler, err := NewPostgreSQLReferenceElementHandler(db)
		if err != nil {
			fmt.Println("Error creating ReferenceElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create ReferenceElement handler. See console for details.")
		}
		handler = refElemHandler
	case "RelationshipElement":
		relElemHandler, err := NewPostgreSQLRelationshipElementHandler(db)
		if err != nil {
			fmt.Println("Error creating RelationshipElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create RelationshipElement handler. See console for details.")
		}
		handler = relElemHandler
	case "SubmodelElementCollection":
		smeColHandler, err := NewPostgreSQLSubmodelElementCollectionHandler(db)
		if err != nil {
			fmt.Println("Error creating SubmodelElementCollection handler:", err)
			return nil, common.NewInternalServerError("Failed to create SubmodelElementCollection handler. See console for details.")
		}
		handler = smeColHandler
	case "SubmodelElementList":
		smeListHandler, err := NewPostgreSQLSubmodelElementListHandler(db)
		if err != nil {
			fmt.Println("Error creating SubmodelElementList handler:", err)
			return nil, common.NewInternalServerError("Failed to create SubmodelElementList handler. See console for details.")
		}
		handler = smeListHandler
	default:
		return nil, errors.New("ModelType " + modelType + " unsupported.")
	}
	return handler, nil
}

// GetSubmodelElementsWithPath retrieves submodel elements with support for hierarchical paths and pagination.
//
// This function performs complex queries to fetch submodel elements either by specific path
// (subtree retrieval) or paginated root-level elements with their complete subtrees. It uses
// Common Table Expressions (CTEs) for efficient hierarchical data retrieval and supports
// cursor-based pagination for large result sets.
//
// The function handles two query modes:
//  1. Path-based (idShortOrPath specified): Retrieves a specific element and its entire subtree
//  2. Pagination (idShortOrPath empty): Retrieves paginated root elements with their subtrees
//
// Features:
//   - Cursor-based pagination for stable paging through results
//   - Single optimized query with JSON aggregation for all element data
//   - In-memory hierarchy reconstruction for nested collections and lists
//   - Type-safe element materialization without reflection
//   - Proper ordering by position and path for consistent results
//
// Parameters:
//   - db: Database connection for reference queries (e.g., semantic IDs)
//   - tx: Transaction context for all queries
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: Optional path to specific element (empty for root pagination)
//   - limit: Maximum number of root elements to return (minimum 1, default 100)
//   - cursor: Pagination cursor (idShort of last element from previous page)
//
// Returns:
//   - []gen.SubmodelElement: Slice of fully populated submodel elements with nested structures
//   - string: Next cursor for pagination (empty if no more pages)
//   - error: An error if database query fails, submodel not found, or invalid cursor
//
// Example usage:
//
//	// Get first page of root elements
//	elements, cursor, err := GetSubmodelElementsWithPath(db, tx, "submodel123", "", 10, "")
//
//	// Get next page
//	nextElements, nextCursor, err := GetSubmodelElementsWithPath(db, tx, "submodel123", "", 10, cursor)
//
//	// Get specific element and its subtree
//	subtree, _, err := GetSubmodelElementsWithPath(db, tx, "submodel123", "prop1.collection2", 0, "")
func GetSubmodelElementsWithPath(db *sql.DB, tx *sql.Tx, submodelID string, idShortOrPath string, limit int, cursor string) ([]gen.SubmodelElement, string, error) {
	if limit < 1 {
		limit = 100
	}
	// Check if Submodel exists
	ds := goqu.Dialect("postgres").
		From("submodel").
		Select("id").
		Where(goqu.Ex{"id": submodelID})

	qExist, argsExist, err := ds.ToSQL()
	if err != nil {
		return nil, "", err
	}
	sRows, err := tx.Query(qExist, argsExist...)
	if err != nil {
		return nil, "", err
	}
	if !sRows.Next() {
		return nil, "", common.NewErrNotFound("Submodel not found")
	}
	_ = sRows.Close()

	// Get OFFSET based on Cursor
	offset := 0
	if cursor != "" {
		ds := goqu.Dialect("postgres").
			From("submodel_element").
			Select(
				goqu.L("ROW_NUMBER() OVER (ORDER BY id_short) AS position"),
				goqu.I("id_short"),
			).
			Where(
				goqu.Ex{
					"submodel_id":   submodelID,
					"parent_sme_id": nil,
				},
			)

		qCursor, argsCursor, err := ds.ToSQL()
		if err != nil {
			return nil, "", err
		}

		cRows, err := tx.Query(qCursor, argsCursor...)
		if err != nil {
			return nil, "", err
		}
		defer func() { _ = cRows.Close() }()
		found := false
		for cRows.Next() {
			var position int
			var idShort string
			if err := cRows.Scan(&position, &idShort); err != nil {
				return nil, "", err
			}
			if idShort == cursor {
				found = true
				offset = position
				// Continue consuming all rows to avoid leftovers
			}
		}
		if err := cRows.Err(); err != nil {
			return nil, "", err
		}
		if !found {
			return nil, "", common.NewErrBadRequest("Invalid cursor " + cursor)
		}
	}

	// --- Build the unified query with CTE ----------------------------------------------------------
	var cte string
	args := []any{submodelID}
	if idShortOrPath != "" {
		// Subtree: Fetch all elements in the subtree
		cte = `
            WITH subtree AS (
                SELECT * FROM submodel_element
                WHERE submodel_id = $1 AND (idshort_path = $2 OR idshort_path LIKE $2 || '.%' OR idshort_path LIKE $2 || '[%')
            )`
		args = append(args, idShortOrPath)
	} else {
		// Pagination: Fetch paginated roots, then their subtrees
		cte = `
			WITH paginated_roots AS (
				SELECT id_short FROM submodel_element
				WHERE submodel_id = $1 AND parent_sme_id IS NULL
				ORDER BY id_short OFFSET $2 LIMIT $3
			),
			subtree_elements AS (
				SELECT sme.* FROM submodel_element sme
				WHERE sme.submodel_id = $1 AND EXISTS (
					SELECT 1 FROM paginated_roots pr
					WHERE sme.idshort_path = pr.id_short
					   OR sme.idshort_path LIKE pr.id_short || '.%'
					   OR sme.idshort_path LIKE pr.id_short || '[%'
				)
			)`
		args = append(args, offset, limit)
	}

	baseQuery := cte + `
        SELECT 
            -- SME base
            sme.id, sme.id_short, sme.category, sme.model_type, sme.idshort_path, sme.position, sme.parent_sme_id, sme.semantic_id,
			` + getSubmodelElementDataQueryPart() + `
        FROM ` + (func() string {
		if idShortOrPath != "" {
			return "subtree sme"
		}
		return "subtree_elements sme"
	}()) + `
		` + getSubmodelElementLeftJoins() + `
        ORDER BY sme.parent_sme_id NULLS FIRST, sme.idshort_path, sme.position`

	rows, err := tx.Query(baseQuery, args...)

	if err != nil {
		return nil, "", err
	}
	defer func() { _ = rows.Close() }()

	// Pre-size conservatively to reduce re-allocations
	nodes := make(map[int64]*node, 256)
	children := make(map[int64][]*node, 256)
	roots := make([]*node, 0, 16)
	var target *node // element whose idshort_path == idShortOrPath (if provided)

	for rows.Next() {
		var (
			// SME base
			id                                        int64
			idShort, category, modelType, idShortPath string
			position                                  sql.NullInt32
			parentSmeID                               sql.NullInt64
			semanticID                                sql.NullInt64
			// Property
			propValueType, propValue sql.NullString
			// Blob
			blobContentType sql.NullString
			blobValue       []byte
			// File
			fileContentType, fileValue sql.NullString
			// Range
			rangeValueType, rangeMin, rangeMax sql.NullString
			// SubmodelElementList
			typeValueListElement, valueTypeListElement sql.NullString
			orderRelevant                              sql.NullBool
			// MultiLanguageProperty
			mlpValueID sql.NullInt64
			// ReferenceElement
			refValueRef sql.NullInt64
			// RelationshipElement
			relFirstRef, relSecondRef sql.NullInt64
			// Entity
			entityType          sql.NullString
			entityGlobalAssetID sql.NullString
			// BasicEventElement
			beeObservedRef      sql.NullInt64
			beeDirection        sql.NullString
			beeState            sql.NullString
			beeMessageTopic     sql.NullString
			beeMessageBrokerRef sql.NullInt64
			beeLastUpdate       sql.NullTime
			beeMinInterval      sql.NullString
			beeMaxInterval      sql.NullString
		)

		if err := rows.Scan(
			&id, &idShort, &category, &modelType, &idShortPath, &position, &parentSmeID, &semanticID,
			&propValueType, &propValue,
			&blobContentType, &blobValue,
			&fileContentType, &fileValue,
			&rangeValueType, &rangeMin, &rangeMax,
			&typeValueListElement, &valueTypeListElement, &orderRelevant,
			&mlpValueID,
			&refValueRef,
			&relFirstRef, &relSecondRef,
			&entityType, &entityGlobalAssetID,
			&beeObservedRef, &beeDirection, &beeState, &beeMessageTopic, &beeMessageBrokerRef, &beeLastUpdate, &beeMinInterval, &beeMaxInterval,
		); err != nil {
			return nil, "", err
		}

		// Materialize the concrete element based on modelType (no reflection)
		start := time.Now().Local().UnixMicro()
		var semanticIDObj *gen.Reference
		if semanticID.Valid {
			semanticIDObj, err = persistenceutils.GetReferenceByReferenceDBID(db, semanticID)
			if err != nil {
				return nil, "", err
			}
		}
		end := time.Now().Local().UnixMicro()
		fmt.Printf("SME SemanticID time: %d microseconds\n", end-start)

		start = time.Now().Local().UnixMicro()
		var el gen.SubmodelElement
		switch modelType {
		case "Property":
			prop := &gen.Property{IdShort: idShort, Category: category, ModelType: modelType, SemanticID: semanticIDObj}
			if propValueType.Valid {
				if vt, err := gen.NewDataTypeDefXsdFromValue(propValueType.String); err == nil {
					prop.ValueType = vt
				}
			}
			if propValue.Valid {
				prop.Value = propValue.String
			}
			el = prop

		case "MultiLanguageProperty":
			// Values are in a separate table; we only hydrate the shell here.
			mlp := &gen.MultiLanguageProperty{IdShort: idShort, Category: category, ModelType: modelType, SemanticID: semanticIDObj}
			el = mlp

		case "Blob":
			blob := &gen.Blob{IdShort: idShort, Category: category, ModelType: modelType, SemanticID: semanticIDObj}
			if blobContentType.Valid {
				blob.ContentType = blobContentType.String
			}
			if blobValue != nil {
				blob.Value = string(blobValue)
			}
			el = blob

		case "File":
			file := &gen.File{IdShort: idShort, Category: category, ModelType: modelType, SemanticID: semanticIDObj}
			if fileContentType.Valid {
				file.ContentType = fileContentType.String
			}
			if fileValue.Valid {
				file.Value = fileValue.String
			}
			el = file

		case "Range":
			rg := &gen.Range{IdShort: idShort, Category: category, ModelType: modelType, SemanticID: semanticIDObj}
			if rangeValueType.Valid {
				if vt, err := gen.NewDataTypeDefXsdFromValue(rangeValueType.String); err == nil {
					rg.ValueType = vt
				}
			}
			if rangeMin.Valid {
				rg.Min = rangeMin.String
			}
			if rangeMax.Valid {
				rg.Max = rangeMax.String
			}
			el = rg

		case "SubmodelElementCollection":
			coll := &gen.SubmodelElementCollection{IdShort: idShort, Category: category, ModelType: modelType, Value: make([]gen.SubmodelElement, 0, 4)}
			el = coll

		case "SubmodelElementList":
			lst := &gen.SubmodelElementList{IdShort: idShort, Category: category, ModelType: modelType, Value: make([]gen.SubmodelElement, 0, 4)}
			if typeValueListElement.Valid {
				if tv, err := gen.NewAasSubmodelElementsFromValue(typeValueListElement.String); err == nil {
					lst.TypeValueListElement = &tv
				}
			}
			if valueTypeListElement.Valid {
				if vt, err := gen.NewDataTypeDefXsdFromValue(valueTypeListElement.String); err == nil {
					lst.ValueTypeListElement = vt
				}
			}
			if orderRelevant.Valid {
				lst.OrderRelevant = orderRelevant.Bool
			}
			el = lst

		case "ReferenceElement":
			refElem := &gen.ReferenceElement{IdShort: idShort, Category: category, ModelType: modelType, SemanticID: semanticIDObj}
			el = refElem

		case "RelationshipElement":
			relElem := &gen.RelationshipElement{IdShort: idShort, Category: category, ModelType: modelType, SemanticID: semanticIDObj}
			el = relElem

		case "AnnotatedRelationshipElement":
			areElem := &gen.AnnotatedRelationshipElement{IdShort: idShort, Category: category, ModelType: modelType, SemanticID: semanticIDObj}
			el = areElem

		case "Entity":
			entity := &gen.Entity{IdShort: idShort, Category: category, ModelType: modelType, SemanticID: semanticIDObj}
			if entityType.Valid {
				if et, err := gen.NewEntityTypeFromValue(entityType.String); err == nil {
					entity.EntityType = et
				}
			}
			if entityGlobalAssetID.Valid {
				entity.GlobalAssetID = entityGlobalAssetID.String
			}
			el = entity

		case "Operation":
			op := &gen.Operation{IdShort: idShort, Category: category, ModelType: modelType, SemanticID: semanticIDObj}
			el = op

		case "BasicEventElement":
			bee := &gen.BasicEventElement{IdShort: idShort, Category: category, ModelType: modelType, SemanticID: semanticIDObj}
			if beeDirection.Valid {
				if d, err := gen.NewDirectionFromValue(beeDirection.String); err == nil {
					bee.Direction = d
				}
			}
			if beeState.Valid {
				if s, err := gen.NewStateOfEventFromValue(beeState.String); err == nil {
					bee.State = s
				}
			}
			if beeMessageTopic.Valid {
				bee.MessageTopic = beeMessageTopic.String
			}
			if beeLastUpdate.Valid {
				bee.LastUpdate = beeLastUpdate.Time.Format(time.RFC3339)
			}
			// Intervals not set for now
			el = bee

		case "Capability":
			capability := &gen.Capability{IdShort: idShort, Category: category, ModelType: modelType, SemanticID: semanticIDObj}
			el = capability

		default:
			// Unknown/unsupported type: skip eagerly.
			continue
		}
		end = time.Now().Local().UnixMicro()
		fmt.Printf("SME Materialization time for %s: %d microseconds\n", modelType, end-start)
		n := &node{
			id:       id,
			parentID: parentSmeID,
			path:     idShortPath,
			position: position,
			element:  el,
		}
		nodes[id] = n

		if parentSmeID.Valid {
			pid := parentSmeID.Int64
			children[pid] = append(children[pid], n)
		} else {
			// For both subtree and pagination, roots are elements with no parent in the fetched data
			roots = append(roots, n)
		}

		if idShortOrPath != "" && idShortPath == idShortOrPath {
			target = n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	// --- Attach children (O(n)) ----------------------------------------------
	// We only attach to SMC/SML parents; other types cannot have children.
	attachChildrenToSubmodelElements(nodes, children)

	// --- Build result ---------------------------------------------------------
	if idShortOrPath != "" && target != nil {
		return []gen.SubmodelElement{target.element}, "", nil
	}

	res := make([]gen.SubmodelElement, 0, len(roots))
	for _, r := range roots {
		res = append(res, r.element)
	}

	if len(res) == 0 {
		return res, "", nil
	}
	// return idShort of last element in res as next cursor
	return res, res[len(res)-1].GetIdShort(), nil
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

		// Stable order: by position (if set), otherwise by path as tie-breaker
		sort.SliceStable(kids, func(i, j int) bool {
			a, b := kids[i], kids[j]
			switch {
			case a.position.Valid && b.position.Valid:
				if a.position.Int32 != b.position.Int32 {
					return a.position.Int32 < b.position.Int32
				}
				return a.path < b.path
			case a.position.Valid:
				return true
			case b.position.Valid:
				return false
			default:
				return a.path < b.path
			}
		})

		switch p := parent.element.(type) {
		case *gen.SubmodelElementCollection:
			for _, ch := range kids {
				p.Value = append(p.Value, ch.element)
			}
		case *gen.SubmodelElementList:
			for _, ch := range kids {
				p.Value = append(p.Value, ch.element)
			}
		}
	}
}

// getSubmodelElementLeftJoins returns the SQL LEFT JOIN clauses for submodel element type tables.
//
// This function generates the JOIN statements needed to fetch type-specific data for all
// submodel element types in a single query. Each element type has its own table with
// specialized columns (e.g., property_element for Property values, blob_element for Blob data).
//
// Returns:
//   - string: SQL fragment containing LEFT JOIN clauses for all element type tables
func getSubmodelElementLeftJoins() string {
	return `
        LEFT JOIN property_element prop ON sme.id = prop.id
        LEFT JOIN blob_element blob ON sme.id = blob.id
        LEFT JOIN file_element file ON sme.id = file.id
        LEFT JOIN range_element range_elem ON sme.id = range_elem.id
        LEFT JOIN submodel_element_list sme_list ON sme.id = sme_list.id
        LEFT JOIN multilanguage_property mlp ON sme.id = mlp.id
        LEFT JOIN reference_element ref_elem ON sme.id = ref_elem.id
        LEFT JOIN relationship_element rel_elem ON sme.id = rel_elem.id
        LEFT JOIN entity_element entity ON sme.id = entity.id
        LEFT JOIN basic_event_element bee ON sme.id = bee.id
	`
}

// getSubmodelElementDataQueryPart returns the SQL SELECT clause for submodel element type-specific data.
//
// This function generates the SELECT portion of the query that fetches all type-specific
// columns from the joined element type tables. It uses COALESCE to handle different value
// storage columns (text, numeric, boolean, time, datetime) and provides proper aliasing
// for all fields.
//
// The returned fragment includes columns for:
//   - Property: value_type and value (text/num/bool/time/datetime variants)
//   - Blob: content_type and binary value
//   - File: content_type and file path/URL
//   - Range: value_type, min and max values (text/num/time/datetime variants)
//   - SubmodelElementList: type_value_list_element, value_type_list_element, order_relevant
//   - MultiLanguageProperty: value_id reference
//   - ReferenceElement: value_ref reference
//   - RelationshipElement: first_ref and second_ref references
//   - Entity: entity_type and global_asset_id
//   - BasicEventElement: observed_ref, direction, state, message_topic, message_broker_ref,
//     last_update, min_interval, max_interval
//
// Returns:
//   - string: SQL fragment containing SELECT columns for all element type data
func getSubmodelElementDataQueryPart() string {
	return `
			-- Property data
            prop.value_type as prop_value_type,
            COALESCE(prop.value_text, prop.value_num::text, prop.value_bool::text, prop.value_time::text, prop.value_datetime::text) as prop_value,
            -- Blob data
            blob.content_type as blob_content_type, blob.value as blob_value,
            -- File data
            file.content_type as file_content_type, file.value as file_value,
            -- Range data
            range_elem.value_type as range_value_type,
            COALESCE(range_elem.min_text, range_elem.min_num::text, range_elem.min_time::text, range_elem.min_datetime::text) as range_min,
            COALESCE(range_elem.max_text, range_elem.max_num::text, range_elem.max_time::text, range_elem.max_datetime::text) as range_max,
            -- SubmodelElementList data
            sme_list.type_value_list_element, sme_list.value_type_list_element, sme_list.order_relevant,
            -- MultiLanguageProperty data
            mlp.value_id as mlp_value_id,
            -- ReferenceElement data
            ref_elem.value_ref as ref_value_ref,
            -- RelationshipElement data
            rel_elem.first_ref as rel_first_ref, rel_elem.second_ref as rel_second_ref,
            -- Entity data
            entity.entity_type as entity_type, entity.global_asset_id as entity_global_asset_id,
            -- BasicEventElement data
            bee.observed_ref as bee_observed_ref, bee.direction as bee_direction, bee.state as bee_state, bee.message_topic as bee_message_topic, bee.message_broker_ref as bee_message_broker_ref, bee.last_update as bee_last_update, bee.min_interval as bee_min_interval, bee.max_interval as bee_max_interval
	`
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
	query := `DELETE FROM submodel_element WHERE submodel_id = $1 AND (idshort_path = $2 OR idshort_path LIKE $2 || '.%' OR idshort_path LIKE $2 || '[%')`
	result, err := tx.Exec(query, submodelID, idShortOrPath)
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
		var parentID int
		err = tx.QueryRow(`SELECT id FROM submodel_element WHERE submodel_id = $1 AND idshort_path = $2`, submodelID, parentPath).Scan(&parentID)
		if err != nil {
			return err
		}

		// update the indices of the remaining elements in the SubmodelElementList
		updateQuery := `UPDATE submodel_element SET position = position - 1 WHERE parent_sme_id = $1 AND position > $2`
		_, err = tx.Exec(updateQuery, parentID, deletedIndex)
		if err != nil {
			return err
		}
		// update their idshort_path as well
		updatePathQuery := `UPDATE submodel_element SET idshort_path = regexp_replace(idshort_path, '\[' || (position + 1) || '\]', '[' || position || ']') WHERE parent_sme_id = $1 AND position >= $2`
		_, err = tx.Exec(updatePathQuery, parentID, deletedIndex)
		if err != nil {
			return err
		}
	}
	if affectedRows == 0 {
		return common.NewErrNotFound("Submodel-Element ID-Short: " + idShortOrPath)
	}
	return nil
}

// node represents an internal tree node for reconstructing submodel element hierarchies.
//
// This type is used during the in-memory reconstruction of the element tree from flat
// database rows. It stores both the database metadata (ID, parent relationship, path)
// and the materialized submodel element object.
//
// Fields:
//   - id: Database primary key of the submodel_element record
//   - parentID: Foreign key to parent element (NULL for root elements)
//   - path: Full idShort path from root (e.g., "collection1.list[0].property2")
//   - position: Numeric position within parent (used for ordering, especially in lists)
//   - element: The actual typed submodel element (Property, Collection, etc.)
type node struct {
	id       int64               // Database ID of the element
	parentID sql.NullInt64       // Parent element ID for hierarchy
	path     string              // Full path for navigation
	position sql.NullInt32       // Position within parent for ordering
	element  gen.SubmodelElement // The actual submodel element data
}
