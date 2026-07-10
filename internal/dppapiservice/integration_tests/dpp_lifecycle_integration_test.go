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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package integration_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	dppComposeTestTimeout        = 8 * time.Minute
	lifecycleTechnicalDataSpec   = "urn:example:semantic:technical-data"
	lifecycleCarbonFootprintSpec = "https://admin-shell.io/idta/CarbonFootprint/CarbonFootprint/1/0"
)

func TestDPPLifecycleWithDockerCompose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker Compose integration test in short mode")
	}
	requireDockerCompose(t)

	port := reserveLocalPort(t)
	composeEnv := dppComposeEnvironment{apiPort: port}
	projectName := fmt.Sprintf("dpp-lifecycle-it-%d", time.Now().UnixNano())
	composeFile := "docker-compose.yml"
	ctx, cancel := context.WithTimeout(context.TODO(), dppComposeTestTimeout)
	defer cancel()

	composeDown(t, context.TODO(), composeFile, projectName, composeEnv)
	composeUp(ctx, t, composeFile, projectName, composeEnv)
	t.Cleanup(func() {
		composeDown(t, context.TODO(), composeFile, projectName, composeEnv)
	})

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForDPPAPI(t, ctx, baseURL)

	client := &http.Client{Timeout: 10 * time.Second}
	now := time.Now().UTC().Truncate(time.Second)
	idSuffix := strings.ReplaceAll(projectName, "-", "")
	dppID := "https://www.example.org/dpp/" + idSuffix
	encodedDPPID := encodedPathParam(dppID)
	productID := "https://www.example.org/" + idSuffix
	encodedProductID := encodedPathParam(productID)
	document := lifecycleDPPDocument(dppID, productID, now)

	createBody := doJSON(t, client, http.MethodPost, baseURL+"/v1/dpps", document, http.StatusCreated)
	assertJSONPathEquals(t, createBody, "digitalProductPassportId", dppID)

	optionalDPPID := "https://www.example.org/dpp/optional/" + idSuffix
	optionalDocument := lifecycleDPPDocument(optionalDPPID, "https://www.example.org/optional/"+idSuffix, now)
	delete(optionalDocument, "facilityId")
	delete(optionalDocument, "contentSpecificationIds")
	optionalCreateBody := doJSON(t, client, http.MethodPost, baseURL+"/v1/dpps", optionalDocument, http.StatusCreated)
	assertJSONPathEquals(t, optionalCreateBody, "digitalProductPassportId", optionalDPPID)
	optionalReadBody := doJSON(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedPathParam(optionalDPPID), nil, http.StatusOK)
	assertJSONFieldMissing(t, optionalReadBody, "facilityId")
	assertJSONFieldMissing(t, optionalReadBody, "contentSpecificationIds")

	readBody := doJSON(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID, nil, http.StatusOK)
	assertJSONPathEquals(t, readBody, "digitalProductPassportId", dppID)
	assertJSONPathEquals(t, readBody, "uniqueProductIdentifier", productID)
	assertDPPSectionPathEquals(t, readBody, lifecycleTechnicalDataSpec, "manufacturerName", "Acme GmbH")
	assertDPPSectionPathEquals(t, readBody, lifecycleTechnicalDataSpec, "manual.url", "https://example.test/manual.pdf")
	assertDPPSectionPathEquals(t, readBody, lifecycleTechnicalDataSpec, "manual.resourceTitle", "User Manual")
	assertDPPSectionPathEquals(t, readBody, lifecycleCarbonFootprintSpec, "PcfCo2eq", "4180.75")

	time.Sleep(30 * time.Millisecond)
	createdVersionDate := time.Now().UTC()
	time.Sleep(30 * time.Millisecond)
	createdVersionBody := doJSON(t, client, http.MethodGet, historyURL(baseURL, encodedDPPID, createdVersionDate, "compressed"), nil, http.StatusOK)
	assertDPPSectionPathEquals(t, createdVersionBody, lifecycleTechnicalDataSpec, "manufacturerName", "Acme GmbH")

	fullBody := doJSON(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID+"?representation=full", nil, http.StatusOK)
	assertFullDPPSectionObjectType(t, fullBody, lifecycleTechnicalDataSpec, "DataElementCollection")
	assertDPPElementObjectType(t, fullBody, lifecycleTechnicalDataSpec, "dimensions", "DataElementCollection")
	assertDPPElementObjectType(t, fullBody, lifecycleTechnicalDataSpec, "manufacturerName", "SingleValuedDataElement")
	assertDPPElementObjectType(t, fullBody, lifecycleTechnicalDataSpec, "manual", "RelatedResource")
	assertDPPElementObjectType(t, fullBody, lifecycleTechnicalDataSpec, "productDescription", "MultiLanguageDataElement")
	assertDPPElementObjectType(t, fullBody, lifecycleTechnicalDataSpec, "serialNumbers", "MultiValuedDataElement")
	assertDPPElementValue(t, fullBody, lifecycleTechnicalDataSpec, "warrantyMonths", "valueDataType", "xsd:long")
	assertDPPElementValue(t, fullBody, lifecycleTechnicalDataSpec, "manual", "resourceTitle", "User Manual")
	assertDPPElementValue(t, fullBody, lifecycleTechnicalDataSpec, "manual", "language", "en-GB")
	assertFullDPPSectionObjectType(t, fullBody, lifecycleCarbonFootprintSpec, "DataElementCollection")
	assertDPPElementObjectType(t, fullBody, lifecycleCarbonFootprintSpec, "PcfCo2eq", "SingleValuedDataElement")

	productBody := doJSON(t, client, http.MethodGet, baseURL+"/v1/dppsByProductId/"+encodedProductID, nil, http.StatusOK)
	assertJSONPathEquals(t, productBody, "digitalProductPassportId", dppID)

	searchBody := doJSON(t, client, http.MethodPost, baseURL+"/v1/dppsByProductIds?limit=1", map[string]any{
		"productIds": []string{productID},
	}, http.StatusOK)
	assertStringSliceContains(t, searchBody["items"], dppID)

	time.Sleep(30 * time.Millisecond)
	beforePatchDate := time.Now().UTC()
	time.Sleep(30 * time.Millisecond)
	patchBody := doJSON(t, client, http.MethodPatch, baseURL+"/v1/dpps/"+encodedDPPID, map[string]any{
		lifecycleTechnicalDataSpec: map[string]any{
			"manufacturerName": "Acme Updated GmbH",
			"warrantyMonths":   36,
		},
	}, http.StatusOK)
	assertDPPSectionPathEquals(t, patchBody, lifecycleTechnicalDataSpec, "manufacturerName", "Acme Updated GmbH")
	assertDPPSectionPathEquals(t, patchBody, lifecycleTechnicalDataSpec, "warrantyMonths", "36")

	prePatchVersionBody := doJSON(t, client, http.MethodGet, historyURL(baseURL, encodedDPPID, beforePatchDate, "compressed"), nil, http.StatusOK)
	assertDPPSectionPathEquals(t, prePatchVersionBody, lifecycleTechnicalDataSpec, "manufacturerName", "Acme GmbH")

	time.Sleep(30 * time.Millisecond)
	updatedVersionDate := time.Now().UTC()
	time.Sleep(30 * time.Millisecond)
	updatedVersionBody := doJSON(t, client, http.MethodGet, historyURL(baseURL, encodedDPPID, updatedVersionDate, "compressed"), nil, http.StatusOK)
	assertDPPSectionPathEquals(t, updatedVersionBody, lifecycleTechnicalDataSpec, "manufacturerName", "Acme Updated GmbH")

	fullVersionBody := doJSON(t, client, http.MethodGet, historyURL(baseURL, encodedDPPID, updatedVersionDate, "full"), nil, http.StatusOK)
	assertFullDPPSectionObjectType(t, fullVersionBody, lifecycleTechnicalDataSpec, "DataElementCollection")
	assertDPPElementObjectType(t, fullVersionBody, lifecycleTechnicalDataSpec, "dimensions", "DataElementCollection")
	assertDPPElementObjectType(t, fullVersionBody, lifecycleTechnicalDataSpec, "manufacturerName", "SingleValuedDataElement")

	elementIDPath := encodedPathParam(dppElementJSONPath(lifecycleTechnicalDataSpec, "manufacturerName"))
	elementBody := doJSONAny(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID+"/elements/"+elementIDPath, nil, http.StatusOK)
	assertScalarEquals(t, elementBody, "Acme Updated GmbH")

	fullElementBody := doJSONAny(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID+"/elements/"+elementIDPath+"?representation=full", nil, http.StatusOK)
	assertDataElementObjectType(t, fullElementBody, "manufacturerName", "SingleValuedDataElement")

	carbonPath := encodedPathParam(dppElementJSONPath(lifecycleCarbonFootprintSpec, "PcfCo2eq"))
	updatedCarbonBody := doJSONAny(t, client, http.MethodPatch, baseURL+"/v1/dpps/"+encodedDPPID+"/elements/"+carbonPath, 4200.5, http.StatusOK)
	assertScalarEquals(t, updatedCarbonBody, "4200.5")

	energyClassPath := encodedPathParam(dppElementJSONPath(lifecycleTechnicalDataSpec, "energyClass"))
	updatedElementBody := doJSONAny(t, client, http.MethodPatch, baseURL+"/v1/dpps/"+encodedDPPID+"/elements/"+energyClassPath, "B", http.StatusOK)
	assertScalarEquals(t, updatedElementBody, "B")
	serialNumberPath := encodedPathParam(dppElementJSONPath(lifecycleTechnicalDataSpec, "serialNumbers") + "[0]")
	serialNumberBody := doJSONAny(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID+"/elements/"+serialNumberPath, nil, http.StatusOK)
	assertScalarEquals(t, serialNumberBody, "SN-001")
	fullSerialNumberBody := doJSONAny(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID+"/elements/"+serialNumberPath+"?representation=full", nil, http.StatusOK)
	assertDataElementObjectType(t, fullSerialNumberBody, "serialNumbers0", "SingleValuedDataElement")
	updatedSerialNumberBody := doJSONAny(t, client, http.MethodPatch, baseURL+"/v1/dpps/"+encodedDPPID+"/elements/"+serialNumberPath, "SN-UPDATED", http.StatusOK)
	assertScalarEquals(t, updatedSerialNumberBody, "SN-UPDATED")

	readAfterElementUpdate := doJSON(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID, nil, http.StatusOK)
	assertDPPSectionPathEquals(t, readAfterElementUpdate, lifecycleTechnicalDataSpec, "energyClass", "B")
	assertDPPSectionArrayValue(t, readAfterElementUpdate, lifecycleTechnicalDataSpec, "serialNumbers", 0, "SN-UPDATED")
	assertDPPSectionPathEquals(t, readAfterElementUpdate, lifecycleCarbonFootprintSpec, "PcfCo2eq", "4200.5")

	time.Sleep(30 * time.Millisecond)
	beforeDeleteDate := time.Now().UTC()
	time.Sleep(30 * time.Millisecond)
	doJSON(t, client, http.MethodDelete, baseURL+"/v1/dpps/"+encodedDPPID, nil, http.StatusNoContent)
	preDeleteVersionBody := doJSON(t, client, http.MethodGet, historyURL(baseURL, encodedDPPID, beforeDeleteDate, "compressed"), nil, http.StatusOK)
	assertDPPSectionPathEquals(t, preDeleteVersionBody, lifecycleTechnicalDataSpec, "energyClass", "B")
	doJSON(t, client, http.MethodGet, historyURL(baseURL, encodedDPPID, time.Now().UTC(), "compressed"), nil, http.StatusNotFound)
	doJSON(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID, nil, http.StatusNotFound)
	doJSON(t, client, http.MethodDelete, baseURL+"/v1/dpps/"+encodedPathParam(optionalDPPID), nil, http.StatusNoContent)
}

