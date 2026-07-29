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
