/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software.
*
* SPDX-License-Identifier: MIT
******************************************************************************/
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

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

	nameplate, ok := doc["digitalNameplate"].(map[string]any)
	if !ok {
		t.Fatalf("digitalNameplate section = %#v, want object", doc["digitalNameplate"])
	}
	if nameplate["manufacturerName"] != "Acme GmbH" {
		t.Fatalf("digitalNamePlate.manufacturerName = %#v", nameplate["manufacturerName"])
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

	submodelID, idShortPath, err := resolveDPPElementPath(resolved, "digitalNameplate/manufacturerName")
	if err != nil {
		t.Fatalf("resolveDPPElementPath() error = %v", err)
	}
	if submodelID != contentSubmodelID(filteringDPPID, "digitalNameplate") {
		t.Fatalf("submodelID = %q", submodelID)
	}
	if idShortPath != "manufacturerName" {
		t.Fatalf("idShortPath = %q", idShortPath)
	}

	_, _, err = resolveDPPElementPath(resolved, "technicalData/internalNote")
	if err == nil || !strings.Contains(err.Error(), "DPP-ELEMPATH-NOTFOUND") {
		t.Fatalf("resolveDPPElementPath() error = %v, want DPP-ELEMPATH-NOTFOUND", err)
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
