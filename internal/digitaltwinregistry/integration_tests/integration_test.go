//nolint:all
package bench

import (
	"encoding/base64"
	"encoding/json"
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

func TestMain(m *testing.M) {
	upArgs := []string{"up", "-d", "--build"}
	if v := getenv("DTR_TEST_BUILD"); v == "1" {
		upArgs = append(upArgs, "--build")
	}

	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile: ComposeFilePath,
		UpArgs:      upArgs,
		HealthURL:   BaseURL + "/health",
	}))
}

func getenv(k string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return ""
}

func deleteAllDescriptors(t *testing.T, runner *testenv.JSONSuiteRunner, stepNumber int, headers map[string]string) {
	body, err := runner.RunStep(testenv.JSONSuiteStep{
		Method:         http.MethodGet,
		Endpoint:       BaseURL + "/shell-descriptors?limit=200",
		ExpectedStatus: http.StatusOK,
		Headers:        headers,
	}, stepNumber)
	require.NoError(t, err)

	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &list))

	for _, item := range list.Result {
		enc := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		_, err := runner.RunStep(testenv.JSONSuiteStep{
			Method:         http.MethodDelete,
			Endpoint:       BaseURL + "/shell-descriptors/" + enc,
			ExpectedStatus: http.StatusNoContent,
			Headers:        headers,
		}, stepNumber)
		require.NoError(t, err)
	}
}

func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		InitialDelay: 2 * time.Second,
		ActionHandlers: map[string]testenv.JSONStepAction{
			"DELETE_ALL_SHELL_DESCRIPTORS": func(t *testing.T, runner *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, stepNumber int) {
				deleteAllDescriptors(t, runner, stepNumber, step.Headers)
			},
		},
	})
}
