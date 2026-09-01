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

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/go-chi/chi/v5"
)

type Module struct {
	Service *Service
	cfg     Config
	stop    chan struct{}
}

func NewModule(db *sql.DB, cfg Config) (*Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return &Module{cfg: cfg}, nil
	}
	repo := NewRepository(db, cfg.MaxAge)
	svc := NewService(repo, cfg)
	return &Module{Service: svc, cfg: cfg, stop: make(chan struct{})}, nil
}

func (m *Module) Enabled() bool {
	return m != nil && m.cfg.Enabled && m.Service != nil
}

func (m *Module) RegisterRoutes(r chi.Router) {
	if m == nil || !m.Enabled() {
		return
	}
	RegisterRoutes(r, m.Service)
}

func (m *Module) StartRetentionLoop(ctx context.Context) {
	if m == nil || !m.Enabled() {
		return
	}
	interval := m.cfg.CleanupInterval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stop:
				return
			case <-ticker.C:
				if _, err := m.Service.RunRetention(ctx); err != nil {
					slog.WarnContext(ctx, "event feed retention failed",
						"error.code", "EVENTFEED-RETENTION-RUN", "error", err)
				}
			}
		}
	}()
}

func (m *Module) Stop() {
	if m == nil || m.stop == nil {
		return
	}
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
}

func (m *Module) PublishAASCreated(ctx context.Context, aasID, globalAssetID string, submodelIDs []string) {
	if !m.Enabled() {
		return
	}
	ev, err := m.Service.Builder().AASCreated(aasID, globalAssetID, submodelIDs)
	m.Service.PublishBestEffort(ctx, ev, err)
	if globalAssetID != "" {
		aev, aerr := m.Service.Builder().AssetCreated(globalAssetID, aasID, submodelIDs)
		m.Service.PublishBestEffort(ctx, aev, aerr)
	}
}

func (m *Module) PublishAASUpdated(ctx context.Context, aasID, globalAssetID string, submodelIDs []string) {
	if !m.Enabled() {
		return
	}
	ev, err := m.Service.Builder().AASUpdated(aasID, globalAssetID, submodelIDs)
	m.Service.PublishBestEffort(ctx, ev, err)
	if globalAssetID != "" {
		aev, aerr := m.Service.Builder().AssetUpdated(globalAssetID, aasID, submodelIDs)
		m.Service.PublishBestEffort(ctx, aev, aerr)
	}
}

func (m *Module) PublishAASDeleted(ctx context.Context, aasID, globalAssetID string, submodelIDs []string) {
	if !m.Enabled() {
		return
	}
	ev, err := m.Service.Builder().AASDeleted(aasID, globalAssetID, submodelIDs)
	m.Service.PublishBestEffort(ctx, ev, err)
	if globalAssetID != "" {
		aev, aerr := m.Service.Builder().AssetDeleted(globalAssetID, aasID, submodelIDs)
		m.Service.PublishBestEffort(ctx, aev, aerr)
	}
}

func (m *Module) PublishSubmodelCreated(ctx context.Context, submodelID, semanticID string, globalAssetIDs []string) {
	if !m.Enabled() {
		return
	}
	ev, err := m.Service.Builder().SubmodelCreated(submodelID, semanticID, globalAssetIDs)
	m.Service.PublishBestEffort(ctx, ev, err)
}

func (m *Module) PublishSubmodelUpdated(ctx context.Context, submodelID, semanticID string, globalAssetIDs []string) {
	if !m.Enabled() {
		return
	}
	ev, err := m.Service.Builder().SubmodelUpdated(submodelID, semanticID, globalAssetIDs)
	m.Service.PublishBestEffort(ctx, ev, err)
}

func (m *Module) PublishSubmodelDeleted(ctx context.Context, submodelID, semanticID string, globalAssetIDs []string) {
	if !m.Enabled() {
		return
	}
	ev, err := m.Service.Builder().SubmodelDeleted(submodelID, semanticID, globalAssetIDs)
	m.Service.PublishBestEffort(ctx, ev, err)
}

func (m *Module) PublishPCN(ctx context.Context, previous, submodel types.ISubmodel, globalAssetIDs []string) {
	if !m.Enabled() || submodel == nil {
		return
	}
	for _, record := range PCNNewRecordValuesFromSubmodel(previous, submodel) {
		ev, err := m.Service.Builder().PCN(submodel.ID(), globalAssetIDs, record)
		m.Service.PublishBestEffort(ctx, ev, err)
	}
}
