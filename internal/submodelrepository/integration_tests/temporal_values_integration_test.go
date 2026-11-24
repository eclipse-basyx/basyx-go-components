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

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
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
	primaryURL := submodelRepositoryBaseURL
	secondaryURL := temporalSecondaryRepositoryURL()
	readers := []string{primaryURL}
	if secondaryURL != primaryURL {
		readers = append(readers, secondaryURL)
	}
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

func TestLegacyTemporalRowsRemainReadableAndCanBeUpdated(t *testing.T) {
	const id = "urn:basyx:integration:legacy-temporal-values"
	elements := []temporalElement{
		{IDShort: "Timestamp", ModelType: "Property", ValueType: "xs:dateTime", Value: new("2024-01-01T08:00:00+00:00")},
		{IDShort: "Period", ModelType: "Range", ValueType: "xs:dateTime", Min: new("2024-01-01T08:00:00+00:00"), Max: new("2024-01-02T08:00:00+00:00")},
	}
	postTemporalSubmodel(t, submodelRepositoryBaseURL, id, elements)
	seedLegacyTemporalRow(t, id, "Timestamp", "property_element", goqu.Record{
		"value_text": nil, "value_datetime": "2024-01-01T10:00:00+02:00",
	})
	seedLegacyTemporalRow(t, id, "Period", "range_element", goqu.Record{
		"min_text": nil, "min_datetime": "2024-01-01T10:00:00+02:00",
		"max_text": nil, "max_datetime": "2024-01-02T10:00:00+02:00",
	})
	primaryEndpoint := temporalSubmodelEndpoint(submodelRepositoryBaseURL, id)
	secondaryEndpoint := temporalSubmodelEndpoint(temporalSecondaryRepositoryURL(), id)
	requireTemporalElements(t, primaryEndpoint, elements)
	requireTemporalElements(t, secondaryEndpoint, elements)

	elements[0].Value = new("2024-01-01T10:00:00+02:00")
	requireValueOnlyStatus(t, http.MethodPut, secondaryEndpoint+"/submodel-elements/Timestamp", elements[0], http.StatusNoContent)
	elements[1].Min = new("2024-01-01T10:00:00")
	requireValueOnlyStatus(t, http.MethodPatch, secondaryEndpoint+"/submodel-elements/Period/$value", map[string]any{"min": elements[1].Min}, http.StatusNoContent)
	requireTemporalElements(t, primaryEndpoint, elements)

	requireValueOnlyStatus(t, http.MethodPatch, secondaryEndpoint+"/$value", json.RawMessage(`{"Timestamp":null,"Period":{"min":null,"max":null}}`), http.StatusNoContent)
	for _, path := range []string{"Timestamp", "Period"} {
		body := requireValueOnlyStatus(t, http.MethodGet, primaryEndpoint+"/submodel-elements/"+path, nil, http.StatusOK)
		var element temporalElement
		require.NoError(t, json.Unmarshal(body, &element))
		require.Nil(t, element.Value)
		require.Nil(t, element.Min)
		require.Nil(t, element.Max)
	}
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
		value := any(element.Value)
		if element.ModelType == "Range" {
			value = map[string]any{"min": element.Min, "max": element.Max}
		}
		want, err := json.Marshal(value)
		require.NoError(t, err)
		body := requireValueOnlyStatus(t, http.MethodGet, endpoint+"/submodel-elements/"+element.IDShort+"/$value", nil, http.StatusOK)
		require.JSONEq(t, string(want), string(body), "%s", element.IDShort)
	}
}

func seedLegacyTemporalRow(t *testing.T, submodelID, idShort, table string, record goqu.Record) {
	t.Helper()
	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	dialect := goqu.Dialect(common.Dialect)
	elementID := dialect.From("submodel_element").Select(goqu.I("submodel_element.id")).
		Join(goqu.T("submodel"), goqu.On(goqu.I("submodel.id").Eq(goqu.I("submodel_element.submodel_id")))).
		Where(goqu.I("submodel.submodel_identifier").Eq(submodelID), goqu.I("submodel_element.id_short").Eq(idShort))
	query, args, err := dialect.Update(table).Set(record).Where(goqu.C("id").Eq(elementID)).ToSQL()
	require.NoError(t, err)
	result, err := db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
	updated, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
}
