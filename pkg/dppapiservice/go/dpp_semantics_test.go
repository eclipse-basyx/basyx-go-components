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
)

func TestSemanticIDsForSectionsDoesNotInferFromSectionNames(t *testing.T) {
	sections := map[string]any{
		"carbonFootprint": map[string]any{"pcfCo2eq": "17.2"},
		"technicalData":   map[string]any{"manufacturerName": "Acme GmbH"},
	}

	_, err := semanticIDsForSections(sections, []string{
		"technicalData - IDTA-02003-1-0",
		"urn:example:semantic:carbon-footprint",
	})
	if err == nil {
		t.Fatal("semanticIDsForSections() error = nil, want explicit mapping error")
	}
	if !strings.Contains(err.Error(), "DPP-SEMSPEC-EXPLICIT") {
		t.Fatalf("semanticIDsForSections() error = %v, want DPP-SEMSPEC-EXPLICIT", err)
	}
}

func TestSemanticIDsForSectionsUsesExpandedDictionaryReference(t *testing.T) {
	sections := map[string]any{
		"carbonFootprint": map[string]any{
			"elementId":           "CarbonFootprint",
			"objectType":          "DataElementCollection",
			"dictionaryReference": "urn:example:semantic:carbon-footprint",
			"value":               []any{},
		},
		"technicalData": map[string]any{
			"elementId":           "TechnicalData",
			"objectType":          "DataElementCollection",
			"dictionaryReference": "urn:example:semantic:technical-data",
			"value":               []any{},
		},
	}

	semanticIDs, err := semanticIDsForSections(sections, []string{
		"urn:example:semantic:technical-data",
		"urn:example:semantic:carbon-footprint",
	})
	if err != nil {
		t.Fatalf("semanticIDsForSections() error = %v", err)
	}
	if semanticIDs["carbonFootprint"] != "urn:example:semantic:carbon-footprint" {
		t.Fatalf("carbonFootprint semantic ID = %q", semanticIDs["carbonFootprint"])
	}
	if semanticIDs["technicalData"] != "urn:example:semantic:technical-data" {
		t.Fatalf("technicalData semantic ID = %q", semanticIDs["technicalData"])
	}
}

func TestSemanticIDsForSectionsAllowsSingleUnambiguousSection(t *testing.T) {
	semanticIDs, err := semanticIDsForSections(
		map[string]any{"technicalData": map[string]any{"manufacturerName": "Acme GmbH"}},
		[]string{"urn:example:semantic:technical-data"},
	)
	if err != nil {
		t.Fatalf("semanticIDsForSections() error = %v", err)
	}
	if semanticIDs["technicalData"] != "urn:example:semantic:technical-data" {
		t.Fatalf("technicalData semantic ID = %q", semanticIDs["technicalData"])
	}
}
