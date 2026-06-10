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

//nolint:all
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRegistrySyncConcurrentSubmodelReferenceCreationDoesNotRaceAASDescriptorUpsert(t *testing.T) {
	resetDatabase(t)

	const iterations = 20
	const parallelReferences = 4

	for iteration := 0; iteration < iterations; iteration++ {
		suffix := fmt.Sprintf("race-%d-%d", time.Now().UnixNano(), iteration)
		aasID := fmt.Sprintf("https://example.org/aas/%s", suffix)
		encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))

		createRegistrySyncRaceSubmodels(t, suffix, parallelReferences)
		createRegistrySyncRaceShell(t, aasID, suffix)

		results := postRegistrySyncRaceSubmodelReferences(t, encodedAASID, suffix, parallelReferences)
		for _, result := range results {
			require.NoError(t, result.err)
			require.Equal(t, http.StatusCreated, result.status, "iteration=%d submodel=%d response=%s", iteration, result.index, string(result.body))
		}

		status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, aasEnvBaseURL+"/shells/"+encodedAASID+"/submodel-refs", nil)
		require.Equal(t, http.StatusOK, status, "response=%s", string(body))
		requireRegistrySyncRaceReferenceCount(t, body, parallelReferences)
	}
}

type registrySyncRacePostResult struct {
	index  int
	status int
	body   []byte
	err    error
}

func createRegistrySyncRaceSubmodels(t *testing.T, suffix string, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		submodelID := registrySyncRaceSubmodelID(suffix, index)
		payload := map[string]any{
			"id":        submodelID,
			"idShort":   fmt.Sprintf("smRace%d", index),
			"modelType": "Submodel",
		}
		status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPost, aasEnvBaseURL+"/submodels", payload)
		require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	}
}

func createRegistrySyncRaceShell(t *testing.T, aasID string, suffix string) {
	t.Helper()
	payload := map[string]any{
		"id":        aasID,
		"idShort":   suffix,
		"modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{
			"assetKind":     "Instance",
			"globalAssetId": fmt.Sprintf("https://example.org/asset/%s", suffix),
		},
	}
	status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPost, aasEnvBaseURL+"/shells", payload)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
}

func postRegistrySyncRaceSubmodelReferences(t *testing.T, encodedAASID string, suffix string, count int) []registrySyncRacePostResult {
	t.Helper()
	results := make([]registrySyncRacePostResult, count)
	var wg sync.WaitGroup
	start := make(chan struct{})

	for index := 0; index < count; index++ {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			status, body, err := doAASEnvRawJSONRequest(
				http.MethodPost,
				aasEnvBaseURL+"/shells/"+encodedAASID+"/submodel-refs",
				map[string]any{
					"type": "ModelReference",
					"keys": []map[string]any{
						{
							"type":  "Submodel",
							"value": registrySyncRaceSubmodelID(suffix, index),
						},
					},
				},
			)
			results[index] = registrySyncRacePostResult{
				index:  index,
				status: status,
				body:   body,
				err:    err,
			}
		}()
	}

	close(start)
	wg.Wait()
	return results
}

func registrySyncRaceSubmodelID(suffix string, index int) string {
	return fmt.Sprintf("https://example.org/sm/%s-%d", suffix, index)
}

func doAASEnvRawJSONRequest(method string, endpoint string, payload any) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		marshaled, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(marshaled)
	}

	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return 0, nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := aasEnvNoRedirectClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, respBody, nil
}

func requireRegistrySyncRaceReferenceCount(t *testing.T, body []byte, expectedCount int) {
	t.Helper()
	var payload struct {
		Result []any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Len(t, payload.Result, expectedCount)
}
