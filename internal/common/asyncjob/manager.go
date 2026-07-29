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
// Author: Aaron Zielstorff (Fraunhofer IESE)

// Package asyncjob provides shared handle tracking for asynchronous server jobs.
package asyncjob

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	defaultRecordTTL           = 15 * time.Minute
	defaultLeaseDuration       = 30 * time.Second
	defaultMaintenanceInterval = time.Minute
	executionStateRunning      = "Running"
	executionStateCompleted    = "Completed"
	executionStateFailed       = "Failed"
)

var errTransitionRejected = errors.New("asynchronous job is no longer running")

// ItemFailure captures a failed item in a bulk request.
type ItemFailure struct {
	Index      int    `json:"index"`
	Identifier string `json:"identifier,omitempty"`
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

// BulkResult captures the final result of an asynchronous bulk job.
type BulkResult struct {
	ExecutionState  string        `json:"executionState"`
	Success         bool          `json:"success"`
	ProcessedCount  int           `json:"processedCount"`
	SuccessfulCount int           `json:"successfulCount"`
	FailedCount     int           `json:"failedCount"`
	Failures        []ItemFailure `json:"failures,omitempty"`
}

// Record stores one asynchronous job state.
type Record struct {
	ManagerKey        string
	JobKind           string
	ExecutionState    string
	Result            BulkResult
	HasResult         bool
	OwnerKey          string
	Metadata          map[string]string
	Payload           any
	ErrorStatus       int
	ErrorBody         any
	WorkerID          string
	CreatedAt         time.Time
	TerminalAt        time.Time
	ExpiresAt         time.Time
	LeaseExpiresAt    time.Time
	ExecutionDeadline time.Time
}

// StartOptions describes the persistent identity and execution boundary of a new job.
type StartOptions struct {
	JobKind           string
	Metadata          map[string]string
	ExecutionDeadline time.Time
}

type recordStore interface {
	Create(context.Context, string, Record) error
	Get(context.Context, string) (Record, bool, error)
	Transition(context.Context, string, string, Record, time.Time) (bool, error)
	RenewLease(context.Context, string, string, time.Time) (bool, error)
	Delete(context.Context, string) error
	RecoverAbandoned(context.Context, string, time.Time, time.Time) (int64, error)
	DeleteExpired(context.Context, string, time.Time) (int64, error)
}

// Manager coordinates asynchronous job lifecycles through a shared store.
type Manager struct {
	store               recordStore
	prefix              string
	workerID            string
	ttl                 time.Duration
	leaseDuration       time.Duration
	maintenanceInterval time.Duration
	maintenanceMu       sync.Mutex
	lastCleanupAt       time.Time
}

// NewManager creates an in-memory manager intended for isolated tests.
func NewManager(prefix string, ttl time.Duration) *Manager {
	return newManager(newMemoryStore(), prefix, ttl)
}

// NewPostgresManager creates a shared PostgreSQL-backed manager and starts maintenance.
func NewPostgresManager(
	ctx context.Context,
	db *sql.DB,
	prefix string,
	ttl time.Duration,
) (*Manager, error) {
	if ctx == nil {
		return nil, errors.New("ASYNCJOB-NEWPOSTGRES-NILCTX lifecycle context must not be nil")
	}
	if db == nil {
		return nil, errors.New("ASYNCJOB-NEWPOSTGRES-NILDB database handle must not be nil")
	}

	manager := newManager(newPostgresStore(db), prefix, ttl)
	if err := manager.maintain(ctx, true); err != nil {
		return nil, fmt.Errorf("ASYNCJOB-NEWPOSTGRES-MAINTAIN %w", err)
	}
	go manager.runMaintenance(ctx)
	return manager, nil
}

func newManager(store recordStore, prefix string, ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = defaultRecordTTL
	}
	if prefix == "" {
		prefix = "ASYNC"
	}
	return &Manager{
		store:               store,
		prefix:              prefix,
		workerID:            newWorkerID(),
		ttl:                 ttl,
		leaseDuration:       defaultLeaseDuration,
		maintenanceInterval: defaultMaintenanceInterval,
	}
}

// Start persists a new running job and returns its opaque handle.
func (m *Manager) Start(ctx context.Context, ownerKey string, options StartOptions) (string, error) {
	if err := m.maintain(ctx, false); err != nil {
		return "", fmt.Errorf("ASYNCJOB-START-MAINTAIN %w", err)
	}

	handle, err := newHandleID(m.prefix)
	if err != nil {
		return "", fmt.Errorf("ASYNCJOB-START-GENERATEHANDLE %w", err)
	}

	now := time.Now().UTC()
	jobKind := options.JobKind
	if jobKind == "" {
		jobKind = m.prefix
	}
	record := Record{
		ManagerKey:        m.prefix,
		JobKind:           jobKind,
		ExecutionState:    executionStateRunning,
		OwnerKey:          normalizeOwnerKey(ownerKey),
		Metadata:          cloneMetadata(options.Metadata),
		WorkerID:          m.workerID,
		CreatedAt:         now,
		LeaseExpiresAt:    now.Add(m.leaseDuration),
		ExecutionDeadline: options.ExecutionDeadline.UTC(),
	}
	if err := m.store.Create(ctx, handle, record); err != nil {
		return "", fmt.Errorf("ASYNCJOB-START-CREATE %w", err)
	}
	return handle, nil
}

