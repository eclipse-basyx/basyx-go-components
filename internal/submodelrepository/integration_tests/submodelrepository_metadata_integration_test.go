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
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

var expectedSubmodelElementMetadataFields = map[string][]string{
	"Temperature":       {"idShort", "modelType", "description", "semanticId", "valueType"},
	"Label":             {"idShort", "modelType"},
	"Limits":            {"idShort", "modelType", "valueType"},
	"Firmware":          {"idShort", "modelType"},
	"Manual":            {"idShort", "modelType"},
	"Link":              {"idShort", "modelType"},
	"Relation":          {"idShort", "modelType"},
	"AnnotatedRelation": {"idShort", "modelType"},
	"Motor":             {"idShort", "modelType"},
	"Event":             {"idShort", "modelType", "direction", "state", "messageTopic"},
	"Calibrate":         {"idShort", "modelType"},
	"Diagnose":          {"idShort", "modelType"},
	"Housing":           {"idShort", "modelType"},
	"Readings":          {"idShort", "modelType", "orderRelevant", "typeValueListElement", "valueTypeListElement"},
}

func metadataTestModelReference(submodelID string, idShort string) map[string]any {
	return map[string]any{
		"type": "ModelReference",
		"keys": []map[string]any{
			{"type": "Submodel", "value": submodelID},
			{"type": "Property", "value": idShort},
		},
	}
}

func metadataTestExternalReference(value string) map[string]any {
	return map[string]any{
		"type": "ExternalReference",
		"keys": []map[string]any{
			{"type": "GlobalReference", "value": value},
		},
	}
}

func metadataTestProperty(idShort string, valueType string, value string) map[string]any {
	property := map[string]any{"modelType": "Property", "valueType": valueType, "value": value}
	if idShort != "" {
		property["idShort"] = idShort
	}
	return property
}

func metadataTestReadingsList() map[string]any {
	return map[string]any{
		"idShort":              "Readings",
		"modelType":            "SubmodelElementList",
		"orderRelevant":        true,
		"typeValueListElement": "Property",
		"valueTypeListElement": "xs:int",
		"value":                []any{metadataTestProperty("", "xs:int", "1"), metadataTestProperty("", "xs:int", "2")},
	}
}

func metadataStrictTestSubmodelPayload(submodelID string) map[string]any {
	return map[string]any{
		"id":        submodelID,
		"idShort":   "MetadataStrictVerification",
		"kind":      "Instance",
		"modelType": "Submodel",
		"submodelElements": []any{
			metadataTestProperty("Temperature", "xs:int", "20"),
			metadataTestReadingsList(),
		},
	}
}

func metadataTestSubmodelPayload(submodelID string) map[string]any {
	temperatureReference := metadataTestModelReference(submodelID, "Temperature")
	temperature := metadataTestProperty("Temperature", "xs:int", "20")
	temperature["valueId"] = metadataTestExternalReference("urn:example:value:temperature")
	temperature["semanticId"] = metadataTestExternalReference("urn:example:semantic:temperature")
	temperature["description"] = []map[string]any{{"language": "en", "text": "Temperature"}}

	return map[string]any{
		"id":        submodelID,
		"idShort":   "MetadataRepresentation",
		"kind":      "Instance",
		"modelType": "Submodel",
		"submodelElements": []any{
			temperature,
			map[string]any{
				"idShort":   "Label",
				"modelType": "MultiLanguageProperty",
				"value":     []map[string]any{{"language": "en", "text": "Motor"}},
				"valueId":   metadataTestExternalReference("urn:example:value:label"),
			},
			map[string]any{"idShort": "Limits", "modelType": "Range", "valueType": "xs:int", "min": "1", "max": "5"},
			map[string]any{
				"idShort":     "Firmware",
				"modelType":   "Blob",
				"contentType": "application/octet-stream",
				"value":       base64.StdEncoding.EncodeToString([]byte("firmware")),
			},
			map[string]any{"idShort": "Manual", "modelType": "File", "contentType": "application/pdf", "value": "/manual.pdf"},
			map[string]any{"idShort": "Link", "modelType": "ReferenceElement", "value": temperatureReference},
			map[string]any{"idShort": "Relation", "modelType": "RelationshipElement", "first": temperatureReference, "second": temperatureReference},
			map[string]any{
				"idShort":     "AnnotatedRelation",
				"modelType":   "AnnotatedRelationshipElement",
				"first":       temperatureReference,
				"second":      temperatureReference,
				"annotations": []any{metadataTestProperty("Note", "xs:string", "note")},
			},
			map[string]any{
				"idShort":          "Motor",
				"modelType":        "Entity",
				"entityType":       "SelfManagedEntity",
				"globalAssetId":    "urn:example:asset:motor",
				"specificAssetIds": []map[string]any{{"name": "serialNumber", "value": "123"}},
				"statements":       []any{metadataTestProperty("Speed", "xs:int", "1")},
			},
			map[string]any{
				"idShort":      "Event",
				"modelType":    "BasicEventElement",
				"observed":     temperatureReference,
				"direction":    "output",
				"state":        "on",
				"messageTopic": "motor/events",
			},
			map[string]any{
				"idShort":        "Calibrate",
				"modelType":      "Operation",
				"inputVariables": []any{map[string]any{"value": metadataTestProperty("Factor", "xs:double", "1.5")}},
			},
			map[string]any{"idShort": "Diagnose", "modelType": "Capability"},
			map[string]any{
				"idShort":   "Housing",
				"modelType": "SubmodelElementCollection",
				"value":     []any{metadataTestProperty("Color", "xs:string", "red")},
			},
			metadataTestReadingsList(),
		},
	}
}

