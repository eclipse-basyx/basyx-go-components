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
// Author: Jannik Fried ( Fraunhofer IESE )

package dppapi

import (
	"strings"
	"testing"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
)

const (
	filteringDPPID             = "https://example.org/dpp/filtering"
	filteringNameplateSemantic = "https://admin-shell.io/idta/nameplate/3/0/Nameplate"
	filteringTechnicalSemantic = "urn:example:semantic:technical-data"
)

func TestComposeResolvedDPPFiltersCompressedContentByContentSpecificationIDs(t *testing.T) {
	resolved := filteringResolvedDPP()

	doc, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	if err != nil {
		t.Fatalf("composeResolvedDPP() error = %v", err)
	}

	nameplate, ok := doc[filteringNameplateSemantic].(map[string]any)
	if !ok {
		t.Fatalf("%s section = %#v, want object", filteringNameplateSemantic, doc[filteringNameplateSemantic])
	}
	if nameplate["manufacturerName"] != "Acme GmbH" {
		t.Fatalf("%s.manufacturerName = %#v", filteringNameplateSemantic, nameplate["manufacturerName"])
	}
	if _, ok := doc["technicalData"]; ok {
		t.Fatalf("technicalData section was included despite missing contentSpecificationIds match: %#v", doc["technicalData"])
	}
}

func TestComposeResolvedDPPFiltersFullContentByContentSpecificationIDs(t *testing.T) {
	resolved := filteringResolvedDPP()

	doc, err := composeResolvedDPP(resolved, REPRESENTATION_FULL)
	if err != nil {
		t.Fatalf("composeResolvedDPP() error = %v", err)
	}

	elements, ok := doc["elements"].([]map[string]any)
	if !ok {
		t.Fatalf("elements = %#v, want DPP element array", doc["elements"])
	}
	if len(elements) != 1 {
		t.Fatalf("elements length = %d, want 1: %#v", len(elements), elements)
	}
	if elements[0]["elementId"] != "DigitalNameplate" {
		t.Fatalf("included elementId = %#v, want DigitalNameplate", elements[0]["elementId"])
	}
	if elements[0]["dictionaryReference"] != filteringNameplateSemantic {
		t.Fatalf("dictionaryReference = %#v, want %q", elements[0]["dictionaryReference"], filteringNameplateSemantic)
	}
}

func TestResolveDPPElementPathFiltersByContentSpecificationIDs(t *testing.T) {
	resolved := filteringResolvedDPP()

	submodelID, idShortPath, err := resolveDPPElementPath(resolved, "$['"+filteringNameplateSemantic+"']['manufacturerName']")
	if err != nil {
		t.Fatalf("resolveDPPElementPath() error = %v", err)
	}
	if submodelID != contentSubmodelID(filteringDPPID, "digitalNameplate") {
		t.Fatalf("submodelID = %q", submodelID)
	}
	if idShortPath != "manufacturerName" {
		t.Fatalf("idShortPath = %q", idShortPath)
	}

	_, _, err = resolveDPPElementPath(resolved, "$['"+filteringTechnicalSemantic+"']['internalNote']")
	if err == nil || !strings.Contains(err.Error(), "DPP-ELEMPATH-NOTFOUND") {
		t.Fatalf("resolveDPPElementPath() error = %v, want DPP-ELEMPATH-NOTFOUND", err)
	}
}

func TestComposeAndResolveDPPUseNewestContentSubmodelForSharedSemanticID(t *testing.T) {
	resolved := filteringResolvedDPP()
	older := filteringContentSubmodel("olderNameplate", "OlderNameplate", filteringNameplateSemantic, stringProperty("manufacturerName", "Older GmbH"))
	newer := filteringContentSubmodel("newerNameplate", "NewerNameplate", filteringNameplateSemantic, stringProperty("manufacturerName", "Newer GmbH"))
	setContentSubmodelUpdatedAt(older, "2026-06-22T12:00:00Z")
	setContentSubmodelUpdatedAt(newer, "2026-06-23T12:00:00Z")
	resolved.submodels = []types.ISubmodel{resolved.metadata, newer, older}

	compressed, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	if err != nil {
		t.Fatalf("composeResolvedDPP() compressed error = %v", err)
	}
	section := compressed[filteringNameplateSemantic].(map[string]any)
	if section["manufacturerName"] != "Newer GmbH" {
		t.Fatalf("compressed section = %#v, want newest submodel content", section)
	}

	full, err := composeResolvedDPP(resolved, REPRESENTATION_FULL)
	if err != nil {
		t.Fatalf("composeResolvedDPP() full error = %v", err)
	}
	elements := full["elements"].([]map[string]any)
	if len(elements) != 1 {
		t.Fatalf("full elements = %#v, want exactly one newest content submodel", elements)
	}
	fullElement := elements[0]["elements"].([]map[string]any)[0]
	if fullElement["value"] != "Newer GmbH" {
		t.Fatalf("full element = %#v, want newest submodel content", fullElement)
	}

	submodelID, _, err := resolveDPPElementPath(resolved, "$['"+filteringNameplateSemantic+"']['manufacturerName']")
	if err != nil {
		t.Fatalf("resolveDPPElementPath() error = %v", err)
	}
	if submodelID != newer.ID() {
		t.Fatalf("resolved submodel ID = %q, want newest submodel %q", submodelID, newer.ID())
	}
}

