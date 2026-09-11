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
	"context"
	"encoding/json"
	"fmt"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/stretchr/testify/require"
	"net/url"
	"testing"
	"time"
)

// MQTTMessage is a decoded broker message used for integration assertions.
type MQTTMessage struct {
	Event       events.FeedRecord
	Topic       string
	ContentType string
	Retained    bool
}

// SubscribeMQTT connects a test subscriber and arranges bounded shutdown.
func SubscribeMQTT(t *testing.T, brokerURL, topic string) <-chan MQTTMessage {
	t.Helper()
	ctx := t.Context()
	broker, err := url.Parse(brokerURL)
	require.NoError(t, err)
	received := make(chan MQTTMessage, 4096)
	connection, err := autopaho.NewConnection(ctx, autopaho.ClientConfig{ServerUrls: []*url.URL{broker}, KeepAlive: 10, ClientConfig: paho.ClientConfig{
		ClientID: fmt.Sprintf("subscriber-%d", time.Now().UnixNano()),
		OnPublishReceived: []func(paho.PublishReceived) (bool, error){func(message paho.PublishReceived) (bool, error) {
			var event events.FeedRecord
			if err := json.Unmarshal(message.Packet.Payload, &event); err != nil {
				return false, err
			}
			item := MQTTMessage{Event: event, Topic: message.Packet.Topic, Retained: message.Packet.Retain}
			if message.Packet.Properties != nil {
				item.ContentType = message.Packet.Properties.ContentType
			}
			select {
			case received <- item:
			case <-ctx.Done():
			}
			return true, nil
		}},
	}})
	require.NoError(t, err)
	deadline, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	require.NoError(t, connection.AwaitConnection(deadline))
	_, err = connection.Subscribe(deadline, &paho.Subscribe{Subscriptions: []paho.SubscribeOptions{{Topic: topic, QoS: 1}}})
	require.NoError(t, err)
	t.Cleanup(func() {
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		require.NoError(t, connection.Disconnect(shutdown))
	})
	return received
}

// AwaitMQTT waits for one matching event while ignoring unrelated mutations.
func AwaitMQTT(t *testing.T, received <-chan MQTTMessage, subject, kind string) MQTTMessage {
	t.Helper()
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		select {
		case item := <-received:
			if item.Event.Subject == subject && item.Event.Type == kind {
				return item
			}
		case <-timer.C:
			t.Fatalf("MQTT event missing: %s %s", subject, kind)
			return MQTTMessage{}
		}
	}
}
