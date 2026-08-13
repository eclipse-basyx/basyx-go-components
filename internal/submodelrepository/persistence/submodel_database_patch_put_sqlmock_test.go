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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestPatchSubmodelIDMismatchReturnsBadRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-body")

	err = sut.PatchSubmodel(contextWithABACDisabled(t), "sm-path", submodel)
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPatchSubmodelNotFoundRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-missing")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	err = sut.PatchSubmodel(contextWithABACDisabled(t), "sm-missing", submodel)
	require.Error(t, err)
	require.True(t, common.IsErrNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPatchSubmodelSuccessReplacesSubmodel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-1")
	idShort := "sm1"
	submodel.SetIDShort(&idShort)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(100))
	expectNoManagedFileReferences(mock)
	mock.ExpectQuery(`SELECT .*file_oid.*FROM .*submodel_element.*file_data`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectExec(`DELETE FROM .*submodel`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO .*submodel.*RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(200))
	mock.ExpectExec(`INSERT INTO .*submodel_payload`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectCurrentSubmodelSnapshotLoad(mock, "sm-1", "sm1")
	mock.ExpectCommit()

	err = sut.PatchSubmodel(contextWithABACDisabled(t), "sm-1", submodel)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPatchSubmodelInTransactionAppendsHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-1")
	idShort := "sm1"
	submodel.SetIDShort(&idShort)

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(100))
	expectNoManagedFileReferences(mock)
	mock.ExpectQuery(`SELECT .*file_oid.*FROM .*submodel_element.*file_data`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectExec(`DELETE FROM .*submodel`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO .*submodel.*RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(200))
	mock.ExpectExec(`INSERT INTO .*submodel_payload`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectCurrentSubmodelSnapshotLoad(mock, "sm-1", "sm1")
	mock.ExpectRollback()

	err = sut.PatchSubmodelInTransaction(contextWithABACDisabled(t), "sm-1", tx, submodel)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPatchSubmodelMetadataInTransactionAppendsHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-1")
	idShort := "sm1"
	submodel.SetIDShort(&idShort)

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(100))
	mock.ExpectExec(`UPDATE "submodel"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "submodel_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "submodel_supplemental_semantic_id_reference"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "submodel_semantic_id_reference"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectExistingSubmodelHistorySnapshot(mock, "sm-1", `{"id":"sm-1","submodelElements":[{"idShort":"existing","modelType":"Capability"}]}`, false)
	mock.ExpectQuery(`INSERT INTO "submodel_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO "submodel_history_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	err = sut.PatchSubmodelMetadataInTransaction(contextWithABACDisabled(t), "sm-1", tx, submodel)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutSubmodelIDMismatchReturnsBadRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-body")

	isUpdate, err := sut.PutSubmodel(contextWithABACDisabled(t), "sm-path", submodel)
	require.Error(t, err)
	require.False(t, isUpdate)
	require.True(t, common.IsErrBadRequest(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutSubmodelCreatePathReturnsFalse(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-new")
	idShort := "smnew"
	submodel.SetIDShort(&idShort)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO .*submodel.*RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(300))
	mock.ExpectExec(`INSERT INTO .*submodel_payload`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectCreatedPutSubmodelSnapshotLoad(mock, "sm-new", "smnew")
	mock.ExpectCommit()

	isUpdate, err := sut.PutSubmodel(contextWithABACDisabled(t), "sm-new", submodel)
	require.NoError(t, err)
	require.False(t, isUpdate)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutSubmodelNoOpUpdateAppendsHistoryWithoutLiveMutation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-existing")
	idShort := "smexisting"
	submodel.SetIDShort(&idShort)

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(400))
	expectBareSubmodelStateLoad(mock, "sm-existing", "smexisting")
	expectSubmodelHistoryAppend(mock)
	mock.ExpectRollback()

	result, err := sut.PutSubmodelInTransactionWithResult(contextWithABACDisabled(t), tx, "sm-existing", submodel)
	require.NoError(t, err)
	require.True(t, result.IsUpdate)
	require.False(t, result.Changed)
	require.NotNil(t, result.Previous)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutSubmodelChangedUpdateExecutesReconciliationAndHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-existing")
	idShort := "new"
	submodel.SetIDShort(&idShort)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(400))
	expectBareSubmodelStateLoad(mock, "sm-existing", "old")
	mock.ExpectQuery(`WITH reconciliation_plan`).
		WillReturnRows(sqlmock.NewRows([]string{"updated_count", "inserted_count", "deleted_count"}).AddRow(0, 0, 0))
	expectBareSubmodelStateLoad(mock, "sm-existing", "new")
	expectSubmodelHistoryAppend(mock)
	mock.ExpectCommit()

	isUpdate, err := sut.PutSubmodel(contextWithABACDisabled(t), "sm-existing", submodel)
	require.NoError(t, err)
	require.True(t, isUpdate)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutSubmodelUpdateFormulaDenialRollsBackBeforeDiff(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-existing")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(400))
	mock.ExpectQuery(`SELECT .*FROM "submodel".*submodel_payload.*`).
		WillReturnRows(sqlmock.NewRows(submodelStateColumns()))
	mock.ExpectRollback()

	_, err = sut.PutSubmodel(contextWithUpdateFormula(t, false), "sm-existing", submodel)
	require.Error(t, err)
	require.Truef(t, common.IsErrDenied(err), "got %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutSubmodelPostUpdateFormulaDenialRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-existing")
	idShort := "smexisting"
	submodel.SetIDShort(&idShort)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(400))
	expectBareSubmodelStateLoad(mock, "sm-existing", idShort)
	mock.ExpectQuery(`SELECT .*FROM "submodel".*submodel_payload.*`).
		WillReturnRows(sqlmock.NewRows(submodelStateColumns()))
	mock.ExpectRollback()

	_, err = sut.PutSubmodel(contextWithUpdateFormula(t, true), "sm-existing", submodel)
	require.Error(t, err)
	require.Truef(t, common.IsErrDenied(err), "got %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func contextWithUpdateFormula(t *testing.T, allowed bool) context.Context {
	t.Helper()
	expression := grammar.LogicalExpression{Boolean: &allowed}
	return auth.WithQueryFilter(contextWithABACDisabled(t), &auth.QueryFilter{
		Formula: &expression,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumUPDATE: expression,
		},
	})
}

func expectSubmodelHistoryAppend(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "row_hash" FROM "submodel_history"`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO "submodel_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO "submodel_history_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectNoManagedFileReferences(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT .*fr.*binary_content_id.*FROM "submodel_element" AS "sme".*file_binary_reference`).
		WillReturnRows(sqlmock.NewRows([]string{"idshort_path", "value", "binary_content_id", "path_token", "safe_file_name"}))
}

func expectMissingSubmodelHistory(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT "history_id" FROM "submodel_history"`).
		WillReturnError(sql.ErrNoRows)
}

func expectExistingSubmodelHistorySnapshot(mock sqlmock.Sqlmock, identifier string, snapshotJSON string, deleted bool) {
	historyID := int64(1)
	operationTime := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	hash := mustSubmodelHistoryHash(snapshotJSON)
	rowHash := mustSubmodelHistoryRowHash(history.ChangeEvent{
		EntityType:   history.TableSubmodel,
		Identifier:   identifier,
		ChangeType:   history.ChangeUpdated,
		Timestamp:    operationTime,
		Deleted:      deleted,
		PayloadType:  history.PayloadTypeSnapshot,
		ContentHash:  hash,
		PayloadHash:  hash,
		PreviousHash: "",
	})
	mock.ExpectQuery(`SELECT "history_id" FROM "submodel_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(historyID))
	mock.ExpectQuery(`SELECT "history_id" FROM "submodel_history".*"payload_type" = 'snapshot'`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(historyID))
	mock.ExpectQuery(`SELECT .*FROM "submodel_history" AS "history" INNER JOIN "submodel_history_payload" AS "payload"`).
		WillReturnRows(sqlmock.NewRows(submodelHistoryChainColumns()).
			AddRow(
				historyID,
				identifier,
				history.ChangeUpdated,
				history.PayloadTypeSnapshot,
				snapshotJSON,
				nil,
				deleted,
				nil,
				nil,
				operationTime,
				hash,
				hash,
				nil,
				rowHash,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
			))
}

func submodelHistoryChainColumns() []string {
	return []string{
		"history_id",
		"identifier",
		"change_type",
		"payload_type",
		"snapshot",
		"diff",
		"deleted",
		"administration_created_at_text",
		"administration_updated_at_text",
		"operation_time",
		"content_hash",
		"payload_hash",
		"previous_hash",
		"row_hash",
		"request_id",
		"correlation_id",
		"actor_subject",
		"actor_issuer",
		"client_id",
		"authorization_result",
		"policy_id",
		"matched_rule_id",
		"source_ip",
		"user_agent",
		"operation",
		"endpoint",
		"http_method",
	}
}

func mustSubmodelHistoryHash(snapshotJSON string) string {
	var snapshot any
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		panic(err)
	}
	hash, err := history.CanonicalJSONHash(snapshot)
	if err != nil {
		panic(err)
	}
	return hash
}

func mustSubmodelHistoryRowHash(event history.ChangeEvent) string {
	hash, err := history.ComputeHistoryRowHash(event)
	if err != nil {
		panic(err)
	}
	return hash
}

func expectSubmodelHistoryAppendWithReadFailure(mock sqlmock.Sqlmock, err error) {
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "history_id" FROM "submodel_history"`).
		WillReturnError(err)
}

func expectMutatedSubmodelHistoryFallback(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectMissingSubmodelHistory(mock)
}

func expectCurrentSubmodelSnapshotLoad(mock sqlmock.Sqlmock, submodelID string, idShort string) {
	expectSubmodelStateLoad(mock, submodelID, idShort, "[]", "[]", "{}", "[]", "[]", "[]", "[]", "{}")
	expectSubmodelHistoryAppend(mock)
}

func expectCreatedPutSubmodelSnapshotLoad(mock sqlmock.Sqlmock, submodelID string, idShort string) {
	expectSubmodelDatabaseIDLoad(mock)
	expectSubmodelMetadataStateLoad(mock, submodelID, idShort, "[]", "[]", "{}", "[]", "[]", "[]", "[]", "{}")
	expectEmptyRootSubmodelElementsLoad(mock)
	expectSubmodelHistoryAppend(mock)
}

func expectBareSubmodelStateLoad(mock sqlmock.Sqlmock, submodelID string, idShort string) {
	expectSubmodelMetadataStateLoad(mock, submodelID, idShort, nil, nil, nil, nil, nil, nil, nil, nil)
	expectEmptyRootSubmodelElementsLoad(mock)
}

func expectSubmodelStateLoad(mock sqlmock.Sqlmock, submodelID string, idShort string, payloads ...any) {
	expectSubmodelMetadataStateLoad(mock, submodelID, idShort, payloads...)
	expectSubmodelDatabaseIDLoad(mock)
	expectEmptyRootSubmodelElementsLoad(mock)
}

func expectSubmodelMetadataStateLoad(mock sqlmock.Sqlmock, submodelID string, idShort string, payloads ...any) {
	mock.ExpectQuery(`SELECT .*FROM "submodel".*submodel_payload.*`).
		WillReturnRows(sqlmock.NewRows(submodelStateColumns()).AddRow(submodelID, idShort, nil, nil, payloads[0], payloads[1], payloads[2], payloads[3], payloads[4], payloads[5], payloads[6], payloads[7], submodelID))
}

func expectSubmodelDatabaseIDLoad(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT .*FROM "submodel"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(100))
}

func expectEmptyRootSubmodelElementsLoad(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT .*FROM "submodel_element"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "idshort_path"}))
}

func submodelStateColumns() []string {
	return []string{
		"submodel_identifier",
		"id_short",
		"category",
		"kind",
		"description_payload",
		"displayname_payload",
		"administrative_information_payload",
		"embedded_data_specification_payload",
		"supplemental_semantic_ids_payload",
		"extensions_payload",
		"qualifiers_payload",
		"semantic_id_payload",
		"sort_submodel_identifier",
	}
}

func TestPatchSubmodelElementByPathNotFoundReturnsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	patchElement := types.NewCapability()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(100))
	mock.ExpectQuery(`SELECT .*model_type.*FROM .*submodel_element`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	err = sut.UpdateSubmodelElement(contextWithABACDisabled(t), "sm-1", "does.not.exist", patchElement, false)
	require.Error(t, err)
	require.True(t, common.IsErrNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPatchSubmodelElementByPathSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	patchElement := types.NewCapability()
	patchedIDShort := "renamedButIgnoredForPatch"
	patchElement.SetIDShort(&patchedIDShort)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(100))
	mock.ExpectQuery(`SELECT .*model_type.*FROM .*submodel_element`).
		WillReturnRows(sqlmock.NewRows([]string{"model_type"}).AddRow(types.ModelTypeCapability))
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(100))
	mock.ExpectQuery(`SELECT .*id.*,.*id_short.*FROM .*submodel_element`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "id_short"}).AddRow(200, "oldIdShort"))
	mock.ExpectExec(`UPDATE .*submodel_element`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO .*submodel_element_payload`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectMutatedSubmodelHistoryFallback(mock)
	expectCurrentSubmodelSnapshotLoad(mock, "sm-1", "sm1")
	mock.ExpectCommit()

	err = sut.UpdateSubmodelElement(contextWithABACDisabled(t), "sm-1", "oldIdShort", patchElement, false)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
