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
	"log/slog"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventoutbox"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/mqtt"
)

// Bind registers a feed-only mutation consumer for existing embedded callers.
func Bind(module *eventfeed.Module) {
	if module == nil || !module.Enabled() {
		return
	}
	history.SetMutationSink(eventfeed.NewMutationSink(module.Service))
	module.SetOnStop(history.ClearMutationSink)
}

// Start binds all enabled event sinks before startup imports and starts asynchronous delivery.
func Start(ctx context.Context, db *sql.DB, cfg *common.Config, feed *eventfeed.Module) error {
	if !cfg.Eventing.MQTTEnabled() {
		Bind(feed)
		return nil
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	publisher, err := mqtt.NewPublisher(runtimeCtx, cfg.Eventing.MQTT)
	if err != nil {
		cancel()
		return err
	}
	repository := eventoutbox.NewRepository(db)
	worker, err := eventoutbox.Start(runtimeCtx, repository, cfg.Eventing.MQTT.SinkID, cfg.Eventing.MQTT.ClientID, publisher)
	if err != nil {
		cancel()
		stopPublisher(ctx, publisher)
		return err
	}
	feedConfig := common.NewEventFeedConfig(cfg)
	writers := []events.EventWriter{}
	if feed.Enabled() {
		writers = append(writers, func(ctx context.Context, tx *sql.Tx, _ events.Mutation, event events.FeedEvent) error {
			return feed.Service.WriteTx(ctx, tx, event)
		})
	}
	writers = append(writers, func(ctx context.Context, tx *sql.Tx, mutation events.Mutation, event events.FeedEvent) error {
		routing, err := mqtt.Routing(cfg.Eventing.TopicPrefix, event)
		if err != nil {
			return err
		}
		return repository.Enqueue(ctx, tx, cfg.Eventing.MQTT.SinkID, mutation.Table+":"+mutation.Identifier, event, routing)
	})
	builder := events.NewBuilder(events.Config{SourceBaseURL: feedConfig.SourceBaseURL, SchemaBaseURL: feedConfig.SchemaBaseURL})
	history.SetMutationSink(events.NewMutationSink(builder, events.Fanout(writers...)))
	feed.SetOnStop(func() { history.ClearMutationSink(); worker.Stop(); cancel(); stopPublisher(ctx, publisher) })
	return nil
}

func stopPublisher(ctx context.Context, publisher *mqtt.Publisher) {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := publisher.Stop(shutdownCtx); err != nil {
		slog.WarnContext(shutdownCtx, "MQTT shutdown incomplete", "error.code", "EVENTING-STOP-MQTT", "error", err)
	}
}
