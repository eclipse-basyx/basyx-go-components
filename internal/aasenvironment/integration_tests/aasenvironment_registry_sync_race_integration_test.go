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

//nolint:all
package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/require"
)

func TestConcurrentPutAndDeleteSubmodelThroughAASDoesNotDeadlock(t *testing.T) {
	testCases := []struct {
		name    string
		baseURL string
		dsn     string
	}{
		{name: "registry synchronization enabled", baseURL: aasEnvBaseURL, dsn: integrationTestDSN},
		{name: "registry synchronization disabled", baseURL: aasEnvSyncOffBaseURL, dsn: uploadSyncDisabledIntegrationDSN},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, testCase.dsn)
			testConcurrentPutAndDeleteSubmodelThroughAAS(t, testCase.baseURL, testCase.dsn)
		})
	}
}

func testConcurrentPutAndDeleteSubmodelThroughAAS(t *testing.T, baseURL string, dsn string) {
	t.Helper()
	testID := time.Now().UnixNano()
	aasID := fmt.Sprintf("https://example.org/aas/put-delete-%d", testID)
	submodelID := fmt.Sprintf("https://example.org/submodel/put-delete-%d", testID)
	encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	encodedSubmodelID := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	endpoint := fmt.Sprintf("%s/shells/%s/submodels/%s", baseURL, encodedAASID, encodedSubmodelID)

	aasPayload := map[string]any{
		"id":        aasID,
		"idShort":   "ConcurrentPutDeleteShell",
		"modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{
			"assetKind": "Instance",
		},
	}
	status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPost, baseURL+"/shells", aasPayload)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))

	initialSubmodel := concurrentPutDeleteSubmodelPayload(submodelID, "before")
	status, body, _ = doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPut, endpoint, initialSubmodel)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	require.Equal(t, 0, databaseLockWaiterCount(t, db))

	blockerTx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	blockerReleased := false
	defer func() {
		if !blockerReleased {
			_ = blockerTx.Rollback()
		}
	}()
	lockConcurrentPutDeleteProperty(t, blockerTx, submodelID)

	type requestResult struct {
		method string
		status int
		body   []byte
		err    error
	}
	results := make(chan requestResult, 2)
	go func() {
		putStatus, putBody, putErr := doAASEnvRawJSONRequest(http.MethodPut, endpoint, concurrentPutDeleteSubmodelPayload(submodelID, "after"))
		results <- requestResult{method: http.MethodPut, status: putStatus, body: putBody, err: putErr}
	}()
	waitForDatabaseLockWaiters(t, db, 1)

	go func() {
		deleteStatus, deleteBody, deleteErr := doAASEnvRawJSONRequest(http.MethodDelete, endpoint, nil)
		results <- requestResult{method: http.MethodDelete, status: deleteStatus, body: deleteBody, err: deleteErr}
	}()
	waitForDatabaseLockWaiters(t, db, 2)

	require.NoError(t, blockerTx.Commit())
	blockerReleased = true

	statuses := make(map[string]int, 2)
	for range 2 {
		select {
		case result := <-results:
			require.NoError(t, result.err)
			require.NotEqual(t, http.StatusInternalServerError, result.status, "%s response=%s", result.method, string(result.body))
			statuses[result.method] = result.status
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for concurrent PUT and DELETE requests")
		}
	}
	require.Equal(t, http.StatusNoContent, statuses[http.MethodPut])
	require.Equal(t, http.StatusNoContent, statuses[http.MethodDelete])

	status, body, _ = doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, endpoint, nil)
	require.Equal(t, http.StatusNotFound, status, "response=%s", string(body))
}

func concurrentPutDeleteSubmodelPayload(submodelID string, value string) map[string]any {
	return map[string]any{
		"id":        submodelID,
		"idShort":   "ConcurrentPutDeleteSubmodel",
		"kind":      "Instance",
		"modelType": "Submodel",
		"submodelElements": []any{
			map[string]any{
				"idShort":   "BlockedProperty",
				"modelType": "Property",
				"valueType": "xs:string",
				"value":     value,
			},
		},
	}
}

func lockConcurrentPutDeleteProperty(t *testing.T, tx *sql.Tx, submodelID string) {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").
		From(goqu.T("property_element").As("pe")).
		Join(goqu.T("submodel_element").As("sme"), goqu.On(goqu.I("sme.id").Eq(goqu.I("pe.id")))).
		Join(goqu.T("submodel").As("sm"), goqu.On(goqu.I("sm.id").Eq(goqu.I("sme.submodel_id")))).
		Select(goqu.I("pe.id")).
		Where(
			goqu.I("sm.submodel_identifier").Eq(submodelID),
			goqu.I("sme.id_short").Eq("BlockedProperty"),
		).
		ForUpdate(goqu.Wait, goqu.I("pe")).
		Prepared(true).
		ToSQL()
	require.NoError(t, err)
	var propertyID int64
	require.NoError(t, tx.QueryRowContext(t.Context(), query, args...).Scan(&propertyID))
}

