//go:build integration
// +build integration

package integration_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"testing"
	"time"
)

func startDockerCompose(t *testing.T) {
	t.Helper()

	cmd := exec.Command("docker", "compose", "up", "--build", "-d")
	cmd.Dir = "."

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to start docker compose: %v\n%s", err, string(out))
	}
}

func stopDockerCompose(t *testing.T) {
	t.Helper()

	cmd := exec.Command("docker", "compose", "down", "-v")
	cmd.Dir = "."

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to stop docker compose: %v\n%s", err, string(out))
	}
}

func waitForService(t *testing.T, baseURL string) {
	t.Helper()

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(60 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/shells")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("service did not become ready within timeout")
}

func TestAASRepository_PostAndGet(t *testing.T) {
	startDockerCompose(t)
	defer stopDockerCompose(t)

	baseURL := "http://localhost:5105"

	waitForService(t, baseURL)

	// ---- POST /shells ----
	createPayload := map[string]any{
		"id":        "aas-integration-test-1",
		"idShort":   "IntegrationTestAAS",
		"category":  "INSTANCE",
		"modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{
			"assetKind": "INSTANCE",
		},
	}

	body, err := json.Marshal(createPayload)
	if err != nil {
		t.Fatalf("failed to marshal create payload: %v", err)
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		baseURL+"/shells",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("failed to create POST request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /shells failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf(
			"unexpected POST status: %d\nresponse body: %s",
			resp.StatusCode,
			string(bodyBytes),
		)
	}

	// ---- GET /shells ----
	listResp, err := http.Get(baseURL + "/shells")
	if err != nil {
		t.Fatalf("GET /shells failed: %v", err)
	}
	defer listResp.Body.Close()

	if listResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(listResp.Body)
		t.Fatalf(
			"unexpected GET list status: %d\nresponse body: %s",
			listResp.StatusCode,
			string(bodyBytes),
		)
	}

	var shells []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&shells); err != nil {
		t.Fatalf("failed to decode shells list: %v", err)
	}

	found := false
	for _, shell := range shells {
		if shell["id"] == "aas-integration-test-1" {
			found = true

			if shell["idShort"] != "IntegrationTestAAS" {
				t.Fatalf("unexpected idShort: %v", shell["idShort"])
			}
			if shell["modelType"] != "AssetAdministrationShell" {
				t.Fatalf("unexpected modelType: %v", shell["modelType"])
			}
		}
	}

	if !found {
		t.Fatalf("created AAS not found in GET /shells response")
	}

}
