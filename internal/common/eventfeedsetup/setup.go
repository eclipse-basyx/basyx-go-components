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

// Package eventfeedsetup connects shared event generation to the history mutation hook.
package eventfeedsetup

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventoutbox"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/kafka"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/mqtt"
)

// Bind registers an enabled feed as the process mutation consumer.
//
// The consumer is cleared when the module stops. A nil or disabled module is
// ignored.
//
// Parameters:
//   - module: Initialized feed module whose service persists captured events.
func Bind(module *eventfeed.Module) {
	if module == nil || !module.Enabled() {
		return
	}
	history.SetMutationSink(eventfeed.NewMutationSink(module.Service))
	module.SetOnStop(history.ClearMutationSink)
}

type publisher interface {
	eventoutbox.Publisher
	Stop(context.Context) error
}
type transport struct {
	sink      string
	publisher publisher
	routing   func(events.Mutation, events.FeedEvent) (json.RawMessage, error)
	worker    *eventoutbox.Worker
}

// Start registers transaction-scoped event writers and starts enabled transports.
// Call before startup imports. The feed module (even when disabled) owns shutdown.
// Invalid local settings fail startup; broker outages leave deliveries pending.
func Start(ctx context.Context, db *sql.DB, cfg *common.Config, feed *eventfeed.Module) error {
	if !cfg.Eventing.TransportsEnabled() {
		Bind(feed)
		return nil
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	repository := eventoutbox.NewRepository(db)
	transports, err := startTransports(runtimeCtx, repository, cfg.Eventing)
	if err != nil {
		cancel()
		stopTransports(ctx, transports)
		return err
	}
	writers := eventWriters(repository, feed, transports)
	feedConfig := common.NewEventFeedConfig(cfg)
	builder := events.NewBuilder(events.Config{SourceBaseURL: feedConfig.SourceBaseURL, SchemaBaseURL: feedConfig.SchemaBaseURL})
	history.SetMutationSink(events.NewMutationSink(builder, events.Fanout(writers...)))
	feed.SetOnStop(func() { history.ClearMutationSink(); cancel(); stopTransports(ctx, transports) })
	return nil
}
func startTransports(ctx context.Context, repository *eventoutbox.Repository, cfg common.EventingConfig) ([]*transport, error) {
	transports := []*transport{}
	for _, name := range cfg.Sinks {
		transport, err := newTransport(ctx, cfg, name)
		if err != nil {
			return transports, err
		}
		transports = append(transports, transport)
		transport.worker, err = eventoutbox.Start(ctx, repository, transport.sink, transport.publisher)
		if err != nil {
			return transports, err
		}
	}
	return transports, nil
}
func newTransport(ctx context.Context, cfg common.EventingConfig, name string) (*transport, error) {
	switch name {
	case "mqtt":
		p, err := mqtt.NewPublisher(ctx, cfg.MQTT)
		return &transport{sink: cfg.MQTT.SinkID, publisher: p, routing: func(_ events.Mutation, event events.FeedEvent) (json.RawMessage, error) {
			return mqtt.Routing(cfg.TopicPrefix, event)
		}}, err
	case "kafka":
		p, err := kafka.NewPublisher(ctx, cfg.Kafka)
		return &transport{sink: cfg.Kafka.SinkID, publisher: p, routing: func(mutation events.Mutation, _ events.FeedEvent) (json.RawMessage, error) {
			return kafka.Routing(cfg.Kafka.Topic, orderingKey(mutation))
		}}, err
	default:
		return nil, fmt.Errorf("EVENTING-START-TRANSPORT unsupported sink")
	}
}
func orderingKey(mutation events.Mutation) string { return mutation.Table + ":" + mutation.Identifier }
func eventWriters(repository *eventoutbox.Repository, feed *eventfeed.Module, transports []*transport) []events.EventWriter {
	writers := []events.EventWriter{}
	if feed.Enabled() {
		writers = append(writers, func(ctx context.Context, tx *sql.Tx, _ events.Mutation, event events.FeedEvent) error {
			return feed.Service.WriteTx(ctx, tx, event)
		})
	}
	for _, transport := range transports {
		writers = append(writers, func(ctx context.Context, tx *sql.Tx, mutation events.Mutation, event events.FeedEvent) error {
			routing, err := transport.routing(mutation, event)
			if err != nil {
				return err
			}
			return repository.Enqueue(ctx, tx, transport.sink, orderingKey(mutation), event, routing)
		})
	}
	return writers
}
func stopTransports(ctx context.Context, transports []*transport) {
	for _, transport := range transports {
		if transport.worker != nil {
			transport.worker.Stop()
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	for _, transport := range transports {
		if err := transport.publisher.Stop(shutdownCtx); err != nil {
			slog.WarnContext(shutdownCtx, "event transport shutdown incomplete", "error.code", "EVENTING-STOP-TRANSPORT", "sink", transport.sink, "error", err)
		}
	}
}
