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

package dppapiservice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

var testOpenAPISpec = fstest.MapFS{
	"openapi.yaml": {
		Data: []byte("openapi: 3.0.3\ninfo:\n  title: DPP API Test\n  version: V3.2.0\npaths:\n  /v1/dpps:\n    get:\n      responses:\n        '200':\n          description: ok\n"),
	},
}

func TestNewHTTPHandlerDoesNotShowVerifyEndpointInSwagger(t *testing.T) {
	cfg := &common.Config{
		Server: common.ServerConfig{
			Host:                          "0.0.0.0",
			Port:                          8080,
			VerificationEndpointAvailable: true,
		},
		Swagger: common.SwaggerConfig{
			Enabled: true,
		},
	}

	handler, err := NewHTTPHandler(context.TODO(), cfg, testOpenAPISpec, nil, nil)
	if err != nil {
		t.Fatalf("unexpected NewHTTPHandler error: %v", err)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api-docs/openapi.yaml", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected OpenAPI spec status 200, got %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "\n  /verify:\n") {
		t.Fatal("expected DPP Swagger spec to not include /verify")
	}
	if !cfg.Server.VerificationEndpointAvailable {
		t.Fatal("expected DPP Swagger setup to not mutate runtime config")
	}
}
