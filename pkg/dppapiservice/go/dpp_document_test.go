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
	"strings"
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

func TestDecodeDPPDocumentAcceptsOptionalHeaderFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "omitted",
			body: `{
				"digitalProductPassportId":"dpp-1",
				"uniqueProductIdentifier":"product-1",
				"granularity":"Item",
				"dppSchemaVersion":"EN18223:v1",
				"dppStatus":"Active",
				"lastUpdate":"2026-06-10T10:00:00Z",
				"economicOperatorId":"operator-1"
			}`,
		},
		{
			name: "empty content specification IDs",
			body: `{
				"digitalProductPassportId":"dpp-1",
				"uniqueProductIdentifier":"product-1",
				"granularity":"Item",
				"dppSchemaVersion":"EN18223:v1",
				"dppStatus":"Active",
				"lastUpdate":"2026-06-10T10:00:00Z",
				"economicOperatorId":"operator-1",
				"contentSpecificationIds":[]
			}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, header, err := decodeDPPDocument([]byte(test.body), true)
			if err != nil {
				t.Fatalf("decodeDPPDocument() error = %v", err)
			}
			if header.FacilityID != "" {
				t.Fatalf("facility ID = %q, want empty", header.FacilityID)
			}
			if len(header.ContentSpecificationIDs) != 0 {
				t.Fatalf("content specification IDs = %#v, want none", header.ContentSpecificationIDs)
			}

			metadata := buildMetadataSubmodel(header.DigitalProductPassportID, header)
			composed, err := composeHeader(metadata)
			if err != nil {
				t.Fatalf("composeHeader() error = %v", err)
			}
			if _, ok := composed[headerFacilityID]; ok {
				t.Fatalf("composed header unexpectedly contains %s: %#v", headerFacilityID, composed)
			}
			if _, ok := composed[headerContentSpecificationIDs]; ok {
				t.Fatalf("composed header unexpectedly contains %s: %#v", headerContentSpecificationIDs, composed)
			}
		})
	}
}

func TestDecodeDPPDocumentRejectsInvalidOptionalHeaderValues(t *testing.T) {
	validHeader := `{
		"digitalProductPassportId":"dpp-1",
		"uniqueProductIdentifier":"product-1",
		"granularity":"Item",
		"dppSchemaVersion":"EN18223:v1",
		"dppStatus":"Active",
		"lastUpdate":"2026-06-10T10:00:00Z",
		"economicOperatorId":"operator-1"`
	tests := []struct {
		name  string
		field string
	}{
		{name: "blank facility ID", field: `,"facilityId":""`},
		{name: "blank content specification ID", field: `,"contentSpecificationIds":[""]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := decodeDPPDocument([]byte(validHeader+test.field+`}`), true)
			if err == nil || !strings.Contains(err.Error(), "DPP-HEADER-INVALID") {
				t.Fatalf("decodeDPPDocument() error = %v, want DPP-HEADER-INVALID", err)
			}
		})
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