func createMetadataTestSubmodel(t *testing.T, baseURL string, prefix string, payload func(submodelID string) map[string]any) (string, string) {
	t.Helper()
	submodelID := fmt.Sprintf("urn:basyx:integration:%s-%d", prefix, time.Now().UnixNano())
	encodedSubmodelID := common.EncodeString(submodelID)

	statusCode, body, err := requestJSON(http.MethodPost, baseURL+"/submodels", payload(submodelID))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))

	t.Cleanup(func() {
		status, cleanupBody, cleanupErr := requestJSON(http.MethodDelete, baseURL+"/submodels/"+encodedSubmodelID, nil)
		if cleanupErr != nil {
			t.Logf("cleanup delete failed: %v", cleanupErr)
			return
		}
		if status != http.StatusNoContent && status != http.StatusNotFound {
			t.Logf("cleanup delete returned status=%d response=%s", status, string(cleanupBody))
		}
	})

	return submodelID, fmt.Sprintf("%s/submodels/%s/submodel-elements", baseURL, encodedSubmodelID)
}

func requireMetadataFields(t *testing.T, metadata map[string]any, expectedFields []string) {
	t.Helper()
	actualFields := make([]string, 0, len(metadata))
	for field := range metadata {
		actualFields = append(actualFields, field)
	}
	require.ElementsMatch(t, expectedFields, actualFields, "metadata=%#v", metadata)
}

func getJSONMap(t *testing.T, endpoint string) map[string]any {
	t.Helper()
	statusCode, body, err := requestJSON(http.MethodGet, endpoint, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))
	return decodeMap(t, body)
}

func sendJSONExpectingStatus(t *testing.T, method string, endpoint string, payload map[string]any, expectedStatus int) {
	t.Helper()
	statusCode, body, err := requestJSON(method, endpoint, payload)
	require.NoError(t, err)
	require.Equal(t, expectedStatus, statusCode, "%s %s payload=%#v response=%s", method, endpoint, payload, string(body))
}

func patchMetadata(t *testing.T, endpoint string, payload map[string]any, expectedStatus int) {
	t.Helper()
	sendJSONExpectingStatus(t, http.MethodPatch, endpoint+"/$metadata", payload, expectedStatus)
}

func requireListMemberCount(t *testing.T, listURL string, expectedCount int) {
	t.Helper()
	members, ok := getJSONMap(t, listURL)["value"].([]any)
	require.True(t, ok, "list value must be an array")
	require.Len(t, members, expectedCount)
}

func TestSubmodelElementMetadataContainsOnlyMetadataFields(t *testing.T) {
	_, elementsURL := createMetadataTestSubmodel(t, submodelRepositoryBaseURL, "sme-metadata-fields", metadataTestSubmodelPayload)

	t.Run("by path", func(t *testing.T) {
		for idShort, expectedFields := range expectedSubmodelElementMetadataFields {
			requireMetadataFields(t, getJSONMap(t, elementsURL+"/"+idShort+"/$metadata"), expectedFields)
		}
	})

	t.Run("nested element by path", func(t *testing.T) {
		requireMetadataFields(t, getJSONMap(t, elementsURL+"/Housing.Color/$metadata"), []string{"idShort", "modelType", "valueType"})
	})

	t.Run("all elements omit child elements", func(t *testing.T) {
		page := getJSONMap(t, elementsURL+"/$metadata?limit=500")
		result, ok := page["result"].([]any)
		require.True(t, ok, "result must be an array: %#v", page)
		require.Len(t, result, len(expectedSubmodelElementMetadataFields))

		for _, rawMetadata := range result {
			metadata, isMap := rawMetadata.(map[string]any)
			require.True(t, isMap, "metadata entry must be an object")
			idShort, _ := metadata["idShort"].(string)
			expectedFields, known := expectedSubmodelElementMetadataFields[idShort]
			require.True(t, known, "unexpected metadata entry %#v", metadata)
			requireMetadataFields(t, metadata, expectedFields)
		}
	})
}

