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

package amqp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	wire "github.com/Azure/go-amqp"
	"github.com/stretchr/testify/require"
)

func TestStructuredMessage(t *testing.T) {
	routing, err := Routing("/queues/original")
	require.NoError(t, err)
	envelope := []byte(`{"specversion":"1.0","id":"same","source":"/test","type":"test","data":{}}`)
	address, message, err := structuredMessage(routing, envelope)
	require.NoError(t, err)
	require.Equal(t, "/queues/original", address)
	require.Equal(t, [][]byte{envelope}, message.Data)
	require.Equal(t, "application/cloudevents+json", *message.Properties.ContentType)
	require.True(t, message.Header.Durable)
	require.Empty(t, message.ApplicationProperties)
	for _, raw := range []string{`{`, `{}`, `{"address":""}`, `{"address":42}`} {
		_, _, err = structuredMessage(json.RawMessage(raw), envelope)
		require.Error(t, err)
	}
}
func TestOnlyAcceptedAcknowledgesDelivery(t *testing.T) {
	require.NoError(t, accepted(&wire.StateAccepted{}))
	for _, state := range []wire.DeliveryState{nil, &wire.StateRejected{}, &wire.StateReleased{}, &wire.StateModified{}} {
		require.Error(t, accepted(state))
	}
}
func TestPublisherStartsOfflineAndStops(t *testing.T) {
	p, err := NewPublisher(t.Context(), Config{Broker: "amqp://127.0.0.1:1", Address: "/queues/test", SinkID: "amqp"})
	require.NoError(t, err)
	require.False(t, p.Connected())
	routing, err := Routing("/queues/test")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.Error(t, p.Publish(ctx, routing, []byte(`{}`)))
	stop, cancelStop := context.WithTimeout(t.Context(), time.Second)
	defer cancelStop()
	require.NoError(t, p.Stop(stop))
	require.NoError(t, p.Stop(stop))
	require.Error(t, p.Publish(t.Context(), routing, []byte(`{}`)))
}
