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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package submodelelements

import (
	"database/sql"
	"strconv"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	jsoniter "github.com/json-iterator/go"
	"github.com/lib/pq"
)

// BatchInsertContext provides context for batch inserting submodel elements.
// It specifies where in the hierarchy the elements should be inserted.
type BatchInsertContext struct {
	ParentID      int    // Database ID of the parent element (0 for top-level elements)
	ParentPath    string // Path of the parent element (empty for top-level elements)
	RootSmeID     int    // Database ID of the root submodel element (0 for top-level elements, will be set to own ID)
	IsFromList    bool   // Whether elements are being inserted into a SubmodelElementList
	StartPosition int    // Starting position for elements (used when adding to existing containers)
}

const insertSubmodelElementsBatchSize = 1000

type flattenedInsertNode struct {
	element       types.ISubmodelElement
	handler       PostgreSQLSMECrudInterface
	position      int
	idShort       string
	idShortPath   string
	parentIndex   int
	parentDBID    int
	depth         int
	rootDBID      int
	rootNodeIndex int
	dbID          int
}

type pendingInsertNode struct {
	element       types.ISubmodelElement
	parentIndex   int
	parentDBID    int
	depth         int
	position      int
	parentPath    string
	isFromList    bool
	rootDBID      int
	rootNodeIndex int
}

func normalizeBatchInsertContext(ctx *BatchInsertContext) *BatchInsertContext {
	if ctx != nil {
		return ctx
	}

	return &BatchInsertContext{
		ParentID:      0,
		ParentPath:    "",
		RootSmeID:     0,
		IsFromList:    false,
		StartPosition: 0,
	}
}

func flattenSubmodelElementsForInsert(db *sql.DB, elements []types.ISubmodelElement, ctx *BatchInsertContext) ([]*flattenedInsertNode, []int, error) {
	pending := make([]pendingInsertNode, 0, len(elements))
	for i, element := range elements {
		pending = append(pending, pendingInsertNode{
			element:       element,
			parentIndex:   -1,
			parentDBID:    ctx.ParentID,
			depth:         0,
			position:      ctx.StartPosition + i,
			parentPath:    ctx.ParentPath,
			isFromList:    ctx.IsFromList,
			rootDBID:      ctx.RootSmeID,
			rootNodeIndex: -1,
		})
	}

	nodes := make([]*flattenedInsertNode, 0, len(elements))
	rootNodeIndexes := make([]int, 0, len(elements))

	for cursor := 0; cursor < len(pending); cursor++ {
		item := pending[cursor]
		handler, handlerErr := GetSMEHandler(item.element, db)
		if handlerErr != nil {
			return nil, nil, handlerErr
		}

		idShort := ""
		if item.element.IDShort() != nil {
			idShort = *item.element.IDShort()
		}

		idShortPath := buildIDShortPath(item.parentPath, item.isFromList, item.position, idShort)

		node := &flattenedInsertNode{
			element:       item.element,
			handler:       handler,
			position:      item.position,
			idShort:       idShort,
			idShortPath:   idShortPath,
			parentIndex:   item.parentIndex,
			parentDBID:    item.parentDBID,
			depth:         item.depth,
			rootDBID:      item.rootDBID,
			rootNodeIndex: item.rootNodeIndex,
			dbID:          0,
		}

		currentIndex := len(nodes)
		if node.parentIndex == -1 {
			rootNodeIndexes = append(rootNodeIndexes, currentIndex)
			if node.rootDBID == 0 {
				node.rootNodeIndex = currentIndex
			}
		}

		nodes = append(nodes, node)

		children := getChildElements(item.element)
		if len(children) == 0 {
			continue
		}

		childrenFromList := item.element.ModelType() == types.ModelTypeSubmodelElementList
		for childPosition, child := range children {
			pending = append(pending, pendingInsertNode{
				element:       child,
				parentIndex:   currentIndex,
				parentDBID:    0,
				depth:         item.depth + 1,
				position:      childPosition,
				parentPath:    idShortPath,
				isFromList:    childrenFromList,
				rootDBID:      node.rootDBID,
				rootNodeIndex: node.rootNodeIndex,
			})
		}
	}

	return nodes, rootNodeIndexes, nil
}

func buildIDShortPath(parentPath string, isFromList bool, position int, idShort string) string {
	if parentPath == "" {
		if isFromList {
			return "[" + strconv.Itoa(position) + "]"
		}
		return idShort
	}

	if isFromList {
		return parentPath + "[" + strconv.Itoa(position) + "]"
	}

	return parentPath + "." + idShort
}

