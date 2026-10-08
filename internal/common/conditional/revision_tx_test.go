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

package conditional

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

const (
	upsertPattern      = `INSERT INTO "resource_revision" .* ON CONFLICT \(kind, identifier\) DO UPDATE SET "revision"=nextval\('basyx_resource_revision_seq'\) RETURNING "kind", "identifier", "revision"`
	placeholderPattern = `INSERT INTO "resource_revision" .* ON CONFLICT DO NOTHING`
	lockPattern        = `SELECT "revision" FROM "resource_revision" WHERE .* FOR UPDATE`
	bumpPattern        = `UPDATE "resource_revision" SET "revision"=nextval\('basyx_resource_revision_seq'\) WHERE .* RETURNING "revision"`
	deletePattern      = `DELETE FROM "resource_revision" WHERE`
)

var (
	submodelA = Ref(KindSubmodel, "sm-a")
	submodelB = Ref(KindSubmodel, "sm-b")
	shellA    = Ref(KindAAS, "aas-a")
)

func newMockTx(t *testing.T) (*sql.Tx, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	return tx, mock
}

func requestContext(method string, target *ResourceRef, header http.Header, requireIfMatch bool) (context.Context, *State) {
	state := &State{method: method, mode: ModeResource, target: target, conds: parseConditions(header), requireIfMatch: requireIfMatch}
	return context.WithValue(context.Background(), stateContextKey{}, state), state
}

