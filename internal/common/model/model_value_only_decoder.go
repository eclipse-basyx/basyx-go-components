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

package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
)

// ErrValueOnlyUnknownElement reports a ValueOnly payload that addresses a submodel element which is not stored.
var ErrValueOnlyUnknownElement = errors.New("MODEL-VALONLY-DECODE-UNKNOWNELEMENT")

// ValueOnlySchemaNode describes a stored submodel element that a ValueOnly payload is decoded against.
// Children holds the named children of collections, entity statements and relationship annotations;
// Items holds the list elements indexed by position; a nil entry is a position that is not stored.
type ValueOnlySchemaNode struct {
	ElementID   int64
	IDShortPath string
	ModelType   types.ModelType
	ValueType   types.DataTypeDefXSD
	Children    map[string]*ValueOnlySchemaNode
	Items       []*ValueOnlySchemaNode
}

// ParseValueOnlyPayload validates that a request body holds exactly one JSON value and returns it undecoded.
// The payload is interpreted later against the stored schema of the addressed element.
func ParseValueOnlyPayload(body []byte) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(body)
	if !json.Valid(trimmed) {
		return nil, errors.New("MODEL-VALONLY-PARSE-JSON request body is not a single valid JSON value")
	}
	return json.RawMessage(trimmed), nil
}

// ParseSubmodelValueOnlyPayload validates a submodel ValueOnly request body and splits it into its top-level members.
func ParseSubmodelValueOnlyPayload(body []byte) (map[string]json.RawMessage, error) {
	raw, err := ParseValueOnlyPayload(body)
	if err != nil {
		return nil, err
	}
	return decodeValueOnlyObject(raw)
}

// DecodeValueOnly decodes the ValueOnly payload of a single submodel element against its stored schema.
// Stored model types and value types determine how every JSON value is interpreted; nothing is inferred
// from the payload shape. Updates are returned in deterministic order, parents before their children.
func DecodeValueOnly(raw json.RawMessage, node *ValueOnlySchemaNode) ([]ValueOnlyUpdate, error) {
	decoder := valueOnlyDecoder{}
	if err := decoder.decode(raw, node); err != nil {
		return nil, err
	}
	return decoder.updates, nil
}

// DecodeSubmodelValueOnly decodes the ValueOnly payload of a submodel against its stored top-level elements.
func DecodeSubmodelValueOnly(fields map[string]json.RawMessage, roots map[string]*ValueOnlySchemaNode) ([]ValueOnlyUpdate, error) {
	decoder := valueOnlyDecoder{}
	if err := decoder.decodeNamedChildren(fields, &ValueOnlySchemaNode{Children: roots}); err != nil {
		return nil, err
	}
	return decoder.updates, nil
}

// ValueOnlyAddressedPaths returns the idShort paths below rootPath that a ValueOnly payload can address:
// object members, members of "statements" and "annotations" objects, and list positions. The result is a
// superset of the elements DecodeValueOnly visits, so only these elements need to be loaded from storage.
func ValueOnlyAddressedPaths(rootPath string, raw json.RawMessage) ([]string, error) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("MODEL-VALONLY-PATHS-JSON %w", err)
	}
	paths := make(map[string]struct{})
	collectValueOnlyAddressedPaths(rootPath, payload, paths)
	return slices.Sorted(maps.Keys(paths)), nil
}

func collectValueOnlyAddressedPaths(path string, value any, paths map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]any:
		for key, member := range typed {
			childPath := joinValueOnlyPath(path, key)
			paths[childPath] = struct{}{}
			collectValueOnlyAddressedPaths(childPath, member, paths)
			if nested, ok := member.(map[string]any); ok && (key == "statements" || key == "annotations") {
				collectValueOnlyAddressedPaths(path, nested, paths)
			}
		}
	case []any:
		for index, item := range typed {
			itemPath := path + "[" + strconv.Itoa(index) + "]"
			paths[itemPath] = struct{}{}
			collectValueOnlyAddressedPaths(itemPath, item, paths)
		}
	}
}

func joinValueOnlyPath(parentPath string, idShort string) string {
	if parentPath == "" {
		return idShort
	}
	return parentPath + "." + idShort
}

