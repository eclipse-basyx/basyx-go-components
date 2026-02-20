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

	"github.com/stretchr/testify/require"
)

const ActionCheckDBIsEmpty = "CHECK_DB_IS_EMPTY"

type TokenCredentials struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

type JSONSuiteStep struct {
	Context        string            `json:"context,omitempty"`
	Method         string            `json:"method"`
	Endpoint       string            `json:"endpoint"`
	Data           string            `json:"data,omitempty"`
	ShouldMatch    string            `json:"shouldMatch,omitempty"`
	ExpectedStatus int               `json:"expectedStatus,omitempty"`
	Action         string            `json:"action,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Token          *TokenCredentials `json:"token,omitempty"`
}

type JSONStepAction func(t *testing.T, runner *JSONSuiteRunner, step JSONSuiteStep, stepNumber int)

type JSONTokenProvider interface {
	GetAccessToken(creds *TokenCredentials) (string, error)
}

type JSONSuiteOptions struct {
	ConfigPath string
	LogsDir    string

	InitialDelay          time.Duration
	RequestTimeout        time.Duration
	DefaultExpectedStatus int

	StepName              func(step JSONSuiteStep, stepNumber int) string
	ShouldCompareResponse func(step JSONSuiteStep) bool
	ShouldSkipStep        func(step JSONSuiteStep) bool

	ActionHandlers map[string]JSONStepAction
	TokenProvider  JSONTokenProvider

	EnableRequestLog bool
	EnableRawDump    bool
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

func CompareMethods(methods ...string) func(step JSONSuiteStep) bool {
	allowed := map[string]struct{}{}
	for _, method := range methods {
		allowed[strings.ToUpper(method)] = struct{}{}
	}
	return func(step JSONSuiteStep) bool {
		if step.ShouldMatch == "" {
			return false
		}
		_, ok := allowed[strings.ToUpper(step.Method)]
		return ok
	}
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
		driver = "postgres"
	}

	schema := strings.TrimSpace(options.Schema)
	if schema == "" {
		schema = "public"
	}

	excluded := make(map[string]struct{}, len(options.ExcludedTables))
	for _, table := range options.ExcludedTables {
		trimmed := strings.TrimSpace(table)
		if trimmed == "" {
			continue
		}
		excluded[trimmed] = struct{}{}
	}

	return func(t *testing.T, _ *JSONSuiteRunner, _ JSONSuiteStep, _ int) {
		require.NotEmpty(t, strings.TrimSpace(options.DSN), "TESTENV-CHECKDB-MISSING-DSN")

		nonEmpty, err := listNonEmptyTables(driver, options.DSN, schema, excluded)
		require.NoError(t, err)
		require.Emptyf(t, nonEmpty, "Expected all tables empty, but found rows in: %v", nonEmpty)
	}
}

func RunJSONSuite(t *testing.T, options JSONSuiteOptions) {
	t.Helper()

	normalized := normalizeJSONSuiteOptions(options)
	steps, err := loadJSONSuiteConfig(normalized.ConfigPath)
	require.NoError(t, err, "Failed to load test config")

	err = os.Mkdir(normalized.LogsDir, 0o755)
	if err != nil && !os.IsExist(err) {
		t.Fatalf("Failed to create logs directory: %v", err)
	}

	if normalized.InitialDelay > 0 {
		time.Sleep(normalized.InitialDelay)
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

			if normalized.ShouldCompareResponse(step) {
				runner.compareJSONResponse(t, step, stepNumber, response)
			}
		})
	}
}

func (r *JSONSuiteRunner) RunStep(step JSONSuiteStep, stepNumber int) (string, error) {
	bodyBytes, err := loadStepBody(step)
	if err != nil {
		return "", err
	}

	req, expectedStatus, err := r.buildRequest(step, bodyBytes)
	if err != nil {
		return "", err
	}

	if r.options.EnableRequestLog {
		r.writeRequestLog(stepNumber, req, bodyBytes)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		r.writeRequestError(stepNumber, err)
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != expectedStatus {
		r.writeStatusMismatchLog(stepNumber, req, expectedStatus, resp.StatusCode, respBody)
		return "", fmt.Errorf("expected status %d but got %d", expectedStatus, resp.StatusCode)
	}

	return string(respBody), nil
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
	if options.ShouldCompareResponse == nil {
		options.ShouldCompareResponse = func(step JSONSuiteStep) bool {
			return step.ShouldMatch != ""
		}
	}
	if options.ActionHandlers == nil {
		options.ActionHandlers = map[string]JSONStepAction{}
	}
	return options
}

func loadJSONSuiteConfig(path string) ([]JSONSuiteStep, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var steps []JSONSuiteStep
	if err := json.NewDecoder(file).Decode(&steps); err != nil {
		return nil, err
	}
	return steps, nil
}

func loadStepBody(step JSONSuiteStep) ([]byte, error) {
	if step.Data == "" {
		return nil, nil
	}
	data, err := os.ReadFile(step.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to read data file: %w", err)
	}
	return data, nil
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

		if _, skip := excluded[table]; skip {
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
