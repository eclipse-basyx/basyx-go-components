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

// Package eventfeed implements the AAS Event Feed API: building, persisting,
// querying, and serving CloudEvents-style change notifications.
package eventfeed

import (
	"context"
	"sync"
)

// RecordAuthorizer decides whether a feed record may be returned to the caller.
type RecordAuthorizer interface {
	Allow(ctx context.Context, eventType, subject string) bool
}

var (
	recordAuthorizerMu sync.RWMutex
	recordAuthorizer   RecordAuthorizer
)

// SetRecordAuthorizer registers the process-wide event visibility check.
func SetRecordAuthorizer(authorizer RecordAuthorizer) {
	recordAuthorizerMu.Lock()
	recordAuthorizer = authorizer
	recordAuthorizerMu.Unlock()
}

func currentRecordAuthorizer() RecordAuthorizer {
	recordAuthorizerMu.RLock()
	defer recordAuthorizerMu.RUnlock()
	return recordAuthorizer
}
