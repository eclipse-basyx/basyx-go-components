/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package markeraccessintegrationtests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

const markerExamplePath = "../../../examples/BaSyxMarkerAccessExample"

var (
	markerDTRBaseURL      = testenv.LocalURLFromEnv("BASYX_MARKER_DTR_PORT", 5004) + "/api/v3"
	markerSMBaseURL       = testenv.LocalURLFromEnv("BASYX_MARKER_SM_PORT", 5005)
	markerKeycloakToken   = testenv.LocalhostURLFromEnv("BASYX_MARKER_KEYCLOAK_PORT", 8080) + "/realms/basyx/protocol/openid-connect/token"
	markerDataDir         = filepath.Join(markerExamplePath, "data")
	markerExpectedDir     = "expected"
	markerTemplateValues  = map[string]string{}
	markerFixtureRewrites = map[string]string{}
)

func TestMarkerAccessIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ConfigPath: "it_config.json",
		TokenProvider: testenv.NewPasswordGrantTokenProvider(
			markerKeycloakToken,
			"basyx-ui",
			10*time.Second,
		),
		TemplateValues: markerTemplateValues,
	})
}

func TestMain(m *testing.M) {
	runtime := testenv.NewComposeRuntimeOrExit("aasenvironment-marker-access", []testenv.PortBinding{
		{Name: "dtr", EnvVar: "BASYX_MARKER_DTR_PORT"},
		{Name: "sm", EnvVar: "BASYX_MARKER_SM_PORT"},
		{Name: "ui", EnvVar: "BASYX_MARKER_UI_PORT"},
		{Name: "keycloak", EnvVar: "BASYX_MARKER_KEYCLOAK_PORT"},
	})
	markerDTRBaseURL = runtime.LocalURL("dtr") + "/api/v3"
	markerSMBaseURL = runtime.LocalURL("sm")
	markerKeycloakToken = runtime.LocalhostURL("keycloak") + "/realms/basyx/protocol/openid-connect/token"
	markerFixtureRewrites = markerReplacements(runtime)

	dtrSecurityEnv := testenv.PrepareSecurityEnvOrExit(filepath.Join(markerExamplePath, "security", "dtr"), markerFixtureRewrites)
	smSecurityEnv := testenv.PrepareSecurityEnvOrExit(filepath.Join(markerExamplePath, "security", "smrepo"), markerFixtureRewrites)
	dataDir := testenv.PrepareSecurityEnvOrExit(filepath.Join(markerExamplePath, "data"), markerFixtureRewrites)
	expectedDir := testenv.PrepareSecurityEnvOrExit("expected", markerFixtureRewrites)
	infraDir, infraFile, err := prepareMarkerFile(filepath.Join(markerExamplePath, "basyx-infra.yml"), "basyx-infra.yml", markerFixtureRewrites)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		_ = os.RemoveAll(dtrSecurityEnv)
		_ = os.RemoveAll(smSecurityEnv)
		_ = os.RemoveAll(dataDir)
		_ = os.RemoveAll(expectedDir)
		os.Exit(1)
	}
	composeDir, composeFile, err := prepareMarkerFile(filepath.Join(markerExamplePath, "docker-compose.yml"), "docker-compose.yml", markerComposeReplacements(runtime, dtrSecurityEnv, smSecurityEnv, infraFile))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		_ = os.RemoveAll(dtrSecurityEnv)
		_ = os.RemoveAll(smSecurityEnv)
		_ = os.RemoveAll(dataDir)
		_ = os.RemoveAll(expectedDir)
		_ = os.RemoveAll(infraDir)
		os.Exit(1)
	}

	markerDataDir = dataDir
	markerExpectedDir = expectedDir
	markerTemplateValues = map[string]string{
		"BASYX_MARKER_DATA_DIR":     markerDataDir,
		"BASYX_MARKER_EXPECTED_DIR": markerExpectedDir,
	}

	code := testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     composeFile,
		ProjectName:     runtime.ProjectName,
		Env:             runtime.Env(),
		PreDownBeforeUp: true,
		HealthURL:       markerDTRBaseURL + "/health",
		WaitForReady: func() error {
			return testenv.WaitHealthyURL(markerSMBaseURL+"/health", 2*time.Minute)
		},
	})

	_ = os.RemoveAll(dtrSecurityEnv)
	_ = os.RemoveAll(smSecurityEnv)
	_ = os.RemoveAll(dataDir)
	_ = os.RemoveAll(expectedDir)
	_ = os.RemoveAll(infraDir)
	_ = os.RemoveAll(composeDir)
	os.Exit(code)
}

