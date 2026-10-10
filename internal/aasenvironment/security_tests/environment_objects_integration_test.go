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

package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	aasjsonization "github.com/FriedJannik/aas-go-sdk/jsonization"
	aasxmlization "github.com/FriedJannik/aas-go-sdk/xmlization"
	aasx "github.com/aas-core-works/aas-package3-golang/v2"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentIdentifiableAuthorization(t *testing.T) {
	provider := testenv.NewPasswordGrantTokenProvider(testKeycloakTokenURL, "basyx-ui", 10*time.Second)
	admin, err := provider.GetAccessToken(&testenv.TokenCredentials{User: "admin", Password: "pwd"})
	require.NoError(t, err)
	editor, err := provider.GetAccessToken(&testenv.TokenCredentials{User: "userx", Password: "pwd"})
	require.NoError(t, err)
	original := activePolicyVersionID(t, admin)
	t.Cleanup(func() {
		restored := clonePolicyVersion(t, original, admin)
		validatePolicyVersion(t, restored, admin)
		activatePolicyVersion(t, restored, admin)
	})
	installEnvironmentRules(t, admin)
	id := fmt.Sprintf("urn:test:environment:security:%d", time.Now().UnixNano())
	seed := environmentFixture(id, "Visible")
	other := environmentFixture(id+":other", "Hidden")
	requireEnvironmentUpload(t, admin, seed, http.StatusOK)
	requireEnvironmentUpload(t, admin, other, http.StatusOK)

	t.Run("type boundaries", func(t *testing.T) { checkEnvironmentTypeGrants(t, admin, editor, id) })
	t.Run("explicit foreign identifiers fail without exporting a subset", func(t *testing.T) { checkExplicitEnvironmentIdentifiers(t, admin, editor, id) })
	t.Run("formula and rule-bound element filters apply to all formats", func(t *testing.T) { checkEnvironmentFormulaAndFilters(t, admin, editor, id) })
	t.Run("create and update rights use actual state for every type", func(t *testing.T) { checkEnvironmentWriteRights(t, admin, editor, id) })
	t.Run("update formula checks both states and returns 403 through wrapper", func(t *testing.T) { checkEnvironmentUpdateStates(t, admin, editor, id) })
	t.Run("allowed earlier objects survive a later denial", func(t *testing.T) { checkEnvironmentPartialImport(t, admin, editor, id) })
	t.Run("AASX binaries follow visible owners and references", func(t *testing.T) { checkEnvironmentBinaryVisibility(t, admin, editor, id) })
	t.Run("pagination retains every permitted submodel", func(t *testing.T) { checkEnvironmentPagination(t, admin, editor, id) })
}

func checkEnvironmentTypeGrants(t *testing.T, admin, editor, id string) {
	t.Helper()
	for scope, collection := range map[string]string{"aas": "assetAdministrationShells", "sm": "submodels", "cd": "conceptDescriptions"} {
		for _, identifier := range []string{id, "*"} {
			t.Run(scope+"/"+identifier, func(t *testing.T) {
				installEnvironmentRules(t, admin, environmentObjectRule(scope, identifier, "READ"))
				status, body := environmentRequest(t, editor, "/serialization", "application/json")
				require.Equal(t, http.StatusOK, status, string(body))
				var result map[string][]map[string]any
				require.NoError(t, json.Unmarshal(body, &result))
				for _, key := range []string{"assetAdministrationShells", "submodels", "conceptDescriptions"} {
					if key != collection {
						require.Empty(t, result[key], "type grant leaked into %s", key)
					}
				}
				if identifier != "*" {
					require.Len(t, result[collection], 1)
					require.Equal(t, id, result[collection][0]["id"])
				}
			})
		}
	}
}

func checkExplicitEnvironmentIdentifiers(t *testing.T, admin, editor, id string) {
	t.Helper()
	installEnvironmentRules(t, admin, environmentObjectRule("sm", id, "READ"))
	query := url.Values{"submodelIds": {common.EncodeString(id), common.EncodeString(id + ":other")}}
	status, _ := environmentRequest(t, editor, "/serialization?"+query.Encode(), "application/json")
	require.Equal(t, http.StatusNotFound, status)
	query = url.Values{"aasIds": {common.EncodeString(id)}}
	status, _ = environmentRequest(t, editor, "/serialization?"+query.Encode(), "application/json")
	require.Equal(t, http.StatusNotFound, status)
}

