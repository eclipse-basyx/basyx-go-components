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
package testenv

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

const ActionCheckDBIsEmpty = "CHECK_DB_IS_EMPTY"
const ActionAssertSubmodelAbsent = "ASSERT_SUBMODEL_ABSENT"
const ActionAssertBulkFailure = "ASSERT_BULK_FAILURE"

type TokenCredentials struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

type JSONSuiteStep struct {
	Context                   string            `json:"context,omitempty"`
	Method                    string            `json:"method"`
	Endpoint                  string            `json:"endpoint"`
	Data                      string            `json:"data,omitempty"`
	ShouldMatch               string            `json:"shouldMatch,omitempty"`
	ExpectedResponseHeaders   map[string]string `json:"expectedResponseHeaders,omitempty"`
	ExpectedStatus            int               `json:"expectedStatus,omitempty"`
	ExpectedBulkFailureStatus int               `json:"expectedBulkFailureStatus,omitempty"`
	ExpectedBulkFailureIndex  int               `json:"expectedBulkFailureIndex,omitempty"`
	ExpectedBulkFailureID     string            `json:"expectedBulkFailureIdentifier,omitempty"`
	Action                    string            `json:"action,omitempty"`
	Headers                   map[string]string `json:"headers,omitempty"`
	Token                     *TokenCredentials `json:"token,omitempty"`
}

type JSONStepResult struct {
	Body    string
	Headers http.Header
}

type JSONStepAction func(t *testing.T, runner *JSONSuiteRunner, step JSONSuiteStep, stepNumber int)

type JSONTokenProvider interface {
	GetAccessToken(creds *TokenCredentials) (string, error)
}

type JSONSuiteOptions struct {
	ConfigPath string
	LogsDir    string

	RequestTimeout        time.Duration
	DefaultExpectedStatus int

	StepName        func(step JSONSuiteStep, stepNumber int) string
	ShouldSkipStep  func(step JSONSuiteStep) bool
	ShouldMatchJSON func(step JSONSuiteStep) bool

	ActionHandlers map[string]JSONStepAction
	TokenProvider  JSONTokenProvider

	EnableRequestLog bool
	EnableRawDump    bool

	TemplateValues map[string]string
}

type JSONSuiteRunner struct {
	options JSONSuiteOptions
	client  *http.Client
}

type CheckDBIsEmptyOptions struct {
	Driver         string
	DSN            string
	Schema         string
	ExcludedTables []string
}

type CheckSubmodelAbsentOptions struct {
	Driver string
	DSN    string
}

type PasswordGrantTokenProvider struct {
	tokenURL string
	clientID string
	client   *http.Client

	mu    sync.Mutex
	cache map[string]string
}

func NewPasswordGrantTokenProvider(tokenURL string, clientID string, timeout time.Duration) *PasswordGrantTokenProvider {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &PasswordGrantTokenProvider{
		tokenURL: tokenURL,
		clientID: clientID,
		client:   &http.Client{Timeout: timeout},
		cache:    map[string]string{},
	}
}

func (p *PasswordGrantTokenProvider) GetAccessToken(creds *TokenCredentials) (string, error) {
	if creds == nil {
		return "", nil
	}

	cacheKey := creds.User + "|" + creds.Password
	p.mu.Lock()
	if token, ok := p.cache[cacheKey]; ok && token != "" {
		p.mu.Unlock()
		return token, nil
	}
	p.mu.Unlock()

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", p.clientID)
	form.Set("username", creds.User)
	form.Set("password", creds.Password)

	req, err := http.NewRequest(http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get token, status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("no access_token in response")
	}

	p.mu.Lock()
	p.cache[cacheKey] = tokenResp.AccessToken
	p.mu.Unlock()
	return tokenResp.AccessToken, nil
}

