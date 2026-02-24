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
		"$sm#idShort",
		"$sm#semanticId.type",
		"$sm#semanticId.keys[0].value",
		"$sme#idShort",
		"$sme#value",
		"$sme#valueType",
		"$sme#language",
		"$cd#id",
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
		"$aas#assetInformation",
		"$aas#submodels",
		"$aas#assetInformation.specificAssetIds[0]",
		"$aas#assetInformation.specificAssetIds[0].externalSubjectId.keys[3]",
		"$sme#idShort",
		"$sme#value",
		"$sm#semanticId.keys[0]",
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
		"$sme",
		"$sme#",
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