func TestUnconditionalWritesUpsertRevisionsInSortedOrder(t *testing.T) {
	tx, mock := newMockTx(t)
	ctx := context.Background()
	require.NoError(t, Touch(ctx, tx, submodelB, OpUpdate))
	require.NoError(t, Touch(ctx, tx, shellA, OpCreate))
	require.NoError(t, Touch(ctx, tx, submodelA, OpNoOp))

	mock.ExpectQuery(upsertPattern).
		WithArgs(shellA.Identifier, string(KindAAS), submodelB.Identifier, string(KindSubmodel)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "identifier", "revision"}).
			AddRow(string(KindAAS), shellA.Identifier, 11).
			AddRow(string(KindSubmodel), submodelB.Identifier, 12))

	flushed, err := FlushTx(tx)
	require.NoError(t, err)
	require.NotNil(t, flushed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestConditionalWriteLocksEvaluatesAndBumpsTheTarget(t *testing.T) {
	tx, mock := newMockTx(t)
	current := ConcurrencyETag(submodelA, 7)
	ctx, state := requestContext(http.MethodPatch, &submodelA, http.Header{"If-Match": {current}}, false)
	require.NoError(t, Touch(ctx, tx, submodelA, OpUpdate))

	mock.ExpectExec(placeholderPattern).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(lockPattern).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(7))
	mock.ExpectQuery(bumpPattern).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(9))

	flushed, err := FlushTx(tx)
	require.NoError(t, err)
	flushed.Committed()
	require.True(t, state.isEvaluated())
	require.Equal(t, ConcurrencyETag(submodelA, 9), state.currentWriteETag())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestConditionalWriteWithStaleTagFailsBeforeBumping(t *testing.T) {
	tx, mock := newMockTx(t)
	ctx, state := requestContext(http.MethodPut, &submodelA, http.Header{"If-Match": {ConcurrencyETag(submodelA, 6)}}, false)
	require.NoError(t, Touch(ctx, tx, submodelA, OpUpdate))

	mock.ExpectExec(placeholderPattern).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(lockPattern).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(7))

	_, err := FlushTx(tx)
	require.True(t, IsPreconditionFailed(err))
	require.True(t, IsPreconditionFailed(state.currentFailure()))
	require.False(t, state.isEvaluated())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestConditionalDeleteValidatesBeforeRemovingTheRevision(t *testing.T) {
	tx, mock := newMockTx(t)
	ctx, _ := requestContext(http.MethodDelete, &submodelA, http.Header{"If-Match": {ConcurrencyETag(submodelA, 7)}}, false)
	require.NoError(t, Touch(ctx, tx, submodelA, OpDelete))

	mock.ExpectExec(placeholderPattern).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(lockPattern).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(7))
	mock.ExpectExec(deletePattern).WillReturnResult(sqlmock.NewResult(0, 1))

	_, err := FlushTx(tx)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplaceKeepsThePreviousRevisionForEvaluation(t *testing.T) {
	tx, mock := newMockTx(t)
	company := Ref(KindCompanyDescriptor, "example.com")
	ctx, _ := requestContext(http.MethodPut, &company, http.Header{"If-Match": {ConcurrencyETag(company, 4)}}, false)
	require.NoError(t, Touch(ctx, tx, company, OpDelete))
	require.NoError(t, Touch(ctx, tx, company, OpCreate))

	mock.ExpectExec(placeholderPattern).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(lockPattern).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(4))
	mock.ExpectQuery(bumpPattern).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(5))

	_, err := FlushTx(tx)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateOnlyWriteFailsWhenARevisionAlreadyExists(t *testing.T) {
	tx, mock := newMockTx(t)
	ctx, _ := requestContext(http.MethodPut, &submodelA, http.Header{"If-None-Match": {"*"}}, false)
	require.NoError(t, Touch(ctx, tx, submodelA, OpCreate))

	mock.ExpectExec(placeholderPattern).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(lockPattern).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(3))

	_, err := FlushTx(tx)
	require.True(t, IsPreconditionFailed(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequireIfMatchAnswers428ForExistingResources(t *testing.T) {
	tx, mock := newMockTx(t)
	ctx, _ := requestContext(http.MethodPatch, &submodelA, http.Header{}, true)
	require.NoError(t, Touch(ctx, tx, submodelA, OpUpdate))

	mock.ExpectExec(placeholderPattern).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(lockPattern).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(3))

	_, err := FlushTx(tx)
	require.True(t, IsPreconditionRequired(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestConditionalTransactionWithoutTargetIsNotCommitted(t *testing.T) {
	tx, mock := newMockTx(t)
	ctx, _ := requestContext(http.MethodPut, &submodelA, http.Header{"If-Match": {"*"}}, false)
	require.NoError(t, Touch(ctx, tx, submodelB, OpUpdate))

	_, err := FlushTx(tx)
	require.ErrorIs(t, err, errNotEvaluated)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSecondTransactionChangingAConditionalTargetIsRejected(t *testing.T) {
	tx, mock := newMockTx(t)
	ctx, state := requestContext(http.MethodPut, &submodelA, http.Header{"If-Match": {"*"}}, false)
	state.recordCommit(true, "")
	require.NoError(t, Touch(ctx, tx, submodelA, OpUpdate))

	_, err := FlushTx(tx)
	require.ErrorIs(t, err, errMultipleTransactions)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCompositeTargetIsEvaluatedAgainstAllMembers(t *testing.T) {
	tx, mock := newMockTx(t)
	dpp := Ref(KindDPP, "dpp-1")
	members := []ResourceRef{shellA, submodelA}
	current := compositeValidator(dpp, members, map[ResourceRef]int64{shellA: 2, submodelA: 5})
	ctx, state := requestContext(http.MethodPatch, &dpp, http.Header{"If-Match": {quote(current)}}, false)
	TouchComposite(ctx, tx, dpp, members, members, OpUpdate)
	require.NoError(t, Touch(ctx, tx, submodelA, OpUpdate))

	mock.ExpectExec(placeholderPattern).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(lockPattern).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(2))
	mock.ExpectExec(placeholderPattern).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(lockPattern).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(5))
	mock.ExpectQuery(bumpPattern).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(8))

	flushed, err := FlushTx(tx)
	require.NoError(t, err)
	flushed.Committed()
	require.Equal(t, quote(compositeValidator(dpp, members, map[ResourceRef]int64{shellA: 2, submodelA: 8})), state.currentWriteETag())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDiscardDropsPendingChanges(t *testing.T) {
	tx, mock := newMockTx(t)
	require.NoError(t, Touch(context.Background(), tx, submodelA, OpUpdate))
	Discard(tx)

	flushed, err := FlushTx(tx)
	require.NoError(t, err)
	require.Nil(t, flushed)
	require.NoError(t, mock.ExpectationsWereMet())
}
