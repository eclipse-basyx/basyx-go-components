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

package openapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type valueOnlyOperationParsingService struct {
	AssetAdministrationShellRepositoryAPIAPIServicer
	invoked bool
}

func (s *valueOnlyOperationParsingService) InvokeOperationValueOnlyAasRepository(_ context.Context, _ string, _ string, _ string, _ model.OperationRequestValueOnly) (model.ImplResponse, error) {
	s.invoked = true
	return model.Response(http.StatusOK, nil), nil
}

func (s *valueOnlyOperationParsingService) InvokeOperationAsyncValueOnlyAasRepository(_ context.Context, _ string, _ string, _ string, _ model.OperationRequestValueOnly) (model.ImplResponse, error) {
	s.invoked = true
	return model.Response(http.StatusAccepted, nil), nil
}

func TestAASValueOnlyOperationTimeoutRequirement(t *testing.T) {
	for _, test := range []struct {
		name, body string
		async      bool
		status     int
	}{
		{"sync_without_timeout", `{}`, false, http.StatusOK},
		{"async_without_timeout", `{}`, true, http.StatusBadRequest},
		{"async_empty_timeout", `{"clientTimeoutDuration":""}`, true, http.StatusBadRequest},
		{"async_blank_timeout", `{"clientTimeoutDuration":" "}`, true, http.StatusBadRequest},
		{"async_with_timeout", `{"clientTimeoutDuration":"PT1S"}`, true, http.StatusAccepted},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(test.body))
			addRouteParam(request, "aasIdentifier", "aas")
			addRouteParam(request, "submodelIdentifier", "sm")
			addRouteParam(request, "idShortPath", "operation")
			service := &valueOnlyOperationParsingService{}
			controller := NewAssetAdministrationShellRepositoryAPIAPIController(service, "", "")
			response := httptest.NewRecorder()
			if test.async {
				controller.InvokeOperationAsyncValueOnlyAasRepository(response, request)
			} else {
				controller.InvokeOperationValueOnlyAasRepository(response, request)
			}
			if response.Code != test.status || service.invoked != (test.status < 400) {
				t.Fatalf("status=%d invoked=%t body=%s", response.Code, service.invoked, response.Body.String())
			}
		})
	}
}
