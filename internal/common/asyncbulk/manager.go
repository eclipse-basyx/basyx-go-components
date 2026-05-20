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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

// Package asyncbulk provides in-memory handle tracking for asynchronous bulk operations.
package asyncbulk

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

const (
	defaultRecordTTL        = 15 * time.Minute
	defaultCleanupInterval  = time.Minute
	executionStateRunning   = "Running"
	executionStateCompleted = "Completed"
)

// ItemFailure captures a failed descriptor operation in a bulk request.
type ItemFailure struct {
	Index      int    `json:"index"`
	Identifier string `json:"identifier,omitempty"`
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

// OperationResult captures the final result of an async bulk operation.
type OperationResult struct {
	ExecutionState  string        `json:"executionState"`
	Success         bool          `json:"success"`
	ProcessedCount  int           `json:"processedCount"`
	SuccessfulCount int           `json:"successfulCount"`
	FailedCount     int           `json:"failedCount"`
	Failures        []ItemFailure `json:"failures,omitempty"`
}

// Record stores a single async bulk execution state.
type Record struct {
	ExecutionState string
	Result         OperationResult
	OwnerKey       string
	Metadata       map[string]string
	Payload        any
	ErrorStatus    int
	ErrorBody      any
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

// Manager stores async bulk operations and their lifecycle.
type Manager struct {
	sync.Mutex
	records         map[string]Record
	prefix          string
	ttl             time.Duration
	cleanupInterval time.Duration
	lastCleanupAt   time.Time
}

// NewManager creates a new async bulk operation manager.
func NewManager(prefix string, ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = defaultRecordTTL
	}
	if prefix == "" {
		prefix = "BULK"
	}
	return &Manager{
		records:         make(map[string]Record),
		prefix:          prefix,
		ttl:             ttl,
		cleanupInterval: defaultCleanupInterval,
	}
}

// Start registers a new running operation and returns the generated handle id.
func (m *Manager) Start(ownerKey string) (string, error) {
	now := time.Now().UTC()
	handle, err := newHandleID(m.prefix)
	if err != nil {
		return "", fmt.Errorf("ASYNCBULK-START-GENERATEHANDLE %w", err)
	}

	m.Lock()
	m.cleanupLocked(now)
	m.records[handle] = Record{
		ExecutionState: executionStateRunning,
		OwnerKey:       normalizeOwnerKey(ownerKey),
		CreatedAt:      now,
		ExpiresAt:      now.Add(m.ttl),
	}
	m.Unlock()

	return handle, nil
}

// Complete marks a running operation as completed and stores the final result.
func (m *Manager) Complete(handleID string, result OperationResult) {
	now := time.Now().UTC()

	m.Lock()
	m.cleanupLocked(now)
	record, found := m.records[handleID]
	if !found {
		m.Unlock()
		return
	}

	result.ExecutionState = executionStateCompleted
	record.ExecutionState = executionStateCompleted
	record.Result = result
	m.records[handleID] = record
	m.Unlock()
}

// Get returns a record by handle id when present and not expired.
func (m *Manager) Get(handleID string) (Record, bool) {
	now := time.Now().UTC()

	m.Lock()
	m.cleanupLocked(now)
	record, found := m.records[handleID]
	if found && now.After(record.ExpiresAt) {
		delete(m.records, handleID)
		m.Unlock()
		return Record{}, false
	}
	m.Unlock()

	return record, found
}

// GetForOwner returns a record only when handle and owner key match.
func (m *Manager) GetForOwner(handleID string, ownerKey string) (Record, bool) {
	record, found := m.Get(handleID)
	if !found {
		return Record{}, false
	}
	if record.OwnerKey != normalizeOwnerKey(ownerKey) {
		return Record{}, false
	}
	return record, true
}

// Update mutates an existing record by handle id.
func (m *Manager) Update(handleID string, updateFn func(record Record) Record) bool {
	now := time.Now().UTC()

	m.Lock()
	m.cleanupLocked(now)
	record, found := m.records[handleID]
	if !found || now.After(record.ExpiresAt) {
		delete(m.records, handleID)
		m.Unlock()
		return false
	}

	updatedRecord := updateFn(record)
	updatedRecord.OwnerKey = record.OwnerKey
	updatedRecord.CreatedAt = record.CreatedAt
	updatedRecord.ExpiresAt = record.ExpiresAt
	m.records[handleID] = updatedRecord
	m.Unlock()

	return true
}

// Delete removes a handle from the manager.
func (m *Manager) Delete(handleID string) {
	now := time.Now().UTC()

	m.Lock()
	m.cleanupLocked(now)
	delete(m.records, handleID)
	m.Unlock()
}

func (m *Manager) cleanupLocked(now time.Time) {
	if !m.lastCleanupAt.IsZero() && now.Sub(m.lastCleanupAt) < m.cleanupInterval {
		return
	}

	for handleID, record := range m.records {
		if now.After(record.ExpiresAt) {
			delete(m.records, handleID)
		}
	}

	m.lastCleanupAt = now
}

func newHandleID(prefix string) (string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", prefix, base64.RawURLEncoding.EncodeToString(randomBytes)), nil
}

func normalizeOwnerKey(ownerKey string) string {
	if ownerKey == "" {
		return "anonymous"
	}
	return ownerKey
}
