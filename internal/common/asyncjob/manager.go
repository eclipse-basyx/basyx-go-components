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
	defaultRecordTTL                   = 15 * time.Minute
	defaultLeaseDuration               = 30 * time.Second
	defaultMaintenanceInterval         = time.Minute
	defaultPersistenceTimeout          = 5 * time.Second
	defaultMaximumConcurrentExecutions = 64
	initialTransitionRetryDelay        = 50 * time.Millisecond
	maximumTransitionRetryDelay        = 500 * time.Millisecond
	executionStateRunning              = "Running"
	executionStateCompleted            = "Completed"
	executionStateFailed               = "Failed"
)

var errTransitionRejected = errors.New("asynchronous job is no longer running")

// ItemFailure captures a failed item in a bulk request.
type ItemFailure struct {
	Index      int    `json:"index"`
	Identifier string `json:"identifier,omitempty"`
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

// BulkResult captures the final result of a bulk job.
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
	Get(context.Context, string, string, string) (Record, bool, error)
	Transition(context.Context, string, string, string, Record, time.Time) (bool, error)
	RenewLease(context.Context, string, string, string, time.Time) (bool, error)
	Delete(context.Context, string, string, string) error
	RecoverAbandoned(context.Context, string, string, string, time.Time, time.Time) (int64, error)
	DeleteExpired(context.Context, string, time.Time) (int64, error)
}

type transactionalRecordStore interface {
	CreateTx(context.Context, *sql.Tx, string, Record) error
	TransitionTx(context.Context, *sql.Tx, string, string, string, Record, time.Time) (bool, error)
}

type executionSlotLeaseContextKey struct{}

// Manager coordinates asynchronous job lifecycles through a shared store.
type Manager struct {
	store               recordStore
	prefix              string
	workerID            string
	ttl                 time.Duration
	leaseDuration       time.Duration
	maintenanceInterval time.Duration
	lifecycleContext    context.Context
	executionSlots      chan struct{}
	maintenanceMu       sync.Mutex
	lastCleanupAt       time.Time
}

// ExecutionSlotLease transfers one execution slot from request admission to a worker.
type ExecutionSlotLease struct {
	mu       sync.Mutex
	release  func()
	claimed  bool
	released bool
}

// NewManager creates an in-memory manager intended for isolated tests.
func NewManager(prefix string, ttl time.Duration) *Manager {
	return newManager(context.TODO(), newMemoryStore(), prefix, ttl)
}

// NewManagerWithExecutionCapacity creates a capacity-constrained in-memory manager.
func NewManagerWithExecutionCapacity(prefix string, ttl time.Duration, maximumConcurrentExecutions int) (*Manager, error) {
	if maximumConcurrentExecutions <= 0 {
		return nil, errors.New("ASYNCJOB-NEWMANAGER-INVALIDCAPACITY maximum concurrent executions must be greater than zero")
	}
	return newManagerWithExecutionCapacity(context.TODO(), newMemoryStore(), prefix, ttl, maximumConcurrentExecutions), nil
}

// NewPostgresManager creates a shared PostgreSQL-backed manager and starts maintenance.
func NewPostgresManager(
	ctx context.Context,
	db *sql.DB,
	prefix string,
	ttl time.Duration,
) (*Manager, error) {
	return newPostgresManager(ctx, db, prefix, ttl, defaultMaximumConcurrentExecutions)
}

// NewPostgresManagerWithExecutionCapacity creates a shared PostgreSQL-backed
// manager with the supplied local execution limit and starts maintenance.
func NewPostgresManagerWithExecutionCapacity(
	ctx context.Context,
	db *sql.DB,
	prefix string,
	ttl time.Duration,
	maximumConcurrentExecutions int,
) (*Manager, error) {
	if maximumConcurrentExecutions <= 0 {
		return nil, errors.New("ASYNCJOB-NEWPOSTGRES-INVALIDCAPACITY maximum concurrent executions must be greater than zero")
	}
	return newPostgresManager(ctx, db, prefix, ttl, maximumConcurrentExecutions)
}

func newPostgresManager(
	ctx context.Context,
	db *sql.DB,
	prefix string,
	ttl time.Duration,
	maximumConcurrentExecutions int,
) (*Manager, error) {
	if ctx == nil {
		return nil, errors.New("ASYNCJOB-NEWPOSTGRES-NILCTX lifecycle context must not be nil")
	}
	if db == nil {
		return nil, errors.New("ASYNCJOB-NEWPOSTGRES-NILDB database handle must not be nil")
	}

	manager := newManagerWithExecutionCapacity(ctx, newPostgresStore(db), prefix, ttl, maximumConcurrentExecutions)
	if err := manager.maintain(ctx, true); err != nil {
		return nil, fmt.Errorf("ASYNCJOB-NEWPOSTGRES-MAINTAIN %w", err)
	}
	go manager.runMaintenance(ctx)
	return manager, nil
}

