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

// AuditContext carries vendor-neutral request and identity metadata for history rows.
//
// The fields are intentionally strings so API layers can copy already-normalized
// security and request metadata into history without coupling this package to a
// specific authentication provider or HTTP framework.
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

// ContextWithAudit stores audit metadata in a context for later history writes.
//
// AppendVersionTx and AppendMutatedVersionTx read this metadata through
// FromContext and persist it with the row hash, making audit metadata part of
// the tamper-evident history chain.
//
// Parameters:
//   - ctx: Base context to extend.
//   - audit: Request and identity metadata to store.
//
// Returns:
//   - context.Context: Context containing audit metadata.
//
// Example:
//
//	ctx = ContextWithAudit(ctx, AuditContext{
//		RequestID:  requestID,
//		HTTPMethod: http.MethodPut,
//		Endpoint:   endpoint,
//	})
func ContextWithAudit(ctx context.Context, audit AuditContext) context.Context {
	return context.WithValue(ctx, auditContextKey{}, audit)
}

// FromContext returns audit metadata stored in ctx.
//
// A nil context or a context without audit metadata returns the zero value, so
// callers can use it unconditionally while building history rows.
//
// Parameters:
//   - ctx: Context that may contain AuditContext metadata.
//
// Returns:
//   - AuditContext: Stored metadata, or the zero value when absent.
//
// Example:
//
//	audit := FromContext(ctx)
//	event.RequestID = audit.RequestID
func FromContext(ctx context.Context) AuditContext {
	if ctx == nil {
		return AuditContext{}
	}
	audit, _ := ctx.Value(auditContextKey{}).(AuditContext)
	return audit
}