func DefaultJSONStepName(step JSONSuiteStep, stepNumber int) string {
	if step.Action != "" {
		return fmt.Sprintf("Step_%d_ACTION_%s", stepNumber, step.Action)
	}
	if step.Context != "" {
		return fmt.Sprintf("Step_%d_%s", stepNumber, step.Context)
	}
	return fmt.Sprintf("Step_%d_%s_%s", stepNumber, strings.ToUpper(step.Method), step.Endpoint)
}

func NewCheckDBIsEmptyAction(options CheckDBIsEmptyOptions) JSONStepAction {
	driver := strings.TrimSpace(options.Driver)
	if driver == "" {
		driver = "pgx"
	}

	schema := strings.TrimSpace(options.Schema)
	if schema == "" {
		schema = "public"
	}

	excluded := defaultCheckDBIsEmptyExcludedTables(options.ExcludedTables)

	return func(t *testing.T, _ *JSONSuiteRunner, _ JSONSuiteStep, _ int) {
		require.NotEmpty(t, strings.TrimSpace(options.DSN), "TESTENV-CHECKDB-MISSING-DSN")

		nonEmpty, err := listNonEmptyTables(driver, options.DSN, schema, excluded)
		require.NoError(t, err)
		require.Emptyf(t, nonEmpty, "Expected all domain tables empty, but found rows in: %v", nonEmpty)
	}
}

func defaultCheckDBIsEmptyExcludedTables(extraTables []string) map[string]struct{} {
	excluded := make(map[string]struct{}, len(extraTables)+10)
	for _, table := range []string{
		"basyxsystem",
		"history_guard_config",
		"aas_history",
		"aas_history_payload",
		"submodel_history",
		"submodel_history_payload",
		"concept_description_history",
		"concept_description_history_payload",
		"descriptor_history",
		"descriptor_history_payload",
		"submodel_descriptor_history",
		"submodel_descriptor_history_payload",
	} {
		excluded[table] = struct{}{}
	}
	for _, table := range extraTables {
		trimmed := strings.TrimSpace(table)
		if trimmed == "" {
			continue
		}
		excluded[strings.ToLower(trimmed)] = struct{}{}
	}
	return excluded
}

func NewCheckSubmodelAbsentAction(options CheckSubmodelAbsentOptions) JSONStepAction {
	driver := strings.TrimSpace(options.Driver)
	if driver == "" {
		driver = "pgx"
	}

	return func(t *testing.T, _ *JSONSuiteRunner, step JSONSuiteStep, _ int) {
		require.NotEmpty(t, strings.TrimSpace(options.DSN), "TESTENV-CHECKSMABSENT-MISSING-DSN")

		identifier := strings.TrimSpace(step.Endpoint)
		require.NotEmpty(t, identifier, "TESTENV-CHECKSMABSENT-MISSING-ID")

		db, err := sql.Open(driver, options.DSN)
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		var count int
		err = db.QueryRow("SELECT COUNT(1) FROM submodel WHERE submodel_identifier = $1", identifier).Scan(&count)
		require.NoError(t, err)
		require.Equalf(t, 0, count, "Expected no submodel row for identifier '%s'", identifier)
	}
}

// AssertBulkFailureAction asserts an asynchronous bulk operation failure.
//
// The helper starts the bulk step, follows the returned operation handle, and
// compares the completed failure status and message with the expected values
// from the JSON suite step.
//
// Parameters:
//   - t: Test handle used for assertions.
//   - runner: JSON suite runner that executes the step and reads the result.
//   - step: JSON suite step containing the bulk request and expected failure.
//   - stepNumber: Step number used in assertion messages.
func AssertBulkFailureAction(t *testing.T, runner *JSONSuiteRunner, step JSONSuiteStep, stepNumber int) {
	t.Helper()

	response, err := runner.RunStep(step, stepNumber)
	require.NoError(t, err)

	location := response.Headers.Get("Location")
	require.NotEmpty(t, location, "expected bulk start response to include Location header")

	resultEndpoint, err := bulkResultEndpoint(step.Endpoint, location)
	require.NoError(t, err)

	resultBody := waitForBulkFailureResult(t, runner, step, stepNumber, resultEndpoint)
	assertBulkFailureResult(t, step, resultBody)
}

