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

package events

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFanoutReusesEventAndPropagatesFailure(t *testing.T) {
	event, err := NewBuilder(DefaultConfig()).AASCreated("urn:aas:1", "", nil)
	require.NoError(t, err)
	failure := errors.New("TEST-SINK-FAILURE")
	seen := []FeedEvent{}
	write := func(_ context.Context, _ *sql.Tx, _ Mutation, event FeedEvent) error {
		seen = append(seen, event)
		return nil
	}
	fail := func(_ context.Context, _ *sql.Tx, _ Mutation, _ FeedEvent) error { return failure }
	err = Fanout(write, write, fail, write)(t.Context(), nil, Mutation{}, event)
	require.ErrorIs(t, err, failure)
	require.Equal(t, []FeedEvent{event, event}, seen)
	record, err := Record(event, false)
	require.NoError(t, err)
	require.Equal(t, "application/json", record.DataContentType)
	require.Equal(t, event.Time.Truncate(time.Microsecond), record.Time)
}
func TestRetryDelayBounded(t *testing.T) {
	for attempt := 0; attempt < 100; attempt++ {
		delay := RetryDelay(attempt)
		require.Positive(t, delay)
		require.LessOrEqual(t, delay, time.Minute)
	}
}