func lifecycleDPPDocument(dppID string, productID string, now time.Time) map[string]any {
	return map[string]any{
		"digitalProductPassportId": dppID,
		"uniqueProductIdentifier":  productID,
		"granularity":              "Item",
		"dppSchemaVersion":         "1.0.0",
		"dppStatus":                "active",
		"lastUpdate":               now.Format(time.RFC3339Nano),
		"economicOperatorId":       "operator-123",
		"facilityId":               "facility-456",
		"contentSpecificationIds":  []string{lifecycleTechnicalDataSpec, lifecycleCarbonFootprintSpec},
		lifecycleTechnicalDataSpec: map[string]any{
			"manufacturerName": "Acme GmbH",
			"warrantyMonths":   24,
			"energyClass":      "A",
			"productDescription": []map[string]any{
				{"language": "en-IE", "value": "One Thing"},
				{"language": "es-ES", "value": "Una Cosa"},
			},
			"serialNumbers": []string{"SN-001", "SN-002"},
			"dimensions": map[string]any{
				"widthMm":  120,
				"heightMm": 80,
			},
			"manual": map[string]any{
				"url":           "https://example.test/manual.pdf",
				"contentType":   "application/pdf",
				"language":      "en-GB",
				"resourceTitle": "User Manual",
			},
		},
		lifecycleCarbonFootprintSpec: map[string]any{
			"PcfCo2eq":                          4180.75,
			"ReferenceImpactUnitForCalculation": "kg CO2e",
			"PcfCalculationMethods":             []string{"ISO 14067"},
		},
	}
}