func insertBaseNodesDepthWise(tx *sql.Tx, dialect goqu.DialectWrapper, submodelDatabaseID int64, nodes []*flattenedInsertNode) error {
	if len(nodes) == 0 {
		return nil
	}

	baseRows := make([]goqu.Record, 0, len(nodes))
	for _, node := range nodes {
		params := baseRecordParams{
			SubmodelID:  submodelDatabaseID,
			Element:     node.element,
			IDShort:     node.idShort,
			IDShortPath: node.idShortPath,
			Position:    node.position,
			ParentID:    0,
			RootSmeID:   node.rootDBID,
		}
		record, recordErr := buildBaseSubmodelElementRecord(params)
		if recordErr != nil {
			return recordErr
		}

		baseRows = append(baseRows, record)
	}

	insertedIDs, insertErr := insertRecordsReturningIDsChunked(
		tx,
		dialect,
		"submodel_element",
		[]string{"submodel_id", "parent_sme_id", "position", "id_short", "category", "model_type", "idshort_path", "root_sme_id"},
		baseRows,
	)
	if insertErr != nil {
		return common.NewInternalServerError("SMREPO-INSSME-INSBASE-EXECQ " + insertErr.Error())
	}

	for idx := range nodes {
		nodes[idx].dbID = insertedIDs[idx]
	}

	return updateHierarchyReferencesChunked(tx, dialect, nodes)
}

func updateHierarchyReferencesChunked(tx *sql.Tx, dialect goqu.DialectWrapper, nodes []*flattenedInsertNode) error {
	if len(nodes) == 0 {
		return nil
	}

	for start := 0; start < len(nodes); start += insertSubmodelElementsBatchSize {
		end := start + insertSubmodelElementsBatchSize
		if end > len(nodes) {
			end = len(nodes)
		}

		chunk := nodes[start:end]
		updateRows := make([]goqu.Record, 0, len(chunk))
		for idx := range chunk {
			node := chunk[idx]

			var parentID interface{}
			switch {
			case node.parentIndex >= 0:
				resolvedParentID := nodes[node.parentIndex].dbID
				if resolvedParentID == 0 {
					return common.NewInternalServerError("SMREPO-INSSME-UPDHIER-MISSINGPARENT Parent SME ID missing for path " + node.idShortPath)
				}
				parentID = resolvedParentID
			case node.parentDBID > 0:
				parentID = node.parentDBID
			default:
				parentID = nil
			}

			resolvedRootID := node.rootDBID
			if resolvedRootID == 0 {
				if node.rootNodeIndex >= 0 {
					resolvedRootID = nodes[node.rootNodeIndex].dbID
				} else {
					resolvedRootID = node.dbID
				}
			}
			if resolvedRootID == 0 {
				return common.NewInternalServerError("SMREPO-INSSME-UPDHIER-MISSINGROOT Root SME ID missing for path " + node.idShortPath)
			}

			updateRows = append(updateRows, goqu.Record{
				"id":            node.dbID,
				"parent_sme_id": parentID,
				"root_sme_id":   resolvedRootID,
			})
		}

		if len(updateRows) == 0 {
			continue
		}

		updatePayload, marshalErr := jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(updateRows)
		if marshalErr != nil {
			return common.NewInternalServerError("SMREPO-INSSME-UPDHIER-MARSHALPAYLOAD " + marshalErr.Error())
		}

		updateQuery, updateArgs, buildErr := dialect.Update(goqu.T("submodel_element").As("sme")).
			Set(goqu.Record{
				"parent_sme_id": goqu.I("v.parent_sme_id"),
				"root_sme_id":   goqu.I("v.root_sme_id"),
			}).
			From(goqu.L("jsonb_to_recordset(?::jsonb) AS v(id bigint, parent_sme_id bigint, root_sme_id bigint)", string(updatePayload))).
			Where(goqu.I("sme.id").Eq(goqu.I("v.id"))).
			ToSQL()
		if buildErr != nil {
			return common.NewInternalServerError("SMREPO-INSSME-UPDHIER-BUILDQ " + buildErr.Error())
		}

		if _, execErr := tx.Exec(updateQuery, updateArgs...); execErr != nil {
			if mappedErr := mapConflictInsertError(execErr); mappedErr != nil {
				return mappedErr
			}
			return common.NewInternalServerError("SMREPO-INSSME-UPDHIER-EXECQ " + execErr.Error())
		}
	}

	return nil
}

