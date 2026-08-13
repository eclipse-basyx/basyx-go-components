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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

type persistedAASReconciliationReference struct {
	id       int64
	position int
}

func TestPutAssetAdministrationShellReconcilesRowsInPlace(t *testing.T) {
	aasID := fmt.Sprintf("urn:basyx:integration:aas-reconciliation-%d", time.Now().UnixNano())
	endpoint := aasRepositoryBaseURL + "/shells/" + base64.RawURLEncoding.EncodeToString([]byte(aasID))
	initial := aasReconciliationPayload(aasID, false)
	encodedInitial, err := json.Marshal(initial)
	require.NoError(t, err)
	_, status, _, err := postJSONResponse(aasRepositoryBaseURL+"/shells", string(encodedInitial))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	t.Cleanup(func() { deleteAASForLargeObjectCleanupTest(t, endpoint) })

	db, err := sql.Open("pgx", integrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	rootBefore := reconciliationAASDatabaseID(t, db, aasID)
	specificBefore := reconciliationSpecificAssetIDRows(t, db, rootBefore)
	referencesBefore := reconciliationAASReferenceRows(t, db, rootBefore)

	replacement := aasReconciliationPayload(aasID, true)
	encodedReplacement, err := json.Marshal(replacement)
	require.NoError(t, err)
	_, status, _, err = putJSONResponse(endpoint, string(encodedReplacement))
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status)

	require.Equal(t, rootBefore, reconciliationAASDatabaseID(t, db, aasID))
	require.Equal(t, specificBefore, reconciliationSpecificAssetIDRows(t, db, rootBefore))
	referencesAfter := reconciliationAASReferenceRows(t, db, rootBefore)
	require.Equal(t, referencesBefore["urn:basyx:sm:one"].id, referencesAfter["urn:basyx:sm:one"].id)
	require.Equal(t, 1, referencesAfter["urn:basyx:sm:one"].position)
	require.Equal(t, referencesBefore["urn:basyx:sm:two"].id, referencesAfter["urn:basyx:sm:two"].id)
	require.Equal(t, 0, referencesAfter["urn:basyx:sm:two"].position)
}

func TestConcurrentIdenticalPutAssetAdministrationShellUpdatesExistingResource(t *testing.T) {
	aasID := fmt.Sprintf("urn:basyx:integration:aas-concurrent-put-%d", time.Now().UnixNano())
	endpoint := aasRepositoryBaseURL + "/shells/" + base64.RawURLEncoding.EncodeToString([]byte(aasID))
	payload := aasReconciliationPayload(aasID, false)
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	_, status, _, err := postJSONResponse(aasRepositoryBaseURL+"/shells", string(encoded))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	t.Cleanup(func() { deleteAASForLargeObjectCleanupTest(t, endpoint) })
	payload["category"] = "concurrent-update"
	encoded, err = json.Marshal(payload)
	require.NoError(t, err)

	const requestCount = 8
	statuses := make(chan int, requestCount)
	errors := make(chan error, requestCount)
	var waitGroup sync.WaitGroup
	for range requestCount {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, putStatus, _, putErr := putJSONResponse(endpoint, string(encoded))
			statuses <- putStatus
			errors <- putErr
		}()
	}
	waitGroup.Wait()
	close(statuses)
	close(errors)
	for requestErr := range errors {
		require.NoError(t, requestErr)
	}
	for putStatus := range statuses {
		require.Equal(t, http.StatusNoContent, putStatus)
	}
}

