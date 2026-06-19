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
	"fmt"
	"net/url"
	"strings"
)

// BuildPostgresDSN creates a PostgreSQL DSN with URL-encoded credentials.
func BuildPostgresDSN(cfg PostgresConfig) string {
	if strings.TrimSpace(cfg.DSN) != "" {
		return NormalizePostgresDSN(cfg.DSN)
	}

	postgresURL := &url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:   cfg.DBName,
	}
	postgresURL.User = url.UserPassword(cfg.User, cfg.Password)
	postgresURL.RawQuery = buildPostgresQuery(cfg).Encode()

	return postgresURL.String()
}

func buildPostgresQuery(cfg PostgresConfig) url.Values {
	query := url.Values{}
	addQueryValue(query, "sslmode", effectivePostgresSSLMode(cfg.SSLMode))
	addQueryValue(query, "sslcert", cfg.SSLCert)
	addQueryValue(query, "sslkey", cfg.SSLKey)
	addQueryValue(query, "sslrootcert", cfg.SSLRootCert)
	addPositiveIntQueryValue(query, "connect_timeout", cfg.ConnectTimeoutSeconds)
	addQueryValue(query, "application_name", cfg.ApplicationName)
	addQueryValue(query, "fallback_application_name", cfg.FallbackApplicationName)
	addQueryValue(query, "search_path", cfg.SearchPath)
	addQueryValue(query, "options", cfg.Options)
	addQueryValue(query, "TimeZone", cfg.TimeZone)

	return query
}

func effectivePostgresSSLMode(sslMode string) string {
	trimmed := strings.TrimSpace(sslMode)
	if trimmed == "" {
		return DefaultConfig.PgSSLMode
	}
	return trimmed
}

func addQueryValue(query url.Values, key string, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	query.Set(key, value)
}

func addPositiveIntQueryValue(query url.Values, key string, value int) {
	if value <= 0 {
		return
	}
	query.Set(key, fmt.Sprintf("%d", value))
}

// NormalizePostgresDSN ensures URL-based PostgreSQL DSNs are encoded correctly.
func NormalizePostgresDSN(dsn string) string {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return trimmed
	}

	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "postgres://") && !strings.HasPrefix(lower, "postgresql://") {
		return trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.User == nil {
		return trimmed
	}

	username := parsed.User.Username()
	password, hasPassword := parsed.User.Password()
	if hasPassword {
		parsed.User = url.UserPassword(username, password)
	} else {
		parsed.User = url.User(username)
	}

	return parsed.String()
}
