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

package postgresstaging

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

type stagingSQLStateError struct {
	code string
}

func (e stagingSQLStateError) Error() string {
	return e.code
}

func (e stagingSQLStateError) SQLState() string {
	return e.code
}

func TestStageWritesAndCleansOneScopedDataset(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	expectStageDDL(mock)
	stage, err := Open(t.Context(), tx)
	require.NoError(t, err)
	writer, err := stage.NewWriter("elements")
	require.NoError(t, err)
	mock.ExpectExec(`INSERT INTO "basyx_mutation_stage"`).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, writer.Add(t.Context(), Row{
		MatchKey: "Collection.Property",
		Ordinal:  1,
		Data:     json.RawMessage(`{"value":"target"}`),
	}))
	require.NoError(t, writer.Flush(t.Context()))

	query, args, err := stage.Dataset(goqu.Dialect(common.Dialect), "elements").Prepared(true).ToSQL()
	require.NoError(t, err)
	require.Contains(t, query, `FROM "basyx_mutation_stage"`)
	require.Len(t, args, 2)

	mock.ExpectExec(`DELETE FROM "basyx_mutation_stage"`).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, stage.Cleanup(t.Context()))
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStagesInOneTransactionUseDifferentOperationIDs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	expectStageDDL(mock)
	first, err := Open(t.Context(), tx)
	require.NoError(t, err)
	expectStageDDL(mock)
	second, err := Open(t.Context(), tx)
	require.NoError(t, err)
	require.NotEqual(t, first.id, second.id)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpenStopsBeforeDatabaseWorkWhenContextIsCancelled(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = Open(ctx, tx)
	require.ErrorIs(t, err, context.Canceled)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWriterRejectsDuplicateMatchKeys(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	expectStageDDL(mock)
	stage, err := Open(t.Context(), tx)
	require.NoError(t, err)
	writer, err := stage.NewWriter("elements")
	require.NoError(t, err)
	require.NoError(t, writer.Add(t.Context(), Row{MatchKey: "duplicate", Data: json.RawMessage(`{}`)}))
	require.NoError(t, writer.Add(t.Context(), Row{MatchKey: "duplicate", Data: json.RawMessage(`{}`)}))
	mock.ExpectExec(`INSERT INTO "basyx_mutation_stage"`).WillReturnError(stagingSQLStateError{code: "23505"})
	err = writer.Flush(t.Context())
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err))
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWriterFlushesInBoundedBatches(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	expectStageDDL(mock)
	stage, err := Open(t.Context(), tx)
	require.NoError(t, err)
	writer, err := stage.NewWriter("elements")
	require.NoError(t, err)
	mock.ExpectExec(`INSERT INTO "basyx_mutation_stage"`).WillReturnResult(sqlmock.NewResult(0, defaultBatchSize))
	for index := range defaultBatchSize {
		require.NoError(t, writer.Add(t.Context(), Row{
			MatchKey: strconv.Itoa(index),
			Ordinal:  int64(index),
			Data:     json.RawMessage(`{}`),
		}))
	}
	require.Empty(t, writer.rows)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWriterFlushesMultipleDatasetsTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	expectStageDDL(mock)
	stage, err := Open(t.Context(), tx)
	require.NoError(t, err)
	writer, err := stage.NewWriter("metadata")
	require.NoError(t, err)
	require.NoError(t, writer.Add(t.Context(), Row{MatchKey: "model", Data: json.RawMessage(`{}`)}))
	require.NoError(t, writer.AddToDataset(t.Context(), "elements", Row{MatchKey: "property", Data: json.RawMessage(`{}`)}))
	mock.ExpectExec(`INSERT INTO "basyx_mutation_stage"`).WillReturnResult(sqlmock.NewResult(0, 2))
	require.NoError(t, writer.Flush(t.Context()))
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWriterPropagatesCancellationBeforeFlush(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	expectStageDDL(mock)
	stage, err := Open(t.Context(), tx)
	require.NoError(t, err)
	writer, err := stage.NewWriter("elements")
	require.NoError(t, err)
	require.NoError(t, writer.Add(t.Context(), Row{MatchKey: "one", Data: json.RawMessage(`{}`)}))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, writer.Flush(ctx), context.Canceled)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectStageDDL(mock sqlmock.Sqlmock) {
	mock.ExpectExec("DO \\$basyx_stage\\$").WillReturnResult(sqlmock.NewResult(0, 0))
}
