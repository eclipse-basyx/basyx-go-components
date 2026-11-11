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

package submodelelements

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"

	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// MetadataTarget identifies the locked element addressed by a metadata PATCH.
type MetadataTarget struct {
	ID        int64
	ModelType types.ModelType
	IDShort   sql.NullString
}

// LockMetadataTargetTx locks the element before authorization and metadata updates.
func LockMetadataTargetTx(ctx context.Context, tx *sql.Tx, submodelDatabaseID int, path string) (MetadataTarget, error) {
	query, args, err := goqu.Dialect("postgres").From("submodel_element").
		Select("id", "model_type", "id_short").
		Where(goqu.Ex{"submodel_id": submodelDatabaseID, "idshort_path": path}).
		ForUpdate(goqu.Wait).Prepared(true).ToSQL()
	if err != nil {
		return MetadataTarget{}, common.NewInternalServerError("SMREPO-METATARGET-BUILDQ " + err.Error())
	}
	var target MetadataTarget
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&target.ID, &target.ModelType, &target.IDShort); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MetadataTarget{}, common.NewErrNotFound("SMREPO-METATARGET-NOTFOUND " + path)
		}
		return MetadataTarget{}, common.NewInternalServerError("SMREPO-METATARGET-EXECQ " + err.Error())
	}
	return target, nil
}

var metadataPayloadColumns = map[string]string{
	"description":                "description_payload",
	"displayName":                "displayname_payload",
	"embeddedDataSpecifications": "embedded_data_specification_payload",
	"supplementalSemanticIds":    "supplemental_semantic_ids_payload",
	"extensions":                 "extensions_payload",
	"qualifiers":                 "qualifiers_payload",
}

var metadataTypeColumns = map[types.ModelType]map[string]string{
	types.ModelTypeSubmodelElementList: {
		"orderRelevant": "order_relevant", "semanticIdListElement": "semantic_id_list_element",
		"typeValueListElement": "type_value_list_element", "valueTypeListElement": "value_type_list_element",
	},
	types.ModelTypeBasicEventElement: {
		"direction": "direction", "state": "state", "messageTopic": "message_topic",
		"messageBroker": "message_broker", "lastUpdate": "last_update",
		"minInterval": "min_interval", "maxInterval": "max_interval",
	},
}

type metadataReferenceColumn struct {
	table, key, column string
}

var metadataReferenceColumns = map[string]metadataReferenceColumn{
	"semanticId":            {"submodel_element_semantic_id_reference_payload", "reference_id", "parent_reference_payload"},
	"semanticIdListElement": {"submodel_element_list", "id", "semantic_id_list_element"},
	"messageBroker":         {"basic_event_element", "id", "message_broker"},
}

// ApplyMetadataPatchTx writes only fields present in a validated metadata patch.
// current is needed only when a Property or Range valueType changes.
func ApplyMetadataPatchTx(ctx context.Context, tx *sql.Tx, target MetadataTarget, patch map[string]any, current types.ISubmodelElement) error {
	if err := validateMetadataIDShorts(target, patch); err != nil {
		return err
	}
	patch, err := mergeMetadataReferencePatchesTx(ctx, tx, target.ID, patch)
	if err != nil {
		return err
	}
	metadata, err := common.ParseSubmodelElementMetadataPayload(patch)
	if err != nil {
		return common.NewErrBadRequest("SMREPO-METADATA-PAYLOAD invalid resulting metadata")
	}
	if metadata.ModelType != target.ModelType {
		return common.NewErrBadRequest("SMREPO-METADATA-MODELTYPEMISMATCH metadata modelType must match stored modelType")
	}
	return applyMetadataFieldsTx(ctx, tx, target, patch, metadata, current)
}

// validateMetadataIDShorts rejects an invalid idShort in the patch and a patch of an element
// whose stored idShort is invalid. A metadata PATCH never renames the element.
func validateMetadataIDShorts(target MetadataTarget, patch map[string]any) error {
	var requestedIDShort *string
	if idShort, present := patch["idShort"].(string); present {
		requestedIDShort = &idShort
	}
	return validateUpdatedIDShort("SMREPO-PATCHSMEMETA", requestedIDShort, target.IDShort, false)
}

