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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type valueOnlyPatchRecordingService struct {
	AssetAdministrationShellRepositoryAPIAPIServicer
	submodelBody map[string]json.RawMessage
	elementBody  json.RawMessage
}

func (s *valueOnlyPatchRecordingService) PatchSubmodelByIdValueOnlyAasRepository(_ context.Context, _ string, _ string, body map[string]json.RawMessage, _ string) (model.ImplResponse, error) {
	s.submodelBody = body
	return model.Response(http.StatusNoContent, nil), nil
}

func (s *valueOnlyPatchRecordingService) PatchSubmodelElementValueByPathValueOnly(_ context.Context, _ string, _ string, _ string, body json.RawMessage, _ string) (model.ImplResponse, error) {
	s.elementBody = body
	return model.Response(http.StatusNoContent, nil), nil
}

func TestAASValueOnlyPatchForwardsPayloadWithoutNumericRounding(t *testing.T) {
	service := &valueOnlyPatchRecordingService{}
	controller := NewAssetAdministrationShellRepositoryAPIAPIController(service, "", "")

	request := valueOnlyPatchRequest(`{"Count":18446744073709551615,"Ratio":0.1000000000000000055511151231257827}`)
	response := httptest.NewRecorder()
	controller.PatchSubmodelByIdValueOnlyAasRepository(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if string(service.submodelBody["Count"]) != "18446744073709551615" || string(service.submodelBody["Ratio"]) != "0.1000000000000000055511151231257827" {
		t.Fatalf("payload was not forwarded verbatim: %v", service.submodelBody)
	}

	request = valueOnlyPatchRequest(`18446744073709551615`)
	response = httptest.NewRecorder()
	controller.PatchSubmodelElementValueByPathValueOnly(response, request)
	if response.Code != http.StatusNoContent || string(service.elementBody) != "18446744073709551615" {
		t.Fatalf("status=%d body=%s forwarded=%s", response.Code, response.Body.String(), service.elementBody)
	}
}

func TestAASValueOnlyPatchRejectsMalformedPayloads(t *testing.T) {
	for _, body := range []string{``, `{`, `null`, `[1]`, `1 2`} {
		service := &valueOnlyPatchRecordingService{}
		controller := NewAssetAdministrationShellRepositoryAPIAPIController(service, "", "")
		response := httptest.NewRecorder()
		controller.PatchSubmodelByIdValueOnlyAasRepository(response, valueOnlyPatchRequest(body))
		if response.Code != http.StatusBadRequest || service.submodelBody != nil {
			t.Fatalf("body=%q status=%d", body, response.Code)
		}
	}
}

func valueOnlyPatchRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString(body))
	addRouteParam(request, "aasIdentifier", "aas")
	addRouteParam(request, "submodelIdentifier", "sm")
	addRouteParam(request, "idShortPath", "Count")
	return request
}
