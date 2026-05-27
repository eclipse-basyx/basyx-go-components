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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func startAdderMicroservice(t *testing.T) (string, func()) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/delegate/add", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var requestPayload any
		if err := json.NewDecoder(r.Body).Decode(&requestPayload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": err.Error()})
			return
		}

		operationVariables := make([]map[string]any, 0)
		switch payload := requestPayload.(type) {
		case []any:
			for _, item := range payload {
				itemAsMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				operationVariables = append(operationVariables, itemAsMap)
			}
		case map[string]any:
			if inputArgumentsRaw, ok := payload["inputArguments"].([]any); ok {
				for _, item := range inputArgumentsRaw {
					itemAsMap, castOK := item.(map[string]any)
					if !castOK {
						continue
					}
					operationVariables = append(operationVariables, itemAsMap)
				}
			}
		}

		sum := 0
		for _, operationVariable := range operationVariables {
			valuePayload, ok := operationVariable["value"].(map[string]any)
			if !ok {
				continue
			}

			rawValue, ok := valuePayload["value"]
			if !ok {
				continue
			}

			valueAsInt, err := strconv.Atoi(fmt.Sprint(rawValue))
			if err != nil {
				continue
			}

			sum += valueAsInt
		}

		if len(operationVariables) == 0 {
			queryA := r.URL.Query().Get("a")
			queryB := r.URL.Query().Get("b")

			parsedA, errA := strconv.Atoi(queryA)
			parsedB, errB := strconv.Atoi(queryB)
			if errA == nil && errB == nil {
				sum = parsedA + parsedB
			}
		}

		responsePayload := []map[string]any{
			{
				"value": map[string]any{
					"modelType": "Property",
					"idShort":   "sum",
					"valueType": "xs:int",
					"value":     strconv.Itoa(sum),
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(responsePayload)
	})

	// #nosec G102 -- integration test listener must be reachable from repository container via host gateway mapping.
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err)

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		_ = server.Serve(listener)
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	delegationURL := fmt.Sprintf("http://localhost:%d/delegate/add?a=5&b=3", port)

	shutdown := func() {
		_ = server.Close()
		_ = listener.Close()
	}

	return delegationURL, shutdown
}

func TestDelegationOperation(t *testing.T) {
	if os.Getenv("BASYX_EXTERNAL_COMPOSE") == "1" {
		t.Skip("delegation callback from container to ephemeral host listener is not reliable in external compose mode")
	}

	baseURL := "http://localhost:6004"

	delegationURL, shutdown := startAdderMicroservice(t)
	defer shutdown()

	submodelID := "DelegationOperationSubmodelIntegrationTest"
	encodedSubmodelID := common.EncodeString(submodelID)

	submodelPayload := map[string]any{
		"modelType": "Submodel",
		"id":        submodelID,
		"idShort":   "DelegationOperationSubmodel",
		"kind":      "Instance",
		"submodelElements": []any{
			map[string]any{
				"modelType": "Operation",
				"idShort":   "AddNumbers",
				"qualifiers": []any{
					map[string]any{
						"type":      "invocationDelegation",
						"valueType": "xs:string",
						"value":     delegationURL,
					},
				},
				"inputVariables": []any{
					map[string]any{"value": map[string]any{"modelType": "Property", "idShort": "a", "valueType": "xs:int", "value": "0"}},
					map[string]any{"value": map[string]any{"modelType": "Property", "idShort": "b", "valueType": "xs:int", "value": "0"}},
				},
				"outputVariables": []any{
					map[string]any{"value": map[string]any{"modelType": "Property", "idShort": "sum", "valueType": "xs:int", "value": "0"}},
				},
			},
		},
	}

	submodelBody, err := json.Marshal(submodelPayload)
	require.NoError(t, err)

	createSubmodelResponse, err := http.Post(baseURL+"/submodels", "application/json", bytes.NewReader(submodelBody))
	require.NoError(t, err)
	defer func() { _ = createSubmodelResponse.Body.Close() }()
	require.Equal(t, http.StatusCreated, createSubmodelResponse.StatusCode)

	t.Cleanup(func() {
		request, requestErr := http.NewRequest(http.MethodDelete, baseURL+"/submodels/"+encodedSubmodelID, nil)
		if requestErr != nil {
			return
		}
		// #nosec G704 -- integration test calls fixed local repository endpoint.
		response, responseErr := (&http.Client{Timeout: 10 * time.Second}).Do(request)
		if responseErr == nil {
			_ = response.Body.Close()
		}
	})

	invokeRequestBody, err := json.Marshal(map[string]any{})
	require.NoError(t, err)

	invokeRequest, err := http.NewRequest(
		http.MethodPost,
		baseURL+"/submodels/"+encodedSubmodelID+"/submodel-elements/AddNumbers/invoke",
		bytes.NewReader(invokeRequestBody),
	)
	require.NoError(t, err)
	invokeRequest.Header.Set("Content-Type", "application/json")

	// #nosec G704 -- integration test calls fixed local repository endpoint.
	invokeResponse, err := (&http.Client{Timeout: 15 * time.Second}).Do(invokeRequest)
	require.NoError(t, err)
	defer func() { _ = invokeResponse.Body.Close() }()
	invokeResponseBody, err := io.ReadAll(invokeResponse.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, invokeResponse.StatusCode, "invoke response body: %s", string(invokeResponseBody))

	var invokeResultObject map[string]any
	if err := json.Unmarshal(invokeResponseBody, &invokeResultObject); err == nil {
		outputArguments, ok := invokeResultObject["outputArguments"].([]any)
		require.True(t, ok)
		require.Len(t, outputArguments, 1)

		outputOperationVariable, ok := outputArguments[0].(map[string]any)
		require.True(t, ok)
		outputValue, ok := outputOperationVariable["value"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "8", fmt.Sprint(outputValue["value"]))
		return
	}

	var invokeResultArray []any
	require.NoError(t, json.Unmarshal(invokeResponseBody, &invokeResultArray))
	require.Len(t, invokeResultArray, 1)

	outputOperationVariable, ok := invokeResultArray[0].(map[string]any)
	require.True(t, ok)
	outputValue, ok := outputOperationVariable["value"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "8", fmt.Sprint(outputValue["value"]))
}
