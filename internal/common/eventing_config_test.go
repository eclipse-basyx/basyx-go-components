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

package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/mqtt"
	"github.com/stretchr/testify/require"
)

func TestMQTTConfigDefaultsAndActivation(t *testing.T) {
	cfg, err := LoadConfig("")
	require.NoError(t, err)
	require.False(t, cfg.Eventing.MQTTEnabled())
	require.Equal(t, 1, cfg.Eventing.MQTT.QoS)
	require.Equal(t, "mqtt", cfg.Eventing.MQTT.SinkID)
	for _, feed := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			c := cfg.Eventing
			c.Feed.Enabled = feed
			c.Enabled = enabled
			if enabled {
				c.Sinks = []string{"mqtt"}
				c.OutboxEnabled = true
				c.MQTT.Broker = "mqtt://localhost:1883"
				c.MQTT.ClientID = "test"
			}
			require.NoError(t, validateEventingConfig(c))
			require.Equal(t, enabled, c.MQTTEnabled())
		}
	}
}
func TestMQTTEnvironmentAndSecrets(t *testing.T) {
	t.Setenv("BASYX_EVENTING_ENABLED", "true")
	t.Setenv("BASYX_EVENTING_SINKS", "mqtt")
	t.Setenv("BASYX_EVENTING_OUTBOX_ENABLED", "true")
	t.Setenv("BASYX_EVENTING_MQTT_BROKER", "tls://localhost:8883")
	t.Setenv("BASYX_EVENTING_MQTT_CLIENT_ID", "replica-1")
	t.Setenv("BASYX_EVENTING_MQTT_PASSWORD", " private secret ")
	t.Setenv("BASYX_EVENTING_MQTT_USERNAME", "private-user")
	t.Setenv("BASYX_EVENTING_MQTT_QOS", "0")
	t.Setenv("BASYX_EVENTING_MQTT_RETAINED", "true")
	cfg, err := LoadConfig("")
	require.NoError(t, err)
	require.Zero(t, cfg.Eventing.MQTT.QoS)
	require.True(t, cfg.Eventing.MQTT.Retained)
	require.Equal(t, " private secret ", cfg.Eventing.MQTT.Password)
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private secret")
	require.NotContains(t, string(raw), "private-user")
	t.Setenv("BASYX_EVENTING_MQTT_QOS", "invalid")
	_, err = LoadConfig("")
	require.ErrorContains(t, err, "CONFIG-EVENTING-MQTTQOS")
}
func TestMQTTYAMLAndURLAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`eventing:
  enabled: true
  sinks: [mqtt]
  outboxEnabled: true
  sourceBaseUrl: https://example.com/api
  mqtt:
    broker: mqtt://localhost:1883
    clientId: example
    qos: 0
`), 0600))
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Zero(t, cfg.Eventing.MQTT.QoS)
	resolved := NewEventFeedConfig(cfg)
	require.False(t, resolved.Enabled)
	require.True(t, resolved.SchemasEnabled)
	require.Equal(t, "https://example.com/api", resolved.SourceBaseURL)
	cfg.Eventing.Feed.SourceBaseURL = "https://example.com/api/"
	require.NoError(t, validateEventingConfig(cfg.Eventing))
	cfg.Eventing.Feed.SourceBaseURL = "https://another.example"
	require.ErrorContains(t, validateEventingConfig(cfg.Eventing), "URLCONFLICT")
}
func TestMQTTRejectsInvalidActivation(t *testing.T) {
	valid := EventingConfig{Enabled: true, OutboxEnabled: true, Sinks: []string{"mqtt"}, TopicPrefix: "basyx", MQTT: mqtt.Config{Broker: "mqtt://localhost:1883", ClientID: "test", SinkID: "mqtt", QoS: 1}}
	cases := []func(*EventingConfig){
		func(c *EventingConfig) { c.Enabled = false }, func(c *EventingConfig) { c.OutboxEnabled = false },
		func(c *EventingConfig) { c.Sinks = []string{"kafka"} }, func(c *EventingConfig) { c.Sinks = []string{"mqtt", "mqtt"} },
		func(c *EventingConfig) { c.MQTT.QoS = 3 }, func(c *EventingConfig) { c.TopicPrefix = "basyx/#" },
		func(c *EventingConfig) { c.MQTT.ClientID = "" }, func(c *EventingConfig) { c.MQTT.Broker = "mqtt://user:secret@host" },
	}
	for _, mutate := range cases {
		c := valid
		mutate(&c)
		err := validateEventingConfig(c)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}

func TestKafkaEnvironmentAndActivation(t *testing.T) {
	cfg, err := LoadConfig("")
	require.NoError(t, err)
	require.False(t, cfg.Eventing.KafkaEnabled())
	require.Equal(t, "basyx.events", cfg.Eventing.Kafka.Topic)
	require.Equal(t, "kafka", cfg.Eventing.Kafka.SinkID)
	require.Equal(t, "basyx", cfg.Eventing.Kafka.ClientID)
	t.Setenv("BASYX_EVENTING_ENABLED", "true")
	t.Setenv("BASYX_EVENTING_OUTBOX_ENABLED", "true")
	t.Setenv("BASYX_EVENTING_SINKS", "kafka,mqtt")
	t.Setenv("BASYX_EVENTING_KAFKA_BROKERS", "localhost:9092, localhost:9093")
	t.Setenv("BASYX_EVENTING_KAFKA_TOPIC", "custom.events")
	t.Setenv("BASYX_EVENTING_KAFKA_TLS_ENABLED", "true")
	t.Setenv("BASYX_EVENTING_KAFKA_SASL_MECHANISM", "SCRAM-SHA-256")
	t.Setenv("BASYX_EVENTING_KAFKA_USERNAME", "private-user")
	t.Setenv("BASYX_EVENTING_KAFKA_PASSWORD", "private-secret")
	t.Setenv("BASYX_EVENTING_MQTT_BROKER", "mqtt://localhost:1883")
	t.Setenv("BASYX_EVENTING_MQTT_CLIENT_ID", "test")
	cfg, err = LoadConfig("")
	require.NoError(t, err)
	require.True(t, cfg.Eventing.KafkaEnabled())
	require.True(t, cfg.Eventing.MQTTEnabled())
	require.Equal(t, []string{"localhost:9092", "localhost:9093"}, cfg.Eventing.Kafka.Brokers)
	require.Equal(t, "custom.events", cfg.Eventing.Kafka.Topic)
	require.True(t, cfg.Eventing.Kafka.TLSEnabled)
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private-")
	cfg.Eventing.Kafka.SinkID = "mqtt"
	require.ErrorContains(t, validateEventingConfig(cfg.Eventing), "SINKID")
	cfg.Eventing.Sinks = []string{"kafka", "kafka"}
	require.ErrorContains(t, validateEventingConfig(cfg.Eventing), "duplicate")
	cfg.Eventing.Sinks = []string{"kafka"}
	require.True(t, NewEventFeedConfig(cfg).SchemasEnabled)
	t.Setenv("BASYX_EVENTING_KAFKA_TLS_ENABLED", "invalid")
	_, err = LoadConfig("")
	require.ErrorContains(t, err, "KAFKATLS")
}

func TestKafkaProducerBatchSizeConfiguration(t *testing.T) {
	cfg, err := LoadConfig("")
	require.NoError(t, err)
	require.Zero(t, cfg.Eventing.Kafka.ProducerBatchMaxBytes)
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`eventing:
  enabled: true
  sinks: [kafka]
  outboxEnabled: true
  kafka:
    brokers: [localhost:9092]
    producerBatchMaxBytes: 2097152
`), 0600))
	cfg, err = LoadConfig(path)
	require.NoError(t, err)
	require.EqualValues(t, 2<<20, cfg.Eventing.Kafka.ProducerBatchMaxBytes)
	t.Setenv("BASYX_EVENTING_KAFKA_PRODUCER_BATCH_MAX_BYTES", "4194304")
	cfg, err = LoadConfig(path)
	require.NoError(t, err)
	require.EqualValues(t, 4<<20, cfg.Eventing.Kafka.ProducerBatchMaxBytes)
	for _, value := range []string{"invalid", "2147483648", "-1", "511", "1073741825"} {
		t.Setenv("BASYX_EVENTING_KAFKA_PRODUCER_BATCH_MAX_BYTES", value)
		_, err = LoadConfig(path)
		require.ErrorContains(t, err, "BATCHSIZE")
	}
}

