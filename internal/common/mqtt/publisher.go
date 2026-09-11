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
	"log/slog"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

// Publisher sends structured CloudEvents to one MQTT destination.
type Publisher struct {
	connection *autopaho.ConnectionManager
	config     Config
	connected  atomic.Bool
}

// NewPublisher starts reconnecting asynchronously; broker availability does not gate startup.
func NewPublisher(ctx context.Context, cfg Config) (*Publisher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	tlsConfig, err := cfg.tlsConfig()
	if err != nil {
		return nil, err
	}
	username, err := readCredential(cfg.Username, cfg.UsernameFile)
	if err != nil {
		return nil, err
	}
	password, err := readCredential(cfg.Password, cfg.PasswordFile)
	if err != nil {
		return nil, err
	}
	broker, err := url.Parse(cfg.Broker)
	if err != nil {
		return nil, fmt.Errorf("MQTT-PUBLISHER-URL invalid broker URL")
	}
	publisher := &Publisher{config: cfg}
	connection, err := autopaho.NewConnection(ctx, autopaho.ClientConfig{
		ServerUrls: []*url.URL{broker}, TlsCfg: tlsConfig, KeepAlive: 30, ConnectTimeout: 10 * time.Second,
		CleanStartOnInitialConnection: true, SessionExpiryInterval: 0,
		ConnectUsername: username, ConnectPassword: []byte(password),
		ReconnectBackoff: events.RetryDelay,
		OnConnectionUp:   func(_ *autopaho.ConnectionManager, _ *paho.Connack) { publisher.connected.Store(true) },
		OnConnectionDown: func() bool { publisher.connected.Store(false); return true },
		OnConnectError: func(_ error) {
			slog.WarnContext(ctx, "MQTT connection unavailable; reconnecting", "error.code", "MQTT-CONNECT-RETRY", "sink", cfg.SinkID)
		},
		ClientConfig: paho.ClientConfig{ClientID: cfg.ClientID},
	})
	if err != nil {
		return nil, fmt.Errorf("MQTT-PUBLISHER-START cannot initialize connection")
	}
	publisher.connection = connection
	return publisher, nil
}

// Connected reports the current broker connection state.
func (p *Publisher) Connected() bool { return p.connected.Load() }

// Publish waits for the configured MQTT acknowledgment, never using a second durable queue.
func (p *Publisher) Publish(ctx context.Context, routing json.RawMessage, envelope []byte) error {
	var topic string
	if err := json.Unmarshal(routing, &topic); err != nil {
		return fmt.Errorf("MQTT-PUBLISH-ROUTING invalid stored topic")
	}
	format := byte(1)
	qos := byte(1)
	switch p.config.QoS {
	case 0:
		qos = 0
	case 2:
		qos = 2
	}
	response, err := p.connection.Publish(ctx, &paho.Publish{Topic: topic, QoS: qos, Retain: p.config.Retained, Payload: envelope, Properties: &paho.PublishProperties{ContentType: "application/cloudevents+json", PayloadFormat: &format}})
	if err != nil {
		return fmt.Errorf("MQTT-PUBLISH-DELIVERY broker delivery not acknowledged")
	}
	if response != nil && response.ReasonCode >= 0x80 {
		return fmt.Errorf("MQTT-PUBLISH-REJECTED broker rejected delivery (reason %d)", response.ReasonCode)
	}
	return nil
}

// Stop closes the broker connection.
func (p *Publisher) Stop(ctx context.Context) error {
	if err := p.connection.Disconnect(ctx); err != nil {
		return fmt.Errorf("MQTT-PUBLISHER-STOP disconnect did not complete")
	}
	return nil
}

// Routing maps the shared event to adapter-specific topic metadata.
func Routing(prefix string, event events.FeedEvent) (json.RawMessage, error) {
	component, resource, operation, err := topicParts(event.Type)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(strings.Join([]string{prefix, component, resource, operation}, "/"))
	if err != nil {
		return nil, fmt.Errorf("MQTT-ROUTING-JSON: %w", err)
	}
	return raw, nil
}
func topicParts(kind string) (string, string, string, error) {
	if kind == events.TypePCN {
		return "submodelrepository", "pcn", "notification", nil
	}
	for _, resource := range []string{"aas", "asset", "submodel"} {
		for _, operation := range []string{"created", "updated", "deleted"} {
			if kind == "io.admin-shell."+resource+"."+operation+".v1" {
				component := "aasrepository"
				if resource == "submodel" {
					component = "submodelrepository"
				}
				return component, resource, operation, nil
			}
		}
	}
	return "", "", "", fmt.Errorf("MQTT-ROUTING-TYPE unsupported event type")
}
