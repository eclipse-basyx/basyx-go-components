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
	"net/http"
	"os"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// AuditContextMiddleware enriches request contexts with history audit metadata.
//
// The middleware copies the request, OIDC, ABAC, and route metadata selected by
// history.auditIdentityMode from the current request. It does not generate
// identities, and it does not store Authorization headers or bearer tokens.
//
// Parameters:
//   - cfg: Process configuration that controls history.auditIdentityMode.
//
// Returns:
//   - func(http.Handler) http.Handler: Middleware that stores AuditContext for
//     AppendVersionTx and WORM history_event artifacts.
func AuditContextMiddleware(cfg *common.Config) func(http.Handler) http.Handler {
	mode := auditIdentityModeFromConfig(cfg)
	policyID := auditPolicyID(cfg, mode)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if mode == AuditIdentityNone {
				next.ServeHTTP(w, r)
				return
			}
			audit := buildRequestAuditContext(r, mode, policyID)
			next.ServeHTTP(w, r.WithContext(ContextWithAudit(r.Context(), audit)))
		})
	}
}

func auditIdentityModeFromConfig(cfg *common.Config) string {
	if cfg == nil {
		return AuditIdentityNone
	}
	switch strings.ToLower(strings.TrimSpace(cfg.History.AuditIdentityMode)) {
	case AuditIdentityMinimal:
		return AuditIdentityMinimal
	case AuditIdentityExtended:
		return AuditIdentityExtended
	default:
		return AuditIdentityNone
	}
}

func auditPolicyID(cfg *common.Config, mode string) string {
	if mode != AuditIdentityExtended || cfg == nil || !cfg.ABAC.Enabled || strings.TrimSpace(cfg.ABAC.ModelPath) == "" {
		return ""
	}
	data, err := os.ReadFile(cfg.ABAC.ModelPath)
	if err != nil {
		return ""
	}
	return SHA256Hex(data)
}

func buildRequestAuditContext(r *http.Request, mode string, policyID string) AuditContext {
	audit := AuditContext{
		RequestID:     firstHeaderValue(r, "X-Request-ID", "X-Request-Id", "Request-ID"),
		CorrelationID: firstHeaderValue(r, "X-Correlation-ID", "X-Correlation-Id", "Correlation-ID"),
		HTTPMethod:    strings.ToUpper(strings.TrimSpace(r.Method)),
	}
	populateAuditIdentity(r, &audit)
	populateAuditAuthorization(r, &audit, mode, policyID)
	populateAuditRoute(r, &audit)
	if mode == AuditIdentityExtended {
		audit.SourceIP = common.RequestSourceIP(r)
		audit.UserAgent = r.UserAgent()
	}
	return audit
}

func populateAuditIdentity(r *http.Request, audit *AuditContext) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		return
	}
	subject, _ := claims.GetString("sub")
	if strings.TrimSpace(subject) == "" || strings.EqualFold(subject, "anonymous") {
		return
	}
	audit.ActorSubject = subject
	audit.ActorIssuer, _ = claims.GetString("iss")
	if clientID, ok := claims.GetString("client_id"); ok {
		audit.ClientID = clientID
		return
	}
	if authorizedParty, ok := claims.GetString("azp"); ok {
		audit.ClientID = authorizedParty
	}
}

func populateAuditAuthorization(r *http.Request, audit *AuditContext, mode string, policyID string) {
	decision, ok := auth.AuthorizationDecisionFromContext(r.Context())
	if !ok {
		return
	}
	audit.AuthorizationResult = decision.Result
	if mode == AuditIdentityExtended {
		audit.PolicyID = policyID
		audit.MatchedRuleID = decision.MatchedRuleID
	}
}

func populateAuditRoute(r *http.Request, audit *AuditContext) {
	coverage, ok := MutationCoverageFromContext(r.Context())
	if ok {
		audit.Endpoint = coverage.Pattern
		audit.Operation = coverage.Operation
		if audit.Operation == "" && coverage.Pattern != "" {
			audit.Operation = coverage.Method + " " + coverage.Pattern
		}
		return
	}
	audit.Endpoint = r.URL.Path
	audit.Operation = audit.HTTPMethod + " " + r.URL.Path
}

func firstHeaderValue(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(r.Header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}
