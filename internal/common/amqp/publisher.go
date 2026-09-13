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
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	wire "github.com/Azure/go-amqp"
)

// Publisher delivers immutable outbox envelopes to AMQP 1.0 destinations.
type Publisher struct {
	broker    string
	options   *wire.ConnOptions
	ctx       context.Context
	cancel    context.CancelFunc
	gate      chan struct{}
	done      chan struct{}
	cleanup   sync.WaitGroup
	connected atomic.Bool
	conn      *publisherConnection
	session   *wire.Session
	senders   map[string]*wire.Sender
}

// NewPublisher validates configuration and loads credentials without waiting for a broker.
func NewPublisher(ctx context.Context, cfg Config) (*Publisher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	opts, err := cfg.connectionOptions()
	if err != nil {
		return nil, err
	}
	// #nosec G118 -- Stop owns cancellation for the publisher lifetime.
	runtimeCtx, cancel := context.WithCancel(ctx)
	p := &Publisher{broker: cfg.Broker, options: opts, ctx: runtimeCtx, cancel: cancel, gate: make(chan struct{}, 1), done: make(chan struct{})}
	go p.shutdown()
	return p, nil
}
func (p *Publisher) shutdown() {
	defer close(p.done)
	<-p.ctx.Done()
	p.gate <- struct{}{}
	defer func() { <-p.gate }()
	p.disconnect()
	p.cleanup.Wait()
}
func (p *Publisher) disconnect() {
	p.connected.Store(false)
	if p.conn != nil {
		p.conn.invalidate()
	}
	p.conn, p.session, p.senders = nil, nil, nil
}

// Connected reports the latest connection/delivery status, or false after shutdown.
func (p *Publisher) Connected() bool { return p.ctx.Err() == nil && p.connected.Load() }

// Publish returns success only after the broker accepts the unchanged structured CloudEvent.
func (p *Publisher) Publish(ctx context.Context, routing json.RawMessage, envelope []byte) error {
	address, message, err := structuredMessage(routing, envelope)
	if err != nil {
		return err
	}
	publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stop := context.AfterFunc(p.ctx, cancel)
	defer stop()
	if p.ctx.Err() != nil {
		return fmt.Errorf("AMQP-PUBLISH-STOPPED publisher has stopped")
	}
	conn, sender, err := p.sender(publishCtx, address)
	if err != nil {
		return err
	}
	receipt, err := sender.SendWithReceipt(publishCtx, message, nil)
	if err == nil {
		var outcome wire.DeliveryState
		outcome, err = receipt.Wait(publishCtx)
		if err == nil {
			return accepted(outcome)
		}
	}
	p.connected.Store(false)
	conn.invalidate()
	return fmt.Errorf("AMQP-PUBLISH-DELIVERY delivery failed or timed out")
}
func accepted(outcome wire.DeliveryState) error {
	if _, ok := outcome.(*wire.StateAccepted); ok {
		return nil
	}
	return fmt.Errorf("AMQP-PUBLISH-OUTCOME broker did not accept delivery (%T)", outcome)
}
func (p *Publisher) sender(ctx context.Context, address string) (*publisherConnection, *wire.Sender, error) {
	select {
	case p.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("AMQP-PUBLISH-CANCELLED delivery cancelled")
	}
	defer func() { <-p.gate }()
	if ctx.Err() != nil || p.ctx.Err() != nil {
		return nil, nil, fmt.Errorf("AMQP-PUBLISH-CANCELLED delivery cancelled")
	}
	if p.conn != nil && !p.conn.active() {
		p.disconnect()
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if p.conn == nil {
		if err := p.connect(connectCtx); err != nil {
			return nil, nil, err
		}
	}
	sender := p.senders[address]
	if sender == nil {
		var err error
		mode := wire.SenderSettleModeUnsettled
		sender, err = p.session.NewSender(connectCtx, address, &wire.SenderOptions{SettlementMode: &mode})
		if err != nil {
			p.disconnect()
			return nil, nil, fmt.Errorf("AMQP-PUBLISH-LINK cannot attach destination")
		}
		p.senders[address] = sender
	}
	p.connected.Store(true)
	return p.conn, sender, nil
}
func (p *Publisher) connect(ctx context.Context) error {
	conn, err := wire.Dial(ctx, p.broker, p.options)
	if err != nil {
		return fmt.Errorf("AMQP-PUBLISH-CONNECT cannot connect or authenticate")
	}
	p.conn = p.trackConnection(conn)
	p.session, err = p.conn.NewSession(ctx, nil)
	if err != nil {
		p.disconnect()
		return fmt.Errorf("AMQP-PUBLISH-SESSION cannot create session")
	}
	p.senders = make(map[string]*wire.Sender)
	return nil
}

type publisherConnection struct {
	*wire.Conn
	closing chan struct{}
	once    sync.Once
}

func (c *publisherConnection) invalidate() { c.once.Do(func() { close(c.closing) }) }

func (c *publisherConnection) active() bool {
	select {
	case <-c.closing:
		return false
	case <-c.Done():
		return false
	default:
		return true
	}
}

func (p *Publisher) trackConnection(conn *wire.Conn) *publisherConnection {
	c := &publisherConnection{Conn: conn, closing: make(chan struct{})}
	p.cleanup.Go(func() {
		<-c.closing
		_ = conn.Close()
	})
	return c
}

// Stop cancels in-flight operations and closes the connection; repeated calls are safe.
func (p *Publisher) Stop(ctx context.Context) error {
	p.cancel()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("AMQP-PUBLISHER-STOP shutdown did not complete")
	}
}

type destination struct {
	Address string `json:"address"`
}

// Routing persists the broker address independently of future configuration changes.
func Routing(address string) (json.RawMessage, error) {
	if !validText(address) {
		return nil, fmt.Errorf("AMQP-ROUTING-ADDRESS invalid address")
	}
	raw, err := json.Marshal(destination{Address: address})
	if err != nil {
		return nil, fmt.Errorf("AMQP-ROUTING-JSON cannot encode destination")
	}
	return raw, nil
}
func structuredMessage(routing json.RawMessage, envelope []byte) (string, *wire.Message, error) {
	var target destination
	if err := json.Unmarshal(routing, &target); err != nil || !validText(target.Address) {
		return "", nil, fmt.Errorf("AMQP-PUBLISH-ROUTING invalid stored destination")
	}
	message := wire.NewMessage(envelope)
	message.Header = &wire.MessageHeader{Durable: true}
	contentType := "application/cloudevents+json"
	message.Properties = &wire.MessageProperties{ContentType: &contentType}
	return target.Address, message, nil
}
