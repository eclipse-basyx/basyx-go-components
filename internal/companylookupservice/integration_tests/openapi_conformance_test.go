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
// Author: Christian Koort ( Fraunhofer IESE )

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestCompanyLookupOpenAPIResponses(t *testing.T) {
	referenceSchema := companyLookupSchema(t, "Reference")
	errorSchema := companyLookupSchema(t, "Result")
	data, err := os.ReadFile("postBody/company_descriptor_1.json")
	require.NoError(t, err)
	var descriptor map[string]any
	require.NoError(t, json.Unmarshal(data, &descriptor))
	domain := fmt.Sprintf("conformance-%d.example.com", time.Now().UnixNano())
	descriptor["domain"] = domain
	endpoint := "/companies/" + common.EncodeString(domain)
	creator := descriptor["administration"].(map[string]any)["creator"]
	created := companyLookupRequest(t, http.MethodPost, "/companies", descriptor, http.StatusCreated)
	t.Cleanup(func() { companyLookupRequest(t, http.MethodDelete, endpoint, nil, http.StatusNoContent) })
	requireCompanyCreator(t, referenceSchema, creator, created)
	retrieved := companyLookupRequest(t, http.MethodGet, endpoint, nil, http.StatusOK)
	requireCompanyCreator(t, referenceSchema, creator, retrieved)

	t.Run("conflict", func(t *testing.T) {
		response := companyLookupRequest(t, http.MethodPost, "/companies", descriptor, http.StatusConflict)
		requireCompanyError(t, errorSchema, response, http.StatusConflict)
	})
	t.Run("invalid_limit", func(t *testing.T) {
		response := companyLookupRequest(t, http.MethodGet, "/companies?limit=0", nil, http.StatusBadRequest)
		requireCompanyError(t, errorSchema, response, http.StatusBadRequest)
	})
	t.Run("not_found", func(t *testing.T) {
		response := companyLookupRequest(t, http.MethodGet, "/companies/"+common.EncodeString("missing-"+domain), nil, http.StatusNotFound)
		requireCompanyError(t, errorSchema, response, http.StatusNotFound)
	})
	t.Run("reference_array", func(t *testing.T) {
		descriptor["administration"].(map[string]any)["creator"] = []any{creator}
		response := companyLookupRequest(t, http.MethodPost, "/companies", descriptor, http.StatusBadRequest)
		requireCompanyError(t, errorSchema, response, http.StatusBadRequest)
	})
}

func companyLookupSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile("../../../cmd/companylookupservice/openapi.yaml")
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, yaml.Unmarshal(data, &document))
	compiler := jsonschema.NewCompiler()
	const resource = "https://basyx.test/company-lookup-openapi"
	require.NoError(t, compiler.AddResource(resource, document))
	schema, err := compiler.Compile(resource + "#/components/schemas/" + name)
	require.NoError(t, err)
	return schema
}

func requireCompanyCreator(t *testing.T, schema *jsonschema.Schema, expected, response any) {
	t.Helper()
	creator := response.(map[string]any)["administration"].(map[string]any)["creator"]
	require.NoError(t, schema.Validate(creator))
	require.Equal(t, expected, creator)
}

func requireCompanyError(t *testing.T, schema *jsonschema.Schema, response any, status int) {
	t.Helper()
	require.NoError(t, schema.Validate(response))
	messages := response.([]any)
	require.NotEmpty(t, messages)
	for _, value := range messages {
		message := value.(map[string]any)
		require.Equal(t, "Error", message["messageType"])
		require.Equal(t, strconv.Itoa(status), message["code"])
		require.NotEmpty(t, message["text"])
	}
}

func companyLookupRequest(t *testing.T, method, path string, body any, expectedStatus int) any {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	request, err := http.NewRequest(method, companyLookupBaseURL+path, bytes.NewReader(data))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	data, err = io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, expectedStatus, response.StatusCode, "%s %s: %s", method, path, data)
	var result any
	if len(data) > 0 {
		require.NoError(t, json.Unmarshal(data, &result))
	}
	return result
}
