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

package testenv

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// APIConformanceEndpoint identifies a paginated API boundary and its encoded filters.
type APIConformanceEndpoint struct {
	Path           string
	Method         string
	Body           string
	Filters        []string
	IdentifierPath string
}

// RunAPIParameterConformance exercises the same malformed inputs against composed and standalone services.
func RunAPIParameterConformance(t *testing.T, baseURL string, headers http.Header, endpoints []APIConformanceEndpoint) {
	t.Helper()
	for _, endpoint := range endpoints {
		t.Run(endpoint.Path, func(t *testing.T) { runAPIEndpointConformance(t, baseURL, headers, endpoint) })
	}
}

func runAPIEndpointConformance(t *testing.T, baseURL string, headers http.Header, endpoint APIConformanceEndpoint) {
	t.Helper()
	runInvalidAPIParameterCases(t, baseURL, headers, endpoint, "limit", []string{"", "0", "-1", "1.5", "2147483648"})
	runInvalidAPIParameterCases(t, baseURL, headers, endpoint, "cursor", []string{"", "YQ=", "YQ===", "YR==", "YR", "Y/Q", "Y+Q", "YQ\n", " YQ", "_w"})
	for _, filter := range endpoint.Filters {
		values := []string{"", "YQ=", "YQ\n", "_w"}
		if filter != "assetType" {
			values = append(values, common.EncodeString("null"), common.EncodeString("[]"), common.EncodeString("{}"), common.EncodeString("urn:plain"))
		}
		runInvalidAPIParameterCases(t, baseURL, headers, endpoint, filter, values)
	}
	if endpoint.IdentifierPath != "" {
		for _, value := range []string{"YQ=", "YR", "_w"} {
			invalid := APIConformanceEndpoint{Path: endpoint.IdentifierPath + value}
			t.Run("identifier="+value, func(t *testing.T) { requireAPIParameterError(t, baseURL, headers, invalid, nil) })
		}
	}
}

func runInvalidAPIParameterCases(t *testing.T, baseURL string, headers http.Header, endpoint APIConformanceEndpoint, name string, values []string) {
	t.Helper()
	for _, value := range values {
		t.Run(name+"="+value, func(t *testing.T) { requireAPIParameterError(t, baseURL, headers, endpoint, url.Values{name: {value}}) })
	}
}

func requireAPIParameterError(t *testing.T, baseURL string, headers http.Header, endpoint APIConformanceEndpoint, query url.Values) {
	t.Helper()
	method := endpoint.Method
	if method == "" {
		method = http.MethodGet
	}
	request, err := http.NewRequest(method, baseURL+endpoint.Path+"?"+query.Encode(), strings.NewReader(endpoint.Body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header = headers.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
	var errors []common.ErrorHandler
	if err := json.Unmarshal(body, &errors); err != nil || len(errors) == 0 {
		t.Fatalf("missing structured errors: %s", body)
	}
	if errors[0].Code != "400" || errors[0].CorrelationID == "" {
		t.Fatalf("missing coded error: %s", body)
	}
}
