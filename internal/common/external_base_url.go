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
	"net/url"
	"strings"
)

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
