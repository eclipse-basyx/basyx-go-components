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

// Package kafka implements the CloudEvents Kafka structured JSON binding.
package kafka

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/brokersecurity"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// Config contains one Kafka destination and its connection settings.
type Config struct {
	Brokers         []string `mapstructure:"brokers" yaml:"brokers" json:"brokers"`
	Topic           string   `mapstructure:"topic" yaml:"topic" json:"topic"`
	SinkID          string   `mapstructure:"sinkId" yaml:"sinkId" json:"sinkId"`
	ClientID        string   `mapstructure:"clientId" yaml:"clientId" json:"clientId"`
	TLSEnabled      bool     `mapstructure:"tlsEnabled" yaml:"tlsEnabled" json:"tlsEnabled"`
	CAFile          string   `mapstructure:"caFile" yaml:"caFile" json:"caFile"`
	CertificateFile string   `mapstructure:"certificateFile" yaml:"certificateFile" json:"certificateFile"`
	KeyFile         string   `mapstructure:"keyFile" yaml:"keyFile" json:"-"`
	SASLMechanism   string   `mapstructure:"saslMechanism" yaml:"saslMechanism" json:"saslMechanism"`
	Username        string   `mapstructure:"username" yaml:"username" json:"-"`
	Password        string   `mapstructure:"password" yaml:"password" json:"-"`
	UsernameFile    string   `mapstructure:"usernameFile" yaml:"usernameFile" json:"-"`
	PasswordFile    string   `mapstructure:"passwordFile" yaml:"passwordFile" json:"-"`

	// ProducerBatchMaxBytes limits uncompressed batches; zero uses the Kafka client default.
	ProducerBatchMaxBytes int32 `mapstructure:"producerBatchMaxBytes" yaml:"producerBatchMaxBytes" json:"producerBatchMaxBytes"`
}

// Validate checks configuration without contacting Kafka or reading files.
func (c Config) Validate() error {
	if err := validateBrokers(c.Brokers); err != nil {
		return err
	}
	if !validText(c.SinkID) || !validText(c.ClientID) {
		return fmt.Errorf("KAFKA-CONFIG-IDENTIFIER clientId and sinkId must be nonempty valid strings")
	}
	if !validTopic(c.Topic) {
		return fmt.Errorf("KAFKA-CONFIG-TOPIC invalid topic name")
	}
	if c.ProducerBatchMaxBytes != 0 && (c.ProducerBatchMaxBytes < 512 || c.ProducerBatchMaxBytes > 1<<30) {
		return fmt.Errorf("KAFKA-CONFIG-BATCHSIZE producerBatchMaxBytes must be zero or between 512 and 1073741824")
	}
	if (c.CertificateFile == "") != (c.KeyFile == "") {
		return fmt.Errorf("KAFKA-CONFIG-CERTPAIR certificateFile and keyFile must be supplied together")
	}
	if !c.TLSEnabled && (c.CAFile != "" || c.CertificateFile != "") {
		return fmt.Errorf("KAFKA-CONFIG-TLS TLS files require tlsEnabled")
	}
	return c.validateSASL()
}
func validateBrokers(brokers []string) error {
	if len(brokers) == 0 {
		return fmt.Errorf("KAFKA-CONFIG-BROKERS at least one bootstrap broker is required")
	}
	for _, broker := range brokers {
		host, port, err := net.SplitHostPort(broker)
		number, parseErr := strconv.Atoi(port)
		if err != nil || parseErr != nil || host == "" || strings.ContainsAny(host, "/@?# \t\r\n") || number < 1 || number > 65535 {
			return fmt.Errorf("KAFKA-CONFIG-BROKER brokers must be host:port addresses without credentials")
		}
	}
	return nil
}
func validText(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value) && !strings.ContainsRune(value, 0) && len(value) <= 32767
}
func validTopic(topic string) bool {
	if len(topic) == 0 || len(topic) > 249 || topic == "." || topic == ".." {
		return false
	}
	for _, c := range topic {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
func (c Config) validateSASL() error {
	if c.Username != "" && c.UsernameFile != "" || c.Password != "" && c.PasswordFile != "" {
		return fmt.Errorf("KAFKA-CONFIG-CREDENTIALS use either a credential value or its file")
	}
	hasUser := c.Username != "" || c.UsernameFile != ""
	hasPassword := c.Password != "" || c.PasswordFile != ""
	switch c.SASLMechanism {
	case "":
		if hasUser || hasPassword {
			return fmt.Errorf("KAFKA-CONFIG-SASL credentials require saslMechanism")
		}
	case "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512":
		if !hasUser || !hasPassword {
			return fmt.Errorf("KAFKA-CONFIG-SASL username and password are required")
		}
	default:
		return fmt.Errorf("KAFKA-CONFIG-SASL supported mechanisms: PLAIN, SCRAM-SHA-256, SCRAM-SHA-512")
	}
	return nil
}
func (c Config) mechanism() (sasl.Mechanism, error) {
	username, err := brokersecurity.ReadCredential("KAFKA", c.Username, c.UsernameFile)
	if err != nil {
		return nil, err
	}
	password, err := brokersecurity.ReadCredential("KAFKA", c.Password, c.PasswordFile)
	if err != nil {
		return nil, err
	}
	if username == "" || password == "" {
		return nil, fmt.Errorf("KAFKA-CONFIG-SASL credentials must not be empty")
	}
	switch c.SASLMechanism {
	case "PLAIN":
		return plain.Auth{User: username, Pass: password}.AsMechanism(), nil
	case "SCRAM-SHA-256":
		return scram.Auth{User: username, Pass: password}.AsSha256Mechanism(), nil
	default:
		return scram.Auth{User: username, Pass: password}.AsSha512Mechanism(), nil
	}
}
func (c Config) clientOptions() ([]kgo.Opt, error) {
	opts := []kgo.Opt{kgo.SeedBrokers(c.Brokers...), kgo.ClientID(c.ClientID), kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)), kgo.AllowIdempotentProduceCancellation(),
		kgo.RecordDeliveryTimeout(10 * time.Second), kgo.ProduceRequestTimeout(5 * time.Second), kgo.RequestTimeoutOverhead(time.Second),
		kgo.DialTimeout(5 * time.Second), kgo.MaxBufferedRecords(4)}
	if c.ProducerBatchMaxBytes != 0 {
		opts = append(opts, kgo.ProducerBatchMaxBytes(c.ProducerBatchMaxBytes), kgo.BrokerMaxWriteBytes(max(100<<20, c.ProducerBatchMaxBytes)))
	}
	if c.TLSEnabled {
		tlsConfig, err := brokersecurity.TLSConfig("KAFKA", c.CAFile, c.CertificateFile, c.KeyFile)
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.DialTLSConfig(tlsConfig))
	}
	if c.SASLMechanism != "" {
		mechanism, err := c.mechanism()
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.SASL(mechanism))
	}
	return opts, nil
}
