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
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
)

// Module wires together the Event Feed service, its HTTP routes, and its
// background retention loop for a single process.
type Module struct {
	Service *Service
	cfg     Config
	stop    chan struct{}
	onStop  func()
}

// NewModule builds a Module from cfg, validating it first. A disabled config
// returns a no-op Module rather than an error.
func NewModule(db *sql.DB, cfg Config) (*Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return &Module{cfg: cfg}, nil
	}
	repo := NewRepository(db, cfg.MaxAge)
	svc := NewService(repo, cfg)
	mod := &Module{Service: svc, cfg: cfg, stop: make(chan struct{})}
	return mod, nil
}

// Enabled reports whether the module is configured and ready to serve.
func (m *Module) Enabled() bool {
	return m != nil && m.cfg.Enabled && m.Service != nil
}

// RegisterRoutes mounts the Event Feed HTTP endpoints on r if the module is enabled.
func (m *Module) RegisterRoutes(r chi.Router) {
	if m == nil || !m.Enabled() {
		return
	}
	RegisterRoutes(r, m.Service)
}

// StartRetentionLoop runs the retention job immediately, then on cfg.CleanupInterval until ctx is done or Stop is called.
func (m *Module) StartRetentionLoop(ctx context.Context) {
	if m == nil || !m.Enabled() {
		return
	}
	interval := m.cfg.CleanupInterval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	go func() {
		run := func() {
			if _, err := m.Service.RunRetention(ctx); err != nil {
				slog.WarnContext(ctx, "event feed retention failed",
					"error.code", "EVENTFEED-RETENTION-RUN", "error", err)
			}
		}
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stop:
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

// SetOnStop registers a callback invoked when the module stops.
func (m *Module) SetOnStop(fn func()) {
	if m == nil {
		return
	}
	m.onStop = fn
}

// Stop invokes the registered onStop callback, if any, and terminates the retention loop.
func (m *Module) Stop() {
	if m == nil {
		return
	}
	if m.onStop != nil {
		m.onStop()
		m.onStop = nil
	}
	if m.stop == nil {
		return
	}
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
}