func insertPayloadAndSemanticReferences(tx *sql.Tx, dialect goqu.DialectWrapper, nodes []*flattenedInsertNode, jsonLib jsoniter.API) error {
	payloadRows := make([]goqu.Record, 0, len(nodes))
	for _, node := range nodes {
		payloadRecord, payloadBuildErr := buildSubmodelElementPayloadRecord(int64(node.dbID), node.element, jsonLib)
		if payloadBuildErr != nil {
			return payloadBuildErr
		}
		payloadRows = append(payloadRows, payloadRecord)
	}

	if len(payloadRows) > 0 {
		if payloadInsertErr := executeRecordInsertChunked(
			tx,
			dialect,
			"submodel_element_payload",
			[]string{"submodel_element_id", "description_payload", "displayname_payload", "administrative_information_payload", "embedded_data_specification_payload", "supplemental_semantic_ids_payload", "extensions_payload", "qualifiers_payload"},
			payloadRows,
			"SMREPO-INSSME-INSPAYLOAD",
		); payloadInsertErr != nil {
			return payloadInsertErr
		}
	}

	return insertSemanticReferencesBulk(tx, dialect, nodes)
}

func insertSemanticReferencesBulk(tx *sql.Tx, dialect goqu.DialectWrapper, nodes []*flattenedInsertNode) error {
	referenceRows := make([]goqu.Record, 0, len(nodes))
	payloadRows := make([]goqu.Record, 0, len(nodes))
	keyRows := make([]goqu.Record, 0)

	for _, node := range nodes {
		semanticReference := node.element.SemanticID()
		if semanticReference == nil {
			continue
		}

		referencePayload, payloadErr := getReferenceAsJSON(semanticReference)
		if payloadErr != nil {
			return common.NewInternalServerError("SMREPO-INSSME-INSSEMREF-BUILDPAYLOAD " + payloadErr.Error())
		}
		if !referencePayload.Valid {
			return common.NewInternalServerError("SMREPO-INSSME-INSSEMREF-INVALIDPAYLOAD Invalid semantic reference payload")
		}

		referenceRows = append(referenceRows, goqu.Record{
			"id":   node.dbID,
			"type": int(semanticReference.Type()),
		})

		payloadRows = append(payloadRows, goqu.Record{
			"reference_id":             node.dbID,
			"parent_reference_payload": goqu.L("?::jsonb", referencePayload.String),
		})

		for keyPosition, key := range semanticReference.Keys() {
			keyRows = append(keyRows, goqu.Record{
				"reference_id": node.dbID,
				"position":     keyPosition,
				"type":         int(key.Type()),
				"value":        key.Value(),
			})
		}
	}

	if len(referenceRows) == 0 {
		return nil
	}

	if err := executeRecordInsertChunked(
		tx,
		dialect,
		"submodel_element_semantic_id_reference",
		[]string{"id", "type"},
		referenceRows,
		"SMREPO-INSSME-INSSEMREF-REF",
	); err != nil {
		return err
	}

	if err := executeRecordInsertChunked(
		tx,
		dialect,
		"submodel_element_semantic_id_reference_payload",
		[]string{"reference_id", "parent_reference_payload"},
		payloadRows,
		"SMREPO-INSSME-INSSEMREF-PAYLOAD",
	); err != nil {
		return err
	}

	if len(keyRows) == 0 {
		return nil
	}

	return executeRecordInsertChunked(
		tx,
		dialect,
		"submodel_element_semantic_id_reference_key",
		[]string{"reference_id", "position", "type", "value"},
		keyRows,
		"SMREPO-INSSME-INSSEMREF-KEY",
	)
}

func insertTypeSpecificRows(tx *sql.Tx, dialect goqu.DialectWrapper, nodes []*flattenedInsertNode) error {
	typeSpecificRows := make(map[string][]goqu.Record)

	for _, node := range nodes {
		queryPart, partErr := node.handler.GetInsertQueryPart(tx, node.dbID, node.element)
		if partErr != nil {
			return partErr
		}
		if queryPart != nil {
			typeSpecificRows[queryPart.TableName] = append(typeSpecificRows[queryPart.TableName], queryPart.Record)
		}
	}

	for tableName, rows := range typeSpecificRows {
		if len(rows) == 0 {
			continue
		}
		if err := executeRecordInsertChunked(tx, dialect, tableName, nil, rows, "SMREPO-INSSME-INSTYPE"); err != nil {
			return err
		}
	}

	return nil
}

