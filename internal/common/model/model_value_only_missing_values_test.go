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

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/stretchr/testify/require"
)

const missingValuesSubmodelJSON = `{
	"modelType": "Submodel",
	"id": "urn:example:novalue",
	"submodelElements": [
		{"modelType": "Property", "idShort": "prop", "valueType": "xs:int"},
		{"modelType": "MultiLanguageProperty", "idShort": "mlp"},
		{"modelType": "Range", "idShort": "rngmin", "valueType": "xs:int", "min": "1"},
		{"modelType": "Range", "idShort": "rngnone", "valueType": "xs:int"},
		{"modelType": "File", "idShort": "file", "contentType": "text/plain"},
		{"modelType": "File", "idShort": "fileNone"},
		{"modelType": "Blob", "idShort": "blob", "value": "aGVsbG8="},
		{"modelType": "Blob", "idShort": "blobNone"},
		{"modelType": "ReferenceElement", "idShort": "ref"},
		{"modelType": "Entity", "idShort": "entity"},
		{"modelType": "SubmodelElementCollection", "idShort": "emptyColl"},
		{"modelType": "SubmodelElementList", "idShort": "emptyList", "typeValueListElement": "Property", "valueTypeListElement": "xs:int"},
		{"modelType": "SubmodelElementCollection", "idShort": "coll",
			"value": [{"modelType": "Property", "idShort": "inner", "valueType": "xs:int"}]},
		{"modelType": "AnnotatedRelationshipElement", "idShort": "rel",
			"first": {"type": "ExternalReference", "keys": [{"type": "GlobalReference", "value": "urn:example:first"}]},
			"second": {"type": "ExternalReference", "keys": [{"type": "GlobalReference", "value": "urn:example:second"}]},
			"annotations": [{"modelType": "Property", "idShort": "note", "valueType": "xs:string"}]},
		{"modelType": "SubmodelElementList", "idShort": "members", "typeValueListElement": "SubmodelElement", "value": [
			{"modelType": "Property", "valueType": "xs:int", "value": "10"},
			{"modelType": "Property", "valueType": "xs:int"},
			{"modelType": "MultiLanguageProperty"},
			{"modelType": "Range", "valueType": "xs:int", "min": "1"},
			{"modelType": "Range", "valueType": "xs:int"},
			{"modelType": "File", "contentType": "text/plain"},
			{"modelType": "Blob"},
			{"modelType": "ReferenceElement"},
			{"modelType": "Entity"},
			{"modelType": "SubmodelElementCollection"},
			{"modelType": "SubmodelElementList", "typeValueListElement": "Property", "valueTypeListElement": "xs:int"}
		]},
		{"modelType": "SubmodelElementList", "idShort": "withOperation", "typeValueListElement": "SubmodelElement", "value": [
			{"modelType": "Property", "valueType": "xs:int", "value": "10"},
			{"modelType": "Operation"}
		]},
		{"modelType": "SubmodelElementCollection", "idShort": "nestedOperationList", "value": [
			{"modelType": "SubmodelElementList", "idShort": "capabilities", "typeValueListElement": "Capability", "value": [
				{"modelType": "Capability"}
			]}
		]}
	]
}`

func TestSubmodelToValueOnlyOmitsElementsWithoutValue(t *testing.T) {
	t.Parallel()

	submodel := missingValuesTestSubmodel(t)

	valueOnly, err := SubmodelToValueOnly(submodel)
	require.NoError(t, err)
	actual, err := json.Marshal(valueOnly)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"rngmin": {"min": 1},
		"file": {"contentType": "text/plain"},
		"blob": {"value": "aGVsbG8="},
		"coll": {},
		"rel": {
			"first": {"type": "ExternalReference", "keys": [{"type": "GlobalReference", "value": "urn:example:first"}]},
			"second": {"type": "ExternalReference", "keys": [{"type": "GlobalReference", "value": "urn:example:second"}]}
		},
		"members": [10, null, [], {"min": 1}, {}, {"contentType": "text/plain"}, {}, null, {}, {}, []],
		"nestedOperationList": {}
	}`, string(actual))
}

func TestSubmodelElementToValueOnlyRendersElementsWithoutValueStandalone(t *testing.T) {
	t.Parallel()

	elements := missingValuesTestElements(t)
	expected := map[string]string{
		"prop":      `null`,
		"mlp":       `[]`,
		"rngmin":    `{"min": 1}`,
		"rngnone":   `{}`,
		"file":      `{"contentType": "text/plain"}`,
		"fileNone":  `{}`,
		"blobNone":  `{}`,
		"ref":       `null`,
		"entity":    `{}`,
		"emptyColl": `{}`,
		"emptyList": `[]`,
		"coll":      `{}`,
	}
	for idShort, want := range expected {
		valueOnly, err := SubmodelElementToValueOnly(elements[idShort])
		require.NoError(t, err, idShort)
		actual, err := json.Marshal(valueOnly)
		require.NoError(t, err, idShort)
		require.JSONEq(t, want, string(actual), idShort)
	}

	withOperation, err := SubmodelElementToValueOnly(elements["withOperation"])
	require.NoError(t, err)
	require.Nil(t, withOperation)
}

func TestSubmodelElementToNamedValueOnlySkipsElementsWithoutValue(t *testing.T) {
	t.Parallel()

	elements := missingValuesTestElements(t)
	for _, idShort := range []string{"prop", "mlp", "rngnone", "fileNone", "blobNone", "ref", "entity", "emptyColl", "emptyList", "withOperation"} {
		valueOnly, err := SubmodelElementToNamedValueOnly(elements[idShort])
		require.NoError(t, err, idShort)
		require.Nil(t, valueOnly, idShort)
	}

	valueOnly, err := SubmodelElementToNamedValueOnly(elements["rngmin"])
	require.NoError(t, err)
	actual, err := json.Marshal(valueOnly)
	require.NoError(t, err)
	require.JSONEq(t, `{"rngmin": {"min": 1}}`, string(actual))
}

func missingValuesTestSubmodel(t *testing.T) types.ISubmodel {
	t.Helper()
	submodel, err := jsonization.SubmodelFromJsonable(decodeTestJSONMap(t, missingValuesSubmodelJSON))
	require.NoError(t, err)
	return submodel
}

func missingValuesTestElements(t *testing.T) map[string]types.ISubmodelElement {
	t.Helper()
	elements := make(map[string]types.ISubmodelElement)
	for _, element := range missingValuesTestSubmodel(t).SubmodelElements() {
		elements[*element.IDShort()] = element
	}
	return elements
}

func TestSubmodelToValueOnlyKeepsExplicitlyEmptyContainers(t *testing.T) {
	t.Parallel()

	collection := types.NewSubmodelElementCollection()
	collection.SetIDShort(stringPointer("collection"))
	collection.SetValue([]types.ISubmodelElement{})
	list := types.NewSubmodelElementList(types.AASSubmodelElementsProperty)
	list.SetIDShort(stringPointer("list"))
	list.SetValue([]types.ISubmodelElement{})
	submodel := types.NewSubmodel("urn:example:empty-containers")
	submodel.SetSubmodelElements([]types.ISubmodelElement{collection, list})

	valueOnly, err := SubmodelToValueOnly(submodel)
	require.NoError(t, err)
	actual, err := json.Marshal(valueOnly)
	require.NoError(t, err)
	require.JSONEq(t, `{"collection": {}, "list": []}`, string(actual))
}