func newManager(lifecycleContext context.Context, store recordStore, prefix string, ttl time.Duration) *Manager {
	return newManagerWithExecutionCapacity(lifecycleContext, store, prefix, ttl, defaultMaximumConcurrentExecutions)
}

func newManagerWithExecutionCapacity(
	lifecycleContext context.Context,
	store recordStore,
	prefix string,
	ttl time.Duration,
	maximumConcurrentExecutions int,
) *Manager {
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
		lifecycleContext:    lifecycleContext,
		executionSlots:      make(chan struct{}, maximumConcurrentExecutions),
	}
}

// TryAcquireExecutionSlotLease reserves capacity that can be handed to a worker.
func (m *Manager) TryAcquireExecutionSlotLease() (*ExecutionSlotLease, bool) {
	release, acquired := m.TryAcquireExecutionSlot()
	if !acquired {
		return nil, false
	}
	return &ExecutionSlotLease{release: release}, true
}

// Claim transfers responsibility for releasing the slot to the caller.
func (lease *ExecutionSlotLease) Claim() (func(), bool) {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.claimed || lease.released {
		return func() {}, false
	}
	lease.claimed = true
	return lease.Release, true
}

// ReleaseIfUnclaimed releases request-owned capacity unless a worker claimed it.
func (lease *ExecutionSlotLease) ReleaseIfUnclaimed() {
	lease.mu.Lock()
	if lease.claimed || lease.released {
		lease.mu.Unlock()
		return
	}
	lease.released = true
	release := lease.release
	lease.mu.Unlock()
	release()
}

// Release returns claimed capacity and is safe to call more than once.
func (lease *ExecutionSlotLease) Release() {
	lease.mu.Lock()
	if lease.released {
		lease.mu.Unlock()
		return
	}
	lease.released = true
	release := lease.release
	lease.mu.Unlock()
	release()
}

// WithExecutionSlotLease attaches pre-staging execution capacity to a request.
func WithExecutionSlotLease(ctx context.Context, lease *ExecutionSlotLease) context.Context {
	return context.WithValue(ctx, executionSlotLeaseContextKey{}, lease)
}

// ExecutionSlotLeaseFromContext returns pre-staging execution capacity, if present.
func ExecutionSlotLeaseFromContext(ctx context.Context) (*ExecutionSlotLease, bool) {
	lease, ok := ctx.Value(executionSlotLeaseContextKey{}).(*ExecutionSlotLease)
	return lease, ok && lease != nil
}

// NewExecutionContext creates a request-value-preserving context bounded by both
// the manager lifecycle and the supplied execution timeout.
func (m *Manager) NewExecutionContext(
	requestContext context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	executionContext, cancel := context.WithTimeout(context.WithoutCancel(requestContext), timeout)
	stopLifecycleCancellation := context.AfterFunc(m.lifecycleContext, cancel)
	return executionContext, func() {
		stopLifecycleCancellation()
		cancel()
	}
}

// NewPersistenceContext creates a short-lived context for recording a terminal
// state after an execution context has ended.
func NewPersistenceContext(executionContext context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(executionContext), defaultPersistenceTimeout)
}

// TryAcquireExecutionSlot reserves bounded local execution capacity.
// The returned release function is safe to call more than once.
func (m *Manager) TryAcquireExecutionSlot() (func(), bool) {
	select {
	case m.executionSlots <- struct{}{}:
		var releaseOnce sync.Once
		return func() {
			releaseOnce.Do(func() {
				<-m.executionSlots
			})
		}, true
	default:
		return func() {}, false
	}
}

// Start persists a new running job and returns its opaque handle.
func (m *Manager) Start(ctx context.Context, ownerKey string, options StartOptions) (string, error) {
	if err := m.cleanup(ctx, false); err != nil {
		return "", fmt.Errorf("ASYNCJOB-START-CLEANUP %w", err)
	}
	return m.start(ctx, nil, ownerKey, options)
}

// StartTx persists a running job in the caller's transaction.
func (m *Manager) StartTx(ctx context.Context, tx *sql.Tx, ownerKey string, options StartOptions) (string, error) {
	if tx == nil {
		return "", errors.New("ASYNCJOB-STARTTX-NILTX transaction must not be nil")
	}
	return m.start(ctx, tx, ownerKey, options)
}

func (m *Manager) start(ctx context.Context, tx *sql.Tx, ownerKey string, options StartOptions) (string, error) {
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
	var createErr error
	if tx == nil {
		createErr = m.store.Create(ctx, handle, record)
	} else if store, ok := m.store.(transactionalRecordStore); ok {
		createErr = store.CreateTx(ctx, tx, handle, record)
	} else {
		createErr = errors.New("transactional asynchronous job storage is unavailable")
	}
	if createErr != nil {
		return "", fmt.Errorf("ASYNCJOB-START-CREATE %w", createErr)
	}
	return handle, nil
}

// Complete stores the terminal result of a bulk job.
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

