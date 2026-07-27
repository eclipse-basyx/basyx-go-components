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
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	commonlogging "github.com/eclipse-basyx/basyx-go-components/internal/common/logging"
)

func TestDiscoveryErrorLogsExcludeRequestValues(t *testing.T) {
	output := configureDiscoveryTestLogger(t)
	controller := NewAssetAdministrationShellBasicDiscoveryAPIAPIController(nil)

	queryRequest := httptest.NewRequest(http.MethodGet, "/lookup/shells", nil)
	queryRequest.URL.RawQuery = "access_token=sentinel-query-secret&bad=%zz"
	controller.GetAllAssetAdministrationShellIdsByAssetLink(httptest.NewRecorder(), queryRequest)

	bodyRequest := httptest.NewRequest(
		http.MethodPost,
		"/lookup/shellsByAssetLink",
		strings.NewReader(`[{"name":"","value":"sentinel-body-secret"}]`),
	)
	controller.SearchAllAssetAdministrationShellIdsByAssetLink(httptest.NewRecorder(), bodyRequest)

	for _, secret := range []string{"sentinel-query-secret", "sentinel-body-secret"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("log output contains request value %q: %s", secret, output.String())
		}
	}
}

func configureDiscoveryTestLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
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

	var output bytes.Buffer
	if _, err := commonlogging.Configure(
		commonlogging.Config{Format: commonlogging.FormatJSON, Level: commonlogging.LevelDebug},
		"discoverytest",
		&output,
	); err != nil {
		t.Fatalf("configure logger: %v", err)
	}
	return &output
}
