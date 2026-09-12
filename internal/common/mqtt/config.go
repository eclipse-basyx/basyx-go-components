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

// Package mqtt implements the CloudEvents MQTT 5 structured binding.
package mqtt

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"
)

// Config contains MQTT destination and connection settings.
type Config struct {
	Broker          string `mapstructure:"broker" yaml:"broker" json:"broker"`
	ClientID        string `mapstructure:"clientId" yaml:"clientId" json:"clientId"`
	SinkID          string `mapstructure:"sinkId" yaml:"sinkId" json:"sinkId"`
	QoS             int    `mapstructure:"qos" yaml:"qos" json:"qos"`
	Retained        bool   `mapstructure:"retained" yaml:"retained" json:"retained"`
	Username        string `mapstructure:"username" yaml:"username" json:"-"`
	Password        string `mapstructure:"password" yaml:"password" json:"-"`
	UsernameFile    string `mapstructure:"usernameFile" yaml:"usernameFile" json:"-"`
	PasswordFile    string `mapstructure:"passwordFile" yaml:"passwordFile" json:"-"`
	CAFile          string `mapstructure:"caFile" yaml:"caFile" json:"caFile"`
	CertificateFile string `mapstructure:"certificateFile" yaml:"certificateFile" json:"certificateFile"`
	KeyFile         string `mapstructure:"keyFile" yaml:"keyFile" json:"-"`
}

// Validate checks destination settings without connecting to the broker.
//
// It checks broker syntax, identifiers, QoS, and credential/TLS combinations.
// NewPublisher loads and validates the configured files.
//
// Returns:
//   - error: Coded validation error with no secret values; otherwise nil.
func (c Config) Validate() error {
	u, err := validateBrokerURL(c.Broker)
	if err != nil {
		return err
	}
	if !validText(c.ClientID) || !validText(c.SinkID) {
		return fmt.Errorf("MQTT-CONFIG-IDENTIFIER clientId and sinkId must be nonempty valid MQTT strings")
	}
	if c.QoS < 0 || c.QoS > 2 {
		return fmt.Errorf("MQTT-CONFIG-QOS qos must be 0, 1, or 2")
	}
	if (c.CertificateFile == "") != (c.KeyFile == "") {
		return fmt.Errorf("MQTT-CONFIG-CERTPAIR certificateFile and keyFile must be supplied together")
	}
	if u.Scheme != "tls" && (c.CAFile != "" || c.CertificateFile != "") {
		return fmt.Errorf("MQTT-CONFIG-TLS TLS files require a tls broker URL")
	}
	if c.Username != "" && c.UsernameFile != "" || c.Password != "" && c.PasswordFile != "" {
		return fmt.Errorf("MQTT-CONFIG-CREDENTIALS use either a credential value or its file")
	}
	return nil
}

// ValidateTopicPrefix checks a prefix for the generated publish topics.
//
// Parameters:
//   - prefix: Topic prefix without MQTT wildcards or a trailing slash.
//
// Returns:
//   - error: Coded error for empty, oversized, or invalid UTF-8/NUL-containing prefixes; otherwise nil.
func ValidateTopicPrefix(prefix string) error {
	if !validText(prefix) || strings.ContainsAny(prefix, "+#") || strings.HasSuffix(prefix, "/") || len(prefix) > 65000 {
		return fmt.Errorf("MQTT-CONFIG-TOPIC invalid topicPrefix")
	}
	return nil
}
func validText(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.ValidString(s) && !strings.ContainsRune(s, 0) && len(s) <= 65535
}

func readCredential(value, file string) (string, error) {
	if file == "" {
		return value, nil
	}
	// #nosec G304 -- the credential path is an explicit operator configuration value.
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("MQTT-CONFIG-SECRETFILE cannot read credential file")
	}
	return strings.TrimRight(string(raw), "\r\n"), nil
}

func (c Config) tlsConfig() (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.CAFile != "" {
		raw, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("MQTT-CONFIG-CAREAD cannot read CA file")
		}
		cfg.RootCAs = x509.NewCertPool()
		if !cfg.RootCAs.AppendCertsFromPEM(raw) {
			return nil, fmt.Errorf("MQTT-CONFIG-CAPEM invalid CA certificate")
		}
	}
	if c.CertificateFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertificateFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("MQTT-CONFIG-CLIENTCERT cannot load client certificate and key")
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

func validateBrokerURL(broker string) (*url.URL, error) {
	u, err := url.Parse(broker)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return nil, fmt.Errorf("MQTT-CONFIG-BROKER invalid broker URL; use mqtt://host:port or tls://host:port without credentials")
	}
	if u.Scheme != "mqtt" && u.Scheme != "tls" {
		return nil, fmt.Errorf("MQTT-CONFIG-SCHEME broker scheme must be mqtt or tls")
	}
	return u, nil
}
