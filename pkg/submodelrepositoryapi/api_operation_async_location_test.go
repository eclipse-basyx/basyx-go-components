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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

type operationAsyncLocationService struct {
	SubmodelRepositoryAPIAPIServicer
}

func (s *operationAsyncLocationService) InvokeOperationAsync(
	_ context.Context,
	_ string,
	_ string,
	_ model.OperationRequest,
) (model.ImplResponse, error) {
	return model.Response(http.StatusAccepted, Redirect{
		Location: "/submodels/c20/submodel-elements/Ops.Add/operation-status/handle-1",
	}), nil
}

func (s *operationAsyncLocationService) InvokeOperationSubmodelRepo(
	_ context.Context,
	_ string,
	_ string,
	_ model.OperationRequest,
	_ bool,
) (model.ImplResponse, error) {
	return model.Response(http.StatusAccepted, Redirect{
		Location: "/submodels/c20/submodel-elements/Ops.Add/operation-status/handle-1",
	}), nil
}

func (s *operationAsyncLocationService) GetOperationAsyncStatus(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (model.ImplResponse, error) {
	return model.Response(http.StatusFound, Redirect{
		Location: "/submodels/c20/submodel-elements/Ops.Add/operation-results/handle-1",
	}), nil
}

func TestInvokeOperationAsyncReturnsContextAwareLocation(t *testing.T) {
	t.Parallel()

	request := operationAsyncRequest(t, "/api/v3/submodels/c20/submodel-elements/Ops.Add/invoke-async")
	response := httptest.NewRecorder()
	controller := NewSubmodelRepositoryAPIAPIController(&operationAsyncLocationService{}, "", "")

	controller.InvokeOperationAsync(response, request)

	require.Equal(t, http.StatusAccepted, response.Code)
	require.Empty(t, response.Body.String())
	require.Equal(
		t,
		"http://example.com/api/v3/submodels/c20/submodel-elements/Ops.Add/operation-status/handle-1",
		response.Header().Get("Location"),
	)
}

func TestInvokeCompatibilityAliasReturnsContextAwareLocation(t *testing.T) {
	t.Parallel()

	request := operationAsyncRequest(t, "/api/v3/submodels/c20/submodel-elements/Ops.Add/invoke?async=true")
	response := httptest.NewRecorder()
	controller := NewSubmodelRepositoryAPIAPIController(&operationAsyncLocationService{}, "", "")

	controller.InvokeOperationSubmodelRepo(response, request)

	require.Equal(t, http.StatusAccepted, response.Code)
	require.Empty(t, response.Body.String())
	require.Equal(
		t,
		"http://example.com/api/v3/submodels/c20/submodel-elements/Ops.Add/operation-status/handle-1",
		response.Header().Get("Location"),
	)
}

func TestGetOperationAsyncStatusReturnsContextAwareResultLocation(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(
		http.MethodGet,
		"http://example.com/api/v3/submodels/c20/submodel-elements/Ops.Add/operation-status/handle-1",
		nil,
	)
	addRouteParam(request, "submodelIdentifier", "c20")
	addRouteParam(request, "idShortPath", "Ops.Add")
	addRouteParam(request, "handleId", "handle-1")
	response := httptest.NewRecorder()
	controller := NewSubmodelRepositoryAPIAPIController(&operationAsyncLocationService{}, "", "")

	controller.GetOperationAsyncStatus(response, request)

	require.Equal(t, http.StatusFound, response.Code)
	require.Equal(
		t,
		"http://example.com/api/v3/submodels/c20/submodel-elements/Ops.Add/operation-results/handle-1",
		response.Header().Get("Location"),
	)
}

func TestInvokeOperationAsyncUsesAASEnvironmentExternalURL(t *testing.T) {
	t.Parallel()

	request := operationAsyncRequest(t, "/internal/submodels/c20/submodel-elements/Ops.Add/invoke-async")
	cfg := &common.Config{}
	cfg.General.ExternalURL = "https://public.example/aas-env"
	request = request.WithContext(common.ContextWithConfig(request.Context(), cfg))
	response := httptest.NewRecorder()
	controller := NewSubmodelRepositoryAPIAPIController(&operationAsyncLocationService{}, "", "")

	controller.InvokeOperationAsync(response, request)

	require.Equal(t, http.StatusAccepted, response.Code)
	require.Equal(
		t,
		"https://public.example/aas-env/submodels/c20/submodel-elements/Ops.Add/operation-status/handle-1",
		response.Header().Get("Location"),
	)
}

func operationAsyncRequest(t *testing.T, requestURL string) *http.Request {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "http://example.com"+requestURL, bytes.NewBufferString(`{"clientTimeoutDuration":"PT5S"}`))
	request.Header.Set("Content-Type", "application/json")
	addRouteParam(request, "submodelIdentifier", "c20")
	addRouteParam(request, "idShortPath", "Ops.Add")
	return request
}
