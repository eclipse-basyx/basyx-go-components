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
package bench

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const (
	tractusBaseURL         = "http://127.0.0.1:5004"
	tractusContextPath     = "/api/v3"
	tractusComposeFilePath = "./docker_compose/docker_compose.yml"
	tractusUseCasesRoot    = "./aas-registry-usecases"
	edcBpnHeaderName       = "Edc-Bpn"
	edcBpnTenantOne        = "TENANT_ONE"
)

type tractusUseCase struct {
	Name  string
	Path  string
	Steps []tractusUseCaseStep
}

type tractusUseCaseStep struct {
	Name         string
	RequestPath  string
	ResponsePath string
}

type tractusStepRequest struct {
	URL    string `json:"url"`
	Tenant string `json:"tenant"`
	Method string `json:"method"`
	Body   any    `json:"body"`
}

type tractusAssertion struct {
	JSONPath string `json:"jsonPath"`
	Equals   any    `json:"equals,omitempty"`
	Exists   *bool  `json:"exists,omitempty"`
	HasSize  *int   `json:"hasSize,omitempty"`
	HasItem  any    `json:"hasItem,omitempty"`
	IsEmpty  *bool  `json:"isEmpty,omitempty"`
}

type tractusStepExpectation struct {
	Status          int                `json:"status"`
	Content         bool               `json:"content"`
	Assertions      []tractusAssertion `json:"assertions"`
	ExpectedPayload any                `json:"expectedPayload"`
}

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile: tractusComposeFilePath,
		UpArgs:      []string{"up", "-d", "--build"},
		HealthURL:   tractusBaseURL + tractusContextPath + "/health",
	}))
}

func TestTractusXAASRegistryUseCases(t *testing.T) {
	useCases, err := discoverUseCases(tractusUseCasesRoot)
	require.NoError(t, err, "DTR-TRACTUS-DISCOVER-USECASES")
	require.NotEmpty(t, useCases, "DTR-TRACTUS-NO-USECASES")

	client := &http.Client{Timeout: 30 * time.Second}

	for _, useCase := range useCases {
		currentUseCase := useCase
		t.Run(currentUseCase.Name, func(t *testing.T) {
			cleanupAllShellDescriptors(t, client)
			runUseCase(t, client, currentUseCase)
		})
	}
}

func runUseCase(t *testing.T, client *http.Client, useCase tractusUseCase) {
	t.Helper()

	for _, step := range useCase.Steps {
		currentStep := step
		t.Run(currentStep.Name, func(t *testing.T) {
			requestCfg, err := loadStepRequest(currentStep.RequestPath)
			require.NoError(t, err, "DTR-TRACTUS-LOAD-REQUEST")

			expectedCfg, err := loadStepExpectation(currentStep.ResponsePath)
			require.NoError(t, err, "DTR-TRACTUS-LOAD-EXPECTED")

			actualStatus, actualBody := runRequest(t, client, requestCfg)
			require.Equal(t, expectedCfg.Status, actualStatus, "DTR-TRACTUS-STATUS-MISMATCH")

			if !expectedCfg.Content {
				require.Empty(t, strings.TrimSpace(string(actualBody)), "DTR-TRACTUS-EXPECTED-EMPTY-BODY")
			}

			compareExpectedPayload(t, expectedCfg.ExpectedPayload, actualBody)
			applyAssertions(t, expectedCfg.Assertions, actualBody)
		})
	}
}

func compareExpectedPayload(t *testing.T, expectedPayload any, actualBody []byte) {
	t.Helper()

	if expectedPayload == nil {
		return
	}

	expectedPayloadBytes, err := json.Marshal(expectedPayload)
	require.NoError(t, err, "DTR-TRACTUS-MARSHAL-EXPECTED")

	expectedNormalized, err := normalizeJSON(expectedPayloadBytes)
	require.NoError(t, err, "DTR-TRACTUS-NORMALIZE-EXPECTED")

	actualNormalized, err := normalizeJSON(actualBody)
	require.NoError(t, err, "DTR-TRACTUS-NORMALIZE-ACTUAL")

	require.Equal(t, expectedNormalized, actualNormalized, "DTR-TRACTUS-PAYLOAD-MISMATCH")
}