func requireDockerCompose(t *testing.T) {
	t.Helper()
	cmd := exec.Command("docker", "compose", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("Docker Compose is required for this integration test: %v: %s", err, output)
	}
}

//nolint:revive
type dppComposeEnvironment struct {
	apiPort       int
	keycloakPort  int
	securityEnv   string
	keycloakRealm string
}

func (environment dppComposeEnvironment) values() []string {
	values := []string{fmt.Sprintf("DPP_IT_PORT=%d", environment.apiPort)}
	if environment.keycloakPort != 0 {
		values = append(values, fmt.Sprintf("DPP_IT_KEYCLOAK_PORT=%d", environment.keycloakPort))
	}
	if environment.securityEnv != "" {
		values = append(values, "DPP_IT_SECURITY_ENV="+environment.securityEnv)
	}
	if environment.keycloakRealm != "" {
		values = append(values, "DPP_IT_KEYCLOAK_REALM="+environment.keycloakRealm)
	}
	return values
}

func composeUp(ctx context.Context, t *testing.T, composeFile string, projectName string, environment dppComposeEnvironment) {
	t.Helper()
	runComposeCommand(ctx, t, environment, "docker compose build failed", "compose", "-f", composeFile, "-p", projectName, "build", "--no-cache")
	runComposeCommand(ctx, t, environment, "docker compose up failed", "compose", "-f", composeFile, "-p", projectName, "up", "-d")
}

