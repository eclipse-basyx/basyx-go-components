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
	cleanupLockPattern      = `SELECT pg_try_advisory_lock\(hashtextextended\(\$1, \$2\)\)`
	cleanupUnlockPattern    = `SELECT pg_advisory_unlock\(hashtextextended\(\$1, \$2\)\)`
	findTombstonePattern    = `SELECT "identifier" FROM "resource_revision" WHERE .* AND \("identifier" > \$\d\)\) ORDER BY "identifier" ASC LIMIT \$\d+$`
	findSubmodelPattern     = `SELECT "identifier" FROM "resource_revision" WHERE \(\(\("kind" = \$1\) AND NOT EXISTS \(SELECT 1 FROM "submodel" AS "live" WHERE \("live"."submodel_identifier" = "resource_revision"."identifier"\)\)\) AND \("identifier" > \$2\)\) ORDER BY "identifier" ASC LIMIT \$3$`
	lockCandidatesPattern   = `SELECT "identifier" FROM "resource_revision" WHERE .*"submodel" AS "live".* AND \("identifier" IN \(\$2, \$3\)\)\) ORDER BY "identifier" ASC FOR UPDATE OF "resource_revision" SKIP LOCKED`
	deleteTombstonePattern  = `DELETE FROM "resource_revision" WHERE \(\(\("kind" = \$1\) AND NOT EXISTS \(SELECT 1 FROM "submodel" AS "live" WHERE \("live"."submodel_identifier" = "resource_revision"."identifier"\)\)\) AND \("identifier" IN \(\$2, \$3\)\)\)`
	identifierColumn        = "identifier"
	cleanupLockResultColumn = "locked"
)

func TestTombstoneCleanupSkipsWhileAnotherInstanceCleansUp(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(cleanupLockPattern).WillReturnRows(sqlmock.NewRows([]string{cleanupLockResultColumn}).AddRow(false))

	removed, err := RemoveTombstones(context.Background(), db)
	require.NoError(t, err)
	require.Zero(t, removed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTombstoneCleanupFindsWithoutLocksAndLocksOnlyCandidates(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(cleanupLockPattern).WillReturnRows(sqlmock.NewRows([]string{cleanupLockResultColumn}).AddRow(true))
	for _, kind := range tombstoneKinds {
		if kind != KindSubmodel {
			mock.ExpectQuery(findTombstonePattern).WillReturnRows(sqlmock.NewRows([]string{identifierColumn}))
			continue
		}
		mock.ExpectQuery(findSubmodelPattern).WithArgs(string(KindSubmodel), "", tombstoneBatchSize).
			WillReturnRows(sqlmock.NewRows([]string{identifierColumn}).AddRow("sm-1").AddRow("sm-2"))
		mock.ExpectBegin()
		mock.ExpectQuery(lockCandidatesPattern).WithArgs(string(KindSubmodel), "sm-1", "sm-2").
			WillReturnRows(sqlmock.NewRows([]string{identifierColumn}).AddRow("sm-1").AddRow("sm-2"))
		mock.ExpectExec(deleteTombstonePattern).WithArgs(string(KindSubmodel), "sm-1", "sm-2").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}
	mock.ExpectQuery(cleanupUnlockPattern).WillReturnRows(sqlmock.NewRows([]string{cleanupLockResultColumn}).AddRow(true))

	removed, err := RemoveTombstones(context.Background(), db)
	require.NoError(t, err)
	require.Equal(t, int64(1), removed, "a resource recreated after the lock keeps its revision")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEveryRevisionKindHasALiveResourceTable(t *testing.T) {
	for _, kind := range tombstoneKinds {
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
