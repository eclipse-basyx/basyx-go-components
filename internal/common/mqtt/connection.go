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
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"golang.org/x/net/proxy"
)

const publishTimeout = 10 * time.Second

type brokerConnection struct {
	net.Conn
	sync.Mutex
	ctx              context.Context
	cancel           context.CancelFunc
	stopCancellation func() bool
}

func newBrokerConnection(ctx context.Context, conn net.Conn) *brokerConnection {
	connectionCtx, cancel := context.WithCancel(ctx)
	return &brokerConnection{
		ctx: connectionCtx, cancel: cancel,
		Conn:             conn,
		stopCancellation: context.AfterFunc(connectionCtx, func() { _ = conn.Close() }),
	}
}

func (c *brokerConnection) Write(data []byte) (int, error) {
	if err := c.SetWriteDeadline(time.Now().Add(publishTimeout)); err != nil {
		return 0, fmt.Errorf("MQTT-CONNECTION-DEADLINE: %w", err)
	}
	return c.Conn.Write(data)
}

func (c *brokerConnection) Close() error {
	c.cancel()
	c.stopCancellation()
	return c.Conn.Close()
}

func (p *Publisher) dial(ctx context.Context, cfg autopaho.ClientConfig, broker *url.URL) (net.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	conn, err := dialBroker(dialCtx, broker, cfg.TlsCfg)
	if err != nil {
		return nil, err
	}
	transport := newBrokerConnection(ctx, conn)
	p.transportMu.Lock()
	defer p.transportMu.Unlock()
	if err := ctx.Err(); err != nil {
		_ = transport.Close()
		return nil, fmt.Errorf("MQTT-CONNECTION-CANCELLED: %w", err)
	}
	p.transport = transport
	return transport, nil
}

func dialBroker(ctx context.Context, broker *url.URL, tlsConfig *tls.Config) (net.Conn, error) {
	conn, err := dialTCP(ctx, broker.Host)
	if err != nil {
		return nil, fmt.Errorf("MQTT-CONNECTION-DIAL: %w", err)
	}
	if broker.Scheme != "tls" {
		return conn, nil
	}
	cfg := tlsConfig.Clone()
	cfg.ServerName = broker.Hostname()
	secure := tls.Client(conn, cfg)
	if err := secure.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("MQTT-CONNECTION-TLS: %w", err)
	}
	return secure, nil
}

func dialTCP(ctx context.Context, address string) (net.Conn, error) {
	if os.Getenv("all_proxy") != "" {
		return proxy.Dial(ctx, "tcp", address)
	}
	return (&net.Dialer{}).DialContext(ctx, "tcp", address)
}

func (p *Publisher) publish(ctx context.Context, packet *paho.Publish) (*paho.PublishResponse, error) {
	publishCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if !p.transportMu.TryRLock() {
		return nil, autopaho.ConnectionDownError
	}
	defer p.transportMu.RUnlock()
	if err := publishCtx.Err(); err != nil {
		return nil, err
	}
	if p.transport == nil {
		return nil, autopaho.ConnectionDownError
	}
	transport := p.transport
	stopLifecycle := context.AfterFunc(transport.ctx, cancel)
	defer stopLifecycle()
	stop := context.AfterFunc(publishCtx, func() { _ = transport.Close() })
	defer stop()
	return p.connection.Publish(publishCtx, packet)
}
