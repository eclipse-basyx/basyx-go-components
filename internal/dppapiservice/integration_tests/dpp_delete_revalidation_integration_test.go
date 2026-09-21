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

package integration_tests

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

type deleteRequestResult struct {
	status int
	err    error
}

func testDPPDeleteRevalidationConcurrency(
	t *testing.T,
	client *http.Client,
	baseURL, aasBaseURL string,
	databasePort int,
	idSuffix string,
	now time.Time,
) {
	t.Helper()
	db := openDPPIntegrationDatabase(t, databasePort)
	defer func() { _ = db.Close() }()

	t.Run("reference changed while delete waits", func(t *testing.T) {
		dppID, metadataID, technicalID := createDeleteRevalidationDPP(t, client, baseURL, databasePort, idSuffix+"-reference", now)
		tx := beginDeleteRevalidationTransaction(t, db)
		defer func() { _ = tx.Rollback() }()
		lockAASForDeleteRevalidation(t, tx, dppID)
		updateAASReferenceForDeleteRevalidation(t, tx, dppID, metadataID, technicalID)
		result := startDPPDelete(t.Context(), client, baseURL, dppID)
		waitForDeleteBlockedByTransaction(t, db, tx)
		commitDeleteRevalidationTransaction(t, tx)
		assertRejectedDeletePreservedResources(t, client, baseURL, aasBaseURL, dppID, metadataID, technicalID, result)
	})

	t.Run("metadata ID changed while delete waits", func(t *testing.T) {
		dppID, metadataID, technicalID := createDeleteRevalidationDPP(t, client, baseURL, databasePort, idSuffix+"-metadata", now)
		tx := beginDeleteRevalidationTransaction(t, db)
		defer func() { _ = tx.Rollback() }()
		lockSubmodelForDeleteRevalidation(t, tx, metadataID)
		updateMetadataIDForDeleteRevalidation(t, tx, metadataID, dppID+"/changed")
		result := startDPPDelete(t.Context(), client, baseURL, dppID)
		waitForDeleteBlockedByTransaction(t, db, tx)
		commitDeleteRevalidationTransaction(t, tx)
		assertRejectedDeletePreservedResources(t, client, baseURL, aasBaseURL, dppID, metadataID, technicalID, result)
	})
}

func createDeleteRevalidationDPP(
	t *testing.T,
	client *http.Client,
	baseURL string,
	databasePort int,
	suffix string,
	now time.Time,
) (string, string, string) {
	t.Helper()
	dppID := "https://www.example.org/dpp/delete-revalidation/" + suffix
	productID := "https://www.example.org/product/delete-revalidation/" + suffix
	doJSON(t, client, http.MethodPost, baseURL+"/v1/dpps", lifecycleDPPDocument(dppID, productID, now), http.StatusCreated)
	return dppID, dppID + "/submodels/DppMetadata", submodelIDBySemanticID(t, databasePort, dppID, lifecycleTechnicalDataSpec)
}

func beginDeleteRevalidationTransaction(t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin delete revalidation transaction: %v", err)
	}
	return tx
}

func lockAASForDeleteRevalidation(t *testing.T, tx *sql.Tx, dppID string) {
	t.Helper()
	dataset := goqu.Dialect("postgres").From("aas").Select("id").Where(goqu.C("aas_id").Eq(dppID)).ForUpdate(goqu.Wait).Prepared(true)
	query, args, err := dataset.ToSQL()
	if err != nil {
		t.Fatalf("build AAS lock query: %v", err)
	}
	var id int64
	if err = tx.QueryRowContext(t.Context(), query, args...).Scan(&id); err != nil {
		t.Fatalf("lock AAS %q: %v", dppID, err)
	}
}

func lockSubmodelForDeleteRevalidation(t *testing.T, tx *sql.Tx, submodelID string) {
	t.Helper()
	dataset := goqu.Dialect("postgres").From("submodel").Select("id").Where(goqu.C("submodel_identifier").Eq(submodelID)).ForUpdate(goqu.Wait).Prepared(true)
	query, args, err := dataset.ToSQL()
	if err != nil {
		t.Fatalf("build Submodel lock query: %v", err)
	}
	var id int64
	if err = tx.QueryRowContext(t.Context(), query, args...).Scan(&id); err != nil {
		t.Fatalf("lock Submodel %q: %v", submodelID, err)
	}
}

func updateAASReferenceForDeleteRevalidation(t *testing.T, tx *sql.Tx, dppID, metadataID, replacementID string) {
	t.Helper()
	referenceKeyID := aasReferenceKeyID(t, tx, dppID, metadataID)
	dataset := goqu.Dialect("postgres").Update("aas_submodel_reference_key").
		Set(goqu.Record{"value": replacementID}).Where(goqu.C("id").Eq(referenceKeyID)).Prepared(true)
	executeDeleteRevalidationUpdate(t, tx, dataset)
}

func aasReferenceKeyID(t *testing.T, tx *sql.Tx, aasID, submodelID string) int64 {
	t.Helper()
	dataset := goqu.Dialect("postgres").
		From(goqu.T("aas_submodel_reference_key").As("key")).
		Join(goqu.T("aas_submodel_reference").As("reference"), goqu.On(goqu.I("reference.id").Eq(goqu.I("key.reference_id")))).
		Join(goqu.T("aas").As("aas"), goqu.On(goqu.I("aas.id").Eq(goqu.I("reference.aas_id")))).
		Select(goqu.I("key.id")).
		Where(goqu.I("aas.aas_id").Eq(aasID), goqu.I("key.value").Eq(submodelID)).
		Prepared(true)
	query, args, err := dataset.ToSQL()
	if err != nil {
		t.Fatalf("build AAS reference key query: %v", err)
	}
	var id int64
	if err = tx.QueryRowContext(t.Context(), query, args...).Scan(&id); err != nil {
		t.Fatalf("read AAS reference key: %v", err)
	}
	return id
}

