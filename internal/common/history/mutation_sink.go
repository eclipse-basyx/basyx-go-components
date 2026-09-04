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

package history

import (
	"context"
	"database/sql"
	"sync"
)

// Mutation is one authoritative model change already executing inside tx.
//
// Persistence layers call AppendVersionTx / AppendMutatedVersionTx for every
// successful create, update, or delete, including PATCH, SubmodelElement, file,
// and thumbnail paths. Optional MutationSink implementations (Event Feed) observe
// that same transaction-scoped change instead of reconstructing it after commit.
type Mutation struct {
	Table            string
	Identifier       string
	ChangeType       string
	PreviousSnapshot map[string]any
	Snapshot         map[string]any
	Deleted          bool
}

// MutationSink consumes normalized mutations inside the writer transaction.
type MutationSink interface {
	HandleMutation(ctx context.Context, tx *sql.Tx, mutation Mutation) error
}

var (
	mutationSinkMu sync.RWMutex
	mutationSink   MutationSink
)

// SetMutationSink registers the process-wide mutation sink. Pass nil to clear.
func SetMutationSink(sink MutationSink) {
	mutationSinkMu.Lock()
	mutationSink = sink
	mutationSinkMu.Unlock()
}

// ClearMutationSink removes the process-wide mutation sink.
func ClearMutationSink() {
	SetMutationSink(nil)
}

func mutationSinkRegistered() bool {
	mutationSinkMu.RLock()
	defer mutationSinkMu.RUnlock()
	return mutationSink != nil
}

func notifyMutationSink(ctx context.Context, tx *sql.Tx, mutation Mutation) error {
	if tx == nil {
		return nil
	}
	mutationSinkMu.RLock()
	sink := mutationSink
	mutationSinkMu.RUnlock()
	if sink == nil {
		return nil
	}
	return sink.HandleMutation(ctx, tx, mutation)
}
