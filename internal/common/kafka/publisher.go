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

package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Publisher delivers immutable outbox envelopes to Kafka.
type Publisher struct {
	client    *kgo.Client
	ctx       context.Context
	cancel    context.CancelFunc
	connected atomic.Bool
	done      chan struct{}
}

// NewPublisher validates settings and loads credentials without waiting for a broker.
// ctx controls the producer lifetime. Stop must be called before closing the database.
func NewPublisher(ctx context.Context, cfg Config) (*Publisher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	opts, err := cfg.clientOptions()
	if err != nil {
		return nil, err
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("KAFKA-PUBLISHER-START cannot initialize producer")
	}
	// #nosec G118 -- Stop owns cancellation for the producer lifetime.
	runtimeCtx, cancel := context.WithCancel(ctx)
	p := &Publisher{client: client, ctx: runtimeCtx, cancel: cancel, done: make(chan struct{})}
	go p.observe()
	return p, nil
}
func (p *Publisher) observe() {
	defer close(p.done)
	defer p.client.Close()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
		err := p.client.Ping(ctx)
		cancel()
		p.connected.Store(err == nil)
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Connected reports whether the latest broker probe or delivery succeeded.
func (p *Publisher) Connected() bool { return p.ctx.Err() == nil && p.connected.Load() }

// Publish waits for all in-sync replicas to acknowledge one structured CloudEvent.
// routing is produced by Routing; envelope is transmitted unchanged. ctx bounds
// delivery to at most ten seconds. Errors leave the outbox row pending for retry.
func (p *Publisher) Publish(ctx context.Context, routing json.RawMessage, envelope []byte) error {
	record, err := structuredRecord(routing, envelope)
	if err != nil {
		return err
	}
	publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stop := context.AfterFunc(p.ctx, cancel)
	defer stop()
	if p.ctx.Err() != nil {
		return fmt.Errorf("KAFKA-PUBLISH-STOPPED publisher has stopped")
	}
	err = p.client.ProduceSync(publishCtx, record).FirstErr()
	p.connected.Store(err == nil)
	if err != nil {
		return fmt.Errorf("KAFKA-PUBLISH-DELIVERY broker delivery not acknowledged")
	}
	return nil
}

// Stop cancels deliveries and closes the producer. ctx bounds the wait; repeated calls are safe.
func (p *Publisher) Stop(ctx context.Context) error {
	p.cancel()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("KAFKA-PUBLISHER-STOP shutdown did not complete")
	}
}

type destination struct {
	Topic string `json:"topic"`
	Key   string `json:"key"`
}

// Routing persists the configured topic and the mutation's unmodified ordering key.
// Neither the key mapping nor publishing adds attributes to the CloudEvent.
func Routing(topic, key string) (json.RawMessage, error) {
	if !validTopic(topic) || !validText(key) {
		return nil, fmt.Errorf("KAFKA-ROUTING-DESTINATION invalid topic or key")
	}
	raw, err := json.Marshal(destination{Topic: topic, Key: key})
	if err != nil {
		return nil, fmt.Errorf("KAFKA-ROUTING-JSON cannot encode destination")
	}
	return raw, nil
}
func structuredRecord(routing json.RawMessage, envelope []byte) (*kgo.Record, error) {
	var target destination
	if err := json.Unmarshal(routing, &target); err != nil {
		return nil, fmt.Errorf("KAFKA-PUBLISH-ROUTING invalid stored destination")
	}
	if !validTopic(target.Topic) || !validText(target.Key) {
		return nil, fmt.Errorf("KAFKA-PUBLISH-ROUTING invalid stored topic or key")
	}
	return &kgo.Record{Topic: target.Topic, Key: []byte(target.Key), Value: envelope, Headers: []kgo.RecordHeader{{Key: "content-type", Value: []byte("application/cloudevents+json")}}}, nil
}
