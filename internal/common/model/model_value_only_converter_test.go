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

const typedValueOnlySubmodelJSON = `{
	"modelType": "Submodel",
	"id": "urn:example:submodel",
	"submodelElements": [
		{"modelType": "Property", "idShort": "Count", "valueType": "xs:int", "value": "42"},
		{"modelType": "Property", "idShort": "Ratio", "valueType": "xs:float", "value": "42.0"},
		{"modelType": "Property", "idShort": "Enabled", "valueType": "xs:boolean", "value": "1"},
		{"modelType": "Property", "idShort": "Serial", "valueType": "xs:string", "value": "42"},
		{"modelType": "Property", "idShort": "Unset", "valueType": "xs:int"},
		{"modelType": "Property", "idShort": "Infinite", "valueType": "xs:double", "value": "INF"},
		{"modelType": "Range", "idShort": "Limits", "valueType": "xs:unsignedLong", "min": "5", "max": "18446744073709551615"},
		{"modelType": "SubmodelElementCollection", "idShort": "Nested", "value": [
			{"modelType": "SubmodelElementList", "idShort": "Values", "typeValueListElement": "Property", "valueTypeListElement": "xs:decimal", "value": [
				{"modelType": "Property", "valueType": "xs:decimal", "value": "0.10"}
			]}
		]},
		{"modelType": "Entity", "idShort": "Asset", "entityType": "SelfManagedEntity", "globalAssetId": "urn:example:asset",
			"specificAssetIds": [{"name": "serial", "value": "4711", "externalSubjectId": {"type": "ExternalReference", "keys": [{"type": "GlobalReference", "value": "urn:example:issuer"}]}}],
			"statements": [{"modelType": "Property", "idShort": "Weight", "valueType": "xs:double", "value": "1.5"}]}
	]
}`

func TestSubmodelToValueOnlyUsesJSONNativeTypes(t *testing.T) {
	t.Parallel()

	jsonable := decodeTestJSONMap(t, typedValueOnlySubmodelJSON)
	submodel, err := jsonization.SubmodelFromJsonable(jsonable)
	require.NoError(t, err)

	valueOnly, err := SubmodelToValueOnly(submodel)
	require.NoError(t, err)
	actual, err := json.Marshal(valueOnly)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"Count": 42,
		"Ratio": 42.0,
		"Enabled": true,
		"Serial": "42",
		"Unset": null,
		"Infinite": "INF",
		"Limits": {"min": 5, "max": 18446744073709551615},
		"Nested": {"Values": [0.10]},
		"Asset": {
			"entityType": "SelfManagedEntity",
			"globalAssetId": "urn:example:asset",
			"specificAssetIds": [{"name": "serial", "value": "4711", "externalSubjectId": {"type": "ExternalReference", "keys": [{"type": "GlobalReference", "value": "urn:example:issuer"}]}}],
			"statements": {"Weight": 1.5}
		}
	}`, string(actual))
	require.Contains(t, string(actual), `"max":18446744073709551615`)
}

func TestSubmodelElementToValueOnlyStrictRejectsInconsistentValues(t *testing.T) {
	t.Parallel()

	property := types.NewProperty(types.DataTypeDefXSDInt)
	property.SetValue(stringPointer("abc"))

	lenient, err := SubmodelElementToValueOnly(property)
	require.NoError(t, err)
	actual, err := json.Marshal(lenient)
	require.NoError(t, err)
	require.Equal(t, `"abc"`, string(actual))

	_, err = SubmodelElementToValueOnlyStrict(property)
	require.ErrorIs(t, err, ErrValueOnlyScalarInconsistent)

	limits := types.NewRange(types.DataTypeDefXSDInt)
	limits.SetMax(stringPointer("1.5"))
	_, err = SubmodelElementToValueOnlyStrict(limits)
	require.ErrorIs(t, err, ErrValueOnlyScalarInconsistent)
}

func decodeTestJSONMap(t *testing.T, payload string) map[string]any {
	t.Helper()
	var jsonable map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &jsonable))
	return jsonable
}