func applyMetadataFieldsTx(ctx context.Context, tx *sql.Tx, target MetadataTarget, patch map[string]any, metadata gen.SubmodelElementMetadata, current types.ISubmodelElement) error {
	if category, present := patch["category"]; present {
		if err := execMetadataRecord(ctx, tx, "submodel_element", target.ID, goqu.Record{"category": category}); err != nil {
			return err
		}
	}
	if err := updateMetadataPayloadTx(ctx, tx, target.ID, patch); err != nil {
		return err
	}
	if err := updateMetadataReferencesTx(ctx, tx, target.ID, patch, metadata); err != nil {
		return err
	}
	table, record, err := buildMetadataTypeRecord(target.ModelType, patch, current)
	if err != nil {
		return err
	}
	return execMetadataRecord(ctx, tx, table, target.ID, record)
}

func mergeMetadataReferencePatchesTx(ctx context.Context, tx *sql.Tx, elementID int64, patch map[string]any) (map[string]any, error) {
	merged := maps.Clone(patch)
	for field, column := range metadataReferenceColumns {
		object, present := patch[field].(map[string]any)
		if !present {
			continue
		}
		previous, err := readMetadataReferenceTx(ctx, tx, elementID, column)
		if err != nil {
			return nil, err
		}
		merged[field] = common.MergeMetadataPatch(previous, object)
	}
	return merged, nil
}

func readMetadataReferenceTx(ctx context.Context, tx *sql.Tx, elementID int64, column metadataReferenceColumn) (map[string]any, error) {
	query, args, err := goqu.Dialect("postgres").From(column.table).
		Select(goqu.C(column.column)).Where(goqu.C(column.key).Eq(elementID)).Prepared(true).ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-METADATA-REFQUERY " + err.Error())
	}
	var payload sql.NullString
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, common.NewInternalServerError("SMREPO-METADATA-REFREAD " + err.Error())
	}
	var reference map[string]any
	if payload.Valid {
		if err = json.Unmarshal([]byte(payload.String), &reference); err != nil {
			return nil, common.NewInternalServerError("SMREPO-METADATA-REFJSON invalid stored reference")
		}
	}
	return reference, nil
}
func metadataJSONColumn(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, common.NewErrBadRequest("SMREPO-METADATA-JSON " + err.Error())
	}
	return goqu.L("?::jsonb", string(encoded)), nil
}

func updateMetadataPayloadTx(ctx context.Context, tx *sql.Tx, elementID int64, patch map[string]any) error {
	record := goqu.Record{}
	for field, column := range metadataPayloadColumns {
		value, present := patch[field]
		if !present {
			continue
		}
		if value == nil {
			value = []any{}
		}
		encoded, err := metadataJSONColumn(value)
		if err != nil {
			return err
		}
		record[column] = encoded
	}
	if len(record) == 0 {
		return nil
	}
	row := goqu.Record{"submodel_element_id": elementID}
	for column, value := range record {
		row[column] = value
	}
	query, args, err := goqu.Dialect("postgres").Insert("submodel_element_payload").
		Rows(row).OnConflict(goqu.DoUpdate("submodel_element_id", record)).
		Prepared(true).ToSQL()
	return execMetadataSQL(ctx, tx, query, args, err)
}

func updateMetadataReferencesTx(ctx context.Context, tx *sql.Tx, elementID int64, patch map[string]any, metadata gen.SubmodelElementMetadata) error {
	if _, present := patch["supplementalSemanticIds"]; present {
		if err := common.ReplaceContextReferences1ToMany(tx, elementID, metadata.SupplementalSemanticIds, common.TblSubmodelElementSuppSemantic, common.ColSubmodelElementID); err != nil {
			return common.NewInternalServerError("SMREPO-METADATA-SUPPSEM " + err.Error())
		}
	}
	if _, present := patch["semanticId"]; !present {
		return nil
	}
	if metadata.SemanticID != nil && !isEmptyReference(metadata.SemanticID) {
		if _, err := CreateContextReferenceByOwnerID(tx, elementID, "submodel_element_semantic_id", metadata.SemanticID); err != nil {
			return common.NewInternalServerError("SMREPO-METADATA-SEMANTIC " + err.Error())
		}
		return nil
	}
	query, args, err := goqu.Dialect("postgres").Delete("submodel_element_semantic_id_reference").
		Where(goqu.C("id").Eq(elementID)).Prepared(true).ToSQL()
	return execMetadataSQL(ctx, tx, query, args, err)
}

