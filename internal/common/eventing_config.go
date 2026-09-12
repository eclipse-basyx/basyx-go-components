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
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/mqtt"
)

// MQTTEnabled checks whether eventing enables the MQTT sink.
//
// Returns:
//   - bool: True when eventing is enabled and sinks contains mqtt; full validation also requires the outbox.
func (c EventingConfig) MQTTEnabled() bool { return c.sinkEnabled("mqtt") }

// KafkaEnabled checks whether eventing enables the Kafka sink.
func (c EventingConfig) KafkaEnabled() bool { return c.sinkEnabled("kafka") }

// TransportsEnabled reports whether any asynchronous event transport is enabled.
func (c EventingConfig) TransportsEnabled() bool { return c.MQTTEnabled() || c.KafkaEnabled() }

func (c EventingConfig) sinkEnabled(name string) bool {
	if !c.Enabled {
		return false
	}
	for _, sink := range c.Sinks {
		if sink == name {
			return true
		}
	}
	return false
}
func validateEventTransports(c EventingConfig) error {
	if len(c.Sinks) == 0 {
		if c.OutboxEnabled {
			return fmt.Errorf("CONFIG-EVENTING-OUTBOX an outbox requires a configured sink")
		}
		return nil
	}
	if !c.Enabled || !c.OutboxEnabled {
		return fmt.Errorf("CONFIG-EVENTING-ACTIVATION transports require eventing.enabled and eventing.outboxEnabled")
	}
	names, ids := map[string]bool{}, map[string]bool{}
	for _, sink := range c.Sinks {
		if names[sink] {
			return fmt.Errorf("CONFIG-EVENTING-SINKS duplicate sink")
		}
		names[sink] = true
		id, err := validateEventTransport(c, sink)
		if err != nil {
			return err
		}
		if ids[id] {
			return fmt.Errorf("CONFIG-EVENTING-SINKID transports must use distinct sink IDs")
		}
		ids[id] = true
	}
	return nil
}
func validateEventTransport(c EventingConfig, sink string) (string, error) {
	switch sink {
	case "mqtt":
		if err := mqtt.ValidateTopicPrefix(c.TopicPrefix); err != nil {
			return "", err
		}
		return c.MQTT.SinkID, c.MQTT.Validate()
	case "kafka":
		return c.Kafka.SinkID, c.Kafka.Validate()
	default:
		return "", fmt.Errorf("CONFIG-EVENTING-SINKS supported sinks: mqtt, kafka")
	}
}

func validateEventURLs(c EventingConfig) error {
	pairs := [][2]string{{c.SourceBaseURL, c.Feed.SourceBaseURL}, {c.SchemaBaseURL, c.Feed.SchemaBaseURL}}
	for _, pair := range pairs {
		shared, legacy := normalizeEventURL(pair[0]), normalizeEventURL(pair[1])
		if shared != "" && legacy != "" && shared != legacy {
			return fmt.Errorf("CONFIG-EVENTING-URLCONFLICT shared and feed URL overrides disagree")
		}
		for _, value := range []string{shared, legacy} {
			if value == "" {
				continue
			}
			u, err := url.Parse(value)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return fmt.Errorf("CONFIG-EVENTING-URL source/schema URLs must be absolute HTTP(S) URLs without credentials, query or fragment")
			}
		}
	}
	return nil
}
func normalizeEventURL(s string) string { return strings.TrimRight(strings.TrimSpace(s), "/") }

func applyMQTTEnvOverrides(cfg *Config) error {
	c := &cfg.Eventing
	fields := map[string]*string{
		"BASYX_EVENTING_SOURCE_BASE_URL":       &c.SourceBaseURL,
		"BASYX_EVENTING_SCHEMA_BASE_URL":       &c.SchemaBaseURL,
		"BASYX_EVENTING_MQTT_BROKER":           &c.MQTT.Broker,
		"BASYX_EVENTING_MQTT_CLIENT_ID":        &c.MQTT.ClientID,
		"BASYX_EVENTING_MQTT_SINK_ID":          &c.MQTT.SinkID,
		"BASYX_EVENTING_MQTT_USERNAME":         &c.MQTT.Username,
		"BASYX_EVENTING_MQTT_PASSWORD":         &c.MQTT.Password,
		"BASYX_EVENTING_MQTT_USERNAME_FILE":    &c.MQTT.UsernameFile,
		"BASYX_EVENTING_MQTT_PASSWORD_FILE":    &c.MQTT.PasswordFile,
		"BASYX_EVENTING_MQTT_CA_FILE":          &c.MQTT.CAFile,
		"BASYX_EVENTING_MQTT_CERTIFICATE_FILE": &c.MQTT.CertificateFile,
		"BASYX_EVENTING_MQTT_KEY_FILE":         &c.MQTT.KeyFile,
	}
	for key, destination := range fields {
		if value, ok := os.LookupEnv(key); ok {
			*destination = value
		}
	}
	if value, ok := os.LookupEnv("BASYX_EVENTING_MQTT_QOS"); ok {
		qos, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("CONFIG-EVENTING-MQTTQOS invalid MQTT QoS")
		}
		c.MQTT.QoS = qos
	}
	if value, ok := os.LookupEnv("BASYX_EVENTING_MQTT_RETAINED"); ok {
		retained, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("CONFIG-EVENTING-MQTTRETAINED invalid MQTT retained flag")
		}
		c.MQTT.Retained = retained
	}
	return nil
}

func applyKafkaEnvOverrides(cfg *Config) error {
	c := &cfg.Eventing.Kafka
	fields := map[string]*string{
		"TOPIC": &c.Topic, "CLIENT_ID": &c.ClientID, "SINK_ID": &c.SinkID, "SASL_MECHANISM": &c.SASLMechanism,
		"USERNAME": &c.Username, "PASSWORD": &c.Password, "USERNAME_FILE": &c.UsernameFile, "PASSWORD_FILE": &c.PasswordFile,
		"CA_FILE": &c.CAFile, "CERTIFICATE_FILE": &c.CertificateFile, "KEY_FILE": &c.KeyFile,
	}
	for key, destination := range fields {
		if value, ok := os.LookupEnv("BASYX_EVENTING_KAFKA_" + key); ok {
			*destination = value
		}
	}
	if value, ok := os.LookupEnv("BASYX_EVENTING_KAFKA_BROKERS"); ok {
		c.Brokers = strings.Split(value, ",")
		for i := range c.Brokers {
			c.Brokers[i] = strings.TrimSpace(c.Brokers[i])
		}
	}
	if value, ok := os.LookupEnv("BASYX_EVENTING_KAFKA_TLS_ENABLED"); ok {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("CONFIG-EVENTING-KAFKATLS invalid TLS enabled flag")
		}
		c.TLSEnabled = enabled
	}
	return nil
}