func bulkResultEndpoint(startEndpoint string, location string) (string, error) {
	baseURL, err := url.Parse(startEndpoint)
	if err != nil {
		return "", err
	}
	locationURL, err := url.Parse(location)
	if err != nil {
		return "", err
	}

	resultURL := baseURL.ResolveReference(locationURL)
	statusPath := "/bulk/status/"
	if !strings.Contains(resultURL.Path, statusPath) {
		return "", fmt.Errorf("TESTENV-BULK-LOCATION expected status location, got %s", location)
	}

	resultURL.Path = strings.Replace(resultURL.Path, statusPath, "/bulk/result/", 1)
	resultURL.RawQuery = ""
	return resultURL.String(), nil
}

func waitForBulkFailureResult(
	t *testing.T,
	runner *JSONSuiteRunner,
	step JSONSuiteStep,
	stepNumber int,
	resultEndpoint string,
) string {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	resultStep := step
	resultStep.Method = http.MethodGet
	resultStep.Endpoint = resultEndpoint
	resultStep.Data = ""
	resultStep.ShouldMatch = ""
	resultStep.ExpectedResponseHeaders = nil

	for {
		body, statusCode, err := runner.runRawStep(resultStep, stepNumber)
		require.NoError(t, err)

		if statusCode == http.StatusNoContent {
			t.Fatalf("expected failed bulk result, got successful 204 response")
		}
		if statusCode == http.StatusBadRequest && bulkResultHasDetails(body) {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for failed bulk result from %s; last status %d body %s", resultEndpoint, statusCode, body)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func bulkResultHasDetails(body string) bool {
	var result struct {
		Details []bulkFailureDetail `json:"details"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return false
	}
	return len(result.Details) > 0
}

type bulkFailureDetail struct {
	Index      int    `json:"index"`
	Identifier string `json:"identifier"`
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

func assertBulkFailureResult(t *testing.T, step JSONSuiteStep, body string) {
	t.Helper()

	var result struct {
		ExecutionState string              `json:"executionState"`
		Success        bool                `json:"success"`
		FailedCount    int                 `json:"failedCount"`
		Details        []bulkFailureDetail `json:"details"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &result))
	require.Equal(t, "Completed", result.ExecutionState)
	require.False(t, result.Success)
	require.NotZero(t, result.FailedCount)

	expectedStatus := step.ExpectedBulkFailureStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusForbidden
	}

	for _, detail := range result.Details {
		if detail.Index != step.ExpectedBulkFailureIndex {
			continue
		}
		if step.ExpectedBulkFailureID != "" && detail.Identifier != step.ExpectedBulkFailureID {
			continue
		}
		require.Equal(t, expectedStatus, detail.StatusCode)
		return
	}

	t.Fatalf(
		"expected bulk failure detail index=%d identifier=%q status=%d, got %+v",
		step.ExpectedBulkFailureIndex,
		step.ExpectedBulkFailureID,
		expectedStatus,
		result.Details,
	)
}

func RunJSONSuite(t *testing.T, options JSONSuiteOptions) {
	t.Helper()

	normalized := normalizeJSONSuiteOptions(options)
	steps, err := loadJSONSuiteConfig(normalized.ConfigPath, normalized.TemplateValues)
	require.NoError(t, err, "Failed to load test config")

	err = os.Mkdir(normalized.LogsDir, 0o755)
	if err != nil && !os.IsExist(err) {
		t.Fatalf("Failed to create logs directory: %v", err)
	}

	runner := &JSONSuiteRunner{
		options: normalized,
		client:  &http.Client{Timeout: normalized.RequestTimeout},
	}

	for idx, rawStep := range steps {
		stepNumber := idx + 1
		step := rawStep
		name := normalized.StepName(step, stepNumber)

		t.Run(name, func(t *testing.T) {
			if normalized.ShouldSkipStep != nil && normalized.ShouldSkipStep(step) {
				return
			}

			if step.Action != "" {
				handler, ok := normalized.ActionHandlers[step.Action]
				if !ok {
					t.Fatalf("unknown action: %s", step.Action)
				}
				handler(t, runner, step, stepNumber)
				return
			}

			response, runErr := runner.RunStep(step, stepNumber)
			require.NoError(t, runErr, "Request failed")

			if len(step.ExpectedResponseHeaders) > 0 {
				runner.compareResponseHeaders(t, step, stepNumber, response.Headers)
			}

			if step.ShouldMatch != "" && normalized.ShouldMatchJSON(step) {
				runner.compareJSONResponse(t, step, stepNumber, response.Body)
			}
		})
	}
}

