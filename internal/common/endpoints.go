/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

//nolint:all
package common

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// HealthProbe reports if the service is healthy and optionally returns detail text.
type HealthProbe func() (bool, string)

// AddHealthEndpoint registers a health check endpoint on the provided router.
//
// The health endpoint provides a simple way to verify that the service is running
// and responsive. It's commonly used by load balancers, monitoring systems,
// and container orchestrators to determine service health.
//
// Endpoint details:
//   - Method: GET
//   - Path: {contextPath}/health
//   - Response: HTTP 200 with JSON body {"status":"UP"}
//   - Content-Type: application/json (implicit)
//
// Parameters:
//   - r: Chi router to register the health endpoint on
//   - config: Configuration containing the server context path
//
// Example:
//
//	router := chi.NewRouter()
//	config := &Config{Server: ServerConfig{ContextPath: "/api/v1"}}
//	AddHealthEndpoint(router, config)
//	// Health check available at: GET /api/v1/health
//
// Response format:
//
//	{
//	  "status": "UP"
//	}
func AddHealthEndpoint(r *chi.Mux, config *Config) {
	AddHealthEndpointWithProbe(r, config, nil)
}

// AddHealthEndpointWithProbe registers a health endpoint with optional readiness probing.
func AddHealthEndpointWithProbe(r *chi.Mux, config *Config, probe HealthProbe) {
	r.Get(config.Server.ContextPath+"/health", func(w http.ResponseWriter, _ *http.Request) {
		if probe != nil {
			healthy, details := probe()
			if !healthy {
				response := map[string]string{"status": "DOWN"}
				if strings.TrimSpace(details) != "" {
					response["details"] = details
				}
				writeHealthResponse(w, http.StatusServiceUnavailable, response)
				return
			}
		}

		writeHealthResponse(w, http.StatusOK, map[string]string{"status": "UP"})
	})
}

func writeHealthResponse(w http.ResponseWriter, statusCode int, body map[string]string) {
	responsePayload, err := json.Marshal(body)
	if err != nil {
		log.Printf("COMMON-WRITEHEALTH-MARSHAL response marshal failed: %v", err)
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if _, err = w.Write(responsePayload); err != nil {
		log.Printf("COMMON-WRITEHEALTH-WRITE response write failed: %v", err)
	}
}
