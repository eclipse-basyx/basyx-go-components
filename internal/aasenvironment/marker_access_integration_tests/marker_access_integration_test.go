/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package markeraccessintegrationtests

import (
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

func TestMarkerAccessIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ConfigPath: "it_config.json",
		TokenProvider: testenv.NewPasswordGrantTokenProvider(
			"http://localhost:8080/realms/basyx/protocol/openid-connect/token",
			"basyx-ui",
			10*time.Second,
		),
	})
}

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     "../../../examples/CatenaXample/docker-compose.yml",
		PreDownBeforeUp: true,
		HealthURL:       "http://127.0.0.1:5004/api/v3/health",
		WaitForReady: func() error {
			return testenv.WaitHealthyURL("http://127.0.0.1:5005/health", 2*time.Minute)
		},
	}))
}
