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

package aasregistrydatabase

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestBulkAASInsertWritesHistoryFromSubmittedDescriptorWithoutReadback(t *testing.T) {
	previousHistoryConfig := history.ActiveConfig()
	t.Cleanup(func() {
		history.Configure(previousHistoryConfig)
	})
	history.Configure(history.Config{
		Mode:              history.ModeAPI,
		Immutability:      history.ImmutabilityNone,
		AuditIdentityMode: history.AuditIdentityNone,
	})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	descriptor := model.AssetAdministrationShellDescriptor{
		Id:        "aas-1",
		IdShort:   "AASOne",
		CreatedAt: &createdAt,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT nextval`).
		WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(11)))
	mock.ExpectExec(`INSERT INTO "descriptor"`).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "row_hash" FROM "descriptor_history"`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO "descriptor_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(1)))
	mock.ExpectExec(`INSERT INTO "descriptor_history_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	failedIndex, err := (&PostgreSQLAASRegistryDatabase{}).InsertAdministrationShellDescriptorsInTransaction(
		context.Background(),
		tx,
		[]model.AssetAdministrationShellDescriptor{descriptor},
	)
	require.NoError(t, err)
	require.Equal(t, -1, failedIndex)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrepareBulkCreatedDescriptorHistorySnapshotsFetchesGeneratedCreatedAt(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	createdAt := time.Date(2026, time.February, 3, 4, 5, 6, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "id", "created_at" FROM "aas_descriptor"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("aas-1", createdAt))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	snapshots, failedIndex, err := prepareBulkCreatedDescriptorHistorySnapshotsTx(
		context.Background(),
		tx,
		[]model.AssetAdministrationShellDescriptor{{Id: "aas-1"}},
	)
	require.NoError(t, err)
	require.Equal(t, -1, failedIndex)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, snapshots, 1)
	require.NotNil(t, snapshots[0].CreatedAt)
	require.True(t, snapshots[0].CreatedAt.Equal(createdAt))

	jsonable, err := snapshots[0].ToJsonable()
	require.NoError(t, err)
	require.Equal(t, createdAt.Format(time.RFC3339Nano), jsonable["createdAt"])
}

func TestPrepareBulkCreatedDescriptorHistorySnapshotsKeepsSubmittedCreatedAt(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	createdAt := time.Date(2026, time.March, 4, 5, 6, 7, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	snapshots, failedIndex, err := prepareBulkCreatedDescriptorHistorySnapshotsTx(
		context.Background(),
		tx,
		[]model.AssetAdministrationShellDescriptor{{Id: "aas-1", CreatedAt: &createdAt}},
	)
	require.NoError(t, err)
	require.Equal(t, -1, failedIndex)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, snapshots, 1)
	require.NotNil(t, snapshots[0].CreatedAt)
	require.True(t, snapshots[0].CreatedAt.Equal(createdAt))
}
