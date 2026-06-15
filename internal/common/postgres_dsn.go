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
	postgresURL := &url.URL{
		Scheme:   "postgres",
		Host:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:     cfg.DBName,
		RawQuery: "sslmode=disable",
	}
	postgresURL.User = url.UserPassword(cfg.User, cfg.Password)

	return postgresURL.String()
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