func (n *ValueOnlySchemaNode) wrap(err error) error {
	return fmt.Errorf("%s: %w", n.IDShortPath, err)
}

func (n *ValueOnlySchemaNode) childPath(idShort string) string {
	return joinValueOnlyPath(n.IDShortPath, idShort)
}

type valueOnlyDecoder struct {
	updates []ValueOnlyUpdate
}

func (d *valueOnlyDecoder) decode(raw json.RawMessage, node *ValueOnlySchemaNode) error {
	switch node.ModelType {
	case types.ModelTypeSubmodelElementCollection:
		fields, err := decodeValueOnlyObject(raw)
		if err != nil {
			return node.wrap(err)
		}
		return d.decodeNamedChildren(fields, node)
	case types.ModelTypeSubmodelElementList:
		return d.decodeListItems(raw, node)
	case types.ModelTypeEntity:
		return d.decodeEntity(raw, node)
	case types.ModelTypeAnnotatedRelationshipElement:
		return d.decodeAnnotatedRelationship(raw, node)
	default:
		patch, err := decodeValueOnlyLeaf(raw, node)
		if err != nil {
			return node.wrap(err)
		}
		d.add(node, patch)
		return nil
	}
}

func (d *valueOnlyDecoder) add(node *ValueOnlySchemaNode, patch ValueOnlyPatch) {
	d.updates = append(d.updates, ValueOnlyUpdate{
		ElementID:   node.ElementID,
		IDShortPath: node.IDShortPath,
		ModelType:   node.ModelType,
		Value:       patch,
	})
}

func (d *valueOnlyDecoder) decodeNamedChildren(fields map[string]json.RawMessage, node *ValueOnlySchemaNode) error {
	for _, idShort := range sortedValueOnlyKeys(fields) {
		child, ok := node.Children[idShort]
		if !ok {
			return fmt.Errorf("%w: %s does not exist", ErrValueOnlyUnknownElement, node.childPath(idShort))
		}
		if err := d.decode(fields[idShort], child); err != nil {
			return err
		}
	}
	return nil
}

func (d *valueOnlyDecoder) decodeListItems(raw json.RawMessage, node *ValueOnlySchemaNode) error {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || items == nil {
		return node.wrap(errors.New("MODEL-VALONLY-DECODE-ARRAY expected JSON array"))
	}
	for i, item := range items {
		if i >= len(node.Items) || node.Items[i] == nil {
			return fmt.Errorf("%w: %s[%d] does not exist", ErrValueOnlyUnknownElement, node.IDShortPath, i)
		}
		if err := d.decode(item, node.Items[i]); err != nil {
			return err
		}
	}
	return nil
}

func (d *valueOnlyDecoder) decodeEntity(raw json.RawMessage, node *ValueOnlySchemaNode) error {
	fields, err := decodeValueOnlyObject(raw, "entityType", "globalAssetId", "specificAssetIds", "statements")
	if err != nil {
		return node.wrap(err)
	}
	patch, err := decodeEntityAttributes(fields)
	if err != nil {
		return node.wrap(err)
	}
	d.add(node, patch)
	return d.decodeNestedChildren(fields, "statements", node)
}

func (d *valueOnlyDecoder) decodeAnnotatedRelationship(raw json.RawMessage, node *ValueOnlySchemaNode) error {
	fields, err := decodeValueOnlyObject(raw, "first", "second", "annotations")
	if err != nil {
		return node.wrap(err)
	}
	patch, err := decodeRelationshipFields(fields, node.ModelType)
	if err != nil {
		return node.wrap(err)
	}
	d.add(node, patch)
	return d.decodeNestedChildren(fields, "annotations", node)
}

func (d *valueOnlyDecoder) decodeNestedChildren(fields map[string]json.RawMessage, name string, node *ValueOnlySchemaNode) error {
	raw, ok := fields[name]
	if !ok {
		return nil
	}
	nested, err := decodeValueOnlyObject(raw)
	if err != nil {
		return node.wrap(fmt.Errorf("%s: %w", name, err))
	}
	return d.decodeNamedChildren(nested, node)
}

