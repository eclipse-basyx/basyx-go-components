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
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

const reconciliationScaleElementCount = 1200

type persistedReconciliationElement struct {
	id       int64
	position int
}

func BenchmarkPutSubmodelStagingChangeRatios(b *testing.B) {
	for _, percentage := range []int{0, 1, 25, 100} {
		b.Run(fmt.Sprintf("changed_%d_percent", percentage), func(b *testing.B) {
			submodelID := fmt.Sprintf("urn:basyx:benchmark:put-staging-%d-%d", percentage, time.Now().UnixNano())
			endpoint := submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(submodelID)
			status, body := sendReconciliationRequestWithoutFailure(
				http.MethodPost,
				submodelRepositoryBaseURL+"/submodels",
				reconciliationBenchmarkSubmodel(submodelID, percentage, 0),
			)
			if status != http.StatusCreated {
				b.Fatalf("initial create status=%d response=%s", status, string(body))
			}
			b.Cleanup(func() { _, _ = sendReconciliationRequestWithoutFailure(http.MethodDelete, endpoint, nil) })

			db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = db.Close() })
			walStart := reconciliationWALLSN(b, db)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := range b.N {
				status, body = sendReconciliationRequestWithoutFailure(
					http.MethodPut,
					endpoint,
					reconciliationBenchmarkSubmodel(submodelID, percentage, iteration+1),
				)
				if status != http.StatusNoContent {
					b.Fatalf("PUT status=%d response=%s", status, string(body))
				}
			}
			b.StopTimer()
			b.ReportMetric(reconciliationWALBytesSince(b, db, walStart)/float64(b.N), "wal-bytes/op")
			b.ReportMetric(float64(reconciliationScaleElementCount*percentage/100), "changed-rows/op")
		})
	}
}

func BenchmarkPutSubmodelDeleteReinsertBaseline(b *testing.B) {
	submodelID := fmt.Sprintf("urn:basyx:benchmark:put-delete-reinsert-%d", time.Now().UnixNano())
	endpoint := submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(submodelID)
	status, body := sendReconciliationRequestWithoutFailure(
		http.MethodPost,
		submodelRepositoryBaseURL+"/submodels",
		reconciliationBenchmarkSubmodel(submodelID, 100, 0),
	)
	if status != http.StatusCreated {
		b.Fatalf("initial create status=%d response=%s", status, string(body))
	}
	b.Cleanup(func() { _, _ = sendReconciliationRequestWithoutFailure(http.MethodDelete, endpoint, nil) })

	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	walStart := reconciliationWALLSN(b, db)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := range b.N {
		status, body = sendReconciliationRequestWithoutFailure(http.MethodDelete, endpoint, nil)
		if status != http.StatusNoContent {
			b.Fatalf("DELETE status=%d response=%s", status, string(body))
		}
		status, body = sendReconciliationRequestWithoutFailure(
			http.MethodPost,
			submodelRepositoryBaseURL+"/submodels",
			reconciliationBenchmarkSubmodel(submodelID, 100, iteration+1),
		)
		if status != http.StatusCreated {
			b.Fatalf("recreate status=%d response=%s", status, string(body))
		}
	}
	b.StopTimer()
	b.ReportMetric(reconciliationWALBytesSince(b, db, walStart)/float64(b.N), "wal-bytes/op")
	b.ReportMetric(float64(reconciliationScaleElementCount), "changed-rows/op")
}

func reconciliationBenchmarkSubmodel(submodelID string, changedPercentage int, generation int) map[string]any {
	changedElements := reconciliationScaleElementCount * changedPercentage / 100
	elements := make([]any, reconciliationScaleElementCount)
	for position := range elements {
		value := "stable"
		if position < changedElements {
			value = fmt.Sprintf("generation-%d", generation%2)
		}
		elements[position] = map[string]any{
			"idShort": fmt.Sprintf("P%04d", position), "modelType": "Property", "valueType": "xs:string", "value": value,
		}
	}
	return map[string]any{
		"id": submodelID, "idShort": "ReconciliationBenchmark", "modelType": "Submodel", "submodelElements": elements,
	}
}

