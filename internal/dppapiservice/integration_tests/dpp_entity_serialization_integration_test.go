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

package integration_tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func testDPPEntitySerialization(
	t *testing.T,
	client *http.Client,
	baseURL, aasBaseURL string,
	databasePort int,
	idSuffix string,
	now time.Time,
) {
	t.Helper()
	dppID := "https://www.example.org/dpp/entity/" + idSuffix
	encodedDPPID := encodedPathParam(dppID)
	doJSON(
		t,
		client,
		http.MethodPost,
		baseURL+"/v1/dpps",
		lifecycleDPPDocument(dppID, "https://www.example.org/product/entity/"+idSuffix, now),
		http.StatusCreated,
	)
	t.Cleanup(func() {
		doJSON(t, client, http.MethodDelete, baseURL+"/v1/dpps/"+encodedDPPID, nil, http.StatusNoContent)
	})

	technicalDataID := submodelIDBySemanticID(t, databasePort, dppID, lifecycleTechnicalDataSpec)
	entity := entityIntegrationElement()
	doJSON(
		t,
		client,
		http.MethodPost,
		aasBaseURL+"/submodels/"+common.EncodeString(technicalDataID)+"/submodel-elements",
		entity,
		http.StatusCreated,
	)
	doJSON(
		t,
		client,
		http.MethodPost,
		aasBaseURL+"/submodels/"+common.EncodeString(technicalDataID)+"/submodel-elements",
		globalEntityIntegrationElement(idSuffix),
		http.StatusCreated,
	)
	attachmentURL := fmt.Sprintf(
		"%s/submodels/%s/submodel-elements/component.manual/attachment",
		aasBaseURL,
		common.EncodeString(technicalDataID),
	)
	attachmentBytes := []byte("managed Entity statement attachment")
	uploadAttachment(t, client, attachmentURL, "entity-manual.txt", attachmentBytes)

	compressed := doJSON(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID, nil, http.StatusOK)
	assertDPPSectionPathEquals(t, compressed, lifecycleTechnicalDataSpec, "component.entityType", "SelfManagedEntity")
	assertCompressedEntityIntegrationSpecificAssetID(t, compressed)
	assertDPPSectionPathEquals(t, compressed, lifecycleTechnicalDataSpec, "globalComponent.globalAssetId", "urn:example:asset:"+idSuffix)
	assertDPPSectionPathEquals(t, compressed, lifecycleTechnicalDataSpec, "component.statements.entityType", "statement value")
	assertDPPSectionPathEquals(t, compressed, lifecycleTechnicalDataSpec, "component.statements.manual.url", attachmentURL)
	assertCompressedEntityIntegrationName(t, compressed)

	full := doJSON(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID+"?representation=full", nil, http.StatusOK)
	entityElement := entityIntegrationElementByID(t, fullDPPSection(t, full, lifecycleTechnicalDataSpec), "component")
	assertEntityIntegrationValue(t, entityElement, "objectType", "DataElementCollection")
	entityElements := entityIntegrationElements(t, entityElement)
	assertEntityIntegrationValue(t, entityIntegrationElementByIDFromElements(t, entityElements, "entityType"), "value", "SelfManagedEntity")
	assertEntityIntegrationSpecificAssetID(t, entityElements)
	statements := entityIntegrationElementByIDFromElements(t, entityElements, "statements")
	statementElements := entityIntegrationElements(t, statements)
	assertEntityIntegrationValue(t, entityIntegrationElementByIDFromElements(t, statementElements, "entityType"), "value", "statement value")
	assertEntityIntegrationValue(t, entityIntegrationElementByIDFromElements(t, statementElements, "manual"), "url", attachmentURL)
	name := entityIntegrationElementByIDFromElements(t, statementElements, "name")
	nameValues, ok := name["value"].([]any)
	if !ok || len(nameValues) != 1 {
		t.Fatalf("name.value = %#v, want one language value", name["value"])
	}
	assertEntityIntegrationValue(t, nameValues[0].(map[string]any), "language", "en")
	assertEntityIntegrationValue(t, nameValues[0].(map[string]any), "value", "Component")
	globalEntity := entityIntegrationElementByID(t, fullDPPSection(t, full, lifecycleTechnicalDataSpec), "globalComponent")
	assertEntityIntegrationValue(
		t,
		entityIntegrationElementByIDFromElements(t, entityIntegrationElements(t, globalEntity), "globalAssetId"),
		"value",
		"urn:example:asset:"+idSuffix,
	)

	entityPath := encodedPathParam(dppElementJSONPath(lifecycleTechnicalDataSpec, "component"))
	fullEntity := doJSONAny(
		t,
		client,
		http.MethodGet,
		baseURL+"/v1/dpps/"+encodedDPPID+"/elements/"+entityPath+"?representation=full",
		nil,
		http.StatusOK,
	)
	assertDataElementObjectType(t, fullEntity, "component", "DataElementCollection")
	assertAttachmentDownload(t, client, attachmentURL, attachmentBytes)

	doJSON(
		t,
		client,
		http.MethodPost,
		aasBaseURL+"/submodels/"+common.EncodeString(technicalDataID)+"/submodel-elements",
		map[string]any{
			"modelType": "ReferenceElement",
			"idShort":   "unsupportedReference",
			"value": map[string]any{
				"type": "ExternalReference",
				"keys": []any{map[string]any{
					"type":  "GlobalReference",
					"value": "urn:example:unsupported-reference",
				}},
			},
		},
		http.StatusCreated,
	)
	doJSON(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID+"?representation=full", nil, http.StatusUnprocessableEntity)
	doJSON(
		t,
		client,
		http.MethodGet,
		baseURL+"/v1/dpps/"+encodedPathParam(dppID+"/missing")+"?representation=full",
		nil,
		http.StatusNotFound,
	)
}