func applyAssertions(t *testing.T, assertions []tractusAssertion, actualBody []byte) {
	t.Helper()

	if len(assertions) == 0 {
		return
	}

	var actualJSON any
	err := json.Unmarshal(actualBody, &actualJSON)
	require.NoError(t, err, "DTR-TRACTUS-PARSE-ASSERTION-BODY")

	for idx, assertion := range assertions {
		applyAssertion(t, actualJSON, idx, assertion)
	}
}

func applyAssertion(t *testing.T, body any, idx int, assertion tractusAssertion) {
	t.Helper()

	values, err := resolveJSONPath(body, assertion.JSONPath)
	require.NoErrorf(t, err, "DTR-TRACTUS-ASSERT-PATH-%d", idx+1)

	if assertion.Exists != nil {
		require.Equalf(t, *assertion.Exists, len(values) > 0, "DTR-TRACTUS-ASSERT-EXISTS-%d", idx+1)
	}

	if assertion.Equals != nil {
		require.NotEmptyf(t, values, "DTR-TRACTUS-ASSERT-EQUALS-NOVALUE-%d", idx+1)
		require.Truef(
			t,
			jsonValueEqual(values[0], assertion.Equals),
			"DTR-TRACTUS-ASSERT-EQUALS-%d",
			idx+1,
		)
	}

	if assertion.HasSize != nil {
		actualSize, err := sizeOfResolvedValues(values)
		require.NoErrorf(t, err, "DTR-TRACTUS-ASSERT-HASSIZE-TYPE-%d", idx+1)
		require.Equalf(t, *assertion.HasSize, actualSize, "DTR-TRACTUS-ASSERT-HASSIZE-%d", idx+1)
	}

	if assertion.HasItem != nil {
		resolvedItems := valuesForContainment(values)
		require.Truef(
			t,
			containsJSONValue(resolvedItems, assertion.HasItem),
			"DTR-TRACTUS-ASSERT-HASITEM-%d",
			idx+1,
		)
	}

	if assertion.IsEmpty != nil {
		actualEmpty, err := isResolvedValueEmpty(values)
		require.NoErrorf(t, err, "DTR-TRACTUS-ASSERT-ISEMPTY-TYPE-%d", idx+1)
		require.Equalf(t, *assertion.IsEmpty, actualEmpty, "DTR-TRACTUS-ASSERT-ISEMPTY-%d", idx+1)
	}
}

func sizeOfResolvedValues(values []any) (int, error) {
	if len(values) == 0 {
		return 0, nil
	}

	if len(values) == 1 {
		return collectionSize(values[0])
	}

	return len(values), nil
}

func isResolvedValueEmpty(values []any) (bool, error) {
	if len(values) == 0 {
		return true, nil
	}

	if len(values) == 1 {
		return collectionEmpty(values[0])
	}

	return len(values) == 0, nil
}

func collectionSize(value any) (int, error) {
	switch casted := value.(type) {
	case []any:
		return len(casted), nil
	case map[string]any:
		return len(casted), nil
	case string:
		return len(casted), nil
	default:
		return 0, fmt.Errorf("DTR-TRACTUS-UNSUPPORTED-SIZE-TYPE")
	}
}

func collectionEmpty(value any) (bool, error) {
	size, err := collectionSize(value)
	if err != nil {
		return false, err
	}
	return size == 0, nil
}

func valuesForContainment(values []any) []any {
	if len(values) == 1 {
		if asSlice, ok := values[0].([]any); ok {
			return asSlice
		}
	}
	return values
}

func containsJSONValue(values []any, item any) bool {
	for _, value := range values {
		if jsonValueEqual(value, item) {
			return true
		}
	}
	return false
}

func jsonValueEqual(left any, right any) bool {
	leftBytes, leftErr := json.Marshal(left)
	rightBytes, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}

	leftNorm, leftNormErr := normalizeJSON(leftBytes)
	rightNorm, rightNormErr := normalizeJSON(rightBytes)
	if leftNormErr != nil || rightNormErr != nil {
		return false
	}

	return leftNorm == rightNorm
}

