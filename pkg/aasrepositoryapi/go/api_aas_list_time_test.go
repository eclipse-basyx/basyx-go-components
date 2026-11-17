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
	"net/url"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

type aasListTimeService struct {
	AssetAdministrationShellRepositoryAPIAPIServicer
	called      bool
	createdFrom *AASListTimeCutoff
	updatedFrom *AASListTimeCutoff
}

func (s *aasListTimeService) GetAllAssetAdministrationShellsWithTimeFilters(_ context.Context, _ []string, _ string, _ int32, _ string, createdFrom, updatedFrom *AASListTimeCutoff) (model.ImplResponse, error) {
	s.called, s.createdFrom, s.updatedFrom = true, createdFrom, updatedFrom
	return model.Response(http.StatusOK, nil), nil
}

func (s *aasListTimeService) GetAllAssetAdministrationShellsReference(ctx context.Context, assetIDs []string, idShort string, limit int32, cursor string, createdFrom, updatedFrom *AASListTimeCutoff) (model.ImplResponse, error) {
	return s.GetAllAssetAdministrationShellsWithTimeFilters(ctx, assetIDs, idShort, limit, cursor, createdFrom, updatedFrom)
}

func TestAASListHandlersForwardTimeFilters(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := created.Add(time.Hour)
	for _, endpoint := range []string{"/shells", "/shells/$reference"} {
		for _, test := range []struct {
			name                string
			query               url.Values
			wantCreated         *time.Time
			wantUpdated         *time.Time
			wantCreatedFraction string
		}{
			{name: "absent"},
			{name: "created", query: url.Values{"createdFrom": {created.Format(time.RFC3339)}}, wantCreated: &created},
			{name: "updated", query: url.Values{"updatedFrom": {updated.Format(time.RFC3339)}}, wantUpdated: &updated},
			{name: "both", query: url.Values{"createdFrom": {"2026-01-02T05:04:05+02:00"}, "updatedFrom": {updated.Format(time.RFC3339)}}, wantCreated: &created, wantUpdated: &updated},
			{name: "year_one", query: url.Values{"createdFrom": {"0001-01-01T00:00:00Z"}}, wantCreated: new(time.Time)},
			{name: "subnanosecond", query: url.Values{"createdFrom": {"2026-01-02T05:04:05.000000000100+02:00"}}, wantCreated: &created, wantCreatedFraction: "0000000001"},
		} {
			t.Run(endpoint+"/"+test.name, func(t *testing.T) {
				service := &aasListTimeService{}
				response := serveAASListTimeRequest(endpoint, test.query, service)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.True(t, service.called)
				requireOptionalAASListTime(t, test.wantCreated, service.createdFrom)
				requireOptionalAASListTime(t, test.wantUpdated, service.updatedFrom)
				if service.createdFrom != nil {
					require.Equal(t, test.wantCreatedFraction, service.createdFrom.FractionalSecondDigits())
				}
			})
		}
	}
}

func requireOptionalAASListTime(t *testing.T, expected *time.Time, actual *AASListTimeCutoff) {
	t.Helper()
	if expected == nil {
		require.Nil(t, actual)
		return
	}
	require.NotNil(t, actual)
	require.True(t, expected.Equal(actual.Time()))
}

func TestAASListHandlersRejectInvalidTimeFilters(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{"/shells", "/shells/$reference"} {
		for _, parameter := range []string{"createdFrom", "updatedFrom"} {
			for _, value := range []string{"", "  ", "not-a-date", "2026-99-01T00:00:00Z"} {
				t.Run(endpoint+"/"+parameter+"/"+value, func(t *testing.T) {
					service := &aasListTimeService{}
					response := serveAASListTimeRequest(endpoint, url.Values{parameter: {value}}, service)
					require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
					require.Contains(t, response.Body.String(), parameter)
					require.False(t, service.called)
				})
			}
		}
	}
}

func serveAASListTimeRequest(endpoint string, query url.Values, service *aasListTimeService) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, endpoint+"?"+query.Encode(), nil)
	response := httptest.NewRecorder()
	controller := NewAssetAdministrationShellRepositoryAPIAPIController(service, "", "")
	if endpoint == "/shells/$reference" {
		controller.GetAllAssetAdministrationShellsReference(response, request)
	} else {
		controller.GetAllAssetAdministrationShells(response, request)
	}
	return response
}
