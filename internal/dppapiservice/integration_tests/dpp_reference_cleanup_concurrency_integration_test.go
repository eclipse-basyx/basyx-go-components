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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package integration_tests

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	"github.com/jackc/pgx/v5/pgconn"
)

type referenceCleanupResult struct {
	deleted bool
	err     error
}

func testSubmodelReferenceCleanupConcurrency(
	t *testing.T,
	client *http.Client,
	aasBaseURL string,
	databasePort int,
	idSuffix string,
) {
	t.Helper()
	db := openDPPIntegrationDatabase(t, databasePort)
	defer func() { _ = db.Close() }()
	repository, err := submodelrepositorydb.NewSubmodelDatabaseFromDB(db, nil, "off")
	if err != nil {
		t.Fatalf("create submodel repository for cleanup concurrency: %v", err)
	}

	testReferenceWriterCommitsBeforeCleanup(t, client, aasBaseURL, db, repository, idSuffix)
	testCleanupCommitsBeforeReferenceWriter(t, client, aasBaseURL, db, repository, idSuffix)
	testReferenceToInitiallyAbsentSubmodel(t, client, aasBaseURL, db, idSuffix)
}

func testReferenceWriterCommitsBeforeCleanup(
	t *testing.T,
	client *http.Client,
	aasBaseURL string,
	db *sql.DB,
	repository *submodelrepositorydb.SubmodelDatabase,
	idSuffix string,
) {
	t.Helper()
	targetID := "https://www.example.org/submodels/concurrency/writer-first/" + idSuffix
	createConcurrencyTargetSubmodel(t, client, aasBaseURL, targetID)
	referenceKeyID := createConcurrencyReferenceSource(t, client, aasBaseURL, db, "writer-first-"+idSuffix)
	ctx, cancel := cleanupConcurrencyContext(t)
	defer cancel()

	writerTx := beginConcurrencyTransaction(ctx, t, db)
	defer func() { _ = writerTx.Rollback() }()
	updateAASReferenceKey(ctx, t, writerTx, referenceKeyID, targetID)

	cleanupTx := beginConcurrencyTransaction(ctx, t, db)
	defer func() { _ = cleanupTx.Rollback() }()
	cleanupPID := transactionBackendPID(ctx, t, cleanupTx)
	result := make(chan referenceCleanupResult, 1)
	go func() {
		deleted, cleanupErr := repository.DeleteUnreferencedSubmodelInTransaction(ctx, cleanupTx, targetID)
		result <- referenceCleanupResult{deleted: deleted, err: cleanupErr}
	}()
	waitForBlockedBackend(ctx, t, db, cleanupPID)
	if err := writerTx.Commit(); err != nil {
		t.Fatalf("commit first reference writer: %v", err)
	}
	cleanupResult := awaitCleanupResult(ctx, t, result)
	if cleanupResult.err != nil || cleanupResult.deleted {
		t.Fatalf("cleanup after reference commit = (deleted %t, error %v), want retained", cleanupResult.deleted, cleanupResult.err)
	}
	if err := cleanupTx.Commit(); err != nil {
		t.Fatalf("commit retained cleanup transaction: %v", err)
	}
	doJSON(t, client, http.MethodGet, aasBaseURL+"/submodels/"+common.EncodeString(targetID), nil, http.StatusOK)
}

func testCleanupCommitsBeforeReferenceWriter(
	t *testing.T,
	client *http.Client,
	aasBaseURL string,
	db *sql.DB,
	repository *submodelrepositorydb.SubmodelDatabase,
	idSuffix string,
) {
	t.Helper()
	targetID := "https://www.example.org/submodels/concurrency/cleanup-first/" + idSuffix
	createConcurrencyTargetSubmodel(t, client, aasBaseURL, targetID)
	referenceKeyID := createConcurrencyReferenceSource(t, client, aasBaseURL, db, "cleanup-first-"+idSuffix)
	ctx, cancel := cleanupConcurrencyContext(t)
	defer cancel()

	cleanupTx := beginConcurrencyTransaction(ctx, t, db)
	defer func() { _ = cleanupTx.Rollback() }()
	deleted, err := repository.DeleteUnreferencedSubmodelInTransaction(ctx, cleanupTx, targetID)
	if err != nil || !deleted {
		t.Fatalf("first cleanup = (deleted %t, error %v), want deleted", deleted, err)
	}

	writerTx := beginConcurrencyTransaction(ctx, t, db)
	defer func() { _ = writerTx.Rollback() }()
	writerPID := transactionBackendPID(ctx, t, writerTx)
	writerResult := make(chan error, 1)
	go func() {
		writerResult <- updateAASReferenceKeyInTransaction(ctx, writerTx, referenceKeyID, targetID)
	}()
	waitForBlockedBackend(ctx, t, db, writerPID)
	if err = cleanupTx.Commit(); err != nil {
		t.Fatalf("commit first cleanup: %v", err)
	}
	writerErr := awaitWriterResult(ctx, t, writerResult)
	assertSQLState(t, writerErr, "40001")
}

func testReferenceToInitiallyAbsentSubmodel(
	t *testing.T,
	client *http.Client,
	aasBaseURL string,
	db *sql.DB,
	idSuffix string,
) {
	t.Helper()
	ctx, cancel := cleanupConcurrencyContext(t)
	defer cancel()
	referenceKeyID := createConcurrencyReferenceSource(t, client, aasBaseURL, db, "initially-absent-"+idSuffix)
	tx := beginConcurrencyTransaction(ctx, t, db)
	defer func() { _ = tx.Rollback() }()
	targetID := "https://www.example.org/submodels/concurrency/initially-absent/" + idSuffix
	if err := updateAASReferenceKeyInTransaction(ctx, tx, referenceKeyID, targetID); err != nil {
		t.Fatalf("write reference to initially absent submodel: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit reference to initially absent submodel: %v", err)
	}
}

func cleanupConcurrencyContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	return context.WithTimeout(ctx, 5*time.Second)
}

func createConcurrencyReferenceSource(
	t *testing.T,
	client *http.Client,
	aasBaseURL string,
	db *sql.DB,
	suffix string,
) int64 {
	t.Helper()
	aasID := "https://www.example.org/aas/concurrency-reference-source/" + suffix
	placeholderID := "https://www.example.org/submodels/concurrency/placeholder/" + suffix
	doJSON(t, client, http.MethodPost, aasBaseURL+"/shells", map[string]any{
		"id":        aasID,
		"idShort":   "ConcurrencyReferenceSource",
		"modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{
			"assetKind": "Instance",
		},
		"submodels": []any{submodelReferencePayload(placeholderID)},
	}, http.StatusCreated)
	return aasSubmodelReferenceKeyID(t, db, aasID, placeholderID)
}

func aasSubmodelReferenceKeyID(t *testing.T, db *sql.DB, aasID string, submodelID string) int64 {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").
		From(goqu.T("aas_submodel_reference_key").As("reference_key")).
		Join(
			goqu.T("aas_submodel_reference").As("reference"),
			goqu.On(goqu.I("reference.id").Eq(goqu.I("reference_key.reference_id"))),
		).
		Join(goqu.T("aas").As("aas"), goqu.On(goqu.I("aas.id").Eq(goqu.I("reference.aas_id")))).
		Select(goqu.I("reference_key.id")).
		Where(goqu.I("aas.aas_id").Eq(aasID), goqu.I("reference_key.value").Eq(submodelID)).
		Prepared(true).
		ToSQL()
	if err != nil {
		t.Fatalf("build concurrency reference key query: %v", err)
	}
	var keyID int64
	if err = db.QueryRowContext(t.Context(), query, args...).Scan(&keyID); err != nil {
		t.Fatalf("read concurrency reference key: %v", err)
	}
	return keyID
}

func createConcurrencyTargetSubmodel(t *testing.T, client *http.Client, aasBaseURL string, targetID string) {
	t.Helper()
	doJSON(t, client, http.MethodPost, aasBaseURL+"/submodels", map[string]any{
		"id":        targetID,
		"idShort":   "ConcurrencyCleanupTarget",
		"modelType": "Submodel",
		"submodelElements": []any{map[string]any{
			"idShort":   "value",
			"modelType": "Property",
			"valueType": "xs:string",
			"value":     "keep or delete deterministically",
		}},
	}, http.StatusCreated)
}

func beginConcurrencyTransaction(ctx context.Context, t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin cleanup concurrency transaction: %v", err)
	}
	return tx
}

func updateAASReferenceKey(ctx context.Context, t *testing.T, tx *sql.Tx, keyID int64, targetID string) {
	t.Helper()
	if err := updateAASReferenceKeyInTransaction(ctx, tx, keyID, targetID); err != nil {
		t.Fatalf("write inbound reference: %v", err)
	}
}

func updateAASReferenceKeyInTransaction(ctx context.Context, tx *sql.Tx, keyID int64, targetID string) error {
	query, args, err := goqu.Dialect("postgres").
		Update("aas_submodel_reference_key").
		Set(goqu.Record{"value": targetID}).
		Where(goqu.C("id").Eq(keyID)).
		Prepared(true).
		ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, query, args...)
	return err
}

func transactionBackendPID(ctx context.Context, t *testing.T, tx *sql.Tx) int {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").Select(goqu.Func("pg_backend_pid")).ToSQL()
	if err != nil {
		t.Fatalf("build backend PID query: %v", err)
	}
	var pid int
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&pid); err != nil {
		t.Fatalf("read transaction backend PID: %v", err)
	}
	return pid
}

func waitForBlockedBackend(ctx context.Context, t *testing.T, db *sql.DB, backendPID int) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		blocked, err := backendHasBlockers(ctx, db, backendPID)
		if err != nil {
			t.Fatalf("inspect blocked backend %d: %v", backendPID, err)
		}
		if blocked {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("backend %d did not enter a lock wait: %v", backendPID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func backendHasBlockers(ctx context.Context, db *sql.DB, backendPID int) (bool, error) {
	query, args, err := goqu.Dialect("postgres").
		Select(goqu.Func("cardinality", goqu.Func("pg_blocking_pids", backendPID))).
		Prepared(true).
		ToSQL()
	if err != nil {
		return false, err
	}
	var blockerCount int
	err = db.QueryRowContext(ctx, query, args...).Scan(&blockerCount)
	return blockerCount > 0, err
}

func awaitCleanupResult(ctx context.Context, t *testing.T, result <-chan referenceCleanupResult) referenceCleanupResult {
	t.Helper()
	select {
	case completed := <-result:
		return completed
	case <-ctx.Done():
		t.Fatalf("cleanup did not complete: %v", ctx.Err())
		return referenceCleanupResult{}
	}
}

func awaitWriterResult(ctx context.Context, t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		t.Fatalf("reference writer did not complete: %v", ctx.Err())
		return nil
	}
}

func assertSQLState(t *testing.T, err error, expected string) {
	t.Helper()
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != expected {
		t.Fatalf("PostgreSQL error = %v, want SQLSTATE %s", err, expected)
	}
}
