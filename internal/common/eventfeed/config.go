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

package eventfeed

import (
	"fmt"
	"strings"
	"time"
)

// Config holds runtime settings for the Event Feed module.
type Config struct {
	Enabled         bool
	MaxAge          time.Duration
	MaxPageSize     int
	SourceBaseURL   string
	SchemaBaseURL   string
	HardDeleteGrace time.Duration
	PublicAccess    bool
	BearerAuth      bool
	RetentionCron   string // informational; Go uses ticker interval
	CleanupInterval time.Duration
}

// DefaultConfig returns safe defaults aligned with the Java reference tests.
func DefaultConfig() Config {
	return Config{
		Enabled:         false,
		MaxAge:          30 * 24 * time.Hour,
		MaxPageSize:     100,
		SourceBaseURL:   "http://localhost",
		SchemaBaseURL:   "https://admin-shell.io/events/schemas",
		HardDeleteGrace: 10 * 24 * time.Hour,
		PublicAccess:    false,
		BearerAuth:      false,
		CleanupInterval: 24 * time.Hour,
	}
}

// Validate checks configuration consistency.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.MaxPageSize < 1 {
		return fmt.Errorf("EVENTFEED-CFG-MAXPAGESIZE maxPageSize must be positive")
	}
	if c.MaxAge <= 0 {
		return fmt.Errorf("EVENTFEED-CFG-MAXAGE maxAge must be positive")
	}
	if c.HardDeleteGrace < 0 {
		return fmt.Errorf("EVENTFEED-CFG-HARDDELETE hardDeleteGrace must not be negative")
	}
	if strings.TrimSpace(c.SourceBaseURL) == "" {
		return fmt.Errorf("EVENTFEED-CFG-SOURCE sourceBaseUrl must not be blank")
	}
	if strings.TrimSpace(c.SchemaBaseURL) == "" {
		return fmt.Errorf("EVENTFEED-CFG-SCHEMA schemaBaseUrl must not be blank")
	}
	return nil
}

func (c Config) MaxAgePeriod() string {
	days := int(c.MaxAge.Hours() / 24)
	if days < 1 {
		days = 1
	}
	return fmt.Sprintf("P%dD", days)
}

func trimTrailingSlash(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}