func checkEnvironmentFormulaAndFilters(t *testing.T, admin, editor, id string) {
	t.Helper()
	rule := environmentObjectRule("sm", "*", "READ")
	rule["FORMULA"] = equalsQueryCondition("$sm#idShort", "Visible")
	rule["FILTERLIST"] = []any{map[string]any{"FRAGMENT": "$sme", "MATCH": true, "CONDITION": equalsQueryCondition("$sme#idShort", "Public")}}
	installEnvironmentRules(t, admin, rule)
	for _, accept := range []string{"application/json", "application/xml", "application/aasx+json", "application/aasx+xml"} {
		for _, query := range []string{"", "?submodelIds=" + common.EncodeString(id)} {
			status, body := environmentRequest(t, editor, "/serialization"+query, accept)
			require.Equal(t, http.StatusOK, status, string(body))
			content := string(body)
			if strings.Contains(accept, "aasx") {
				content = environmentPackageContents(t, body)
			}
			require.Contains(t, content, id)
			require.Contains(t, content, "public-value")
			require.NotContains(t, content, "secret-value")
			require.NotContains(t, content, id+":other")
		}
	}
	second := environmentObjectRule("sm", id+":other", "READ")
	installEnvironmentRules(t, admin, rule, second)
	status, body := environmentRequest(t, editor, "/serialization", "application/json")
	require.Equal(t, http.StatusOK, status)
	var result struct {
		Submodels []map[string]any `json:"submodels"`
	}
	require.NoError(t, json.Unmarshal(body, &result))
	for _, sm := range result.Submodels {
		serialized, marshalErr := json.Marshal(sm)
		require.NoError(t, marshalErr)
		if sm["id"] == id {
			require.NotContains(t, string(serialized), "secret-value")
		} else {
			require.Contains(t, string(serialized), "secret-value")
		}
	}
}

func checkEnvironmentWriteRights(t *testing.T, admin, editor, id string) {
	t.Helper()
	for scope, collection := range map[string]string{"aas": "assetAdministrationShells", "sm": "submodels", "cd": "conceptDescriptions"} {
		newID := id + ":create:" + scope
		payload := map[string]any{collection: environmentFixture(newID, "Visible")[collection]}
		installEnvironmentRules(t, admin, environmentObjectRule(scope, newID, "CREATE"))
		requireEnvironmentUpload(t, editor, payload, http.StatusOK)
		requireEnvironmentUpload(t, editor, payload, http.StatusForbidden)
		installEnvironmentRules(t, admin, environmentObjectRule(scope, newID, "UPDATE"))
		requireEnvironmentUpload(t, editor, payload, http.StatusOK)
		payload[collection] = environmentFixture(newID+":missing", "Visible")[collection]
		installEnvironmentRules(t, admin, environmentObjectRule(scope, "*", "UPDATE"))
		requireEnvironmentUpload(t, editor, payload, http.StatusForbidden)
		xmlID := newID + ":xml"
		installEnvironmentRules(t, admin, environmentObjectRule(scope, xmlID, "CREATE"))
		xmlPayload := environmentXML(t, map[string]any{collection: environmentFixture(xmlID, "Visible")[collection]})
		status, body := postEnvironmentFile(t, editor, "environment.xml", xmlPayload)
		require.Equal(t, http.StatusOK, status, string(body))
		status, body = postEnvironmentFile(t, editor, "environment.xml", xmlPayload)
		require.Equal(t, http.StatusForbidden, status, string(body))
	}
}

func environmentXML(t *testing.T, payload map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	var jsonable map[string]any
	require.NoError(t, json.Unmarshal(data, &jsonable))
	environment, err := aasjsonization.EnvironmentFromJsonable(jsonable)
	require.NoError(t, err)
	var buffer bytes.Buffer
	encoder := xml.NewEncoder(&buffer)
	require.NoError(t, aasxmlization.Marshal(encoder, environment, true))
	require.NoError(t, encoder.Flush())
	return buffer.Bytes()
}

