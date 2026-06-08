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
	"net"
	"net/http"
	"net/url"
	"strings"
)

// RequestScheme determines the effective scheme ("http" or "https") for an incoming HTTP request.
//
// This function first checks if the request originates from a trusted proxy (as defined by configuration and source IP).
// If so, it will honor the "Forwarded" or "X-Forwarded-Proto" headers to determine the scheme, allowing correct URL generation
// when the service is behind a reverse proxy or load balancer. If the proxy is not trusted, or no valid header is present,
// it falls back to the direct connection: "https" if TLS is enabled, otherwise "http".
//
// Parameters:
// - r: The HTTP request to inspect.
//
// Returns:
// - string: The scheme ("http" or "https") determined from trusted proxy headers or the direct connection.
func RequestScheme(r *http.Request) string {
	if shouldTrustForwardedHeaders(r) {
		if forwardedProto := normalizedForwardedProto(r); forwardedProto != "" {
			return forwardedProto
		}
	}

	if r.TLS != nil {
		return "https"
	}

	return "http"
}

// RequestHost determines the effective host (domain and optional port) for an incoming HTTP request.
//
// This function checks if the request comes from a trusted proxy (per configuration and source IP).
// If so, it will honor the "Forwarded" or "X-Forwarded-Host" headers to determine the host, which is essential
// for generating correct absolute URLs when the service is behind a reverse proxy or load balancer. If the proxy is not trusted,
// or no valid header is present, it falls back to the direct request's Host field.
//
// Parameters:
// - r: The HTTP request to inspect.
//
// Returns:
// - string: The host, determined from trusted proxy headers or the direct connection.
func RequestHost(r *http.Request) string {
	if shouldTrustForwardedHeaders(r) {
		if forwardedHost := normalizedForwardedHost(r); forwardedHost != "" {
			return forwardedHost
		}
	}

	return normalizeHostValue(r.Host)
}

// RequestSourceIP returns the client source IP accepted for audit metadata.
//
// Forwarded and X-Forwarded-For headers are honored only when the request comes
// from a configured trusted proxy. Otherwise the direct remote address is used.
func RequestSourceIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if shouldTrustForwardedHeaders(r) {
		if forwardedFor := normalizedForwardedFor(r); forwardedFor != "" {
			return forwardedFor
		}
	}
	ip := parseRemoteAddrIP(r.RemoteAddr)
	if ip == nil {
		return ""
	}
	return ip.String()
}

func shouldTrustForwardedHeaders(r *http.Request) bool {
	if r == nil {
		return false
	}

	cfg, ok := ConfigFromContext(r.Context())
	if !ok || cfg == nil || !cfg.General.TrustProxyHeaders {
		return false
	}

	return remoteAddrInTrustedCIDRs(r.RemoteAddr, cfg.General.TrustedProxyCIDRs)
}

func remoteAddrInTrustedCIDRs(remoteAddr string, cidrs []string) bool {
	if len(cidrs) == 0 {
		return false
	}

	ip := parseRemoteAddrIP(remoteAddr)
	if ip == nil {
		return false
	}

	for _, rawCIDR := range cidrs {
		_, ipNet, err := net.ParseCIDR(strings.TrimSpace(rawCIDR))
		if err != nil {
			continue
		}
		if ipNet.Contains(ip) {
			return true
		}
	}

	return false
}

func parseRemoteAddrIP(remoteAddr string) net.IP {
	trimmed := strings.TrimSpace(remoteAddr)
	if trimmed == "" {
		return nil
	}

	host, _, err := net.SplitHostPort(trimmed)
	if err == nil {
		return net.ParseIP(strings.Trim(host, "[]"))
	}

	return net.ParseIP(strings.Trim(trimmed, "[]"))
}

func normalizedForwardedFor(r *http.Request) string {
	if forwardedFor := parseForwardedHeaderValue(r.Header.Get("Forwarded"), "for"); forwardedFor != "" {
		return normalizeForwardedIP(forwardedFor)
	}
	if xForwardedFor := firstForwardedValue(r.Header.Get("X-Forwarded-For")); xForwardedFor != "" {
		return normalizeForwardedIP(xForwardedFor)
	}
	return ""
}

func normalizeForwardedIP(rawIP string) string {
	ip := parseRemoteAddrIP(rawIP)
	if ip == nil {
		return ""
	}
	return ip.String()
}

func normalizedForwardedProto(r *http.Request) string {
	if forwardedProto := parseForwardedHeaderValue(r.Header.Get("Forwarded"), "proto"); forwardedProto != "" {
		return normalizeProtoValue(forwardedProto)
	}

	if xForwardedProto := firstForwardedValue(r.Header.Get("X-Forwarded-Proto")); xForwardedProto != "" {
		return normalizeProtoValue(xForwardedProto)
	}

	return ""
}

func normalizedForwardedHost(r *http.Request) string {
	if forwardedHost := parseForwardedHeaderValue(r.Header.Get("Forwarded"), "host"); forwardedHost != "" {
		return normalizeHostValue(forwardedHost)
	}

	if xForwardedHost := firstForwardedValue(r.Header.Get("X-Forwarded-Host")); xForwardedHost != "" {
		return normalizeHostValue(xForwardedHost)
	}

	return ""
}

func parseForwardedHeaderValue(forwarded string, key string) string {
	firstEntry, _, _ := strings.Cut(forwarded, ",")

	for _, token := range strings.Split(firstEntry, ";") {
		pair := strings.SplitN(strings.TrimSpace(token), "=", 2)
		if len(pair) != 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(pair[0]), key) {
			return strings.Trim(strings.TrimSpace(pair[1]), "\"")
		}
	}

	return ""
}

func firstForwardedValue(value string) string {
	if value == "" {
		return ""
	}

	first, _, _ := strings.Cut(value, ",")

	return strings.TrimSpace(first)
}

func normalizeProtoValue(rawProto string) string {
	proto := strings.ToLower(strings.TrimSpace(rawProto))
	if proto != "http" && proto != "https" {
		return ""
	}

	return proto
}

func normalizeHostValue(rawHost string) string {
	host := strings.TrimSpace(rawHost)
	if host == "" {
		return ""
	}

	parsed, err := url.Parse("http://" + host)
	if err != nil {
		return ""
	}
	if parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}

	return parsed.Host
}
