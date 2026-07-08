package main

import (
	"os"
	"strings"
	"testing"
)

func TestOpenAPIDocumentsShellDescriptorAssetIDs(t *testing.T) {
	spec, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}

	operation := shellDescriptorsGetOperation(t, string(spec))
	for _, want := range []string{
		"../Part2-API-Schemas/openapi.yaml#/components/parameters/AssetIds",
		"#/components/parameters/DTRCreatedAfter",
		"Supports assetIds query parameters",
	} {
		if !strings.Contains(operation, want) {
			t.Fatalf("GET /shell-descriptors is missing %q", want)
		}
	}
}

func shellDescriptorsGetOperation(t *testing.T, spec string) string {
	t.Helper()

	pathIndex := strings.Index(spec, "  /shell-descriptors:\n")
	if pathIndex < 0 {
		t.Fatal("missing /shell-descriptors path")
	}
	getIndex := strings.Index(spec[pathIndex:], "    get:\n")
	if getIndex < 0 {
		t.Fatal("missing GET /shell-descriptors operation")
	}
	operationStart := pathIndex + getIndex
	operationEnd := strings.Index(spec[operationStart:], "    post:\n")
	if operationEnd < 0 {
		t.Fatal("missing POST /shell-descriptors operation boundary")
	}

	return spec[operationStart : operationStart+operationEnd]
}