func TestLargeAASSubmodelReferenceEndpointsMutateOnlyTargetedRows(t *testing.T) {
	const referenceCount = 1200

	testID := time.Now().UnixNano()
	aasID := fmt.Sprintf("urn:basyx:integration:aas-targeted-references-%d", testID)
	encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	aasEndpoint := aasRepositoryBaseURL + "/shells/" + encodedAASID
	referenceEndpoint := aasEndpoint + "/submodel-refs"
	references := make([]any, 0, referenceCount)
	referenceIDs := make([]string, 0, referenceCount)
	for index := range referenceCount {
		referenceID := fmt.Sprintf("urn:basyx:integration:sm-targeted-reference-%d-%d", testID, index)
		referenceIDs = append(referenceIDs, referenceID)
		references = append(references, aasReconciliationReference(referenceID))
	}
	payload := map[string]any{
		"id": aasID, "modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{"assetKind": "Instance"},
		"submodels":        references,
	}
	encodedPayload, err := json.Marshal(payload)
	require.NoError(t, err)
	_, status, _, err := postJSONResponse(aasRepositoryBaseURL+"/shells", string(encodedPayload))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	t.Cleanup(func() { deleteAASForLargeObjectCleanupTest(t, aasEndpoint) })

	db, err := sql.Open("pgx", integrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	aasDatabaseID := reconciliationAASDatabaseID(t, db, aasID)
	rowsBefore := reconciliationAASReferenceRows(t, db, aasDatabaseID)
	require.Len(t, rowsBefore, referenceCount)

	addedReferenceID := fmt.Sprintf("urn:basyx:integration:sm-targeted-reference-%d-added", testID)
	encodedReference, err := json.Marshal(aasReconciliationReference(addedReferenceID))
	require.NoError(t, err)
	status, err = postResponseStatus(referenceEndpoint, string(encodedReference))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	rowsAfterPost := reconciliationAASReferenceRows(t, db, aasDatabaseID)
	require.Len(t, rowsAfterPost, referenceCount+1)
	for referenceID, rowBefore := range rowsBefore {
		require.Equal(t, rowBefore, rowsAfterPost[referenceID])
	}
	require.Equal(t, referenceCount, rowsAfterPost[addedReferenceID].position)

	deletedReferenceID := referenceIDs[referenceCount/2]
	deleteEndpoint := referenceEndpoint + "/" + base64.RawURLEncoding.EncodeToString([]byte(deletedReferenceID))
	status, err = deleteResponseStatus(deleteEndpoint)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status)

	rowsAfterDelete := reconciliationAASReferenceRows(t, db, aasDatabaseID)
	require.Len(t, rowsAfterDelete, referenceCount)
	require.NotContains(t, rowsAfterDelete, deletedReferenceID)
	for referenceID, rowBefore := range rowsAfterPost {
		if referenceID != deletedReferenceID {
			require.Equal(t, rowBefore, rowsAfterDelete[referenceID])
		}
	}
}

func aasReconciliationPayload(aasID string, replacement bool) map[string]any {
	specificAssetIDs := []any{
		map[string]any{"name": "serial", "value": "one"},
		map[string]any{"name": "batch", "value": "two"},
	}
	references := []any{
		aasReconciliationReference("urn:basyx:sm:one"),
		aasReconciliationReference("urn:basyx:sm:two"),
	}
	if replacement {
		specificAssetIDs[1].(map[string]any)["value"] = "changed"
		references[0], references[1] = references[1], references[0]
	}
	return map[string]any{
		"id": aasID, "idShort": "Reconciliation", "modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{
			"assetKind": "Instance", "globalAssetId": aasID + ":asset", "specificAssetIds": specificAssetIDs,
		},
		"submodels": references,
	}
}

func aasReconciliationReference(submodelID string) map[string]any {
	return map[string]any{
		"type": "ModelReference",
		"keys": []any{map[string]any{"type": "Submodel", "value": submodelID}},
	}
}

func reconciliationAASDatabaseID(t *testing.T, db *sql.DB, aasID string) int64 {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("aas").Select("id").Where(goqu.C("aas_id").Eq(aasID)).Prepared(true).ToSQL()
	require.NoError(t, err)
	var id int64
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&id))
	return id
}

func reconciliationSpecificAssetIDRows(t *testing.T, db *sql.DB, aasDatabaseID int64) map[int]int64 {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("specific_asset_id").Select("position", "id").
		Where(goqu.C("asset_information_id").Eq(aasDatabaseID)).Prepared(true).ToSQL()
	require.NoError(t, err)
	rows, err := db.QueryContext(t.Context(), query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	result := make(map[int]int64)
	for rows.Next() {
		var position int
		var id int64
		require.NoError(t, rows.Scan(&position, &id))
		result[position] = id
	}
	require.NoError(t, rows.Err())
	return result
}

func reconciliationAASReferenceRows(t *testing.T, db *sql.DB, aasDatabaseID int64) map[string]persistedAASReconciliationReference {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From(goqu.T("aas_submodel_reference").As("reference")).
		Join(goqu.T("aas_submodel_reference_key").As("key"), goqu.On(goqu.I("key.reference_id").Eq(goqu.I("reference.id")))).
		Select(goqu.I("key.value"), goqu.I("reference.id"), goqu.I("reference.position")).
		Where(goqu.I("reference.aas_id").Eq(aasDatabaseID)).Prepared(true).ToSQL()
	require.NoError(t, err)
	rows, err := db.QueryContext(t.Context(), query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	result := make(map[string]persistedAASReconciliationReference)
	for rows.Next() {
		var value string
		var row persistedAASReconciliationReference
		require.NoError(t, rows.Scan(&value, &row.id, &row.position))
		result[value] = row
	}
	require.NoError(t, rows.Err())
	return result
}
