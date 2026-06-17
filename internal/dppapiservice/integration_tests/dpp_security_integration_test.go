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
// Author: Jannik Fried ( Fraunhofer IESE )

package integration_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const dppSecurityRealm = "basyx"

func TestDPPSecurityWithDockerCompose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker Compose integration test in short mode")
	}
	requireDockerCompose(t)

	port := reserveLocalPort(t)
	keycloakPort := reserveLocalPort(t)
	issuerURL := dppSecurityIssuerURL(keycloakPort)
	securityEnv := writeDPPSecurityEnvironment(t, issuerURL)
	keycloakRealm := writeDPPSecurityKeycloakRealm(t)
	composeEnv := dppComposeEnvironment{
		apiPort:       port,
		keycloakPort:  keycloakPort,
		securityEnv:   securityEnv,
		keycloakRealm: keycloakRealm,
	}
	projectName := "dpp-security-it-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	composeFile := "docker-compose-security.yml"
	ctx, cancel := context.WithTimeout(context.TODO(), dppComposeTestTimeout)
	defer cancel()

	composeDown(t, context.TODO(), composeFile, projectName, composeEnv)
	composeUp(ctx, t, composeFile, projectName, composeEnv)
	t.Cleanup(func() {
		composeDown(t, context.TODO(), composeFile, projectName, composeEnv)
	})

	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	waitForDPPAPI(t, ctx, baseURL)

	client := &http.Client{Timeout: 10 * time.Second}
	tokenEndpoint := issuerURL + "/protocol/openid-connect/token"
	viewerToken := passwordGrantToken(t, client, tokenEndpoint, "usera", "pwd")
	editorToken := passwordGrantToken(t, client, tokenEndpoint, "userx", "pwd")

	dppID := "https://www.example.org/dpp/security-" + strings.ReplaceAll(projectName, "-", "")
	productID := "https://www.example.org/products/security-" + strings.ReplaceAll(projectName, "-", "")
	encodedDPPID := encodedPathParam(dppID)
	document := lifecycleDPPDocument(dppID, productID, time.Now().UTC())

	doJSONAnyAuth(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID, "", nil, http.StatusForbidden)
	doJSONAnyAuth(t, client, http.MethodGet, baseURL+"/v1/not-a-dpp-route", "", nil, http.StatusNotFound)
	doJSONAnyAuth(t, client, http.MethodPost, baseURL+"/v1/dpps", viewerToken, document, http.StatusForbidden)

	createBody := doJSONAuth(t, client, http.MethodPost, baseURL+"/v1/dpps", editorToken, document, http.StatusCreated)
	assertJSONPathEquals(t, createBody, "digitalProductPassportId", dppID)

	readBody := doJSONAuth(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedDPPID, viewerToken, nil, http.StatusOK)
	assertJSONPathEquals(t, readBody, "digitalProductPassportId", dppID)

	doJSONAnyAuth(t, client, http.MethodPatch, baseURL+"/v1/dpps/"+encodedDPPID, viewerToken, map[string]any{
		"technicalData": map[string]any{
			"manufacturerName": "Denied GmbH",
		},
	}, http.StatusForbidden)

	patchBody := doJSONAuth(t, client, http.MethodPatch, baseURL+"/v1/dpps/"+encodedDPPID, editorToken, map[string]any{
		"technicalData": map[string]any{
			"manufacturerName": "Secured Updated GmbH",
		},
	}, http.StatusOK)
	assertJSONPathEquals(t, patchBody, "technicalData.manufacturerName", "Secured Updated GmbH")

	elementBody := doJSONAnyAuth(t, client, http.MethodPut, baseURL+"/v1/dpps/"+encodedDPPID+"/elements/technicalData/energyClass", editorToken, "B", http.StatusOK)
	assertScalarEquals(t, elementBody, "B")

	doJSONAnyAuth(t, client, http.MethodDelete, baseURL+"/v1/dpps/"+encodedDPPID, editorToken, nil, http.StatusNoContent)
}

func dppSecurityIssuerURL(keycloakPort int) string {
	return fmt.Sprintf("http://keycloak.localhost:%d/realms/%s", keycloakPort, dppSecurityRealm)
}

func writeDPPSecurityEnvironment(t *testing.T, issuerURL string) string {
	t.Helper()

	directory, err := os.MkdirTemp(".", ".dpp-security-env-*")
	if err != nil {
		t.Fatalf("create DPP security env: %v", err)
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		_ = os.RemoveAll(directory)
		t.Fatalf("resolve DPP security env path: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(directory)
	})
	//nolint:gosec // Generated test fixtures must be readable by non-root Docker containers.
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatalf("chmod DPP security env: %v", err)
	}

	copyDPPSecurityAccessRules(t, directory)
	writeDPPSecurityTrustlist(t, directory, issuerURL)
	return directory
}

