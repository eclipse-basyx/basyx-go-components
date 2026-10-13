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
	"encoding/json"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/stretchr/testify/require"
)

const testReferenceJSON = `{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"urn:example:first"}]}`

func schemaNode(path string, modelType types.ModelType) *ValueOnlySchemaNode {
	return &ValueOnlySchemaNode{ElementID: int64(len(path)), IDShortPath: path, ModelType: modelType}
}

func scalarNode(path string, modelType types.ModelType, valueType types.DataTypeDefXSD) *ValueOnlySchemaNode {
	node := schemaNode(path, modelType)
	node.ValueType = valueType
	return node
}

func withChildren(node *ValueOnlySchemaNode, children ...*ValueOnlySchemaNode) *ValueOnlySchemaNode {
	node.Children = make(map[string]*ValueOnlySchemaNode, len(children))
	for _, child := range children {
		idShort := child.IDShortPath[len(node.IDShortPath)+1:]
		node.Children[idShort] = child
	}
	return node
}

func decodeSingle(t *testing.T, payload string, node *ValueOnlySchemaNode) ValueOnlyPatch {
	t.Helper()
	updates, err := DecodeValueOnly(json.RawMessage(payload), node)
	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.Equal(t, node.ElementID, updates[0].ElementID)
	require.Equal(t, node.ModelType, updates[0].ModelType)
	return updates[0].Value
}

func TestDecodeValueOnlyProperty(t *testing.T) {
	t.Parallel()

	patch := decodeSingle(t, `42`, scalarNode("Temperature", types.ModelTypeProperty, types.DataTypeDefXSDInt))
	require.Equal(t, PropertyValue{Value: stringPointer("42"), ValueType: types.DataTypeDefXSDInt}, patch)

	patch = decodeSingle(t, `null`, scalarNode("Temperature", types.ModelTypeProperty, types.DataTypeDefXSDInt))
	require.Equal(t, PropertyValue{ValueType: types.DataTypeDefXSDInt}, patch)

	patch = decodeSingle(t, `"42"`, scalarNode("Temperature", types.ModelTypeProperty, types.DataTypeDefXSDInt))
	require.Equal(t, PropertyValue{Value: stringPointer("42"), ValueType: types.DataTypeDefXSDInt}, patch)

	_, err := DecodeValueOnly(json.RawMessage(`true`), scalarNode("Temperature", types.ModelTypeProperty, types.DataTypeDefXSDInt))
	require.ErrorIs(t, err, ErrValueOnlyScalarKind)
	require.ErrorContains(t, err, "Temperature: ")
}

func TestDecodeValueOnlyRange(t *testing.T) {
	t.Parallel()

	node := scalarNode("Limits", types.ModelTypeRange, types.DataTypeDefXSDDouble)
	patch := decodeSingle(t, `{"min":1.5,"max":null}`, node)
	require.Equal(t, RangeValuePatch{
		ValueType: types.DataTypeDefXSDDouble,
		Min:       ScalarPatch{Set: true, Value: stringPointer("1.5")},
		Max:       ScalarPatch{Set: true},
	}, patch)

	patch = decodeSingle(t, `{"max":"INF"}`, node)
	require.Equal(t, RangeValuePatch{ValueType: types.DataTypeDefXSDDouble, Max: ScalarPatch{Set: true, Value: stringPointer("INF")}}, patch)

	for _, payload := range []string{`{"min":"low"}`, `{"min":true}`, `{"min":1,"step":2}`, `[1,2]`, `null`} {
		_, err := DecodeValueOnly(json.RawMessage(payload), node)
		require.Error(t, err, payload)
	}
}

func TestDecodeValueOnlyMultiLanguageProperty(t *testing.T) {
	t.Parallel()

	node := schemaNode("Name", types.ModelTypeMultiLanguageProperty)
	patch := decodeSingle(t, `[{"en":"Name"},{"de":"Name"}]`, node)
	require.Equal(t, MultiLanguagePropertyValue{{"en": "Name"}, {"de": "Name"}}, patch)

	for _, payload := range []string{
		`[{"language":"en","text":"Name"}]`,
		`[{"en":"a"},{"en":"b"}]`,
		`[{"en":1}]`,
		`{"en":"Name"}`,
		`null`,
	} {
		_, err := DecodeValueOnly(json.RawMessage(payload), node)
		require.Error(t, err, payload)
	}
}

