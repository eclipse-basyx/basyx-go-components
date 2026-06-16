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

package grammar

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestModelStringPattern_UnmarshalJSON_AcceptsAASAndSME(t *testing.T) {
	t.Parallel()

	cases := []string{
		"$aas#idShort",
		"$aas#id",
		"$aas#assetInformation.assetKind",
		"$aas#assetInformation.assetType",
		"$aas#assetInformation.globalAssetId",
		"$aas#assetInformation.specificAssetIds[0].name",
		"$aas#assetInformation.specificAssetIds[2].externalSubjectId.keys[3].value",
		"$aas#submodels[0].keys[0].value",
		"$sm#idShort",
		"$sm#semanticId.type",
		"$sm#semanticId.keys[0].value",
		"$sm#supplementalSemanticIds[0].keys[1].value",
		"$sme#idShort",
		"$sme#value",
		"$sme#valueType",
		"$sme#language",
		"$sme.temperature#supplementalSemanticIds[0].type",
		"$cd#id",
		"$aasdesc#submodelDescriptors[0].supplementalSemanticIds[1].keys[2].type",
		"$smdesc#supplementalSemanticIds[0].keys[0].value",
	}

	for _, in := range cases {
		in := in
		t.Run(in, func(t *testing.T) {
			var p ModelStringPattern
			if err := json.Unmarshal([]byte("\""+in+"\""), &p); err != nil {
				t.Fatalf("expected to accept %q, got error: %v", in, err)
			}
			if string(p) != in {
				t.Fatalf("roundtrip mismatch: got %q want %q", string(p), in)
			}
		})
	}
}

func TestModelStringPattern_UnmarshalJSON_RejectsInvalid(t *testing.T) {
	t.Parallel()

	// These should be rejected by the regex validator.
	cases := []string{
		"",
		"not-a-pattern",
		"$aas",
		"$aas#",
		"$sme",
		"$sme#",
		"$sm#unknownField",
		"$aasdesc#createdAt",
		"$smdesc#createdAt",
		"$bd#createdAt",
	}

	for _, in := range cases {
		in := in
		t.Run(in, func(t *testing.T) {
			var p ModelStringPattern
			err := json.Unmarshal([]byte("\""+in+"\""), &p)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", in)
			}
			if !strings.Contains(err.Error(), "pattern match") {
				t.Fatalf("expected pattern error for %q, got: %v", in, err)
			}
		})
	}
}

func TestFragmentStringPattern_UnmarshalJSON_AcceptsAASAndSMEFragments(t *testing.T) {
	t.Parallel()

	cases := []string{
		"$aas#assetInformation.assetType",
		"$aas#assetInformation.specificAssetIds",
		"$aas#submodels[0]",
		"$aas#submodels",
		"$aas#submodels[0].keys",
		"$aas#assetInformation.specificAssetIds[0]",
		"$aas#assetInformation.specificAssetIds[0].externalSubjectId.keys",
		"$aas#assetInformation.specificAssetIds[0].externalSubjectId.keys[3]",
		"$aasdesc#description",
		"$aasdesc#displayName",
		"$aasdesc#administration",
		"$aasdesc#endpoints",
		"$aasdesc#submodelDescriptors",
		"$aasdesc#submodelDescriptors[0].supplementalSemanticIds[1].keys",
		"$sme#idShort",
		"$sme#value",
		"$sme#supplementalSemanticIds[0].keys",
		"$sm#supplementalSemanticIds[0].keys",
		"$smdesc#supplementalSemanticIds[0].keys",
		"$sm#semanticId.keys[0]",
		"$sm#semanticId.keys",
	}

	for _, in := range cases {
		in := in
		t.Run(in, func(t *testing.T) {
			var p FragmentStringPattern
			if err := json.Unmarshal([]byte("\""+in+"\""), &p); err != nil {
				t.Fatalf("expected to accept %q, got error: %v", in, err)
			}
			if string(p) != in {
				t.Fatalf("roundtrip mismatch: got %q want %q", string(p), in)
			}
		})
	}
}

func TestFragmentStringPattern_UnmarshalJSON_RejectsInvalid(t *testing.T) {
	t.Parallel()

	cases := []string{
		"",
		"not-a-pattern",
		"$aas",
		"$aas#",
		"$sme#",
		"$sm#id",
		"$aasdesc#endpoints[].protocolinformation",
	}

	for _, in := range cases {
		in := in
		t.Run(in, func(t *testing.T) {
			var p FragmentStringPattern
			err := json.Unmarshal([]byte("\""+in+"\""), &p)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", in)
			}
			if !strings.Contains(err.Error(), "pattern match") {
				t.Fatalf("expected pattern error for %q, got: %v", in, err)
			}
		})
	}
}
