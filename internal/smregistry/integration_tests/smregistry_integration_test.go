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
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
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
	administrationTimestamp := time.Now().UTC().Format(time.RFC3339Nano)
	return map[string]any{
		"id": submodelID,
		"administration": map[string]any{
			"createdAt": administrationTimestamp,
			"updatedAt": administrationTimestamp,
		},
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

func assertLocationHeaderMatches(t *testing.T, expectedLocation string, actualLocation string) {
	t.Helper()

	require.NotEmpty(t, actualLocation)

	expectedURL, err := url.Parse(expectedLocation)
	require.NoError(t, err)

	actualURL, err := url.Parse(actualLocation)
	require.NoError(t, err)

	require.Equal(t, expectedURL.Scheme, actualURL.Scheme)
	require.Equal(t, expectedURL.Path, actualURL.Path)
	require.Equal(t, expectedURL.RawQuery, actualURL.RawQuery)
	require.Equal(t, expectedURL.Port(), actualURL.Port())

	expectedHost := strings.ToLower(expectedURL.Hostname())
	actualHost := strings.ToLower(actualURL.Hostname())
	if expectedHost == actualHost {
		return
	}

	allowedLoopbackHosts := []string{"localhost", "127.0.0.1"}
	require.Contains(t, allowedLoopbackHosts, expectedHost)
	require.Contains(t, allowedLoopbackHosts, actualHost)
}

func TestSubmodelRegistryRecentChanges(t *testing.T) {
	prefix := fmt.Sprintf("urn:example:sm:recent-%d", time.Now().UnixNano())
	currentID := prefix + "-current"
	secondID := prefix + "-second"
	deletedID := prefix + "-deleted"
	t.Cleanup(func() {
		cleanupSubmodelDescriptorHTTP(t, currentID)
		cleanupSubmodelDescriptorHTTP(t, secondID)
		cleanupSubmodelDescriptorHTTP(t, deletedID)
	})

	changedAfter := time.Now().UTC()
	time.Sleep(50 * time.Millisecond)

	endpoint := smRegistryBaseURL + "/submodel-descriptors"
	statusCode, body, _ := doRequest(t, smNoRedirectClient, http.MethodPost, endpoint, buildSubmodelDescriptorPayload(currentID, "v1"))
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))

	currentIdentifier := base64.RawURLEncoding.EncodeToString([]byte(currentID))
	statusCode, body, _ = doRequest(t, smNoRedirectClient, http.MethodPut, endpoint+"/"+currentIdentifier, buildSubmodelDescriptorPayload(currentID, "v2"))
	require.Equal(t, http.StatusNoContent, statusCode, "response=%s", string(body))

	statusCode, body, _ = doRequest(t, smNoRedirectClient, http.MethodPost, endpoint, buildSubmodelDescriptorPayload(secondID, "second"))
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))

	statusCode, body, _ = doRequest(t, smNoRedirectClient, http.MethodPost, endpoint, buildSubmodelDescriptorPayload(deletedID, "deleted"))
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))
	deletedIdentifier := base64.RawURLEncoding.EncodeToString([]byte(deletedID))
	statusCode, body, _ = doRequest(t, smNoRedirectClient, http.MethodDelete, endpoint+"/"+deletedIdentifier, nil)
	require.Equal(t, http.StatusNoContent, statusCode, "response=%s", string(body))

	updatedPage := getSubmodelDescriptorPage(t, url.Values{
		"updatedFrom": []string{changedAfter.Format(time.RFC3339Nano)},
		"limit":       []string{"10"},
	})
	require.ElementsMatch(t, []string{"v2"}, descriptorTagValues(updatedPage, currentID))
	require.ElementsMatch(t, []string{"second"}, descriptorTagValues(updatedPage, secondID))
	require.Empty(t, descriptorTagValues(updatedPage, deletedID))

	createdPage := getSubmodelDescriptorPage(t, url.Values{
		"createdFrom": []string{changedAfter.Format(time.RFC3339Nano)},
		"limit":       []string{"10"},
	})
	require.ElementsMatch(t, []string{"v2"}, descriptorTagValues(createdPage, currentID))
	require.ElementsMatch(t, []string{"second"}, descriptorTagValues(createdPage, secondID))
	require.Empty(t, descriptorTagValues(createdPage, deletedID))

	firstPage := getSubmodelDescriptorPage(t, url.Values{
		"updatedFrom": []string{changedAfter.Format(time.RFC3339Nano)},
		"limit":       []string{"1"},
	})
	require.Len(t, firstPage.Result, 1)
	require.NotEmpty(t, firstPage.PagingMetadata.Cursor)
	firstPageID, _ := firstPage.Result[0]["id"].(string)
	require.Contains(t, []string{currentID, secondID}, firstPageID)

	secondPage := getSubmodelDescriptorPage(t, url.Values{
		"updatedFrom": []string{changedAfter.Format(time.RFC3339Nano)},
		"limit":       []string{"1"},
		"cursor":      []string{firstPage.PagingMetadata.Cursor},
	})
	require.Len(t, secondPage.Result, 1)
	secondPageID, _ := secondPage.Result[0]["id"].(string)
	require.Contains(t, []string{currentID, secondID}, secondPageID)
	require.NotEqual(t, firstPageID, secondPageID)
}

