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
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	wire "github.com/Azure/go-amqp"
	"github.com/stretchr/testify/require"
)

func TestPublishDeadlineWithBlockedConnectionClose(t *testing.T) {
	p, peer := publisherWithBlockedPeer(t)
	routing, err := Routing("test")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = p.Publish(ctx, routing, []byte(`{}`))
	elapsed := time.Since(started)
	require.ErrorContains(t, err, "AMQP-PUBLISH-DELIVERY")
	require.Less(t, elapsed, time.Second, "closing a blocked connection must not extend the publish deadline")
	require.False(t, p.Connected())
	retry, cancelRetry := context.WithTimeout(t.Context(), time.Second)
	defer cancelRetry()
	require.ErrorContains(t, p.Publish(retry, routing, []byte(`{}`)), "AMQP-PUBLISH-CONNECT")
	stop, cancelStop := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancelStop()
	require.ErrorContains(t, p.Stop(stop), "AMQP-PUBLISHER-STOP")
	require.NoError(t, peer.Close())
	finished, cancelFinished := context.WithTimeout(t.Context(), time.Second)
	defer cancelFinished()
	require.NoError(t, p.Stop(finished))
	require.NoError(t, p.Stop(finished))
}

func publisherWithBlockedPeer(t *testing.T) (*Publisher, net.Conn) {
	t.Helper()
	client, peer := net.Pipe()
	t.Cleanup(func() { _ = peer.Close(); _ = client.Close() })
	handshake := make(chan error, 1)
	go func() { handshake <- blockedPeerHandshake(peer) }()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, err := wire.NewConn(ctx, client, &wire.ConnOptions{WriteTimeout: 5 * time.Second})
	require.NoError(t, err)
	t.Cleanup(func() { _ = peer.Close(); _ = conn.Close() })
	session, err := conn.NewSession(ctx, nil)
	require.NoError(t, err)
	sender, err := session.NewSender(ctx, "test", &wire.SenderOptions{Name: "deadline"})
	require.NoError(t, err)
	require.NoError(t, <-handshake)
	p, err := NewPublisher(t.Context(), Config{Broker: "amqp://127.0.0.1:1", Address: "test", SinkID: "amqp"})
	require.NoError(t, err)
	p.conn, p.session = p.trackConnection(conn), session
	p.senders = map[string]*wire.Sender{"test": sender}
	t.Cleanup(func() {
		_ = peer.Close()
		stop, cancelStop := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancelStop()
		require.NoError(t, p.Stop(stop))
	})
	return p, peer
}

func blockedPeerHandshake(peer net.Conn) error {
	header := make([]byte, 8)
	if _, err := io.ReadFull(peer, header); err != nil {
		return err
	}
	if _, err := peer.Write(header); err != nil {
		return err
	}
	replies := []string{
		// AMQP Open, Begin and Attach; deliberately omit link credit and further reads.
		"0000002a02000000005310d00000001a00000005a10c626c6f636b65642d70656572404040700000ea60",
		"0000002802000000005311d000000018000000056000005201700000138870000003e87000007fff",
		"0000004202000000005312d0000000320000000ba108646561646c696e65434150004040005329d00000000a00000001a104746573744040408000000000ffffffff",
	}
	for _, reply := range replies {
		if err := exchangeAMQPFrame(peer, reply); err != nil {
			return err
		}
	}
	return nil
}

func exchangeAMQPFrame(peer net.Conn, reply string) error {
	header := make([]byte, 8)
	if _, err := io.ReadFull(peer, header); err != nil {
		return err
	}
	size := int64(binary.BigEndian.Uint32(header))
	if size < 8 || size > 4096 {
		return fmt.Errorf("AMQP-TEST-FRAME unexpected frame size %d", size)
	}
	if _, err := io.CopyN(io.Discard, peer, size-8); err != nil {
		return err
	}
	frame, err := hex.DecodeString(reply)
	if err != nil {
		return err
	}
	_, err = peer.Write(frame)
	return err
}
