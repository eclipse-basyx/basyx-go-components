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
// Author: Jannik Fried (Fraunhofer IESE)

package submodelelements

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	jsoniter "github.com/json-iterator/go"
)

// ReconciliationReferenceKey is one normalized reference key used by the
// single-statement Submodel reconciler.
type ReconciliationReferenceKey struct {
	Position int    `json:"position"`
	Type     int    `json:"type"`
	Value    string `json:"value"`
}

// ReconciliationReference is a database-ready reference representation.
type ReconciliationReference struct {
	Position int                          `json:"position"`
	Type     int                          `json:"type"`
	Payload  json.RawMessage              `json:"payload"`
	Keys     []ReconciliationReferenceKey `json:"keys"`
}

// ReconciliationLanguageValue is one MultiLanguageProperty value row.
type ReconciliationLanguageValue struct {
	Language string `json:"language"`
	Text     string `json:"text"`
}

// ReconciliationElementPayload contains the JSONB payload columns shared by
// all SubmodelElement types.
type ReconciliationElementPayload struct {
	Description                json.RawMessage `json:"description"`
	DisplayName                json.RawMessage `json:"displayName"`
	EmbeddedDataSpecifications json.RawMessage `json:"embeddedDataSpecifications"`
	SupplementalSemanticIDs    json.RawMessage `json:"supplementalSemanticIds"`
	Extensions                 json.RawMessage `json:"extensions"`
	Qualifiers                 json.RawMessage `json:"qualifiers"`
}

// ReconciliationElementChanges selects the normalized persistence sections
// that the reconciliation statement is allowed to mutate.
type ReconciliationElementChanges struct {
	Core           bool `json:"core"`
	Payload        bool `json:"payload"`
	SemanticID     bool `json:"semanticId"`
	SupplementalID bool `json:"supplementalId"`
	TypeData       bool `json:"typeData"`
	LanguageValues bool `json:"languageValues"`
	ValueID        bool `json:"valueId"`
}

// AllReconciliationElementChanges returns the mutation mask for a newly
// inserted element.
func AllReconciliationElementChanges() ReconciliationElementChanges {
	return ReconciliationElementChanges{
		Core:           true,
		Payload:        true,
		SemanticID:     true,
		SupplementalID: true,
		TypeData:       true,
		LanguageValues: true,
		ValueID:        true,
	}
}

// ReconciliationElementRow is a normalized, side-effect-free persistence row.
// Existing insert conversion code is reused to populate TypeData.
type ReconciliationElementRow struct {
	Path                    string                        `json:"path"`
	ParentPath              string                        `json:"parentPath"`
	RootPath                string                        `json:"rootPath"`
	Position                int                           `json:"position"`
	Depth                   int                           `json:"depth"`
	IDShort                 string                        `json:"idShort"`
	Category                *string                       `json:"category"`
	ModelType               int                           `json:"modelType"`
	Payload                 ReconciliationElementPayload  `json:"payload"`
	SemanticID              *ReconciliationReference      `json:"semanticId"`
	SupplementalSemanticIDs []ReconciliationReference     `json:"supplementalSemanticIds"`
	TypeTable               string                        `json:"typeTable"`
	TypeData                map[string]any                `json:"typeData"`
	LanguageValues          []ReconciliationLanguageValue `json:"languageValues"`
	ValueID                 json.RawMessage               `json:"valueId"`
	Changes                 ReconciliationElementChanges  `json:"changes"`
}

