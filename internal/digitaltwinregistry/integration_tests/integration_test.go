//nolint:all
package bench

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const (
	BaseURL         = "http://127.0.0.1:5004"
	ComposeFilePath = "./docker_compose/docker_compose.yml"
)

var (
	composeAvailable bool
	composeEngine    string
	composeArgsBase  []string
)

type TestConfig struct {
	Method         string            `json:"method"`
	Endpoint       string            `json:"endpoint"`
	Data           string            `json:"data,omitempty"`
	ShouldMatch    string            `json:"shouldMatch,omitempty"`
	ExpectedStatus int               `json:"expectedStatus,omitempty"`
	Action         string            `json:"action,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
}

func loadTestConfig(filename string) ([]TestConfig, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var configs []TestConfig
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&configs); err != nil {
		return nil, err
	}
	return configs, nil
}

func TestMain(m *testing.M) {
	eng, baseArgs, err := testenv.FindCompose()
	if err != nil {
		_, _ = fmt.Println("compose engine not found:", err)
		composeAvailable = false
		os.Exit(m.Run())
	}
	composeAvailable = true
	composeEngine = eng
	composeArgsBase = baseArgs

	upArgs := append(composeArgsBase, "-f", ComposeFilePath, "up", "-d", "--build")
	if v := getenv("DTR_TEST_BUILD"); v == "1" {
		upArgs = append(upArgs, "--build")
	}

	ctxUp, cancelUp := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancelUp()
	if err := testenv.RunCompose(ctxUp, composeEngine, upArgs...); err != nil {
		_, _ = fmt.Println("failed to start compose:", err)
		os.Exit(1)
	}

	code := m.Run()

	ctxDown, cancelDown := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancelDown()
	_ = testenv.RunCompose(ctxDown, composeEngine, append(composeArgsBase, "-f", ComposeFilePath, "down")...)

	os.Exit(code)
}

func mustHaveCompose(tb testing.TB) {
	tb.Helper()
	if !composeAvailable {
		tb.Skip("compose not available in this environment")
	}
}

func waitUntilHealthy(tb testing.TB) {
	tb.Helper()
	testenv.WaitHealthy(tb, BaseURL+"/health", 2*time.Minute)
}

func getenv(k string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return ""
}

func makeRequest(config TestConfig, stepNumber int) (string, error) {
	var req *http.Request
	var err error

	switch config.Method {
	case http.MethodGet, http.MethodDelete:
		req, err = http.NewRequest(config.Method, config.Endpoint, nil)
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		if config.Data != "" {
			data, readErr := os.ReadFile(config.Data)
			if readErr != nil {
				return "", fmt.Errorf("failed to read data file: %v", readErr)
			}
			req, err = http.NewRequest(config.Method, config.Endpoint, bytes.NewBuffer(data))
			if err == nil {
				req.Header.Set("Content-Type", "application/json")
			}
		} else {
			req, err = http.NewRequest(config.Method, config.Endpoint, nil)
		}
	default:
		return "", fmt.Errorf("unsupported method: %s", config.Method)
	}
	if err != nil {
		return "", err
	}

	for k, v := range config.Headers {
		req.Header.Set(k, v)
	}

	client := testenv.HTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != config.ExpectedStatus {
		logFile := fmt.Sprintf("logs/STEP_%d.log", stepNumber)
		f, ferr := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if ferr == nil {
			fmt.Fprintf(f, "Expected status %d but got %d\n", config.ExpectedStatus, resp.StatusCode) //nolint:errcheck
			_, _ = f.Write(body)
			_ = f.Close()
		}
		return "", fmt.Errorf("expected status %d but got %d", config.ExpectedStatus, resp.StatusCode)
	}

	return string(body), nil
}

func deleteAllDescriptors(stepNumber int, headers map[string]string) error {
	listCfg := TestConfig{
		Method:         http.MethodGet,
		Endpoint:       BaseURL + "/shell-descriptors?limit=200",
		ExpectedStatus: http.StatusOK,
		Headers:        headers,
	}
	body, err := makeRequest(listCfg, stepNumber)
	if err != nil {
		return err
	}
	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		return err
	}
	for _, item := range list.Result {
		enc := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		delCfg := TestConfig{
			Method:         http.MethodDelete,
			Endpoint:       BaseURL + "/shell-descriptors/" + enc,
			ExpectedStatus: http.StatusNoContent,
			Headers:        headers,
		}
		_, _ = makeRequest(delCfg, stepNumber)
	}
	return nil
}

func TestIntegration(t *testing.T) {
	mustHaveCompose(t)
	waitUntilHealthy(t)

	configs, err := loadTestConfig("it_config.json")
	require.NoError(t, err, "Failed to load test config")

	if err := os.Mkdir("logs", 0755); err != nil && !os.IsExist(err) {
		t.Fatalf("Failed to create logs directory: %v", err)
	}

	time.Sleep(2 * time.Second)

	for i, config := range configs {
		stepNumber := i + 1
		name := fmt.Sprintf("Step_%d_%s_%s", stepNumber, config.Method, config.Endpoint)
		if config.Action != "" {
			name = fmt.Sprintf("Step_%d_ACTION_%s", stepNumber, config.Action)
		}
		t.Run(name, func(t *testing.T) {
			switch config.Action {
			case "DELETE_ALL_SHELL_DESCRIPTORS":
				require.NoError(t, deleteAllDescriptors(stepNumber, config.Headers))
				return
			}

			response, err := makeRequest(config, stepNumber)
			require.NoError(t, err, "Request failed")

			if config.ShouldMatch != "" {
				expected, err := os.ReadFile(config.ShouldMatch)
				require.NoError(t, err, "Failed to read expected response file")

				var expectedJSON, responseJSON interface{}
				err = json.Unmarshal(expected, &expectedJSON)
				require.NoError(t, err, "Failed to parse expected JSON")
				err = json.Unmarshal([]byte(response), &responseJSON)
				require.NoError(t, err, "Failed to parse response JSON")

				expectedBytes, _ := json.Marshal(expectedJSON)
				responseBytes, _ := json.Marshal(responseJSON)
				require.Equal(t, string(expectedBytes), string(responseBytes))
			}
		})
	}
}
