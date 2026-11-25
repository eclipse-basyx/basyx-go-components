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

package migrationintegrationtests

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const temporalMigrationID = "urn:basyx:migration:temporal"

type temporalMigrationElement struct {
	idShort, table, valueType string
	stored                    goqu.Record
	want                      map[string]any
}

func temporalMigrationElements() []temporalMigrationElement {
	return []temporalMigrationElement{
		{"Clock", "property_element", "xs:time", goqu.Record{"value_time": "24:00:00"}, map[string]any{"value_text": "24:00:00"}},
		{"Date", "property_element", "xs:date", goqu.Record{"value_date": "2024-02-29"}, map[string]any{"value_text": "2024-02-29"}},
		{"Timestamp", "property_element", "xs:dateTime", goqu.Record{"value_datetime": "2024-01-01T10:00:00.123456+02:00"}, map[string]any{"value_text": "2024-01-01T08:00:00.123456+00:00"}},
		{"BCE", "property_element", "xs:dateTime", goqu.Record{"value_datetime": "0001-01-01T00:30:00.123+02:00"}, map[string]any{"value_text": "-0001-12-31T22:30:00.123+00:00"}},
		{"Exact", "property_element", "xs:dateTime", goqu.Record{"value_text": "2024-01-01T00:30:00.123456789+02:00"}, map[string]any{"value_text": "2024-01-01T00:30:00.123456789+02:00"}},
		{"AlreadyBackfilled", "property_element", "xs:date", goqu.Record{"value_text": "2024-02-29", "value_date": "2024-02-29"}, map[string]any{"value_text": "2024-02-29"}},
		{"EmptyText", "property_element", "xs:string", goqu.Record{"value_text": ""}, map[string]any{"value_text": ""}},
		{"Count", "property_element", "xs:int", goqu.Record{"value_num": 42}, map[string]any{"value_text": nil, "value_num": "42"}},
		{"Flag", "property_element", "xs:boolean", goqu.Record{"value_bool": true}, map[string]any{"value_text": nil, "value_bool": "true"}},
		{"NoValue", "property_element", "xs:dateTime", nil, map[string]any{"value_text": nil}},
		{"Times", "range_element", "xs:time", goqu.Record{"min_time": "08:00:00.5", "max_time": "24:00:00"}, map[string]any{"min_text": "08:00:00.5", "max_text": "24:00:00"}},
		{"Dates", "range_element", "xs:date", goqu.Record{"min_date": "2024-02-28", "max_date": "2024-02-29"}, map[string]any{"min_text": "2024-02-28", "max_text": "2024-02-29"}},
		{"Period", "range_element", "xs:dateTime", goqu.Record{"min_datetime": "2024-01-01T10:00:00+02:00", "max_datetime": "2024-01-02T10:00:00-02:00"}, map[string]any{"min_text": "2024-01-01T08:00:00+00:00", "max_text": "2024-01-02T12:00:00+00:00"}},
		{"Mixed", "range_element", "xs:dateTime", goqu.Record{"min_text": "2024-01-01T00:30:00.123456789", "max_datetime": "2024-01-02T10:00:00+02:00"}, map[string]any{"min_text": "2024-01-01T00:30:00.123456789", "max_text": "2024-01-02T08:00:00+00:00"}},
		{"Open", "range_element", "xs:date", goqu.Record{"max_date": "2024-02-29"}, map[string]any{"min_text": nil, "max_text": "2024-02-29"}},
		{"Numbers", "range_element", "xs:int", goqu.Record{"min_num": 3, "max_num": 7}, map[string]any{"min_text": nil, "max_text": nil, "min_num": "3", "max_num": "7"}},
	}
}

func seedLegacyTemporalMigrationFixture(t *testing.T) {
	t.Helper()
	fixtures := temporalMigrationElements()
	elements := make([]map[string]any, 0, len(fixtures))
	for _, fixture := range fixtures {
		modelType := "Property"
		if fixture.table == "range_element" {
			modelType = "Range"
		}
		elements = append(elements, map[string]any{"idShort": fixture.idShort, "modelType": modelType, "valueType": fixture.valueType})
	}
	body, err := json.Marshal(map[string]any{
		"id": temporalMigrationID, "idShort": "LegacyTemporal", "modelType": "Submodel", "submodelElements": elements,
	})
	require.NoError(t, err)
	response := testenv.DoHTTP(t, http.MethodPost, migrationBaseURL+"/submodels", body, nil)
	require.Equal(t, http.StatusCreated, response.Status, "%s", response.Body)
	for _, fixture := range fixtures {
		if len(fixture.stored) != 0 {
			testenv.OverwriteSubmodelElementRow(t, migrationDSN, temporalMigrationID, fixture.idShort, fixture.table, fixture.stored)
		}
	}
}

