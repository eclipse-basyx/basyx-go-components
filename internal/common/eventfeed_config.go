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
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
)

// NewEventFeedConfig maps service settings to feed storage and HTTP routes.
//
// Event source and schema URLs default to the configured public API base URL.
// Shared URL overrides take precedence over the compatible feed-specific aliases.
//
// Parameters:
//   - cfg: Validated service configuration; nil returns feed defaults.
//
// Returns:
//   - eventfeed.Config: Feed settings with schema routes enabled for any active event transport.
func NewEventFeedConfig(cfg *Config) eventfeed.Config {
	runtime := eventfeed.DefaultConfig()
	if cfg == nil {
		return runtime
	}
	feed := cfg.Eventing.Feed
	runtime.Enabled = feed.Enabled
	runtime.SchemasEnabled = feed.Enabled || cfg.Eventing.TransportsEnabled()
	runtime.MaxAge = eventFeedDuration(feed.MaxAgeDays, 24*time.Hour, runtime.MaxAge)
	runtime.HardDeleteGrace = time.Duration(feed.HardDeleteGraceDays) * 24 * time.Hour
	runtime.CleanupInterval = eventFeedDuration(feed.CleanupIntervalHours, time.Hour, runtime.CleanupInterval)
	runtime.PublishInterval = eventFeedDuration(feed.PublishIntervalMillis, time.Millisecond, runtime.PublishInterval)
	if feed.MaxPageSize != 0 {
		runtime.MaxPageSize = feed.MaxPageSize
	}
	runtime.SourceBaseURL = eventFeedSourceBaseURL(cfg)
	runtime.SchemaBaseURL = normalizeEventURL(cfg.Eventing.SchemaBaseURL)
	if runtime.SchemaBaseURL == "" {
		runtime.SchemaBaseURL = normalizeEventURL(feed.SchemaBaseURL)
	}
	if runtime.SchemaBaseURL == "" {
		runtime.SchemaBaseURL = runtime.SourceBaseURL + eventfeed.SchemaPath
	}
	return runtime
}

func eventFeedDuration(value int, unit, fallback time.Duration) time.Duration {
	if value == 0 {
		return fallback
	}
	return time.Duration(value) * unit
}

func eventFeedSourceBaseURL(cfg *Config) string {
	if source := normalizeEventURL(cfg.Eventing.SourceBaseURL); source != "" {
		return source
	}
	if source := strings.TrimRight(strings.TrimSpace(cfg.Eventing.Feed.SourceBaseURL), "/"); source != "" {
		return source
	}
	if external := NormalizePrimaryExternalBaseURL(cfg.General.ExternalURL); external != "" {
		return external
	}
	port := cfg.Server.Port
	if port == 0 {
		port = DefaultConfig.ServerPort
	}
	return "http://" + net.JoinHostPort("localhost", strconv.Itoa(port)) + NormalizeBasePath(cfg.Server.ContextPath)
}
