/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

//nolint:revive
package common

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

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
	r.Get(config.Server.ContextPath+"/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("{\"status\":\"UP\"}"))
		if err != nil {
			http.Error(w, "Failed to write response", http.StatusInternalServerError)
		}
	})
}