func assertTemporalMigrationRejectsConflicts(t *testing.T) {
	t.Helper()
	patch, err := os.ReadFile("../../../database/patches/1_2_4.sql")
	require.NoError(t, err)
	db := openMigrationDatabase(t)
	defer func() { require.NoError(t, db.Close()) }()
	dialect := goqu.Dialect(common.Dialect)

	for _, test := range []struct {
		table    string
		target   goqu.Ex
		conflict goqu.Record
	}{
		{"property_element", goqu.Ex{"value_date": "2024-02-29", "value_text": nil}, goqu.Record{"value_text": "2024-02-28"}},
		{"range_element", goqu.Ex{"max_date": "2024-02-29", "min_date": nil}, goqu.Record{"max_time": "08:00:00"}},
	} {
		t.Run(test.table+" rejects conflicting storage", func(t *testing.T) {
			tx, err := db.BeginTx(t.Context(), nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			query, args, err := dialect.Update(test.table).Set(test.conflict).Where(test.target).ToSQL()
			require.NoError(t, err)
			result, err := tx.ExecContext(t.Context(), query, args...)
			require.NoError(t, err)
			rows, err := result.RowsAffected()
			require.NoError(t, err)
			require.EqualValues(t, 1, rows)
			_, err = tx.ExecContext(t.Context(), string(patch))
			require.ErrorContains(t, err, "BASYXCFG-TEMPORAL-CONFLICT: "+test.table)
			require.NoError(t, tx.Rollback())
		})
	}
}

func assertTemporalMigration(t *testing.T) {
	t.Helper()
	db := openMigrationDatabase(t)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	query, args, err := goqu.Dialect(common.Dialect).From(goqu.I("information_schema.columns")).
		Select(goqu.COUNT("*")).
		Where(goqu.C("table_schema").Eq("public"), goqu.Or(
			goqu.Ex{"table_name": "property_element", "column_name": []string{"value_time", "value_date", "value_datetime"}},
			goqu.Ex{"table_name": "range_element", "column_name": []string{"min_time", "min_date", "min_datetime", "max_time", "max_date", "max_datetime"}},
		)).ToSQL()
	require.NoError(t, err)
	var remaining int
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&remaining))
	require.Zero(t, remaining, "all nine legacy temporal columns must be dropped")

	for _, fixture := range temporalMigrationElements() {
		columns := make([]string, 0, len(fixture.want))
		for column := range fixture.want {
			columns = append(columns, column)
		}
		require.Equal(t, fixture.want, testenv.ReadSubmodelElementRow(t, migrationDSN, temporalMigrationID, fixture.idShort, fixture.table, columns...), fixture.idShort)
	}

	endpoint := migrationBaseURL + "/submodels/" + common.EncodeString(temporalMigrationID)
	value := testenv.DoHTTP(t, http.MethodGet, endpoint+"/submodel-elements/Timestamp/$value", nil, nil)
	require.Equal(t, http.StatusOK, value.Status, "%s", value.Body)
	require.JSONEq(t, `"2024-01-01T08:00:00.123456+00:00"`, string(value.Body))
	page := testenv.DoHTTP(t, http.MethodGet, endpoint+"/submodel-elements?limit=100", nil, nil)
	require.Equal(t, http.StatusOK, page.Status, "%s", page.Body)
	var elements struct{ Result []struct{ IDShort string } }
	require.NoError(t, json.Unmarshal(page.Body, &elements))
	require.Len(t, elements.Result, len(temporalMigrationElements()))

	response := testenv.DoHTTP(t, http.MethodPost, migrationBaseURL+"/query/submodels", []byte(`{
        "$condition": {"$and": [
            {"$eq": [{"$field": "$sm#id"}, {"$strVal": "urn:basyx:migration:temporal"}]},
            {"$eq": [{"$field": "$sme.Timestamp#value"}, {"$strVal": "2024-01-01T08:00:00.123456+00:00"}]},
            {"$eq": [{"$dateTimeCast": {"$field": "$sme.Timestamp#value"}}, {"$dateTimeVal": "2024-01-01T08:00:00.123456Z"}]}
        ]}
    }`), nil)
	require.Equal(t, http.StatusOK, response.Status, "%s", response.Body)
	var matches struct{ Result []struct{ ID string } }
	require.NoError(t, json.Unmarshal(response.Body, &matches))
	require.Len(t, matches.Result, 1)
	require.Equal(t, temporalMigrationID, matches.Result[0].ID)
}
