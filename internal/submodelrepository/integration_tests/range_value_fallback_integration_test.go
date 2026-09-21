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
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestRangeValueOnlyRejectsInvalidDateTimeInPermissiveMode(t *testing.T) {
	submodelID := fmt.Sprintf("urn:basyx:integration:range-value-fallback-%d", time.Now().UnixNano())
	endpoint := submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(submodelID)
	rangeEndpoint := endpoint + "/submodel-elements/DateTimeRange"
	status, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/submodels", map[string]any{
		"id":        submodelID,
		"idShort":   "RangeValueFallback",
		"modelType": "Submodel",
		"submodelElements": []any{map[string]any{
			"idShort":   "DateTimeRange",
			"modelType": "Range",
			"valueType": "xs:dateTime",
			"min":       "2024-04-22T00:00:00Z",
			"max":       "2024-04-23T00:00:00Z",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status, "create response: %s", body)
	t.Cleanup(func() {
		_, _, _ = requestJSON(http.MethodDelete, endpoint, nil)
	})

	status, body, err = requestJSON(http.MethodPatch, rangeEndpoint+"/$value", map[string]string{
		"min": "22.04.2024",
		"max": "2024-04-23T00:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, status, "patch response: %s", body)
	require.Contains(t, string(body), "not consistent with xs:dateTime")

	status, body, err = requestJSON(http.MethodGet, rangeEndpoint, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "get response: %s", body)
	var value struct {
		Min       string `json:"min"`
		Max       string `json:"max"`
		ValueType string `json:"valueType"`
	}
	require.NoError(t, json.Unmarshal(body, &value))
	minTime, err := time.Parse(time.RFC3339, value.Min)
	require.NoError(t, err)
	maxTime, err := time.Parse(time.RFC3339, value.Max)
	require.NoError(t, err)
	require.True(t, minTime.Equal(time.Date(2024, time.April, 22, 0, 0, 0, 0, time.UTC)))
	require.True(t, maxTime.Equal(time.Date(2024, time.April, 23, 0, 0, 0, 0, time.UTC)))
	require.Equal(t, "xs:dateTime", value.ValueType)
}
