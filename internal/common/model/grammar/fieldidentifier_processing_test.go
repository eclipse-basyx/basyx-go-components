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
// Author: Martin Stemmer ( Fraunhofer IESE )

//nolint:all
package grammar

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type fidTestCase struct {
	Name          string
	Kind          string // "scalar" or "fragment"
	Input         string // field identifier pattern, e.g. $aasdesc#idShort
	WantScalar    *expectedScalar
	WantFragment  *expectedFragment
	ShouldFail    bool
	ErrorContains string
}

type expectedBinding struct {
	Alias string     `json:"alias"`
	Index ArrayIndex `json:"index"`
}

type expectedScalar struct {
	Column   string            `json:"column"`
	Bindings []expectedBinding `json:"bindings"`
}

type expectedFragment struct {
	Bindings []expectedBinding `json:"bindings"`
}

func idx(i int) ArrayIndex {
	return NewArrayIndexPosition(i)
}

func sidx(s string) ArrayIndex {
	return NewArrayIndexString(s)
}

func mustMarshalPrettyJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

var fieldIdentifierProcessingCases = []fidTestCase{
	{
		Name:  "aasdesc_specificAsset_externalSubject_keys_value_indexed",
		Kind:  "scalar",
		Input: `$aasdesc#specificAssetIds[2].externalSubjectId.keys[5].value`,
		WantScalar: &expectedScalar{
			Column: "external_subject_reference_key.value",
			Bindings: []expectedBinding{
				{Alias: "specific_asset_id.position", Index: idx(2)},
				{Alias: "external_subject_reference_key.position", Index: idx(5)},
			},
		},
	},
	{
		Name:  "aasdesc_submodelDescriptor_endpoints_href_indexed",
		Kind:  "scalar",
		Input: `$aasdesc#submodelDescriptors[1].endpoints[0].protocolinformation.href`,
		WantScalar: &expectedScalar{
			Column: "submodel_descriptor_endpoint.href",
			Bindings: []expectedBinding{
				{Alias: "submodel_descriptor.position", Index: idx(1)},
				{Alias: "submodel_descriptor_endpoint.position", Index: idx(0)},
			},
		},
	},
	{
		Name:       "aasdesc_endpoints_href_wildcard",
		Kind:       "scalar",
		Input:      `$aasdesc#endpoints[].protocolinformation.href`,
		WantScalar: &expectedScalar{Column: "aas_descriptor_endpoint.href", Bindings: []expectedBinding{}},
	},
	{
		Name:  "aasdesc_endpoints_interface_indexed",
		Kind:  "scalar",
		Input: `$aasdesc#endpoints[2].interface`,
		WantScalar: &expectedScalar{
			Column:   "aas_descriptor_endpoint.interface",
			Bindings: []expectedBinding{{Alias: "aas_descriptor_endpoint.position", Index: idx(2)}},
		},
	},
	{
		Name:  "aasdesc_submodelDescriptor_semanticId_keys_type_indexed",
		Kind:  "scalar",
		Input: `$aasdesc#submodelDescriptors[3].semanticId.keys[7].type`,
		WantScalar: &expectedScalar{
			Column: "aasdesc_submodel_descriptor_semantic_id_reference_key.type",
			Bindings: []expectedBinding{
				{Alias: "submodel_descriptor.position", Index: idx(3)},
				{Alias: "aasdesc_submodel_descriptor_semantic_id_reference_key.position", Index: idx(7)},
			},
		},
	},
	{
		Name:  "smdesc_endpoints_href_indexed",
		Kind:  "scalar",
		Input: `$smdesc#endpoints[0].protocolinformation.href`,
		WantScalar: &expectedScalar{
			Column:   "submodel_descriptor_endpoint.href",
			Bindings: []expectedBinding{{Alias: "submodel_descriptor_endpoint.position", Index: idx(0)}},
		},
	},
	{
		Name:  "sm_semanticId_keys_value_indexed",
		Kind:  "scalar",
		Input: `$sm#semanticId.keys[0].value`,
		WantScalar: &expectedScalar{
			Column:   "semantic_id_reference_key.value",
			Bindings: []expectedBinding{{Alias: "semantic_id_reference_key.position", Index: idx(0)}},
		},
	},
	{
		Name:       "aasdesc_idShort_scalar",
		Kind:       "scalar",
		Input:      `$aasdesc#idShort`,
		WantScalar: &expectedScalar{Column: "aas_descriptor.id_short", Bindings: []expectedBinding{}},
	},
	{
		Name:       "aasdesc_assetKind_scalar",
		Kind:       "scalar",
		Input:      `$aasdesc#assetKind`,
		WantScalar: &expectedScalar{Column: "aas_descriptor.asset_kind", Bindings: []expectedBinding{}},
	},
	{
		Name:       "aasdesc_specificAssetIds_name_wildcard",
		Kind:       "scalar",
		Input:      `$aasdesc#specificAssetIds[].name`,
		WantScalar: &expectedScalar{Column: "specific_asset_id.name", Bindings: []expectedBinding{}},
	},
	{
		Name:       "aasdesc_specificAssetIds_value_indexed",
		Kind:       "scalar",
		Input:      `$aasdesc#specificAssetIds[0].value`,
		WantScalar: &expectedScalar{Column: "specific_asset_id.value", Bindings: []expectedBinding{{Alias: "specific_asset_id.position", Index: idx(0)}}},
	},
	{
		Name:  "aasdesc_specificAsset_externalSubject_keys_type_indexed",
		Kind:  "scalar",
		Input: `$aasdesc#specificAssetIds[2].externalSubjectId.keys[5].type`,
		WantScalar: &expectedScalar{
			Column: "external_subject_reference_key.type",
			Bindings: []expectedBinding{
				{Alias: "specific_asset_id.position", Index: idx(2)},
				{Alias: "external_subject_reference_key.position", Index: idx(5)},
			},
		},
	},
	{
		Name:       "aasdesc_submodelDescriptor_id_indexed",
		Kind:       "scalar",
		Input:      `$aasdesc#submodelDescriptors[1].id`,
		WantScalar: &expectedScalar{Column: "submodel_descriptor.id", Bindings: []expectedBinding{{Alias: "submodel_descriptor.position", Index: idx(1)}}},
	},
	{
		Name:       "aasdesc_submodelDescriptor_semanticId_type_indexed",
		Kind:       "scalar",
		Input:      `$aasdesc#submodelDescriptors[1].semanticId.type`,
		WantScalar: &expectedScalar{Column: "aasdesc_submodel_descriptor_semantic_id_reference.type", Bindings: []expectedBinding{{Alias: "submodel_descriptor.position", Index: idx(1)}}},
	},
	{
		Name:       "aasdesc_submodelDescriptor_endpoints_href_wildcard",
		Kind:       "scalar",
		Input:      `$aasdesc#submodelDescriptors[1].endpoints[].protocolinformation.href`,
		WantScalar: &expectedScalar{Column: "submodel_descriptor_endpoint.href", Bindings: []expectedBinding{{Alias: "submodel_descriptor.position", Index: idx(1)}}},
	},
	{
		Name:       "smdesc_semanticId_keys_value_wildcard",
		Kind:       "scalar",
		Input:      `$smdesc#semanticId.keys[].value`,
		WantScalar: &expectedScalar{Column: "aasdesc_submodel_descriptor_semantic_id_reference_key.value", Bindings: []expectedBinding{}},
	},
	{
		Name:       "sm_semanticId_type_scalar",
		Kind:       "scalar",
		Input:      `$sm#semanticId.type`,
		WantScalar: &expectedScalar{Column: "semantic_id_reference.type", Bindings: []expectedBinding{}},
	},
	{
		Name:         "aasdesc_endpoints_fragment_wildcard",
		Kind:         "fragment",
		Input:        `$aasdesc#endpoints[]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{}},
	},
	{
		Name:         "aasdesc_endpoints_fragment_indexed",
		Kind:         "fragment",
		Input:        `$aasdesc#endpoints[2]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{{Alias: "aas_descriptor_endpoint.position", Index: idx(2)}}},
	},
	{
		Name:         "aasdesc_specificAssetIds_fragment_wildcard",
		Kind:         "fragment",
		Input:        `$aasdesc#specificAssetIds[]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{}},
	},
	{
		Name:         "aasdesc_specificAssetIds_fragment_indexed",
		Kind:         "fragment",
		Input:        `$aasdesc#specificAssetIds[4]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{{Alias: "specific_asset_id.position", Index: idx(4)}}},
	},
	{
		Name:         "aasdesc_specificAsset_externalSubject_keys_fragment_wildcard",
		Kind:         "fragment",
		Input:        `$aasdesc#specificAssetIds[2].externalSubjectId.keys[]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{{Alias: "specific_asset_id.position", Index: idx(2)}}},
	},
	{
		Name:         "aasdesc_specificAsset_externalSubject_keys_fragment_indexed",
		Kind:         "fragment",
		Input:        `$aasdesc#specificAssetIds[].externalSubjectId.keys[3]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{{Alias: "external_subject_reference_key.position", Index: idx(3)}}},
	},
	{
		Name:         "aasdesc_submodelDescriptors_fragment_wildcard",
		Kind:         "fragment",
		Input:        `$aasdesc#submodelDescriptors[]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{}},
	},
	{
		Name:         "aasdesc_submodelDescriptor_endpoints_fragment_indexed",
		Kind:         "fragment",
		Input:        `$aasdesc#submodelDescriptors[1].endpoints[2]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{{Alias: "submodel_descriptor.position", Index: idx(1)}, {Alias: "submodel_descriptor_endpoint.position", Index: idx(2)}}},
	},
	{
		Name:         "aasdesc_submodelDescriptor_semanticId_keys_fragment_wildcard",
		Kind:         "fragment",
		Input:        `$aasdesc#submodelDescriptors[3].semanticId.keys[]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{{Alias: "submodel_descriptor.position", Index: idx(3)}}},
	},
	{
		Name:         "smdesc_endpoints_fragment_wildcard",
		Kind:         "fragment",
		Input:        `$smdesc#endpoints[]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{}},
	},
	{
		Name:         "sm_semanticId_keys_fragment_indexed",
		Kind:         "fragment",
		Input:        `$sm#semanticId.keys[0]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{{Alias: "semantic_id_reference_key.position", Index: idx(0)}}},
	},
	{
		Name:         "aasdesc_submodelDescriptor_endpoints_fragment_indexed_submodelWildcard",
		Kind:         "fragment",
		Input:        `$aasdesc#submodelDescriptors[].endpoints[2]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{{Alias: "submodel_descriptor_endpoint.position", Index: idx(2)}}},
	},
	{Name: "scalar_rejects_fragment", Kind: "scalar", Input: `$aasdesc#endpoints[0]`, ShouldFail: true, ErrorContains: "must not end in an array segment"},
	{Name: "sql_scalar_rejects_aas_root", Kind: "scalar", Input: `$aas#idShort`, ShouldFail: true, ErrorContains: "unsupported field root"},
	{Name: "sql_fragment_rejects_aas_root", Kind: "fragment", Input: `$aas#assetInformation.specificAssetIds[0]`, ShouldFail: true, ErrorContains: "unsupported field root"},
	{
		Name:       "sql_scalar_sme_value",
		Kind:       "scalar",
		Input:      `$sme#value`,
		WantScalar: &expectedScalar{Column: "COALESCE(property_element.value_text, property_element.value_num::text, property_element.value_bool::text, property_element.value_time::text, property_element.value_datetime::text)", Bindings: []expectedBinding{}},
	},
	{
		Name:         "sql_fragment_sme_semanticId_keys_indexed",
		Kind:         "fragment",
		Input:        `$sme.temperature#semanticId.keys[0]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{{Alias: "submodel_element.idshort_path", Index: sidx("temperature")}, {Alias: "semantic_id_reference_key.position", Index: idx(0)}}},
	},
	{
		Name:       "sme_idShort_scalar",
		Kind:       "scalar",
		Input:      `$sme.temperature#idShort`,
		WantScalar: &expectedScalar{Column: "submodel_element.id_short", Bindings: []expectedBinding{{Alias: "submodel_element.idshort_path", Index: sidx("temperature")}}},
	},
	{
		Name:       "sme_valueType_scalar",
		Kind:       "scalar",
		Input:      `$sme.MyList[2].temp#valueType`,
		WantScalar: &expectedScalar{Column: "property_element.value_type", Bindings: []expectedBinding{{Alias: "submodel_element.idshort_path", Index: sidx("MyList[2].temp")}}},
	},
	{
		Name:       "sme_semanticId_keys_type_indexed_scalar",
		Kind:       "scalar",
		Input:      `$sme.some.path#semanticId.keys[3].type`,
		WantScalar: &expectedScalar{Column: "sme_semantic_id_reference_key.type", Bindings: []expectedBinding{{Alias: "submodel_element.idshort_path", Index: sidx("some.path")}, {Alias: "semantic_id_reference_key.position", Index: idx(3)}}},
	},
	{
		Name:         "sme_semanticId_keys_wildcard_fragment",
		Kind:         "fragment",
		Input:        `$sme.temperature#semanticId.keys[]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{{Alias: "submodel_element.idshort_path", Index: sidx("temperature")}}},
	},
	{
		Name:         "sme_semanticId_keys_indexed_fragment",
		Kind:         "fragment",
		Input:        `$sme.engine#semanticId.keys[1]`,
		WantFragment: &expectedFragment{Bindings: []expectedBinding{{Alias: "submodel_element.idshort_path", Index: sidx("engine")}, {Alias: "semantic_id_reference_key.position", Index: idx(1)}}},
	},
}

