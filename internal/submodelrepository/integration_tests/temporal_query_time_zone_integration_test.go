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
		{IDShort: "BeforeMidnight", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-12-31T23:59:59.9999999Z")},
		{IDShort: "ShortTimestamp", ModelType: "Property", ValueType: "xs:string", Value: new("2024-01-01 08:00")},
		{IDShort: "FirstCenturyBC", ModelType: "Property", ValueType: "xs:dateTime", Value: new("0001-01-01T00:30:00+02:00")},
		{IDShort: "HalfSecond", ModelType: "Property", ValueType: "xs:time", Value: new("08:00:00.5")},
		{IDShort: "Padded", ModelType: "Property", ValueType: "xs:string", Value: new("\t2024-01-01T08:00:00Z\n")},
		{IDShort: "FarFuture", ModelType: "Property", ValueType: "xs:dateTime", Value: new("294277-01-01T00:00:00+14:00")},
		{IDShort: "LocalDate", ModelType: "Property", ValueType: "xs:date", Value: new("2024-01-01")},
		{IDShort: "WestDate", ModelType: "Property", ValueType: "xs:date", Value: new("2024-01-01-05:00")},
		{IDShort: "OffsetTime", ModelType: "Property", ValueType: "xs:time", Value: new("10:00:00+02:00")},
		{IDShort: "EarlyYear", ModelType: "Property", ValueType: "xs:dateTime", Value: new("-0001-01-01T00:00:00Z")},
		{IDShort: "EndOfDay", ModelType: "Property", ValueType: "xs:time", Value: new("24:00:00")},
		{IDShort: "BackfilledDateTime", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-01-01T08:00:00+00:00")},
		{IDShort: "BackfilledDate", ModelType: "Property", ValueType: "xs:date", Value: new("2024-01-01")},
		{IDShort: "BackfilledTime", ModelType: "Property", ValueType: "xs:time", Value: new("08:00:00.5")},
	}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)

	cases := []temporalQueryCase{
		{"zone-less dateTime is UTC", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.Local#value"}},{"$dateTimeVal":"2024-01-01T08:00:00Z"}]}`, true},
		{"zone-less field and string literal agree", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.Local#value"}},{"$dateTimeCast":{"$strVal":"2024-01-01T08:00:00"}}]}`, true},
		{"dateTime literal without T and seconds", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.Local#value"}},{"$dateTimeVal":"2024-01-0108:00"}]}`, true},
		{"year of a zone-less string literal", `{"$eq":[{"$year":{"$dateTimeCast":{"$strVal":"2024-01-01T08:00:00"}}},{"$numVal":2024}]}`, true},
		{"text that is not an xs:dateTime never matches", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.ShortTimestamp#value"}},{"$dateTimeVal":"2024-01-01T08:00:00Z"}]}`, false},
		{"an xs:date is not an xs:dateTime", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.LocalDate#value"}},{"$dateTimeVal":"2024-01-01T00:00:00Z"}]}`, false},
		{"an xs:date with offset is not an xs:dateTime", `{"$le":[{"$dateTimeCast":{"$field":"$sme.WestDate#value"}},{"$dateTimeVal":"2025-01-01T00:00:00Z"}]}`, false},
		{"day of month of a field", `{"$eq":[{"$dayOfMonth":{"$dateTimeCast":{"$field":"$sme.LateUTC#value"}}},{"$numVal":1}]}`, true},
		{"years are taken in the value's own timezone", `{"$eq":[{"$year":{"$dateTimeCast":{"$field":"$sme.EarlyOffset#value"}}},{"$numVal":2024}]}`, true},
		{"weekdays are taken in the value's own timezone", `{"$eq":[{"$dayOfWeek":{"$dateTimeCast":{"$field":"$sme.EarlyOffset#value"}}},{"$numVal":1}]}`, true},
		{"field and literal years agree", `{"$eq":[{"$year":{"$dateTimeCast":{"$field":"$sme.EarlyOffset#value"}}},{"$year":{"$dateTimeVal":"2024-01-01T00:30:00+02:00"}}]}`, true},
		{"field and literal weekdays agree", `{"$eq":[{"$dayOfWeek":{"$dateTimeCast":{"$field":"$sme.EarlyOffset#value"}}},{"$dayOfWeek":{"$dateTimeVal":"2024-01-01T00:30:00+02:00"}}]}`, true},
		{"fractional seconds stay in the submitted year", `{"$eq":[{"$year":{"$dateTimeCast":{"$field":"$sme.BeforeMidnight#value"}}},{"$numVal":2024}]}`, true},
		{"field and literal calendar days agree before midnight", `{"$eq":[{"$dayOfMonth":{"$dateTimeCast":{"$field":"$sme.BeforeMidnight#value"}}},{"$dayOfMonth":{"$dateTimeVal":"2024-12-31T23:59:59.9999999Z"}}]}`, true},
		{"field equals a literal beyond microsecond precision", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.BeforeMidnight#value"}},{"$dateTimeVal":"2024-12-31T23:59:59.9999999Z"}]}`, true},
		{"sub-microsecond digits do not round into the next year", `{"$lt":[{"$dateTimeCast":{"$field":"$sme.BeforeMidnight#value"}},{"$dateTimeVal":"2025-01-01T00:00:00Z"}]}`, true},
		{"local year of an instant before year 1", `{"$eq":[{"$year":{"$dateTimeCast":{"$field":"$sme.FirstCenturyBC#value"}}},{"$numVal":1}]}`, true},
		{"time comparisons keep fractional seconds", `{"$gt":[{"$timeCast":{"$field":"$sme.HalfSecond#value"}},{"$timeVal":"08:00"}]}`, true},
		{"XSD whitespace around a stored value is ignored", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.Padded#value"}},{"$dateTimeVal":"2024-01-01T08:00:00Z"}]}`, true},
		{"instant beyond the local timestamp range", `{"$gt":[{"$dateTimeCast":{"$field":"$sme.FarFuture#value"}},{"$dateTimeCast":{"$strVal":"294276-12-31T00:00:00Z"}}]}`, true},
		{"date part beyond the local timestamp range never matches", `{"$eq":[{"$year":{"$dateTimeCast":{"$field":"$sme.FarFuture#value"}}},{"$numVal":294277}]}`, false},
		{"string cast keeps the offset", `{"$eq":[{"$strCast":{"$dateTimeCast":{"$field":"$sme.EarlyOffset#value"}}},{"$strVal":"2024-01-01T00:30:00+02:00"}]}`, true},
		{"string cast keeps a value zone-less", `{"$eq":[{"$strCast":{"$dateTimeCast":{"$field":"$sme.Local#value"}}},{"$strVal":"2024-01-01T08:00:00"}]}`, true},
		{"string cast of a time", `{"$eq":[{"$strCast":{"$timeCast":{"$field":"$sme.OffsetTime#value"}}},{"$strVal":"10:00:00+02:00"}]}`, true},
		{"other day of month", `{"$eq":[{"$dayOfMonth":{"$dateTimeCast":{"$field":"$sme.LateUTC#value"}}},{"$numVal":2}]}`, false},
		{"times with an offset compare as instants", `{"$eq":[{"$timeCast":{"$field":"$sme.OffsetTime#value"}},{"$timeVal":"08:00:00Z"}]}`, true},
		{"time offsets are not dropped", `{"$eq":[{"$timeCast":{"$field":"$sme.OffsetTime#value"}},{"$timeVal":"10:00:00Z"}]}`, false},
		{"time of a zone-less timestamp", `{"$eq":[{"$timeCast":{"$dateTimeCast":{"$field":"$sme.Local#value"}}},{"$timeVal":"08:00:00Z"}]}`, true},
		{"time of an offset timestamp keeps its timezone", `{"$eq":[{"$timeCast":{"$dateTimeCast":{"$field":"$sme.EarlyOffset#value"}}},{"$timeVal":"00:30:00+02:00"}]}`, true},
		{"time of an offset timestamp is not its UTC clock time", `{"$eq":[{"$timeCast":{"$dateTimeCast":{"$field":"$sme.EarlyOffset#value"}}},{"$timeVal":"22:30:00Z"}]}`, false},
		{"unsupported year never matches", `{"$lt":[{"$dateTimeCast":{"$field":"$sme.EarlyYear#value"}},{"$dateTimeVal":"2024-01-01T00:00:00Z"}]}`, false},
		{"end of day text", `{"$eq":[{"$field":"$sme.EndOfDay#value"},{"$strVal":"24:00:00"}]}`, true},
		{"24:00:00 is the start of the day", `{"$eq":[{"$timeCast":{"$field":"$sme.EndOfDay#value"}},{"$timeVal":"00:00"}]}`, true},
		{"backfilled dateTime text matches the read value", `{"$eq":[{"$field":"$sme.BackfilledDateTime#value"},{"$strVal":"2024-01-01T08:00:00+00:00"}]}`, true},
		{"backfilled dateTime instant", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.BackfilledDateTime#value"}},{"$dateTimeVal":"2024-01-01T08:00:00Z"}]}`, true},
		{"backfilled date text", `{"$eq":[{"$field":"$sme.BackfilledDate#value"},{"$strVal":"2024-01-01"}]}`, true},
		{"backfilled xs:date is not an xs:dateTime", `{"$eq":[{"$dateTimeCast":{"$field":"$sme.BackfilledDate#value"}},{"$dateTimeVal":"2024-01-01T00:00:00Z"}]}`, false},
		{"backfilled time text", `{"$eq":[{"$field":"$sme.BackfilledTime#value"},{"$strVal":"08:00:00.5"}]}`, true},
		{"backfilled time instant", `{"$eq":[{"$timeCast":{"$field":"$sme.BackfilledTime#value"}},{"$timeVal":"08:00:00.5Z"}]}`, true},
		{"backfilled valueType is declared", `{"$eq":[{"$field":"$sme.BackfilledTime#valueType"},{"$strVal":"xs:time"}]}`, true},
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

func TestWholeSubmodelPutReplacesTemporalValues(t *testing.T) {
	const id = "urn:basyx:integration:temporal-put"
	elements := []temporalElement{
		{IDShort: "Time", ModelType: "Property", ValueType: "xs:time", Value: new("10:00:00")},
		{IDShort: "Date", ModelType: "Property", ValueType: "xs:date", Value: new("2024-01-01")},
		{IDShort: "Dates", ModelType: "Range", ValueType: "xs:date", Min: new("2024-01-01"), Max: new("2024-01-02")},
	}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)
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

func TestTemporalValueFollowsMetadataValueTypeChange(t *testing.T) {
	const id = "urn:basyx:integration:temporal-metadata"
	elements := []temporalElement{
		{IDShort: "Timestamp", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-01-01T08:00:00+00:00")},
	}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)
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

func TestDeeplyNestedTemporalStringCastsStayCheap(t *testing.T) {
	const id = "urn:basyx:integration:nested-temporal-casts"
	elements := []temporalElement{{IDShort: "Stamp", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-01-01T00:30:00+02:00")}}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)

	operand := `{"$field":"$sme.Stamp#value"}`
	for range 24 {
		operand = `{"$dateTimeCast":{"$strCast":` + operand + `}}`
	}
	nested := `{"$eq":[` + operand + `,{"$dateTimeVal":"2023-12-31T22:30:00Z"}]}`
	for _, reader := range requireTemporalRepositoriesInBothTimeZones(t) {
		t.Run(reader, func(t *testing.T) {
			requireTemporalQueryCases(t, reader, id, []temporalQueryCase{{"24 nested string and date-time casts", nested, true}})
		})
	}
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