func TestResolveDPPElementPathRejectsIDShortAliasForSemanticContentSection(t *testing.T) {
	_, _, err := resolveDPPElementPath(filteringResolvedDPP(), "$['digitalNameplate']['manufacturerName']")
	if err == nil || !strings.Contains(err.Error(), "DPP-ELEMPATH-NOTFOUND") {
		t.Fatalf("resolveDPPElementPath() error = %v, want DPP-ELEMPATH-NOTFOUND", err)
	}
}

func TestResolveDPPElementPathRejectsLegacySectionPath(t *testing.T) {
	_, _, err := resolveDPPElementPath(filteringResolvedDPP(), "digitalNameplate/manufacturerName")
	if err == nil {
		t.Fatal("resolveDPPElementPath() error = nil, want invalid legacy path")
	}
	if !strings.Contains(err.Error(), "DPP-ELEMPATH-INVALID") {
		t.Fatalf("resolveDPPElementPath() error = %v, want DPP-ELEMPATH-INVALID", err)
	}
}

func TestResolveDPPElementPathSupportsContentSpecificationIDSectionNames(t *testing.T) {
	semanticID := "https://admin-shell.io/idta/CarbonFootprint/CarbonFootprint/1/0"
	header := dppHeader{
		DigitalProductPassportID: filteringDPPID,
		UniqueProductIdentifier:  "https://example.org/products/filtering",
		Granularity:              "Item",
		DppSchemaVersion:         "1.0.0",
		DppStatus:                "active",
		LastUpdate:               time.Date(2026, time.June, 23, 12, 0, 0, 0, time.UTC),
		EconomicOperatorID:       "operator-123",
		FacilityID:               "facility-456",
		ContentSpecificationIDs:  []string{semanticID},
	}
	metadata := buildMetadataSubmodel(filteringDPPID, header)
	content := filteringContentSubmodel(semanticID, contentSectionIDShort(semanticID), semanticID, stringProperty("ProductCarbonFootprints", "17.2"))
	resolved := resolvedDPP{
		metadata:  metadata,
		submodels: []types.ISubmodel{metadata, content},
	}
	if idShort := idShortOrID(content); idShort == semanticID {
		t.Fatalf("content submodel idShort = %q, want AAS-safe idShort", idShort)
	}

	submodelID, idShortPath, err := resolveDPPElementPath(resolved, "$['"+semanticID+"']['ProductCarbonFootprints']")
	if err != nil {
		t.Fatalf("resolveDPPElementPath() error = %v", err)
	}
	if submodelID != contentSubmodelID(filteringDPPID, semanticID) {
		t.Fatalf("submodelID = %q", submodelID)
	}
	if idShortPath != "ProductCarbonFootprints" {
		t.Fatalf("idShortPath = %q", idShortPath)
	}
}

func TestResolveDPPElementPathConvertsJSONPathIndexes(t *testing.T) {
	semanticID := "https://admin-shell.io/idta/CarbonFootprint/CarbonFootprint/1/0"
	header := dppHeader{
		DigitalProductPassportID: filteringDPPID,
		UniqueProductIdentifier:  "https://example.org/products/filtering",
		Granularity:              "Item",
		DppSchemaVersion:         "1.0.0",
		DppStatus:                "active",
		LastUpdate:               time.Date(2026, time.June, 23, 12, 0, 0, 0, time.UTC),
		EconomicOperatorID:       "operator-123",
		FacilityID:               "facility-456",
		ContentSpecificationIDs:  []string{semanticID},
	}
	metadata := buildMetadataSubmodel(filteringDPPID, header)
	content := filteringContentSubmodel(semanticID, contentSectionIDShort(semanticID), semanticID, stringProperty("ProductCarbonFootprints", "17.2"))
	resolved := resolvedDPP{
		metadata:  metadata,
		submodels: []types.ISubmodel{metadata, content},
	}

	submodelID, idShortPath, err := resolveDPPElementPath(resolved, "$['"+semanticID+"']['ProductCarbonFootprints'][0]['PcfCo2eq']")
	if err != nil {
		t.Fatalf("resolveDPPElementPath() error = %v", err)
	}
	if submodelID != contentSubmodelID(filteringDPPID, semanticID) {
		t.Fatalf("submodelID = %q", submodelID)
	}
	if idShortPath != "ProductCarbonFootprints[0].PcfCo2eq" {
		t.Fatalf("idShortPath = %q", idShortPath)
	}
}