func markerReplacements(runtime *testenv.ComposeRuntime) map[string]string {
	keycloakURL := fmt.Sprintf("http://keycloak.localhost:%d", runtime.Port("keycloak"))
	return map[string]string{
		"http://localhost:5004":             runtime.LocalhostURL("dtr"),
		"http://127.0.0.1:5004":             runtime.LocalURL("dtr"),
		"http://localhost:5005":             runtime.LocalhostURL("sm"),
		"http://127.0.0.1:5005":             runtime.LocalURL("sm"),
		"http://keycloak.localhost:8080":    keycloakURL,
		"{{BASYX_MARKER_SM_LOCALHOST_URL}}": runtime.LocalhostURL("sm"),
	}
}

func markerComposeReplacements(runtime *testenv.ComposeRuntime, dtrSecurityEnv string, smSecurityEnv string, infraFile string) map[string]string {
	absoluteExamplePath, err := filepath.Abs(markerExamplePath)
	if err != nil {
		absoluteExamplePath = markerExamplePath
	}
	repositoryRoot := filepath.Clean(filepath.Join(absoluteExamplePath, "..", ".."))
	replacements := markerReplacements(runtime)
	replacements["    container_name: marker-access-aas-web-ui\n"] = ""
	replacements["context: ../.."] = fmt.Sprintf("context: %q", repositoryRoot)
	replacements["- ./security/dtr:/security:ro"] = "- " + dtrSecurityEnv + ":/security:ro"
	replacements["- ./security/smrepo:/security:ro"] = "- " + smSecurityEnv + ":/security:ro"
	replacements["- ./basyx-infra.yml:/basyx-infra.yml:ro"] = "- " + infraFile + ":/basyx-infra.yml:ro"
	replacements["- ./keycloak/realm:/opt/keycloak/data/import:ro"] = "- " + filepath.Join(absoluteExamplePath, "keycloak", "realm") + ":/opt/keycloak/data/import:ro"
	replacements[`- "5004:5004"`] = fmt.Sprintf(`- "127.0.0.1:%d:5004"`, runtime.Port("dtr"))
	replacements[`- "5005:5004"`] = fmt.Sprintf(`- "127.0.0.1:%d:5004"`, runtime.Port("sm"))
	replacements[`- "3000:3000"`] = fmt.Sprintf(`- "127.0.0.1:%d:3000"`, runtime.Port("ui"))
	replacements[`- "8080:8080"`] = fmt.Sprintf(`- "0.0.0.0:%d:8080"`, runtime.Port("keycloak"))
	return replacements
}

func prepareMarkerFile(sourcePath string, targetName string, replacements map[string]string) (string, string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", "", fmt.Errorf("AASEMV-MARKER-FILESTAT: %w", err)
	}
	//nolint:gosec // The marker test reads a fixed fixture path assembled in TestMain.
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", "", fmt.Errorf("AASEMV-MARKER-FILEREAD: %w", err)
	}
	content := string(data)
	for oldValue, newValue := range replacements {
		content = strings.ReplaceAll(content, oldValue, newValue)
	}

	targetDir, err := os.MkdirTemp(".", ".basyx-marker-file-*")
	if err != nil {
		return "", "", fmt.Errorf("AASEMV-MARKER-FILEDIR: %w", err)
	}
	targetDir, err = filepath.Abs(targetDir)
	if err != nil {
		_ = os.RemoveAll(targetDir)
		return "", "", fmt.Errorf("AASEMV-MARKER-FILEABS: %w", err)
	}
	//nolint:gosec // Docker containers run as non-root users and need to read mounted fixture directories.
	if err = os.Chmod(targetDir, 0o755); err != nil {
		_ = os.RemoveAll(targetDir)
		return "", "", fmt.Errorf("AASEMV-MARKER-FILECHMOD: %w", err)
	}
	targetPath := filepath.Join(targetDir, targetName)
	if err = os.WriteFile(targetPath, []byte(content), info.Mode().Perm()); err != nil {
		_ = os.RemoveAll(targetDir)
		return "", "", fmt.Errorf("AASEMV-MARKER-FILEWRITE: %w", err)
	}
	return targetDir, targetPath, nil
}
