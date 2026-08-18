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
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestOpenAPIContainsAASXFileServerSSP002Contract(t *testing.T) {
	specificationBytes, err := os.ReadFile("openapi.yaml")
	require.NoError(t, err)

	var specification map[string]any
	require.NoError(t, yaml.Unmarshal(specificationBytes, &specification))
	info := mapValue(t, specification, "info")
	require.NotContains(t, info, "x-profile-identifier")
	profileIdentifiers := stringSlice(t, info["x-profile-identifiers"])
	require.Contains(t, profileIdentifiers, "https://admin-shell.io/aas/API/3/2/AasxFileServerServiceSpecification/SSP-001")
	require.Contains(t, profileIdentifiers, "https://admin-shell.io/aas/API/3/2/AasxFileServerServiceSpecification/SSP-002")

	paths := mapValue(t, specification, "paths")

	post := mapValue(t, mapValue(t, paths, "/packages-async"), "post")
	require.Equal(t, "PostAsyncAASXPackage", post["operationId"])
	multipartContent := mapValue(t, mapValue(t, mapValue(t, post, "requestBody"), "content"), "multipart/form-data")
	multipartSchema := mapValue(t, multipartContent, "schema")
	require.Contains(t, stringSlice(t, multipartSchema["required"]), "file")
	fileEncoding := mapValue(t, mapValue(t, multipartContent, "encoding"), "file")
	require.Equal(t, "application/asset-administration-shell-package", fileEncoding["contentType"])
	accepted := mapValue(t, mapValue(t, post, "responses"), "202")
	require.Equal(t, "../Part2-API-Schemas/openapi.yaml#/components/schemas/OperationHandle", responseSchemaRef(t, accepted))
	require.Contains(t, mapValue(t, accepted, "headers"), "Location")

	status := mapValue(t, mapValue(t, paths, "/packages-async/status/{handleId}"), "get")
	require.Equal(t, "GetAasxAsyncStatus", status["operationId"])
	statusResponses := mapValue(t, status, "responses")
	require.Equal(t, "../Part2-API-Schemas/openapi.yaml#/components/schemas/BaseOperationResult", responseSchemaRef(t, mapValue(t, statusResponses, "200")))
	require.Equal(t, "../Part2-API-Schemas/openapi.yaml#/components/responses/found", mapValue(t, statusResponses, "302")["$ref"])

	result := mapValue(t, mapValue(t, paths, "/packages-async/result/{handleId}"), "get")
	require.Equal(t, "GetAasxAsyncResult", result["operationId"])
	require.Equal(t, "../Part2-API-Schemas/openapi.yaml#/components/schemas/BaseOperationResult", responseSchemaRef(t, mapValue(t, mapValue(t, result, "responses"), "200")))

	description := mapValue(t, mapValue(t, paths, "/description"), "get")
	descriptionResponse := mapValue(t, mapValue(t, description, "responses"), "200")
	descriptionExample := mapValue(t, mapValue(t, mapValue(t, descriptionResponse, "content"), "application/json"), "example")
	require.ElementsMatch(t, profileIdentifiers, stringSlice(t, descriptionExample["profiles"]))
}

func responseSchemaRef(t *testing.T, response map[string]any) string {
	t.Helper()
	content := mapValue(t, response, "content")
	applicationJSON := mapValue(t, content, "application/json")
	reference, valid := mapValue(t, applicationJSON, "schema")["$ref"].(string)
	require.True(t, valid, "OpenAPI response schema must contain a string $ref")
	return reference
}

func mapValue(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, found := parent[key]
	require.Truef(t, found, "OpenAPI field %q is missing", key)
	result, valid := value.(map[string]any)
	require.Truef(t, valid, "OpenAPI field %q has type %T, expected object", key, value)
	return result
}

func stringSlice(t *testing.T, value any) []string {
	t.Helper()
	raw, valid := value.([]any)
	require.Truef(t, valid, "OpenAPI field has type %T, expected array", value)
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		text, valid := item.(string)
		require.Truef(t, valid, "OpenAPI array item has type %T, expected string", item)
		result = append(result, text)
	}
	return result
}