func TestParseDPPJSONElementPathAcceptsRFC9535NormalizedPath(t *testing.T) {
	parsed, err := parseDPPJSONElementPath(`$['sec\u0000tion']['line\nfeed'][0]['quote\'and\\slash']`)
	if err != nil {
		t.Fatalf("parseDPPJSONElementPath() error = %v", err)
	}
	if parsed.sectionName != "sec\x00tion" {
		t.Fatalf("sectionName = %q, want escaped control character", parsed.sectionName)
	}
	if parsed.idShortPath != "line\nfeed[0].quote'and\\slash" {
		t.Fatalf("idShortPath = %q", parsed.idShortPath)
	}
}

func TestParseDPPJSONElementPathRejectsNonNormalizedPath(t *testing.T) {
	paths := []string{
		`$.section.property`,
		`$["section"]["property"]`,
		`$['section']['bad\q']`,
		`$['section']['bad\u0061']`,
		`$['section']['bad\/slash']`,
		`$['section'][01]`,
		`$['section'][-1]`,
		`$['section'][*]`,
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			_, err := parseDPPJSONElementPath(path)
			if err == nil || !strings.Contains(err.Error(), "DPP-ELEMPATH-INVALID") {
				t.Fatalf("parseDPPJSONElementPath(%q) error = %v, want DPP-ELEMPATH-INVALID", path, err)
			}
		})
	}
}

func TestResolveDPPElementPathRejectsNonSingularJSONPath(t *testing.T) {
	_, _, err := resolveDPPElementPath(filteringResolvedDPP(), "$['https://admin-shell.io/idta/nameplate/3/0/Nameplate'][*]")
	if err == nil {
		t.Fatal("resolveDPPElementPath() error = nil, want invalid JSONPath")
	}
	if !strings.Contains(err.Error(), "DPP-ELEMPATH-INVALID") {
		t.Fatalf("resolveDPPElementPath() error = %v, want DPP-ELEMPATH-INVALID", err)
	}
}

func TestComposeResolvedDPPIncludesAllContentWhenContentSpecificationIDsEmpty(t *testing.T) {
	resolved := filteringResolvedDPP()
	resolved.metadata.SetSubmodelElements(replaceSubmodelElement(
		resolved.metadata.SubmodelElements(),
		headerContentSpecificationIDs,
		emptyStringList(headerContentSpecificationIDs),
	))

	doc, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	if err != nil {
		t.Fatalf("composeResolvedDPP() error = %v", err)
	}

	assertDPPContentSectionExists(t, doc, "digitalNameplate")
	assertDPPContentSectionExists(t, doc, "technicalData")
}

func TestComposeResolvedDPPIncludesAllContentWhenContentSpecificationIDsMissing(t *testing.T) {
	resolved := filteringResolvedDPP()
	resolved.metadata.SetSubmodelElements(withoutSubmodelElement(resolved.metadata.SubmodelElements(), headerContentSpecificationIDs))

	doc, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	if err != nil {
		t.Fatalf("composeResolvedDPP() error = %v", err)
	}

	assertDPPContentSectionExists(t, doc, "digitalNameplate")
	assertDPPContentSectionExists(t, doc, "technicalData")
}

func TestStaleContentSubmodelIDsUsesOnlySelectedCurrentContent(t *testing.T) {
	resolved := filteringResolvedDPP()
	currentContent, err := selectedResolvedContentSubmodels(resolved)
	if err != nil {
		t.Fatalf("selectedResolvedContentSubmodels() error = %v", err)
	}

	stale := staleContentSubmodelIDs(currentContent, []types.ISubmodel{resolved.metadata})
	if len(stale) != 1 {
		t.Fatalf("stale IDs = %#v, want only selected content submodel", stale)
	}
	if stale[0] != contentSubmodelID(filteringDPPID, "digitalNameplate") {
		t.Fatalf("stale[0] = %q", stale[0])
	}
}

