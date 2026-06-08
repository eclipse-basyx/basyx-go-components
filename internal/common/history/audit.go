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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	// AuthorizationResultSystemInternal marks history rows written by trusted internal service work.
	AuthorizationResultSystemInternal = "SYSTEM_INTERNAL"
	// AuditHTTPMethodSystem marks history rows that were not caused directly by an HTTP method.
	AuditHTTPMethodSystem = "SYSTEM"
)

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

// SystemAuditOptions configures synthetic audit metadata for trusted internal work.
type SystemAuditOptions struct {
	ActorSubject  string
	ActorIssuer   string
	ClientID      string
	Operation     string
	Endpoint      string
	RequestID     string
	CorrelationID string
	SourceIP      string
	UserAgent     string
	PolicyID      string
	MatchedRuleID string
	HTTPMethod    string
	IDPrefix      string
	Authorization string
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

// ContextWithSystemAudit stores synthetic audit metadata for non-HTTP service work.
//
// The helper is intended for startup jobs, scheduled jobs, and internal recovery
// or synchronization work. It marks the authorization result as
// SYSTEM_INTERNAL by default and generates missing request/correlation IDs so
// background writes are still traceable without inventing a human identity.
//
// Parameters:
//   - ctx: Base context to extend.
//   - options: System actor and operation metadata.
//
// Returns:
//   - context.Context: Context containing synthetic system audit metadata.
func ContextWithSystemAudit(ctx context.Context, options SystemAuditOptions) context.Context {
	requestID := strings.TrimSpace(options.RequestID)
	if requestID == "" {
		requestID = NewAuditID(options.IDPrefix)
	}
	correlationID := strings.TrimSpace(options.CorrelationID)
	if correlationID == "" {
		correlationID = requestID
	}
	authorization := strings.TrimSpace(options.Authorization)
	if authorization == "" {
		authorization = AuthorizationResultSystemInternal
	}
	httpMethod := strings.TrimSpace(options.HTTPMethod)
	if httpMethod == "" {
		httpMethod = AuditHTTPMethodSystem
	}
	return ContextWithAudit(ctx, AuditContext{
		ActorSubject:        strings.TrimSpace(options.ActorSubject),
		ActorIssuer:         strings.TrimSpace(options.ActorIssuer),
		ClientID:            strings.TrimSpace(options.ClientID),
		AuthorizationResult: authorization,
		PolicyID:            strings.TrimSpace(options.PolicyID),
		MatchedRuleID:       strings.TrimSpace(options.MatchedRuleID),
		RequestID:           requestID,
		CorrelationID:       correlationID,
		SourceIP:            strings.TrimSpace(options.SourceIP),
		UserAgent:           strings.TrimSpace(options.UserAgent),
		Operation:           strings.TrimSpace(options.Operation),
		Endpoint:            strings.TrimSpace(options.Endpoint),
		HTTPMethod:          httpMethod,
	})
}

// ContextWithAuditOperation overrides operation and endpoint in an existing audit context.
//
// This keeps the original actor, request ID, correlation ID, and authorization
// metadata while attributing an internal side effect, such as registry
// synchronization, to the internal operation that actually wrote the history row.
//
// Parameters:
//   - ctx: Base context containing optional audit metadata.
//   - operation: Replacement operation name.
//   - endpoint: Replacement logical endpoint or internal source.
//
// Returns:
//   - context.Context: Context containing the updated audit metadata.
func ContextWithAuditOperation(ctx context.Context, operation string, endpoint string) context.Context {
	audit := FromContext(ctx)
	if strings.TrimSpace(operation) != "" {
		audit.Operation = strings.TrimSpace(operation)
	}
	if strings.TrimSpace(endpoint) != "" {
		audit.Endpoint = strings.TrimSpace(endpoint)
	}
	return ContextWithAudit(ctx, audit)
}

// NewAuditID returns a short random identifier suitable for audit correlation fields.
//
// The value is not a security token. It is generated from cryptographic random
// bytes when available, with a time-based fallback so audit attribution can still
// proceed if the host random source fails.
//
// Parameters:
//   - prefix: Optional stable prefix, for example "aas-preconfiguration".
//
// Returns:
//   - string: Prefix plus a random hexadecimal suffix.
func NewAuditID(prefix string) string {
	cleanPrefix := cleanAuditIDPrefix(prefix)
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Sprintf("%s-%d", cleanPrefix, time.Now().UTC().UnixNano())
	}
	return cleanPrefix + "-" + hex.EncodeToString(randomBytes)
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

func cleanAuditIDPrefix(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return "audit"
	}
	cleanedChars := make([]rune, 0, len(trimmed))
	lastWasSeparator := false
	for _, char := range strings.ToLower(trimmed) {
		if isAuditIDPrefixChar(char) {
			cleanedChars = append(cleanedChars, char)
			lastWasSeparator = false
			continue
		}
		if !lastWasSeparator {
			cleanedChars = append(cleanedChars, '-')
			lastWasSeparator = true
		}
	}
	cleaned := strings.Trim(string(cleanedChars), "-")
	if cleaned == "" {
		return "audit"
	}
	return cleaned
}

func isAuditIDPrefixChar(char rune) bool {
	return char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_'
}
