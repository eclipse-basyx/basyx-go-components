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

package asyncjob

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStartCreatesOpaqueHandle(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)

	handleID, err := manager.Start(t.Context(), "owner-a", StartOptions{JobKind: "test"})
	require.NoError(t, err)
	require.Contains(t, handleID, "ASYNC-TEST-")
	require.NotContains(t, handleID, "|")
}

func TestGetForOwnerHidesForeignHandle(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)

	handleID, err := manager.Start(t.Context(), "owner-a", StartOptions{JobKind: "test"})
	require.NoError(t, err)

	_, found, err := manager.GetForOwner(t.Context(), handleID, "owner-b")
	require.NoError(t, err)
	require.False(t, found)

	_, found, err = manager.GetForOwner(t.Context(), handleID, "owner-a")
	require.NoError(t, err)
	require.True(t, found)
}

func TestGetForOwnerDoesNotRecoverForeignHandle(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)
	manager.leaseDuration = -time.Second

	handleID, err := manager.Start(t.Context(), "owner-a", StartOptions{JobKind: "test"})
	require.NoError(t, err)

	_, found, err := manager.GetForOwner(t.Context(), handleID, "owner-b")
	require.NoError(t, err)
	require.False(t, found)

	store := manager.store.(*memoryStore)
	store.Lock()
	record := store.records[handleID]
	store.Unlock()
	require.Equal(t, executionStateRunning, record.ExecutionState)
}

func TestGetForOwnerDoesNotDeleteForeignExpiredHandle(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)
	handleID, err := manager.Start(t.Context(), "owner-a", StartOptions{JobKind: "test"})
	require.NoError(t, err)
	require.NoError(t, manager.CompletePayload(t.Context(), handleID, map[string]any{"success": true}))

	store := manager.store.(*memoryStore)
	store.Lock()
	record := store.records[handleID]
	record.ExpiresAt = time.Now().UTC().Add(-time.Second)
	store.records[handleID] = record
	store.Unlock()

	_, found, err := manager.GetForOwner(t.Context(), handleID, "owner-b")
	require.NoError(t, err)
	require.False(t, found)

	store.Lock()
	_, stillStored := store.records[handleID]
	store.Unlock()
	require.True(t, stillStored)
}

func TestDeleteForOwnerDoesNotDeleteForeignHandle(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)
	handleID, err := manager.Start(t.Context(), "owner-a", StartOptions{JobKind: "test"})
	require.NoError(t, err)

	require.NoError(t, manager.DeleteForOwner(t.Context(), handleID, "owner-b"))
	_, found, err := manager.GetForOwner(t.Context(), handleID, "owner-a")
	require.NoError(t, err)
	require.True(t, found)

	require.NoError(t, manager.DeleteForOwner(t.Context(), handleID, "owner-a"))
	_, found, err = manager.GetForOwner(t.Context(), handleID, "owner-a")
	require.NoError(t, err)
	require.False(t, found)
}

func TestStartDoesNotRunGlobalRecovery(t *testing.T) {
	store := &recoveryCountingStore{recordStore: newMemoryStore()}
	manager := newManager(t.Context(), store, "ASYNC-TEST", time.Minute)

	_, err := manager.Start(t.Context(), "owner-a", StartOptions{JobKind: "test"})
	require.NoError(t, err)
	require.Zero(t, store.recoveryCalls)
}

func TestTransitionRetriesTransientStoreFailure(t *testing.T) {
	store := &flakyTransitionStore{
		memoryStore:       newMemoryStore(),
		failuresRemaining: 1,
	}
	manager := newManager(t.Context(), store, "ASYNC-TEST", time.Minute)
	handleID, err := manager.Start(t.Context(), "owner-a", StartOptions{JobKind: "test"})
	require.NoError(t, err)

	persistenceCtx, cancelPersistence := context.WithTimeout(t.Context(), time.Second)
	defer cancelPersistence()
	require.NoError(t, manager.CompletePayload(persistenceCtx, handleID, map[string]any{"success": true}))
	require.Equal(t, 2, store.transitionCalls)
}

func TestTransitionAcceptsAmbiguousCommittedWrite(t *testing.T) {
	store := &flakyTransitionStore{
		memoryStore:       newMemoryStore(),
		failuresRemaining: 1,
		commitBeforeError: true,
	}
	manager := newManager(t.Context(), store, "ASYNC-TEST", time.Minute)
	handleID, err := manager.Start(t.Context(), "owner-a", StartOptions{JobKind: "test"})
	require.NoError(t, err)

	persistenceCtx, cancelPersistence := context.WithTimeout(t.Context(), time.Second)
	defer cancelPersistence()
	require.NoError(t, manager.CompletePayload(persistenceCtx, handleID, map[string]any{"success": true}))
}

func TestExecutionSlotsAreBoundedAndReusable(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)
	releases := make([]func(), 0, defaultMaximumConcurrentExecutions)
	for range defaultMaximumConcurrentExecutions {
		release, acquired := manager.TryAcquireExecutionSlot()
		require.True(t, acquired)
		releases = append(releases, release)
	}

	_, acquired := manager.TryAcquireExecutionSlot()
	require.False(t, acquired)

	releases[0]()
	release, acquired := manager.TryAcquireExecutionSlot()
	require.True(t, acquired)
	release()
	for _, releaseSlot := range releases[1:] {
		releaseSlot()
	}
}