func TestFieldIdentifierProcessing(t *testing.T) {
	t.Parallel()

	for _, tc := range fieldIdentifierProcessingCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			fieldStr := strings.TrimSpace(tc.Input)
			if fieldStr == "" {
				t.Fatalf("empty input")
			}

			switch tc.Kind {
			case "scalar":
				f := ModelStringPattern(fieldStr)
				got, err := ResolveScalarFieldToSQL(&f)

				if tc.ShouldFail {
					if err == nil {
						t.Fatalf("expected error, got nil")
					}
					if tc.ErrorContains != "" && !strings.Contains(err.Error(), tc.ErrorContains) {
						t.Fatalf("expected error to contain %q, got %v", tc.ErrorContains, err)
					}
					return
				}

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tc.WantScalar == nil {
					t.Fatalf("missing WantScalar for passing case")
				}

				want := *tc.WantScalar
				if want.Bindings == nil {
					want.Bindings = []expectedBinding{}
				}

				gotExp := expectedScalar{Column: got.Column, Bindings: make([]expectedBinding, 0, len(got.ArrayBindings))}
				for _, b := range got.ArrayBindings {
					gotExp.Bindings = append(gotExp.Bindings, expectedBinding{Alias: b.Alias, Index: b.Index})
				}

				if !reflect.DeepEqual(gotExp, want) {
					t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", mustMarshalPrettyJSON(t, gotExp), mustMarshalPrettyJSON(t, want))
				}

			case "fragment":
				f := FragmentStringPattern(fieldStr)
				got, err := ResolveFragmentFieldToSQL(&f)

				if tc.ShouldFail {
					if err == nil {
						t.Fatalf("expected error, got nil")
					}
					if tc.ErrorContains != "" && !strings.Contains(err.Error(), tc.ErrorContains) {
						t.Fatalf("expected error to contain %q, got %v", tc.ErrorContains, err)
					}
					return
				}

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tc.WantFragment == nil {
					t.Fatalf("missing WantFragment for passing case")
				}

				want := *tc.WantFragment
				if want.Bindings == nil {
					want.Bindings = []expectedBinding{}
				}

				gotExp := expectedFragment{Bindings: make([]expectedBinding, 0, len(got))}
				for _, b := range got {
					gotExp.Bindings = append(gotExp.Bindings, expectedBinding{Alias: b.Alias, Index: b.Index})
				}

				if !reflect.DeepEqual(gotExp, want) {
					t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", mustMarshalPrettyJSON(t, gotExp), mustMarshalPrettyJSON(t, want))
				}

			default:
				t.Fatalf("unknown kind %q (expected scalar|fragment)", tc.Kind)
			}
		})
	}
}
