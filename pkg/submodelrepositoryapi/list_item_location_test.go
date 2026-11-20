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
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

type listItemLocationService struct {
	SubmodelRepositoryAPIAPIServicer
	status int
}

func (s *listItemLocationService) PostSubmodelElementByPathSubmodelRepo(_ context.Context, _ string, _ string, _ types.ISubmodelElement) (model.ImplResponse, error) {
	return model.Response(s.status, model.CreatedSubmodelElement{
		Path:  "Rows[4]",
		Value: map[string]any{"modelType": "Property", "valueType": "xs:int", "value": "2"},
	}), nil
}

func TestPostListItemUsesCommittedLocationAndPreservesBody(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			service := &listItemLocationService{status: status}
			controller := NewSubmodelRepositoryAPIAPIController(service, "/api/v3", "")
			request := httptest.NewRequest(http.MethodPost, "http://internal.example/api/v3/submodels/c20/submodel-elements/Rows", strings.NewReader(`{"modelType":"Property","valueType":"xs:int","value":"2"}`))
			route := chi.NewRouteContext()

			route.URLParams.Add("submodelIdentifier", "c20")
			route.URLParams.Add("idShortPath", "Rows")
			cfg := &common.Config{}
			cfg.ABAC.Enabled = false
			cfg.General.ExternalURL = "https://public.example/environment"
			ctx := common.ContextWithConfig(request.Context(), cfg)
			request = request.WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
			response := httptest.NewRecorder()
			controller.PostSubmodelElementByPathSubmodelRepo(response, request)
			require.Equal(t, status, response.Code, response.Body.String())
			if status == http.StatusCreated {
				require.Equal(t, "https://public.example/environment/submodels/c20/submodel-elements/Rows%5B4%5D", response.Header().Get("Location"))
			} else {
				require.Empty(t, response.Header().Get("Location"))
			}
			require.JSONEq(t, `{"modelType":"Property","valueType":"xs:int","value":"2"}`, response.Body.String())
		})
	}
}
