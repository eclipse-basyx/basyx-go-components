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
	"strconv"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // Postgres Driver for Goqu

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	jsoniter "github.com/json-iterator/go"
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
func UpdateNestedElementsValueOnly(db *sql.DB, elems []ValueOnlyElementsToProcess, idShortOrPath string, submodelID string, tx *sql.Tx) error {
	for _, elem := range elems {
		if elem.IdShortPath == idShortOrPath {
			continue // Skip the root element as it's already processed
		}
		modelType := elem.Element.GetModelType()
		if modelType == types.ModelTypeFile {
			// We have to check the database because File could be ambiguous between File and Blob
			actual, err := GetModelTypeByIdShortPathAndSubmodelIDTx(tx, submodelID, elem.IdShortPath)
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
		err = handler.UpdateValueOnly(submodelID, elem.IdShortPath, elem.Element, tx)
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
func UpdateNestedElements(db *sql.DB, elems []SubmodelElementToProcess, idShortOrPath string, submodelID string, tx *sql.Tx, isPut bool) error {
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
	return getModelTypeByIdShortPathAndSubmodelID(db, submodelID, idShortOrPath)
}

// GetModelTypeByIdShortPathAndSubmodelIDTx retrieves the model type within an existing transaction.
func GetModelTypeByIdShortPathAndSubmodelIDTx(tx *sql.Tx, submodelID string, idShortOrPath string) (*types.ModelType, error) {
	return getModelTypeByIdShortPathAndSubmodelID(tx, submodelID, idShortOrPath)
}

type queryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}

func getModelTypeByIdShortPathAndSubmodelID(queryer queryRower, submodelID string, idShortOrPath string) (*types.ModelType, error) {
	dialect := goqu.Dialect("postgres")
	resolveQuery, resolveArgs, err := dialect.From("submodel").
		Select("id").
		Where(goqu.C("submodel_identifier").Eq(submodelID)).
		ToSQL()
	if err != nil {
		return nil, err
	}

	var submodelDatabaseID int
	err = queryer.QueryRow(resolveQuery, resolveArgs...).Scan(&submodelDatabaseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.NewErrNotFound("SMREPO-GETMODELTYPE-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return nil, err
	}

	query, args, err := dialect.From("submodel_element").
		Select("model_type").
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("idshort_path").Eq(idShortOrPath),
		).
		ToSQL()
	if err != nil {
		return nil, err
	}

	var modelType types.ModelType
	err = queryer.QueryRow(query, args...).Scan(&modelType)
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
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return common.NewErrNotFound("SMREPO-DELSMEBPATH-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return common.NewInternalServerError("SMREPO-DELSMEBPATH-GETSMDATABASEID Failed to resolve Submodel database ID: " + err.Error())
	}

	affectedRows, err := deleteSubmodelElementTree(tx, submodelDatabaseID, idShortOrPath)
	if err != nil {
		return err
	}
	if affectedRows == 0 {
		return common.NewErrNotFound("SMREPO-DELSMEBPATH-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
	}

	if !isListElementPath(idShortOrPath) {
		return nil
	}

	parentPath, deletedIndex, err := splitListElementPath(idShortOrPath)
	if err != nil {
		return err
	}
	return compactListAfterDelete(tx, submodelDatabaseID, parentPath, deletedIndex)
}

func deleteSubmodelElementTree(tx *sql.Tx, submodelDatabaseID int, idShortOrPath string) (int64, error) {
	escapedPath := escapeSQLLikePattern(idShortOrPath)
	del := goqu.Delete("submodel_element").Where(
		goqu.And(
			goqu.I("submodel_id").Eq(submodelDatabaseID),
			goqu.Or(
				goqu.I("idshort_path").Eq(idShortOrPath),
				idShortPathLikeEscaped(goqu.I("idshort_path"), escapedPath+".%"),
				idShortPathLikeEscaped(goqu.I("idshort_path"), escapedPath+"[%"),
			),
		),
	)
	sqlQuery, args, err := del.ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("SMREPO-DELSMEBPATH-TOSQL Failed to build delete query: " + err.Error())
	}
	result, err := tx.Exec(sqlQuery, args...)
	if err != nil {
		return 0, common.NewInternalServerError("SMREPO-DELSMEBPATH-EXEC Failed to execute delete query: " + err.Error())
	}
	affectedRows, err := result.RowsAffected()
	if err != nil {
		return 0, common.NewInternalServerError("SMREPO-DELSMEBPATH-ROWSAFFECTED Failed to get affected rows: " + err.Error())
	}
	return affectedRows, nil
}

func isListElementPath(idShortPath string) bool {
	return strings.HasSuffix(idShortPath, "]")
}

func splitListElementPath(idShortPath string) (string, int, error) {
	indexStart := strings.LastIndex(idShortPath, "[")
	if indexStart < 0 {
		return "", 0, common.NewInternalServerError("SMREPO-DELSMEBPATH-PARSEINDEX Missing list index in path: " + idShortPath)
	}

	deletedIndex, err := strconv.Atoi(idShortPath[indexStart+1 : len(idShortPath)-1])
	if err != nil {
		return "", 0, common.NewInternalServerError("SMREPO-DELSMEBPATH-PARSEINDEX Failed to parse index: " + err.Error())
	}
	return idShortPath[:indexStart], deletedIndex, nil
}

type listChildToCompact struct {
	id          int
	oldPath     string
	oldPosition int
}

func compactListAfterDelete(tx *sql.Tx, submodelDatabaseID int, parentPath string, deletedIndex int) error {
	parentID, err := getListParentID(tx, submodelDatabaseID, parentPath)
	if err != nil {
		return err
	}

	children, err := getListChildrenAfterDeletedIndex(tx, submodelDatabaseID, parentID, deletedIndex)
	if err != nil {
		return err
	}

	for _, child := range children {
		if err = moveListChildOneSlotLeft(tx, submodelDatabaseID, parentPath, child); err != nil {
			return err
		}
	}
	return nil
}

func getListParentID(tx *sql.Tx, submodelDatabaseID int, parentPath string) (int, error) {
	dialect := goqu.Dialect("postgres")
	selectQuery, selectArgs, err := dialect.From("submodel_element").
		Select("id").
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("idshort_path").Eq(parentPath),
		).
		ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("SMREPO-DELSMEBPATH-SELECTPARENT-TOSQL Failed to build select query: " + err.Error())
	}

	var parentID int
	err = tx.QueryRow(selectQuery, selectArgs...).Scan(&parentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, common.NewErrNotFound("SMREPO-DELSMEBPATH-SELECTPARENT-NOTFOUND Parent ID-Short: " + parentPath)
		}
		return 0, common.NewInternalServerError("SMREPO-DELSMEBPATH-SELECTPARENT-EXEC Failed to execute select query: " + err.Error())
	}
	return parentID, nil
}

