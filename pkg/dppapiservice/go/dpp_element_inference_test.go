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
	"encoding/json"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
)

func TestInferElementMapsCompactShapes(t *testing.T) {
	element, err := inferElement("productDescription", []any{
		map[string]any{"language": "en-IE", "value": "One Thing"},
		map[string]any{"language": "es-ES", "value": "Una Cosa"},
	})
	if err != nil {
		t.Fatalf("inferElement() multilingual error = %v", err)
	}
	multiLanguage, ok := element.(*types.MultiLanguageProperty)
	if !ok {
		t.Fatalf("element type = %T, want MultiLanguageProperty", element)
	}
	if len(multiLanguage.Value()) != 2 || multiLanguage.Value()[0].Language() != "en-IE" {
		t.Fatalf("unexpected multilingual value: %#v", multiLanguage.Value())
	}

	element, err = inferElement("supportedVoltages", []any{json.Number("110"), json.Number("220.5")})
	if err != nil {
		t.Fatalf("inferElement() numeric list error = %v", err)
	}
	list, ok := element.(*types.SubmodelElementList)
	if !ok {
		t.Fatalf("element type = %T, want SubmodelElementList", element)
	}
	if list.TypeValueListElement() != types.AASSubmodelElementsProperty {
		t.Fatalf("list type = %v, want property", list.TypeValueListElement())
	}
	if list.ValueTypeListElement() == nil || *list.ValueTypeListElement() != types.DataTypeDefXSDDouble {
		t.Fatalf("list value type = %v, want double", list.ValueTypeListElement())
	}

	element, err = inferElement("manual", map[string]any{
		"url":           "https://example.test/manual.pdf",
		"contentType":   "application/pdf",
		"language":      "en-GB",
		"resourceTitle": "User Manual",
	})
	if err != nil {
		t.Fatalf("inferElement() related resource error = %v", err)
	}
	file, ok := element.(*types.File)
	if !ok {
		t.Fatalf("element type = %T, want File", element)
	}
	if extensionValue(file.Extensions(), dppLanguageExtensionName) != "en-GB" {
		t.Fatalf("language extension missing: %#v", file.Extensions())
	}
	if extensionValue(file.Extensions(), dppResourceTitleExtensionName) != "User Manual" {
		t.Fatalf("resource title extension missing: %#v", file.Extensions())
	}
}

func TestInferElementRejectsAmbiguousArrays(t *testing.T) {
	tests := []struct {
		name  string
		value []any
	}{
		{name: "empty", value: []any{}},
		{name: "mixed scalar", value: []any{"A", true}},
		{name: "mixed object and scalar", value: []any{map[string]any{"field": "value"}, "A"}},
		{name: "partial multilingual", value: []any{map[string]any{"language": "en-IE", "value": "One Thing"}, map[string]any{"field": "value"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := inferElement("ambiguous", test.value); err == nil {
				t.Fatal("inferElement() error = nil, want rejection")
			}
		})
	}
}

func TestBuildAASMapsGranularityToAssetKind(t *testing.T) {
	tests := []struct {
		granularity string
		assetKind   types.AssetKind
	}{
		{granularity: "Item", assetKind: types.AssetKindInstance},
		{granularity: "Model", assetKind: types.AssetKindType},
		{granularity: "Batch", assetKind: types.AssetKindBatch},
	}

	for _, test := range tests {
		t.Run(test.granularity, func(t *testing.T) {
			aas := buildAAS(dppHeader{
				DigitalProductPassportID: "dpp-" + test.granularity,
				UniqueProductIdentifier:  "product-" + test.granularity,
				Granularity:              test.granularity,
			}, nil)
			if aas.AssetInformation().AssetKind() != test.assetKind {
				t.Fatalf("asset kind = %v, want %v", aas.AssetInformation().AssetKind(), test.assetKind)
			}
		})
	}
}