func TestPatchSubmodelElementMetadataTypeSpecificFields(t *testing.T) {
	_, elementsURL := createMetadataTestSubmodel(t, submodelRepositoryBaseURL, "sme-metadata-patch", metadataTestSubmodelPayload)
	temperatureURL := elementsURL + "/Temperature"
	readingsURL := elementsURL + "/Readings"
	eventURL := elementsURL + "/Event"

	t.Run("accepts the valueType returned by GET and keeps the value", func(t *testing.T) {
		patchMetadata(t, temperatureURL, map[string]any{
			"modelType":   "Property",
			"idShort":     "Temperature",
			"valueType":   "xs:int",
			"description": []map[string]any{{"language": "en", "text": "Heat"}},
		}, http.StatusNoContent)

		element := getJSONMap(t, temperatureURL)
		require.Equal(t, "20", element["value"])
		description, ok := element["description"].([]any)
		require.True(t, ok, "description must be an array: %#v", element)
		require.Equal(t, "Heat", description[0].(map[string]any)["text"])
	})

	t.Run("changes valueType when the stored value stays consistent", func(t *testing.T) {
		patchMetadata(t, temperatureURL, map[string]any{"modelType": "Property", "valueType": "xs:long"}, http.StatusNoContent)

		element := getJSONMap(t, temperatureURL)
		require.Equal(t, "xs:long", element["valueType"])
		require.Equal(t, "20", element["value"])
	})

	t.Run("accepts submodel element list metadata fields and keeps children", func(t *testing.T) {
		patchMetadata(t, readingsURL, map[string]any{
			"modelType":             "SubmodelElementList",
			"orderRelevant":         false,
			"semanticIdListElement": metadataTestExternalReference("urn:example:semantic:reading"),
		}, http.StatusNoContent)

		metadata := getJSONMap(t, readingsURL+"/$metadata")
		require.Equal(t, false, metadata["orderRelevant"])
		require.Contains(t, metadata, "semanticIdListElement")
		children, ok := getJSONMap(t, readingsURL)["value"].([]any)
		require.True(t, ok, "list value must be an array")
		require.Len(t, children, 2)
	})

	t.Run("accepts basic event element metadata fields and keeps observed", func(t *testing.T) {
		patchMetadata(t, eventURL, map[string]any{
			"modelType":    "BasicEventElement",
			"state":        "off",
			"messageTopic": "motor/alerts",
		}, http.StatusNoContent)

		metadata := getJSONMap(t, eventURL+"/$metadata")
		require.Equal(t, "off", metadata["state"])
		require.Equal(t, "motor/alerts", metadata["messageTopic"])
		require.Contains(t, getJSONMap(t, eventURL), "observed")
	})

	t.Run("rejects fields outside the metadata representation", func(t *testing.T) {
		testCases := []struct {
			idShort string
			payload map[string]any
		}{
			{idShort: "Temperature", payload: map[string]any{"modelType": "Property", "value": "21"}},
			{idShort: "Temperature", payload: map[string]any{"modelType": "Property", "valueId": metadataTestExternalReference("urn:example:value:other")}},
			{idShort: "Label", payload: map[string]any{"modelType": "MultiLanguageProperty", "valueType": "xs:string"}},
			{idShort: "Firmware", payload: map[string]any{"modelType": "Blob", "contentType": "text/plain"}},
			{idShort: "Motor", payload: map[string]any{"modelType": "Entity", "entityType": "CoManagedEntity"}},
			{idShort: "Event", payload: map[string]any{"modelType": "BasicEventElement", "observed": metadataTestExternalReference("urn:example:observed")}},
			{idShort: "Calibrate", payload: map[string]any{"modelType": "Operation", "inputVariables": []any{}}},
			{idShort: "Housing", payload: map[string]any{"modelType": "SubmodelElementCollection", "value": []any{}}},
		}
		for _, testCase := range testCases {
			patchMetadata(t, elementsURL+"/"+testCase.idShort, testCase.payload, http.StatusBadRequest)
		}

		require.Equal(t, "20", getJSONMap(t, temperatureURL)["value"])
	})

	t.Run("rejects null for type-specific fields", func(t *testing.T) {
		patchMetadata(t, eventURL, map[string]any{"modelType": "BasicEventElement", "messageTopic": nil}, http.StatusBadRequest)

		require.Equal(t, "motor/alerts", getJSONMap(t, eventURL+"/$metadata")["messageTopic"])
	})
}

