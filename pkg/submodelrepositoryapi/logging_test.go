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

package openapi

import (
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	commonlogging "github.com/eclipse-basyx/basyx-go-components/internal/common/logging"
)

func TestQuerySubmodelsParseErrorIsStructured(t *testing.T) {
	var output bytes.Buffer
	previousLogger := slog.Default()
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})
	if _, err := commonlogging.Configure(
		commonlogging.Config{Format: commonlogging.FormatJSON, Level: commonlogging.LevelInfo},
		"submodelrepositorytest",
		&output,
	); err != nil {
		t.Fatalf("configure logger: %v", err)
	}

	controller := NewSubmodelRepositoryAPIAPIController(nil, "", "permissive")
	request := httptest.NewRequest(http.MethodPost, "/submodels/$query", nil)
	request.URL.RawQuery = "bad=%zz"
	controller.QuerySubmodels(httptest.NewRecorder(), request)

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log record: %v", err)
	}
	if record["msg"] != "submodel query parameter parsing failed" {
		t.Fatalf("unexpected message: %#v", record["msg"])
	}
	if record["error.code"] != "SUBMODELREPOSITORYAPI-QUERYSUBMODELS-PARSEQUERY" {
		t.Fatalf("unexpected error code: %#v", record["error.code"])
	}
	if record["error"] == nil || record["error"] == "" {
		t.Fatalf("missing underlying error: %#v", record)
	}
}