func checkEnvironmentPagination(t *testing.T, admin, editor, id string) {
	t.Helper()
	installEnvironmentRules(t, admin)
	models := make([]any, 0, 105)
	for index := range 105 {
		models = append(models, environmentFixture(fmt.Sprintf("%s:page:%03d", id, index), "Pagination")["submodels"].([]any)[0])
	}
	requireEnvironmentUpload(t, admin, map[string]any{"submodels": models}, http.StatusOK)
	rule := environmentObjectRule("sm", "*", "READ")
	rule["FORMULA"] = equalsQueryCondition("$sm#idShort", "Pagination")
	installEnvironmentRules(t, admin, rule)
	status, body := environmentRequest(t, editor, "/serialization", "application/json")
	require.Equal(t, http.StatusOK, status, string(body))
	var result struct {
		Submodels []struct {
			ID string `json:"id"`
		} `json:"submodels"`
	}
	require.NoError(t, json.Unmarshal(body, &result))
	actual := make(map[string]struct{}, len(result.Submodels))
	for _, model := range result.Submodels {
		actual[model.ID] = struct{}{}
	}
	require.Len(t, actual, 105)
	for index := range 105 {
		require.Contains(t, actual, fmt.Sprintf("%s:page:%03d", id, index))
	}
}

func checkEnvironmentUpdateStates(t *testing.T, admin, editor, id string) {
	t.Helper()
	rule := environmentObjectRule("sm", "*", "UPDATE")
	rule["FORMULA"] = equalsQueryCondition("$sm#idShort", "Visible")
	installEnvironmentRules(t, admin, rule)
	requireEnvironmentUpload(t, editor, map[string]any{"submodels": environmentFixture(id, "Rejected")["submodels"]}, http.StatusForbidden)
	requireEnvironmentUpload(t, editor, map[string]any{"submodels": environmentFixture(id+":other", "Visible")["submodels"]}, http.StatusForbidden)
	status, stored := doAuthorizedRequest(t, http.MethodGet, testBaseURL+"/submodels/"+common.EncodeString(id), "", admin)
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, stored, `"idShort":"Visible"`)
	rule["FILTER"] = map[string]any{"FRAGMENT": "$sme", "CONDITION": map[string]bool{"$boolean": false}}
	installEnvironmentRules(t, admin, rule)
	requireEnvironmentUpload(t, editor, map[string]any{"submodels": environmentFixture(id, "Visible")["submodels"]}, http.StatusOK)
	status, stored = doAuthorizedRequest(t, http.MethodGet, testBaseURL+"/submodels/"+common.EncodeString(id), "", admin)
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, stored, "secret-value", "read projection must not truncate a replacement upload")
}

func checkEnvironmentPartialImport(t *testing.T, admin, editor, id string) {
	t.Helper()
	partialID := id + ":partial"
	installEnvironmentRules(t, admin, environmentObjectRule("cd", partialID, "CREATE"))
	requireEnvironmentUpload(t, editor, environmentFixture(partialID, "Visible"), http.StatusForbidden)
	status, _ := doAuthorizedRequest(t, http.MethodGet, testBaseURL+"/concept-descriptions/"+common.EncodeString(partialID), "", admin)
	require.Equal(t, http.StatusOK, status)
	status, _ = doAuthorizedRequest(t, http.MethodGet, testBaseURL+"/submodels/"+common.EncodeString(partialID), "", admin)
	require.Equal(t, http.StatusNotFound, status)
}

