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
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	nestedMatchSubmodelID        = "urn:basyx:integration:query-match:nested"
	foreignNestedMatchSubmodelID = "urn:basyx:integration:query-match:foreign"
)

func TestQueryMatchFiltersNestedFragmentResults(t *testing.T) {
	endpointForID := func(id string) string {
		encodedID := base64.RawURLEncoding.EncodeToString([]byte(id))
		return fmt.Sprintf("%s/submodels/%s", submodelRepositoryBaseURL, encodedID)
	}
	for _, id := range []string{nestedMatchSubmodelID, foreignNestedMatchSubmodelID} {
		_, _, _ = requestJSON(http.MethodDelete, endpointForID(id), nil)
	}

	statusCode, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/submodels", nestedMatchSubmodel())
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))
	statusCode, body, err = requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/submodels", foreignNestedMatchSubmodel())
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))
	t.Cleanup(func() {
		for _, id := range []string{nestedMatchSubmodelID, foreignNestedMatchSubmodelID} {
			deleteStatus, deleteBody, deleteErr := requestJSON(http.MethodDelete, endpointForID(id), nil)
			assert.NoError(t, deleteErr)
			assert.Equal(t, http.StatusNoContent, deleteStatus, "response=%s", string(deleteBody))
		}
	})

	t.Run("OmittedMatchKeepsExistentialBehavior", func(t *testing.T) {
		result := queryNestedMatchSubmodel(t, nestedMatchFilter(
			"$sme.a[].b[]",
			false,
			equalsCondition("$sme.a[].b[]#value", "a0-b1"),
		))

		assert.Equal(t, [][]string{{"a0-b0", "a0-b1"}, {"a1-b0", "a1-b1"}}, nestedBValues(t, result))
	})

	for _, test := range []struct {
		name          string
		explicitFalse bool
	}{
		{name: "OmittedMatchKeepsSubmodelSemanticID"},
		{name: "ExplicitFalseMatchKeepsSubmodelSemanticID", explicitFalse: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			filter := nestedMatchFilter(
				"$sm#semanticId.keys[]",
				false,
				equalsCondition("$sm#semanticId.keys[].value", "submodel-semantic-allowed"),
			)
			if test.explicitFalse {
				filter["$match"] = false
			}

			result := queryNestedMatchSubmodel(t, filter)

			assert.Equal(t,
				[]string{"submodel-semantic-allowed", "submodel-semantic-extra"},
				referenceKeyValues(t, []any{result["semanticId"]}),
			)
		})
	}

	t.Run("SamePathMatchesCurrentNestedListEntry", func(t *testing.T) {
		result := queryNestedMatchSubmodel(t, nestedMatchFilter(
			"$sme.a[].b[]",
			true,
			equalsCondition("$sme.a[].b[]#value", "a0-b1"),
		))

		assert.Equal(t, [][]string{{"a0-b1"}, {}}, nestedBValues(t, result))
	})

	t.Run("ExplicitFirstParentIndexOnlyFiltersThatParent", func(t *testing.T) {
		result := queryNestedMatchSubmodel(t, nestedMatchFilter(
			"$sme.a[0].b[]",
			true,
			equalsCondition("$sme.a[].b[]#value", "a0-b1"),
		))

		assert.Equal(t, [][]string{{"a0-b1"}, {"a1-b0", "a1-b1"}}, nestedBValues(t, result))
	})

	t.Run("ExplicitSecondParentIndexOnlyFiltersThatParent", func(t *testing.T) {
		result := queryNestedMatchSubmodel(t, nestedMatchFilter(
			"$sme.a[1].b[]",
			true,
			map[string]any{
				"$contains": []any{
					map[string]any{"$field": "$sme.a[].b[]#value"},
					map[string]any{"$strVal": "-b0"},
				},
			},
		))

		assert.Equal(t, [][]string{{"a0-b0", "a0-b1"}, {"a1-b0"}}, nestedBValues(t, result))
	})

	t.Run("DescendantPathMatchesSharedParentListIndex", func(t *testing.T) {
		result := queryNestedMatchSubmodel(t, nestedMatchFilter(
			"$sme.a[]",
			true,
			equalsCondition("$sme.a[].b[]#value", "a0-b1"),
		))

		assert.Equal(t, [][]string{{"a0-b0", "a0-b1"}}, nestedBValues(t, result))
	})

	t.Run("DescendantPathCannotMatchAnotherSubmodel", func(t *testing.T) {
		result := queryNestedMatchSubmodel(t, nestedMatchFilter(
			"$sme.a[]",
			true,
			equalsCondition("$sme.a[].b[]#value", "foreign-b0"),
		))

		assert.Empty(t, nestedBValues(t, result))
	})

	t.Run("ConditionMayContinueThroughMoreArrays", func(t *testing.T) {
		result := queryNestedMatchSubmodel(t, nestedMatchFilter(
			"$sme.a[].b[]",
			true,
			equalsCondition("$sme.a[].b[]#semanticId.keys[].value", "semantic-a1-b0-allowed"),
		))

		assert.Equal(t, [][]string{{}, {"a1-b0"}}, nestedBValues(t, result))
	})

	t.Run("NestedSemanticKeyFragmentMatchesItsCurrentKey", func(t *testing.T) {
		result := queryNestedMatchSubmodel(t, nestedMatchFilter(
			"$sme.a[].b[]#semanticId.keys[]",
			true,
			equalsCondition("$sme.a[].b[]#semanticId.keys[].value", "semantic-a0-b0-allowed"),
		))

		assert.Equal(t, []string{"semantic-a0-b0-allowed"}, nestedSemanticKeyValues(t, result))
		assert.Equal(t, [][]string{{"a0-b0", "a0-b1"}, {"a1-b0", "a1-b1"}}, nestedBValues(t, result))
	})

	t.Run("DifferentPathActsAsSubmodelGuard", func(t *testing.T) {
		result := queryNestedMatchSubmodel(t, nestedMatchFilter(
			"$sme.a[].b[]",
			true,
			equalsCondition("$sme.guard#value", "enabled"),
		))

		assert.Equal(t, [][]string{{"a0-b0", "a0-b1"}, {"a1-b0", "a1-b1"}}, nestedBValues(t, result))
	})

	t.Run("MixedSameDescendantAndDifferentPaths", func(t *testing.T) {
		condition := map[string]any{
			"$and": []any{
				equalsCondition("$sme.a[].b[]#value", "a0-b0"),
				equalsCondition("$sme.a[].b[]#semanticId.keys[].value", "semantic-a0-b0-allowed"),
				equalsCondition("$sme.guard#value", "enabled"),
			},
		}
		result := queryNestedMatchSubmodel(t, nestedMatchFilter("$sme.a[].b[]", true, condition))

		assert.Equal(t, [][]string{{"a0-b0"}, {}}, nestedBValues(t, result))
	})

	t.Run("MultipleFragmentsMatchIndependently", func(t *testing.T) {
		result := queryNestedMatchSubmodel(t,
			nestedMatchFilter(
				"$sme.a[].b[]",
				true,
				map[string]any{
					"$contains": []any{
						map[string]any{"$field": "$sme.a[].b[]#value"},
						map[string]any{"$strVal": "-b0"},
					},
				},
			),
			nestedMatchFilter(
				"$sme.a[].b[]#semanticId.keys[]",
				true,
				map[string]any{
					"$ends-with": []any{
						map[string]any{"$field": "$sme.a[].b[]#semanticId.keys[].value"},
						map[string]any{"$strVal": "-allowed"},
					},
				},
			),
			nestedMatchFilter(
				"$sm#supplementalSemanticIds[]",
				true,
				equalsCondition("$sm#supplementalSemanticIds[].keys[].value", "submodel-allowed"),
			),
		)

		assert.Equal(t, [][]string{{"a0-b0"}, {"a1-b0"}}, nestedBValues(t, result))
		assert.Equal(t, []string{"semantic-a0-b0-allowed", "semantic-a1-b0-allowed"}, nestedSemanticKeyValues(t, result))
		assert.Equal(t, []string{"submodel-allowed"}, referenceKeyValues(t, result["supplementalSemanticIds"]))
	})
}