type submodelDescriptorPage struct {
	PagingMetadata struct {
		Cursor string `json:"cursor"`
	} `json:"paging_metadata"`
	Result []map[string]any `json:"result"`
}

func getSubmodelDescriptorPage(t *testing.T, query url.Values) submodelDescriptorPage {
	t.Helper()

	endpoint := smRegistryBaseURL + "/submodel-descriptors"
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	statusCode, body, _ := doRequest(t, smNoRedirectClient, http.MethodGet, endpoint, nil)
	require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))

	var page submodelDescriptorPage
	require.NoError(t, json.Unmarshal(body, &page))
	return page
}

func descriptorTagValues(page submodelDescriptorPage, descriptorID string) []string {
	values := []string{}
	for _, descriptor := range page.Result {
		if descriptor["id"] != descriptorID {
			continue
		}
		extensions, _ := descriptor["extensions"].([]any)
		for _, extensionValue := range extensions {
			extension, _ := extensionValue.(map[string]any)
			if extension["name"] == "tag" {
				if tag, ok := extension["value"].(string); ok {
					values = append(values, tag)
				}
			}
		}
	}
	return values
}

func TestLocationHeadersForCreateEndpointsSubmodelRegistry(t *testing.T) {
	t.Run("PostSubmodelDescriptorSetsLocation", func(t *testing.T) {
		submodelID := fmt.Sprintf("urn:example:sm:location-post-%d", time.Now().UnixNano())
		t.Cleanup(func() { cleanupSubmodelDescriptorHTTP(t, submodelID) })

		endpoint := smRegistryBaseURL + "/submodel-descriptors"
		expectedIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))

		statusCode, _, headers := doRequest(t, smNoRedirectClient, http.MethodPost, endpoint, buildSubmodelDescriptorPayload(submodelID, "v1"))
		require.Equal(t, http.StatusCreated, statusCode)
		assertLocationHeaderMatches(t, endpoint+"/"+expectedIdentifier, headers.Get("Location"))
	})

	t.Run("PutSubmodelDescriptorByIdSetsLocationOnlyOnCreate", func(t *testing.T) {
		submodelID := fmt.Sprintf("urn:example:sm:location-put-%d", time.Now().UnixNano())
		t.Cleanup(func() { cleanupSubmodelDescriptorHTTP(t, submodelID) })

		submodelIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
		endpoint := smRegistryBaseURL + "/submodel-descriptors/" + submodelIdentifier

		createStatusCode, _, createHeaders := doRequest(t, smNoRedirectClient, http.MethodPut, endpoint, buildSubmodelDescriptorPayload(submodelID, "before"))
		require.Equal(t, http.StatusCreated, createStatusCode)
		assertLocationHeaderMatches(t, endpoint, createHeaders.Get("Location"))

		updateStatusCode, _, updateHeaders := doRequest(t, smNoRedirectClient, http.MethodPut, endpoint, buildSubmodelDescriptorPayload(submodelID, "after"))
		require.Equal(t, http.StatusNoContent, updateStatusCode)
		require.Empty(t, updateHeaders.Get("Location"))
	})
}

func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
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
