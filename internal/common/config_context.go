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

// nolint:all
package common

import (
	"context"
	"net/http"
)

// configKey is an unexported type used as the context key.
type configKey struct{}

// ConfigMiddleware injects the process-wide *Config into each request context.
// This lets downstream handlers fetch configuration without adding parameters.
func ConfigMiddleware(cfg *Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), configKey{}, cfg)
			requestWithConfig := r.WithContext(ctx)
			ctx = ContextWithRequestExternalBaseURL(ctx, ExternalBaseURLFromRequest(requestWithConfig))
			next.ServeHTTP(w, requestWithConfig.WithContext(ctx))
		})
	}
}

// ConfigFromContext retrieves the *Config stored in context. The boolean
// indicates whether a config was present.
func ConfigFromContext(ctx context.Context) (*Config, bool) {
	cfg, ok := ctx.Value(configKey{}).(*Config)
	return cfg, ok
}

// ContextWithConfig returns a context containing the process-wide *Config.
func ContextWithConfig(ctx context.Context, cfg *Config) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, configKey{}, cfg)
}

// BulkBatchLimitFromContext returns the configured bulk batch limit.
//
// The function reads the request config from the context and falls back to the
// default limit when no positive value is configured.
//
// Parameters:
//   - ctx: Request context that may contain a process config.
//
// Returns:
//   - int: Positive row limit for bulk SQL statements.
func BulkBatchLimitFromContext(ctx context.Context) int {
	cfg, ok := ConfigFromContext(ctx)
	if !ok || cfg == nil || cfg.General.BulkBatchLimit <= 0 {
		return DefaultConfig.GeneralBulkBatchLimit
	}
	return cfg.General.BulkBatchLimit
}