func TestDecodeDPPDocumentRejectsExpandedPayloads(t *testing.T) {
	body := []byte(`{
		"digitalProductPassportId":"dpp-1",
		"uniqueProductIdentifier":"product-1",
		"granularity":"Item",
		"dppSchemaVersion":"EN18223:v1",
		"dppStatus":"Active",
		"lastUpdate":"2026-06-10T10:00:00Z",
		"economicOperatorId":"operator-1",
		"facilityId":"facility-1",
		"contentSpecificationIds":["technicalData-specification"],
		"elements":[{
			"elementId":"technicalData",
			"objectType":"DataElementCollection",
			"elements":[]
		}]
	}`)

	_, _, err := decodeDPPDocument(body, true)
	if err == nil {
		t.Fatal("decodeDPPDocument() error = nil, want expanded payload rejection")
	}
	if !strings.Contains(err.Error(), "DPP-COMPACT-FULLWRITE") {
		t.Fatalf("decodeDPPDocument() error = %v, want DPP-COMPACT-FULLWRITE", err)
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
	manual.SetExtensions([]types.IExtension{
		stringExtension(dppResourceTitleExtensionName, "User Manual"),
		stringExtension(dppLanguageExtensionName, "en-GB"),
	})

	documentation := types.NewSubmodelElementCollection()
	documentationIDShort := "documentation"
	documentation.SetIDShort(&documentationIDShort)
	documentation.SetValue([]types.ISubmodelElement{name, manual})

	packageItem := types.NewSubmodelElementCollection()
	packageItemIDShort := "packages0"
	packageItem.SetIDShort(&packageItemIDShort)
	packageItem.SetValue([]types.ISubmodelElement{stringProperty("material", "steel")})

	packages := types.NewSubmodelElementList(types.AASSubmodelElementsSubmodelElementCollection)
	packagesIDShort := "packages"
	packages.SetIDShort(&packagesIDShort)
	packages.SetValue([]types.ISubmodelElement{packageItem})

	submodel := types.NewSubmodel("technical-data")
	submodelIDShort := "TechnicalData"
	submodel.SetIDShort(&submodelIDShort)
	submodel.SetSubmodelElements([]types.ISubmodelElement{
		manufacturerName,
		energyClasses,
		packages,
		documentation,
	})

	content, err := fullContent(submodel)
	if err != nil {
		t.Fatalf("fullContent() error = %v", err)
	}

	root := content.(map[string]any)
	assertMapValue(t, root, "objectType", "DataElementCollection")
	assertMapValue(t, root, "elementId", "TechnicalData")

	elements := root["elements"].([]map[string]any)
	assertMapValue(t, elements[0], "objectType", "SingleValuedDataElement")
	assertMapValue(t, elements[0], "elementId", "manufacturerName")
	assertMapValue(t, elements[0], "valueDataType", "xsd:string")
	assertMapValue(t, elements[0], "value", "Acme GmbH")

	assertMapValue(t, elements[1], "objectType", "MultiValuedDataElement")
	assertMapValue(t, elements[1], "elementId", "energyClasses")
	assertMapValue(t, elements[1], "valueDataType", "xsd:string")
	energyClassElements := elements[1]["elements"].([]map[string]any)
	assertMapValue(t, energyClassElements[0], "elementId", "energyClasses0")
	assertMapValue(t, energyClassElements[0], "value", "A")
	assertMapValue(t, energyClassElements[1], "elementId", "energyClasses1")
	assertMapValue(t, energyClassElements[1], "value", "B")

	assertMapValue(t, elements[2], "objectType", "MultiValuedDataElement")
	assertMapValue(t, elements[2], "elementId", "packages")
	assertMapMissing(t, elements[2], "valueDataType")
	packageElements := elements[2]["elements"].([]map[string]any)
	assertMapValue(t, packageElements[0], "objectType", "DataElementCollection")

	assertMapValue(t, elements[3], "objectType", "DataElementCollection")
	nested := elements[3]["elements"].([]map[string]any)
	assertMapValue(t, nested[0], "objectType", "MultiLanguageDataElement")
	assertMapValue(t, nested[1], "objectType", "RelatedResource")
	assertMapValue(t, nested[1], "contentType", "application/pdf")
	assertMapValue(t, nested[1], "url", "https://example.test/manual.pdf")
	assertMapValue(t, nested[1], "resourceTitle", "User Manual")
	assertMapValue(t, nested[1], "language", "en-GB")
}

func TestCompressedContentEnrichesRelatedResourceMetadata(t *testing.T) {
	manual := types.NewFile()
	manualIDShort := "manual"
	manualURL := "https://example.test/manual.pdf"
	manualContentType := "application/pdf"
	manual.SetIDShort(&manualIDShort)
	manual.SetValue(&manualURL)
	manual.SetContentType(&manualContentType)
	manual.SetExtensions([]types.IExtension{
		stringExtension(dppResourceTitleExtensionName, "User Manual"),
		stringExtension(dppLanguageExtensionName, "en-GB"),
	})

	submodel := types.NewSubmodel("technical-data")
	submodel.SetSubmodelElements([]types.ISubmodelElement{manual})

	content, err := compressedContent(submodel)
	if err != nil {
		t.Fatalf("compressedContent() error = %v", err)
	}
	manualValue := content.(map[string]any)["manual"].(map[string]any)
	assertMapValue(t, manualValue, "url", "https://example.test/manual.pdf")
	assertMapValue(t, manualValue, "contentType", "application/pdf")
	assertMapValue(t, manualValue, "resourceTitle", "User Manual")
	assertMapValue(t, manualValue, "language", "en-GB")
}

func assertMapValue(t *testing.T, value map[string]any, key string, expected any) {
	t.Helper()
	if value[key] != expected {
		t.Fatalf("%s = %#v, want %#v in %#v", key, value[key], expected, value)
	}
}

func assertMapMissing(t *testing.T, value map[string]any, key string) {
	t.Helper()
	if _, ok := value[key]; ok {
		t.Fatalf("%s unexpectedly present in %#v", key, value)
	}
}
