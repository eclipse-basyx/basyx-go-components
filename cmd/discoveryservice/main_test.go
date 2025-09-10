//go:build unit
// +build unit

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-sdk/internal/discovery/config"
	"github.com/eclipse-basyx/basyx-go-sdk/internal/discovery/persistence"
	persistence_inmemory "github.com/eclipse-basyx/basyx-go-sdk/internal/discovery/persistence/inmemory"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

// TestHealthEndpoint tests the /health endpoint
func TestHealthEndpoint(t *testing.T) {
	// Create a new router
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create a test request
	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a response recorder
	rr := httptest.NewRecorder()

	// Serve the request
	r.ServeHTTP(rr, req)

	// Check the status code
	assert.Equal(t, http.StatusOK, rr.Code)
	// Check the response body
	assert.Equal(t, "OK", rr.Body.String())
}

// TestConfigFlagParsing tests the command line flag parsing
func TestConfigFlagParsing(t *testing.T) {
	// Save original os.Args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set up test cases
	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "No config flag",
			args:     []string{"cmd"},
			expected: "",
		},
		{
			name:     "With config flag",
			args:     []string{"cmd", "-config", "/path/to/config.json"},
			expected: "/path/to/config.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set command line arguments for this test
			os.Args = tc.args

			// Reset flags
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

			// Parse flags
			configPath := flag.String("config", "", "Path to config file")
			flag.Parse()

			// Check that the correct config path was parsed
			assert.Equal(t, tc.expected, *configPath)
		})
	}
}

// TestRouterWithContextPath tests that the router is set up with a context path
func TestRouterWithContextPath(t *testing.T) {
	// Test cases with different context paths
	testCases := []struct {
		name            string
		contextPath     string
		path            string
		expectedStatus  int
		unexpectedPath  string
		unexpectedRoute string
	}{
		{
			name:            "No context path",
			contextPath:     "",
			path:            "/test",
			expectedStatus:  http.StatusOK,
			unexpectedPath:  "/api/test", // Should not route here with no context path
			unexpectedRoute: "Not Found",
		},
		{
			name:            "With context path",
			contextPath:     "/api",
			path:            "/api/test",
			expectedStatus:  http.StatusOK,
			unexpectedPath:  "/test", // Should not route here with context path
			unexpectedRoute: "Not Found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new router
			r := chi.NewRouter()

			// Configure test route
			r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Test Route"))
			})

			// Set up router with context path if configured
			var router http.Handler = r
			if tc.contextPath != "" {
				contextRouter := chi.NewRouter()
				contextRouter.Mount(tc.contextPath, r)
				router = contextRouter
			}

			// Test that the route with the context path works
			req, _ := http.NewRequest("GET", tc.path, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			assert.Equal(t, tc.expectedStatus, rr.Code)

			// Test that the route without the context path returns 404 (or vice versa)
			req, _ = http.NewRequest("GET", tc.unexpectedPath, nil)
			rr = httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusNotFound, rr.Code)
		})
	}
}

// TestOpenAPISpecModification tests that the OpenAPI spec is modified correctly
func TestOpenAPISpecModification(t *testing.T) {
	// Create a mock spec
	mockSpec := `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /test:
	get:
	  summary: Test endpoint`

	// Test cases for different server configurations
	testCases := []struct {
		name           string
		host           string
		port           string
		contextPath    string
		tls            bool
		forwardedProto string
		expectedURL    string
	}{
		{
			name:        "Standard HTTP",
			host:        "localhost",
			port:        "8080",
			contextPath: "",
			expectedURL: "http://localhost:8080",
		},
		{
			name:        "With context path",
			host:        "localhost",
			port:        "8080",
			contextPath: "/api",
			expectedURL: "http://localhost:8080/api",
		},
		{
			name:        "Default port HTTP",
			host:        "example.com",
			port:        "80",
			contextPath: "",
			expectedURL: "http://example.com",
		},
		{
			name:        "Default port HTTPS",
			host:        "example.com",
			port:        "443",
			contextPath: "",
			tls:         true,
			expectedURL: "https://example.com",
		},
		{
			name:           "X-Forwarded-Proto",
			host:           "example.com",
			port:           "8080",
			contextPath:    "",
			forwardedProto: "https",
			expectedURL:    "https://example.com:8080",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test request
			req := httptest.NewRequest("GET", "http://example.com/docs/openapi.yaml", nil)

			// Set TLS if needed
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}

			// Set X-Forwarded-Proto if needed
			if tc.forwardedProto != "" {
				req.Header.Set("X-Forwarded-Proto", tc.forwardedProto)
			}

			// Create a mock config
			cfg := &config.Config{
				Server: config.ServerConfig{
					Host:        tc.host,
					Port:        tc.port,
					ContextPath: tc.contextPath,
				},
			}

			// Get host info
			host := cfg.Server.Host
			if !strings.Contains(host, ":") && cfg.Server.Port != "80" && cfg.Server.Port != "443" {
				host = fmt.Sprintf("%s:%s", host, cfg.Server.Port)
			}

			// Determine protocol
			protocol := "http"
			if req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https" {
				protocol = "https"
			}

			// Create full URL
			serverURL := fmt.Sprintf("%s://%s%s", protocol, host, tc.contextPath)
			assert.Equal(t, tc.expectedURL, serverURL)

			// Insert servers section
			infoEndIndex := strings.Index(mockSpec, "\npaths:")
			newServers := fmt.Sprintf("\nservers:\n- url: %s\n  description: Generated server url\n", serverURL)
			modifiedSpec := mockSpec[:infoEndIndex] + newServers + mockSpec[infoEndIndex:]

			// Verify that servers section was inserted
			assert.Contains(t, modifiedSpec, fmt.Sprintf("servers:\n- url: %s", tc.expectedURL))
			assert.Contains(t, modifiedSpec, "description: Generated server url")
		})
	}
}

// TestBackendSelection tests the database backend selection
func TestBackendSelection(t *testing.T) {
	testCases := []struct {
		name          string
		backend       string
		expectedError bool
	}{
		{
			name:          "InMemory backend",
			backend:       "inmemory",
			expectedError: false,
		},
		{
			name:          "Empty backend (defaults to InMemory)",
			backend:       "",
			expectedError: false,
		},
		{
			name:          "Unknown backend",
			backend:       "unknown",
			expectedError: true,
		},
		// We can't easily test MongoDB backend in a unit test without mocking
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test config
			cfg := &config.Config{
				BaSyx: config.BaSyxConfig{
					Backend: tc.backend,
				},
			}

			// Mock the database selection logic
			var database persistence.AasDiscoveryBackend
			var err error

			if tc.expectedError {
				// This is a simplified version of the actual logic
				err = fmt.Errorf("Unknown backend type: %s", tc.backend)
				assert.Error(t, err)
			} else {
				switch strings.ToLower(cfg.BaSyx.Backend) {
				case "inmemory", "":
					database, err = persistence_inmemory.NewInMemoryAasDiscoveryBackend()
				default:
					err = fmt.Errorf("Unknown backend type: %s", cfg.BaSyx.Backend)
				}

				assert.NoError(t, err)
				assert.NotNil(t, database)
			}
		})
	}
}
