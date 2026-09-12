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

// Package brokersecurity loads shared event transport credentials and TLS material.
package brokersecurity

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
)

// ReadCredential loads an operator-provided credential, trimming only trailing line endings.
func ReadCredential(component, value, file string) (string, error) {
	if file == "" {
		return value, nil
	}
	// #nosec G304 -- the credential path is an explicit operator configuration value.
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("%s-CONFIG-SECRETFILE cannot read credential file", component)
	}
	return strings.TrimRight(string(raw), "\r\n"), nil
}

// TLSConfig builds verified TLS 1.2+ settings with optional CA and client certificates.
func TLSConfig(component, caFile, certificateFile, keyFile string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		// #nosec G304 -- the CA path is an explicit operator configuration value.
		raw, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("%s-CONFIG-CAREAD cannot read CA file", component)
		}
		cfg.RootCAs = x509.NewCertPool()
		if !cfg.RootCAs.AppendCertsFromPEM(raw) {
			return nil, fmt.Errorf("%s-CONFIG-CAPEM invalid CA certificate", component)
		}
	}
	if certificateFile != "" {
		cert, err := tls.LoadX509KeyPair(certificateFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("%s-CONFIG-CLIENTCERT cannot load client certificate and key", component)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}
