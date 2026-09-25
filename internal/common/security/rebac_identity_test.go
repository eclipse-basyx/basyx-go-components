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
	"encoding/json"
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
)

func TestReBACIdentityFromContextBuildsIssuerScopedIdentity(t *testing.T) {
	claims := Claims{
		"iss":    "https://issuer.example",
		"sub":    "alice",
		"exp":    json.Number("4102444800"),
		"groups": []any{"operators", "operators", "readers"},
	}
	cfg := common.ReBACConfig{
		Administrators: []common.ReBACAdministratorConfig{
			{Type: "group", Issuer: "https://issuer.example", Subject: "operators"},
			{Type: "user", Issuer: "https://other-issuer.example", Subject: "alice"},
		},
		GroupClaimMappings: []common.ReBACGroupClaimMappingConfig{
			{Issuer: "https://issuer.example", Claim: "groups"},
			{Issuer: "https://other-issuer.example", Claim: "other_groups"},
		},
	}

	identity, err := ReBACIdentityFromContext(validatedReBACContext(t, claims), cfg)
	if err != nil {
		t.Fatalf("ReBACIdentityFromContext() error = %v", err)
	}
	if identity.Principal == nil || *identity.Principal != (rebac.Principal{Kind: "user", Issuer: "https://issuer.example", ID: "alice"}) {
		t.Fatalf("principal = %#v", identity.Principal)
	}
	if identity.Issuer != "https://issuer.example" || identity.Subject != "alice" || !identity.Administrator {
		t.Fatalf("identity = %+v", identity)
	}
	if len(identity.Groups) != 2 || identity.Groups[0] != "operators" || identity.Groups[1] != "readers" {
		t.Fatalf("groups = %#v", identity.Groups)
	}
	if len(identity.ContextualTuples) != 2 {
		t.Fatalf("contextual tuples = %#v", identity.ContextualTuples)
	}
	if identity.ExpiresAt.Unix() != 4102444800 {
		t.Fatalf("expires at = %v", identity.ExpiresAt)
	}
}

func TestReBACIdentityFromContextSupportsAnonymousIdentity(t *testing.T) {
	ctx := context.WithValue(t.Context(), ClaimsKey, Claims{"iss": "untrusted", "sub": "untrusted"})
	identity, err := ReBACIdentityFromContext(ctx, common.ReBACConfig{})
	if err != nil {
		t.Fatalf("ReBACIdentityFromContext() error = %v", err)
	}
	if identity.Principal != nil || identity.Administrator || !identity.ExpiresAt.IsZero() || len(identity.ContextualTuples) != 0 {
		t.Fatalf("anonymous identity = %+v", identity)
	}
}

func TestReBACIdentityFromContextRejectsInvalidValidatedClaims(t *testing.T) {
	cases := []struct {
		name   string
		claims Claims
		want   string
	}{
		{
			name:   "expired token",
			claims: Claims{"iss": "https://issuer.example", "sub": "alice", "exp": json.Number("1")},
			want:   "SECURITY-REBACIDENTITY-EXPIRED",
		},
		{
			name:   "missing expiration",
			claims: Claims{"iss": "https://issuer.example", "sub": "alice"},
			want:   "SECURITY-REBACIDENTITY-EXPIRY",
		},
		{
			name:   "wrong group claim shape",
			claims: Claims{"iss": "https://issuer.example", "sub": "alice", "exp": json.Number("4102444800"), "groups": "operators"},
			want:   "SECURITY-REBACIDENTITY-GROUPS",
		},
	}
	cfg := common.ReBACConfig{GroupClaimMappings: []common.ReBACGroupClaimMappingConfig{{Issuer: "https://issuer.example", Claim: "groups"}}}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ReBACIdentityFromContext(validatedReBACContext(t, testCase.claims), cfg)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("ReBACIdentityFromContext() error = %v, want %s", err, testCase.want)
			}
		})
	}
}

func TestReBACIdentityFromContextDoesNotUseOtherIssuerGroupClaims(t *testing.T) {
	claims := Claims{
		"iss":    "https://issuer.example",
		"sub":    "alice",
		"exp":    json.Number("4102444800"),
		"groups": []any{"administrators"},
	}
	cfg := common.ReBACConfig{
		Administrators:     []common.ReBACAdministratorConfig{{Type: "group", Issuer: "https://issuer.example", Subject: "administrators"}},
		GroupClaimMappings: []common.ReBACGroupClaimMappingConfig{{Issuer: "https://other-issuer.example", Claim: "groups"}},
	}

	identity, err := ReBACIdentityFromContext(validatedReBACContext(t, claims), cfg)
	if err != nil {
		t.Fatalf("ReBACIdentityFromContext() error = %v", err)
	}
	if identity.Administrator || len(identity.Groups) != 0 || len(identity.ContextualTuples) != 0 {
		t.Fatalf("identity unexpectedly used groups from another issuer: %+v", identity)
	}
}

func validatedReBACContext(t *testing.T, claims Claims) context.Context {
	t.Helper()
	ctx := context.WithValue(t.Context(), ClaimsKey, claims)
	return context.WithValue(ctx, authenticatedKey, true)
}
