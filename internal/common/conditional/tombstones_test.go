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
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

const (
	cleanupLockPattern     = `SELECT pg_try_advisory_xact_lock\(hashtextextended\(\$1, \$2\)\)`
	lockTombstonePattern   = `SELECT "identifier" FROM "resource_revision" WHERE \(\("kind" = \$1\) AND NOT EXISTS \(SELECT 1 FROM "submodel" AS "live" WHERE \("live"."submodel_identifier" = "resource_revision"."identifier"\)\)\) ORDER BY "identifier" ASC LIMIT \$2 FOR UPDATE OF "resource_revision" SKIP LOCKED`
	deleteTombstonePattern = `DELETE FROM "resource_revision" WHERE \(\(\("kind" = \$1\) AND NOT EXISTS \(SELECT 1 FROM "submodel" AS "live" WHERE \("live"."submodel_identifier" = "resource_revision"."identifier"\)\)\) AND \("identifier" IN \(\$2, \$3\)\)\)`
)

func TestTombstoneBatchSkipsWhileAnotherInstanceCleansUp(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectQuery(cleanupLockPattern).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
	mock.ExpectRollback()

	removed, err := removeTombstoneBatch(context.Background(), db, KindSubmodel)
	require.NoError(t, err)
	require.Zero(t, removed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTombstoneBatchLocksThenDeletesRevisionsOfDeletedResources(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectQuery(cleanupLockPattern).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery(lockTombstonePattern).WithArgs(string(KindSubmodel), tombstoneBatchSize).
		WillReturnRows(sqlmock.NewRows([]string{"identifier"}).AddRow("sm-1").AddRow("sm-2"))
	mock.ExpectExec(deleteTombstonePattern).WithArgs(string(KindSubmodel), "sm-1", "sm-2").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	removed, err := removeTombstoneBatch(context.Background(), db, KindSubmodel)
	require.NoError(t, err)
	require.Equal(t, int64(1), removed, "a resource recreated after the lock keeps its revision")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEveryRevisionKindHasALiveResourceTable(t *testing.T) {
	for _, kind := range []Kind{KindAAS, KindSubmodel, KindConceptDescription, KindAASDescriptor,
		KindSubmodelDescriptor, KindDiscoveryEntry, KindAASXPackage, KindCompanyDescriptor} {
		require.Contains(t, liveResources, kind)
	}
}

const liveRevisionPattern = `SELECT EXISTS \(SELECT 1 FROM "submodel" AS "live" WHERE \("live"."submodel_identifier" = \$1\)\), COALESCE\(\(SELECT "revision" FROM "resource_revision" WHERE \(\("identifier" = \$2\) AND \("kind" = \$3\)\)\), \$4\)`

func verifyContext(ifMatch string) context.Context {
	state := &State{method: http.MethodPost, mode: ModeVerifyOnly, target: &submodelA, conds: parseConditions(http.Header{"If-Match": {ifMatch}})}
	return context.WithValue(context.Background(), stateContextKey{}, state)
}

func TestVerifyTargetFailsForDeletedTargetsDespiteTheirKeptRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(liveRevisionPattern).WillReturnRows(sqlmock.NewRows([]string{"exists", "revision"}).AddRow(false, 7))

	require.ErrorIs(t, VerifyTarget(verifyContext("*"), db), ErrTargetNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyTargetEvaluatesTheLiveRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(liveRevisionPattern).WillReturnRows(sqlmock.NewRows([]string{"exists", "revision"}).AddRow(true, 7))
	mock.ExpectQuery(liveRevisionPattern).WillReturnRows(sqlmock.NewRows([]string{"exists", "revision"}).AddRow(true, 7))

	require.True(t, IsPreconditionFailed(VerifyTarget(verifyContext(ConcurrencyETag(submodelA, 6)), db)))
	require.NoError(t, VerifyTarget(verifyContext(ConcurrencyETag(submodelA, 7)), db))
	require.NoError(t, mock.ExpectationsWereMet())
}
