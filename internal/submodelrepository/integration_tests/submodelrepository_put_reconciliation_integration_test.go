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
	"sync"
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

func TestConcurrentIdenticalPutSubmodelUpdatesExistingResource(t *testing.T) {
	submodelID := fmt.Sprintf("urn:basyx:integration:concurrent-put-%d", time.Now().UnixNano())
	endpoint := submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(submodelID)
	payload := map[string]any{
		"id": submodelID, "idShort": "ConcurrentPut", "modelType": "Submodel",
		"submodelElements": []any{map[string]any{
			"idShort": "Value", "modelType": "Property", "valueType": "xs:string", "value": "same",
		}},
	}
	status, body := sendReconciliationRequest(t, http.MethodPost, submodelRepositoryBaseURL+"/submodels", payload)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	t.Cleanup(func() { _, _ = sendReconciliationRequestWithoutFailure(http.MethodDelete, endpoint, nil) })
	payload["category"] = "updated"

	type result struct {
		status int
		body   []byte
	}
	const requestCount = 8
	results := make(chan result, requestCount)
	var waitGroup sync.WaitGroup
	for range requestCount {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			putStatus, putBody := sendReconciliationRequestWithoutFailure(http.MethodPut, endpoint, payload)
			results <- result{status: putStatus, body: putBody}
		}()
	}
	waitGroup.Wait()
	close(results)
	for putResult := range results {
		require.Equal(t, http.StatusNoContent, putResult.status, "response=%s", string(putResult.body))
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
	query, args, err := goqu.Dialect(common.Dialect).
		From(goqu.T("submodel_element").As("element")).
		Join(goqu.T("submodel").As("submodel"), goqu.On(goqu.I("submodel.id").Eq(goqu.I("element.submodel_id")))).
		Select(goqu.I("element.idshort_path"), goqu.I("element.id"), goqu.I("element.position")).
		Where(
			goqu.I("submodel.submodel_identifier").Eq(submodelID),
			goqu.I("element.parent_sme_id").IsNull(),
		).
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
