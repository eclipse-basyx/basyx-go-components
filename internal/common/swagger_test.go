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

package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestDetectPart2SchemaVersion(t *testing.T) {
	spec := []byte("openapi: 3.0.3\ninfo:\n  version: V3.2.0\n")
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

	specRelative := []byte("openapi: 3.0.3\ninfo:\n  version: V3.2.0\npaths:\n  /x:\n    get:\n      responses:\n        '400':\n          $ref: '../Part2-API-Schemas/openapi.yaml#/components/responses/bad-request'\n")
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
	if !strings.Contains(specRecorder.Body.String(), "\n  /verify:\n") {
		t.Fatal("expected spec to contain injected /verify endpoint")
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

func TestAddSwaggerUIDoesNotInjectVerifyEndpointWhenDisabled(t *testing.T) {
	r := chi.NewRouter()
	includeVerifyEndpoint := false
	AddSwaggerUI(r, SwaggerUIConfig{
		Title:                 "test",
		SpecURL:               "/api-docs/openapi.yaml",
		UIPath:                "/swagger",
		SpecPath:              "/api-docs/openapi.yaml",
		SpecContent:           []byte("openapi: 3.0.3\npaths:\n  /x:\n    get:\n      responses:\n        '200':\n          description: ok\n"),
		BasePath:              "/",
		IncludeVerifyEndpoint: &includeVerifyEndpoint,
	})

	specReq := httptest.NewRequest(http.MethodGet, "/api-docs/openapi.yaml", nil)
	specRecorder := httptest.NewRecorder()
	r.ServeHTTP(specRecorder, specReq)
	if specRecorder.Code != http.StatusOK {
		t.Fatalf("expected spec status 200, got %d", specRecorder.Code)
	}
	if strings.Contains(specRecorder.Body.String(), "\n  /verify:\n") {
		t.Fatal("expected spec to not contain injected /verify endpoint when disabled")
	}
}

func TestInjectVerifyEndpoint_DoesNotDuplicateExistingPath(t *testing.T) {
	spec := []byte("openapi: 3.0.3\npaths:\n  /verify:\n    post:\n      summary: Existing\n")
	injected := injectVerifyEndpoint(spec)
	if string(injected) != string(spec) {
		t.Fatal("expected existing /verify path to remain unchanged")
	}
}

func TestInjectVerifyEndpoint_ReflectsSupportedVerifyRequestFormats(t *testing.T) {
	injected := string(injectVerifyEndpoint([]byte("openapi: 3.0.3\npaths:\n")))
	if strings.Contains(injected, "- type: array") {
		t.Fatal("expected injected /verify requestBody to not allow top-level JSON arrays")
	}
	if !strings.Contains(injected, "application/aasx+xml:") {
		t.Fatal("expected injected /verify requestBody to document application/aasx+xml")
	}
	if !strings.Contains(injected, "application/aasx+json:") {
		t.Fatal("expected injected /verify requestBody to document application/aasx+json")
	}
}

func TestInjectServerURL_DoesNotInheritServerBasePathFromSpec(t *testing.T) {
	spec := []byte("openapi: 3.0.3\nservers:\n- url: 'https://admin-shell.io/api/v3'\npaths:\n  /packages:\n    get:\n      responses:\n        '200':\n          description: ok\n")
	injected := injectServerURL(spec, "http://localhost:5004")
	if !strings.Contains(string(injected), "- url: 'http://localhost:5004'") {
		t.Fatal("expected injected server URL to not inherit /api/v3 base path")
	}
}