func TestAppendUnselectedContentSubmodelReferencesPreservesBaseSubmodelRefs(t *testing.T) {
	resolved := filteringResolvedDPP()
	currentContent, err := selectedResolvedContentSubmodels(resolved)
	if err != nil {
		t.Fatalf("selectedResolvedContentSubmodels() error = %v", err)
	}
	refs := []types.IReference{
		submodelReference(resolved.metadata.ID()),
		submodelReference(contentSubmodelID(filteringDPPID, "digitalNameplate")),
	}

	refs = appendUnselectedContentSubmodelReferences(refs, resolved, currentContent)
	refs = appendUnselectedContentSubmodelReferences(refs, resolved, currentContent)

	if len(refs) != 3 {
		t.Fatalf("refs length = %d, want metadata, selected, and one unselected ref: %#v", len(refs), refs)
	}
	if !referenceListContains(refs, contentSubmodelID(filteringDPPID, "technicalData")) {
		t.Fatalf("refs do not contain unselected technicalData submodel: %#v", refs)
	}
}

func filteringResolvedDPP() resolvedDPP {
	header := dppHeader{
		DigitalProductPassportID: filteringDPPID,
		UniqueProductIdentifier:  "https://example.org/products/filtering",
		Granularity:              "Item",
		DppSchemaVersion:         "1.0.0",
		DppStatus:                "active",
		LastUpdate:               time.Date(2026, time.June, 23, 12, 0, 0, 0, time.UTC),
		EconomicOperatorID:       "operator-123",
		FacilityID:               "facility-456",
		ContentSpecificationIDs:  []string{filteringNameplateSemantic},
	}
	metadata := buildMetadataSubmodel(filteringDPPID, header)
	nameplate := filteringContentSubmodel("digitalNameplate", "DigitalNameplate", filteringNameplateSemantic, stringProperty("manufacturerName", "Acme GmbH"))
	technicalData := filteringContentSubmodel("technicalData", "TechnicalData", filteringTechnicalSemantic, stringProperty("internalNote", "not part of the DPP"))

	return resolvedDPP{
		metadata:  metadata,
		submodels: []types.ISubmodel{metadata, technicalData, nameplate},
	}
}

func filteringContentSubmodel(sectionName string, idShort string, semanticID string, elements ...types.ISubmodelElement) types.ISubmodel {
	submodel := types.NewSubmodel(contentSubmodelID(filteringDPPID, sectionName))
	submodel.SetIDShort(&idShort)
	submodel.SetSemanticID(globalReference(semanticID))
	submodel.SetSubmodelElements(elements)
	return submodel
}

func setContentSubmodelUpdatedAt(submodel types.ISubmodel, updatedAt string) {
	administration := types.NewAdministrativeInformation()
	administration.SetUpdatedAt(&updatedAt)
	submodel.SetAdministration(administration)
}

func assertDPPContentSectionExists(t *testing.T, doc dppDocument, sectionName string) {
	t.Helper()
	section, ok := doc[sectionName].(map[string]any)
	if !ok {
		t.Fatalf("%s section = %#v, want object", sectionName, doc[sectionName])
	}
	if len(section) == 0 {
		t.Fatalf("%s section is empty", sectionName)
	}
}

func withoutSubmodelElement(elements []types.ISubmodelElement, idShort string) []types.ISubmodelElement {
	filtered := make([]types.ISubmodelElement, 0, len(elements))
	for _, element := range elements {
		if element.IDShort() != nil && *element.IDShort() == idShort {
			continue
		}
		filtered = append(filtered, element)
	}
	return filtered
}

func replaceSubmodelElement(elements []types.ISubmodelElement, idShort string, replacement types.ISubmodelElement) []types.ISubmodelElement {
	replaced := make([]types.ISubmodelElement, 0, len(elements))
	for _, element := range elements {
		if element.IDShort() != nil && *element.IDShort() == idShort {
			replaced = append(replaced, replacement)
			continue
		}
		replaced = append(replaced, element)
	}
	return replaced
}

func emptyStringList(idShort string) types.ISubmodelElement {
	list := types.NewSubmodelElementList(types.AASSubmodelElementsProperty)
	list.SetIDShort(&idShort)
	valueType := types.DataTypeDefXSDString
	list.SetValueTypeListElement(&valueType)
	list.SetValue([]types.ISubmodelElement{})
	return list
}

func referenceListContains(refs []types.IReference, value string) bool {
	for _, ref := range refs {
		if referenceLastValue(ref) == value {
			return true
		}
	}
	return false
}