func getListChildrenAfterDeletedIndex(tx *sql.Tx, submodelDatabaseID int, parentID int, deletedIndex int) ([]listChildToCompact, error) {
	dialect := goqu.Dialect("postgres")
	query, args, err := dialect.From("submodel_element").
		Select("id", "idshort_path", "position").
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("parent_sme_id").Eq(parentID),
			goqu.C("position").Gt(deletedIndex),
		).
		Order(goqu.C("position").Asc()).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-DELSMEBPATH-SELECTSIBLINGS-TOSQL Failed to build sibling query: " + err.Error())
	}

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-DELSMEBPATH-SELECTSIBLINGS-EXEC Failed to execute sibling query: " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	children := make([]listChildToCompact, 0)
	for rows.Next() {
		var child listChildToCompact
		if err = rows.Scan(&child.id, &child.oldPath, &child.oldPosition); err != nil {
			return nil, common.NewInternalServerError("SMREPO-DELSMEBPATH-SELECTSIBLINGS-SCAN Failed to scan sibling row: " + err.Error())
		}
		children = append(children, child)
	}
	if err = rows.Err(); err != nil {
		return nil, common.NewInternalServerError("SMREPO-DELSMEBPATH-SELECTSIBLINGS-ROWS Failed to read sibling rows: " + err.Error())
	}
	return children, nil
}