func insertMultiLanguagePropertyValues(tx *sql.Tx, dialect goqu.DialectWrapper, nodes []*flattenedInsertNode) error {
	mlpRows := make([]goqu.Record, 0)
	for _, node := range nodes {
		if node.element.ModelType() != types.ModelTypeMultiLanguageProperty {
			continue
		}

		mlp, ok := node.element.(*types.MultiLanguageProperty)
		if !ok || len(mlp.Value()) == 0 {
			continue
		}

		for _, val := range mlp.Value() {
			mlpRows = append(mlpRows, goqu.Record{
				"mlp_id":   node.dbID,
				"language": val.Language(),
				"text":     val.Text(),
			})
		}
	}

	if len(mlpRows) == 0 {
		return nil
	}

	return executeRecordInsertChunked(
		tx,
		dialect,
		"multilanguage_property_value",
		[]string{"mlp_id", "language", "text"},
		mlpRows,
		"SMREPO-INSSME-INSMLPVAL",
	)
}

func insertPropertyPayloadRows(tx *sql.Tx, dialect goqu.DialectWrapper, nodes []*flattenedInsertNode) error {
	propertyPayloadRows := make([]goqu.Record, 0)
	for _, node := range nodes {
		if node.element.ModelType() != types.ModelTypeProperty {
			continue
		}

		property, ok := node.element.(*types.Property)
		if !ok {
			continue
		}

		valueIDPayload := "[]"
		if property.ValueID() != nil && !isEmptyReference(property.ValueID()) {
			valueIDJSONString, serErr := serializeIClassSliceToJSON([]types.IClass{property.ValueID()}, "SMREPO-INSSME-PROP-VALREF")
			if serErr != nil {
				return serErr
			}
			valueIDPayload = valueIDJSONString
		}

		propertyPayloadRows = append(propertyPayloadRows, goqu.Record{
			"property_element_id": node.dbID,
			"value_id_payload":    goqu.L("?::jsonb", valueIDPayload),
		})
	}

	if len(propertyPayloadRows) == 0 {
		return nil
	}

	return executeRecordInsertChunked(
		tx,
		dialect,
		"property_element_payload",
		[]string{"property_element_id", "value_id_payload"},
		propertyPayloadRows,
		"SMREPO-INSSME-INSPROPPAYLOAD",
	)
}

func insertRecordsReturningIDsChunked(tx *sql.Tx, dialect goqu.DialectWrapper, tableName string, cols []string, rows []goqu.Record) ([]int, error) {
	if len(rows) == 0 {
		return []int{}, nil
	}

	insertedIDs := make([]int, 0, len(rows))
	for start := 0; start < len(rows); start += insertSubmodelElementsBatchSize {
		end := start + insertSubmodelElementsBatchSize
		if end > len(rows) {
			end = len(rows)
		}

		chunk := rows[start:end]
		chunkRows := make([]interface{}, len(chunk))
		for i, row := range chunk {
			chunkRows[i] = row
		}

		insert := dialect.Insert(tableName)
		if len(cols) > 0 {
			insertCols := make([]interface{}, 0, len(cols))
			for _, col := range cols {
				insertCols = append(insertCols, col)
			}
			insert = insert.Cols(insertCols...)
		}
		insert = insert.Rows(chunkRows...).Returning("id")
		sqlQuery, args, buildErr := insert.ToSQL()
		if buildErr != nil {
			return nil, buildErr
		}

		resultRows, execErr := tx.Query(sqlQuery, args...)
		if execErr != nil {
			if mappedErr := mapConflictInsertError(execErr); mappedErr != nil {
				return nil, mappedErr
			}
			return nil, execErr
		}

		chunkIDs := make([]int, 0, len(chunk))
		for resultRows.Next() {
			var id int
			if scanErr := resultRows.Scan(&id); scanErr != nil {
				_ = resultRows.Close()
				return nil, scanErr
			}
			chunkIDs = append(chunkIDs, id)
		}
		if closeErr := resultRows.Close(); closeErr != nil {
			return nil, closeErr
		}
		if rowErr := resultRows.Err(); rowErr != nil {
			return nil, rowErr
		}

		if len(chunkIDs) != len(chunk) {
			return nil, common.NewInternalServerError("SMREPO-INSSME-INSBASE-IDMISMATCH returned IDs count does not match input rows")
		}

		insertedIDs = append(insertedIDs, chunkIDs...)
	}

	if len(insertedIDs) != len(rows) {
		return nil, common.NewInternalServerError("SMREPO-INSSME-INSBASE-IDCOUNTMISMATCH returned IDs count does not match all rows")
	}

	return insertedIDs, nil
}

