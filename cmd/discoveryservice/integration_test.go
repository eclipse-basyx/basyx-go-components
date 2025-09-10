//go:build integration
// +build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	base64url "github.com/eclipse-basyx/basyx-go-sdk/internal/common"
	"github.com/eclipse-basyx/basyx-go-sdk/internal/discovery/config"
	openapi "github.com/eclipse-basyx/basyx-go-sdk/pkg/discoveryapi/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig is a minimal test configuration for integration testing
var testConfig = &config.Config{
	Server: config.ServerConfig{
		Host:        "localhost",
		Port:        "8099", // Use a dedicated port for testing
		ContextPath: "",
	},
	CORS: config.CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	},
	BaSyx: config.BaSyxConfig{
		Backend: "InMemory", // Always use in-memory for tests
	},
}

// saveTestConfig saves the test configuration to a temporary file
func saveTestConfig() (string, error) {
	tmpFile, err := os.CreateTemp("", "test-config-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}

	// Marshal config to JSON
	configJSON, err := json.Marshal(testConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %v", err)
	}

	// Write to temp file
	if _, err := tmpFile.Write(configJSON); err != nil {
		return "", fmt.Errorf("failed to write to temp file: %v", err)
	}

	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp file: %v", err)
	}

	return tmpFile.Name(), nil
}

// TestIntegrationDiscoveryService is the main integration test that tests the full API
func TestIntegrationDiscoveryService(t *testing.T) {
	// Save test config to temporary file
	configPath, err := saveTestConfig()
	require.NoError(t, err, "Failed to save test config")
	defer os.Remove(configPath) // Clean up

	// Set up a cancellable context to stop the server after tests
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // This will trigger server shutdown when test completes

	// Start the server in a goroutine
	go func() {
		err := runServer(ctx, configPath)
		if err != nil {
			t.Logf("Server shutdown with error: %v", err)
		}
	}()

	// Wait for server to start
	baseURL := fmt.Sprintf("http://%s:%s", testConfig.Server.Host, testConfig.Server.Port)
	var isServerUp bool
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		resp, err := http.Get(baseURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			isServerUp = true
			resp.Body.Close()
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
	require.True(t, isServerUp, "Server did not start within the expected time")

	// Now run the individual tests against the running server
	t.Run("TestHealth", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		body := buf.String()

		assert.Equal(t, "OK", body)
	})

	t.Run("TestSwaggerUI", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/swagger-ui/index.html")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("TestOpenAPISpec", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/docs/openapi.yaml")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/yaml", resp.Header.Get("Content-Type"))
	})

	// Test full API workflow
	t.Run("TestDiscoveryAPIWorkflow", func(t *testing.T) {
		// 1. Check that discovery service is initially empty
		resp, err := http.Get(baseURL + "/lookup/shells")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result openapi.GetAllAssetAdministrationShellIdsByAssetLink200Response
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Verify the structure has both properties with expected empty values
		assert.NotNil(t, result.PagingMetadata, "PagingMetadata should not be nil")
		assert.Empty(t, result.PagingMetadata, "PagingMetadata should be empty")
		assert.NotNil(t, result.Result, "Result array should not be nil")
		assert.Empty(t, result.Result, "Result array should be empty")

		// 2. Create asset IDs
		globalAssetId := openapi.SpecificAssetId{
			Name:  "globalAssetId",
			Value: "https://example.com/ids/asset/1373_9090_4042_5900",
			ExternalSubjectId: &openapi.Reference{
				Type: "ExternalReference",
				Keys: []openapi.Key{
					{
						Type:  "GlobalReference",
						Value: "test-asset-123",
					},
				},
			},
		}

		specificAssetId := openapi.SpecificAssetId{
			Name:  "specificAssetId",
			Value: "http://example.com/test-asset-123",
			ExternalSubjectId: &openapi.Reference{
				Type: "ExternalReference",
				Keys: []openapi.Key{
					{
						Type:  "GlobalReference",
						Value: "test-asset-123",
					},
				},
			},
		}

		assetIds := []openapi.SpecificAssetId{globalAssetId, specificAssetId}

		payload, err := json.Marshal(assetIds)
		require.NoError(t, err)

		// 3. Create a base64 url encoded aasId
		aasId := "https://example.com/ids/aas/1273_9090_4042_4918"
		encodedAasId := base64url.EncodeString(aasId)

		// 4. Post a new asset link
		resp, err = http.Post(
			baseURL+"/lookup/shells/"+encodedAasId,
			"application/json",
			bytes.NewBuffer(payload),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var returnedAssetIds []openapi.SpecificAssetId
		err = json.NewDecoder(resp.Body).Decode(&returnedAssetIds)
		require.NoError(t, err)

		assert.Len(t, returnedAssetIds, 2, "Response should contain both asset IDs")

		assert.Equal(t, "globalAssetId", returnedAssetIds[0].Name)
		assert.Equal(t, "https://example.com/ids/asset/1373_9090_4042_5900", returnedAssetIds[0].Value)
		assert.Equal(t, "specificAssetId", returnedAssetIds[1].Name)
		assert.Equal(t, "http://example.com/test-asset-123", returnedAssetIds[1].Value)

		// 5. Get the asset link by aasId
		resp, err = http.Get(baseURL + "/lookup/shells/" + encodedAasId)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var getResult = []openapi.SpecificAssetId{}
		err = json.NewDecoder(resp.Body).Decode(&getResult)
		require.NoError(t, err)

		assert.Len(t, getResult, 2, "Expected two asset IDs in the response")

		assert.Equal(t, "globalAssetId", getResult[0].Name)
		assert.Equal(t, "https://example.com/ids/asset/1373_9090_4042_5900", getResult[0].Value)
		assert.Equal(t, "specificAssetId", getResult[1].Name)
		assert.Equal(t, "http://example.com/test-asset-123", getResult[1].Value)

		// 6. Search for shell IDs by asset IDs
		searchAssetId := openapi.SpecificAssetId{
			Name:  "globalAssetId",
			Value: "https://example.com/ids/asset/1373_9090_4042_5900",
		}

		assetIdJSON, err := json.Marshal(searchAssetId)
		require.NoError(t, err)

		encodedAssetId := base64url.EncodeString(string(assetIdJSON))

		resp, err = http.Get(baseURL + "/lookup/shells?assetIds=" + encodedAssetId)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var searchResult openapi.GetAllAssetAdministrationShellIdsByAssetLink200Response
		err = json.NewDecoder(resp.Body).Decode(&searchResult)
		require.NoError(t, err)

		assert.NotNil(t, searchResult.PagingMetadata, "PagingMetadata should not be nil")
		assert.NotNil(t, searchResult.Result, "Result array should not be nil")

		assert.Len(t, searchResult.Result, 1, "Expected one shell ID in the response")

		assert.Equal(t, aasId, searchResult.Result[0], "Expected shell ID to match the one created earlier")

		// 7. Delete the asset link
		req, err := http.NewRequest(http.MethodDelete, baseURL+"/lookup/shells/"+encodedAasId, nil)
		require.NoError(t, err)

		client := &http.Client{}
		resp, err = client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)

		// 8. Verify shell is gone
		resp, err = http.Get(baseURL + "/lookup/shells/" + encodedAasId)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
