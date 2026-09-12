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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testConfig() Config {
	return Config{Brokers: []string{"localhost:9092"}, Topic: "basyx.events", SinkID: "kafka", ClientID: "test"}
}

func TestConfigValidation(t *testing.T) {
	valid := testConfig()
	require.NoError(t, valid.Validate())
	cases := []func(*Config){
		func(c *Config) { c.Brokers = nil }, func(c *Config) { c.Brokers = []string{"user:secret@host:9092"} },
		func(c *Config) { c.Brokers = []string{"localhost:0"} }, func(c *Config) { c.Topic = "a/b" },
		func(c *Config) { c.Topic = ".." }, func(c *Config) { c.SinkID = "" }, func(c *Config) { c.ClientID = "" },
		func(c *Config) { c.CAFile = "ca.pem" }, func(c *Config) { c.TLSEnabled = true; c.CertificateFile = "cert.pem" },
		func(c *Config) { c.SASLMechanism = "OAUTHBEARER" }, func(c *Config) { c.SASLMechanism = "PLAIN" },
		func(c *Config) { c.Username = "secret" }, func(c *Config) {
			c.Username = "u"
			c.Password = "secret"
			c.UsernameFile = "file"
			c.SASLMechanism = "PLAIN"
		},
	}
	for _, mutate := range cases {
		c := valid
		mutate(&c)
		err := c.Validate()
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
	for _, mechanism := range []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"} {
		c := valid
		c.SASLMechanism = mechanism
		c.Username = "u"
		c.Password = "secret"
		require.NoError(t, c.Validate())
		_, err := c.clientOptions()
		require.NoError(t, err)
		raw, err := json.Marshal(c)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "secret")
	}
}

func TestCredentialFilesAndStartupFailure(t *testing.T) {
	c := testConfig()
	c.SASLMechanism = "PLAIN"
	c.UsernameFile = filepath.Join(t.TempDir(), "username")
	c.PasswordFile = c.UsernameFile + ".password"
	require.NoError(t, os.WriteFile(c.UsernameFile, []byte("user\n"), 0600))
	require.NoError(t, os.WriteFile(c.PasswordFile, []byte(" secret \r\n"), 0600))
	_, err := c.clientOptions()
	require.NoError(t, err)
	c.PasswordFile += "missing"
	_, err = NewPublisher(t.Context(), c)
	require.ErrorContains(t, err, "SECRETFILE")
	c = testConfig()
	c.TLSEnabled = true
	c.CAFile = c.PasswordFile + "missing"
	_, err = NewPublisher(t.Context(), c)
	require.ErrorContains(t, err, "CAREAD")
}

func TestUnavailableBrokerCancellationAndStop(t *testing.T) {
	c := testConfig()
	c.Brokers = []string{"127.0.0.1:1"}
	p, err := NewPublisher(t.Context(), c)
	require.NoError(t, err)
	routing, err := Routing(c.Topic, "entity")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	require.ErrorContains(t, p.Publish(ctx, routing, []byte(`{}`)), "KAFKA-PUBLISH-DELIVERY")
	require.Less(t, time.Since(started), 2*time.Second)
	require.False(t, p.Connected())
	shutdown, stop := context.WithTimeout(t.Context(), time.Second)
	defer stop()
	require.NoError(t, p.Stop(shutdown))
	require.NoError(t, p.Stop(shutdown))
	require.Error(t, p.Publish(t.Context(), routing, []byte(`{}`)))
}
