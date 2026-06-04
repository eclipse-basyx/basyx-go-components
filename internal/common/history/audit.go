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

import "context"

// AuditContext carries vendor-neutral request and identity metadata.
type AuditContext struct {
	ActorSubject        string
	ActorIssuer         string
	ClientID            string
	AuthorizationResult string
	PolicyID            string
	MatchedRuleID       string
	RequestID           string
	CorrelationID       string
	SourceIP            string
	UserAgent           string
	Operation           string
	Endpoint            string
	HTTPMethod          string
}

type auditContextKey struct{}

// ContextWithAudit stores audit metadata in a context.
func ContextWithAudit(ctx context.Context, audit AuditContext) context.Context {
	return context.WithValue(ctx, auditContextKey{}, audit)
}

// FromContext returns audit metadata stored in ctx.
func FromContext(ctx context.Context) AuditContext {
	if ctx == nil {
		return AuditContext{}
	}
	audit, _ := ctx.Value(auditContextKey{}).(AuditContext)
	return audit
}
