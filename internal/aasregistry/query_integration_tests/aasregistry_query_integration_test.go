//nolint:all
package main

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		InitialDelay:          15 * time.Second,
		DefaultExpectedStatus: http.StatusOK,
		TokenProvider: testenv.NewPasswordGrantTokenProvider(
			"http://localhost:8081/realms/basyx/protocol/openid-connect/token",
			"basyx-ui",
			10*time.Second,
		),
	})
}

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile: "docker_compose/docker_compose.yml",
		HealthURL:   "http://localhost:6005/health",
	}))
}
