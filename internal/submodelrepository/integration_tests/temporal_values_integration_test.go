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
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

type temporalElement struct {
	IDShort   string  `json:"idShort"`
	ModelType string  `json:"modelType"`
	ValueType string  `json:"valueType"`
	Value     *string `json:"value,omitempty"`
	Min       *string `json:"min,omitempty"`
	Max       *string `json:"max,omitempty"`
}

func TestTemporalValuesPreserveLexicalFormsAcrossDatabaseTimeZones(t *testing.T) {
	elements := []temporalElement{
		{IDShort: "Time", ModelType: "Property", ValueType: "xs:time", Value: new("10:00:00.123456789+02:00")},
		{IDShort: "Date", ModelType: "Property", ValueType: "xs:date", Value: new("2024-01-01+02:00")},
		{IDShort: "LocalDateTime", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-01-01T10:00:00")},
		{IDShort: "ZonedDateTime", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-01-01T10:00:00.1234561+02:00")},
		{IDShort: "Times", ModelType: "Range", ValueType: "xs:time", Min: new("10:00:00Z"), Max: new("11:00:00-02:00")},
		{IDShort: "Dates", ModelType: "Range", ValueType: "xs:date", Min: new("2024-01-01Z"), Max: new("2024-01-02")},
		{IDShort: "DateTimes", ModelType: "Range", ValueType: "xs:dateTime", Min: new("2024-01-01T10:00:00Z"), Max: new("2024-01-02T10:00:00")},
	}
	readers := requireTemporalRepositoriesInBothTimeZones(t)
	for index, writer := range readers {
		t.Run(writer, func(t *testing.T) {
			id := fmt.Sprintf("urn:basyx:integration:temporal-values:%d", index)
			postTemporalSubmodel(t, writer, id, elements)
			for _, reader := range readers {
				requireTemporalElements(t, temporalSubmodelEndpoint(reader, id), elements)
			}
		})
	}
}

func TestTemporalValuesSurviveReplacementAndValueOnlyUpdates(t *testing.T) {
	const id = "urn:basyx:integration:temporal-replacement"
	elements := []temporalElement{
		{IDShort: "Timestamp", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-01-01T10:00:00.1234561Z")},
		{IDShort: "Period", ModelType: "Range", ValueType: "xs:dateTime", Min: new("2024-01-01T10:00:00Z"), Max: new("2024-01-02T10:00:00")},
	}
	postTemporalSubmodel(t, temporalSecondaryRepositoryURL(), id, elements)
	primaryEndpoint := temporalSubmodelEndpoint(submodelRepositoryBaseURL, id)
	secondaryEndpoint := temporalSubmodelEndpoint(temporalSecondaryRepositoryURL(), id)

	elements[0].Value = new("2024-01-01T12:00:00.1234562+02:00")
	elements[1].Min = new("2024-01-01T12:00:00+02:00")
	requireValueOnlyStatus(t, http.MethodPut, primaryEndpoint, temporalSubmodel(id, elements), http.StatusNoContent)
	requireTemporalElements(t, secondaryEndpoint, elements)

	elements[0].Value = new("2024-01-01T10:00:00.123456789")
	requireValueOnlyStatus(t, http.MethodPatch, secondaryEndpoint+"/submodel-elements/Timestamp/$value", elements[0].Value, http.StatusNoContent)
	elements[1].Max = new("2024-01-02T10:00:00.987654321Z")
	requireValueOnlyStatus(t, http.MethodPatch, secondaryEndpoint+"/submodel-elements/Period/$value", map[string]any{"max": elements[1].Max}, http.StatusNoContent)
	requireTemporalElements(t, primaryEndpoint, elements)

	elements[1].Min = new("2024-01-01T09:00:00-01:00")
	requireValueOnlyStatus(t, http.MethodPut, primaryEndpoint+"/submodel-elements/Period", elements[1], http.StatusNoContent)
	requireTemporalElements(t, secondaryEndpoint, elements)
}

func TestCanonicalTemporalTextRemainsReadableAndCanBeUpdated(t *testing.T) {
	const id = "urn:basyx:integration:canonical-temporal-values"
	elements := []temporalElement{
		{IDShort: "Timestamp", ModelType: "Property", ValueType: "xs:dateTime", Value: new("-0001-12-31T22:30:00.123+00:00")},
		{IDShort: "Period", ModelType: "Range", ValueType: "xs:dateTime", Min: new("2024-01-01T08:00:00+00:00"), Max: new("2024-01-02T08:00:00+00:00")},
	}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)
	primaryEndpoint := temporalSubmodelEndpoint(submodelRepositoryBaseURL, id)
	secondaryEndpoint := temporalSubmodelEndpoint(temporalSecondaryRepositoryURL(), id)
	requireTemporalElements(t, primaryEndpoint, elements)
	requireTemporalElements(t, secondaryEndpoint, elements)

	elements[0].Value = new("2024-01-01T10:00:00+02:00")
	requireValueOnlyStatus(t, http.MethodPut, secondaryEndpoint+"/submodel-elements/Timestamp", elements[0], http.StatusNoContent)
	elements[1].Min = new("2024-01-01T10:00:00")
	requireValueOnlyStatus(t, http.MethodPatch, secondaryEndpoint+"/submodel-elements/Period/$value", map[string]any{"min": elements[1].Min}, http.StatusNoContent)
	requireTemporalElements(t, primaryEndpoint, elements)
	requireStoredTemporalColumns(t, id, "Timestamp", "property_element", map[string]any{"value_text": *elements[0].Value})
	requireStoredTemporalColumns(t, id, "Period", "range_element", map[string]any{"min_text": *elements[1].Min, "max_text": *elements[1].Max})

	requireValueOnlyStatus(t, http.MethodPatch, secondaryEndpoint+"/$value", json.RawMessage(`{"Timestamp":null,"Period":{"min":null,"max":null}}`), http.StatusNoContent)
	elements[0].Value, elements[1].Min, elements[1].Max = nil, nil, nil
	requireTemporalElements(t, primaryEndpoint, elements)
}

func TestRangePatchPreservesOmittedTemporalBounds(t *testing.T) {
	const id = "urn:basyx:integration:temporal-range-patch"
	elements := []temporalElement{
		{IDShort: "Period", ModelType: "Range", ValueType: "xs:dateTime", Min: new("2024-01-01T08:00:00+00:00"), Max: new("2024-01-02T08:00:00+00:00")},
	}
	baseURL := temporalSecondaryRepositoryURL()
	postTemporalSubmodel(t, baseURL, id, elements)
	before := testenv.ReadSubmodelElementRow(t, submodelRepositoryIntegrationTestDSN, id, "Period", "range_element", "min_text", "max_text")

	endpoint := temporalSubmodelEndpoint(baseURL, id)
	rangeURL := endpoint + "/submodel-elements/Period"
	requireValueOnlyStatus(t, http.MethodPatch, rangeURL, map[string]any{
		"modelType": "Range", "valueType": "xs:dateTime",
		"description": []any{map[string]any{"language": "en", "text": "Updated description"}},
	}, http.StatusNoContent)
	requireStoredTemporalColumns(t, id, "Period", "range_element", before)
	var updatedRange struct{ Description json.RawMessage }
	require.NoError(t, json.Unmarshal(requireValueOnlyStatus(t, http.MethodGet, rangeURL, nil, http.StatusOK), &updatedRange))
	require.JSONEq(t, `[{"language":"en","text":"Updated description"}]`, string(updatedRange.Description))
	requireTemporalElements(t, endpoint, elements)

	elements[0].Min = new("2024-01-01T10:30:00.123456789+02:00")
	requireValueOnlyStatus(t, http.MethodPatch, rangeURL, map[string]any{
		"modelType": "Range", "valueType": "xs:dateTime", "min": elements[0].Min,
	}, http.StatusNoContent)
	requireStoredTemporalColumns(t, id, "Period", "range_element", map[string]any{
		"min_text": *elements[0].Min,
		"max_text": before["max_text"],
	})
	requireTemporalElements(t, endpoint, elements)

	elements[0].ValueType = "xs:string"
	requireValueOnlyStatus(t, http.MethodPatch, rangeURL, map[string]any{
		"modelType": "Range", "valueType": elements[0].ValueType,
	}, http.StatusNoContent)
	requireStoredTemporalColumns(t, id, "Period", "range_element", map[string]any{
		"min_text": *elements[0].Min,
		"max_text": *elements[0].Max,
	})
	requireTemporalElements(t, endpoint, elements)
}

func TestRangePatchConflictsWithConcurrentValueTypeChange(t *testing.T) {
	const id = "urn:basyx:integration:concurrent-range-type-patch"
	const idShort = "Period"
	elements := []temporalElement{
		{IDShort: idShort, ModelType: "Range", ValueType: "xs:dateTime", Min: new("2024-01-01T08:00:00Z"), Max: new("2024-01-02T08:00:00Z")},
	}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)
	endpoint := temporalSubmodelEndpoint(submodelRepositoryBaseURL, id)

	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	dialect := goqu.Dialect(common.Dialect)
	target := dialect.From("submodel_element").Select(goqu.I("submodel_element.id")).
		Join(goqu.T("submodel"), goqu.On(goqu.I("submodel.id").Eq(goqu.I("submodel_element.submodel_id")))).
		Where(goqu.I("submodel.submodel_identifier").Eq(id), goqu.I("submodel_element.idshort_path").Eq(idShort))
	update, updateArgs, err := dialect.Update("range_element").Set(goqu.Record{
		"value_type": types.DataTypeDefXSDString, "min_text": "new lower bound", "max_text": "new upper bound",
	}).Where(goqu.C("id").Eq(target)).Returning(goqu.Func("pg_backend_pid")).Prepared(true).ToSQL()
	require.NoError(t, err)
	var blockerPID int
	require.NoError(t, tx.QueryRowContext(t.Context(), update, updateArgs...).Scan(&blockerPID))

	type patchResult struct {
		status int
		body   []byte
		err    error
	}
	patchDone := make(chan patchResult, 1)
	patch := map[string]any{
		"modelType": "Range", "valueType": "xs:dateTime", "min": "2024-01-01T09:00:00Z",
	}
	go func() {
		status, body, err := requestJSON(http.MethodPatch, endpoint+"/submodel-elements/"+idShort, patch)
		patchDone <- patchResult{status: status, body: body, err: err}
	}()

	waitingQuery, waitArgs, err := dialect.From("pg_stat_activity").Select(goqu.COUNT(goqu.Star())).
		Where(goqu.L("? = ANY(pg_blocking_pids(?))", blockerPID, goqu.C("pid"))).Prepared(true).ToSQL()
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var count int
		return db.QueryRowContext(t.Context(), waitingQuery, waitArgs...).Scan(&count) == nil && count > 0
	}, 5*time.Second, 20*time.Millisecond, "the PATCH must reach the Range update before the concurrent type change commits")
	require.NoError(t, tx.Commit())

	result := <-patchDone
	require.NoError(t, result.err)
	require.Equal(t, http.StatusConflict, result.status, string(result.body))
	elements[0].ValueType = "xs:string"
	elements[0].Min = new("new lower bound")
	elements[0].Max = new("new upper bound")
	requireTemporalElements(t, endpoint, elements)
}

