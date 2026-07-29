/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package asyncjob

import (
	"context"
	"sync"
	"time"
)

type memoryStore struct {
	sync.Mutex
	records map[string]Record
}

func newMemoryStore() *memoryStore {
	return &memoryStore{records: make(map[string]Record)}
}

func (s *memoryStore) Create(_ context.Context, handleID string, record Record) error {
	s.Lock()
	defer s.Unlock()
	s.records[handleID] = record
	return nil
}

func (s *memoryStore) Get(_ context.Context, handleID string, managerKey string) (Record, bool, error) {
	s.Lock()
	defer s.Unlock()
	record, found := s.records[handleID]
	if found && record.ManagerKey != managerKey {
		return Record{}, false, nil
	}
	return record, found, nil
}

func (s *memoryStore) Transition(
	_ context.Context,
	handleID string,
	managerKey string,
	workerID string,
	terminal Record,
	expiresAt time.Time,
) (bool, error) {
	s.Lock()
	defer s.Unlock()
	record, found := s.records[handleID]
	if !found || record.ManagerKey != managerKey ||
		record.ExecutionState != executionStateRunning || record.WorkerID != workerID {
		return false, nil
	}
	record.ExecutionState = terminal.ExecutionState
	record.Result = terminal.Result
	record.HasResult = terminal.HasResult
	record.Payload = terminal.Payload
	record.ErrorStatus = terminal.ErrorStatus
	record.ErrorBody = terminal.ErrorBody
	record.TerminalAt = time.Now().UTC()
	record.ExpiresAt = expiresAt
	record.LeaseExpiresAt = time.Time{}
	s.records[handleID] = record
	return true, nil
}

func (s *memoryStore) RenewLease(
	_ context.Context,
	handleID string,
	managerKey string,
	workerID string,
	expiresAt time.Time,
) (bool, error) {
	s.Lock()
	defer s.Unlock()
	record, found := s.records[handleID]
	if !found || record.ManagerKey != managerKey ||
		record.ExecutionState != executionStateRunning || record.WorkerID != workerID {
		return false, nil
	}
	record.LeaseExpiresAt = expiresAt
	s.records[handleID] = record
	return true, nil
}

func (s *memoryStore) Delete(_ context.Context, handleID string, managerKey string) error {
	s.Lock()
	defer s.Unlock()
	if record, found := s.records[handleID]; found && record.ManagerKey == managerKey {
		delete(s.records, handleID)
	}
	return nil
}

func (s *memoryStore) RecoverAbandoned(
	_ context.Context,
	managerKey string,
	targetHandleID string,
	now time.Time,
	expiresAt time.Time,
) (int64, error) {
	s.Lock()
	defer s.Unlock()
	var recovered int64
	for handleID, record := range s.records {
		if targetHandleID != "" && handleID != targetHandleID {
			continue
		}
		if record.ManagerKey != managerKey || record.ExecutionState != executionStateRunning {
			continue
		}
		leaseExpired := !record.LeaseExpiresAt.IsZero() && !record.LeaseExpiresAt.After(now)
		if !leaseExpired {
			continue
		}
		record.ExecutionState = executionStateFailed
		record.ErrorStatus = 500
		record.ErrorBody = abandonedErrorBody()
		record.TerminalAt = now
		record.ExpiresAt = expiresAt
		record.LeaseExpiresAt = time.Time{}
		s.records[handleID] = record
		recovered++
	}
	return recovered, nil
}

func (s *memoryStore) DeleteExpired(
	_ context.Context,
	managerKey string,
	now time.Time,
) (int64, error) {
	s.Lock()
	defer s.Unlock()
	var deleted int64
	for handleID, record := range s.records {
		if record.ManagerKey != managerKey || record.ExpiresAt.IsZero() || record.ExpiresAt.After(now) {
			continue
		}
		delete(s.records, handleID)
		deleted++
	}
	return deleted, nil
}

func abandonedErrorBody() map[string]any {
	return map[string]any{
		"executionState": executionStateFailed,
		"success":        false,
		"messages": []map[string]any{{
			"code":        "500",
			"messageType": "Error",
			"text":        "The worker processing the asynchronous request stopped before completion.",
		}},
	}
}