func TestAMQPConfiguration(t *testing.T) {
	cfg := &Config{}
	applyAMQPEnvOverrides(cfg)
	t.Setenv("BASYX_EVENTING_AMQP_BROKER", "amqp://localhost:5672")
	t.Setenv("BASYX_EVENTING_AMQP_ADDRESS", "/queues/events")
	t.Setenv("BASYX_EVENTING_AMQP_SINK_ID", "amqp")
	t.Setenv("BASYX_EVENTING_AMQP_HOST_NAME", "vhost:test")
	t.Setenv("BASYX_EVENTING_AMQP_USERNAME", "user")
	t.Setenv("BASYX_EVENTING_AMQP_PASSWORD", "secret")
	applyAMQPEnvOverrides(cfg)
	require.Equal(t, "vhost:test", cfg.Eventing.AMQP.HostName)
	require.Equal(t, "secret", cfg.Eventing.AMQP.Password)
	cfg.Eventing.Enabled = true
	cfg.Eventing.OutboxEnabled = true
	cfg.Eventing.Sinks = []string{"amqp"}
	require.True(t, cfg.Eventing.AMQPEnabled())
	require.True(t, cfg.Eventing.TransportsEnabled())
	require.NoError(t, validateEventTransports(cfg.Eventing))
	cfg.Eventing.Sinks = []string{"amqp", "amqp"}
	require.Error(t, validateEventTransports(cfg.Eventing))
	cfg.Eventing.Sinks = []string{"amqp"}
	cfg.Eventing.OutboxEnabled = false
	require.Error(t, validateEventTransports(cfg.Eventing))
}