func (r *JSONSuiteRunner) RunStep(step JSONSuiteStep, stepNumber int) (JSONStepResult, error) {
	bodyBytes, err := r.loadStepBody(step)
	if err != nil {
		return JSONStepResult{}, err
	}

	req, expectedStatus, err := r.buildRequest(step, bodyBytes)
	if err != nil {
		return JSONStepResult{}, err
	}

	if r.options.EnableRequestLog {
		r.writeRequestLog(stepNumber, req, bodyBytes)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		r.writeRequestError(stepNumber, err)
		return JSONStepResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return JSONStepResult{}, err
	}

	if resp.StatusCode != expectedStatus {
		r.writeStatusMismatchLog(stepNumber, req, expectedStatus, resp.StatusCode, respBody)
		return JSONStepResult{}, fmt.Errorf("expected status %d but got %d", expectedStatus, resp.StatusCode)
	}

	return JSONStepResult{
		Body:    string(respBody),
		Headers: resp.Header,
	}, nil
}

func (r *JSONSuiteRunner) runRawStep(step JSONSuiteStep, stepNumber int) (string, int, error) {
	bodyBytes, err := r.loadStepBody(step)
	if err != nil {
		return "", 0, err
	}

	req, _, err := r.buildRequest(step, bodyBytes)
	if err != nil {
		return "", 0, err
	}

	if r.options.EnableRequestLog {
		r.writeRequestLog(stepNumber, req, bodyBytes)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		r.writeRequestError(stepNumber, err)
		return "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	return string(respBody), resp.StatusCode, nil
}

func (r *JSONSuiteRunner) compareResponseHeaders(t *testing.T, step JSONSuiteStep, stepNumber int, headers http.Header) {
	t.Helper()

	for key, expectedValue := range step.ExpectedResponseHeaders {
		actualValue := headers.Get(key)
		if actualValue != expectedValue {
			r.writeHeaderMismatchLog(stepNumber, key, expectedValue, actualValue)
		}

		require.Equalf(t, expectedValue, actualValue, "Response header mismatch for %s", key)
	}
}

func (r *JSONSuiteRunner) buildRequest(step JSONSuiteStep, bodyBytes []byte) (*http.Request, int, error) {
	method := strings.ToUpper(step.Method)
	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, step.Endpoint, bodyReader)
	if err != nil {
		return nil, 0, err
	}

	if len(bodyBytes) > 0 {
		if method == http.MethodPatch {
			trimmed := strings.TrimSpace(string(bodyBytes))
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
				req.Header.Set("Content-Type", "application/merge-patch+json")
			} else {
				req.Header.Set("Content-Type", "application/json")
			}
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	for key, value := range step.Headers {
		req.Header.Set(key, value)
	}

	if step.Token != nil {
		if r.options.TokenProvider == nil {
			return nil, 0, fmt.Errorf("token credentials provided but no token provider configured")
		}
		token, tokenErr := r.options.TokenProvider.GetAccessToken(step.Token)
		if tokenErr != nil {
			return nil, 0, fmt.Errorf("failed to get access token: %w", tokenErr)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	expectedStatus := step.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = r.options.DefaultExpectedStatus
	}
	if expectedStatus == 0 {
		expectedStatus = http.StatusOK
	}

	return req, expectedStatus, nil
}

func (r *JSONSuiteRunner) compareJSONResponse(t *testing.T, step JSONSuiteStep, stepNumber int, actualBody string) {
	t.Helper()

	expectedRaw, err := os.ReadFile(step.ShouldMatch)
	require.NoError(t, err, "Failed to read expected response file")
	expectedRaw = applyJSONSuiteTemplateValues(expectedRaw, r.options.TemplateValues)

	expectedJSON, err := normalizeJSON(expectedRaw)
	require.NoError(t, err, "Failed to parse expected JSON")
	actualJSON, err := normalizeJSON([]byte(actualBody))
	require.NoError(t, err, "Failed to parse response JSON")

	if expectedJSON != actualJSON {
		r.writeJSONMismatchLog(stepNumber, expectedJSON, actualJSON)
	}

	require.Equal(t, expectedJSON, actualJSON, "Response does not match expected")
}

func normalizeJSONSuiteOptions(options JSONSuiteOptions) JSONSuiteOptions {
	if options.ConfigPath == "" {
		options.ConfigPath = "it_config.json"
	}
	if options.LogsDir == "" {
		options.LogsDir = "logs"
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 10 * time.Second
	}
	if options.StepName == nil {
		options.StepName = DefaultJSONStepName
	}
	if options.ShouldMatchJSON == nil {
		options.ShouldMatchJSON = func(JSONSuiteStep) bool { return true }
	}
	if options.ActionHandlers == nil {
		options.ActionHandlers = map[string]JSONStepAction{}
	}
	return options
}

func loadJSONSuiteConfig(path string, templateValues map[string]string) ([]JSONSuiteStep, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = applyJSONSuiteTemplateValues(data, templateValues)

	var steps []JSONSuiteStep
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, err
	}
	return steps, nil
}

func applyJSONSuiteTemplateValues(data []byte, templateValues map[string]string) []byte {
	values := jsonSuiteTemplateValuesFromEnv()
	for key, value := range templateValues {
		values[key] = value
	}
	if len(values) == 0 {
		return data
	}

	result := string(data)
	for key, value := range values {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	return []byte(result)
}

func jsonSuiteTemplateValuesFromEnv() map[string]string {
	values := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(key, "BASYX_") {
			continue
		}
		values[key] = value
	}
	return values
}

func (r *JSONSuiteRunner) loadStepBody(step JSONSuiteStep) ([]byte, error) {
	if step.Data == "" {
		return nil, nil
	}
	data, err := os.ReadFile(step.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to read data file: %w", err)
	}
	return applyJSONSuiteTemplateValues(data, r.options.TemplateValues), nil
}

func normalizeJSON(input []byte) (string, error) {
	var parsed any
	if err := json.Unmarshal(input, &parsed); err != nil {
		return "", err
	}
	stripDynamicCreatedAt(parsed)
	normalized, err := json.Marshal(parsed)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}

func stripDynamicCreatedAt(value any) {
	switch casted := value.(type) {
	case map[string]any:
		delete(casted, "createdAt")
		for _, nested := range casted {
			stripDynamicCreatedAt(nested)
		}
	case []any:
		for _, nested := range casted {
			stripDynamicCreatedAt(nested)
		}
	}
}

func (r *JSONSuiteRunner) writeRequestLog(stepNumber int, req *http.Request, bodyBytes []byte) {
	var b strings.Builder
	b.WriteString(req.Method + " " + req.URL.String() + "\n")
	for key, values := range req.Header {
		b.WriteString(fmt.Sprintf("%s: %s\n", key, strings.Join(values, ",")))
	}
	if len(bodyBytes) > 0 {
		b.WriteString("\n")
		b.WriteString(string(bodyBytes))
		b.WriteString("\n")
	}
	_ = os.WriteFile(filepath.Join(r.options.LogsDir, fmt.Sprintf("REQUEST_STEP_%d.log", stepNumber)), []byte(b.String()), 0o644)

	if r.options.EnableRawDump {
		if dump, err := httputil.DumpRequestOut(req, false); err == nil {
			_ = os.WriteFile(filepath.Join(r.options.LogsDir, fmt.Sprintf("RAW_REQUEST_STEP_%d.dump", stepNumber)), dump, 0o644)
		}
	}
}

func (r *JSONSuiteRunner) writeRequestError(stepNumber int, err error) {
	_ = os.WriteFile(filepath.Join(r.options.LogsDir, fmt.Sprintf("REQUEST_STEP_%d.error.log", stepNumber)), []byte(err.Error()), 0o644)
}

func (r *JSONSuiteRunner) writeStatusMismatchLog(stepNumber int, req *http.Request, expected int, actual int, body []byte) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s %s\n", req.Method, req.URL.String()))
	b.WriteString(fmt.Sprintf("Expected status %d but got %d\n", expected, actual))
	b.WriteString("Response body: ")
	b.WriteString(string(body))
	b.WriteString("\n")
	_ = os.WriteFile(filepath.Join(r.options.LogsDir, fmt.Sprintf("STEP_%d.log", stepNumber)), []byte(b.String()), 0o644)
}