func TestDecodeValueOnlyFileAndBlob(t *testing.T) {
	t.Parallel()

	file := decodeSingle(t, `{"contentType":"application/pdf","value":"/manual.pdf"}`, schemaNode("Manual", types.ModelTypeFile))
	require.Equal(t, FileValuePatch{ContentType: stringPointer("application/pdf"), Value: stringPointer("/manual.pdf")}, file)

	blob := decodeSingle(t, `{"contentType":"application/octet-stream","value":"AP9oYWxsbw=="}`, schemaNode("Payload", types.ModelTypeBlob))
	content := []byte{0x00, 0xff, 'h', 'a', 'l', 'l', 'o'}
	require.Equal(t, BlobValuePatch{ContentType: stringPointer("application/octet-stream"), Value: &content}, blob)

	withoutValue := decodeSingle(t, `{"contentType":"text/plain"}`, schemaNode("Payload", types.ModelTypeBlob))
	require.Equal(t, BlobValuePatch{ContentType: stringPointer("text/plain")}, withoutValue)

	_, err := DecodeValueOnly(json.RawMessage(`{"contentType":"text/plain","value":"hallo"}`), schemaNode("Payload", types.ModelTypeBlob))
	require.ErrorContains(t, err, "MODEL-VALONLY-DECODE-BASE64")
	_, err = DecodeValueOnly(json.RawMessage(`{"contentType":"text/plain","path":"x"}`), schemaNode("Manual", types.ModelTypeFile))
	require.ErrorContains(t, err, "MODEL-VALONLY-DECODE-UNKNOWNFIELD")
}

func TestDecodeValueOnlyReferences(t *testing.T) {
	t.Parallel()

	reference := decodeSingle(t, testReferenceJSON, schemaNode("Ref", types.ModelTypeReferenceElement)).(ReferenceElementValuePatch)
	require.Equal(t, "urn:example:first", reference.Value.Keys()[0].Value())

	cleared := decodeSingle(t, `{}`, schemaNode("Ref", types.ModelTypeReferenceElement))
	require.Equal(t, ReferenceElementValuePatch{}, cleared)

	relationship := decodeSingle(t, `{"second":`+testReferenceJSON+`}`, schemaNode("Rel", types.ModelTypeRelationshipElement)).(RelationshipValuePatch)
	require.Nil(t, relationship.First)
	require.NotNil(t, relationship.Second)
	require.Equal(t, types.ModelTypeRelationshipElement, relationship.GetModelType())

	event := decodeSingle(t, `{"observed":`+testReferenceJSON+`}`, schemaNode("Event", types.ModelTypeBasicEventElement)).(BasicEventElementValuePatch)
	require.NotNil(t, event.Observed)

	unknownMember := `{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"urn:example:first","extra":1}]}`
	_, err := DecodeValueOnly(json.RawMessage(unknownMember), schemaNode("Ref", types.ModelTypeReferenceElement))
	require.ErrorContains(t, err, "MODEL-VALONLY-DECODE-REFERENCE")
	_, err = DecodeValueOnly(json.RawMessage(`{"type":"Unknown"}`), schemaNode("Ref", types.ModelTypeReferenceElement))
	require.ErrorContains(t, err, "MODEL-VALONLY-DECODE-REFERENCE")
}

func TestDecodeValueOnlyEntityAppliesAttributesAndStatements(t *testing.T) {
	t.Parallel()

	node := withChildren(schemaNode("Entity", types.ModelTypeEntity),
		scalarNode("Entity.Count", types.ModelTypeProperty, types.DataTypeDefXSDInt))
	payload := `{"entityType":"SelfManagedEntity","globalAssetId":"urn:example:asset",` +
		`"specificAssetIds":[{"name":"serial","value":"4711"}],"statements":{"Count":3}}`
	updates, err := DecodeValueOnly(json.RawMessage(payload), node)
	require.NoError(t, err)
	require.Len(t, updates, 2)

	entity := updates[0].Value.(EntityValuePatch)
	require.Equal(t, types.EntityTypeSelfManagedEntity, *entity.EntityType)
	require.Equal(t, "urn:example:asset", *entity.GlobalAssetID)
	require.Len(t, *entity.SpecificAssetIDs, 1)
	require.Equal(t, PropertyValue{Value: stringPointer("3"), ValueType: types.DataTypeDefXSDInt}, updates[1].Value)

	untouched := decodeSingle(t, `{}`, schemaNode("Entity", types.ModelTypeEntity))
	require.Equal(t, EntityValuePatch{}, untouched)

	_, err = DecodeValueOnly(json.RawMessage(`{"entityType":"Unknown"}`), node)
	require.ErrorContains(t, err, "MODEL-VALONLY-DECODE-ENTITYTYPE")
	_, err = DecodeValueOnly(json.RawMessage(`{"statements":{"Missing":1}}`), node)
	require.ErrorIs(t, err, ErrValueOnlyUnknownElement)
}

