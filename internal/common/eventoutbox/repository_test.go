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
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type testPublisher func(context.Context, json.RawMessage, []byte) error

func (p testPublisher) Publish(ctx context.Context, r json.RawMessage, e []byte) error {
	return p(ctx, r, e)
}
func TestOutboxQueryExcludesEarlierEventsAndSkipsLockedRows(t *testing.T) {
	query, args, err := claimQuery("mqtt")
	require.NoError(t, err)
	require.Contains(t, query, "NOT EXISTS")
	require.Contains(t, query, "FOR UPDATE SKIP LOCKED")
	require.Contains(t, query, `"earlier"."ordering_key" = "pending"."ordering_key"`)
	require.Contains(t, args, "mqtt")
}
func TestDeliveryPersistsRetryOrDeletesAcknowledgedEntry(t *testing.T) {
	for _, fail := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT.*FOR UPDATE SKIP LOCKED").WillReturnRows(sqlmock.NewRows([]string{"seq", "envelope", "routing", "attempts"}).AddRow(1, `{"id":"stable"}`, `"topic"`, 0))
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
		operation := "DELETE"
		if fail {
			operation = "UPDATE"
		}
		mock.ExpectExec(operation).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		found, deliveryErr := NewRepository(db).DeliverOne(t.Context(), "mqtt", "replica", testPublisher(func(ctx context.Context, _ json.RawMessage, payload []byte) error {
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