func runRequest(t *testing.T, client *http.Client, cfg tractusStepRequest) (int, []byte) {
	t.Helper()

	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	require.NotEmpty(t, method, "DTR-TRACTUS-EMPTY-METHOD")

	endpoint := strings.TrimSpace(cfg.URL)
	require.NotEmpty(t, endpoint, "DTR-TRACTUS-EMPTY-URL")

	fullURL := tractusBaseURL + endpoint
	requestBody := buildRequestBody(t, cfg.Body)
	tenant := strings.TrimSpace(cfg.Tenant)
	require.NotEmpty(t, tenant, "DTR-TRACTUS-EMPTY-TENANT")

	req, err := http.NewRequest(method, fullURL, requestBody)
	require.NoError(t, err, "DTR-TRACTUS-CREATE-REQUEST")
	req.Header.Set(edcBpnHeaderName, tenant)
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	require.NoError(t, err, "DTR-TRACTUS-EXEC-REQUEST")
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "DTR-TRACTUS-READ-BODY")

	return resp.StatusCode, respBody
}

func buildRequestBody(t *testing.T, body any) io.Reader {
	t.Helper()

	if body == nil {
		return nil
	}

	data, err := json.Marshal(body)
	require.NoError(t, err, "DTR-TRACTUS-MARSHAL-BODY")
	return bytes.NewReader(data)
}

func cleanupAllShellDescriptors(t *testing.T, client *http.Client) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, tractusBaseURL+tractusContextPath+"/shell-descriptors?limit=500", nil)
	require.NoError(t, err, "DTR-TRACTUS-CLEANUP-LIST-REQ")
	req.Header.Set(edcBpnHeaderName, edcBpnTenantOne)

	resp, err := client.Do(req)
	require.NoError(t, err, "DTR-TRACTUS-CLEANUP-LIST-EXEC")
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "DTR-TRACTUS-CLEANUP-LIST-STATUS")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "DTR-TRACTUS-CLEANUP-LIST-READ")

	var shellList struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &shellList), "DTR-TRACTUS-CLEANUP-LIST-PARSE")

	for _, item := range shellList.Result {
		encodedID := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		deleteReq, deleteErr := http.NewRequest(http.MethodDelete, tractusBaseURL+tractusContextPath+"/shell-descriptors/"+encodedID, nil)
		require.NoError(t, deleteErr, "DTR-TRACTUS-CLEANUP-DELETE-REQ")
		deleteReq.Header.Set(edcBpnHeaderName, edcBpnTenantOne)

		deleteResp, deleteExecErr := client.Do(deleteReq)
		require.NoError(t, deleteExecErr, "DTR-TRACTUS-CLEANUP-DELETE-EXEC")
		_ = deleteResp.Body.Close()
		require.Equal(t, http.StatusNoContent, deleteResp.StatusCode, "DTR-TRACTUS-CLEANUP-DELETE-STATUS")
	}
}

func discoverUseCases(root string) ([]tractusUseCase, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	sortedDirs := filterAndSortDirs(entries)
	useCases := make([]tractusUseCase, 0, len(sortedDirs))
	for _, dirEntry := range sortedDirs {
		useCasePath := filepath.Join(root, dirEntry.Name())
		steps, stepsErr := discoverUseCaseSteps(useCasePath)
		if stepsErr != nil {
			return nil, stepsErr
		}

		useCases = append(useCases, tractusUseCase{
			Name:  dirEntry.Name(),
			Path:  useCasePath,
			Steps: steps,
		})
	}

	return useCases, nil
}

func discoverUseCaseSteps(useCasePath string) ([]tractusUseCaseStep, error) {
	stepEntries, err := os.ReadDir(useCasePath)
	if err != nil {
		return nil, err
	}

	sortedStepDirs := filterAndSortDirs(stepEntries)
	steps := make([]tractusUseCaseStep, 0, len(sortedStepDirs))
	for _, stepEntry := range sortedStepDirs {
		stepPath := filepath.Join(useCasePath, stepEntry.Name())
		requestPath := filepath.Join(stepPath, "request.json")
		responsePath := filepath.Join(stepPath, "expected-response.json")

		_, requestErr := os.Stat(requestPath)
		_, responseErr := os.Stat(responsePath)
		if requestErr != nil && responseErr != nil {
			continue
		}
		if requestErr != nil {
			return nil, fmt.Errorf("DTR-TRACTUS-MISSING-REQUEST-FILE: %s", requestPath)
		}
		if responseErr != nil {
			return nil, fmt.Errorf("DTR-TRACTUS-MISSING-EXPECTED-FILE: %s", responsePath)
		}

		steps = append(steps, tractusUseCaseStep{
			Name:         stepEntry.Name(),
			RequestPath:  requestPath,
			ResponsePath: responsePath,
		})
	}

	return steps, nil
}

