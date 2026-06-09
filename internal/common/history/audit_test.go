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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestAuditContextRoundTrip(t *testing.T) {
	t.Parallel()

	expected := AuditContext{
		ActorSubject:  "subject",
		ActorIssuer:   "issuer",
		ClientID:      "client",
		RequestID:     "request",
		CorrelationID: "correlation",
		Endpoint:      "/shells",
		HTTPMethod:    "POST",
	}

	actual := FromContext(ContextWithAudit(context.Background(), expected))

	require.Equal(t, expected, actual)
}

func TestContextWithSystemAuditGeneratesTraceIDs(t *testing.T) {
	t.Parallel()

	ctx := ContextWithSystemAudit(context.Background(), SystemAuditOptions{
		ActorSubject: "system:aas-preconfiguration",
		ActorIssuer:  "basyx:aasenvironmentservice",
		ClientID:     "aasenvironmentservice",
		Operation:    "AASPreconfiguration",
		Endpoint:     "startup:aas-preconfiguration",
		IDPrefix:     "aas-preconfiguration",
	})

	audit := FromContext(ctx)

	require.Equal(t, "system:aas-preconfiguration", audit.ActorSubject)
	require.Equal(t, "basyx:aasenvironmentservice", audit.ActorIssuer)
	require.Equal(t, "aasenvironmentservice", audit.ClientID)
	require.Equal(t, AuthorizationResultSystemInternal, audit.AuthorizationResult)
	require.Equal(t, "AASPreconfiguration", audit.Operation)
	require.Equal(t, "startup:aas-preconfiguration", audit.Endpoint)
	require.Equal(t, AuditHTTPMethodSystem, audit.HTTPMethod)
	require.NotEmpty(t, audit.RequestID)
	require.Equal(t, audit.RequestID, audit.CorrelationID)
}

func TestContextWithAuditOperationPreservesRequestActorAndAuthorization(t *testing.T) {
	t.Parallel()

	ctx := ContextWithAudit(context.Background(), AuditContext{
		RequestID:           "request-1",
		CorrelationID:       "correlation-1",
		ActorSubject:        "user-1",
		ActorIssuer:         "issuer-1",
		ClientID:            "client-1",
		AuthorizationResult: "ALLOW",
		HTTPMethod:          http.MethodPut,
		Operation:           "PutAssetAdministrationShellById",
		Endpoint:            "/shells/{aasIdentifier}",
	})

	audit := FromContext(ContextWithAuditOperation(ctx, "AASRegistrySync.UpsertDescriptor", "internal:aas-registry-sync"))

	require.Equal(t, "request-1", audit.RequestID)
	require.Equal(t, "correlation-1", audit.CorrelationID)
	require.Equal(t, "user-1", audit.ActorSubject)
	require.Equal(t, "issuer-1", audit.ActorIssuer)
	require.Equal(t, "client-1", audit.ClientID)
	require.Equal(t, "ALLOW", audit.AuthorizationResult)
	require.Equal(t, http.MethodPut, audit.HTTPMethod)
	require.Equal(t, "AASRegistrySync.UpsertDescriptor", audit.Operation)
	require.Equal(t, "internal:aas-registry-sync", audit.Endpoint)
}

func TestAuditContextMiddlewarePopulatesAuthenticatedMinimalFields(t *testing.T) {
	cfg := &common.Config{History: common.HistoryConfig{AuditIdentityMode: AuditIdentityMinimal}, ABAC: common.ABACConfig{Enabled: true}}
	var captured AuditContext
	handler := AuditContextMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
	}))
	request := httptest.NewRequest(http.MethodPut, "/shells/aas-1", nil)
	request.Header.Set("X-Request-ID", "request-1")
	request.Header.Set("X-Correlation-ID", "correlation-1")
	ctx := context.WithValue(request.Context(), auth.ClaimsKey, auth.Claims{
		"sub":       "user-1",
		"iss":       "https://issuer.example",
		"client_id": "client-1",
	})
	ctx = auth.ContextWithAuthorizationDecision(ctx, auth.AuthorizationDecision{
		Result:        string(auth.DecisionAllow),
		MatchedRuleID: "rule:1:abcdef0123456789",
	})
	ctx = contextWithMutationCoverage(ctx, http.MethodPut, mutationRoute{pattern: "/shells/{aasIdentifier}", operation: "PutAssetAdministrationShellById"}, true)

	handler.ServeHTTP(httptest.NewRecorder(), request.WithContext(ctx))

	require.Equal(t, "request-1", captured.RequestID)
	require.Equal(t, "correlation-1", captured.CorrelationID)
	require.Equal(t, "user-1", captured.ActorSubject)
	require.Equal(t, "https://issuer.example", captured.ActorIssuer)
	require.Equal(t, "client-1", captured.ClientID)
	require.Equal(t, string(auth.DecisionAllow), captured.AuthorizationResult)
	require.Equal(t, "PutAssetAdministrationShellById", captured.Operation)
	require.Equal(t, "/shells/{aasIdentifier}", captured.Endpoint)
	require.Empty(t, captured.MatchedRuleID)
	require.Empty(t, captured.SourceIP)
	require.Empty(t, captured.UserAgent)
}