func checkEnvironmentBinaryVisibility(t *testing.T, admin, editor, id string) {
	t.Helper()
	binaryID := id + ":binary"
	installEnvironmentRules(t, admin, environmentObjectRule("aas", binaryID, "CREATE", "READ"), environmentObjectRule("sm", binaryID, "CREATE", "READ"))
	binaryEnvironment := environmentFixture(binaryID, "Visible")
	delete(binaryEnvironment, "conceptDescriptions")
	shells := binaryEnvironment["assetAdministrationShells"].([]any)
	shells[0].(map[string]any)["assetInformation"].(map[string]any)["defaultThumbnail"] = map[string]any{"path": "/thumbnail.png", "contentType": "image/png"}
	sm := binaryEnvironment["submodels"].([]any)[0].(map[string]any)
	sm["submodelElements"] = []any{map[string]any{"modelType": "File", "idShort": "PublicFile", "contentType": "text/plain", "value": "/public.txt"}, map[string]any{"modelType": "File", "idShort": "SecretFile", "contentType": "text/plain", "value": "/secret.txt"}}
	payload := buildEnvironmentPackage(t, binaryEnvironment)
	status, body := postEnvironmentFile(t, editor, "environment.aasx", payload)
	require.Equal(t, http.StatusOK, status, string(body))
	smRule := environmentObjectRule("sm", binaryID, "READ")
	smRule["FILTER"] = map[string]any{"FRAGMENT": "$sme", "MATCH": true, "CONDITION": equalsQueryCondition("$sme#idShort", "PublicFile")}
	installEnvironmentRules(t, admin, smRule)
	status, body = environmentRequest(t, editor, "/serialization", "application/aasx+json")
	require.Equal(t, http.StatusOK, status, string(body))
	content := environmentPackageContents(t, body)
	require.Contains(t, content, "PUBLIC-BINARY")
	require.NotContains(t, content, "SECRET-BINARY")
	require.NotContains(t, content, "THUMBNAIL-BINARY")
	require.NotContains(t, content, "SecretFile")
	require.NotContains(t, content, "defaultThumbnail")
	smRule["FILTER"] = map[string]any{"FRAGMENT": "$sme.SecretFile#value", "CONDITION": map[string]bool{"$boolean": false}}
	installEnvironmentRules(t, admin, smRule)
	status, body = environmentRequest(t, editor, "/serialization", "application/aasx+json")
	require.Equal(t, http.StatusOK, status, string(body))
	content = environmentPackageContents(t, body)
	require.Contains(t, content, "PUBLIC-BINARY")
	require.NotContains(t, content, "SECRET-BINARY")
	require.NotContains(t, content, "THUMBNAIL-BINARY")
	installEnvironmentRules(t, admin, environmentObjectRule("aas", binaryID, "READ"))
	status, body = environmentRequest(t, editor, "/serialization", "application/aasx+xml")
	require.Equal(t, http.StatusOK, status, string(body))
	content = environmentPackageContents(t, body)
	require.Contains(t, content, "THUMBNAIL-BINARY")
	require.NotContains(t, content, "PUBLIC-BINARY")
	require.NotContains(t, content, "SECRET-BINARY")
}

func environmentObjectRule(scope, id string, rights ...string) map[string]any {
	return map[string]any{
		"ACL":     map[string]any{"ATTRIBUTES": []any{map[string]string{"CLAIM": "role"}}, "RIGHTS": rights, "ACCESS": "ALLOW"},
		"OBJECTS": []any{map[string]string{"IDENTIFIABLE": fmt.Sprintf("$%s(%q)", scope, id)}},
		"FORMULA": map[string]bool{"$boolean": true},
	}
}

