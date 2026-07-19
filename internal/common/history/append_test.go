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

package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestAppendVersionTxInsertsWithoutUpdatingPreviousRows(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeAPI, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityMinimal})
	})
	Configure(Config{Mode: ModeAPI, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityMinimal})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "row_hash" FROM "aas_history"`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO "aas_history_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeCreated, map[string]any{"id": "aas-1"}, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxSnapshotIntervalOneUsesPreviousHashOnly(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeAPI, FullSnapshotInterval: 1, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "row_hash" FROM "aas_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"row_hash"}).AddRow("previous"))
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(2))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"snapshot"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeUpdated, map[string]any{"id": "aas-1", "counter": json.Number("9007199254740993")}, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxWritesEvidenceArtifactBeforeCommitWhenEnabled(t *testing.T) {
	store := &recordingEvidenceStore{}
	matchedRuleID := "rule:1:abcdef0123456789,rule:3:0123456789abcdef"
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{
		Mode:                 ModeAPI,
		FullSnapshotInterval: 1,
		Immutability:         ImmutabilityNone,
		AuditIdentityMode:    AuditIdentityNone,
		EvidenceEnabled:      true,
		EvidenceProvider:     EvidenceProviderS3,
		EvidenceStore:        store,
	})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(mutationEvidenceStateQuery).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history"`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO "aas_history".*"matched_rule_id".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"snapshot"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "history_id", "row_hash" FROM "aas_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id", "row_hash"}).AddRow(1, "history-row-hash"))
	expectMutationEvidenceCatalogInsert(mock, 1, false)
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	ctx := ContextWithAudit(context.Background(), AuditContext{
		AuthorizationResult: "ALLOW",
		PolicyID:            "policy-1",
		MatchedRuleID:       matchedRuleID,
	})
	err = AppendVersionTx(ctx, tx, TableAAS, "aas-1", ChangeCreated, map[string]any{"id": "aas-1"}, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	require.Len(t, store.artifacts, 1)
	artifact := store.artifacts[0]
	require.Equal(t, EvidenceArtifactMutation, artifact.ArtifactType)
	require.Contains(t, artifact.ObjectKey, "mutation-events/aas_history/aas-1/1-")
	var artifactPayload map[string]any
	require.NoError(t, decodeJSONPreservingNumbers(artifact.Data, &artifactPayload))
	require.Equal(t, PayloadTypeSnapshot, artifactPayload["payload_type"])
	require.Equal(t, "aas-1", artifactPayload["identifier"])
	require.Equal(t, json.Number("1"), artifactPayload["event_sequence"])
	require.Equal(t, mutationEventArtifactVersion, artifactPayload["artifact_version"])
	payload, ok := artifactPayload["payload"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "aas-1", payload["id"])
	effectiveDiff, ok := artifactPayload["effective_diff"].([]any)
	require.True(t, ok)
	require.Len(t, effectiveDiff, 1)
	operation, ok := effectiveDiff[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "add", operation["op"])
	require.Equal(t, "/id", operation["path"])
	require.Equal(t, "aas-1", operation["value"])
	audit, ok := artifactPayload["audit"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, matchedRuleID, audit["matched_rule_id"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxWritesEvidenceWhenHistoryIsOff(t *testing.T) {
	store := &recordingEvidenceStore{}
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{
		Mode: ModeOff, FullSnapshotInterval: 3, Immutability: ImmutabilityNone,
		AuditIdentityMode: AuditIdentityNone, EvidenceEnabled: true,
		EvidenceProvider: EvidenceProviderS3, EvidenceStore: store,
	})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(mutationEvidenceStateQuery).
		WillReturnError(sql.ErrNoRows)
	expectMutationEvidenceCatalogInsert(mock, 1, false)
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, AppendVersionTx(t.Context(), tx, TableAAS, "aas-1", ChangeCreated, map[string]any{"id": "aas-1"}, false))
	require.NoError(t, tx.Commit())
	require.Len(t, store.artifacts, 1)
	require.Equal(t, EvidenceArtifactMutation, store.artifacts[0].ArtifactType)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMutationEvidenceContentDoesNotDependOnHistoryLink(t *testing.T) {
	fixedTime := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	baseWrite := mutationEvidenceWrite{
		table: TableAAS, identifier: "aas-1", changeType: ChangeUpdated,
		snapshot:      map[string]any{"id": "aas-1", "idShort": "updated"},
		payload:       historyPayload{payloadType: PayloadTypeSnapshot, json: []byte(`{"id":"aas-1","idShort":"updated"}`), hash: strings.Repeat("p", 64)},
		effectiveDiff: []map[string]any{{"op": "replace", "path": "/idShort", "value": "updated"}},
		previousHash:  strings.Repeat("e", 64), sequence: 2, now: fixedTime,
	}

	artifacts := make([]EvidenceArtifact, 0, 2)
	for _, historyID := range []int64{0, 42} {
		store := &recordingEvidenceStore{}
		cfg := Config{EvidenceEnabled: true, EvidenceStore: store, EvidenceWriteTimeout: time.Second}
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "mutation_evidence_artifacts"`).
			WillReturnRows(sqlmock.NewRows([]string{"artifact_id"}).AddRow(1))
		mock.ExpectExec(`INSERT INTO "mutation_evidence_state"`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectRollback()
		tx, beginErr := db.Begin()
		require.NoError(t, beginErr)
		write := baseWrite
		write.historyID = historyID
		write.historyRowHash = strings.Repeat("h", 64)
		_, err = publishMutationEvidenceTx(t.Context(), tx, cfg, write)
		require.NoError(t, err)
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
		artifacts = append(artifacts, store.artifacts[0])
		_ = db.Close()
	}
	require.Equal(t, artifacts[0].Data, artifacts[1].Data)
}

func TestMutationEvidenceHashCommitsBinaryDescriptor(t *testing.T) {
	fixedTime := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	write := mutationEvidenceWrite{
		table: TableSubmodel, identifier: "sm-1", changeType: ChangeUpdated,
		snapshot: map[string]any{"id": "sm-1", "value": "/aasx/files/token/manual.pdf"},
		payload: historyPayload{
			payloadType: PayloadTypeSnapshot,
			json:        []byte(`{"id":"sm-1","value":"/aasx/files/token/manual.pdf"}`),
			hash:        strings.Repeat("p", 64),
		},
		sequence: 2, previousHash: strings.Repeat("e", 64), now: fixedTime,
	}

	artifacts := make([]EvidenceArtifact, 0, 2)
	for _, digest := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
		store := &recordingEvidenceStore{}
		cfg := Config{EvidenceEnabled: true, EvidenceStore: store, EvidenceWriteTimeout: time.Second}
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "mutation_evidence_artifacts"`).
			WillReturnRows(sqlmock.NewRows([]string{"artifact_id"}).AddRow(1))
		mock.ExpectExec(`INSERT INTO "mutation_evidence_state"`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectRollback()
		tx, beginErr := db.Begin()
		require.NoError(t, beginErr)
		expectation := BinaryReferenceExpectation{
			ModelPath: "/aasx/files/token/manual.pdf", SHA256: digest, SizeBytes: 12,
			FileName: "manual.pdf", ContentType: "application/pdf",
			BinaryReference: EvidenceReference{
				Provider: EvidenceProviderS3, Bucket: "history-evidence",
				ObjectKey: "binary-content/" + digest, VersionID: "version-1",
			},
		}
		ctx := WithBinaryReferenceExpected(t.Context(), expectation)
		_, err = publishMutationEvidenceTx(ctx, tx, cfg, write)
		require.NoError(t, err)
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
		artifacts = append(artifacts, store.artifacts[0])
		_ = db.Close()
	}

	require.NotEqual(t, artifacts[0].Data, artifacts[1].Data)
}

func TestAppendVersionTxRollsBackWhenEvidenceStoreFails(t *testing.T) {
	store := &recordingEvidenceStore{err: errors.New("object storage unavailable")}
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{
		Mode:                 ModeAPI,
		FullSnapshotInterval: 1,
		Immutability:         ImmutabilityNone,
		AuditIdentityMode:    AuditIdentityNone,
		EvidenceEnabled:      true,
		EvidenceProvider:     EvidenceProviderS3,
		EvidenceStore:        store,
	})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(mutationEvidenceStateQuery).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history"`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"snapshot"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "history_id", "row_hash" FROM "aas_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id", "row_hash"}).AddRow(1, "history-row-hash"))
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeCreated, map[string]any{"id": "aas-1"}, false)
	require.ErrorContains(t, err, "HISTORY-EVIDENCE-MUTATION-PUT")
	require.ErrorContains(t, err, "503 Service Unavailable")
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxRollsBackWhenEvidenceReceiptCatalogFails(t *testing.T) {
	store := &recordingEvidenceStore{}
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{
		Mode:                 ModeAPI,
		FullSnapshotInterval: 1,
		Immutability:         ImmutabilityNone,
		AuditIdentityMode:    AuditIdentityNone,
		EvidenceEnabled:      true,
		EvidenceProvider:     EvidenceProviderS3,
		EvidenceStore:        store,
	})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(mutationEvidenceStateQuery).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history"`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"snapshot"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "history_id", "row_hash" FROM "aas_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id", "row_hash"}).AddRow(1, "history-row-hash"))
	mock.ExpectQuery(`INSERT INTO "mutation_evidence_artifacts"`).
		WillReturnError(errors.New("catalog insert failed"))
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeCreated, map[string]any{"id": "aas-1"}, false)
	require.ErrorContains(t, err, "HISTORY-EVIDENCE-MUTATION-CATALOGINSERT")
	require.NoError(t, tx.Rollback())
	require.Len(t, store.artifacts, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildLockIdentifierQueryUsesPostgresPlaceholders(t *testing.T) {
	query, args, err := buildLockIdentifierQuery(TableAAS, "aas-1")

	require.NoError(t, err)
	require.Equal(t, "SELECT pg_advisory_xact_lock(hashtextextended($1, $2))", query)
	require.Equal(t, []any{"aas_history:aas-1", int64(0)}, args)
}

func TestAppendMutatedVersionTxUsesLatestSnapshotAndRowHash(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeAPI, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityMinimal})
	})
	Configure(Config{Mode: ModeAPI, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityMinimal})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	latestSnapshot := map[string]any{"id": "sm-1", "submodelElements": []any{}}
	expectLatestSnapshotRestore(mock, TableSubmodel, "submodel_history_payload", "sm-1", 7, latestSnapshot, false)
	mock.ExpectQuery(`INSERT INTO "submodel_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO "submodel_history_payload"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendMutatedVersionTx(context.Background(), tx, TableSubmodel, "sm-1", ChangeUpdated, func(snapshot map[string]any) error {
		snapshot["idShort"] = "updated"
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendMutatedVersionTxStoresDiffFromOriginalSnapshot(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeAPI, FullSnapshotInterval: 3, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	largeUnchangedValue := strings.Repeat("unchanged-", 40)
	latestSnapshot := map[string]any{"id": "sm-1", "idShort": "before", "description": largeUnchangedValue}

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectLatestSnapshotRestore(mock, TableSubmodel, "submodel_history_payload", "sm-1", 7, latestSnapshot, false)
	mock.ExpectQuery(`INSERT INTO "submodel_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(8))
	mock.ExpectExec(`INSERT INTO "submodel_history_payload".*"diff".*/idShort.*after`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendMutatedVersionTx(context.Background(), tx, TableSubmodel, "sm-1", ChangeUpdated, func(snapshot map[string]any) error {
		snapshot["idShort"] = "after"
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxStoresDiffWhenIntervalAllows(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeAPI, FullSnapshotInterval: 3, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	largeUnchangedValue := strings.Repeat("unchanged-", 40)
	baseSnapshot := map[string]any{"id": "aas-1", "idShort": "before", "description": largeUnchangedValue}

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectLatestSnapshotRestore(mock, TableAAS, "aas_history_payload", "aas-1", 1, baseSnapshot, false)
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(2))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"diff"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeUpdated, map[string]any{"id": "aas-1", "idShort": "after", "description": largeUnchangedValue}, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxWritesDiffEvidenceArtifactWhenDiffPayloadIsSelected(t *testing.T) {
	store := &recordingEvidenceStore{}
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{
		Mode:                 ModeAPI,
		FullSnapshotInterval: 3,
		Immutability:         ImmutabilityNone,
		AuditIdentityMode:    AuditIdentityNone,
		EvidenceEnabled:      true,
		EvidenceProvider:     EvidenceProviderS3,
		EvidenceStore:        store,
	})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	largeUnchangedValue := strings.Repeat("unchanged-", 40)
	baseSnapshot := map[string]any{"id": "aas-1", "idShort": "before", "description": largeUnchangedValue}
	seed := seedMutationEvidenceStore(t, store, TableAAS, "aas-1", 1, baseSnapshot)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectMutationEvidenceState(mock, seed, 0, baseSnapshot)
	expectLatestSnapshotRestore(mock, TableAAS, "aas_history_payload", "aas-1", 1, baseSnapshot, false)
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(2))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"diff"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT "history_id", "row_hash" FROM "aas_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id", "row_hash"}).AddRow(2, "history-row-hash-2"))
	expectMutationEvidenceCatalogInsert(mock, 2, true)
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeUpdated, map[string]any{"id": "aas-1", "idShort": "after", "description": largeUnchangedValue}, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	require.Len(t, store.artifacts, 2)
	var artifactPayload map[string]any
	require.NoError(t, decodeJSONPreservingNumbers(store.artifacts[1].Data, &artifactPayload))
	require.Equal(t, PayloadTypeDiff, artifactPayload["payload_type"])
	diff, ok := artifactPayload["payload"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, diff)
	effectiveDiff, ok := artifactPayload["effective_diff"].([]any)
	require.True(t, ok)
	require.Equal(t, diff, effectiveDiff)
	require.Zero(t, store.getCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxSnapshotEvidenceIncludesEffectiveDiffOnlyForChangedFields(t *testing.T) {
	store := &recordingEvidenceStore{}
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{
		Mode:                 ModeAPI,
		FullSnapshotInterval: DefaultFullSnapshotInterval,
		Immutability:         ImmutabilityNone,
		AuditIdentityMode:    AuditIdentityNone,
		EvidenceEnabled:      true,
		EvidenceProvider:     EvidenceProviderS3,
		EvidenceStore:        store,
	})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	largeUnchangedValue := strings.Repeat("unchanged-", 40)
	baseSnapshot := map[string]any{"id": "aas-1", "idShort": "before", "description": largeUnchangedValue}
	targetSnapshot := map[string]any{"id": "aas-1", "idShort": "after", "description": largeUnchangedValue}
	seed := seedMutationEvidenceStore(t, store, TableAAS, "aas-1", 1, baseSnapshot)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectMutationEvidenceState(mock, seed, 0, baseSnapshot)
	expectLatestSnapshotRestore(mock, TableAAS, "aas_history_payload", "aas-1", 1, baseSnapshot, false)
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(2))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"snapshot"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT "history_id", "row_hash" FROM "aas_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id", "row_hash"}).AddRow(2, "history-row-hash-2"))
	expectMutationEvidenceCatalogInsert(mock, 2, false)
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeUpdated, targetSnapshot, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	require.Len(t, store.artifacts, 2)
	var artifactPayload map[string]any
	require.NoError(t, decodeJSONPreservingNumbers(store.artifacts[1].Data, &artifactPayload))
	require.Equal(t, PayloadTypeSnapshot, artifactPayload["payload_type"])
	effectiveDiff, ok := artifactPayload["effective_diff"].([]any)
	require.True(t, ok)
	require.Len(t, effectiveDiff, 1)
	operation, ok := effectiveDiff[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "replace", operation["op"])
	require.Equal(t, "/idShort", operation["path"])
	require.Equal(t, "after", operation["value"])
	require.Zero(t, store.getCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxStoresSnapshotWhenDiffWouldBeLarger(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeAPI, FullSnapshotInterval: 3, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	baseSnapshot := map[string]any{"items": []any{"a", "b", "c", "d", "e", "f"}}
	targetSnapshot := map[string]any{"items": []any{"f", "e", "d", "c", "b", "a"}}

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectLatestSnapshotRestore(mock, TableAAS, "aas_history_payload", "aas-1", 1, baseSnapshot, false)
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(2))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"snapshot"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeUpdated, targetSnapshot, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildHistoryPayloadFallsBackToSnapshotWhenDiffIsNotSmaller(t *testing.T) {
	baseSnapshot := map[string]any{"items": []any{"a", "b", "c", "d", "e", "f"}}
	targetSnapshot := map[string]any{"items": []any{"f", "e", "d", "c", "b", "a"}}
	latest := &latestVersion{
		snapshot:          baseSnapshot,
		rowsSinceSnapshot: 1,
	}

	payload, err := buildHistoryPayload(targetSnapshot, latest, Config{FullSnapshotInterval: 3})
	require.NoError(t, err)

	require.Equal(t, PayloadTypeSnapshot, payload.payloadType)
	expectedHash, err := CanonicalJSONHash(targetSnapshot)
	require.NoError(t, err)
	require.Equal(t, expectedHash, payload.hash)
}

func TestBuildHistoryPayloadHashesSelectedDiffPayload(t *testing.T) {
	largeUnchangedValue := strings.Repeat("unchanged-", 40)
	baseSnapshot := map[string]any{"id": "aas-1", "idShort": "before", "description": largeUnchangedValue}
	targetSnapshot := map[string]any{"id": "aas-1", "idShort": "after", "description": largeUnchangedValue}
	latest := &latestVersion{
		snapshot:          baseSnapshot,
		rowsSinceSnapshot: 1,
	}

	payload, err := buildHistoryPayload(targetSnapshot, latest, Config{FullSnapshotInterval: 3})
	require.NoError(t, err)

	patch, err := BuildJSONPatch(baseSnapshot, targetSnapshot)
	require.NoError(t, err)
	expectedHash, err := CanonicalJSONHash(patch)
	require.NoError(t, err)
	require.Equal(t, PayloadTypeDiff, payload.payloadType)
	require.Equal(t, expectedHash, payload.hash)
}

func TestAppendVersionTxStoresScheduledSnapshotAtIntervalBoundary(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeAPI, FullSnapshotInterval: 3, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	checkpoint := map[string]any{"id": "aas-1", "idShort": "v1"}
	v2 := map[string]any{"id": "aas-1", "idShort": "v2"}
	v3 := map[string]any{"id": "aas-1", "idShort": "v3"}
	patch12, err := BuildJSONPatch(checkpoint, v2)
	require.NoError(t, err)
	patch23, err := BuildJSONPatch(v2, v3)
	require.NoError(t, err)
	operationTime := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(3)))
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history".*"payload_type" = 'snapshot'`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history" INNER JOIN "aas_history_payload" AS "payload"`).
		WillReturnRows(newHistoryChainRows(TableAAS,
			historyChainRowSpec{HistoryID: 1, Identifier: "aas-1", ChangeType: ChangeCreated, PayloadType: PayloadTypeSnapshot, Snapshot: checkpoint, Deleted: false, OperationTime: operationTime},
			historyChainRowSpec{HistoryID: 2, Identifier: "aas-1", ChangeType: ChangeUpdated, PayloadType: PayloadTypeDiff, Patch: patch12, ContentSnapshot: v2, Deleted: false, OperationTime: operationTime},
			historyChainRowSpec{HistoryID: 3, Identifier: "aas-1", ChangeType: ChangeUpdated, PayloadType: PayloadTypeDiff, Patch: patch23, ContentSnapshot: v3, Deleted: false, OperationTime: operationTime},
		))
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(4))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"snapshot"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeUpdated, map[string]any{"id": "aas-1", "idShort": "v4"}, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxSkipsWritesWhenHistoryModeOff(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeAPI, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityMinimal})
	})
	Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityMinimal})

	err := AppendVersionTx(context.Background(), nil, TableAAS, "aas-1", ChangeCreated, map[string]any{"id": "aas-1"}, false)
	require.NoError(t, err)
}

func TestNullableTimestampAcceptsSharedISO8601Formats(t *testing.T) {
	offsetTimestamp, ok := nullableTimestamp("2026-05-28T14:30:00+0200").(time.Time)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 5, 28, 12, 30, 0, 0, time.UTC), offsetTimestamp)

	utcTimestamp, ok := nullableTimestamp("2026-05-28T12:30:00.123456789 UTC").(time.Time)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 5, 28, 12, 30, 0, 123456789, time.UTC), utcTimestamp)
}

type recordingEvidenceStore struct {
	err       error
	artifacts []EvidenceArtifact
	getCount  int
}

const mutationEvidenceStateQuery = `SELECT "last_sequence", "last_event_hash", "last_content_hash", "events_since_snapshot", "current_snapshot" FROM "mutation_evidence_state"`

type seededMutationEvidence struct {
	sequence    int64
	eventHash   string
	contentHash string
	objectKey   string
	sha256      string
}

func seedMutationEvidenceStore(t *testing.T, store *recordingEvidenceStore, table string, identifier string, sequence int64, snapshot map[string]any) seededMutationEvidence {
	t.Helper()
	contentHash, err := CanonicalJSONHash(snapshot)
	require.NoError(t, err)
	eventHash := strings.Repeat("e", 64)
	body, err := CanonicalJSON(map[string]any{
		"artifact_version":    mutationEventArtifactVersion,
		"event_sequence":      sequence,
		"previous_event_hash": "",
		"payload_type":        PayloadTypeSnapshot,
		"payload":             snapshot,
		"content_hash":        contentHash,
		"event_hash":          eventHash,
	})
	require.NoError(t, err)
	objectKey := fmt.Sprintf("mutation-events/%s/%s/%d-%s.json", table, identifier, sequence, eventHash)
	store.artifacts = append(store.artifacts, EvidenceArtifact{
		ArtifactType: EvidenceArtifactMutation,
		ObjectKey:    objectKey,
		ContentType:  manifestJSONContentType,
		Data:         body,
	})
	return seededMutationEvidence{sequence: sequence, eventHash: eventHash, contentHash: contentHash, objectKey: objectKey, sha256: SHA256Hex(body)}
}

func expectMutationEvidenceState(mock sqlmock.Sqlmock, seed seededMutationEvidence, eventsSinceSnapshot int, snapshot map[string]any) {
	snapshotJSON, err := CanonicalJSON(snapshot)
	if err != nil {
		panic(err)
	}
	mock.ExpectQuery(mutationEvidenceStateQuery).
		WillReturnRows(sqlmock.NewRows([]string{"last_sequence", "last_event_hash", "last_content_hash", "events_since_snapshot", "current_snapshot"}).
			AddRow(seed.sequence, seed.eventHash, seed.contentHash, eventsSinceSnapshot, snapshotJSON))
}

func expectMutationEvidenceCatalogInsert(mock sqlmock.Sqlmock, artifactID int64, _ bool) {
	mock.ExpectQuery(`INSERT INTO "mutation_evidence_artifacts"`).
		WillReturnRows(sqlmock.NewRows([]string{"artifact_id"}).AddRow(artifactID))
	mock.ExpectExec(`INSERT INTO "mutation_evidence_state"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func (store *recordingEvidenceStore) PutArtifact(_ context.Context, artifact EvidenceArtifact) (*EvidenceReceipt, error) {
	if store.err != nil {
		return nil, store.err
	}
	store.artifacts = append(store.artifacts, artifact)
	return &EvidenceReceipt{
		Reference: EvidenceReference{
			Provider:  EvidenceProviderS3,
			Bucket:    "history-evidence",
			ObjectKey: artifact.ObjectKey,
			VersionID: "version-1",
		},
		SHA256:        SHA256Hex(artifact.Data),
		SizeBytes:     int64(len(artifact.Data)),
		ContentType:   artifact.ContentType,
		RetentionMode: "governance",
		RetainUntil: func() *time.Time {
			retainUntil := time.Now().UTC().Add(24 * time.Hour)
			return &retainUntil
		}(),
		StoredAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
		Metadata: artifact.Metadata,
	}, nil
}

func (store *recordingEvidenceStore) GetArtifact(_ context.Context, ref EvidenceReference) (*EvidenceObject, error) {
	store.getCount++
	for _, artifact := range store.artifacts {
		if artifact.ObjectKey == ref.ObjectKey {
			return &EvidenceObject{Reference: ref, Data: artifact.Data, ContentType: artifact.ContentType, Metadata: artifact.Metadata}, nil
		}
	}
	return nil, errors.New("artifact not found")
}

func (store *recordingEvidenceStore) VerifyArtifact(_ context.Context, _ EvidenceReference, _ string) (*EvidenceReceipt, error) {
	return nil, errors.New("not implemented")
}