func assertCompressedEntityIntegrationName(t *testing.T, dpp map[string]any) {
	t.Helper()
	section := dpp[lifecycleTechnicalDataSpec].(map[string]any)
	entity := section["component"].(map[string]any)
	statements := entity["statements"].(map[string]any)
	values, ok := statements["name"].([]any)
	if !ok || len(values) != 1 {
		t.Fatalf("component.statements.name = %#v, want one language value", statements["name"])
	}
	languageValue := values[0].(map[string]any)
	assertEntityIntegrationValue(t, languageValue, "language", "en")
	assertEntityIntegrationValue(t, languageValue, "value", "Component")
}

func assertCompressedEntityIntegrationSpecificAssetID(t *testing.T, dpp map[string]any) {
	t.Helper()
	section, ok := dpp[lifecycleTechnicalDataSpec].(map[string]any)
	if !ok {
		t.Fatalf("%s section = %#v, want object", lifecycleTechnicalDataSpec, dpp[lifecycleTechnicalDataSpec])
	}
	entity, ok := section["component"].(map[string]any)
	if !ok {
		t.Fatalf("component = %#v, want object", section["component"])
	}
	specificAssetIDs, ok := entity["specificAssetIds"].([]any)
	if !ok || len(specificAssetIDs) != 1 {
		t.Fatalf("component.specificAssetIds = %#v, want one item", entity["specificAssetIds"])
	}
	specificAssetID := specificAssetIDs[0].(map[string]any)
	externalSubjectID := specificAssetID["externalSubjectId"].(map[string]any)
	assertEntityIntegrationValue(t, externalSubjectID, "type", "ExternalReference")
	keys, ok := externalSubjectID["keys"].([]any)
	if !ok || len(keys) != 1 {
		t.Fatalf("externalSubjectId.keys = %#v, want one item", externalSubjectID["keys"])
	}
	key := keys[0].(map[string]any)
	assertEntityIntegrationValue(t, key, "type", "GlobalReference")
	assertEntityIntegrationValue(t, key, "value", "urn:example:subject:manufacturer")
}