func TestSubmodelElementWritesStrictVerification(t *testing.T) {
	_, elementsURL := createMetadataTestSubmodel(t, submodelRepositoryStrictBaseURL, "sme-metadata-strict", metadataStrictTestSubmodelPayload)
	temperatureURL := elementsURL + "/Temperature"
	readingsURL := elementsURL + "/Readings"
	firstReadingURL := readingsURL + "[0]"

	t.Run("accepts a valueType consistent with the stored value", func(t *testing.T) {
		patchMetadata(t, temperatureURL, map[string]any{"modelType": "Property", "valueType": "xs:long"}, http.StatusNoContent)

		require.Equal(t, "xs:long", getJSONMap(t, temperatureURL+"/$metadata")["valueType"])
	})

	t.Run("rejects a valueType inconsistent with the stored value", func(t *testing.T) {
		patchMetadata(t, temperatureURL, map[string]any{"modelType": "Property", "valueType": "xs:boolean"}, http.StatusBadRequest)

		element := getJSONMap(t, temperatureURL)
		require.Equal(t, "xs:long", element["valueType"])
		require.Equal(t, "20", element["value"])
	})

	t.Run("rejects a list member valueType that conflicts with the enclosing list", func(t *testing.T) {
		patchMetadata(t, firstReadingURL, map[string]any{"modelType": "Property", "valueType": "xs:long"}, http.StatusBadRequest)

		element := getJSONMap(t, firstReadingURL)
		require.Equal(t, "xs:int", element["valueType"])
		require.Equal(t, "1", element["value"])
	})

	t.Run("rejects a list value type that conflicts with the list members", func(t *testing.T) {
		patchMetadata(t, readingsURL, map[string]any{"modelType": "SubmodelElementList", "valueTypeListElement": "xs:string"}, http.StatusBadRequest)

		require.Equal(t, "xs:int", getJSONMap(t, readingsURL+"/$metadata")["valueTypeListElement"])
	})

	t.Run("rejects a regular PATCH of a list member that conflicts with the enclosing list", func(t *testing.T) {
		sendJSONExpectingStatus(t, http.MethodPatch, firstReadingURL, map[string]any{"modelType": "Property", "valueType": "xs:long"}, http.StatusBadRequest)

		require.Equal(t, "xs:int", getJSONMap(t, firstReadingURL)["valueType"])
	})

	t.Run("rejects a regular PATCH of a list that conflicts with its members", func(t *testing.T) {
		sendJSONExpectingStatus(t, http.MethodPatch, readingsURL, map[string]any{
			"modelType":            "SubmodelElementList",
			"typeValueListElement": "Property",
			"valueTypeListElement": "xs:string",
		}, http.StatusBadRequest)

		require.Equal(t, "xs:int", getJSONMap(t, readingsURL+"/$metadata")["valueTypeListElement"])
	})

	t.Run("rejects a PUT of a list member that conflicts with the enclosing list", func(t *testing.T) {
		sendJSONExpectingStatus(t, http.MethodPut, firstReadingURL, metadataTestProperty("", "xs:long", "1"), http.StatusBadRequest)

		element := getJSONMap(t, firstReadingURL)
		require.Equal(t, "xs:int", element["valueType"])
		require.Equal(t, "1", element["value"])
	})

	t.Run("rejects a POST of a list member that conflicts with the list", func(t *testing.T) {
		sendJSONExpectingStatus(t, http.MethodPost, readingsURL, metadataTestProperty("", "xs:long", "3"), http.StatusBadRequest)

		requireListMemberCount(t, readingsURL, 2)
	})

	t.Run("accepts writes of list members that match the list", func(t *testing.T) {
		sendJSONExpectingStatus(t, http.MethodPost, readingsURL, metadataTestProperty("", "xs:int", "3"), http.StatusCreated)
		sendJSONExpectingStatus(t, http.MethodPut, firstReadingURL, metadataTestProperty("", "xs:int", "10"), http.StatusNoContent)

		requireListMemberCount(t, readingsURL, 3)
		require.Equal(t, "10", getJSONMap(t, firstReadingURL)["value"])
	})
}

