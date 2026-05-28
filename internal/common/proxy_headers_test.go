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
// Author: Christian Koort ( Fraunhofer IESE )

package common

import (
	"context"
	"net/http"
	"testing"
)

// TestRequestHostAndSchemeIgnoreForwardedHeadersByDefault verifies that forwarded headers are ignored by default.
func TestRequestHostAndSchemeIgnoreForwardedHeadersByDefault(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://service.local/api", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "service.local"
	req.RemoteAddr = "10.10.10.10:12345"
	req.Header.Set("Forwarded", "proto=https;host=public.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "public.example")

	if got := RequestScheme(req); got != "http" {
		t.Fatalf("expected fallback scheme http, got %q", got)
	}
	if got := RequestHost(req); got != "service.local" {
		t.Fatalf("expected fallback host service.local, got %q", got)
	}
}

// TestRequestHostAndSchemeUseForwardedHeadersWhenProxyTrusted verifies that forwarded headers are used when the proxy is trusted.
func TestRequestHostAndSchemeUseForwardedHeadersWhenProxyTrusted(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://service.local/api", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "service.local"
	req.RemoteAddr = "10.10.10.10:12345"
	req.Header.Set("Forwarded", "proto=https;host=public.example")

	cfg := &Config{}
	cfg.General.TrustProxyHeaders = true
	cfg.General.TrustedProxyCIDRs = []string{"10.10.10.0/24"}
	req = req.WithContext(ContextWithConfig(context.Background(), cfg))

	if got := RequestScheme(req); got != "https" {
		t.Fatalf("expected forwarded scheme https, got %q", got)
	}
	if got := RequestHost(req); got != "public.example" {
		t.Fatalf("expected forwarded host public.example, got %q", got)
	}
}

// TestRequestHostAndSchemeIgnoreForwardedHeadersWhenRemoteUntrusted verifies that forwarded headers are ignored when the remote address is not trusted.
func TestRequestHostAndSchemeIgnoreForwardedHeadersWhenRemoteUntrusted(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://service.local/api", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "service.local"
	req.RemoteAddr = "10.10.10.10:12345"
	req.Header.Set("Forwarded", "proto=https;host=public.example")

	cfg := &Config{}
	cfg.General.TrustProxyHeaders = true
	cfg.General.TrustedProxyCIDRs = []string{"192.168.0.0/16"}
	req = req.WithContext(ContextWithConfig(context.Background(), cfg))

	if got := RequestScheme(req); got != "http" {
		t.Fatalf("expected fallback scheme http, got %q", got)
	}
	if got := RequestHost(req); got != "service.local" {
		t.Fatalf("expected fallback host service.local, got %q", got)
	}
}

// TestRequestHostRejectsInvalidForwardedHost verifies that invalid forwarded host values are rejected.
func TestRequestHostRejectsInvalidForwardedHost(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://service.local/api", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "service.local"
	req.RemoteAddr = "10.10.10.10:12345"
	req.Header.Set("Forwarded", "proto=https;host=evil.example/path")

	cfg := &Config{}
	cfg.General.TrustProxyHeaders = true
	cfg.General.TrustedProxyCIDRs = []string{"10.10.10.0/24"}
	req = req.WithContext(ContextWithConfig(context.Background(), cfg))

	if got := RequestHost(req); got != "service.local" {
		t.Fatalf("expected fallback host service.local, got %q", got)
	}
}
