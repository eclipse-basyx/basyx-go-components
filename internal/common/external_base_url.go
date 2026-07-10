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
	"net"
	"net/http"
	"net/url"
	"strings"
)

type requestExternalBaseURLKey struct{}

// ExternalBaseURLFromContext returns the normalized external base URL from the request context configuration.
//
// Parameters:
//   - ctx: Context containing the configuration.
//
// Returns:
//   - The normalized external base URL string, or an empty string if not configured or invalid.
func ExternalBaseURLFromContext(ctx context.Context) string {
	cfg, ok := ConfigFromContext(ctx)
	if !ok || cfg == nil {
		return ""
	}

	return NormalizePrimaryExternalBaseURL(cfg.General.ExternalURL)
}

// ContextWithRequestExternalBaseURL returns a context containing a normalized request-derived external base URL.
func ContextWithRequestExternalBaseURL(ctx context.Context, externalBaseURL string) context.Context {
	if ctx == nil {
		ctx = context.TODO()
	}
	return context.WithValue(ctx, requestExternalBaseURLKey{}, NormalizePrimaryExternalBaseURL(externalBaseURL))
}

// RequestExternalBaseURLFromContext returns the normalized request-derived external base URL from context.
func RequestExternalBaseURLFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	externalBaseURL, ok := ctx.Value(requestExternalBaseURLKey{}).(string)
	if !ok {
		return ""
	}
	return NormalizePrimaryExternalBaseURL(externalBaseURL)
}

// ExternalBaseURLFromRequest returns the normalized public base URL derived from a request.
func ExternalBaseURLFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}

	cfg, hasConfig := ConfigFromContext(r.Context())
	if !hasConfig || cfg == nil {
		return ""
	}

	host := requestExternalHost(r, cfg)
	if host == "" {
		return ""
	}

	basePath := NormalizeBasePath(cfg.Server.ContextPath)
	if basePath == "/" {
		basePath = ""
	}

	return NormalizePrimaryExternalBaseURL(RequestScheme(r) + "://" + host + basePath)
}

func requestExternalHost(r *http.Request, cfg *Config) string {
	if cfg.General.TrustProxyHeaders && remoteAddrInTrustedCIDRs(r.RemoteAddr, cfg.General.TrustedProxyCIDRs) {
		host := RequestHost(r)
		if len(cfg.General.TrustedDynamicHosts) == 0 || hostAllowed(host, cfg.General.TrustedDynamicHosts) {
			return host
		}
		return ""
	}

	host := normalizeHostValue(r.Host)
	if !hostAllowed(host, cfg.General.TrustedDynamicHosts) {
		return ""
	}
	return host
}

func hostAllowed(host string, allowedHosts []string) bool {
	hostOnly, hostPort, hostHasPort := canonicalHostForAllowlist(host)
	if hostOnly == "" {
		return false
	}

	for _, allowedHost := range allowedHosts {
		allowedOnly, allowedHostPort, allowedHasPort := canonicalHostForAllowlist(allowedHost)
		if allowedOnly == "" {
			continue
		}
		if hostHasPort {
			if allowedHasPort && hostPort == allowedHostPort {
				return true
			}
			continue
		}
		if !allowedHasPort && hostOnly == allowedOnly {
			return true
		}
	}

	return false
}

func canonicalHostForAllowlist(host string) (string, string, bool) {
	normalizedHost := strings.ToLower(strings.TrimSpace(host))
	if normalizedHost == "" {
		return "", "", false
	}

	if parsedHost, parsedPort, err := net.SplitHostPort(normalizedHost); err == nil {
		hostOnly := strings.Trim(parsedHost, "[]")
		return hostOnly, net.JoinHostPort(hostOnly, parsedPort), true
	}

	hostOnly := strings.Trim(normalizedHost, "[]")
	return hostOnly, hostOnly, false
}

// NormalizePrimaryExternalBaseURL parses and normalizes the first external base URL from a config string.
//
// Parameters:
//   - rawExternalURL: Raw external URL string (may be comma-separated).
//
// Returns:
//   - A normalized, validated external base URL string, or an empty string if invalid.
func NormalizePrimaryExternalBaseURL(rawExternalURL string) string {
	trimmed := strings.TrimSpace(rawExternalURL)
	if trimmed == "" {
		return ""
	}

	firstEntry, _, _ := strings.Cut(trimmed, ",")
	entry := strings.TrimSpace(firstEntry)
	if entry == "" {
		return ""
	}

	parsed, err := url.Parse(entry)
	if err != nil {
		return ""
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return ""
	}
	if strings.TrimSpace(parsed.Host) == "" || parsed.User != nil {
		return ""
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}

	parsed.Scheme = scheme
	parsed.Host = strings.TrimSpace(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return strings.TrimRight(parsed.String(), "/")
}