func installEnvironmentRules(t *testing.T, admin string, rules ...map[string]any) {
	t.Helper()
	role := func(value string) any {
		return map[string]any{"$eq": []any{map[string]any{"$attribute": map[string]string{"CLAIM": "role"}}, map[string]string{"$strVal": value}}}
	}
	all := []any{map[string]any{
		"ACL":     map[string]any{"ATTRIBUTES": []any{map[string]string{"CLAIM": "role"}}, "RIGHTS": []string{"ALL"}, "ACCESS": "ALLOW"},
		"OBJECTS": []any{map[string]string{"ROUTE": "*"}}, "FORMULA": role("admin"),
	}}
	for _, rule := range rules {
		copied := make(map[string]any, len(rule))
		for key, value := range rule {
			copied[key] = value
		}
		copied["FORMULA"] = map[string]any{"$and": []any{role("editor"), rule["FORMULA"]}}
		all = append(all, copied)
	}
	payload, err := json.Marshal(map[string]any{"source_ref": "integration-test:environment-objects", "policy": map[string]any{"AllAccessPermissionRules": map[string]any{"rules": all}}})
	require.NoError(t, err)
	status, response := doAuthorizedRequest(t, "POST", testBaseURL+"/security/abac/policy-versions", string(payload), admin)
	require.Equal(t, http.StatusCreated, status, response)
	var version struct {
		ID int64 `json:"version_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(response), &version))
	validatePolicyVersion(t, version.ID, admin)
	activatePolicyVersion(t, version.ID, admin)
}

func environmentFixture(id, idShort string) map[string]any {
	return map[string]any{
		"assetAdministrationShells": []any{map[string]any{"modelType": "AssetAdministrationShell", "id": id, "idShort": idShort, "assetInformation": map[string]any{"assetKind": "Instance"}, "submodels": []any{map[string]any{"type": "ModelReference", "keys": []any{map[string]any{"type": "Submodel", "value": id}}}}}},
		"submodels": []any{map[string]any{"modelType": "Submodel", "id": id, "idShort": idShort, "submodelElements": []any{
			map[string]any{"modelType": "Property", "idShort": "Public", "valueType": "xs:string", "value": "public-value"},
			map[string]any{"modelType": "Property", "idShort": "Secret", "valueType": "xs:string", "value": "secret-value"},
		}}},
		"conceptDescriptions": []any{map[string]any{"modelType": "ConceptDescription", "id": id, "idShort": idShort}},
	}
}

func requireEnvironmentUpload(t *testing.T, token string, environment map[string]any, expected int) {
	t.Helper()
	payload, err := json.Marshal(environment)
	require.NoError(t, err)
	status, body := postEnvironmentFile(t, token, "environment.json", payload)
	require.Equal(t, expected, status, string(body))
}

func postEnvironmentFile(t *testing.T, token, name string, content []byte) (int, []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/upload", &body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return environmentHTTP(t, req)
}

func environmentRequest(t *testing.T, token, path, accept string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, testBaseURL+path, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", accept)
	return environmentHTTP(t, req)
}

func environmentHTTP(t *testing.T, req *http.Request) (int, []byte) {
	t.Helper()
	response, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, body
}

func buildEnvironmentPackage(t *testing.T, environment map[string]any) []byte {
	t.Helper()
	var output bytes.Buffer
	pkg, err := aasx.NewPackaging().CreateWriter(&output)
	require.NoError(t, err)
	payload, err := json.Marshal(environment)
	require.NoError(t, err)
	specURI, err := url.Parse("/aasx/environment.json")
	require.NoError(t, err)
	spec, err := pkg.PutPartFromStream(specURI, "application/json", bytes.NewReader(payload))
	require.NoError(t, err)
	require.NoError(t, pkg.MakeSpec(spec))
	for name, content := range map[string]string{"/public.txt": "PUBLIC-BINARY", "/secret.txt": "SECRET-BINARY", "/thumbnail.png": "THUMBNAIL-BINARY"} {
		uri, parseErr := url.Parse(name)
		require.NoError(t, parseErr)
		part, partErr := pkg.PutPartFromStream(uri, "application/octet-stream", strings.NewReader(content))
		require.NoError(t, partErr)
		require.NoError(t, pkg.RelateSupplementaryToSpec(part, spec))
		if name == "/thumbnail.png" {
			require.NoError(t, pkg.SetThumbnail(part))
		}
	}
	require.NoError(t, pkg.Close())
	return output.Bytes()
}

func environmentPackageContents(t *testing.T, payload []byte) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	require.NoError(t, err)
	var result strings.Builder
	for _, part := range reader.File {
		stream, openErr := part.Open()
		require.NoError(t, openErr)
		content, readErr := io.ReadAll(stream)
		require.NoError(t, readErr)
		require.NoError(t, stream.Close())
		_, writeErr := result.Write(content)
		require.NoError(t, writeErr)
	}
	return result.String()
}
