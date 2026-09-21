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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	wire "github.com/Azure/go-amqp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/stretchr/testify/require"
)

// AMQPAddress resolves an allocated integration listener.
func AMQPAddress(variable string) string { return net.JoinHostPort("127.0.0.1", os.Getenv(variable)) }

// AMQPCertificate resolves disposable, public test TLS fixtures.
func AMQPCertificate(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "amqp", name)
}

// AMQPConsumer uses a private queue so independent assertions never compete for messages.
type AMQPConsumer struct {
	receiver *wire.Receiver
	pending  map[string]*wire.Message
	Order    []string
}

// AMQPManagement provisions isolated test topology through RabbitMQ's HTTP API.
func AMQPManagement(t *testing.T, method, path string, body any) {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, "http://"+AMQPAddress("BASYX_IT_AMQP_MANAGEMENT_PORT")+"/api"+path, bytes.NewReader(raw))
	require.NoError(t, err)
	req.SetBasicAuth("basyx", "secret")
	req.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	if method == http.MethodDelete && response.StatusCode == http.StatusNotFound {
		return
	}
	require.Less(t, response.StatusCode, 300, path)
}

// SubscribeAMQP attaches a native AMQP 1.0 receiver before the test mutates models.
func SubscribeAMQP(t *testing.T) *AMQPConsumer {
	t.Helper()
	queue := fmt.Sprintf("test-%d", time.Now().UnixNano())
	path := "/queues/%2F/" + url.PathEscape(queue)
	AMQPManagement(t, http.MethodPut, path, map[string]any{"durable": false, "auto_delete": false, "arguments": map[string]any{}})
	t.Cleanup(func() { AMQPManagement(t, http.MethodDelete, path, nil) })
	AMQPManagement(t, http.MethodPost, "/bindings/%2F/e/basyx.events/q/"+url.PathEscape(queue), map[string]any{"routing_key": "", "arguments": map[string]any{}})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := wire.Dial(ctx, "amqp://"+AMQPAddress("BASYX_IT_AMQP_PORT"), &wire.ConnOptions{SASLType: wire.SASLTypePlain("basyx", "secret")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	session, err := conn.NewSession(ctx, nil)
	require.NoError(t, err)
	receiver, err := session.NewReceiver(ctx, "/queues/"+queue, nil)
	require.NoError(t, err)
	return &AMQPConsumer{receiver: receiver, pending: make(map[string]*wire.Message)}
}
func (c *AMQPConsumer) await(t *testing.T, matches func(events.FeedRecord) bool) *wire.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for {
		for id, message := range c.pending {
			var event events.FeedRecord
			require.NoError(t, json.Unmarshal(message.GetData(), &event))
			if matches(event) {
				delete(c.pending, id)
				return message
			}
		}
		message, err := c.receiver.Receive(ctx, nil)
		require.NoError(t, err, "AMQP-TEST-RECEIVE")
		require.NoError(t, c.receiver.AcceptMessage(ctx, message))
		var event events.FeedRecord
		require.NoError(t, json.Unmarshal(message.GetData(), &event))
		c.pending[event.ID] = message
		c.Order = append(c.Order, event.ID)
	}
}

// AssertEvent verifies the native structured binding and complete event parity.
func (c *AMQPConsumer) AssertEvent(t *testing.T, event events.FeedRecord) *wire.Message {
	t.Helper()
	message := c.await(t, func(actual events.FeedRecord) bool { return actual.ID == event.ID })
	require.NotNil(t, message.Properties)
	require.NotNil(t, message.Properties.ContentType)
	require.Equal(t, "application/cloudevents+json", *message.Properties.ContentType)
	require.NotNil(t, message.Header)
	require.True(t, message.Header.Durable)
	var actual events.FeedRecord
	require.NoError(t, json.Unmarshal(message.GetData(), &actual))
	require.Equal(t, event, actual)
	return message
}

// AwaitSubject returns a uniquely identified mutation event.
func (c *AMQPConsumer) AwaitSubject(t *testing.T, subject, kind string) events.FeedRecord {
	t.Helper()
	message := c.await(t, func(actual events.FeedRecord) bool { return actual.Subject == subject && actual.Type == kind })
	var event events.FeedRecord
	require.NoError(t, json.Unmarshal(message.GetData(), &event))
	return event
}

// AssertNoEvent checks rollback and no-op cases, including buffered messages.
func (c *AMQPConsumer) AssertNoEvent(t *testing.T, subject, kind string) {
	t.Helper()
	check := func(message *wire.Message) {
		var event events.FeedRecord
		require.NoError(t, json.Unmarshal(message.GetData(), &event))
		require.False(t, event.Subject == subject && event.Type == kind)
	}
	for _, message := range c.pending {
		check(message)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	for {
		message, err := c.receiver.Receive(ctx, nil)
		if ctx.Err() != nil {
			return
		}
		require.NoError(t, err)
		check(message)
		require.NoError(t, c.receiver.AcceptMessage(ctx, message))
	}
}
