package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestDetectPart2SchemaVersion(t *testing.T) {
	spec := []byte("openapi: 3.0.3\ninfo:\n  version: V3.2.0_SSP-001\n")
	version := detectPart2SchemaVersion(spec)
	if version != "V3.2.0" {
		t.Fatalf("expected V3.2.0, got %s", version)
	}

	fallback := detectPart2SchemaVersion([]byte("openapi: 3.0.3\ninfo:\n  title: x\n"))
	if fallback != "V3.1.1" {
		t.Fatalf("expected fallback V3.1.1, got %s", fallback)
	}
}

func TestLocalizePart2SchemaReferences_RewritesRemoteAndRelativeRefs(t *testing.T) {
	specRemote := []byte("openapi: 3.0.3\ninfo:\n  version: V3.1.1_SSP-001\npaths:\n  /x:\n    get:\n      responses:\n        '400':\n          $ref: 'https://api.swaggerhub.com/domains/Plattform_i40/Part2-API-Schemas/V3.1.1#/components/responses/bad-request'\n")
	localizedRemote := localizePart2SchemaReferences(specRemote, "/api-docs/openapi.yaml")
	localizedRemoteText := string(localizedRemote)
	if strings.Contains(localizedRemoteText, "api.swaggerhub.com/domains/Plattform_i40/Part2-API-Schemas") {
		t.Fatal("expected remote references to be removed")
	}
	if !strings.Contains(localizedRemoteText, "/api-docs/part2-schemas/V3.1.1/openapi.yaml#/components/responses/bad-request") {
		t.Fatal("expected local V3.1.1 part2 schema reference")
	}

	specRelative := []byte("openapi: 3.0.3\ninfo:\n  version: V3.2.0_SSP-001\npaths:\n  /x:\n    get:\n      responses:\n        '400':\n          $ref: '../Part2-API-Schemas/openapi.yaml#/components/responses/bad-request'\n")
	localizedRelative := localizePart2SchemaReferences(specRelative, "/api-docs/openapi.yaml")
	localizedRelativeText := string(localizedRelative)
	if strings.Contains(localizedRelativeText, "../Part2-API-Schemas/openapi.yaml") {
		t.Fatal("expected relative references to be removed")
	}
	if !strings.Contains(localizedRelativeText, "/api-docs/part2-schemas/V3.2.0/openapi.yaml#/components/responses/bad-request") {
		t.Fatal("expected local V3.2.0 part2 schema reference")
	}
}

func TestAddSwaggerUIServesLocalPart2Schemas(t *testing.T) {
	r := chi.NewRouter()
	AddSwaggerUI(r, SwaggerUIConfig{
		Title:       "test",
		SpecURL:     "/api-docs/openapi.yaml",
		UIPath:      "/swagger",
		SpecPath:    "/api-docs/openapi.yaml",
		SpecContent: []byte("openapi: 3.0.3\ninfo:\n  version: V3.1.1_SSP-001\npaths:\n  /x:\n    get:\n      responses:\n        '400':\n          $ref: 'https://api.swaggerhub.com/domains/Plattform_i40/Part2-API-Schemas/V3.1.1#/components/responses/bad-request'\n"),
		BasePath:    "/",
	})

	specReq := httptest.NewRequest(http.MethodGet, "/api-docs/openapi.yaml", nil)
	specRecorder := httptest.NewRecorder()
	r.ServeHTTP(specRecorder, specReq)
	if specRecorder.Code != http.StatusOK {
		t.Fatalf("expected spec status 200, got %d", specRecorder.Code)
	}
	if !strings.Contains(specRecorder.Body.String(), "/api-docs/part2-schemas/V3.1.1/openapi.yaml#") {
		t.Fatal("expected spec to contain localized part2 schema path")
	}

	schemaReq := httptest.NewRequest(http.MethodGet, "/api-docs/part2-schemas/V3.1.1/openapi.yaml", nil)
	schemaRecorder := httptest.NewRecorder()
	r.ServeHTTP(schemaRecorder, schemaReq)
	if schemaRecorder.Code != http.StatusOK {
		t.Fatalf("expected schema status 200, got %d", schemaRecorder.Code)
	}
	if !strings.Contains(schemaRecorder.Body.String(), "openapi: 3.0.3") {
		t.Fatal("expected schema response to contain OpenAPI content")
	}
}

func TestInjectServerURL_PreservesServerBasePathFromSpec(t *testing.T) {
	spec := []byte("openapi: 3.0.3\nservers:\n- url: 'https://admin-shell.io/api/v3'\npaths:\n  /packages:\n    get:\n      responses:\n        '200':\n          description: ok\n")
	injected := injectServerURL(spec, "http://localhost:5004")
	if !strings.Contains(string(injected), "- url: 'http://localhost:5004/api/v3'") {
		t.Fatal("expected injected server URL to preserve /api/v3 base path")
	}
}
