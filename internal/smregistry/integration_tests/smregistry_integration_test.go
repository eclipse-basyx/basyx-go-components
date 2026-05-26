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
// Author: Martin Stemmer ( Fraunhofer IESE )

//nolint:all
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq" // PostgreSQL Treiber
	"github.com/stretchr/testify/require"
)

func deleteAllSubmodelDescriptors(t *testing.T, runner *testenv.JSONSuiteRunner, stepNumber int) {
	response, err := runner.RunStep(testenv.JSONSuiteStep{
		Method:         http.MethodGet,
		Endpoint:       "http://127.0.0.1:6004/submodel-descriptors",
		ExpectedStatus: http.StatusOK,
	}, stepNumber)
	require.NoError(t, err)

	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.Body), &list))

	for _, item := range list.Result {
		enc := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		_, err := runner.RunStep(testenv.JSONSuiteStep{
			Method:         http.MethodDelete,
			Endpoint:       fmt.Sprintf("http://127.0.0.1:6004/submodel-descriptors/%s", enc),
			ExpectedStatus: http.StatusNoContent,
		}, stepNumber)
		require.NoError(t, err)
	}
}

func cleanupSubmodelDescriptorHTTP(t *testing.T, submodelID string) {
	t.Helper()

	encodedSubmodelIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	endpoint := smRegistryBaseURL + "/submodel-descriptors/" + encodedSubmodelIdentifier

	statusCode, _, _ := doRequest(t, smNoRedirectClient, http.MethodDelete, endpoint, nil)
	require.Contains(t, []int{http.StatusNoContent, http.StatusNotFound}, statusCode)
}

func buildSubmodelDescriptorPayload(submodelID string, tag string) map[string]any {
	return map[string]any{
		"id": submodelID,
		"endpoints": []any{
			map[string]any{
				"interface": "AAS-3.0",
				"protocolInformation": map[string]any{
					"href":             "https://example.com/submodels/" + base64.RawURLEncoding.EncodeToString([]byte(submodelID)),
					"endpointProtocol": "https",
				},
			},
		},
		"extensions": []any{
			map[string]any{
				"name":      "tag",
				"valueType": "xs:string",
				"value":     tag,
			},
		},
	}
}

func TestLocationHeadersForCreateEndpointsSubmodelRegistry(t *testing.T) {
	t.Run("PostSubmodelDescriptorSetsLocation", func(t *testing.T) {
		submodelID := fmt.Sprintf("urn:example:sm:location-post-%d", time.Now().UnixNano())
		t.Cleanup(func() { cleanupSubmodelDescriptorHTTP(t, submodelID) })

		endpoint := smRegistryBaseURL + "/submodel-descriptors"
		expectedIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))

		statusCode, _, headers := doRequest(t, smNoRedirectClient, http.MethodPost, endpoint, buildSubmodelDescriptorPayload(submodelID, "v1"))
		require.Equal(t, http.StatusCreated, statusCode)
		require.Equal(t, endpoint+"/"+expectedIdentifier, headers.Get("Location"))
	})

	t.Run("PutSubmodelDescriptorByIdSetsLocationOnlyOnCreate", func(t *testing.T) {
		submodelID := fmt.Sprintf("urn:example:sm:location-put-%d", time.Now().UnixNano())
		t.Cleanup(func() { cleanupSubmodelDescriptorHTTP(t, submodelID) })

		submodelIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
		endpoint := smRegistryBaseURL + "/submodel-descriptors/" + submodelIdentifier

		createStatusCode, _, createHeaders := doRequest(t, smNoRedirectClient, http.MethodPut, endpoint, buildSubmodelDescriptorPayload(submodelID, "before"))
		require.Equal(t, http.StatusCreated, createStatusCode)
		require.Equal(t, endpoint, createHeaders.Get("Location"))

		updateStatusCode, _, updateHeaders := doRequest(t, smNoRedirectClient, http.MethodPut, endpoint, buildSubmodelDescriptorPayload(submodelID, "after"))
		require.Equal(t, http.StatusNoContent, updateStatusCode)
		require.Empty(t, updateHeaders.Get("Location"))
	})
}

func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ShouldCompareResponse: testenv.CompareMethods(http.MethodGet),
		ActionHandlers: map[string]testenv.JSONStepAction{
			"DELETE_ALL_SUBMODEL_DESCRIPTORS": func(t *testing.T, runner *testenv.JSONSuiteRunner, _ testenv.JSONSuiteStep, stepNumber int) {
				deleteAllSubmodelDescriptors(t, runner, stepNumber)
			},
		},
	})
}

func TestMain(m *testing.M) {
	if os.Getenv("BASYX_EXTERNAL_COMPOSE") == "1" {
		os.Exit(m.Run())
	}

	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:   "docker_compose/docker_compose.yml",
		HealthURL:     "http://127.0.0.1:6004/health",
		HealthTimeout: 2 * time.Minute,
	}))
}
