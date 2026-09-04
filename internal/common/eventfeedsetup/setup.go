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

// Package eventfeedsetup wires the eventfeed module into the process-wide
// history mutation hook.
package eventfeedsetup

import (
	"context"
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
)

// Bind registers the Event Feed mutation sink on the process-wide history hook.
func Bind(module *eventfeed.Module) {
	if module == nil || !module.Enabled() {
		return
	}
	history.SetMutationSink(&historySink{inner: eventfeed.NewMutationSink(module.Service)})
	module.SetOnStop(history.ClearMutationSink)
}

type historySink struct {
	inner *eventfeed.MutationSink
}

func (s *historySink) HandleMutation(ctx context.Context, tx *sql.Tx, mutation history.Mutation) error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.HandleMutation(ctx, tx, eventfeed.Mutation{
		Table:            mutation.Table,
		Identifier:       mutation.Identifier,
		ChangeType:       mutation.ChangeType,
		PreviousSnapshot: mutation.PreviousSnapshot,
		Snapshot:         mutation.Snapshot,
		Deleted:          mutation.Deleted,
	})
}