//nolint:revive
func composeDown(t *testing.T, ctx context.Context, composeFile string, projectName string, environment dppComposeEnvironment) {
	t.Helper()
	args := []string{"compose", "-f", composeFile, "-p", projectName, "down", "-v", "--remove-orphans"}
	cmd := composeCommand(ctx, environment, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("docker compose down failed: %v\n%s", err, output)
	}
}

func runComposeCommand(ctx context.Context, t *testing.T, environment dppComposeEnvironment, errorMessage string, args ...string) {
	t.Helper()
	cmd := composeCommand(ctx, environment, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", errorMessage, err, output)
	}
}

func composeCommand(ctx context.Context, environment dppComposeEnvironment, args ...string) *exec.Cmd {
	//nolint:gosec
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = append(os.Environ(), environment.values()...)
	return cmd
}

//nolint:revive
func waitForDPPAPI(t *testing.T, ctx context.Context, baseURL string) {
	t.Helper()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api-docs/openapi.yaml", nil)
		if err != nil {
			t.Fatalf("create readiness request: %v", err)
		}
		response, err := http.DefaultClient.Do(request) //nolint:gosec
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("DPP API did not become ready: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func reserveLocalPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local port: %v", err)
	}
	defer func() {
		_ = listener.Close()
	}()
	return listener.Addr().(*net.TCPAddr).Port
}

