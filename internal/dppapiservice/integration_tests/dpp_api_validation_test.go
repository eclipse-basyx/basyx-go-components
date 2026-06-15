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

func TestDPPAPIRejectsInvalidQueryParameters(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		statusCode int
		errorCode  string
	}{
		{
			name:       "read by id rejects invalid representation",
			method:     http.MethodGet,
			target:     "/v1/dpps/dpp-1?representation=invalid",
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-REPRESENTATION-INVALID",
		},
		{
			name:       "read by product id rejects invalid representation",
			method:     http.MethodGet,
			target:     "/v1/dppsByProductId/product-1?representation=invalid",
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-REPRESENTATION-INVALID",
		},
		{
			name:       "history read rejects missing date",
			method:     http.MethodGet,
			target:     "/v1/dppsByIdAndDate/dpp-1",
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-READDATE-PARSE",
		},
		{
			name:       "history read rejects invalid date",
			method:     http.MethodGet,
			target:     "/v1/dppsByIdAndDate/dpp-1?date=not-a-date",
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-READDATE-PARSE",
		},
		{
			name:       "element read rejects invalid representation",
			method:     http.MethodGet,
			target:     "/v1/dpps/dpp-1/elements/technicalData/manufacturerName?representation=invalid",
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-REPRESENTATION-INVALID",
		},
		{
			name:       "product id search rejects zero limit",
			method:     http.MethodPost,
			target:     "/v1/dppsByProductIds?limit=0",
			body:       `{"productIds":["product-1"]}`,
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-READIDS-LIMIT",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertDPPAPIError(t, test.method, test.target, test.body, test.statusCode, test.errorCode)
		})
	}
}

func TestDPPAPIRejectsInvalidPayloadsBeforePersistence(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		statusCode int
		errorCode  string
	}{
		{
			name:       "create rejects malformed json",
			method:     http.MethodPost,
			target:     "/v1/dpps",
			body:       `{"digitalProductPassportId":`,
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-DECDOC-DECODE",
		},
		{
			name:       "create rejects non object payload",
			method:     http.MethodPost,
			target:     "/v1/dpps",
			body:       `null`,
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-DECDOC-EMPTY",
		},
		{
			name:       "create rejects missing header",
			method:     http.MethodPost,
			target:     "/v1/dpps",
			body:       `{"carbonFootprint":{}}`,
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-HEADER-MISSING",
		},
		{
			name:       "create rejects blank header value",
			method:     http.MethodPost,
			target:     "/v1/dpps",
			body:       withDPPField("digitalProductPassportId", `""`),
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-HEADER-INVALID",
		},
		{
			name:       "create rejects invalid timestamp",
			method:     http.MethodPost,
			target:     "/v1/dpps",
			body:       withDPPField("lastUpdate", `"not-a-date"`),
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-HEADER-PARSETIME",
		},
		{
			name:       "create rejects empty contentSpecificationIds",
			method:     http.MethodPost,
			target:     "/v1/dpps",
			body:       withDPPField("contentSpecificationIds", `[]`),
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-HEADER-MISSING",
		},
		{
			name:       "create rejects content section scalar",
			method:     http.MethodPost,
			target:     "/v1/dpps",
			body:       withDPPField("technicalData", `"not-an-object"`),
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-BUILDSM-CONTENTSECTION",
		},
		{
			name:       "create rejects ambiguous content specification",
			method:     http.MethodPost,
			target:     "/v1/dpps",
			body:       withDPPField("contentSpecificationIds", `["technicalData alpha","technicalData beta"]`),
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-SEMSPEC-AMBIGUOUS",
		},
		{
			name:       "update rejects malformed json",
			method:     http.MethodPatch,
			target:     "/v1/dpps/dpp-1",
			body:       `{"technicalData":`,
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-DECDOC-DECODE",
		},
		{
			name:       "product id search rejects malformed json",
			method:     http.MethodPost,
			target:     "/v1/dppsByProductIds",
			body:       `{"productIds":`,
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-READIDS-DECODE",
		},
		{
			name:       "product id search rejects unknown fields",
			method:     http.MethodPost,
			target:     "/v1/dppsByProductIds",
			body:       `{"productIds":["product-1"],"extra":true}`,
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-READIDS-DECODE",
		},
		{
			name:       "product id search rejects missing product ids",
			method:     http.MethodPost,
			target:     "/v1/dppsByProductIds",
			body:       `{}`,
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-READIDS-MISSING",
		},
		{
			name:       "product id search rejects blank product ids",
			method:     http.MethodPost,
			target:     "/v1/dppsByProductIds",
			body:       `{"productIds":[" "]}`,
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-READIDS-INVALID",
		},
		{
			name:       "element update rejects malformed json",
			method:     http.MethodPut,
			target:     "/v1/dpps/dpp-1/elements/technicalData/manufacturerName",
			body:       `{"value":`,
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-UPDELEM-DECODE",
		},
		{
			name:       "element read rejects invalid path",
			method:     http.MethodGet,
			target:     "/v1/dpps/dpp-1/elements/technicalData",
			statusCode: http.StatusBadRequest,
			errorCode:  "DPP-ELEMPATH-INVALID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertDPPAPIError(t, test.method, test.target, test.body, test.statusCode, test.errorCode)
		})
	}
}

func assertDPPAPIError(t *testing.T, method string, target string, body string, statusCode int, errorCode string) {
	t.Helper()
	service := dppapi.NewDPPRepositoryService(nil, nil)
	router := dppapi.NewRouter(dppapi.NewDPPRepositoryRouter(service))

	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != statusCode {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, statusCode, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), errorCode) {
		t.Fatalf("response body does not contain %s: %s", errorCode, response.Body.String())
	}
}

func withDPPField(field string, value string) string {
	replacements := map[string]string{
		"digitalProductPassportId": `"digitalProductPassportId":"dpp-1"`,
		"lastUpdate":               `"lastUpdate":"2026-01-02T03:04:05Z"`,
		"contentSpecificationIds":  `"contentSpecificationIds":["technicalData-specification"]`,
		"technicalData":            `"technicalData":{"manufacturerName":"Acme GmbH"}`,
	}
	return strings.Replace(validDPPBody(), replacements[field], `"`+field+`":`+value, 1)
}

func validDPPBody() string {
	return `{
		"digitalProductPassportId":"dpp-1",
		"uniqueProductIdentifier":"product-1",
		"granularity":"item",
		"dppSchemaVersion":"1.0.0",
		"dppStatus":"active",
		"lastUpdate":"2026-01-02T03:04:05Z",
		"economicOperatorId":"operator-1",
		"facilityId":"facility-1",
		"contentSpecificationIds":["technicalData-specification"],
		"technicalData":{"manufacturerName":"Acme GmbH"}
	}`
}