func executeRecordInsertChunked(tx *sql.Tx, dialect goqu.DialectWrapper, tableName string, cols []string, rows []goqu.Record, errCode string) error {
	if len(rows) == 0 {
		return nil
	}

	for start := 0; start < len(rows); start += insertSubmodelElementsBatchSize {
		end := start + insertSubmodelElementsBatchSize
		if end > len(rows) {
			end = len(rows)
		}

		chunk := rows[start:end]
		chunkRows := make([]interface{}, len(chunk))
		for i, row := range chunk {
			chunkRows[i] = row
		}

		insert := dialect.Insert(tableName)
		if len(cols) > 0 {
			insertCols := make([]interface{}, 0, len(cols))
			for _, col := range cols {
				insertCols = append(insertCols, col)
			}
			insert = insert.Cols(insertCols...)
		}
		insert = insert.Rows(chunkRows...)

		sqlQuery, args, buildErr := insert.ToSQL()
		if buildErr != nil {
			return common.NewInternalServerError(errCode + "-BUILDQ " + buildErr.Error())
		}
		if _, execErr := tx.Exec(sqlQuery, args...); execErr != nil {
			if mappedErr := mapConflictInsertError(execErr); mappedErr != nil {
				return mappedErr
			}
			return common.NewInternalServerError(errCode + "-EXECQ " + execErr.Error())
		}
	}

	return nil
}

func mapConflictInsertError(err error) error {
	if err == nil {
		return nil
	}

	pqErr, ok := err.(*pq.Error)
	if !ok {
		return nil
	}

	if pqErr.Code == "23505" {
		return common.NewErrConflict("SMREPO-INSSME-CONFLICT Duplicate submodel element")
	}

	return nil
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
	SubmodelID  int64
	Element     types.ISubmodelElement
	IDShort     string
	IDShortPath string
	Position    int
	ParentID    int
	RootSmeID   int
}

// buildBaseSubmodelElementRecord builds the base submodel_element record using pre-computed reference IDs.
func buildBaseSubmodelElementRecord(params baseRecordParams) (goqu.Record, error) {
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
		"submodel_id":   params.SubmodelID,
		"parent_sme_id": parentDBId,
		"position":      params.Position,
		"id_short":      params.IDShort,
		"category":      params.Element.Category(),
		"model_type":    params.Element.ModelType(),
		"idshort_path":  params.IDShortPath,
		"root_sme_id":   rootDbID,
	}, nil
}

func buildSubmodelElementPayloadRecord(submodelElementID int64, element types.ISubmodelElement, jsonLib jsoniter.API) (goqu.Record, error) {
	_ = jsonLib

	descriptionPayload, err := serializeIClassSliceToJSON(toIClassSlice(element.Description()), "SMREPO-SMEBATCH-PAYLOAD-DESC")
	if err != nil {
		return nil, err
	}

	displayNamePayload, err := serializeIClassSliceToJSON(toIClassSlice(element.DisplayName()), "SMREPO-SMEBATCH-PAYLOAD-DISPNAME")
	if err != nil {
		return nil, err
	}

	administrativeInformationPayload := "[]"

	embeddedDataSpecificationPayload, err := serializeIClassSliceToJSON(toIClassSlice(element.EmbeddedDataSpecifications()), "SMREPO-SMEBATCH-PAYLOAD-EDS")
	if err != nil {
		return nil, err
	}

	supplementalSemanticIDsPayload, err := serializeIClassSliceToJSON(toIClassSlice(element.SupplementalSemanticIDs()), "SMREPO-SMEBATCH-PAYLOAD-SUPPLSEM")
	if err != nil {
		return nil, err
	}

	extensionsPayload, err := serializeIClassSliceToJSON(toIClassSlice(element.Extensions()), "SMREPO-SMEBATCH-PAYLOAD-EXT")
	if err != nil {
		return nil, err
	}

	qualifiersPayload, err := serializeIClassSliceToJSON(toIClassSlice(element.Qualifiers()), "SMREPO-SMEBATCH-PAYLOAD-QUAL")
	if err != nil {
		return nil, err
	}

	return goqu.Record{
		"submodel_element_id":                 submodelElementID,
		"description_payload":                 descriptionPayload,
		"displayname_payload":                 displayNamePayload,
		"administrative_information_payload":  administrativeInformationPayload,
		"embedded_data_specification_payload": embeddedDataSpecificationPayload,
		"supplemental_semantic_ids_payload":   supplementalSemanticIDsPayload,
		"extensions_payload":                  extensionsPayload,
		"qualifiers_payload":                  qualifiersPayload,
	}, nil
}