func TestExecutionSlotLeaseTransfersCapacityToWorker(t *testing.T) {
	manager, err := NewManagerWithExecutionCapacity("ASYNC-TEST", time.Minute, 1)
	require.NoError(t, err)

	lease, acquired := manager.TryAcquireExecutionSlotLease()
	require.True(t, acquired)
	releaseWorkerSlot, claimed := lease.Claim()
	require.True(t, claimed)
	lease.ReleaseIfUnclaimed()

	_, acquired = manager.TryAcquireExecutionSlot()
	require.False(t, acquired, "request cleanup released a slot already transferred to a worker")

	releaseWorkerSlot()
	release, acquired := manager.TryAcquireExecutionSlot()
	require.True(t, acquired)
	release()
}

func TestExecutionSlotLeaseReleasesUnclaimedCapacity(t *testing.T) {
	manager, err := NewManagerWithExecutionCapacity("ASYNC-TEST", time.Minute, 1)
	require.NoError(t, err)

	lease, acquired := manager.TryAcquireExecutionSlotLease()
	require.True(t, acquired)
	lease.ReleaseIfUnclaimed()

	release, acquired := manager.TryAcquireExecutionSlot()
	require.True(t, acquired)
	release()
}

func TestCompleteStartsTerminalRetention(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)

	handleID, err := manager.Start(t.Context(), "owner-a", StartOptions{JobKind: "test"})
	require.NoError(t, err)
	beforeCompletion := time.Now().UTC()
	require.NoError(t, manager.Complete(t.Context(), handleID, BulkResult{Success: true}))

	record, found, err := manager.Get(t.Context(), handleID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Completed", record.ExecutionState)
	require.GreaterOrEqual(t, record.ExpiresAt, beforeCompletion.Add(time.Minute))
	require.True(t, record.HasResult)
}

func TestAbandonedRunningRecordBecomesFailed(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)
	manager.leaseDuration = time.Millisecond

	handleID, err := manager.Start(t.Context(), "owner-a", StartOptions{JobKind: "test"})
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)

	record, found, err := manager.Get(t.Context(), handleID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Failed", record.ExecutionState)
	require.Equal(t, 500, record.ErrorStatus)
	require.NotZero(t, record.ExpiresAt)
}

func TestGetHidesExpiredTerminalRecord(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)

	handleID, err := manager.Start(t.Context(), "owner-a", StartOptions{JobKind: "test"})
	require.NoError(t, err)
	require.NoError(t, manager.CompletePayload(t.Context(), handleID, map[string]any{"success": true}))

	store := manager.store.(*memoryStore)
	store.Lock()
	record := store.records[handleID]
	record.ExpiresAt = time.Now().UTC().Add(-time.Second)
	store.records[handleID] = record
	store.Unlock()

	_, found, err := manager.Get(t.Context(), handleID)
	require.NoError(t, err)
	require.False(t, found)
}

func TestValidLeasePreventsDeadlineRecovery(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)

	handleID, err := manager.Start(t.Context(), "owner-a", StartOptions{
		JobKind:           "test",
		ExecutionDeadline: time.Now().UTC().Add(-time.Second),
	})
	require.NoError(t, err)

	record, found, err := manager.Get(t.Context(), handleID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Running", record.ExecutionState)
}

func TestExecutionContextStopsWithManagerLifecycle(t *testing.T) {
	lifecycleCtx, stopLifecycle := context.WithCancel(t.Context())
	manager := newManager(lifecycleCtx, newMemoryStore(), "ASYNC-TEST", time.Minute)

	executionCtx, cancelExecution := manager.NewExecutionContext(t.Context(), time.Minute)
	defer cancelExecution()
	stopLifecycle()

	select {
	case <-executionCtx.Done():
		require.ErrorIs(t, executionCtx.Err(), context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("execution context was not canceled with manager lifecycle")
	}
}

func TestExecutionContextHasDeadline(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)

	executionCtx, cancelExecution := manager.NewExecutionContext(t.Context(), time.Minute)
	defer cancelExecution()

	deadline, hasDeadline := executionCtx.Deadline()
	require.True(t, hasDeadline)
	require.WithinDuration(t, time.Now().UTC().Add(time.Minute), deadline, time.Second)
}

type recoveryCountingStore struct {
	recordStore
	recoveryCalls int
}

func (s *recoveryCountingStore) RecoverAbandoned(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
	_ time.Time,
) (int64, error) {
	s.recoveryCalls++
	return 0, nil
}

type flakyTransitionStore struct {
	*memoryStore
	failuresRemaining int
	transitionCalls   int
	commitBeforeError bool
}

func (s *flakyTransitionStore) Transition(
	ctx context.Context,
	handleID string,
	managerKey string,
	workerID string,
	terminal Record,
	expiresAt time.Time,
) (bool, error) {
	s.transitionCalls++
	if s.failuresRemaining == 0 {
		return s.memoryStore.Transition(ctx, handleID, managerKey, workerID, terminal, expiresAt)
	}

	s.failuresRemaining--
	if s.commitBeforeError {
		_, _ = s.memoryStore.Transition(ctx, handleID, managerKey, workerID, terminal, expiresAt)
	}
	return false, errors.New("ASYNCJOB-TEST-TRANSITION transient store failure")
}