func TestAuditContextMiddlewarePopulatesExtendedAuthorizationFields(t *testing.T) {
	policyPath := filepath.Join(t.TempDir(), "access-rules.json")
	require.NoError(t, os.WriteFile(policyPath, []byte(`{"AllAccessPermissionRules":{"rules":[]}}`), 0600))

	cfg := &common.Config{
		History: common.HistoryConfig{AuditIdentityMode: AuditIdentityExtended},
		ABAC: common.ABACConfig{
			Enabled:   true,
			ModelPath: policyPath,
		},
	}
	var captured AuditContext
	handler := AuditContextMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
	}))
	request := httptest.NewRequest(http.MethodPut, "/shells/aas-1", nil)
	ctx := context.WithValue(request.Context(), auth.ClaimsKey, auth.Claims{
		"sub":       "user-1",
		"iss":       "https://issuer.example",
		"client_id": "client-1",
	})
	ctx = auth.ContextWithAuthorizationDecision(ctx, auth.AuthorizationDecision{
		Result:        string(auth.DecisionAllow),
		MatchedRuleID: "rule:1:abcdef0123456789,rule:3:0123456789abcdef",
	})

	handler.ServeHTTP(httptest.NewRecorder(), request.WithContext(ctx))

	require.Equal(t, string(auth.DecisionAllow), captured.AuthorizationResult)
	require.NotEmpty(t, captured.PolicyID)
	require.Equal(t, "rule:1:abcdef0123456789,rule:3:0123456789abcdef", captured.MatchedRuleID)
}

func TestAuditContextMiddlewareLeavesMatchedRuleIDEmptyWhenUnavailable(t *testing.T) {
	cfg := &common.Config{History: common.HistoryConfig{AuditIdentityMode: AuditIdentityExtended}}
	var captured AuditContext
	handler := AuditContextMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
	}))
	request := httptest.NewRequest(http.MethodPost, "/shells", nil)

	handler.ServeHTTP(httptest.NewRecorder(), request)

	require.Empty(t, captured.AuthorizationResult)
	require.Empty(t, captured.MatchedRuleID)
}

func TestAuditContextMiddlewareNoneModeDoesNotStoreMatchedRuleID(t *testing.T) {
	cfg := &common.Config{History: common.HistoryConfig{AuditIdentityMode: AuditIdentityNone}}
	var captured AuditContext
	handler := AuditContextMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
	}))
	request := httptest.NewRequest(http.MethodPost, "/shells", nil)
	ctx := auth.ContextWithAuthorizationDecision(request.Context(), auth.AuthorizationDecision{
		Result:        string(auth.DecisionAllow),
		MatchedRuleID: "rule:1:abcdef0123456789",
	})

	handler.ServeHTTP(httptest.NewRecorder(), request.WithContext(ctx))

	require.Empty(t, captured.AuthorizationResult)
	require.Empty(t, captured.MatchedRuleID)
}