func updateMetadataIDForDeleteRevalidation(t *testing.T, tx *sql.Tx, metadataID, replacementDPPID string) {
	t.Helper()
	dataset := goqu.Dialect("postgres").
		From(goqu.T("property_element").As("property")).
		Join(goqu.T("submodel_element").As("element"), goqu.On(goqu.I("element.id").Eq(goqu.I("property.id")))).
		Join(goqu.T("submodel").As("submodel"), goqu.On(goqu.I("submodel.id").Eq(goqu.I("element.submodel_id")))).
		Select(goqu.I("property.id")).
		Where(
			goqu.I("submodel.submodel_identifier").Eq(metadataID),
			goqu.I("element.id_short").Eq("digitalProductPassportId"),
		).
		Prepared(true)
	query, args, err := dataset.ToSQL()
	if err != nil {
		t.Fatalf("build metadata property query: %v", err)
	}
	var propertyID int64
	if err = tx.QueryRowContext(t.Context(), query, args...).Scan(&propertyID); err != nil {
		t.Fatalf("read metadata property: %v", err)
	}
	update := goqu.Dialect("postgres").Update("property_element").
		Set(goqu.Record{"value_text": replacementDPPID}).Where(goqu.C("id").Eq(propertyID)).Prepared(true)
	executeDeleteRevalidationUpdate(t, tx, update)
}

func executeDeleteRevalidationUpdate(t *testing.T, tx *sql.Tx, dataset *goqu.UpdateDataset) {
	t.Helper()
	query, args, err := dataset.ToSQL()
	if err != nil {
		t.Fatalf("build delete revalidation update: %v", err)
	}
	result, err := tx.ExecContext(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("execute delete revalidation update: %v", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		t.Fatalf("delete revalidation update affected %d rows: %v", affected, err)
	}
}

func startDPPDelete(ctx context.Context, client *http.Client, baseURL, dppID string) <-chan deleteRequestResult {
	result := make(chan deleteRequestResult, 1)
	go func() {
		request, err := http.NewRequestWithContext(ctx, http.MethodDelete, baseURL+"/v1/dpps/"+encodedPathParam(dppID), nil)
		if err != nil {
			result <- deleteRequestResult{err: err}
			return
		}
		response, err := client.Do(request)
		if err != nil {
			result <- deleteRequestResult{err: err}
			return
		}
		defer func() { _ = response.Body.Close() }()
		_, err = io.Copy(io.Discard, response.Body)
		result <- deleteRequestResult{status: response.StatusCode, err: err}
	}()
	return result
}

func waitForDeleteBlockedByTransaction(t *testing.T, db *sql.DB, tx *sql.Tx) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	writerPID := transactionPID(t, tx)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if transactionBlocksAnotherBackend(t, db, writerPID) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("delete did not block behind writer transaction %d: %v", writerPID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func transactionPID(t *testing.T, tx *sql.Tx) int {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").Select(goqu.Func("pg_backend_pid")).Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("build backend PID query: %v", err)
	}
	var pid int
	if err = tx.QueryRowContext(t.Context(), query, args...).Scan(&pid); err != nil {
		t.Fatalf("read backend PID: %v", err)
	}
	return pid
}

func transactionBlocksAnotherBackend(t *testing.T, db *sql.DB, writerPID int) bool {
	t.Helper()
	dataset := goqu.Dialect("postgres").From("pg_stat_activity").Select(goqu.COUNT(goqu.Star())).
		Where(goqu.L("? = ANY(pg_blocking_pids(?))", writerPID, goqu.I("pid"))).Prepared(true)
	query, args, err := dataset.ToSQL()
	if err != nil {
		t.Fatalf("build blocker query: %v", err)
	}
	var count int
	if err = db.QueryRowContext(t.Context(), query, args...).Scan(&count); err != nil {
		t.Fatalf("inspect blocked delete: %v", err)
	}
	return count > 0
}

func commitDeleteRevalidationTransaction(t *testing.T, tx *sql.Tx) {
	t.Helper()
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit delete revalidation transaction: %v", err)
	}
}

func assertRejectedDeletePreservedResources(
	t *testing.T,
	client *http.Client,
	baseURL, aasBaseURL, dppID, metadataID, contentID string,
	result <-chan deleteRequestResult,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	select {
	case completed := <-result:
		if completed.err != nil || completed.status != http.StatusNotFound {
			t.Fatalf("delete result = status %d, error %v, want 404", completed.status, completed.err)
		}
	case <-ctx.Done():
		t.Fatalf("delete did not complete after writer commit: %v", ctx.Err())
	}
	doJSON(t, client, http.MethodGet, aasBaseURL+"/shells/"+common.EncodeString(dppID), nil, http.StatusOK)
	doJSON(t, client, http.MethodGet, aasBaseURL+"/submodels/"+common.EncodeString(metadataID), nil, http.StatusOK)
	doJSON(t, client, http.MethodGet, aasBaseURL+"/submodels/"+common.EncodeString(contentID), nil, http.StatusOK)
	doJSONAny(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedPathParam(dppID), nil, http.StatusNotFound)
}