func reconciliationWALLSN(tb testing.TB, db *sql.DB) string {
	tb.Helper()
	query, args, err := goqu.Dialect(common.Dialect).Select(goqu.L("pg_current_wal_lsn()::text")).Prepared(true).ToSQL()
	if err != nil {
		tb.Fatal(err)
	}
	var lsn string
	if err = db.QueryRowContext(tb.Context(), query, args...).Scan(&lsn); err != nil {
		tb.Fatal(err)
	}
	return lsn
}

func reconciliationWALBytesSince(tb testing.TB, db *sql.DB, start string) float64 {
	tb.Helper()
	query, args, err := goqu.Dialect(common.Dialect).
		Select(goqu.Func("pg_wal_lsn_diff", goqu.Func("pg_current_wal_lsn"), goqu.L("?::pg_lsn", start))).
		Prepared(true).
		ToSQL()
	if err != nil {
		tb.Fatal(err)
	}
	var walBytes float64
	if err = db.QueryRowContext(tb.Context(), query, args...).Scan(&walBytes); err != nil {
		tb.Fatal(err)
	}
	return walBytes
}

func TestPutSubmodelReconcilesThousandsOfElementsInPlace(t *testing.T) {
	submodelID := fmt.Sprintf("urn:basyx:integration:put-reconciliation-%d", time.Now().UnixNano())
	endpoint := submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(submodelID)
	initial := scaledReconciliationSubmodel(submodelID, false)
	status, body := sendReconciliationRequest(t, http.MethodPost, submodelRepositoryBaseURL+"/submodels", initial)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	t.Cleanup(func() { _, _ = sendReconciliationRequestWithoutFailure(http.MethodDelete, endpoint, nil) })

	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	rootIDBefore := reconciliationSubmodelDatabaseID(t, db, submodelID)
	before := reconciliationElementRows(t, db, submodelID)
	require.Len(t, before, reconciliationScaleElementCount)

	replacement := scaledReconciliationSubmodel(submodelID, true)
	status, body = sendReconciliationRequest(t, http.MethodPut, endpoint, replacement)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))

	require.Equal(t, rootIDBefore, reconciliationSubmodelDatabaseID(t, db, submodelID))
	after := reconciliationElementRows(t, db, submodelID)
	require.Len(t, after, reconciliationScaleElementCount)
	for path, oldRow := range before {
		newRow, exists := after[path]
		require.True(t, exists, "missing path %s", path)
		require.Equal(t, oldRow.id, newRow.id, "path %s was recreated", path)
		require.Equal(t, reconciliationScaleElementCount-1-oldRow.position, newRow.position, "path %s has wrong position", path)
	}

	typeChanged := scaledReconciliationSubmodel(submodelID, true)
	typeChanged["submodelElements"].([]any)[reconciliationScaleElementCount-1] = map[string]any{
		"idShort":   "P0000",
		"modelType": "Capability",
	}
	status, body = sendReconciliationRequest(t, http.MethodPut, endpoint, typeChanged)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))
	typeChangedRows := reconciliationElementRows(t, db, submodelID)
	require.NotEqual(t, after["P0000"].id, typeChangedRows["P0000"].id)
	for path, previousRow := range after {
		if path != "P0000" {
			require.Equal(t, previousRow.id, typeChangedRows[path].id, "unrelated path %s was recreated", path)
		}
	}
}