func (r *JSONSuiteRunner) writeJSONMismatchLog(stepNumber int, expected string, actual string) {
	var b strings.Builder
	b.WriteString("JSON mismatch:\nExpected: ")
	b.WriteString(expected)
	b.WriteString("\nActual: ")
	b.WriteString(actual)
	b.WriteString("\n")
	_ = os.WriteFile(filepath.Join(r.options.LogsDir, fmt.Sprintf("STEP_%d.log", stepNumber)), []byte(b.String()), 0o644)
}

func (r *JSONSuiteRunner) writeHeaderMismatchLog(stepNumber int, header string, expected string, actual string) {
	var b strings.Builder
	b.WriteString("Header mismatch:\n")
	b.WriteString(fmt.Sprintf("Header: %s\n", header))
	b.WriteString(fmt.Sprintf("Expected: %s\n", expected))
	b.WriteString(fmt.Sprintf("Actual: %s\n", actual))
	_ = os.WriteFile(filepath.Join(r.options.LogsDir, fmt.Sprintf("STEP_%d.log", stepNumber)), []byte(b.String()), 0o644)
}

func listNonEmptyTables(driver string, dsn string, schema string, excluded map[string]struct{}) ([]string, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("TESTENV-CHECKDB-OPEN: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("TESTENV-CHECKDB-PING: %w", err)
	}

	rows, err := db.Query("SELECT tablename FROM pg_tables WHERE schemaname = $1", schema)
	if err != nil {
		return nil, fmt.Errorf("TESTENV-CHECKDB-LISTTABLES: %w", err)
	}
	defer func() { _ = rows.Close() }()

	nonEmpty := []string{}
	for rows.Next() {
		var table string
		if scanErr := rows.Scan(&table); scanErr != nil {
			return nil, fmt.Errorf("TESTENV-CHECKDB-SCANTABLE: %w", scanErr)
		}

		if _, skip := excluded[strings.ToLower(strings.TrimSpace(table))]; skip {
			continue
		}

		count, countErr := countRowsInTable(db, schema, table)
		if countErr != nil {
			return nil, countErr
		}
		if count != 0 {
			nonEmpty = append(nonEmpty, fmt.Sprintf("%s:%d", table, count))
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("TESTENV-CHECKDB-ITERATE: %w", err)
	}

	return nonEmpty, nil
}

func countRowsInTable(db *sql.DB, schema string, table string) (int, error) {
	query := fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.%s",
		quoteSQLIdentifier(schema),
		quoteSQLIdentifier(table),
	)

	var count int
	if err := db.QueryRow(query).Scan(&count); err != nil {
		return 0, fmt.Errorf("TESTENV-CHECKDB-COUNTROWS: %w", err)
	}

	return count, nil
}

func quoteSQLIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
