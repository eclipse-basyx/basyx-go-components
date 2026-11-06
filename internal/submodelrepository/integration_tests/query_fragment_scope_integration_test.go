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

//nolint:all
package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fragmentScopeSubmodelID = "urn:basyx:integration:query-fragment-scope"

func TestQueryFragmentFiltersOnlyTargetedFields(t *testing.T) {
	encodedID := base64.RawURLEncoding.EncodeToString([]byte(fragmentScopeSubmodelID))
	endpoint := submodelRepositoryBaseURL + "/submodels/" + encodedID
	_, _, _ = requestJSON(http.MethodDelete, endpoint, nil)
	status, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/submodels", fragmentScopeSubmodel())
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status, "response=%s", body)
	t.Cleanup(func() {
		status, body, err := requestJSON(http.MethodDelete, endpoint, nil)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, status, "response=%s", body)
	})

	baseline := queryFragmentScope(t, "")
	for _, fragment := range []string{"$sm#semanticId.keys[]", "$sm#semanticId.keys[0]", "$sm#supplementalSemanticIds", "$sm#supplementalSemanticIds[]", "$sm#supplementalSemanticIds[0]", "$sm#supplementalSemanticIds[].keys[]", "$sm#supplementalSemanticIds[0].keys[0]"} {
		t.Run("SubmodelScope/"+fragment, func(t *testing.T) {
			result := queryFragmentScope(t, fragment)
			assert.Equal(t, baseline["submodelElements"], result["submodelElements"])
		})
	}
	tests := []struct {
		name     string
		fragment string
		remove   func(*testing.T, map[string]any)
	}{
		{"ListMembersValue", "$sme.items[]#value", func(t *testing.T, sm map[string]any) {
			for _, value := range fragmentScopeList(t, sm, "items") {
				delete(objectValue(t, value), "value")
			}
		}},
		{"IndexedListMemberValue", "$sme.items[0]#value", func(t *testing.T, sm map[string]any) {
			delete(objectValue(t, fragmentScopeList(t, sm, "items")[0]), "value")
		}},
		{"ListValue", "$sme.items#value", func(t *testing.T, sm map[string]any) {
			delete(fragmentScopeElement(t, sm, "items"), "value")
		}},
		{"CollectionMemberValue", "$sme.details.name#value", func(t *testing.T, sm map[string]any) {
			delete(fragmentScopeCollectionMember(t, sm, "details", "name"), "value")
		}},
		{"CollectionValue", "$sme.details#value", func(t *testing.T, sm map[string]any) {
			delete(fragmentScopeElement(t, sm, "details"), "value")
		}},
		{"CollectionMemberInListValue", "$sme.records[].name#value", func(t *testing.T, sm map[string]any) {
			for _, entry := range fragmentScopeList(t, sm, "records") {
				collection := objectValue(t, entry)
				delete(elementByIDShort(t, arrayValue(t, collection["value"]), "name"), "value")
			}
		}},
		{"IndexedCollectionMemberInListValue", "$sme.records[0].name#value", func(t *testing.T, sm map[string]any) {
			collection := objectValue(t, fragmentScopeList(t, sm, "records")[0])
			delete(elementByIDShort(t, arrayValue(t, collection["value"]), "name"), "value")
		}},
		{"CollectionMemberSemanticID", "$sme.details.name#semanticId", func(t *testing.T, sm map[string]any) {
			delete(fragmentScopeCollectionMember(t, sm, "details", "name"), "semanticId")
		}},
		{"IndexedListMemberSemanticID", "$sme.items[0]#semanticId", func(t *testing.T, sm map[string]any) {
			delete(objectValue(t, fragmentScopeList(t, sm, "items")[0]), "semanticId")
		}},
		{"SubmodelSemanticID", "$sm#semanticId", func(_ *testing.T, sm map[string]any) {
			delete(sm, "semanticId")
		}},
		{"SubmodelIDShort", "$sm#idShort", func(_ *testing.T, sm map[string]any) {
			delete(sm, "idShort")
		}},
		{"IndexedListMemberIDShort", "$sme.items[0]#idShort", func(t *testing.T, sm map[string]any) {
			delete(objectValue(t, fragmentScopeList(t, sm, "items")[0]), "idShort")
		}},
		{"AllElementIDShorts", "$sme#idShort", func(t *testing.T, sm map[string]any) {
			for _, list := range []string{"items", "records"} {
				for _, entry := range fragmentScopeList(t, sm, list) {
					delete(objectValue(t, entry), "idShort")
				}
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(baseline)
			require.NoError(t, err)
			var expected map[string]any
			require.NoError(t, json.Unmarshal(encoded, &expected))
			test.remove(t, expected)
			assert.Equal(t, expected, queryFragmentScope(t, test.fragment))
		})
	}
}

func queryFragmentScope(t *testing.T, fragment string) map[string]any {
	t.Helper()
	query := map[string]any{"$condition": equalsCondition("$sm#id", fragmentScopeSubmodelID)}
	if fragment != "" {
		query["$filters"] = []map[string]any{nestedMatchFilter(fragment, false, map[string]any{"$boolean": false})}
	}
	status, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/query/submodels", query)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", body)
	var page struct {
		Result []map[string]any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &page))
	require.Len(t, page.Result, 1)
	return page.Result[0]
}

func fragmentScopeElement(t *testing.T, sm map[string]any, idShort string) map[string]any {
	t.Helper()
	return elementByIDShort(t, arrayValue(t, sm["submodelElements"]), idShort)
}

func fragmentScopeList(t *testing.T, sm map[string]any, idShort string) []any {
	t.Helper()
	entries := arrayValue(t, fragmentScopeElement(t, sm, idShort)["value"])
	require.Len(t, entries, 2)
	return entries
}

func fragmentScopeCollectionMember(t *testing.T, sm map[string]any, collection, member string) map[string]any {
	t.Helper()
	return elementByIDShort(t, arrayValue(t, fragmentScopeElement(t, sm, collection)["value"]), member)
}

func fragmentScopeSubmodel() map[string]any {
	propertyWithSemanticID := func(idShort, value string) map[string]any {
		result := property(idShort, value, []string{"urn:example:semantic:" + value})
		result["supplementalSemanticIds"] = []any{reference("urn:example:supplemental:" + value)}
		return result
	}
	collection := func(idShort, prefix string) map[string]any {
		result := map[string]any{
			"modelType": "SubmodelElementCollection",
			"value": []any{
				propertyWithSemanticID("name", prefix+"Name"),
				propertyWithSemanticID("other", prefix+"Other"),
			},
		}
		if idShort != "" {
			result["idShort"] = idShort
		}
		return result
	}
	listMember := func(value string) map[string]any {
		member := property("", value, nil)
		member["semanticId"] = reference("urn:example:semantic:list-member")
		return member
	}
	return map[string]any{
		"id": fragmentScopeSubmodelID, "idShort": "FragmentScope", "modelType": "Submodel",
		"semanticId":              reference("urn:example:semantic:submodel"),
		"supplementalSemanticIds": []any{reference("urn:example:supplemental:submodel")},
		"submodelElements": []any{
			propertyWithSemanticID("outside", "outside"),
			collection("details", "details"),
			map[string]any{
				"idShort": "items", "modelType": "SubmodelElementList",
				"typeValueListElement": "Property", "valueTypeListElement": "xs:string",
				"value": []any{listMember("first"), listMember("second")},
			},
			map[string]any{
				"idShort": "records", "modelType": "SubmodelElementList",
				"typeValueListElement": "SubmodelElementCollection",
				"value":                []any{collection("", "first"), collection("", "second")},
			},
		},
	}
}
