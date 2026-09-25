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

package auth

import (
	"context"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/audit"
	"sync"
)

type rebacMutationAuditKey struct{}
type rebacMutationAudit struct {
	mu     sync.Mutex
	events []audit.Event
}

// BeginReBACMutationAudit retains attempted decisions if the writer transaction fails.
// Finish must be called after that transaction has committed or rolled back.
func BeginReBACMutationAudit(ctx context.Context) (context.Context, func(error) error) {
	request, ok := ctx.Value(rebacRequestKey{}).(*rebacRequest)
	if !ok || request == nil || !request.security.audit.Enabled {
		return ctx, func(err error) error { return err }
	}
	journal := &rebacMutationAudit{}
	ctx = context.WithValue(ctx, rebacMutationAuditKey{}, journal)
	return ctx, func(operationErr error) error {
		if operationErr == nil {
			return nil
		}
		journal.mu.Lock()
		defer journal.mu.Unlock()
		for _, event := range journal.events {
			event.Payload.Details["transaction"] = "failed_or_unconfirmed"
			if _, err := request.security.audit.Repository.Record(ctx, "authorization/"+request.security.cfg.ReBAC.Scope, event); err != nil {
				return common.NewErrServiceUnavailable("REBAC-AUDIT-ROLLBACK " + err.Error())
			}
		}
		return operationErr
	}
}

func rememberReBACMutationAudit(ctx context.Context, event audit.Event) {
	journal, ok := ctx.Value(rebacMutationAuditKey{}).(*rebacMutationAudit)
	if !ok {
		return
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	journal.events = append(journal.events, event)
}