func TestPutSubmodelReconcilesNestedSubtreesAndKeepsRetainedIDs(t *testing.T) {
	submodelID := fmt.Sprintf("urn:basyx:integration:put-reconciliation-nested-%d", time.Now().UnixNano())
	endpoint := submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(submodelID)
	initial := nestedReconciliationSubmodel(submodelID, false)
	status, body := sendReconciliationRequest(t, http.MethodPost, submodelRepositoryBaseURL+"/submodels", initial)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	t.Cleanup(func() { _, _ = sendReconciliationRequestWithoutFailure(http.MethodDelete, endpoint, nil) })

	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	before := reconciliationAllElementRows(t, db, submodelID)
	require.Contains(t, before, "Container.Retained")
	require.Contains(t, before, "Container.Replaced")
	require.Contains(t, before, "Container.Replaced.Child")
	require.Contains(t, before, "Container.Items[0]")

	replacement := nestedReconciliationSubmodel(submodelID, true)
	status, body = sendReconciliationRequest(t, http.MethodPut, endpoint, replacement)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))
	after := reconciliationAllElementRows(t, db, submodelID)
	for _, path := range []string{"Container", "Container.Retained", "Container.Replaced", "Container.Replaced.Child", "Container.Items", "Container.Items[0]"} {
		require.Contains(t, after, path)
	}
	require.Equal(t, before["Container"].id, after["Container"].id)
	require.Equal(t, before["Container.Retained"].id, after["Container.Retained"].id)
	require.Equal(t, before["Container.Items"].id, after["Container.Items"].id)
	require.Equal(t, before["Container.Items[0]"].id, after["Container.Items[0]"].id)
	require.NotEqual(t, before["Container.Replaced"].id, after["Container.Replaced"].id)
	require.NotEqual(t, before["Container.Replaced.Child"].id, after["Container.Replaced.Child"].id)

	status, body = sendReconciliationRequest(t, http.MethodPut, endpoint, replacement)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))
	noOp := reconciliationAllElementRows(t, db, submodelID)
	for path, previous := range after {
		require.Equal(t, previous.id, noOp[path].id, "no-op PUT recreated path %s", path)
	}
}

func TestPutSubmodelAcceptsTypedEquivalentLexicalValues(t *testing.T) {
	submodelID := fmt.Sprintf("urn:basyx:integration:put-reconciliation-typed-%d", time.Now().UnixNano())
	endpoint := submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(submodelID)
	initial := typedReconciliationSubmodel(submodelID, "2026-08-10T12:00:00+02:00", "1.0", "2.00")
	status, body := sendReconciliationRequest(t, http.MethodPost, submodelRepositoryBaseURL+"/submodels", initial)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	t.Cleanup(func() { _, _ = sendReconciliationRequestWithoutFailure(http.MethodDelete, endpoint, nil) })

	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	before := reconciliationAllElementRows(t, db, submodelID)

	equivalent := typedReconciliationSubmodel(submodelID, "2026-08-10T10:00:00Z", "1.00", "2")
	status, body = sendReconciliationRequest(t, http.MethodPut, endpoint, equivalent)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))
	after := reconciliationAllElementRows(t, db, submodelID)
	require.Equal(t, before["Timestamp"].id, after["Timestamp"].id)
	require.Equal(t, before["Bounds"].id, after["Bounds"].id)
}

func nestedReconciliationSubmodel(submodelID string, replacement bool) map[string]any {
	replaced := map[string]any{
		"idShort":   "Replaced",
		"modelType": "SubmodelElementCollection",
		"value": []any{map[string]any{
			"idShort": "Child", "modelType": "Property", "valueType": "xs:string", "value": "old",
		}},
	}
	retainedValue := "old"
	if replacement {
		retainedValue = "new"
		replaced = map[string]any{
			"idShort": "Replaced", "modelType": "Entity", "entityType": "CoManagedEntity",
			"statements": []any{map[string]any{
				"idShort": "Child", "modelType": "Property", "valueType": "xs:string", "value": "new",
			}},
		}
	}
	return map[string]any{
		"id": submodelID, "idShort": "NestedReconciliation", "modelType": "Submodel",
		"submodelElements": []any{map[string]any{
			"idShort": "Container", "modelType": "SubmodelElementCollection",
			"value": []any{
				map[string]any{"idShort": "Retained", "modelType": "Property", "valueType": "xs:string", "value": retainedValue},
				replaced,
				map[string]any{
					"idShort": "Items", "modelType": "SubmodelElementList", "typeValueListElement": "Property", "valueTypeListElement": "xs:string",
					"value": []any{map[string]any{"modelType": "Property", "valueType": "xs:string", "value": "item"}},
				},
			},
		}},
	}
}

