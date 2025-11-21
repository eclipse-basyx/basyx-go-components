/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

package builder

import "testing"

type expectedToken struct {
	Name    string
	IsArray bool
	Index   int
}

var tokenizeCases = []struct {
	name   string
	field  string
	tokens []expectedToken
}{
	{
		name:  "$aasdesc#specificAssetIds[].externalSubjectId.keys[].value",
		field: "$aasdesc#specificAssetIds[].externalSubjectId.keys[].value",
		tokens: []expectedToken{
			{Name: "specificAssetIds", IsArray: true, Index: -1},
			{Name: "externalSubjectId", IsArray: false},
			{Name: "keys", IsArray: true, Index: -1},
			{Name: "value", IsArray: false},
		},
	},
	{
		name:  "$aasdesc#endpoints[].interface",
		field: "$aasdesc#endpoints[].interface",
		tokens: []expectedToken{
			{Name: "endpoints", IsArray: true, Index: -1},
			{Name: "interface", IsArray: false},
		},
	},
	{
		name:  "$smdesc#id",
		field: "$smdesc#id",
		tokens: []expectedToken{
			{Name: "id", IsArray: false},
		},
	},
	{
		name:  "$aasdesc#submodelDescriptors[].endpoints[].protocolinformation.href",
		field: "$aasdesc#submodelDescriptors[].endpoints[].protocolinformation.href",
		tokens: []expectedToken{
			{Name: "submodelDescriptors", IsArray: true, Index: -1},
			{Name: "endpoints", IsArray: true, Index: -1},
			{Name: "protocolinformation", IsArray: false},
			{Name: "href", IsArray: false},
		},
	},
	{
		name:  "$aasdesc#specificAssetIds[2].externalSubjectId.keys[4].value",
		field: "$aasdesc#specificAssetIds[2].externalSubjectId.keys[4].value",
		tokens: []expectedToken{
			{Name: "specificAssetIds", IsArray: true, Index: 2},
			{Name: "externalSubjectId", IsArray: false},
			{Name: "keys", IsArray: true, Index: 4},
			{Name: "value", IsArray: false},
		},
	},
	{
		name:  "$aasdesc#specificAssetIds[3].externalSubjectId.keys[8].value",
		field: "$aasdesc#specificAssetIds[3].externalSubjectId.keys[8].value",
		tokens: []expectedToken{
			{Name: "specificAssetIds", IsArray: true, Index: 3},
			{Name: "externalSubjectId", IsArray: false},
			{Name: "keys", IsArray: true, Index: 8},
			{Name: "value", IsArray: false},
		},
	},
	{
		name:  "$aasdesc#submodelDescriptors[1].endpoints[0].protocolinformation.href",
		field: "$aasdesc#submodelDescriptors[1].endpoints[0].protocolinformation.href",
		tokens: []expectedToken{
			{Name: "submodelDescriptors", IsArray: true, Index: 1},
			{Name: "endpoints", IsArray: true, Index: 0},
			{Name: "protocolinformation", IsArray: false},
			{Name: "href", IsArray: false},
		},
	},
	{
		name:  "$aasdesc#submodelDescriptors[1].endpoints[].protocolinformation.href",
		field: "$aasdesc#submodelDescriptors[1].endpoints[].protocolinformation.href",
		tokens: []expectedToken{
			{Name: "submodelDescriptors", IsArray: true, Index: 1},
			{Name: "endpoints", IsArray: true, Index: -1},
			{Name: "protocolinformation", IsArray: false},
			{Name: "href", IsArray: false},
		},
	},
}

func TestTokenizeFieldPaths(t *testing.T) {
	for _, tt := range tokenizeCases {
		t.Run(tt.name, func(t *testing.T) {
			tokens := TokenizeField(tt.field)
			if len(tokens) != len(tt.tokens) {
				t.Fatalf("unexpected token count for %s: got %d want %d", tt.field, len(tokens), len(tt.tokens))
			}
			for i, exp := range tt.tokens {
				if exp.IsArray {
					token, ok := tokens[i].(ArrayToken)
					if !ok {
						t.Fatalf("token %d for %s expected ArrayToken, got %T", i, tt.field, tokens[i])
					}
					if token.Name != exp.Name || token.Index != exp.Index {
						t.Fatalf("token %d for %s mismatch: got {%s %d} want {%s %d}", i, tt.field, token.Name, token.Index, exp.Name, exp.Index)
					}
				} else {
					token, ok := tokens[i].(SimpleToken)
					if !ok {
						t.Fatalf("token %d for %s expected SimpleToken, got %T", i, tt.field, tokens[i])
					}
					if token.Name != exp.Name {
						t.Fatalf("token %d for %s mismatch: got %s want %s", i, tt.field, token.Name, exp.Name)
					}
				}
			}
		})
	}
}
