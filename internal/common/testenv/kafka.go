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

package testenv

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

// KafkaAddress returns a dynamically allocated integration broker listener.
func KafkaAddress(variable string) string { return net.JoinHostPort("127.0.0.1", os.Getenv(variable)) }

// KafkaCertificate resolves checked-in public test fixtures independently of the calling package.
func KafkaCertificate(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "kafka", name)
}

// KafkaConsumer keeps unmatched records across partition polls for event parity assertions.
type KafkaConsumer struct {
	client  *kgo.Client
	pending map[string]*kgo.Record
}

// SubscribeKafka opens the integration topic from its beginning; assertions use unique event IDs.
func SubscribeKafka(t *testing.T) *KafkaConsumer {
	t.Helper()
	client, err := kgo.NewClient(kgo.SeedBrokers(KafkaAddress("BASYX_IT_KAFKA_PORT")), kgo.ConsumeTopics("basyx.events"), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	require.NoError(t, err)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	require.NoError(t, client.Ping(ctx))
	return &KafkaConsumer{client: client, pending: map[string]*kgo.Record{}}
}

// Await returns the record with id, preserving records for other IDs in the same batch.
func (c *KafkaConsumer) Await(t *testing.T, id string) *kgo.Record {
	t.Helper()
	return c.await(t, func(event events.FeedRecord) bool { return event.ID == id })
}

// AwaitSubject waits for an event with a unique test subject and type.
func (c *KafkaConsumer) AwaitSubject(t *testing.T, subject, kind string) events.FeedRecord {
	t.Helper()
	record := c.await(t, func(event events.FeedRecord) bool { return event.Subject == subject && event.Type == kind })
	var event events.FeedRecord
	require.NoError(t, json.Unmarshal(record.Value, &event))
	return event
}
func (c *KafkaConsumer) await(t *testing.T, matches func(events.FeedRecord) bool) *kgo.Record {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for {
		for id, record := range c.pending {
			var event events.FeedRecord
			require.NoError(t, json.Unmarshal(record.Value, &event))
			if matches(event) {
				delete(c.pending, id)
				return record
			}
		}
		fetches := c.client.PollFetches(ctx)
		require.NoError(t, fetches.Err(), "KAFKA-TEST-FETCH")
		fetches.EachRecord(func(record *kgo.Record) {
			var event events.FeedRecord
			require.NoError(t, json.Unmarshal(record.Value, &event))
			c.pending[event.ID] = record
		})
	}
}

// AssertEvent verifies the structured binding, entity key, and unchanged event contract.
func (c *KafkaConsumer) AssertEvent(t *testing.T, event events.FeedRecord, key string) *kgo.Record {
	t.Helper()
	record := c.Await(t, event.ID)
	require.Equal(t, "basyx.events", record.Topic)
	require.Equal(t, key, string(record.Key))
	require.Equal(t, []kgo.RecordHeader{{Key: "content-type", Value: []byte("application/cloudevents+json")}}, record.Headers)
	var actual events.FeedRecord
	require.NoError(t, json.Unmarshal(record.Value, &actual))
	require.Equal(t, event, actual)
	return record
}

// AssertNoEvent checks that a unique subject/type has no record, including already fetched records.
func (c *KafkaConsumer) AssertNoEvent(t *testing.T, subject, kind string) {
	t.Helper()
	check := func(record *kgo.Record) {
		var event events.FeedRecord
		require.NoError(t, json.Unmarshal(record.Value, &event))
		require.False(t, event.Subject == subject && event.Type == kind, fmt.Sprintf("unexpected %s event for %s", kind, subject))
	}
	for _, record := range c.pending {
		check(record)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	for ctx.Err() == nil {
		c.client.PollFetches(ctx).EachRecord(check)
	}
}