func typedReconciliationSubmodel(submodelID string, dateTime string, minValue string, maxValue string) map[string]any {
	return map[string]any{
		"id": submodelID, "idShort": "TypedReconciliation", "modelType": "Submodel",
		"submodelElements": []any{
			map[string]any{"idShort": "Timestamp", "modelType": "Property", "valueType": "xs:dateTime", "value": dateTime},
			map[string]any{"idShort": "Bounds", "modelType": "Range", "valueType": "xs:decimal", "min": minValue, "max": maxValue},
		},
	}
}

func scaledReconciliationSubmodel(submodelID string, replacement bool) map[string]any {
	elements := make([]any, reconciliationScaleElementCount)
	for position := range elements {
		logicalIndex := position
		value := "old"
		if replacement {
			logicalIndex = reconciliationScaleElementCount - 1 - position
			value = "new"
		}
		elements[position] = map[string]any{
			"idShort":   fmt.Sprintf("P%04d", logicalIndex),
			"modelType": "Property",
			"valueType": "xs:string",
			"value":     value,
		}
	}
	return map[string]any{
		"id":               submodelID,
		"idShort":          "ReconciliationScale",
		"kind":             "Instance",
		"modelType":        "Submodel",
		"submodelElements": elements,
	}
}

func sendReconciliationRequest(t *testing.T, method string, endpoint string, payload any) (int, []byte) {
	t.Helper()
	status, body := sendReconciliationRequestWithoutFailure(method, endpoint, payload)
	return status, body
}

func sendReconciliationRequestWithoutFailure(method string, endpoint string, payload any) (int, []byte) {
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return 0, []byte(err.Error())
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return 0, []byte(err.Error())
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	// #nosec G704 -- integration test calls a fixed local repository endpoint.
	response, err := (&http.Client{Timeout: time.Minute}).Do(request)
	if err != nil {
		return 0, []byte(err.Error())
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, []byte(err.Error())
	}
	return response.StatusCode, responseBody
}

func reconciliationSubmodelDatabaseID(t *testing.T, db *sql.DB, submodelID string) int64 {
	t.Helper()
	query, args, err := goqu.Dialect(common.Dialect).
		From("submodel").
		Select("id").
		Where(goqu.C("submodel_identifier").Eq(submodelID)).
		Prepared(true).
		ToSQL()
	require.NoError(t, err)
	var id int64
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&id))
	return id
}

func reconciliationElementRows(t *testing.T, db *sql.DB, submodelID string) map[string]persistedReconciliationElement {
	t.Helper()
	return queryReconciliationElementRows(t, db, submodelID, true)
}

func reconciliationAllElementRows(t *testing.T, db *sql.DB, submodelID string) map[string]persistedReconciliationElement {
	t.Helper()
	return queryReconciliationElementRows(t, db, submodelID, false)
}

func queryReconciliationElementRows(t *testing.T, db *sql.DB, submodelID string, rootsOnly bool) map[string]persistedReconciliationElement {
	t.Helper()
	dataset := goqu.Dialect(common.Dialect).
		From(goqu.T("submodel_element").As("element")).
		Join(goqu.T("submodel").As("submodel"), goqu.On(goqu.I("submodel.id").Eq(goqu.I("element.submodel_id")))).
		Select(goqu.I("element.idshort_path"), goqu.I("element.id"), goqu.I("element.position")).
		Where(goqu.I("submodel.submodel_identifier").Eq(submodelID))
	if rootsOnly {
		dataset = dataset.Where(goqu.I("element.parent_sme_id").IsNull())
	}
	query, args, err := dataset.
		Prepared(true).
		ToSQL()
	require.NoError(t, err)
	rows, err := db.QueryContext(t.Context(), query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	result := make(map[string]persistedReconciliationElement, reconciliationScaleElementCount)
	for rows.Next() {
		var path string
		var row persistedReconciliationElement
		require.NoError(t, rows.Scan(&path, &row.id, &row.position))
		result[path] = row
	}
	require.NoError(t, rows.Err())
	return result
}
