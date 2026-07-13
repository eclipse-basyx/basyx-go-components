/*
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
 */

/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.2.0
 * Contact: info@idtwin.org
 */

package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type operationRequestParsingService struct {
	SubmodelRepositoryAPIAPIServicer
	invoked bool
}

func (s *operationRequestParsingService) InvokeOperationSubmodelRepo(_ context.Context, _ string, _ string, _ model.OperationRequest, _ bool) (model.ImplResponse, error) {
	s.invoked = true
	return model.Response(http.StatusOK, nil), nil
}

func (s *operationRequestParsingService) InvokeOperationValueOnly(_ context.Context, _ string, _ string, _ string, _ model.OperationRequestValueOnly, _ bool) (model.ImplResponse, error) {
	s.invoked = true
	return model.Response(http.StatusOK, nil), nil
}

func TestInvokeOperationSubmodelRepoReturnsStandardizedErrorForBooleanValue(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/submodels/sm/submodel-elements/operation/invoke",
		bytes.NewBufferString(`{"inputArguments":[{"value":{"modelType":"Property","idShort":"enabled","valueType":"xs:boolean","value":true}}]}`),
	)
	addRouteParam(request, "submodelIdentifier", "sm")
	addRouteParam(request, "idShortPath", "operation")

	service := &operationRequestParsingService{}
	controller := NewSubmodelRepositoryAPIAPIController(service, "", "")
	response := httptest.NewRecorder()

	controller.InvokeOperationSubmodelRepo(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, response.Code, response.Body.String())
	}
	if service.invoked {
		t.Fatal("expected invalid request to be rejected before service invocation")
	}

	assertStandardizedOperationError(t, response.Body.Bytes(), "400")
}

func TestInvokeOperationValueOnlyReturnsUnprocessableEntityForMissingRequiredField(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/submodels/sm/submodel-elements/operation/invoke/$value",
		bytes.NewBufferString(`{}`),
	)
	addRouteParam(request, "submodelIdentifier", "sm")
	addRouteParam(request, "idShortPath", "operation")

	service := &operationRequestParsingService{}
	controller := NewSubmodelRepositoryAPIAPIController(service, "", "")
	response := httptest.NewRecorder()

	controller.InvokeOperationValueOnly(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusUnprocessableEntity, response.Code, response.Body.String())
	}
	if service.invoked {
		t.Fatal("expected invalid request to be rejected before service invocation")
	}

	assertStandardizedOperationError(t, response.Body.Bytes(), "422")
}

func assertStandardizedOperationError(t *testing.T, responseBody []byte, expectedCode string) {
	t.Helper()

	var body []common.ErrorHandler
	if err := json.Unmarshal(responseBody, &body); err != nil {
		t.Fatalf("failed to decode standardized error response: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("expected one error entry, got %d", len(body))
	}
	if body[0].MessageType != "Error" || body[0].Code != expectedCode {
		t.Fatalf("expected standardized error code %q, got %#v", expectedCode, body[0])
	}
	if body[0].Text == "" || body[0].CorrelationID == "" || body[0].Timestamp == "" {
		t.Fatalf("expected populated standardized error fields, got %#v", body[0])
	}
}