func doJSON(t *testing.T, client *http.Client, method string, requestURL string, body any, expectedStatus int) map[string]any {
	t.Helper()
	responseBody := doJSONAny(t, client, method, requestURL, body, expectedStatus)
	if responseBody == nil {
		return nil
	}
	object, ok := responseBody.(map[string]any)
	if !ok {
		t.Fatalf("%s %s response = %#v, want object", method, requestURL, responseBody)
	}
	return object
}

func doJSONAny(t *testing.T, client *http.Client, method string, requestURL string, body any, expectedStatus int) any {
	t.Helper()
	payload, err := encodeBody(body)
	if err != nil {
		t.Fatalf("encode request body: %v", err)
	}
	request, err := http.NewRequest(method, requestURL, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create request %s %s: %v", method, requestURL, err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request) //nolint:gosec
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, requestURL, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	var responseBody any
	if response.StatusCode != http.StatusNoContent {
		decoder := json.NewDecoder(response.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&responseBody); err != nil {
			t.Fatalf("decode response for %s %s: %v", method, requestURL, err)
		}
	}
	if response.StatusCode != expectedStatus {
		t.Fatalf("%s %s status = %d, want %d, body = %#v", method, requestURL, response.StatusCode, expectedStatus, responseBody)
	}
	return responseBody
}

func encodeBody(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	if text, ok := body.(string); ok {
		return json.Marshal(text)
	}
	return json.Marshal(body)
}

func encodedPathParam(value string) string {
	return url.PathEscape(value)
}

func dppElementJSONPath(sectionName string, elementNames ...string) string {
	parts := make([]string, 0, len(elementNames)+2)
	parts = append(parts, "$", jsonPathMember(sectionName))
	for _, elementName := range elementNames {
		parts = append(parts, jsonPathMember(elementName))
	}
	return strings.Join(parts, "")
}

func jsonPathMember(value string) string {
	return "['" + strings.ReplaceAll(value, "'", "\\'") + "']"
}

func historyURL(baseURL string, encodedDPPID string, date time.Time, representation string) string {
	query := url.Values{}
	query.Set("date", date.Format(time.RFC3339Nano))
	query.Set("representation", representation)
	return baseURL + "/v1/dppsByIdAndDate/" + encodedDPPID + "?" + query.Encode()
}

func assertJSONPathEquals(t *testing.T, body map[string]any, path string, expected string) {
	t.Helper()
	value, err := valueAtPath(body, path)
	if err != nil {
		t.Fatal(err)
	}
	if value != expected {
		t.Fatalf("%s = %#v, want %q", path, value, expected)
	}
}

func assertJSONFieldMissing(t *testing.T, body map[string]any, field string) {
	t.Helper()
	if _, ok := body[field]; ok {
		t.Fatalf("%s unexpectedly present in %#v", field, body)
	}
}

func assertDPPSectionPathEquals(t *testing.T, body map[string]any, sectionName string, path string, expected string) {
	t.Helper()
	section, ok := body[sectionName].(map[string]any)
	if !ok {
		t.Fatalf("%s section = %#v, want object", sectionName, body[sectionName])
	}
	value, err := valueAtPath(section, path)
	if err != nil {
		t.Fatal(err)
	}
	if value != expected {
		t.Fatalf("%s.%s = %#v, want %q", sectionName, path, value, expected)
	}
}

func assertDPPSectionArrayValue(t *testing.T, body map[string]any, sectionName string, path string, index int, expected string) {
	t.Helper()
	section, ok := body[sectionName].(map[string]any)
	if !ok {
		t.Fatalf("%s section = %#v, want object", sectionName, body[sectionName])
	}
	value, err := valueAtPath(section, path)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := value.([]any)
	if !ok || index < 0 || index >= len(items) {
		t.Fatalf("%s.%s = %#v, want array containing index %d", sectionName, path, value, index)
	}
	if items[index] != expected {
		t.Fatalf("%s.%s[%d] = %#v, want %q", sectionName, path, index, items[index], expected)
	}
}

func assertDPPElementObjectType(t *testing.T, body map[string]any, sectionName string, elementID string, expectedObjectType string) {
	t.Helper()
	element := fullDPPElement(t, body, sectionName, elementID)
	if element["objectType"] != expectedObjectType {
		t.Fatalf("%s.%s objectType = %#v, want %q", sectionName, elementID, element["objectType"], expectedObjectType)
	}
}

func assertDPPElementValue(t *testing.T, body map[string]any, sectionName string, elementID string, field string, expected string) {
	t.Helper()
	element := fullDPPElement(t, body, sectionName, elementID)
	if element[field] != expected {
		t.Fatalf("%s.%s.%s = %#v, want %q", sectionName, elementID, field, element[field], expected)
	}
}

func assertFullDPPSectionObjectType(t *testing.T, body map[string]any, sectionName string, expectedObjectType string) {
	t.Helper()
	section := fullDPPSection(t, body, sectionName)
	if section["objectType"] != expectedObjectType {
		t.Fatalf("%s objectType = %#v, want %q", sectionName, section["objectType"], expectedObjectType)
	}
}

func fullDPPElement(t *testing.T, body map[string]any, sectionName string, elementID string) map[string]any {
	t.Helper()
	section := fullDPPSection(t, body, sectionName)
	elements, ok := section["elements"].([]any)
	if !ok {
		t.Fatalf("%s.elements = %#v, want array", sectionName, section["elements"])
	}
	if element, ok := findFullElement(elements, elementID); ok {
		return element
	}
	t.Fatalf("%s.elements does not contain elementId %q: %#v", sectionName, elementID, elements)
	return nil
}

func fullDPPSection(t *testing.T, body map[string]any, sectionName string) map[string]any {
	t.Helper()
	elements, ok := body["elements"].([]any)
	if !ok {
		t.Fatalf("elements = %#v, want array", body["elements"])
	}
	if section, ok := findFullElementByDictionaryReference(elements, sectionName); ok {
		return section
	}
	if section, ok := findFullElement(elements, upperFirst(sectionName)); ok {
		return section
	}
	t.Fatalf("elements does not contain section %q: %#v", sectionName, elements)
	return nil
}

func findFullElement(elements []any, elementID string) (map[string]any, bool) {
	for _, item := range elements {
		element, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if element["elementId"] == elementID {
			return element, true
		}
	}
	return nil, false
}

func findFullElementByDictionaryReference(elements []any, dictionaryReference string) (map[string]any, bool) {
	for _, item := range elements {
		element, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if element["dictionaryReference"] == dictionaryReference {
			return element, true
		}
	}
	return nil, false
}

func upperFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func assertDataElementObjectType(t *testing.T, body any, elementID string, expectedObjectType string) {
	t.Helper()
	element, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("element response = %#v, want object", body)
	}
	if element["elementId"] != elementID {
		t.Fatalf("elementId = %#v, want %q", element["elementId"], elementID)
	}
	if element["objectType"] != expectedObjectType {
		t.Fatalf("%s objectType = %#v, want %q", elementID, element["objectType"], expectedObjectType)
	}
}

func assertScalarEquals(t *testing.T, body any, expected string) {
	t.Helper()
	if body == expected {
		return
	}
	object, ok := body.(map[string]any)
	if ok {
		value, ok := object["value"]
		if ok {
			if value != expected {
				t.Fatalf("element value = %#v, want %q", value, expected)
			}
			return
		}
		if len(object) == 1 {
			for _, onlyValue := range object {
				if onlyValue != expected {
					t.Fatalf("element response = %#v, want %q", onlyValue, expected)
				}
				return
			}
		}
	}
	t.Fatalf("element response = %#v, want scalar %q", body, expected)
}

func assertStringSliceContains(t *testing.T, value any, expected string) {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("items = %#v, want array", value)
	}
	for _, item := range items {
		if item == expected {
			return
		}
	}
	t.Fatalf("items = %#v, want to contain %q", items, expected)
}

func valueAtPath(body map[string]any, path string) (any, error) {
	var current any = body
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s parent is %#v, want object", path, current)
		}
		value, ok := object[part]
		if !ok {
			return nil, fmt.Errorf("%s missing at %s in %#v", path, part, object)
		}
		current = value
	}
	if current == nil {
		return nil, errors.New(path + " is nil")
	}
	return current, nil
}