func buildMetadataTypeRecord(modelType types.ModelType, patch map[string]any, current types.ISubmodelElement) (string, goqu.Record, error) {
	if value, present := patch["valueType"]; present {
		return buildMetadataValueTypeRecord(current, value)
	}
	table := ""
	switch modelType {
	case types.ModelTypeSubmodelElementList:
		table = "submodel_element_list"
	case types.ModelTypeBasicEventElement:
		table = "basic_event_element"
	}
	record := goqu.Record{}
	for field, column := range metadataTypeColumns[modelType] {
		value, present := patch[field]
		if !present {
			continue
		}
		encoded, err := metadataTypeColumnValue(field, value)
		if err != nil {
			return "", nil, err
		}
		record[column] = encoded
	}
	return table, record, nil
}

func metadataTypeColumnValue(field string, value any) (any, error) {
	switch field {
	case "typeValueListElement":
		return metadataEnumValue(value, stringification.AASSubmodelElementsFromString)
	case "valueTypeListElement":
		return metadataEnumValue(value, stringification.DataTypeDefXSDFromString)
	case "direction":
		return metadataEnumValue(value, stringification.DirectionFromString)
	case "state":
		return metadataEnumValue(value, stringification.StateOfEventFromString)
	case "semanticIdListElement", "messageBroker":
		return metadataJSONColumn(value)
	case "lastUpdate", "minInterval", "maxInterval":
		if value == "" {
			return nil, nil
		}
		return value, nil
	default:
		return value, nil
	}
}

func metadataEnumValue[T any](value any, fromString func(string) (T, bool)) (any, error) {
	literal, ok := value.(string)
	if ok {
		if parsed, valid := fromString(literal); valid {
			return parsed, nil
		}
	}
	return nil, common.NewErrBadRequest("SMREPO-METADATA-ENUM invalid metadata enum")
}

func buildMetadataValueTypeRecord(current types.ISubmodelElement, value any) (string, goqu.Record, error) {
	parsed, err := metadataEnumValue(value, stringification.DataTypeDefXSDFromString)
	if err != nil {
		return "", nil, err
	}
	valueType := parsed.(types.DataTypeDefXSD)
	switch element := current.(type) {
	case *types.Property:
		element.SetValueType(valueType)
		record, buildErr := buildUpdatePropertyRecordObject(element, false, nil)
		if buildErr != nil {
			return "", nil, common.NewErrBadRequest("SMREPO-METADATA-PROPVALUETYPE stored value is incompatible with the requested valueType")
		}
		return "property_element", record, nil
	case *types.Range:
		element.SetValueType(valueType)
		record, buildErr := buildUpdateRangeRecordObject(element, false)
		if buildErr != nil {
			return "", nil, common.NewErrBadRequest("SMREPO-METADATA-RANGEVALUETYPE stored bounds are incompatible with the requested valueType")
		}
		return "range_element", record, nil
	default:
		return "", nil, common.NewInternalServerError("SMREPO-METADATA-VALUETYPE missing Property or Range state")
	}
}

func execMetadataRecord(ctx context.Context, tx *sql.Tx, table string, elementID int64, record goqu.Record) error {
	if len(record) == 0 {
		return nil
	}
	query, args, err := goqu.Dialect("postgres").Update(table).Set(record).
		Where(goqu.C("id").Eq(elementID)).Prepared(true).ToSQL()
	return execMetadataSQL(ctx, tx, query, args, err)
}

func execMetadataSQL(ctx context.Context, tx *sql.Tx, query string, args []any, buildErr error) error {
	if buildErr != nil {
		return common.NewInternalServerError("SMREPO-METADATA-BUILDQ " + buildErr.Error())
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("SMREPO-METADATA-EXECQ " + err.Error())
	}
	return nil
}
