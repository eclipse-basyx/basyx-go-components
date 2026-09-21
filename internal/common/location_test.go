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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIBaseLocationNormalizesContextPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contextPath string
		want        string
	}{
		{name: "empty", want: "http://example.com"},
		{name: "root", contextPath: "/", want: "http://example.com"},
		{name: "whitespace", contextPath: " \t ", want: "http://example.com"},
		{name: "bare path", contextPath: "api/v3", want: "http://example.com/api/v3"},
		{name: "trimmed path", contextPath: " /api/v3/ \t", want: "http://example.com/api/v3"},
		{name: "outer slashes", contextPath: "///api/v3///", want: "http://example.com/api/v3"},
		{name: "repeated slashes", contextPath: "///", want: "http://example.com/"},
		{name: "interior slashes", contextPath: "/api//v3/", want: "http://example.com/api//v3"},
		{name: "escaped path", contextPath: "/aas%20environment/v3/", want: "http://example.com/aas%20environment/v3"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodPost, "http://example.com/unrelated/request/path", nil)
			require.Equal(t, test.want, APIBaseLocation(request, test.contextPath))
		})
	}
}

func TestAPIBaseLocationResolvesOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		requestURL   string
		host         string
		externalURL  string
		trustProxy   bool
		trustedCIDRs []string
		forwarded    string
		xProto       string
		xHost        string
		want         string
	}{
		{
			name: "direct HTTP with port", requestURL: "http://internal.example/",
			host: "internal.example:8080", want: "http://internal.example:8080/api/v3",
		},
		{
			name: "direct TLS", requestURL: "https://internal.example/",
			host: "internal.example", want: "https://internal.example/api/v3",
		},
		{
			name: "IPv6 host", requestURL: "http://internal.example/",
			host: "[::1]:8080", want: "http://[::1]:8080/api/v3",
		},
		{
			name: "missing host", requestURL: "http://internal.example/", want: "",
		},
		{
			name: "invalid host", requestURL: "http://internal.example/",
			host: "invalid.example/path", want: "",
		},
		{
			name: "external URL wins over proxy and context path", requestURL: "http://internal.example/",
			host: "internal.example", externalURL: " https://public.example/aas%20environment///, https://secondary.example ",
			trustProxy: true, trustedCIDRs: []string{"10.0.0.0/8"},
			forwarded: "proto=https;host=proxy.example", want: "https://public.example/aas%20environment",
		},
		{
			name: "external URL without request host", requestURL: "http://internal.example/",
			externalURL: "https://public.example/", want: "https://public.example",
		},
		{
			name: "invalid external URL falls back", requestURL: "http://internal.example/",
			host: "internal.example", externalURL: "https://public.example/?query=invalid", want: "http://internal.example/api/v3",
		},
		{
			name: "proxy headers disabled", requestURL: "http://internal.example/",
			host: "internal.example", trustedCIDRs: []string{"10.0.0.0/8"},
			forwarded: "proto=https;host=proxy.example", xProto: "https", xHost: "proxy.example",
			want: "http://internal.example/api/v3",
		},
		{
			name: "untrusted peer", requestURL: "http://internal.example/",
			host: "internal.example", trustProxy: true, trustedCIDRs: []string{"192.168.0.0/16"},
			forwarded: "proto=https;host=proxy.example", xProto: "https", xHost: "proxy.example",
			want: "http://internal.example/api/v3",
		},
		{
			name: "trusted Forwarded takes precedence", requestURL: "http://internal.example/",
			host: "internal.example", trustProxy: true, trustedCIDRs: []string{"10.0.0.0/8"},
			forwarded: "proto=https;host=proxy.example:8443", xProto: "http", xHost: "other.example",
			want: "https://proxy.example:8443/api/v3",
		},
		{
			name: "trusted X-Forwarded", requestURL: "http://internal.example/",
			host: "internal.example", trustProxy: true, trustedCIDRs: []string{"10.0.0.0/8"},
			xProto: "https, http", xHost: "proxy.example, other.example", want: "https://proxy.example/api/v3",
		},
		{
			name: "invalid forwarded host falls back", requestURL: "http://internal.example/",
			host: "internal.example", trustProxy: true, trustedCIDRs: []string{"10.0.0.0/8"},
			forwarded: "proto=https;host=invalid.example/path", want: "https://internal.example/api/v3",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodPost, test.requestURL, nil)
			request.Host = test.host
			request.RemoteAddr = "10.1.2.3:1234"
			request.Header.Set("Forwarded", test.forwarded)
			request.Header.Set("X-Forwarded-Proto", test.xProto)
			request.Header.Set("X-Forwarded-Host", test.xHost)
			cfg := &Config{}
			cfg.ABAC.Enabled = false
			cfg.General.ExternalURL = test.externalURL
			cfg.General.TrustProxyHeaders = test.trustProxy
			cfg.General.TrustedProxyCIDRs = test.trustedCIDRs
			request = request.WithContext(ContextWithConfig(request.Context(), cfg))

			require.Equal(t, test.want, APIBaseLocation(request, " /api/v3/ "))
		})
	}
}

func TestContextualizeAPIResourceLocationPreservesEscapedSegments(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(
		http.MethodPost,
		"http://example.com/api/submodels/c20/submodel-elements/Ops%2FAdd/invoke-async",
		nil,
	)

	location := ContextualizeAPIResourceLocation(
		request,
		"/submodels/c20/submodel-elements/Ops%2FAdd/operation-status/handle-1",
		"/submodels/",
	)

	require.Equal(
		t,
		"http://example.com/api/submodels/c20/submodel-elements/Ops%2FAdd/operation-status/handle-1",
		location,
	)
}

func TestContextualizeAPIResourceLocationUsesEscapedExternalBaseURL(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(
		http.MethodPost,
		"http://internal.example/internal/submodels/c20/submodel-elements/Ops%2FAdd/invoke-async",
		nil,
	)
	cfg := &Config{}
	cfg.General.ExternalURL = "https://public.example/aas%20environment"
	request = request.WithContext(ContextWithConfig(request.Context(), cfg))

	location := ContextualizeAPIResourceLocation(
		request,
		"/submodels/c20/submodel-elements/Ops%2FAdd/operation-status/handle-1",
		"/submodels/",
	)

	require.Equal(
		t,
		"https://public.example/aas%20environment/submodels/c20/submodel-elements/Ops%2FAdd/operation-status/handle-1",
		location,
	)
}