func copyDPPSecurityAccessRules(t *testing.T, directory string) {
	t.Helper()

	accessRules, err := os.ReadFile(filepath.Join("security_env", "access-rules.json")) // #nosec G304 -- test fixture path is static.
	if err != nil {
		t.Fatalf("read DPP access rules: %v", err)
	}
	//nolint:gosec // Generated test fixtures must be readable by non-root Docker containers.
	if err := os.WriteFile(filepath.Join(directory, "access-rules.json"), accessRules, 0o644); err != nil {
		t.Fatalf("write DPP access rules: %v", err)
	}
}

func writeDPPSecurityTrustlist(t *testing.T, directory string, issuerURL string) {
	t.Helper()

	trustlist := []map[string]any{{
		"issuer":   issuerURL,
		"audience": "",
		"scopes":   []string{"email", "profile"},
	}}
	data, err := json.MarshalIndent(trustlist, "", "  ")
	if err != nil {
		t.Fatalf("encode DPP trustlist: %v", err)
	}
	//nolint:gosec // Generated test fixtures must be readable by non-root Docker containers.
	if err := os.WriteFile(filepath.Join(directory, "trustlist.json"), data, 0o644); err != nil {
		t.Fatalf("write DPP trustlist: %v", err)
	}
}

func writeDPPSecurityKeycloakRealm(t *testing.T) string {
	t.Helper()

	directory, err := os.MkdirTemp(".", ".dpp-keycloak-realm-*")
	if err != nil {
		t.Fatalf("create DPP Keycloak realm dir: %v", err)
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		_ = os.RemoveAll(directory)
		t.Fatalf("resolve DPP Keycloak realm path: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(directory)
	})
	//nolint:gosec // Generated test fixtures must be readable by non-root Docker containers.
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatalf("chmod DPP Keycloak realm dir: %v", err)
	}

	realm, err := readDPPSecurityKeycloakRealm()
	if err != nil {
		t.Fatalf("read DPP Keycloak realm: %v", err)
	}
	realm["sslRequired"] = "none"
	data, err := json.MarshalIndent(realm, "", "  ")
	if err != nil {
		t.Fatalf("encode DPP Keycloak realm: %v", err)
	}
	//nolint:gosec // Generated test fixtures must be readable by non-root Docker containers.
	if err := os.WriteFile(filepath.Join(directory, "basyx-realm.json"), data, 0o644); err != nil {
		t.Fatalf("write DPP Keycloak realm: %v", err)
	}
	return directory
}

func readDPPSecurityKeycloakRealm() (map[string]any, error) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "BaSyxSecuredExample", "keycloak", "realm", "basyx-realm.json")) // #nosec G304 -- test fixture path is static.
	if err != nil {
		return nil, err
	}
	var realm map[string]any
	if err := json.Unmarshal(data, &realm); err != nil {
		return nil, err
	}
	return realm, nil
}

func passwordGrantToken(t *testing.T, client *http.Client, tokenEndpoint string, username string, password string) string {
	t.Helper()

	form := url.Values{}
	form.Set("client_id", "basyx-ui")
	form.Set("grant_type", "password")
	form.Set("username", username)
	form.Set("password", password)

	request, err := http.NewRequest(http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("create token request: %v", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := client.Do(request) //nolint:gosec
	if err != nil {
		t.Fatalf("request token for %s: %v", username, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read token response for %s: %v", username, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("token status for %s = %d, want %d, body = %s", username, response.StatusCode, http.StatusOK, body)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode token response for %s: %v", username, err)
	}
	accessToken, ok := payload["access_token"].(string)
	if !ok || accessToken == "" {
		t.Fatalf("token response for %s did not contain access_token", username)
	}
	return accessToken
}

func doJSONAuth(t *testing.T, client *http.Client, method string, requestURL string, token string, body any, expectedStatus int) map[string]any {
	t.Helper()

	responseBody := doJSONAnyAuth(t, client, method, requestURL, token, body, expectedStatus)
	if responseBody == nil {
		return nil
	}
	object, ok := responseBody.(map[string]any)
	if !ok {
		t.Fatalf("%s %s response = %#v, want object", method, requestURL, responseBody)
	}
	return object
}

func doJSONAnyAuth(t *testing.T, client *http.Client, method string, requestURL string, token string, body any, expectedStatus int) any {
	t.Helper()

	payload, err := encodeBody(body)
	if err != nil {
		t.Fatalf("encode request body: %v", err)
	}
	request, err := http.NewRequest(method, requestURL, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create request %s %s: %v", method, requestURL, err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
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

	responseBody := decodeOptionalJSONResponse(t, response, method, requestURL)
	if response.StatusCode != expectedStatus {
		t.Fatalf("%s %s status = %d, want %d, body = %#v", method, requestURL, response.StatusCode, expectedStatus, responseBody)
	}
	return responseBody
}

func decodeOptionalJSONResponse(t *testing.T, response *http.Response, method string, requestURL string) any {
	t.Helper()

	if response.StatusCode == http.StatusNoContent {
		return nil
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response for %s %s: %v", method, requestURL, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	var responseBody any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&responseBody); err != nil {
		return string(data)
	}
	return responseBody
}