func TestRangePatchCanChangeTypeWithAnAbsentBound(t *testing.T) {
	const id = "urn:basyx:integration:open-range-type-patch"
	elements := []temporalElement{
		{IDShort: "Period", ModelType: "Range", ValueType: "xs:dateTime", Min: new("2024-01-01T08:00:00+00:00")},
	}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)

	endpoint := temporalSubmodelEndpoint(submodelRepositoryBaseURL, id)
	elements[0].ValueType = "xs:string"
	requireValueOnlyStatus(t, http.MethodPatch, endpoint+"/submodel-elements/Period", map[string]any{
		"modelType": "Range", "valueType": elements[0].ValueType,
	}, http.StatusNoContent)
	requireStoredTemporalColumns(t, id, "Period", "range_element", map[string]any{
		"min_text": *elements[0].Min,
		"max_text": nil,
	})
	requireTemporalElements(t, endpoint, elements)
}

func requireTemporalRepositoriesInBothTimeZones(t *testing.T) []string {
	t.Helper()
	secondaryURL := temporalSecondaryRepositoryURL()
	if secondaryURL == submodelRepositoryBaseURL {
		t.Skip("BASYX_IT_STRICT_API_PORT is not set; the Europe/Berlin repository instance is unavailable")
	}
	return []string{submodelRepositoryBaseURL, secondaryURL}
}