func decodeValueOnlyLeaf(raw json.RawMessage, node *ValueOnlySchemaNode) (ValueOnlyPatch, error) {
	switch node.ModelType {
	case types.ModelTypeProperty:
		return decodePropertyPatch(raw, node.ValueType)
	case types.ModelTypeRange:
		return decodeRangePatch(raw, node.ValueType)
	case types.ModelTypeMultiLanguageProperty:
		return decodeMultiLanguagePropertyPatch(raw)
	case types.ModelTypeFile:
		return decodeFilePatch(raw)
	case types.ModelTypeBlob:
		return decodeBlobPatch(raw)
	case types.ModelTypeReferenceElement:
		return decodeReferenceElementPatch(raw)
	case types.ModelTypeRelationshipElement:
		fields, err := decodeValueOnlyObject(raw, "first", "second")
		if err != nil {
			return nil, err
		}
		return decodeRelationshipFields(fields, node.ModelType)
	case types.ModelTypeBasicEventElement:
		return decodeBasicEventElementPatch(raw)
	default:
		name, _ := stringification.ModelTypeToString(node.ModelType)
		return nil, fmt.Errorf("MODEL-VALONLY-DECODE-UNSUPPORTED %s has no ValueOnly representation", name)
	}
}

func decodePropertyPatch(raw json.RawMessage, valueType types.DataTypeDefXSD) (ValueOnlyPatch, error) {
	lexical, err := decodeScalarLexical(raw, valueType)
	if err != nil {
		return nil, err
	}
	return PropertyValue{Value: lexical, ValueType: valueType}, nil
}

func decodeRangePatch(raw json.RawMessage, valueType types.DataTypeDefXSD) (ValueOnlyPatch, error) {
	fields, err := decodeValueOnlyObject(raw, "min", "max")
	if err != nil {
		return nil, err
	}
	patch := RangeValuePatch{ValueType: valueType}
	if patch.Min, err = decodeScalarField(fields, "min", valueType); err != nil {
		return nil, err
	}
	if patch.Max, err = decodeScalarField(fields, "max", valueType); err != nil {
		return nil, err
	}
	return patch, nil
}

func decodeScalarField(fields map[string]json.RawMessage, name string, valueType types.DataTypeDefXSD) (ScalarPatch, error) {
	raw, ok := fields[name]
	if !ok {
		return ScalarPatch{}, nil
	}
	lexical, err := decodeScalarLexical(raw, valueType)
	if err != nil {
		return ScalarPatch{}, fmt.Errorf("%s: %w", name, err)
	}
	return ScalarPatch{Set: true, Value: lexical}, nil
}

func decodeScalarLexical(raw json.RawMessage, valueType types.DataTypeDefXSD) (*string, error) {
	value, err := decodeValueOnlyJSON(raw)
	if err != nil {
		return nil, err
	}
	return ValueOnlyScalarFromJSON(value, valueType)
}

func decodeMultiLanguagePropertyPatch(raw json.RawMessage) (ValueOnlyPatch, error) {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || items == nil {
		return nil, errors.New("MODEL-VALONLY-DECODE-MLP expected JSON array of language objects")
	}
	result := make(MultiLanguagePropertyValue, 0, len(items))
	languages := make(map[string]struct{}, len(items))
	for i, item := range items {
		language, text, err := decodeLangString(item)
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		if _, duplicate := languages[language]; duplicate {
			return nil, fmt.Errorf("MODEL-VALONLY-DECODE-MLPDUPLICATE language %q occurs more than once", language)
		}
		languages[language] = struct{}{}
		result = append(result, map[string]string{language: text})
	}
	return result, nil
}

func decodeLangString(item map[string]json.RawMessage) (string, string, error) {
	if len(item) != 1 {
		return "", "", errors.New("MODEL-VALONLY-DECODE-MLPITEM each item must contain exactly one language")
	}
	for language, rawText := range item {
		text, err := decodeValueOnlyString(rawText)
		if err != nil {
			return "", "", fmt.Errorf("%s: %w", language, err)
		}
		return language, text, verifyValueOnlyClass("MODEL-VALONLY-DECODE-MLPVERIFY", types.NewLangStringTextType(language, text))
	}
	return "", "", nil
}

