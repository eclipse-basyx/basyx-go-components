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

	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/require"
)

type temporalQueryCase struct {
	name, condition string
	match           bool
}

func TestTemporalQueriesAreIndependentOfDatabaseTimeZone(t *testing.T) {
	const id = "urn:basyx:integration:temporal-query-time-zones"
	elements := []temporalElement{
		{IDShort: "Local", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-01-01T08:00:00")},
		{IDShort: "LateUTC", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-01-01T23:30:00Z")},
		{IDShort: "EarlyOffset", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-01-01T00:30:00+02:00")},
		{IDShort: "LocalDate", ModelType: "Property", ValueType: "xs:date", Value: new("2024-01-01")},
		{IDShort: "OffsetTime", ModelType: "Property", ValueType: "xs:time", Value: new("10:00:00+02:00")},
		{IDShort: "EarlyYear", ModelType: "Property", ValueType: "xs:dateTime", Value: new("-0001-01-01T00:00:00Z")},
		{IDShort: "EndOfDay", ModelType: "Property", ValueType: "xs:time", Value: new("24:00:00")},
		{IDShort: "LegacyDateTime", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-01-01T08:00:00+00:00")},
		{IDShort: "LegacyDate", ModelType: "Property", ValueType: "xs:date", Value: new("2024-01-01")},
		{IDShort: "LegacyTime", ModelType: "Property", ValueType: "xs:time", Value: new("08:00:00.5")},
	}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)
	seedLegacyTemporalRow(t, id, "LegacyDateTime", "property_element", goqu.Record{"value_text": nil, "value_datetime": "2024-01-01T10:00:00+02:00"})
	seedLegacyTemporalRow(t, id, "LegacyDate", "property_element", goqu.Record{"value_text": nil, "value_date": "2024-01-01"})
	seedLegacyTemporalRow(t, id, "LegacyTime", "property_element", goqu.Record{"value_text": nil, "value_time": "08:00:00.5"})

	cases := []temporalQueryCase{
		{"zone-less dateTime is UTC", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.Local#value"}},{"$dateTimeVal":"2024-01-01T08:00:00Z"}]}`, true},
		{"zone-less date is UTC midnight", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.LocalDate#value"}},{"$dateTimeVal":"2024-01-01T00:00:00Z"}]}`, true},
		{"day of month is extracted in UTC", `{"$eq":[{"$dayOfMonth":{"$dateTimeCast":{"$field":"$sme.LateUTC#value"}}},{"$numVal":1}]}`, true},
		{"year of a field", `{"$eq":[{"$year":{"$dateTimeCast":{"$field":"$sme.LateUTC#value"}}},{"$numVal":2024}]}`, true},
		{"offset year is UTC", `{"$eq":[{"$year":{"$dateTimeCast":{"$field":"$sme.EarlyOffset#value"}}},{"$numVal":2023}]}`, true},
		{"field and literal years agree", `{"$eq":[{"$year":{"$dateTimeCast":{"$field":"$sme.EarlyOffset#value"}}},{"$year":{"$dateTimeVal":"2024-01-01T00:30:00+02:00"}}]}`, true},
		{"field and literal weekdays agree", `{"$eq":[{"$dayOfWeek":{"$dateTimeCast":{"$field":"$sme.EarlyOffset#value"}}},{"$dayOfWeek":{"$dateTimeVal":"2024-01-01T00:30:00+02:00"}}]}`, true},
		{"other day of month", `{"$eq":[{"$dayOfMonth":{"$dateTimeCast":{"$field":"$sme.LateUTC#value"}}},{"$numVal":2}]}`, false},
		{"time drops its offset", `{"$eq":[{"$timeCast":{"$field":"$sme.OffsetTime#value"}},{"$timeVal":"10:00:00Z"}]}`, true},
		{"unsupported year never matches", `{"$lt":[{"$dateTimeCast":{"$field":"$sme.EarlyYear#value"}},{"$dateTimeVal":"2024-01-01T00:00:00Z"}]}`, false},
		{"end of day text", `{"$eq":[{"$field":"$sme.EndOfDay#value"},{"$strVal":"24:00:00"}]}`, true},
		{"legacy dateTime text matches the read value", `{"$eq":[{"$field":"$sme.LegacyDateTime#value"},{"$strVal":"2024-01-01T08:00:00+00:00"}]}`, true},
		{"legacy dateTime instant", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.LegacyDateTime#value"}},{"$dateTimeVal":"2024-01-01T08:00:00Z"}]}`, true},
		{"legacy date text", `{"$eq":[{"$field":"$sme.LegacyDate#value"},{"$strVal":"2024-01-01"}]}`, true},
		{"legacy date is UTC midnight", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.LegacyDate#value"}},{"$dateTimeVal":"2024-01-01T00:00:00Z"}]}`, true},
		{"legacy time text", `{"$eq":[{"$field":"$sme.LegacyTime#value"},{"$strVal":"08:00:00.5"}]}`, true},
		{"legacy valueType is declared", `{"$eq":[{"$field":"$sme.LegacyTime#valueType"},{"$strVal":"xs:time"}]}`, true},
	}
	for _, reader := range requireTemporalRepositoriesInBothTimeZones(t) {
		t.Run(reader, func(t *testing.T) {
			requireTemporalElements(t, temporalSubmodelEndpoint(reader, id), elements)
			requireTemporalQueryCases(t, reader, id, cases)
		})
	}
}

func TestDeclaredValueTypeIsQueryable(t *testing.T) {
	const id = "urn:basyx:integration:declared-value-types"
	elements := []temporalElement{
		{IDShort: "Count", ModelType: "Property", ValueType: "xs:int", Value: new("42")},
		{IDShort: "Flag", ModelType: "Property", ValueType: "xs:boolean"},
		{IDShort: "Period", ModelType: "Property", ValueType: "xs:duration", Value: new("P1D")},
	}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)
	requireTemporalQueryCases(t, submodelRepositoryBaseURL, id, []temporalQueryCase{
		{"integer", `{"$eq":[{"$field":"$sme.Count#valueType"},{"$strVal":"xs:int"}]}`, true},
		{"integer is not double", `{"$eq":[{"$field":"$sme.Count#valueType"},{"$strVal":"xs:double"}]}`, false},
		{"boolean without value", `{"$eq":[{"$field":"$sme.Flag#valueType"},{"$strVal":"xs:boolean"}]}`, true},
		{"duration", `{"$eq":[{"$field":"$sme.Period#valueType"},{"$strVal":"xs:duration"}]}`, true},
	})
}

func TestLegacyTemporalRowsAreReplacedByWholeSubmodelPut(t *testing.T) {
	const id = "urn:basyx:integration:legacy-temporal-put"
	elements := []temporalElement{
		{IDShort: "Time", ModelType: "Property", ValueType: "xs:time", Value: new("10:00:00")},
		{IDShort: "Date", ModelType: "Property", ValueType: "xs:date", Value: new("2024-01-01")},
		{IDShort: "Dates", ModelType: "Range", ValueType: "xs:date", Min: new("2024-01-01"), Max: new("2024-01-02")},
	}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)
	seedLegacyTemporalRow(t, id, "Time", "property_element", goqu.Record{"value_text": nil, "value_time": "10:00:00"})
	seedLegacyTemporalRow(t, id, "Date", "property_element", goqu.Record{"value_text": nil, "value_date": "2024-01-01"})
	seedLegacyTemporalRow(t, id, "Dates", "range_element", goqu.Record{
		"min_text": nil, "min_date": "2024-01-01", "max_text": nil, "max_date": "2024-01-02",
	})
	endpoint := temporalSubmodelEndpoint(submodelRepositoryBaseURL, id)
	requireTemporalElements(t, endpoint, elements)

	elements[0].Value = new("10:00:00+02:00")
	elements[1].Value = new("2024-01-01+02:00")
	elements[2].Min = new("2024-01-01Z")
	requireValueOnlyStatus(t, http.MethodPut, endpoint, temporalSubmodel(id, elements), http.StatusNoContent)
	requireTemporalElements(t, endpoint, elements)

	requireValueOnlyStatus(t, http.MethodPatch, endpoint+"/$value", json.RawMessage(`{"Time":null,"Date":null,"Dates":{"min":null,"max":null}}`), http.StatusNoContent)
	elements[0].Value, elements[1].Value, elements[2].Min, elements[2].Max = nil, nil, nil, nil
	requireTemporalElements(t, endpoint, elements)
}

func TestLegacyTemporalRowFollowsMetadataValueTypeChange(t *testing.T) {
	const id = "urn:basyx:integration:legacy-temporal-metadata"
	elements := []temporalElement{
		{IDShort: "Timestamp", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-01-01T08:00:00+00:00")},
	}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)
	seedLegacyTemporalRow(t, id, "Timestamp", "property_element", goqu.Record{"value_text": nil, "value_datetime": "2024-01-01T08:00:00Z"})
	endpoint := temporalSubmodelEndpoint(submodelRepositoryBaseURL, id)

	patchMetadata(t, endpoint+"/submodel-elements/Timestamp", map[string]any{"modelType": "Property", "valueType": "xs:string"}, http.StatusNoContent)
	elements[0].ValueType = "xs:string"
	requireTemporalElements(t, endpoint, elements)
	requireTemporalQueryCases(t, submodelRepositoryBaseURL, id, []temporalQueryCase{
		{"declared string", `{"$eq":[{"$field":"$sme.Timestamp#valueType"},{"$strVal":"xs:string"}]}`, true},
	})

	requireValueOnlyStatus(t, http.MethodPatch, endpoint+"/submodel-elements/Timestamp/$value", json.RawMessage(`null`), http.StatusNoContent)
	elements[0].Value = nil
	requireTemporalElements(t, endpoint, elements)
}

func TestRangePutAcceptsSingleBound(t *testing.T) {
	const id = "urn:basyx:integration:range-put-single-bound"
	elements := []temporalElement{
		{IDShort: "Period", ModelType: "Range", ValueType: "xs:dateTime", Min: new("2024-01-01T08:00:00Z"), Max: new("2024-01-02T08:00:00Z")},
	}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)
	endpoint := temporalSubmodelEndpoint(submodelRepositoryBaseURL, id)

	elements[0].Max = nil
	requireValueOnlyStatus(t, http.MethodPut, endpoint+"/submodel-elements/Period", elements[0], http.StatusNoContent)
	requireTemporalElements(t, endpoint, elements)
}

func requireTemporalQueryCases(t *testing.T, baseURL, submodelID string, cases []temporalQueryCase) {
	t.Helper()
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			payload := map[string]any{"$condition": map[string]any{"$and": []any{
				map[string]any{"$eq": []any{map[string]string{"$field": "$sm#id"}, map[string]string{"$strVal": submodelID}}},
				json.RawMessage(test.condition),
			}}}
			body := requireValueOnlyStatus(t, http.MethodPost, baseURL+"/query/submodels", payload, http.StatusOK)
			var page struct{ Result []struct{ ID string } }
			require.NoError(t, json.Unmarshal(body, &page))
			if !test.match {
				require.Empty(t, page.Result, "%s", body)
				return
			}
			require.Len(t, page.Result, 1, "%s", body)
			require.Equal(t, submodelID, page.Result[0].ID)
		})
	}
}
