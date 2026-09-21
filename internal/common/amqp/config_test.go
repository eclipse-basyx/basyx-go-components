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

package amqp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func validConfig() Config {
	return Config{Broker: "amqp://localhost:5672", Address: "/queues/test", SinkID: "amqp"}
}
func TestConfigValidation(t *testing.T) {
	require.NoError(t, validConfig().Validate())
	for _, broker := range []string{"", "http://localhost", "amqp://user:secret@localhost", "amqp://localhost/path", "amqp://localhost?secret", "amqp://localhost?", "amqp://localhost#fragment", "amqp://localhost:0", "amqp://localhost:65536"} {
		cfg := validConfig()
		cfg.Broker = broker
		require.Error(t, cfg.Validate(), broker)
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.Address = "" }, func(c *Config) { c.SinkID = "" }, func(c *Config) { c.HostName = "\x00" },
		func(c *Config) { c.Username = "user" }, func(c *Config) { c.Password = "secret" },
		func(c *Config) { c.Username = "u"; c.Password = "p"; c.PasswordFile = "file" },
		func(c *Config) { c.CertificateFile = "cert" }, func(c *Config) { c.CAFile = "ca" },
	} {
		cfg := validConfig()
		change(&cfg)
		require.Error(t, cfg.Validate())
	}
}
func TestCredentialsAndTLS(t *testing.T) {
	cfg := validConfig()
	cfg.Broker = "amqps://localhost:5671"
	cfg.HostName = "vhost:tenant"
	opts, err := cfg.connectionOptions()
	require.NoError(t, err)
	require.Equal(t, "localhost", opts.TLSConfig.ServerName)
	require.Equal(t, "vhost:tenant", opts.HostName)
	require.False(t, opts.TLSConfig.InsecureSkipVerify)
	cfg.UsernameFile = filepath.Join(t.TempDir(), "username")
	cfg.PasswordFile = filepath.Join(t.TempDir(), "password")
	require.NoError(t, os.WriteFile(cfg.UsernameFile, []byte("user\n"), 0600))
	require.NoError(t, os.WriteFile(cfg.PasswordFile, []byte("secret\r\n"), 0600))
	opts, err = cfg.connectionOptions()
	require.NoError(t, err)
	require.NotNil(t, opts.SASLType)
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NotContains(t, string(raw), cfg.PasswordFile)
	require.NotContains(t, string(raw), cfg.UsernameFile)
	require.NoError(t, os.WriteFile(cfg.PasswordFile, []byte("\n"), 0600))
	_, err = cfg.connectionOptions()
	require.Error(t, err)
	cfg.PasswordFile = "/nonexistent/amqp-secret"
	_, err = cfg.connectionOptions()
	require.Error(t, err)
	require.NotContains(t, err.Error(), cfg.PasswordFile)
	cfg = validConfig()
	cfg.Broker = "amqps://localhost"
	cfg.CAFile = "/nonexistent/amqp-ca"
	_, err = NewPublisher(t.Context(), cfg)
	require.Error(t, err)
}