func decodeFilePatch(raw json.RawMessage) (ValueOnlyPatch, error) {
	fields, err := decodeValueOnlyObject(raw, "contentType", "value")
	if err != nil {
		return nil, err
	}
	contentType, err := decodeContentType(fields)
	if err != nil {
		return nil, err
	}
	value, err := decodeOptionalString(fields, "value")
	if err != nil {
		return nil, err
	}
	return FileValuePatch{ContentType: contentType, Value: value}, nil
}

func decodeBlobPatch(raw json.RawMessage) (ValueOnlyPatch, error) {
	fields, err := decodeValueOnlyObject(raw, "contentType", "value")
	if err != nil {
		return nil, err
	}
	contentType, err := decodeContentType(fields)
	if err != nil {
		return nil, err
	}
	encoded, err := decodeOptionalString(fields, "value")
	if err != nil || encoded == nil {
		return BlobValuePatch{ContentType: contentType}, err
	}
	content, err := decodeStandardBase64(*encoded)
	if err != nil {
		return nil, err
	}
	return BlobValuePatch{ContentType: contentType, Value: &content}, nil
}

func decodeReferenceElementPatch(raw json.RawMessage) (ValueOnlyPatch, error) {
	jsonable, err := decodeValueOnlyJSON(raw)
	if err != nil {
		return nil, err
	}
	if object, ok := jsonable.(map[string]any); ok && len(object) == 0 {
		return ReferenceElementValuePatch{}, nil
	}
	reference, err := decodeReferenceJsonable(jsonable)
	if err != nil {
		return nil, err
	}
	return ReferenceElementValuePatch{Value: reference}, nil
}

func decodeRelationshipFields(fields map[string]json.RawMessage, modelType types.ModelType) (ValueOnlyPatch, error) {
	patch := RelationshipValuePatch{ModelType: modelType}
	var err error
	if patch.First, err = decodeOptionalReference(fields, "first"); err != nil {
		return nil, err
	}
	if patch.Second, err = decodeOptionalReference(fields, "second"); err != nil {
		return nil, err
	}
	return patch, nil
}

func decodeBasicEventElementPatch(raw json.RawMessage) (ValueOnlyPatch, error) {
	fields, err := decodeValueOnlyObject(raw, "observed")
	if err != nil {
		return nil, err
	}
	observed, err := decodeOptionalReference(fields, "observed")
	if err != nil {
		return nil, err
	}
	return BasicEventElementValuePatch{Observed: observed}, nil
}

func decodeEntityAttributes(fields map[string]json.RawMessage) (EntityValuePatch, error) {
	patch := EntityValuePatch{}
	var err error
	if patch.EntityType, err = decodeEntityType(fields); err != nil {
		return patch, err
	}
	if patch.GlobalAssetID, err = decodeGlobalAssetID(fields); err != nil {
		return patch, err
	}
	patch.SpecificAssetIDs, err = decodeSpecificAssetIDs(fields)
	return patch, err
}

func decodeEntityType(fields map[string]json.RawMessage) (*types.EntityType, error) {
	text, err := decodeOptionalString(fields, "entityType")
	if err != nil || text == nil {
		return nil, err
	}
	entityType, ok := stringification.EntityTypeFromString(*text)
	if !ok {
		return nil, fmt.Errorf("MODEL-VALONLY-DECODE-ENTITYTYPE unknown entityType %q", *text)
	}
	return &entityType, nil
}

func decodeValueOnlyObject(raw json.RawMessage, allowed ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, errors.New("MODEL-VALONLY-DECODE-OBJECT expected JSON object")
	}
	if len(allowed) == 0 {
		return fields, nil
	}
	for _, name := range sortedValueOnlyKeys(fields) {
		if !slices.Contains(allowed, name) {
			return nil, fmt.Errorf("MODEL-VALONLY-DECODE-UNKNOWNFIELD %s is not a ValueOnly field, expected one of %s", strconv.Quote(name), allowed)
		}
	}
	return fields, nil
}

func sortedValueOnlyKeys(fields map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