func entityIntegrationElement() map[string]any {
	return map[string]any{
		"modelType":  "Entity",
		"idShort":    "component",
		"entityType": "SelfManagedEntity",
		"specificAssetIds": []any{map[string]any{
			"name":  "serialNumber",
			"value": "SN-42",
			"externalSubjectId": map[string]any{
				"type": "ExternalReference",
				"keys": []any{map[string]any{
					"type":  "GlobalReference",
					"value": "urn:example:subject:manufacturer",
				}},
			},
		}},
		"statements": []any{
			map[string]any{
				"modelType": "Property",
				"idShort":   "entityType",
				"valueType": "xs:string",
				"value":     "statement value",
			},
			map[string]any{
				"modelType": "MultiLanguageProperty",
				"idShort":   "name",
				"value": []any{map[string]any{
					"language": "en",
					"text":     "Component",
				}},
			},
			map[string]any{
				"modelType":   "File",
				"idShort":     "manual",
				"contentType": "text/plain",
				"value":       "https://example.test/entity-manual.txt",
			},
		},
	}
}

func globalEntityIntegrationElement(idSuffix string) map[string]any {
	return map[string]any{
		"modelType":     "Entity",
		"idShort":       "globalComponent",
		"entityType":    "SelfManagedEntity",
		"globalAssetId": "urn:example:asset:" + idSuffix,
	}
}

func assertEntityIntegrationSpecificAssetID(t *testing.T, entityElements []any) {
	t.Helper()
	specificAssetIDs := entityIntegrationElementByIDFromElements(t, entityElements, "specificAssetIds")
	specificAsset := entityIntegrationElementByIDFromElements(t, entityIntegrationElements(t, specificAssetIDs), "specificAssetId0")
	specificAssetElements := entityIntegrationElements(t, specificAsset)
	assertEntityIntegrationValue(t, entityIntegrationElementByIDFromElements(t, specificAssetElements, "name"), "value", "serialNumber")
	assertEntityIntegrationValue(t, entityIntegrationElementByIDFromElements(t, specificAssetElements, "value"), "value", "SN-42")
	externalSubjectID := entityIntegrationElementByIDFromElements(t, specificAssetElements, "externalSubjectId")
	referenceElements := entityIntegrationElements(t, externalSubjectID)
	assertEntityIntegrationValue(t, entityIntegrationElementByIDFromElements(t, referenceElements, "type"), "value", "ExternalReference")
	keys := entityIntegrationElementByIDFromElements(t, referenceElements, "keys")
	key := entityIntegrationElementByIDFromElements(t, entityIntegrationElements(t, keys), "key0")
	keyElements := entityIntegrationElements(t, key)
	assertEntityIntegrationValue(t, entityIntegrationElementByIDFromElements(t, keyElements, "type"), "value", "GlobalReference")
	assertEntityIntegrationValue(t, entityIntegrationElementByIDFromElements(t, keyElements, "value"), "value", "urn:example:subject:manufacturer")
}

func entityIntegrationElementByID(t *testing.T, collection map[string]any, elementID string) map[string]any {
	t.Helper()
	return entityIntegrationElementByIDFromElements(t, entityIntegrationElements(t, collection), elementID)
}

func entityIntegrationElementByIDFromElements(t *testing.T, elements []any, elementID string) map[string]any {
	t.Helper()
	if element, ok := findFullElement(elements, elementID); ok {
		return element
	}
	t.Fatalf("expanded elements do not contain elementId %q: %#v", elementID, elements)
	return nil
}

func entityIntegrationElements(t *testing.T, collection map[string]any) []any {
	t.Helper()
	elements, ok := collection["elements"].([]any)
	if !ok {
		t.Fatalf("%s.elements = %#v, want array", collection["elementId"], collection["elements"])
	}
	return elements
}

func assertEntityIntegrationValue(t *testing.T, element map[string]any, field string, expected any) {
	t.Helper()
	if element[field] != expected {
		t.Fatalf("%s.%s = %#v, want %#v", element["elementId"], field, element[field], expected)
	}
}