// Complete stores the terminal result of an asynchronous bulk job.
func (m *Manager) Complete(ctx context.Context, handleID string, result BulkResult) error {
	result.ExecutionState = executionStateCompleted
	return m.transition(ctx, handleID, Record{
		ExecutionState: executionStateCompleted,
		Result:         result,
		HasResult:      true,
	})
}

// CompletePayload stores a successful terminal payload.
func (m *Manager) CompletePayload(ctx context.Context, handleID string, payload any) error {
	return m.transition(ctx, handleID, Record{
		ExecutionState: executionStateCompleted,
		Payload:        payload,
	})
}

// Fail stores a failed terminal response.
func (m *Manager) Fail(ctx context.Context, handleID string, status int, body any) error {
	return m.transition(ctx, handleID, Record{
		ExecutionState: executionStateFailed,
		ErrorStatus:    status,
		ErrorBody:      body,
	})
}

func (m *Manager) transition(ctx context.Context, handleID string, terminal Record) error {
	now := time.Now().UTC()
	updated, err := m.store.Transition(ctx, handleID, m.workerID, terminal, now.Add(m.ttl))
	if err != nil {
		return fmt.Errorf("ASYNCJOB-TRANSITION-EXECUTE %w", err)
	}
	if !updated {
		return fmt.Errorf("ASYNCJOB-TRANSITION-REJECTED %w", errTransitionRejected)
	}
	return nil
}

// Get returns a record after recovering abandoned work in the manager namespace.
func (m *Manager) Get(ctx context.Context, handleID string) (Record, bool, error) {
	if err := m.recover(ctx); err != nil {
		return Record{}, false, err
	}
	record, found, err := m.store.Get(ctx, handleID)
	if err != nil {
		return Record{}, false, fmt.Errorf("ASYNCJOB-GET-READ %w", err)
	}
	return record, found && record.ManagerKey == m.prefix, nil
}

// GetForOwner returns a record only when handle and owner key match.
func (m *Manager) GetForOwner(ctx context.Context, handleID string, ownerKey string) (Record, bool, error) {
	record, found, err := m.Get(ctx, handleID)
	if err != nil || !found {
		return Record{}, false, err
	}
	if record.OwnerKey != normalizeOwnerKey(ownerKey) {
		return Record{}, false, nil
	}
	return record, true, nil
}

// Delete removes a handle from the shared store.
func (m *Manager) Delete(ctx context.Context, handleID string) error {
	if err := m.store.Delete(ctx, handleID); err != nil {
		return fmt.Errorf("ASYNCJOB-DELETE-EXECUTE %w", err)
	}
	return nil
}

// KeepAlive renews the worker lease until the returned function is called or ctx ends.
func (m *Manager) KeepAlive(ctx context.Context, handleID string) func() {
	heartbeatCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(m.leaseDuration / 3)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case now := <-ticker.C:
				renewed, err := m.store.RenewLease(
					heartbeatCtx,
					handleID,
					m.workerID,
					now.UTC().Add(m.leaseDuration),
				)
				if err != nil {
					slog.ErrorContext(heartbeatCtx, "async handle lease renewal failed", "error.code", "ASYNCJOB-KEEPALIVE-RENEW", "error", err, "async_job.handle_id", handleID)
					continue
				}
				if !renewed {
					return
				}
			}
		}
	}()
	return cancel
}

func (m *Manager) runMaintenance(ctx context.Context) {
	ticker := time.NewTicker(m.maintenanceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.maintain(ctx, true); err != nil {
				slog.ErrorContext(ctx, "async handle maintenance failed", "error.code", "ASYNCJOB-MAINTENANCE-EXECUTE", "error", err)
			}
		}
	}
}

func (m *Manager) maintain(ctx context.Context, force bool) error {
	if err := m.recover(ctx); err != nil {
		return err
	}

	m.maintenanceMu.Lock()
	defer m.maintenanceMu.Unlock()
	now := time.Now().UTC()
	if !force && !m.lastCleanupAt.IsZero() && now.Sub(m.lastCleanupAt) < m.maintenanceInterval {
		return nil
	}
	if _, err := m.store.DeleteExpired(ctx, m.prefix, now); err != nil {
		return fmt.Errorf("ASYNCJOB-MAINTAIN-CLEANUP %w", err)
	}
	m.lastCleanupAt = now
	return nil
}

func (m *Manager) recover(ctx context.Context) error {
	now := time.Now().UTC()
	if _, err := m.store.RecoverAbandoned(ctx, m.prefix, now, now.Add(m.ttl)); err != nil {
		return fmt.Errorf("ASYNCJOB-RECOVER-EXECUTE %w", err)
	}
	return nil
}

func newHandleID(prefix string) (string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", prefix, base64.RawURLEncoding.EncodeToString(randomBytes)), nil
}

func newWorkerID() string {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Sprintf("worker-%d", time.Now().UTC().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(randomBytes)
}

func normalizeOwnerKey(ownerKey string) string {
	if ownerKey == "" {
		return "anonymous"
	}
	return ownerKey
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		result[key] = value
	}
	return result
}