func queryNestedMatchSubmodel(t *testing.T, filters ...map[string]any) map[string]any {
	t.Helper()

	payload := map[string]any{
		"$condition": equalsCondition("$sm#id", nestedMatchSubmodelID),
		"$filters":   filters,
	}
	statusCode, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/query/submodels", payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))

	var response struct {
		Result []map[string]any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &response), "response=%s", string(body))
	require.Len(t, response.Result, 1, "response=%s", string(body))
	require.Equal(t, nestedMatchSubmodelID, response.Result[0]["id"])
	return response.Result[0]
}

func nestedMatchFilter(fragment string, match bool, condition map[string]any) map[string]any {
	filter := map[string]any{
		"$fragment":  fragment,
		"$condition": condition,
	}
	if match {
		filter["$match"] = true
	}
	return filter
}

func equalsCondition(field string, value string) map[string]any {
	return map[string]any{
		"$eq": []any{
			map[string]any{"$field": field},
			map[string]any{"$strVal": value},
		},
	}
}

func nestedBValues(t *testing.T, submodel map[string]any) [][]string {
	t.Helper()

	a := elementByIDShort(t, arrayValue(t, submodel["submodelElements"]), "a")
	result := make([][]string, 0)
	for _, aEntry := range arrayValue(t, a["value"]) {
		collection := objectValue(t, aEntry)
		b := elementByIDShort(t, arrayValue(t, collection["value"]), "b")
		values := make([]string, 0)
		for _, bEntry := range optionalArrayValue(t, b["value"]) {
			property := objectValue(t, bEntry)
			value, ok := property["value"].(string)
			require.True(t, ok, "property value is not a string: %#v", property["value"])
			values = append(values, value)
		}
		result = append(result, values)
	}
	return result
}