func moveListChildOneSlotLeft(tx *sql.Tx, submodelDatabaseID int, parentPath string, child listChildToCompact) error {
	newPosition := child.oldPosition - 1
	newPath := parentPath + "[" + strconv.Itoa(newPosition) + "]"

	if err := updateListChildPath(tx, submodelDatabaseID, child.oldPath, newPath); err != nil {
		return err
	}
	return updateListChildPosition(tx, submodelDatabaseID, child.id, newPosition)
}

func escapeSQLLikePattern(value string) string {
	escaped := strings.ReplaceAll(value, "!", "!!")
	escaped = strings.ReplaceAll(escaped, "%", "!%")
	escaped = strings.ReplaceAll(escaped, "_", "!_")
	return escaped
}

func idShortPathLikeEscaped(idShortPath goqu.Expression, pattern string) goqu.Expression {
	return goqu.L("? LIKE ? ESCAPE '!'", idShortPath, pattern)
}

func updateListChildPath(tx *sql.Tx, submodelDatabaseID int, oldPath string, newPath string) error {
	dialect := goqu.Dialect("postgres")
	escapedOldPath := escapeSQLLikePattern(oldPath)
	query, args, err := dialect.Update("submodel_element").
		Set(goqu.Record{
			"idshort_path": goqu.L("? || SUBSTRING(idshort_path FROM ?)", newPath, len(oldPath)+1),
		}).
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.Or(
				goqu.C("idshort_path").Eq(oldPath),
				idShortPathLikeEscaped(goqu.C("idshort_path"), escapedOldPath+".%"),
				idShortPathLikeEscaped(goqu.C("idshort_path"), escapedOldPath+"[%"),
			),
		).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSMEBPATH-UPDATEPATH-TOSQL Failed to build update path query: " + err.Error())
	}

	if _, err = tx.Exec(query, args...); err != nil {
		return common.NewInternalServerError("SMREPO-DELSMEBPATH-UPDATEPATH-EXEC Failed to execute update path query: " + err.Error())
	}
	return nil
}