func TestAMQPYAMLDefaultsAndSinkIsolation(t *testing.T) {
	cfg, err := LoadConfig("")
	require.NoError(t, err)
	require.Equal(t, "amqp", cfg.Eventing.AMQP.SinkID)
	require.False(t, cfg.Eventing.AMQPEnabled())
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`eventing:
  enabled: true
  sinks: [amqp]
  outboxEnabled: true
  amqp:
    broker: amqp://localhost:5672
    address: /queues/events
`), 0600))
	cfg, err = LoadConfig(path)
	require.NoError(t, err)
	require.True(t, cfg.Eventing.AMQPEnabled())
	require.Equal(t, "/queues/events", cfg.Eventing.AMQP.Address)
	cfg.Eventing.Sinks = []string{"amqp", "kafka"}
	cfg.Eventing.Kafka.Brokers = []string{"localhost:9092"}
	cfg.Eventing.Kafka.SinkID = "amqp"
	require.ErrorContains(t, validateEventTransports(cfg.Eventing), "CONFIG-EVENTING-SINKID")
	cfg.Eventing.Kafka.SinkID = "kafka"
	require.NoError(t, validateEventTransports(cfg.Eventing))
	t.Setenv("BASYX_EVENTING_AMQP_ADDRESS", "/queues/override")
	t.Setenv("BASYX_EVENTING_AMQP_USERNAME", "private-amqp-user")
	t.Setenv("BASYX_EVENTING_AMQP_PASSWORD", "private-amqp-secret")
	cfg, err = LoadConfig(path)
	require.NoError(t, err)
	require.Equal(t, "/queues/override", cfg.Eventing.AMQP.Address)
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private-amqp-user")
	require.NotContains(t, string(raw), "private-amqp-secret")
}
