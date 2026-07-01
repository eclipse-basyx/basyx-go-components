package apis

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestParseCSVQueryValuesSplitsRepeatedAndCommaSeparatedValues(t *testing.T) {
	t.Parallel()

	got := parseCSVQueryValues([]string{"a,b", " c "})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCSVQueryValues() = %#v, want %#v", got, want)
	}
}

func TestParseCSVQueryValuesPreservesProvidedButEmptyValues(t *testing.T) {
	t.Parallel()

	got := parseCSVQueryValues([]string{"", " , "})
	want := []string{""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCSVQueryValues() = %#v, want %#v", got, want)
	}
}

func TestValidateAllowedQueryParametersAcceptsKnownShellDescriptorParameters(t *testing.T) {
	t.Parallel()

	query := url.Values{
		"limit":        {"10"},
		"cursor":       {"abc"},
		"assetKind":    {"Instance"},
		"assetType":    {"part"},
		"assetIds":     {"encoded"},
		"createdFrom":  {"2026-01-01T00:00:00Z"},
		"updatedFrom":  {"2026-01-01T00:00:00Z"},
		"createdAfter": {"2026-01-01T00:00:00Z"},
	}

	if err := validateAllowedQueryParameters(query, getAllShellDescriptorsQueryParameters); err != nil {
		t.Fatalf("validateAllowedQueryParameters() returned error: %v", err)
	}
}

func TestValidateAllowedQueryParametersRejectsUnknownShellDescriptorParameters(t *testing.T) {
	t.Parallel()

	err := validateAllowedQueryParameters(url.Values{"AssetIds": {"encoded"}}, getAllShellDescriptorsQueryParameters)
	if err == nil {
		t.Fatal("expected unknown query parameter error")
	}
	if !strings.Contains(err.Error(), "AssetIds") {
		t.Fatalf("expected error to contain unknown parameter name, got %q", err.Error())
	}
}