// BuildReconciliationElementRows flattens an AAS element tree and converts it
// into database-ready rows without executing SQL.
func BuildReconciliationElementRows(db *sql.DB, elements []types.ISubmodelElement) ([]ReconciliationElementRow, error) {
	ctx := normalizeBatchInsertContext(nil)
	nodes, _, err := flattenSubmodelElementsForInsert(db, elements, ctx)
	if err != nil {
		return nil, err
	}

	rows := make([]ReconciliationElementRow, 0, len(nodes))
	for _, node := range nodes {
		row, buildErr := buildReconciliationElementRow(node)
		if buildErr != nil {
			return nil, buildErr
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func buildReconciliationElementRow(node *flattenedInsertNode) (ReconciliationElementRow, error) {
	payload, err := reconciliationElementPayload(node.element)
	if err != nil {
		return ReconciliationElementRow{}, err
	}
	semanticID, err := reconciliationReference(node.element.SemanticID(), 0, true)
	if err != nil {
		return ReconciliationElementRow{}, err
	}
	supplemental, err := reconciliationReferences(node.element.SupplementalSemanticIDs())
	if err != nil {
		return ReconciliationElementRow{}, err
	}
	typeTable, typeData, err := reconciliationTypeData(node)
	if err != nil {
		return ReconciliationElementRow{}, err
	}
	languageValues, valueID, err := reconciliationValueRows(node.element)
	if err != nil {
		return ReconciliationElementRow{}, err
	}

	parentPath := ""
	if node.parentIndex >= 0 {
		parentPath = nodePathFromIndex(node, node.parentIndex)
	}
	rootPath := node.idShortPath
	if node.rootNodeIndex >= 0 {
		rootPath = nodePathFromIndex(node, node.rootNodeIndex)
	}

	return ReconciliationElementRow{
		Path:                    node.idShortPath,
		ParentPath:              parentPath,
		RootPath:                rootPath,
		Position:                node.position,
		Depth:                   node.depth,
		IDShort:                 node.idShort,
		Category:                node.element.Category(),
		ModelType:               int(node.element.ModelType()),
		Payload:                 payload,
		SemanticID:              semanticID,
		SupplementalSemanticIDs: supplemental,
		TypeTable:               typeTable,
		TypeData:                typeData,
		LanguageValues:          languageValues,
		ValueID:                 valueID,
	}, nil
}

func nodePathFromIndex(node *flattenedInsertNode, index int) string {
	// flattenSubmodelElementsForInsert stores nodes breadth-first. Parent and
	// root indices therefore always point to nodes already materialized in the
	// same backing slice. The paths are also derivable from the current path.
	if index == node.parentIndex {
		return parentPath(node.idShortPath)
	}
	return rootPath(node.idShortPath)
}

func parentPath(path string) string {
	for index := len(path) - 1; index >= 0; index-- {
		switch path[index] {
		case '.':
			return path[:index]
		case '[':
			return path[:index]
		}
	}
	return ""
}

func rootPath(path string) string {
	for index, char := range path {
		if char == '.' || char == '[' {
			return path[:index]
		}
	}
	return path
}

func reconciliationElementPayload(element types.ISubmodelElement) (ReconciliationElementPayload, error) {
	record, _, err := buildSubmodelElementPayloadRecord(0, element, jsoniter.ConfigCompatibleWithStandardLibrary)
	if err != nil {
		return ReconciliationElementPayload{}, err
	}
	if record == nil {
		record = goqu.Record{}
	}
	return ReconciliationElementPayload{
		Description:                rawJSONRecordValue(record, "description_payload", "[]"),
		DisplayName:                rawJSONRecordValue(record, "displayname_payload", "[]"),
		EmbeddedDataSpecifications: rawJSONRecordValue(record, "embedded_data_specification_payload", "[]"),
		SupplementalSemanticIDs:    rawJSONRecordValue(record, "supplemental_semantic_ids_payload", "[]"),
		Extensions:                 rawJSONRecordValue(record, "extensions_payload", "[]"),
		Qualifiers:                 rawJSONRecordValue(record, "qualifiers_payload", "[]"),
	}, nil
}

func rawJSONRecordValue(record goqu.Record, key string, fallback string) json.RawMessage {
	value, ok := record[key]
	if !ok || value == nil {
		return json.RawMessage(fallback)
	}
	if text, ok := value.(string); ok && json.Valid([]byte(text)) {
		return json.RawMessage(text)
	}
	return json.RawMessage(fallback)
}

func reconciliationReference(reference types.IReference, position int, fullPayload bool) (*ReconciliationReference, error) {
	if reference == nil || isEmptyReference(reference) {
		return nil, nil
	}
	var payload json.RawMessage
	var err error
	if fullPayload {
		payloadString, payloadErr := getReferenceAsJSON(reference)
		if payloadErr != nil {
			return nil, payloadErr
		}
		payload = json.RawMessage(payloadString.String)
	} else {
		payload, err = common.BuildReferencePayload(reference.ReferredSemanticID())
		if err != nil {
			return nil, err
		}
	}
	keys := make([]ReconciliationReferenceKey, 0, len(reference.Keys()))
	for keyPosition, key := range reference.Keys() {
		keys = append(keys, ReconciliationReferenceKey{
			Position: keyPosition,
			Type:     int(key.Type()),
			Value:    key.Value(),
		})
	}
	return &ReconciliationReference{
		Position: position,
		Type:     int(reference.Type()),
		Payload:  payload,
		Keys:     keys,
	}, nil
}

func reconciliationReferences(references []types.IReference) ([]ReconciliationReference, error) {
	result := make([]ReconciliationReference, 0, len(references))
	for position, reference := range references {
		normalized, err := reconciliationReference(reference, position, false)
		if err != nil {
			return nil, err
		}
		if normalized != nil {
			result = append(result, *normalized)
		}
	}
	return result, nil
}

// BuildReconciliationReference converts one reference into the normalized
// representation used by the Submodel reconciliation statement.
func BuildReconciliationReference(reference types.IReference, fullPayload bool) (*ReconciliationReference, error) {
	return reconciliationReference(reference, 0, fullPayload)
}

// BuildReconciliationReferences converts an ordered reference collection into
// normalized reconciliation rows.
func BuildReconciliationReferences(references []types.IReference) ([]ReconciliationReference, error) {
	return reconciliationReferences(references)
}

func reconciliationTypeData(node *flattenedInsertNode) (string, map[string]any, error) {
	part, err := node.handler.GetInsertQueryPart(nil, 0, node.element)
	if err != nil {
		return "", nil, err
	}
	if part == nil {
		return "", map[string]any{}, nil
	}
	result := make(map[string]any, len(part.Record))
	for key, value := range part.Record {
		if key == "id" {
			continue
		}
		result[key] = normalizedReconciliationRecordValue(key, value)
	}
	return part.TableName, result, nil
}

func normalizedReconciliationRecordValue(column string, value any) any {
	normalized := reconciliationRecordValue(value)
	if !strings.HasSuffix(column, "_datetime") {
		return normalized
	}
	text, ok := normalized.(string)
	if !ok {
		return normalized
	}
	parsed, err := common.ParseISO8601DateTime(text)
	if err != nil {
		return normalized
	}
	return parsed.UTC().Round(time.Microsecond).Format(time.RFC3339Nano)
}

func reconciliationRecordValue(value any) any {
	switch typed := value.(type) {
	case sql.NullString:
		if !typed.Valid {
			return nil
		}
		return typed.String
	case sql.NullInt64:
		if !typed.Valid {
			return nil
		}
		return typed.Int64
	case sql.NullBool:
		if !typed.Valid {
			return nil
		}
		return typed.Bool
	case sql.NullFloat64:
		if !typed.Valid {
			return nil
		}
		return typed.Float64
	}
	reflected := reflect.ValueOf(value)
	if reflected.IsValid() && reflected.Kind() == reflect.Pointer {
		if reflected.IsNil() {
			return nil
		}
		return reconciliationRecordValue(reflected.Elem().Interface())
	}
	return value
}

func reconciliationValueRows(element types.ISubmodelElement) ([]ReconciliationLanguageValue, json.RawMessage, error) {
	valueID := json.RawMessage("null")
	switch typed := element.(type) {
	case *types.MultiLanguageProperty:
		values := make([]ReconciliationLanguageValue, 0, len(typed.Value()))
		for _, value := range typed.Value() {
			values = append(values, ReconciliationLanguageValue{Language: value.Language(), Text: value.Text()})
		}
		serialized, err := reconciliationValueID(typed.ValueID())
		return values, serialized, err
	case *types.Property:
		serialized, err := reconciliationValueID(typed.ValueID())
		return nil, serialized, err
	default:
		return nil, valueID, nil
	}
}

func reconciliationValueID(reference types.IReference) (json.RawMessage, error) {
	if reference == nil || isEmptyReference(reference) {
		return json.RawMessage("null"), nil
	}
	serialized, err := serializeIClassSliceToJSON([]types.IClass{reference}, "SMREPO-RECON-VALID")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(serialized), nil
}