// CompletePayloadTx stores a successful terminal payload in the caller's transaction.
func (m *Manager) CompletePayloadTx(ctx context.Context, tx *sql.Tx, handleID string, payload any) error {
	if tx == nil {
		return errors.New("ASYNCJOB-COMPLETETX-NILTX transaction must not be nil")
	}
	store, ok := m.store.(transactionalRecordStore)
	if !ok {
		return errors.New("ASYNCJOB-COMPLETETX-UNSUPPORTED transactional asynchronous job storage is unavailable")
	}
	terminal := Record{ExecutionState: executionStateCompleted, Payload: payload}
	updated, err := store.TransitionTx(ctx, tx, handleID, m.prefix, m.workerID, terminal, time.Now().UTC().Add(m.ttl))
	if err != nil {
		return fmt.Errorf("ASYNCJOB-COMPLETETX-EXECUTE %w", err)
	}
	if !updated {
		return fmt.Errorf("ASYNCJOB-COMPLETETX-REJECTED %w", errTransitionRejected)
	}
	return nil
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
	retryContext, cancelRetry := transitionRetryContext(ctx)
	defer cancelRetry()

	retryDelay := initialTransitionRetryDelay
	for {
		updated, err := m.store.Transition(
			retryContext,
			handleID,
			m.prefix,
			m.workerID,
			terminal,
			time.Now().UTC().Add(m.ttl),
		)
		if updated {
			return nil
		}
		if m.transitionAlreadyApplied(retryContext, handleID, terminal.ExecutionState) {
			return nil
		}
		if err == nil {
			return fmt.Errorf("ASYNCJOB-TRANSITION-REJECTED %w", errTransitionRejected)
		}
		if !waitForTransitionRetry(retryContext, retryDelay) {
			return fmt.Errorf("ASYNCJOB-TRANSITION-EXECUTE %w", err)
		}
		retryDelay = min(retryDelay*2, maximumTransitionRetryDelay)
	}
}

func transitionRetryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultPersistenceTimeout)
}

func (m *Manager) transitionAlreadyApplied(ctx context.Context, handleID string, executionState string) bool {
	record, found, err := m.store.Get(ctx, handleID, m.prefix, "")
	return err == nil && found &&
		record.WorkerID == m.workerID &&
		record.ExecutionState == executionState
}

func waitForTransitionRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Get returns a non-expired record and recovers the requested handle when its lease expired.
func (m *Manager) Get(ctx context.Context, handleID string) (Record, bool, error) {
	return m.get(ctx, handleID, "")
}

func (m *Manager) get(ctx context.Context, handleID string, ownerKey string) (Record, bool, error) {
	record, found, err := m.store.Get(ctx, handleID, m.prefix, ownerKey)
	if err != nil {
		return Record{}, false, fmt.Errorf("ASYNCJOB-GET-READ %w", err)
	}
	if !found {
		return Record{}, false, nil
	}

	now := time.Now().UTC()
	if record.ExecutionState != executionStateRunning {
		if record.ExpiresAt.IsZero() || record.ExpiresAt.After(now) {
			return record, true, nil
		}
		if err := m.store.Delete(ctx, handleID, m.prefix, ownerKey); err != nil {
			return Record{}, false, fmt.Errorf("ASYNCJOB-GET-DELETEEXPIRED %w", err)
		}
		return Record{}, false, nil
	}

	if record.LeaseExpiresAt.IsZero() || record.LeaseExpiresAt.After(now) {
		return record, true, nil
	}
	if err := m.recover(ctx, handleID, ownerKey); err != nil {
		return Record{}, false, err
	}
	record, found, err = m.store.Get(ctx, handleID, m.prefix, ownerKey)
	if err != nil {
		return Record{}, false, fmt.Errorf("ASYNCJOB-GET-READRECOVERED %w", err)
	}
	return record, found, nil
}

// GetForOwner returns a record only when handle and owner key match.
func (m *Manager) GetForOwner(ctx context.Context, handleID string, ownerKey string) (Record, bool, error) {
	return m.get(ctx, handleID, normalizeOwnerKey(ownerKey))
}

// Delete removes a handle from the shared store.
func (m *Manager) Delete(ctx context.Context, handleID string) error {
	return m.delete(ctx, handleID, "")
}

// DeleteForOwner removes a handle only when handle and owner key match.
func (m *Manager) DeleteForOwner(ctx context.Context, handleID string, ownerKey string) error {
	return m.delete(ctx, handleID, normalizeOwnerKey(ownerKey))
}

func (m *Manager) delete(ctx context.Context, handleID string, ownerKey string) error {
	if err := m.store.Delete(ctx, handleID, m.prefix, ownerKey); err != nil {
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
					m.prefix,
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
	if err := m.recover(ctx, "", ""); err != nil {
		return err
	}
	return m.cleanup(ctx, force)
}

func (m *Manager) cleanup(ctx context.Context, force bool) error {
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

func (m *Manager) recover(ctx context.Context, handleID string, ownerKey string) error {
	now := time.Now().UTC()
	if _, err := m.store.RecoverAbandoned(ctx, m.prefix, handleID, ownerKey, now, now.Add(m.ttl)); err != nil {
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
