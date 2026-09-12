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

package eventoutbox

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/stretchr/testify/require"
)

type testPublisher func(context.Context, json.RawMessage, []byte) error

func (p testPublisher) Publish(ctx context.Context, r json.RawMessage, e []byte) error {
	return p(ctx, r, e)
}
func TestOutboxQueryExcludesEarlierEventsAndSkipsLockedRows(t *testing.T) {
	query, args, err := claimQuery("mqtt", "previous")
	require.NoError(t, err)
	require.Contains(t, query, "WITH RECURSIVE")
	require.Contains(t, query, "CROSS JOIN LATERAL")
	require.Contains(t, query, `FOR UPDATE OF "pending" SKIP LOCKED`)
	require.Contains(t, query, `"ordering_key" > "heads"."ordering_key"`)
	require.Contains(t, args, "mqtt")
}
func TestDeliveryPersistsRetryOrDeletesAcknowledgedEntry(t *testing.T) {
	for _, fail := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT.*FOR UPDATE.*SKIP LOCKED").WillReturnRows(sqlmock.NewRows([]string{"seq", "envelope", "routing", "attempts", "ordering_key"}).AddRow(1, `{"id":"stable"}`, `"topic"`, 0, "entity"))
		operation := "DELETE"
		if fail {
			operation = "UPDATE"
		}
		mock.ExpectExec(operation).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		found, deliveryErr := NewRepository(db).DeliverOne(t.Context(), "mqtt", testPublisher(func(ctx context.Context, _ json.RawMessage, payload []byte) error {
			require.JSONEq(t, `{"id":"stable"}`, string(payload))
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.LessOrEqual(t, time.Until(deadline), 10*time.Second)
			if fail {
				return errors.New("TEST-PUBLISH-UNAVAILABLE")
			}
			return nil
		}))
		require.True(t, found, "%v", deliveryErr)
		require.Equal(t, fail, deliveryErr != nil)
		require.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		require.NoError(t, db.Close())
	}
}
func TestEnqueueFailurePropagatesToMutation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	mock.ExpectExec("INSERT INTO").WillReturnError(errors.New("TEST-OUTBOX-UNAVAILABLE"))
	event, err := events.NewBuilder(events.DefaultConfig()).AASCreated("urn:aas:1", "", nil)
	require.NoError(t, err)
	require.ErrorContains(t, NewRepository(db).Enqueue(t.Context(), tx, "mqtt", "aas:1", event, json.RawMessage(`"topic"`)), "OUTBOX-ENQUEUE-EXEC")
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
	mock.ExpectClose()
	require.NoError(t, db.Close())
}

func TestWorkerWaitsAfterSlowFailedAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"seq", "envelope", "routing", "attempts", "ordering_key"}).AddRow(1, `{}`, `"topic"`, 0, "entity"))
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"seq", "envelope", "routing", "attempts", "ordering_key"}).AddRow(2, `{}`, `"topic"`, 0, "entity"))
		mock.ExpectRollback()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		var attempts int
		started := time.Now()
		var secondAttempt time.Duration
		publisher := testPublisher(func(context.Context, json.RawMessage, []byte) error {
			attempts++
			if attempts == 1 {
				time.Sleep(time.Second)
				return errors.New("TEST-PUBLISH-RETRY")
			}
			secondAttempt = time.Since(started)
			cancel()
			return context.Canceled
		})
		instruments, err := newInstruments("worker-delay-test")
		require.NoError(t, err)
		worker := &Worker{repository: NewRepository(db), sink: "test", publisher: publisher, metrics: instruments}
		worker.run(ctx)
		synctest.Wait()
		require.Equal(t, 2, attempts)
		require.GreaterOrEqual(t, secondAttempt, 1250*time.Millisecond)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestWorkerRotatesEntityCursorAndWraps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		keys := []string{"a", "b", "c", "a"}
		afterKey := ""
		for i, key := range keys {
			mock.ExpectBegin()
			if i == len(keys)-1 {
				expectOutboxClaim(t, mock, afterKey, outboxDeliveryRows())
				afterKey = ""
			}
			expectOutboxClaim(t, mock, afterKey, outboxDeliveryRows().AddRow(i+1, key, `"topic"`, 0, key))
			if i == len(keys)-1 {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			afterKey = key
		}
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		var delivered []string
		publisher := testPublisher(func(_ context.Context, _ json.RawMessage, raw []byte) error {
			delivered = append(delivered, string(raw))
			if len(delivered) == len(keys) {
				cancel()
			}
			return nil
		})
		instruments, err := newInstruments("worker-cursor-test")
		require.NoError(t, err)
		worker := &Worker{repository: NewRepository(db), sink: "test", publisher: publisher, metrics: instruments}
		worker.run(ctx)
		synctest.Wait()
		require.Equal(t, keys, delivered)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func expectOutboxClaim(t *testing.T, mock sqlmock.Sqlmock, afterKey string, rows *sqlmock.Rows) {
	t.Helper()
	query, args, err := claimQuery("test", afterKey)
	require.NoError(t, err)
	values := make([]driver.Value, len(args))
	for i, value := range args {
		values[i] = value
	}
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(values...).WillReturnRows(rows)
}

func outboxDeliveryRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"seq", "envelope", "routing", "attempts", "ordering_key"})
}