func waitForDatabaseLockWaiters(t *testing.T, db *sql.DB, expected int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if databaseLockWaiterCount(t, db) >= expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d database lock waiters", expected)
}

func databaseLockWaiterCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").
		From("pg_stat_activity").
		Select(goqu.COUNT("*")).
		Where(
			goqu.C("datname").Eq(goqu.Func("current_database")),
			goqu.C("pid").Neq(goqu.Func("pg_backend_pid")),
			goqu.C("wait_event_type").Eq("Lock"),
		).
		Prepared(true).
		ToSQL()
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&count))
	return count
}

func TestRegistrySyncConcurrentSubmodelReferenceCreationDoesNotRaceAASDescriptorUpsert(t *testing.T) {
	resetDatabase(t)

	const iterations = 20
	const parallelReferences = 4

	for iteration := 0; iteration < iterations; iteration++ {
		suffix := fmt.Sprintf("race-%d-%d", time.Now().UnixNano(), iteration)
		aasID := fmt.Sprintf("https://example.org/aas/%s", suffix)
		encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))

		createRegistrySyncRaceSubmodels(t, suffix, parallelReferences)
		createRegistrySyncRaceShell(t, aasID, suffix)

		results := postRegistrySyncRaceSubmodelReferences(t, encodedAASID, suffix, parallelReferences)
		for _, result := range results {
			require.NoError(t, result.err)
			require.Equal(t, http.StatusCreated, result.status, "iteration=%d submodel=%d response=%s", iteration, result.index, string(result.body))
		}

		status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, aasEnvBaseURL+"/shells/"+encodedAASID+"/submodel-refs", nil)
		require.Equal(t, http.StatusOK, status, "response=%s", string(body))
		requireRegistrySyncRaceReferenceCount(t, body, parallelReferences)
	}
}

type registrySyncRacePostResult struct {
	index  int
	status int
	body   []byte
	err    error
}

func createRegistrySyncRaceSubmodels(t *testing.T, suffix string, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		submodelID := registrySyncRaceSubmodelID(suffix, index)
		payload := map[string]any{
			"id":        submodelID,
			"idShort":   fmt.Sprintf("smRace%d", index),
			"modelType": "Submodel",
		}
		status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPost, aasEnvBaseURL+"/submodels", payload)
		require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	}
}

func createRegistrySyncRaceShell(t *testing.T, aasID string, suffix string) {
	t.Helper()
	payload := map[string]any{
		"id":        aasID,
		"idShort":   suffix,
		"modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{
			"assetKind":     "Instance",
			"globalAssetId": fmt.Sprintf("https://example.org/asset/%s", suffix),
		},
	}
	status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPost, aasEnvBaseURL+"/shells", payload)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
}

func postRegistrySyncRaceSubmodelReferences(t *testing.T, encodedAASID string, suffix string, count int) []registrySyncRacePostResult {
	t.Helper()
	results := make([]registrySyncRacePostResult, count)
	var wg sync.WaitGroup
	start := make(chan struct{})

	for index := 0; index < count; index++ {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			status, body, err := doAASEnvRawJSONRequest(
				http.MethodPost,
				aasEnvBaseURL+"/shells/"+encodedAASID+"/submodel-refs",
				map[string]any{
					"type": "ModelReference",
					"keys": []map[string]any{
						{
							"type":  "Submodel",
							"value": registrySyncRaceSubmodelID(suffix, index),
						},
					},
				},
			)
			results[index] = registrySyncRacePostResult{
				index:  index,
				status: status,
				body:   body,
				err:    err,
			}
		}()
	}

	close(start)
	wg.Wait()
	return results
}

func registrySyncRaceSubmodelID(suffix string, index int) string {
	return fmt.Sprintf("https://example.org/sm/%s-%d", suffix, index)
}

func doAASEnvRawJSONRequest(method string, endpoint string, payload any) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		marshaled, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(marshaled)
	}

	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return 0, nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := aasEnvNoRedirectClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, respBody, nil
}

func requireRegistrySyncRaceReferenceCount(t *testing.T, body []byte, expectedCount int) {
	t.Helper()
	var payload struct {
		Result []any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Len(t, payload.Result, expectedCount)
}