func TestAuditContextMiddlewareDoesNotInventAnonymousPrincipal(t *testing.T) {
	cfg := &common.Config{History: common.HistoryConfig{AuditIdentityMode: AuditIdentityMinimal}}
	var captured AuditContext
	handler := AuditContextMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
	}))
	request := httptest.NewRequest(http.MethodPost, "/shells", nil)
	ctx := context.WithValue(request.Context(), auth.ClaimsKey, auth.Claims{"sub": "anonymous"})

	handler.ServeHTTP(httptest.NewRecorder(), request.WithContext(ctx))

	require.Empty(t, captured.ActorSubject)
	require.Equal(t, http.MethodPost, captured.HTTPMethod)
}

func TestAuditContextMiddlewareUsesTrustedProxySourceIPInExtendedMode(t *testing.T) {
	cfg := &common.Config{
		History: common.HistoryConfig{AuditIdentityMode: AuditIdentityExtended},
		General: common.GeneralConfig{
			TrustProxyHeaders: true,
			TrustedProxyCIDRs: []string{"10.0.0.0/8"},
		},
	}
	var captured AuditContext
	handler := AuditContextMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
	}))
	request := httptest.NewRequest(http.MethodPatch, "/submodels/sm-1", nil)
	request.RemoteAddr = "10.1.2.3:12345"
	request.Header.Set("X-Forwarded-For", "203.0.113.8")
	request.Header.Set("User-Agent", "audit-test")
	request = request.WithContext(common.ContextWithConfig(request.Context(), cfg))

	handler.ServeHTTP(httptest.NewRecorder(), request)

	require.Equal(t, "203.0.113.8", captured.SourceIP)
	require.Equal(t, "audit-test", captured.UserAgent)
}

func TestHistoryEventArtifactContainsMatchingAuditMetadata(t *testing.T) {
	audit := AuditContext{
		RequestID:           "request-1",
		CorrelationID:       "correlation-1",
		ActorSubject:        "user-1",
		ActorIssuer:         "https://issuer.example",
		ClientID:            "client-1",
		AuthorizationResult: "ALLOW",
		PolicyID:            "policy-1",
		MatchedRuleID:       "rule-1",
		SourceIP:            "203.0.113.8",
		UserAgent:           "audit-test",
		Operation:           "PutAssetAdministrationShellById",
		Endpoint:            "/shells/{aasIdentifier}",
		HTTPMethod:          http.MethodPut,
	}
	snapshot := map[string]any{"id": "aas-1"}
	payloadJSON, err := CanonicalJSON(snapshot)
	require.NoError(t, err)
	contentHash, err := CanonicalJSONHash(snapshot)
	require.NoError(t, err)
	event := ChangeEvent{
		EntityType:          TableAAS,
		Identifier:          "aas-1",
		ChangeType:          ChangeUpdated,
		Timestamp:           time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC),
		PayloadType:         PayloadTypeSnapshot,
		ContentHash:         contentHash,
		PayloadHash:         contentHash,
		RequestID:           audit.RequestID,
		CorrelationID:       audit.CorrelationID,
		ActorSubject:        audit.ActorSubject,
		ActorIssuer:         audit.ActorIssuer,
		ClientID:            audit.ClientID,
		AuthorizationResult: audit.AuthorizationResult,
		PolicyID:            audit.PolicyID,
		MatchedRuleID:       audit.MatchedRuleID,
		SourceIP:            audit.SourceIP,
		UserAgent:           audit.UserAgent,
		Operation:           audit.Operation,
		Endpoint:            audit.Endpoint,
		HTTPMethod:          audit.HTTPMethod,
	}
	rowHash, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)
	event.RowHash = rowHash

	artifact, err := buildHistoryEventEvidenceArtifact(TableAAS, 1, event, historyPayload{payloadType: PayloadTypeSnapshot, json: payloadJSON, hash: contentHash}, nil, "", "")
	require.NoError(t, err)
	var decoded struct {
		Audit map[string]string `json:"audit"`
	}
	require.NoError(t, json.Unmarshal(artifact.Data, &decoded))

	require.Equal(t, audit.RequestID, decoded.Audit["request_id"])
	require.Equal(t, audit.ActorSubject, decoded.Audit["actor_subject"])
	require.Equal(t, audit.AuthorizationResult, decoded.Audit["authorization_result"])
	require.Equal(t, audit.MatchedRuleID, decoded.Audit["matched_rule_id"])
	require.Equal(t, audit.Endpoint, decoded.Audit["endpoint"])
}
