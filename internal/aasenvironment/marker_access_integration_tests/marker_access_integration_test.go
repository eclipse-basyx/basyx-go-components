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

const markerExamplePath = "../../../examples/CatenaXample"

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
	infraDir, infraFile, err := prepareMarkerFile(filepath.Join(markerExamplePath, "basyx-infra.yml"), "basyx-infra.yml", markerFixtureRewrites, nil)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		_ = os.RemoveAll(dtrSecurityEnv)
		_ = os.RemoveAll(smSecurityEnv)
		_ = os.RemoveAll(dataDir)
		_ = os.RemoveAll(expectedDir)
		os.Exit(1)
	}
	composeReplacements, requiredComposeSnippets := markerComposeReplacements(runtime, dtrSecurityEnv, smSecurityEnv, infraFile)
	composeDir, composeFile, err := prepareMarkerFile(
		filepath.Join(markerExamplePath, "docker-compose.yml"),
		"docker-compose.yml",
		composeReplacements,
		requiredComposeSnippets,
	)
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
		"BASYX_MARKER_DATA_DIR":     filepath.ToSlash(markerDataDir),
		"BASYX_MARKER_EXPECTED_DIR": filepath.ToSlash(markerExpectedDir),
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

func markerComposeReplacements(
	runtime *testenv.ComposeRuntime,
	dtrSecurityEnv string,
	smSecurityEnv string,
	infraFile string,
) (map[string]string, []string) {
	absoluteExamplePath, err := filepath.Abs(markerExamplePath)
	if err != nil {
		absoluteExamplePath = markerExamplePath
	}
	repositoryRoot := filepath.Clean(filepath.Join(absoluteExamplePath, "..", ".."))
	replacements := markerReplacements(runtime)
	configurationImage := "    image: eclipsebasyx/basyxconfigurationservice-go:SNAPSHOT\n    pull_policy: always"
	digitalTwinRegistryImage := "    image: eclipsebasyx/digitaltwinregistry-go:SNAPSHOT\n    pull_policy: always"
	submodelRepositoryImage := "    image: eclipsebasyx/submodelrepository-go:SNAPSHOT\n    pull_policy: always"
	required := []string{
		"    container_name: marker-access-aas-web-ui\n",
		configurationImage,
		digitalTwinRegistryImage,
		submodelRepositoryImage,
		"- ./security/dtr:/security:ro",
		"- ./security/smrepo:/security:ro",
		"- ./basyx-infra.yml:/basyx-infra.yml:ro",
		"- ./keycloak/realm:/opt/keycloak/data/import:ro",
		`- "5004:5004"`,
		`- "5005:5004"`,
		`- "3000:3000"`,
		`- "8080:8080"`,
	}
	replacements[required[0]] = ""
	replacements[required[1]] = markerLocalBuild(repositoryRoot, "./cmd/basyxconfigurationservice/Dockerfile")
	replacements[required[2]] = markerLocalBuild(repositoryRoot, "./cmd/digitaltwinregistryservice/Dockerfile")
	replacements[required[3]] = markerLocalBuild(repositoryRoot, "./cmd/submodelrepositoryservice/Dockerfile")
	replacements[required[4]] = "- " + dtrSecurityEnv + ":/security:ro"
	replacements[required[5]] = "- " + smSecurityEnv + ":/security:ro"
	replacements[required[6]] = "- " + infraFile + ":/basyx-infra.yml:ro"
	replacements[required[7]] = "- " + filepath.Join(absoluteExamplePath, "keycloak", "realm") + ":/opt/keycloak/data/import:ro"
	replacements[required[8]] = fmt.Sprintf(`- "127.0.0.1:%d:5004"`, runtime.Port("dtr"))
	replacements[required[9]] = fmt.Sprintf(`- "127.0.0.1:%d:5004"`, runtime.Port("sm"))
	replacements[required[10]] = fmt.Sprintf(`- "127.0.0.1:%d:3000"`, runtime.Port("ui"))
	replacements[required[11]] = fmt.Sprintf(`- "0.0.0.0:%d:8080"`, runtime.Port("keycloak"))
	return replacements, required
}

func markerLocalBuild(repositoryRoot string, dockerfile string) string {
	return fmt.Sprintf("    build:\n      context: %q\n      dockerfile: %s", repositoryRoot, dockerfile)
}

func prepareMarkerFile(
	sourcePath string,
	targetName string,
	replacements map[string]string,
	requiredSnippets []string,
) (string, string, error) {
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
	if missing := missingMarkerSnippets(content, requiredSnippets); len(missing) > 0 {
		return "", "", fmt.Errorf("AASEMV-MARKER-FILEREPLACE missing required snippets in %s: %s", sourcePath, strings.Join(missing, ", "))
	}
	for oldValue, newValue := range replacements {
		content = strings.ReplaceAll(content, oldValue, newValue)
	}

	targetDir, err := os.MkdirTemp("", "basyx-marker-file-*")
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

func missingMarkerSnippets(content string, requiredSnippets []string) []string {
	missing := make([]string, 0)
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			missing = append(missing, snippet)
		}
	}
	return missing
}
