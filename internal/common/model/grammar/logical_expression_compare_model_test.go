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

// Package grammar defines the data structures for representing logical expressions in the grammar model.
// Author: Martin Stemmer ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func TestLogicalExpression_EvaluateModel(t *testing.T) {
	kind := model.ASSETKIND_INSTANCE
	desc := model.AssetAdministrationShellDescriptor{
		Id:            "aas-1",
		IdShort:       "shell-short",
		GlobalAssetId: "global-42",
		AssetKind:     &kind,
		AssetType:     "type-X",
		SpecificAssetIds: []model.SpecificAssetID{
			{
				Name:  "serial",
				Value: "SN-001",
				ExternalSubjectID: &model.Reference{
					Type: model.REFERENCETYPES_EXTERNAL_REFERENCE,
					Keys: []model.Key{
						{Type: model.KEYTYPES_GLOBAL_REFERENCE, Value: "martin"},
					},
				},
			},
		},
		SubmodelDescriptors: []model.SubmodelDescriptor{
			{
				Id:      "sub-1",
				IdShort: "sub-short",
				Endpoints: []model.Endpoint{
					{
						Interface: "HTTP",
						ProtocolInformation: model.ProtocolInformation{
							Href: "https://example.com",
						},
					},
				},
				SemanticId: &model.Reference{
					Type: model.REFERENCETYPES_EXTERNAL_REFERENCE,
					Keys: []model.Key{
						{Type: model.KEYTYPES_GLOBAL_REFERENCE, Value: "klopapier"},
					},
				},
			},
		},
	}

	tests := []struct {
		name    string
		expr    LogicalExpression
		want    bool
		wantErr string
	}{
		{
			name: "eq identifier",
			expr: LogicalExpression{
				Eq: ComparisonItems{
					field("$aasdesc#id"),
					strVal("aas-1"),
				},
			},
			want: true,
		},
		{
			name: "eq identifier mismatch",
			expr: LogicalExpression{
				Eq: ComparisonItems{
					field("$aasdesc#id"),
					strVal("other"),
				},
			},
			want: false,
		},
		{
			name: "or with one matching branch",
			expr: LogicalExpression{
				Or: []LogicalExpression{
					{
						Eq: ComparisonItems{
							field("$aasdesc#idShort"),
							strVal("nope"),
						},
					},
					{
						Eq: ComparisonItems{
							field("$aasdesc#globalAssetId"),
							strVal("global-42"),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "specific asset id field",
			expr: LogicalExpression{
				Eq: ComparisonItems{
					field("$aasdesc#specificAssetIds[0].value"),
					strVal("SN-001"),
				},
			},
			want: true,
		},
		{
			name: "specific asset id field",
			expr: LogicalExpression{
				Eq: ComparisonItems{
					field("$aasdesc#specificAssetIds[100].value"),
					strVal("SN-001"),
				},
			},
			want: false,
		},
		{
			name: "submodel descriptor alias",
			expr: LogicalExpression{
				Eq: ComparisonItems{
					field("$smdesc#idShort"),
					strVal("sub-short"),
				},
			},
			want: true,
		},
		{
			name: "not operator",
			expr: LogicalExpression{
				Not: &LogicalExpression{
					Eq: ComparisonItems{
						field("$aasdesc#assetType"),
						strVal("type-Y"),
					},
				},
			},
			want: true,
		},
		{
			name: "match expressions unsupported",
			expr: LogicalExpression{
				Match: []MatchExpression{{Boolean: boolPtr(true)}},
			},
			wantErr: "match expressions are not supported",
		},
		{
			name: "finds external subject id",
			expr: LogicalExpression{
				Eq: ComparisonItems{
					field("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"),
					strVal("martin"),
				},
			},
			want: true,
		},
		{
			name: "finds external subject id",
			expr: LogicalExpression{
				Eq: ComparisonItems{
					field("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"),
					strVal("martiniooo"),
				},
			},
			want: false,
		},
		{
			name: "what happens if it gets a smdesc field",
			expr: LogicalExpression{
				Eq: ComparisonItems{
					field("$smdesc#semanticId.keys[].value"),
					strVal("klopapier"),
				},
			},
			want: true,
		},
		{
			name: "starts-with passes",
			expr: LogicalExpression{
				StartsWith: StringItems{
					strField("$aasdesc#id"),
					strString("aas-"),
				},
			},
			want: true,
		},
		{
			name: "ends-with passes",
			expr: LogicalExpression{
				EndsWith: StringItems{
					strField("$aasdesc#id"),
					strString("1"),
				},
			},
			want: true,
		},
		{
			name: "contains fails",
			expr: LogicalExpression{
				Contains: StringItems{
					strField("$aasdesc#globalAssetId"),
					strString("MISSING"),
				},
			},
			want: false,
		},
		{
			name: "regex matches id",
			expr: LogicalExpression{
				Regex: StringItems{
					strField("$aasdesc#id"),
					strString("^aas-[0-9]+$"),
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.expr.EvaluateAssetAdministrationShellDescriptor(desc)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestLogicalExpression_EvaluateSubmodelDescriptor(t *testing.T) {
	sub := model.SubmodelDescriptor{
		Id:      "sub-42",
		IdShort: "sub-short",
		Endpoints: []model.Endpoint{
			{
				Interface: "MQTT",
				ProtocolInformation: model.ProtocolInformation{
					Href: "mqtt://broker",
				},
			},
		},
	}

	tests := []struct {
		name string
		expr LogicalExpression
		want bool
	}{
		{
			name: "simple alias match",
			expr: LogicalExpression{
				Eq: ComparisonItems{
					field("$smdesc#id"),
					strVal("sub-42"),
				},
			},
			want: true,
		},
		{
			name: "complex or/and with negation",
			expr: LogicalExpression{
				Or: []LogicalExpression{
					{
						And: []LogicalExpression{
							{
								Eq: ComparisonItems{
									field("$smdesc#idShort"),
									strVal("missing"),
								},
							},
						},
					},
					{
						And: []LogicalExpression{
							{
								Eq: ComparisonItems{
									field("$smdesc#idShort"),
									strVal("sub-short"),
								},
							},
							{
								Not: &LogicalExpression{
									Eq: ComparisonItems{
										field("$smdesc#id"),
										strVal("other"),
									},
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "negative match",
			expr: LogicalExpression{
				Not: &LogicalExpression{
					Eq: ComparisonItems{
						field("$smdesc#id"),
						strVal("other"),
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.expr.EvaluateSubmodelDescriptor(sub)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func field(value string) Value {
	p := ModelStringPattern(value)
	return Value{Field: &p}
}

func strField(value string) StringValue {
	p := ModelStringPattern(value)
	return StringValue{Field: &p}
}

func strVal(value string) Value {
	s := StandardString(value)
	return Value{StrVal: &s}
}

func strString(value string) StringValue {
	s := StandardString(value)
	return StringValue{StrVal: &s}
}

func boolPtr(b bool) *bool {
	return &b
}
