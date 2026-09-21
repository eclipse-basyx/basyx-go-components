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

// Package amqp implements the CloudEvents AMQP 1.0 structured JSON binding.
package amqp

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	wire "github.com/Azure/go-amqp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/brokersecurity"
)

// Config contains one AMQP destination and its connection settings.
type Config struct {
	Broker          string `mapstructure:"broker" yaml:"broker" json:"broker"`
	Address         string `mapstructure:"address" yaml:"address" json:"address"`
	SinkID          string `mapstructure:"sinkId" yaml:"sinkId" json:"sinkId"`
	HostName        string `mapstructure:"hostName" yaml:"hostName" json:"hostName"`
	Username        string `mapstructure:"username" yaml:"username" json:"-"`
	Password        string `mapstructure:"password" yaml:"password" json:"-"`
	UsernameFile    string `mapstructure:"usernameFile" yaml:"usernameFile" json:"-"`
	PasswordFile    string `mapstructure:"passwordFile" yaml:"passwordFile" json:"-"`
	CAFile          string `mapstructure:"caFile" yaml:"caFile" json:"caFile"`
	CertificateFile string `mapstructure:"certificateFile" yaml:"certificateFile" json:"certificateFile"`
	KeyFile         string `mapstructure:"keyFile" yaml:"keyFile" json:"-"`
}

// Validate checks local settings without contacting the broker or reading files.
func (c Config) Validate() error {
	u, err := brokerURL(c.Broker)
	if err != nil {
		return err
	}
	if !validText(c.Address) || !validText(c.SinkID) || (c.HostName != "" && !validText(c.HostName)) {
		return fmt.Errorf("AMQP-CONFIG-IDENTIFIER invalid address, sinkId or hostName")
	}
	if (c.CertificateFile == "") != (c.KeyFile == "") {
		return fmt.Errorf("AMQP-CONFIG-CERTPAIR certificateFile and keyFile must be supplied together")
	}
	if u.Scheme != "amqps" && (c.CAFile != "" || c.CertificateFile != "") {
		return fmt.Errorf("AMQP-CONFIG-TLS TLS files require amqps")
	}
	if c.Username != "" && c.UsernameFile != "" || c.Password != "" && c.PasswordFile != "" {
		return fmt.Errorf("AMQP-CONFIG-CREDENTIALS use either a credential value or its file")
	}
	if (c.Username != "" || c.UsernameFile != "") != (c.Password != "" || c.PasswordFile != "") {
		return fmt.Errorf("AMQP-CONFIG-CREDENTIALS username and password are required together")
	}
	return nil
}
func brokerURL(broker string) (*url.URL, error) {
	u, err := url.Parse(broker)
	if err != nil || u.Hostname() == "" || (u.Scheme != "amqp" && u.Scheme != "amqps") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, fmt.Errorf("AMQP-CONFIG-BROKER expected amqp(s)://host[:port] without credentials, path, query or fragment")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("AMQP-CONFIG-PORT invalid broker port")
		}
	}
	return u, nil
}
func validText(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value) && !strings.ContainsRune(value, 0) && len(value) <= 32767
}
func (c Config) connectionOptions() (*wire.ConnOptions, error) {
	opts := &wire.ConnOptions{HostName: c.HostName, WriteTimeout: 5 * time.Second}
	if err := c.configureCredentials(opts); err != nil {
		return nil, err
	}
	u, err := brokerURL(c.Broker)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "amqps" {
		opts.TLSConfig, err = brokersecurity.TLSConfig("AMQP", c.CAFile, c.CertificateFile, c.KeyFile)
		if err != nil {
			return nil, err
		}
		opts.TLSConfig.ServerName = u.Hostname()
	}
	return opts, nil
}
func (c Config) configureCredentials(opts *wire.ConnOptions) error {
	if c.Username == "" && c.UsernameFile == "" {
		return nil
	}
	username, err := brokersecurity.ReadCredential("AMQP", c.Username, c.UsernameFile)
	if err != nil {
		return err
	}
	password, err := brokersecurity.ReadCredential("AMQP", c.Password, c.PasswordFile)
	if err != nil {
		return err
	}
	if username == "" || password == "" {
		return fmt.Errorf("AMQP-CONFIG-CREDENTIALS credentials must not be empty")
	}
	opts.SASLType = wire.SASLTypePlain(username, password)
	return nil
}