const concurrentListCount = 20

func concurrentListTestSubmodelPayload(submodelID string) map[string]any {
	lists := make([]any, 0, concurrentListCount)
	for index := range concurrentListCount {
		lists = append(lists, map[string]any{
			"idShort":              fmt.Sprintf("List%d", index),
			"modelType":            "SubmodelElementList",
			"typeValueListElement": "Property",
			"valueTypeListElement": "xs:int",
			"value":                []any{metadataTestProperty("", "xs:int", "1")},
		})
	}
	return map[string]any{
		"id":               submodelID,
		"idShort":          "ConcurrentListWrites",
		"kind":             "Instance",
		"modelType":        "Submodel",
		"submodelElements": lists,
	}
}

func TestConcurrentListAndMemberWritesKeepListsValid(t *testing.T) {
	_, elementsURL := createMetadataTestSubmodel(t, submodelRepositoryStrictBaseURL, "sme-list-concurrency", concurrentListTestSubmodelPayload)
	listPatch := map[string]any{"modelType": "SubmodelElementList", "semanticIdListElement": metadataTestExternalReference("urn:example:semantic:list")}
	memberPatch := map[string]any{"modelType": "Property", "semanticId": metadataTestExternalReference("urn:example:semantic:member")}

	var writes sync.WaitGroup
	for index := range concurrentListCount {
		listURL := fmt.Sprintf("%s/List%d", elementsURL, index)
		writes.Go(func() { _, _, _ = requestJSON(http.MethodPatch, listURL+"/$metadata", listPatch) })
		writes.Go(func() { _, _, _ = requestJSON(http.MethodPatch, listURL+"[0]/$metadata", memberPatch) })
	}
	writes.Wait()

	for index := range concurrentListCount {
		list := getJSONMap(t, fmt.Sprintf("%s/List%d", elementsURL, index))
		members, ok := list["value"].([]any)
		require.True(t, ok, "list value must be an array: %#v", list)
		require.Len(t, members, 1)
		member, ok := members[0].(map[string]any)
		require.True(t, ok, "list member must be an object")

		_, listHasSemanticID := list["semanticIdListElement"]
		_, memberHasSemanticID := member["semanticId"]
		require.False(t, listHasSemanticID && memberHasSemanticID, "list %d violates AASd-107: %#v", index, list)
		require.True(t, listHasSemanticID || memberHasSemanticID, "list %d lost both writes: %#v", index, list)
	}
}

func TestConcurrentListChildWritesDoNotDeadlock(t *testing.T) {
	_, elementsURL := createMetadataTestSubmodel(t, submodelRepositoryStrictBaseURL, "sme-list-lock-order", concurrentListTestSubmodelPayload)
	listChildrenPatch := map[string]any{
		"modelType":            "SubmodelElementList",
		"typeValueListElement": "Property",
		"valueTypeListElement": "xs:int",
		"value":                []any{metadataTestProperty("", "xs:int", "5")},
	}

	type writeResult struct {
		request string
		status  int
		body    string
		err     error
	}
	results := make(chan writeResult, concurrentListCount*4)
	send := func(method string, endpoint string, payload map[string]any) {
		status, body, err := requestJSON(method, endpoint, payload)
		results <- writeResult{request: method + " " + endpoint, status: status, body: string(body), err: err}
	}

	var writes sync.WaitGroup
	for index := range concurrentListCount {
		listURL := fmt.Sprintf("%s/List%d", elementsURL, index)
		writes.Go(func() { send(http.MethodPatch, listURL, listChildrenPatch) })
		writes.Go(func() { send(http.MethodPost, listURL, metadataTestProperty("", "xs:int", "7")) })
		writes.Go(func() { send(http.MethodPut, listURL+"[0]", metadataTestProperty("", "xs:int", "9")) })
		writes.Go(func() {
			send(http.MethodPatch, listURL+"[0]/$metadata", map[string]any{"modelType": "Property", "description": []map[string]any{{"language": "en", "text": "member"}}})
		})
	}
	writes.Wait()
	close(results)

	for result := range results {
		require.NoError(t, result.err, result.request)
		require.Less(t, result.status, http.StatusInternalServerError, "%s returned %d: %s", result.request, result.status, result.body)
	}
}
