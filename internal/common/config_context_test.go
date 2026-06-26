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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContextWithConfig_RoundTrip(t *testing.T) {
	cfg := &Config{Server: ServerConfig{Port: 6004}}

	ctx := ContextWithConfig(context.Background(), cfg)
	resolved, ok := ConfigFromContext(ctx)
	if !ok {
		t.Fatalf("expected config in context")
	}
	if resolved != cfg {
		t.Fatalf("expected same config pointer from context")
	}
}

func TestExternalBaseURLFromRequestUsesTrustedForwardedHeaders(t *testing.T) {
	cfg := &Config{Server: ServerConfig{ContextPath: "/api/v3"}}
	cfg.General.TrustProxyHeaders = true
	cfg.General.TrustedProxyCIDRs = []string{"10.10.10.0/24"}

	req, err := http.NewRequest(http.MethodGet, "http://service.local/api/v3/shells", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "service.local"
	req.RemoteAddr = "10.10.10.10:12345"
	req.Header.Set("Forwarded", "proto=https;host=public.example")
	req = req.WithContext(ContextWithConfig(context.Background(), cfg))

	if got := ExternalBaseURLFromRequest(req); got != "https://public.example/api/v3" {
		t.Fatalf("expected trusted forwarded external base URL, got %q", got)
	}
}

func TestExternalBaseURLFromRequestRejectsUntrustedProxySource(t *testing.T) {
	cfg := &Config{Server: ServerConfig{ContextPath: "/api/v3"}}
	cfg.General.TrustProxyHeaders = true
	cfg.General.TrustedProxyCIDRs = []string{"10.10.10.0/24"}

	req, err := http.NewRequest(http.MethodGet, "http://service.local/api/v3/shells", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "service.local"
	req.RemoteAddr = "192.0.2.10:12345"
	req.Header.Set("Forwarded", "proto=https;host=evil.example")
	req.Header.Set("X-Forwarded-Host", "evil.example")
	req = req.WithContext(ContextWithConfig(context.Background(), cfg))

	if got := ExternalBaseURLFromRequest(req); got != "" {
		t.Fatalf("expected no external base URL from untrusted proxy source, got %q", got)
	}
}

func TestExternalBaseURLFromRequestUsesAllowedDirectHost(t *testing.T) {
	cfg := &Config{Server: ServerConfig{ContextPath: "/api/v3"}}
	cfg.General.TrustedDynamicHosts = []string{"service.local"}

	req, err := http.NewRequest(http.MethodGet, "http://service.local/api/v3/shells", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "service.local"
	req = req.WithContext(ContextWithConfig(context.Background(), cfg))

	if got := ExternalBaseURLFromRequest(req); got != "http://service.local/api/v3" {
		t.Fatalf("expected allowed direct host external base URL, got %q", got)
	}
}

func TestExternalBaseURLFromRequestHonorsAllowedDirectHostPort(t *testing.T) {
	cfg := &Config{Server: ServerConfig{ContextPath: "/api/v3"}}
	cfg.General.TrustedDynamicHosts = []string{"service.local:8443"}

	req, err := http.NewRequest(http.MethodGet, "http://service.local:8443/api/v3/shells", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "service.local:8443"
	req = req.WithContext(ContextWithConfig(context.Background(), cfg))

	if got := ExternalBaseURLFromRequest(req); got != "http://service.local:8443/api/v3" {
		t.Fatalf("expected allowed direct host with port external base URL, got %q", got)
	}

	req.Host = "service.local:9443"
	if got := ExternalBaseURLFromRequest(req); got != "" {
		t.Fatalf("expected direct host with unallowed port to be rejected, got %q", got)
	}
}

func TestExternalBaseURLFromRequestRejectsUnallowedDirectHost(t *testing.T) {
	cfg := &Config{Server: ServerConfig{ContextPath: "/api/v3"}}

	req, err := http.NewRequest(http.MethodGet, "http://service.local/api/v3/shells", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "evil.example"
	req = req.WithContext(ContextWithConfig(context.Background(), cfg))

	if got := ExternalBaseURLFromRequest(req); got != "" {
		t.Fatalf("expected no external base URL from unallowed direct host, got %q", got)
	}
}

func TestConfigMiddlewareStoresRequestExternalBaseURL(t *testing.T) {
	cfg := &Config{Server: ServerConfig{ContextPath: "/api/v3"}}
	cfg.General.TrustedDynamicHosts = []string{"service.local"}

	handler := ConfigMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if got := RequestExternalBaseURLFromContext(r.Context()); got != "http://service.local/api/v3" {
			t.Fatalf("expected request external base URL in context, got %q", got)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "http://service.local/api/v3/shells", nil)
	req.Host = "service.local"
	handler.ServeHTTP(httptest.NewRecorder(), req)
}
