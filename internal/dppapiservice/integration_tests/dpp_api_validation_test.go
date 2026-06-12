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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package integration_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dppapi "github.com/eclipse-basyx/basyx-go-components/pkg/dppapiservice/go"
)

func TestReadDPPRejectsInvalidRepresentation(t *testing.T) {
	service := dppapi.NewDPPRepositoryService(nil, nil)
	router := dppapi.NewRouter(dppapi.NewDPPRepositoryRouter(service))

	request := httptest.NewRequest(http.MethodGet, "/v1/dpps/dpp-1?representation=invalid", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "DPP-REPRESENTATION-INVALID") {
		t.Fatalf("response body does not contain error code: %s", response.Body.String())
	}
}

func TestCreateDPPRejectsMissingHeaderBeforePersistence(t *testing.T) {
	service := dppapi.NewDPPRepositoryService(nil, nil)
	router := dppapi.NewRouter(dppapi.NewDPPRepositoryRouter(service))

	request := httptest.NewRequest(http.MethodPost, "/v1/dpps", strings.NewReader(`{"carbonFootprint":{}}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "DPP-HEADER-MISSING") {
		t.Fatalf("response body does not contain validation error code: %s", response.Body.String())
	}
}