func temporalSecondaryRepositoryURL() string {
	if os.Getenv("BASYX_EXTERNAL_COMPOSE") == "1" && os.Getenv("BASYX_IT_STRICT_API_PORT") == "" {
		return submodelRepositoryBaseURL
	}
	return submodelRepositoryStrictBaseURL
}

func postTemporalSubmodel(t *testing.T, baseURL, id string, elements []temporalElement) {
	t.Helper()
	requireValueOnlyStatus(t, http.MethodPost, baseURL+"/submodels", temporalSubmodel(id, elements), http.StatusCreated)
	t.Cleanup(func() {
		requireValueOnlyStatus(t, http.MethodDelete, temporalSubmodelEndpoint(baseURL, id), nil, http.StatusNoContent)
	})
}

func temporalSubmodel(id string, elements []temporalElement) map[string]any {
	return map[string]any{"id": id, "idShort": "TemporalValues", "modelType": "Submodel", "submodelElements": elements}
}

func temporalSubmodelEndpoint(baseURL, id string) string {
	return baseURL + "/submodels/" + common.EncodeString(id)
}

func requireTemporalElements(t *testing.T, endpoint string, expected []temporalElement) {
	t.Helper()
	var submodel struct{ SubmodelElements []temporalElement }
	require.NoError(t, json.Unmarshal(requireValueOnlyStatus(t, http.MethodGet, endpoint, nil, http.StatusOK), &submodel))
	require.ElementsMatch(t, expected, submodel.SubmodelElements)

	var page struct{ Result []temporalElement }
	require.NoError(t, json.Unmarshal(requireValueOnlyStatus(t, http.MethodGet, endpoint+"/submodel-elements?limit=100", nil, http.StatusOK), &page))
	require.ElementsMatch(t, expected, page.Result)

	for _, element := range expected {
		want, err := json.Marshal(temporalValueOnly(element))
		require.NoError(t, err)
		body := requireValueOnlyStatus(t, http.MethodGet, endpoint+"/submodel-elements/"+element.IDShort+"/$value", nil, http.StatusOK)
		require.JSONEq(t, string(want), string(body), "%s", element.IDShort)
	}
}

func temporalValueOnly(element temporalElement) any {
	if element.ModelType != "Range" {
		return element.Value
	}
	bounds := map[string]*string{}
	if element.Min != nil {
		bounds["min"] = element.Min
	}
	if element.Max != nil {
		bounds["max"] = element.Max
	}
	return bounds
}

func requireStoredTemporalColumns(t *testing.T, submodelID, idShort, table string, want map[string]any) {
	t.Helper()
	columns := make([]string, 0, len(want))
	for column := range want {
		columns = append(columns, column)
	}
	stored := testenv.ReadSubmodelElementRow(t, submodelRepositoryIntegrationTestDSN, submodelID, idShort, table, columns...)
	require.Equal(t, want, stored)
}