func TestDecodeValueOnlyAnnotatedRelationshipElement(t *testing.T) {
	t.Parallel()

	node := withChildren(schemaNode("Rel", types.ModelTypeAnnotatedRelationshipElement),
		scalarNode("Rel.Note", types.ModelTypeProperty, types.DataTypeDefXSDString))
	updates, err := DecodeValueOnly(json.RawMessage(`{"first":`+testReferenceJSON+`,"annotations":{"Note":"checked"}}`), node)
	require.NoError(t, err)
	require.Len(t, updates, 2)
	relationship := updates[0].Value.(RelationshipValuePatch)
	require.NotNil(t, relationship.First)
	require.Nil(t, relationship.Second)
	require.Equal(t, types.ModelTypeAnnotatedRelationshipElement, relationship.GetModelType())
	require.Equal(t, "Rel.Note", updates[1].IDShortPath)
}

func TestDecodeValueOnlyCollectionsAndListsUseStoredSchema(t *testing.T) {
	t.Parallel()

	list := schemaNode("Rows", types.ModelTypeSubmodelElementList)
	for _, path := range []string{"Rows[0]", "Rows[1]"} {
		list.Items = append(list.Items, withChildren(schemaNode(path, types.ModelTypeSubmodelElementCollection),
			scalarNode(path+".Name", types.ModelTypeProperty, types.DataTypeDefXSDString)))
	}
	collection := withChildren(schemaNode("Data", types.ModelTypeSubmodelElementCollection),
		scalarNode("Data.Enabled", types.ModelTypeProperty, types.DataTypeDefXSDBoolean),
		scalarNode("Data.Count", types.ModelTypeProperty, types.DataTypeDefXSDLong))
	roots := map[string]*ValueOnlySchemaNode{"Rows": list, "Data": collection}

	updates, err := DecodeSubmodelValueOnly(map[string]json.RawMessage{
		"Rows": json.RawMessage(`[{"Name":"first"}]`),
		"Data": json.RawMessage(`{"Enabled":true,"Count":9007199254740993}`),
	}, roots)
	require.NoError(t, err)
	paths := make([]string, 0, len(updates))
	for _, update := range updates {
		paths = append(paths, update.IDShortPath)
	}
	require.Equal(t, []string{"Data.Count", "Data.Enabled", "Rows[0].Name"}, paths)
	require.Equal(t, stringPointer("9007199254740993"), updates[0].Value.(PropertyValue).Value)

	_, err = DecodeValueOnly(json.RawMessage(`[{},{},{}]`), list)
	require.ErrorIs(t, err, ErrValueOnlyUnknownElement)
	_, err = DecodeValueOnly(json.RawMessage(`{"Unknown":1}`), collection)
	require.ErrorIs(t, err, ErrValueOnlyUnknownElement)
	_, err = DecodeSubmodelValueOnly(map[string]json.RawMessage{"Unknown": json.RawMessage(`1`)}, roots)
	require.ErrorIs(t, err, ErrValueOnlyUnknownElement)
	_, err = DecodeValueOnly(json.RawMessage(`[{"en":"text"}]`), schemaNode("Data", types.ModelTypeSubmodelElementCollection))
	require.ErrorContains(t, err, "MODEL-VALONLY-DECODE-OBJECT")
}

func TestDecodeValueOnlyRejectsElementsWithoutValueOnlyRepresentation(t *testing.T) {
	t.Parallel()

	for _, modelType := range []types.ModelType{types.ModelTypeOperation, types.ModelTypeCapability} {
		_, err := DecodeValueOnly(json.RawMessage(`{}`), schemaNode("Element", modelType))
		require.ErrorContains(t, err, "MODEL-VALONLY-DECODE-UNSUPPORTED")
	}
}

func TestParseValueOnlyPayload(t *testing.T) {
	t.Parallel()

	raw, err := ParseValueOnlyPayload([]byte(" 42 \n"))
	require.NoError(t, err)
	require.Equal(t, json.RawMessage(`42`), raw)
	for _, body := range []string{``, `{`, `1 2`} {
		_, err = ParseValueOnlyPayload([]byte(body))
		require.Error(t, err, body)
	}
	fields, err := ParseSubmodelValueOnlyPayload([]byte(`{"Count":18446744073709551615}`))
	require.NoError(t, err)
	require.Equal(t, json.RawMessage(`18446744073709551615`), fields["Count"])
	_, err = ParseSubmodelValueOnlyPayload([]byte(`null`))
	require.Error(t, err)
}
