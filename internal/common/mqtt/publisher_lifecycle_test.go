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

package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eclipse/paho.golang/packets"
	"github.com/stretchr/testify/require"
)

func TestPublisherDeadlineInterruptsBlockedWrite(t *testing.T) {
	for _, qos := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("QoS%d", qos), func(t *testing.T) {
			publisher, writing := stalledPublisher(t, qos)
			ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
			defer cancel()
			result := publishLargeEvent(ctx, publisher)
			awaitWrite(t, writing)
			awaitPublishFailure(t, result)
			require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
		})
	}
}

func TestPublisherCancellationInterruptsBlockedWrite(t *testing.T) {
	publisher, writing := stalledPublisher(t, 1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := publishLargeEvent(ctx, publisher)
	awaitWrite(t, writing)
	cancel()
	awaitPublishFailure(t, result)
}

func TestPublisherStopInterruptsBlockedWrite(t *testing.T) {
	publisher, writing := stalledPublisher(t, 1)
	result := publishLargeEvent(t.Context(), publisher)
	awaitWrite(t, writing)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, publisher.Stop(ctx))
	awaitPublishFailure(t, result)
	require.False(t, publisher.Connected())
}

func TestPublisherCancellationReleasesConcurrentPublishes(t *testing.T) {
	publisher, writing := stalledPublisher(t, 1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	results := []<-chan error{publishLargeEvent(ctx, publisher)}
	awaitWrite(t, writing)
	for range 3 {
		result := make(chan error, 1)
		go func() { result <- publisher.Publish(ctx, json.RawMessage(`"test"`), []byte(`{"data":"queued"}`)) }()
		results = append(results, result)
	}
	cancel()
	for _, result := range results {
		awaitPublishFailure(t, result)
	}
}

func TestPublisherWithoutCallerDeadlineUsesBoundedAttempt(t *testing.T) {
	publisher, writing := stalledPublisher(t, 1)
	result := publishLargeEvent(t.Context(), publisher)
	awaitWrite(t, writing)
	select {
	case err := <-result:
		require.ErrorContains(t, err, "MQTT-PUBLISH-DELIVERY")
	case <-time.After(12 * time.Second):
		t.Fatal("publish exceeded the default attempt timeout")
	}
}

func publishLargeEvent(ctx context.Context, publisher *Publisher) <-chan error {
	result := make(chan error, 1)
	payload := []byte(`{"data":"` + strings.Repeat("x", 16*1024*1024) + `"}`)
	go func() { result <- publisher.Publish(ctx, json.RawMessage(`"test"`), payload) }()
	return result
}

func awaitWrite(t *testing.T, writing <-chan struct{}) {
	t.Helper()
	select {
	case <-writing:
	case <-time.After(3 * time.Second):
		t.Fatal("broker did not receive the beginning of the publish packet")
	}
}

func awaitPublishFailure(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		require.ErrorContains(t, err, "MQTT-PUBLISH-DELIVERY")
	case <-time.After(2 * time.Second):
		t.Fatal("publish remained blocked after its context or publisher was stopped")
	}
}

func stalledPublisher(t *testing.T, qos int) (*Publisher, <-chan struct{}) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	release := make(chan struct{})
	writing := make(chan struct{})
	finished := make(chan struct{})
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(release); _ = listener.Close() }) }
	go func() {
		defer close(finished)
		serveStalledBroker(listener, writing, release)
	}()
	t.Cleanup(func() { stop(); <-finished })
	ctx, cancel := context.WithCancel(t.Context())
	publisher, err := NewPublisher(ctx, Config{Broker: "mqtt://" + listener.Addr().String(), ClientID: "stalled-test", SinkID: "mqtt", QoS: qos})
	require.NoError(t, err)
	t.Cleanup(func() {
		cancel()
		stop()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(t.Context()), 2*time.Second)
		defer shutdownCancel()
		require.NoError(t, publisher.Stop(shutdownCtx))
	})
	connectCtx, connectCancel := context.WithTimeout(ctx, 5*time.Second)
	defer connectCancel()
	require.NoError(t, publisher.connection.AwaitConnection(connectCtx))
	return publisher, writing
}

func serveStalledBroker(listener net.Listener, writing chan<- struct{}, release <-chan struct{}) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetReadBuffer(1024)
	}
	if _, err := packets.ReadPacket(conn); err != nil {
		return
	}
	keepalive := uint16(0)
	ack := packets.Connack{Properties: &packets.Properties{ServerKeepAlive: &keepalive}}
	if _, err := ack.WriteTo(conn); err != nil {
		return
	}
	var firstByte [1]byte
	if _, err := io.ReadFull(conn, firstByte[:]); err != nil {
		return
	}
	close(writing)
	<-release
}