func filterAndSortDirs(entries []os.DirEntry) []os.DirEntry {
	result := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			result = append(result, entry)
		}
	}

	sort.Slice(result, func(i int, j int) bool {
		left := result[i].Name()
		right := result[j].Name()

		leftOrder, leftOk := numericPrefix(left)
		rightOrder, rightOk := numericPrefix(right)

		if leftOk && rightOk && leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if leftOk != rightOk {
			return leftOk
		}

		return left < right
	})

	return result
}

func numericPrefix(name string) (int, bool) {
	prefix := name
	if idx := strings.Index(name, "_"); idx > 0 {
		prefix = name[:idx]
	}

	value, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, false
	}
	return value, true
}

func loadStepRequest(path string) (tractusStepRequest, error) {
	file, err := os.Open(path)
	if err != nil {
		return tractusStepRequest{}, err
	}
	defer func() { _ = file.Close() }()

	var cfg tractusStepRequest
	if decodeErr := json.NewDecoder(file).Decode(&cfg); decodeErr != nil {
		return tractusStepRequest{}, decodeErr
	}

	return cfg, nil
}

func loadStepExpectation(path string) (tractusStepExpectation, error) {
	file, err := os.Open(path)
	if err != nil {
		return tractusStepExpectation{}, err
	}
	defer func() { _ = file.Close() }()

	var cfg tractusStepExpectation
	if decodeErr := json.NewDecoder(file).Decode(&cfg); decodeErr != nil {
		return tractusStepExpectation{}, decodeErr
	}

	return cfg, nil
}

func normalizeJSON(input []byte) (string, error) {
	var parsed any
	if err := json.Unmarshal(input, &parsed); err != nil {
		return "", err
	}

	normalized, err := json.Marshal(parsed)
	if err != nil {
		return "", err
	}

	return string(normalized), nil
}

func resolveJSONPath(root any, path string) ([]any, error) {
	if !strings.HasPrefix(path, "$.") {
		return nil, fmt.Errorf("DTR-TRACTUS-UNSUPPORTED-PATH: %s", path)
	}

	segments := strings.Split(strings.TrimPrefix(path, "$."), ".")
	current := []any{root}
	for _, segment := range segments {
		next, err := resolvePathSegment(current, segment)
		if err != nil {
			return nil, err
		}
		current = next
	}

	return current, nil
}

func resolvePathSegment(values []any, segment string) ([]any, error) {
	name, accessor, hasAccessor, parseErr := parseSegment(segment)
	if parseErr != nil {
		return nil, parseErr
	}

	next := make([]any, 0)
	for _, value := range values {
		resolved, ok := resolveSegmentName(value, name)
		if !ok {
			continue
		}

		if !hasAccessor {
			next = append(next, resolved)
			continue
		}

		appended, err := applyAccessor(next, resolved, accessor)
		if err != nil {
			return nil, err
		}
		next = appended
	}

	return next, nil
}

func parseSegment(segment string) (string, string, bool, error) {
	left := strings.Index(segment, "[")
	if left < 0 {
		return segment, "", false, nil
	}

	right := strings.Index(segment, "]")
	if right < left {
		return "", "", false, fmt.Errorf("DTR-TRACTUS-INVALID-PATH-SEGMENT: %s", segment)
	}

	return segment[:left], segment[left+1 : right], true, nil
}

func resolveSegmentName(value any, name string) (any, bool) {
	if name == "" {
		return value, true
	}

	asMap, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}

	mapped, found := asMap[name]
	return mapped, found
}

func applyAccessor(existing []any, value any, accessor string) ([]any, error) {
	asList, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("DTR-TRACTUS-ACCESSOR-NON-LIST")
	}

	if accessor == "*" {
		return append(existing, asList...), nil
	}

	idx, err := strconv.Atoi(accessor)
	if err != nil {
		return nil, fmt.Errorf("DTR-TRACTUS-ACCESSOR-NON-INDEX")
	}

	if idx < 0 || idx >= len(asList) {
		return existing, nil
	}

	return append(existing, asList[idx]), nil
}