func updateListChildPosition(tx *sql.Tx, submodelDatabaseID int, childID int, newPosition int) error {
	dialect := goqu.Dialect("postgres")
	query, args, err := dialect.Update("submodel_element").
		Set(goqu.Record{"position": newPosition}).
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("id").Eq(childID),
		).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSMEBPATH-UPDATEPOSITION-TOSQL Failed to build update position query: " + err.Error())
	}

	if _, err = tx.Exec(query, args...); err != nil {
		return common.NewInternalServerError("SMREPO-DELSMEBPATH-UPDATEPOSITION-EXEC Failed to execute update position query: " + err.Error())
	}
	return nil
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

	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(localTx, submodelId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return common.NewErrNotFound("SMREPO-DELALLCHILDREN-SMNOTFOUND Submodel with ID '" + submodelId + "' not found")
		}
		return common.NewInternalServerError("SMREPO-DELALLCHILDREN-GETSMDATABASEID Failed to resolve Submodel database ID: " + err.Error())
	}

	escapedPath := escapeSQLLikePattern(idShortPath)

	del := goqu.Delete("submodel_element").Where(
		goqu.And(
			goqu.I("submodel_id").Eq(submodelDatabaseID),
			goqu.Or(
				idShortPathLikeEscaped(goqu.I("idshort_path"), escapedPath+".%"),
				idShortPathLikeEscaped(goqu.I("idshort_path"), escapedPath+"[%"),
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

// InsertSubmodelElements inserts submodel elements with maximum throughput while preserving
// data-equivalent persistence semantics of BatchInsert.
//
// It flattens the full element tree, inserts base records depth-wise in large batches, then
// performs bulk inserts for payloads and type-specific records.
//
//nolint:revive // cognitive-complexity is acceptable for performance-focused persistence orchestration
func InsertSubmodelElements(db *sql.DB, submodelID string, elements []types.ISubmodelElement, tx *sql.Tx, ctx *BatchInsertContext) ([]int, error) {
	return insertSubmodelElements(db, elements, tx, ctx, func(localTx *sql.Tx) (int, error) {
		submodelDatabaseID, submodelDatabaseIDErr := persistenceutils.GetSubmodelDatabaseID(localTx, submodelID)
		if submodelDatabaseIDErr != nil {
			return 0, common.NewInternalServerError("SMREPO-INSSME-GETSMDATABASEID " + submodelDatabaseIDErr.Error())
		}
		return submodelDatabaseID, nil
	})
}

// InsertSubmodelElementsForSubmodelDatabaseID inserts submodel elements when the caller already knows the submodel database ID.
func InsertSubmodelElementsForSubmodelDatabaseID(db *sql.DB, submodelDatabaseID int, elements []types.ISubmodelElement, tx *sql.Tx, ctx *BatchInsertContext) ([]int, error) {
	return insertSubmodelElements(db, elements, tx, ctx, func(_ *sql.Tx) (int, error) {
		if submodelDatabaseID <= 0 {
			return 0, common.NewInternalServerError("SMREPO-INSSME-SMDATABASEIDINVALID Submodel database ID must be positive")
		}
		return submodelDatabaseID, nil
	})
}

func insertSubmodelElements(db *sql.DB, elements []types.ISubmodelElement, tx *sql.Tx, ctx *BatchInsertContext, resolveSubmodelDatabaseID func(*sql.Tx) (int, error)) ([]int, error) {
	// Handle empty elements slice
	if len(elements) == 0 {
		return []int{}, nil
	}

	ctx = normalizeBatchInsertContext(ctx)

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

	submodelDatabaseID, submodelDatabaseIDErr := resolveSubmodelDatabaseID(localTx)
	if submodelDatabaseIDErr != nil {
		err = submodelDatabaseIDErr
		return nil, err
	}

	nodes, rootNodeIndexes, flattenErr := flattenSubmodelElementsForInsert(db, elements, ctx)
	if flattenErr != nil {
		err = flattenErr
		return nil, err
	}

	insertBaseErr := insertBaseNodesDepthWise(localTx, dialect, int64(submodelDatabaseID), nodes)
	if insertBaseErr != nil {
		err = insertBaseErr
		return nil, err
	}

	payloadErr := insertPayloadAndSemanticReferences(localTx, dialect, nodes, jsonLib)
	if payloadErr != nil {
		err = payloadErr
		return nil, err
	}

	typeRowsErr := insertTypeSpecificRows(localTx, dialect, nodes)
	if typeRowsErr != nil {
		err = typeRowsErr
		return nil, err
	}

	mlpErr := insertMultiLanguagePropertyValues(localTx, dialect, nodes)
	if mlpErr != nil {
		err = mlpErr
		return nil, err
	}

	mlpPayloadErr := insertMultiLanguagePropertyPayloadRows(localTx, dialect, nodes)
	if mlpPayloadErr != nil {
		err = mlpPayloadErr
		return nil, err
	}

	propertyPayloadErr := insertPropertyPayloadRows(localTx, dialect, nodes)
	if propertyPayloadErr != nil {
		err = propertyPayloadErr
		return nil, err
	}

	// Commit if we own the transaction
	if ownTransaction {
		if commitErr := localTx.Commit(); commitErr != nil {
			err = common.NewInternalServerError("SMREPO-INSSME-COMMITTX Failed to commit insert transaction: " + commitErr.Error())
			return nil, err
		}
	}

	ids := make([]int, 0, len(rootNodeIndexes))
	for _, rootIndex := range rootNodeIndexes {
		ids = append(ids, nodes[rootIndex].dbID)
	}

	return ids, nil
}
