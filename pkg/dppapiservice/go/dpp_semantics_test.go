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

	"github.com/FriedJannik/aas-go-sdk/verification"
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

func TestSemanticIDsForSectionsUsesContentSpecificationIDSectionNames(t *testing.T) {
	carbonFootprintSemanticID := "https://admin-shell.io/idta/CarbonFootprint/CarbonFootprint/1/0"
	handoverDocumentationSemanticID := "https://admin-shell-io/idta/digitalproductpassport/HandoverDocumentation/2"
	sections := map[string]any{
		carbonFootprintSemanticID: map[string]any{
			"ProductCarbonFootprints": []any{
				map[string]any{"PcfCo2eq": "17.2"},
			},
		},
		handoverDocumentationSemanticID: map[string]any{
			"Documents": []any{
				map[string]any{"Version": "V1.2"},
			},
		},
	}

	semanticIDs, err := semanticIDsForSections(sections, []string{
		carbonFootprintSemanticID,
		handoverDocumentationSemanticID,
	})
	if err != nil {
		t.Fatalf("semanticIDsForSections() error = %v", err)
	}
	if semanticIDs[carbonFootprintSemanticID] != carbonFootprintSemanticID {
		t.Fatalf("carbonFootprint semantic ID = %q", semanticIDs[carbonFootprintSemanticID])
	}
	if semanticIDs[handoverDocumentationSemanticID] != handoverDocumentationSemanticID {
		t.Fatalf("handoverDocumentation semantic ID = %q", semanticIDs[handoverDocumentationSemanticID])
	}
}

func TestBuildContentSubmodelUsesSafeIDShortForContentSpecificationIDSectionName(t *testing.T) {
	semanticID := "https://admin-shell.io/idta/CarbonFootprint/CarbonFootprint/1/0"

	submodel, err := buildContentSubmodel("dpp-1", semanticID, semanticID, map[string]any{
		"PcfCo2eq": "17.2",
	})
	if err != nil {
		t.Fatalf("buildContentSubmodel() error = %v", err)
	}
	if submodel.IDShort() == nil {
		t.Fatal("submodel idShort = nil")
	}
	if *submodel.IDShort() != "CarbonFootprint" {
		t.Fatalf("submodel idShort = %q, want CarbonFootprint", *submodel.IDShort())
	}
	if referenceToString(submodel.SemanticID()) != semanticID {
		t.Fatalf("submodel semantic ID = %q, want %q", referenceToString(submodel.SemanticID()), semanticID)
	}
	verificationErrors := make([]string, 0)
	verification.VerifySubmodel(submodel, func(err *verification.VerificationError) bool {
		verificationErrors = append(verificationErrors, err.Error())
		return false
	})
	if len(verificationErrors) != 0 {
		t.Fatalf("VerifySubmodel() errors = %#v", verificationErrors)
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
