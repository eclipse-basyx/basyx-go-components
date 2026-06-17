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

package dppapi

import (
	"encoding/json"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
)

func TestDecodeDPPDocumentPreservesContentSections(t *testing.T) {
	body := []byte(`{
		"digitalProductPassportId":"dpp-1",
		"uniqueProductIdentifier":"product-1",
		"granularity":"Item",
		"dppSchemaVersion":"EN18223:v1",
		"dppStatus":"Active",
		"lastUpdate":"2026-06-10T10:00:00Z",
		"economicOperatorId":"operator-1",
		"facilityId":"facility-1",
		"contentSpecificationIds":["carbonFootprint - IDTA-02023-1-0"],
		"carbonFootprint":{"PcfCo2eq":17.2}
	}`)

	doc, header, err := decodeDPPDocument(body, true)
	if err != nil {
		t.Fatalf("decodeDPPDocument() error = %v", err)
	}
	if header.DigitalProductPassportID != "dpp-1" {
		t.Fatalf("header DPP ID = %q", header.DigitalProductPassportID)
	}
	sections := contentSections(doc)
	if _, ok := sections["carbonFootprint"]; !ok {
		t.Fatalf("content section was not preserved: %#v", sections)
	}
}

func TestApplyMergePatchRemovesAndUpdatesFields(t *testing.T) {
	target := map[string]any{
		"dppStatus": "Active",
		"content": map[string]any{
			"old":  "value",
			"keep": "value",
		},
	}
	patch := map[string]any{
		"dppStatus": "Deprecated",
		"content": map[string]any{
			"old": nil,
			"new": "value",
		},
	}

	merged := applyMergePatch(target, patch).(map[string]any)
	content := merged["content"].(map[string]any)
	if merged["dppStatus"] != "Deprecated" {
		t.Fatalf("dppStatus = %v", merged["dppStatus"])
	}
	if _, ok := content["old"]; ok {
		t.Fatalf("old field still exists: %#v", content)
	}
	if content["new"] != "value" || content["keep"] != "value" {
		t.Fatalf("unexpected content merge result: %#v", content)
	}
}

func TestNormalizeValueOnlyConvertsFileAndLanguageShapes(t *testing.T) {
	raw := []byte(`{
		"title":[{"en":"Manual"}],
		"document":{"contentType":"application/pdf","value":"https://example.test/manual.pdf"}
	}`)
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}

	normalizeValueOnly(value)
	normalized := value.(map[string]any)
	title := normalized["title"].([]any)[0].(map[string]any)
	document := normalized["document"].(map[string]any)

	if title["language"] != "en" || title["value"] != "Manual" {
		t.Fatalf("unexpected language value: %#v", title)
	}
	if document["url"] != "https://example.test/manual.pdf" {
		t.Fatalf("file value was not converted to url: %#v", document)
	}
	if _, ok := document["value"]; ok {
		t.Fatalf("file value field still exists: %#v", document)
	}
}

func TestFullContentMapsAASElementsToDPPDataElements(t *testing.T) {
	manufacturerName := types.NewProperty(types.DataTypeDefXSDString)
	manufacturerNameIDShort := "manufacturerName"
	manufacturerNameValue := "Acme GmbH"
	manufacturerName.SetIDShort(&manufacturerNameIDShort)
	manufacturerName.SetValue(&manufacturerNameValue)

	energyClasses := types.NewSubmodelElementList(types.AASSubmodelElementsProperty)
	energyClassesIDShort := "energyClasses"
	energyClasses.SetIDShort(&energyClassesIDShort)
	energyClassesValueType := types.DataTypeDefXSDString
	energyClasses.SetValueTypeListElement(&energyClassesValueType)
	energyClasses.SetValue([]types.ISubmodelElement{
		stringProperty("", "A"),
		stringProperty("", "B"),
	})

	name := types.NewMultiLanguageProperty()
	nameIDShort := "name"
	name.SetIDShort(&nameIDShort)
	name.SetValue([]types.ILangStringTextType{
		types.NewLangStringTextType("en", "Manual"),
		types.NewLangStringTextType("de", "Handbuch"),
	})

	manual := types.NewFile()
	manualIDShort := "manual"
	manualURL := "https://example.test/manual.pdf"
	manualContentType := "application/pdf"
	manual.SetIDShort(&manualIDShort)
	manual.SetValue(&manualURL)
	manual.SetContentType(&manualContentType)

	documentation := types.NewSubmodelElementCollection()
	documentationIDShort := "documentation"
	documentation.SetIDShort(&documentationIDShort)
	documentation.SetValue([]types.ISubmodelElement{name, manual})

	submodel := types.NewSubmodel("technical-data")
	submodelIDShort := "TechnicalData"
	submodel.SetIDShort(&submodelIDShort)
	submodel.SetSubmodelElements([]types.ISubmodelElement{
		manufacturerName,
		energyClasses,
		documentation,
	})

	content, err := fullContent(submodel)
	if err != nil {
		t.Fatalf("fullContent() error = %v", err)
	}

	root := content.(map[string]any)
	assertMapValue(t, root, "objectType", "DataElementCollection")
	assertMapValue(t, root, "elementId", "TechnicalData")

	elements := root["value"].([]map[string]any)
	assertMapValue(t, elements[0], "objectType", "SingleValuedDataElement")
	assertMapValue(t, elements[0], "elementId", "manufacturerName")
	assertMapValue(t, elements[0], "valueDataType", "xsd:string")
	assertMapValue(t, elements[0], "value", "Acme GmbH")

	assertMapValue(t, elements[1], "objectType", "MultiValuedDataElement")
	assertMapValue(t, elements[1], "elementId", "energyClasses")
	assertMapValue(t, elements[1], "valueDataType", "xsd:string")
	assertSliceValue(t, elements[1]["value"], 0, "A")
	assertSliceValue(t, elements[1]["value"], 1, "B")

	assertMapValue(t, elements[2], "objectType", "DataElementCollection")
	nested := elements[2]["value"].([]map[string]any)
	assertMapValue(t, nested[0], "objectType", "MultiLanguageDataElement")
	assertMapValue(t, nested[1], "objectType", "RelatedResource")
	assertMapValue(t, nested[1], "contentType", "application/pdf")
	assertMapValue(t, nested[1], "url", "https://example.test/manual.pdf")
}

func assertMapValue(t *testing.T, value map[string]any, key string, expected any) {
	t.Helper()
	if value[key] != expected {
		t.Fatalf("%s = %#v, want %#v in %#v", key, value[key], expected, value)
	}
}

func assertSliceValue(t *testing.T, value any, index int, expected any) {
	t.Helper()
	items := value.([]any)
	if items[index] != expected {
		t.Fatalf("value[%d] = %#v, want %#v in %#v", index, items[index], expected, items)
	}
}