func nestedSemanticKeyValues(t *testing.T, submodel map[string]any) []string {
	t.Helper()

	a := elementByIDShort(t, arrayValue(t, submodel["submodelElements"]), "a")
	result := make([]string, 0)
	for _, aEntry := range arrayValue(t, a["value"]) {
		collection := objectValue(t, aEntry)
		b := elementByIDShort(t, arrayValue(t, collection["value"]), "b")
		for _, bEntry := range arrayValue(t, b["value"]) {
			property := objectValue(t, bEntry)
			semanticID, exists := property["semanticId"]
			if !exists {
				continue
			}
			result = append(result, referenceKeyValues(t, []any{semanticID})...)
		}
	}
	return result
}

func referenceKeyValues(t *testing.T, referencesValue any) []string {
	t.Helper()

	result := make([]string, 0)
	for _, referenceValue := range arrayValue(t, referencesValue) {
		reference := objectValue(t, referenceValue)
		for _, keyValue := range arrayValue(t, reference["keys"]) {
			key := objectValue(t, keyValue)
			value, ok := key["value"].(string)
			require.True(t, ok, "reference key value is not a string: %#v", key["value"])
			result = append(result, value)
		}
	}
	return result
}

func elementByIDShort(t *testing.T, elements []any, idShort string) map[string]any {
	t.Helper()

	for _, elementValue := range elements {
		element := objectValue(t, elementValue)
		if element["idShort"] == idShort {
			return element
		}
	}
	require.FailNow(t, "submodel element not found", "idShort=%s elements=%#v", idShort, elements)
	return nil
}

func arrayValue(t *testing.T, value any) []any {
	t.Helper()

	result, ok := value.([]any)
	require.True(t, ok, "value is not an array: %#v", value)
	return result
}

func optionalArrayValue(t *testing.T, value any) []any {
	t.Helper()

	if value == nil {
		return []any{}
	}
	return arrayValue(t, value)
}

func objectValue(t *testing.T, value any) map[string]any {
	t.Helper()

	result, ok := value.(map[string]any)
	require.True(t, ok, "value is not an object: %#v", value)
	return result
}

func nestedMatchSubmodel() map[string]any {
	return map[string]any{
		"id":         nestedMatchSubmodelID,
		"idShort":    "NestedMatch",
		"modelType":  "Submodel",
		"semanticId": referenceWithKeys("submodel-semantic-allowed", "submodel-semantic-extra"),
		"supplementalSemanticIds": []any{
			reference("submodel-allowed"),
			reference("submodel-denied"),
		},
		"submodelElements": []any{
			property("guard", "enabled", nil),
			map[string]any{
				"idShort":              "a",
				"modelType":            "SubmodelElementList",
				"orderRelevant":        true,
				"typeValueListElement": "SubmodelElementCollection",
				"value": []any{
					nestedMatchAEntry("a0"),
					nestedMatchAEntry("a1"),
				},
			},
		},
	}
}

func foreignNestedMatchSubmodel() map[string]any {
	return map[string]any{
		"id":        foreignNestedMatchSubmodelID,
		"idShort":   "ForeignNestedMatch",
		"modelType": "Submodel",
		"submodelElements": []any{
			map[string]any{
				"idShort":              "a",
				"modelType":            "SubmodelElementList",
				"orderRelevant":        true,
				"typeValueListElement": "SubmodelElementCollection",
				"value": []any{
					nestedMatchAEntry("foreign"),
				},
			},
		},
	}
}

func nestedMatchAEntry(prefix string) map[string]any {
	return map[string]any{
		"modelType": "SubmodelElementCollection",
		"value": []any{
			map[string]any{
				"idShort":              "b",
				"modelType":            "SubmodelElementList",
				"orderRelevant":        true,
				"typeValueListElement": "Property",
				"value": []any{
					property("", prefix+"-b0", []string{"semantic-" + prefix + "-b0-allowed", "semantic-" + prefix + "-b0-extra"}),
					property("", prefix+"-b1", []string{"semantic-" + prefix + "-b1-extra"}),
				},
			},
		},
	}
}

func property(idShort string, value string, semanticKeys []string) map[string]any {
	result := map[string]any{
		"modelType": "Property",
		"value":     value,
		"valueType": "xs:string",
	}
	if idShort != "" {
		result["idShort"] = idShort
	}
	if len(semanticKeys) > 0 {
		result["semanticId"] = referenceWithKeys(semanticKeys...)
	}
	return result
}

func reference(value string) map[string]any {
	return referenceWithKeys(value)
}

func referenceWithKeys(values ...string) map[string]any {
	keys := make([]any, 0, len(values))
	for _, value := range values {
		keys = append(keys, map[string]any{
			"type":  "GlobalReference",
			"value": value,
		})
	}
	return map[string]any{
		"type": "ExternalReference",
		"keys": keys,
	}
}
