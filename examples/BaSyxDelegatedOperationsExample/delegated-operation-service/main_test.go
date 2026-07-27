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

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAddReturnsSum(t *testing.T) {
	requestBody := `{
		"inputArguments": [
			{"value": {"modelType": "Property", "idShort": "numberA", "valueType": "xs:int", "value": "5"}},
			{"value": {"modelType": "Property", "idShort": "numberB", "valueType": "xs:int", "value": "3"}}
		]
	}`
	request := httptest.NewRequest(http.MethodPost, "/delegate/add/sync", strings.NewReader(requestBody))
	response := httptest.NewRecorder()

	handleAdd(response, request, 0)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}

	var result []operationVariable
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result) != 1 || result[0].Value.IDShort != "sum" || result[0].Value.Value != "8" {
		t.Fatalf("unexpected operation result: %#v", result)
	}
}

func TestHandleAddRejectsOversizedRequest(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/delegate/add/sync",
		strings.NewReader(strings.Repeat("x", maxRequestBodyBytes+1)),
	)
	response := httptest.NewRecorder()

	handleAdd(response, request, 0)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d: %s", http.StatusRequestEntityTooLarge, response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "request body too large") {
		t.Fatalf("expected size-limit error, got %s", response.Body.String())
	}
}

func TestNewHTTPServerConfiguresTimeouts(t *testing.T) {
	server := newHTTPServer(":8080", http.NewServeMux())

	if server.ReadHeaderTimeout != readHeaderTimeout {
		t.Fatalf("unexpected read header timeout: %s", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != readTimeout {
		t.Fatalf("unexpected read timeout: %s", server.ReadTimeout)
	}
	if server.WriteTimeout != writeTimeout {
		t.Fatalf("unexpected write timeout: %s", server.WriteTimeout)
	}
	if server.IdleTimeout != idleTimeout {
		t.Fatalf("unexpected idle timeout: %s", server.IdleTimeout)
	}
}
