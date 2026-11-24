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
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

func TestQueryTemporalPropertyValues(t *testing.T) {
	const id = "urn:basyx:integration:temporal-query"
	const payload = `{
		"id": "urn:basyx:integration:temporal-query", "idShort": "TemporalQuery", "modelType": "Submodel",
		"submodelElements": [
			{"idShort": "TemporalGate", "modelType": "Property", "valueType": "xs:dateTime", "value": "2024-01-01T10:00:00+02:00"},
			{"idShort": "Local", "modelType": "Property", "valueType": "xs:dateTime", "value": "2024-01-01T08:00:00"},
			{"idShort": "LocalGate", "modelType": "Property", "valueType": "xs:dateTime", "value": "2024-01-01T08:00:00"},
			{"idShort": "LegacyGate", "modelType": "Property", "valueType": "xs:dateTime", "value": "2024-01-01T08:00:00+00:00"},
			{"idShort": "Date", "modelType": "Property", "valueType": "xs:date", "value": "2024-01-01Z"},
			{"idShort": "Time", "modelType": "Property", "valueType": "xs:time", "value": "10:00:00+02:00"}
		]
	}`
	provider := testenv.NewPasswordGrantTokenProvider(securityKeycloakTokenURL, "basyx-ui", 10*time.Second)
	admin := temporalQueryHeaders(t, provider, "admin")
	require.Equal(t, http.StatusCreated, testenv.DoHTTP(t, http.MethodPost, securityBaseURL+"/submodels", []byte(payload), admin).Status)
	endpoint := securityBaseURL + "/submodels/" + common.EncodeString(id)
	t.Cleanup(func() {
		require.Equal(t, http.StatusNoContent, testenv.DoHTTP(t, http.MethodDelete, endpoint, nil, admin).Status)
	})
	testenv.OverwriteSubmodelElementRow(t, queryRepositoryDSN, id, "LegacyGate", "property_element",
		goqu.Record{"value_text": nil, "value_datetime": "2024-01-01T10:00:00+02:00"})

	for _, test := range []struct {
		name, field, operand string
		match                bool
	}{
		{"declaredDateTime", "$sme.TemporalGate#valueType", `{"$strVal":"xs:dateTime"}`, true},
		{"declaredDate", "$sme.Date#valueType", `{"$strVal":"xs:date"}`, true},
		{"declaredTime", "$sme.Time#valueType", `{"$strVal":"xs:time"}`, true},
		{"dateTimeIsNotString", "$sme.TemporalGate#valueType", `{"$strVal":"xs:string"}`, false},
		{"timeLexicalValue", "$sme.Time#value", `{"$strVal":"10:00:00+02:00"}`, true},
		{"dateLexicalValue", "$sme.Date#value", `{"$strVal":"2024-01-01Z"}`, true},
		{"dateTimeLexicalValue", "$sme.TemporalGate#value", `{"$strVal":"2024-01-01T10:00:00+02:00"}`, true},
		{"dateTimeInstant", "$sme.TemporalGate#value", `{"$dateTimeVal":"2024-01-01T08:00:00Z"}`, true},
		{"differentInstant", "$sme.TemporalGate#value", `{"$dateTimeVal":"2024-01-01T10:00:00Z"}`, false},
		{"localDateTimeLexicalValue", "$sme.Local#value", `{"$strVal":"2024-01-01T08:00:00"}`, true},
		{"localDateTimeIsUTC", "$sme.Local#value", `{"$dateTimeVal":"2024-01-01T08:00:00Z"}`, true},
		{"legacyDateTimeText", "$sme.LegacyGate#value", `{"$strVal":"2024-01-01T08:00:00+00:00"}`, true},
		{"legacyDateTimeInstant", "$sme.LegacyGate#value", `{"$dateTimeVal":"2024-01-01T08:00:00Z"}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			condition := map[string]any{"$and": []any{
				map[string]any{"$eq": []any{map[string]string{"$field": "$sm#id"}, map[string]string{"$strVal": id}}},
				map[string]any{"$eq": []any{map[string]string{"$field": test.field}, json.RawMessage(test.operand)}},
			}}
			request, err := json.Marshal(map[string]any{"$condition": condition})
			require.NoError(t, err)
			response := testenv.DoHTTP(t, http.MethodPost, securityBaseURL+"/query/submodels", request, admin)
			require.Equal(t, http.StatusOK, response.Status, "%s", response.Body)
			var page struct{ Result []struct{ ID string } }
			require.NoError(t, json.Unmarshal(response.Body, &page))
			if test.match {
				require.Len(t, page.Result, 1)
				require.Equal(t, id, page.Result[0].ID)
			} else {
				require.Empty(t, page.Result)
			}
		})
	}

	t.Run("access rule compares the instant", func(t *testing.T) {
		editor := temporalQueryHeaders(t, provider, "userx")
		elementIDs := readTemporalQueryElementIDs(t, endpoint, editor)
		require.Subset(t, elementIDs, []string{"TemporalGate", "LocalGate", "LegacyGate"})
		response := testenv.DoHTTP(t, http.MethodPut, endpoint+"/submodel-elements/TemporalGate",
			[]byte(`{"idShort":"TemporalGate","modelType":"Property","valueType":"xs:dateTime","value":"2024-01-01T10:00:00Z"}`), admin)
		require.Equal(t, http.StatusNoContent, response.Status, "%s", response.Body)
		elementIDs = readTemporalQueryElementIDs(t, endpoint, editor)
		require.NotContains(t, elementIDs, "TemporalGate")
		require.Contains(t, elementIDs, "Local")
	})
}

func temporalQueryHeaders(t *testing.T, provider *testenv.PasswordGrantTokenProvider, user string) map[string]string {
	t.Helper()
	token, err := provider.GetAccessToken(&testenv.TokenCredentials{User: user, Password: "pwd"})
	require.NoError(t, err)
	return map[string]string{"Authorization": "Bearer " + token}
}

func readTemporalQueryElementIDs(t *testing.T, endpoint string, headers map[string]string) []string {
	t.Helper()
	response := testenv.DoHTTP(t, http.MethodGet, endpoint, nil, headers)
	require.Equal(t, http.StatusOK, response.Status, "%s", response.Body)
	var submodel struct{ SubmodelElements []struct{ IDShort string } }
	require.NoError(t, json.Unmarshal(response.Body, &submodel))
	ids := make([]string, 0, len(submodel.SubmodelElements))
	for _, element := range submodel.SubmodelElements {
		ids = append(ids, element.IDShort)
	}
	return ids
}
