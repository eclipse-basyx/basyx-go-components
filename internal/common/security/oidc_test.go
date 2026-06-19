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
*******************************************************************************/

package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestOIDCVerifierConfig_UsesClientIDWhenAudienceProvided(t *testing.T) {
	t.Parallel()

	cfg := oidcVerifierConfig("discovery-service")
	if cfg == nil {
		t.Fatalf("expected verifier config, got nil")
		return
	}
	if cfg.SkipClientIDCheck {
		t.Fatalf("expected SkipClientIDCheck=false when audience is provided")
	}
	if cfg.ClientID != "discovery-service" {
		t.Fatalf("expected ClientID=discovery-service, got %q", cfg.ClientID)
	}
}

func TestOIDCVerifierConfig_SkipsClientIDCheckWhenAudienceMissing(t *testing.T) {
	t.Parallel()

	cfg := oidcVerifierConfig("   ")
	if cfg == nil {
		t.Fatalf("expected verifier config, got nil")
		return
	}
	if !cfg.SkipClientIDCheck {
		t.Fatalf("expected SkipClientIDCheck=true when audience is missing")
	}
	if cfg.ClientID != "" {
		t.Fatalf("expected empty ClientID when audience is missing, got %q", cfg.ClientID)
	}
}

func TestExtractIssuer_EntraV2AccessToken(t *testing.T) {
	t.Parallel()

	const want = "https://login.microsoftonline.com/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee/v2.0"

	got, err := extractIssuer(unsignedTokenWithClaims(t, Claims{"iss": want}))
	if err != nil {
		t.Fatalf("extractIssuer() error = %v", err)
	}
	if got != want {
		t.Fatalf("extractIssuer() = %q, want %q", got, want)
	}
}

func TestHasAllScopes_AcceptsEntraDelegatedPermissionClaim(t *testing.T) {
	t.Parallel()

	claims := Claims{"scp": "access_as_user profile"}
	if !hasAllScopes(claims, []string{"access_as_user"}) {
		t.Fatalf("expected Entra scp claim to satisfy required scope")
	}
}

func TestValidateCompactSignedJWT_RejectsOpaqueAndEncryptedTokens(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"opaque-token",
		"one.two",
		"one.two.three.four.five",
		".payload.signature",
	} {
		if err := validateCompactSignedJWT(raw); err == nil {
			t.Fatalf("validateCompactSignedJWT(%q) expected error", raw)
		}
	}
}

func TestOIDCMiddleware_RejectsUnknownIssuer(t *testing.T) {
	t.Parallel()

	middleware := (&OIDC{verifiers: map[string]issuerVerifier{}}).Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatalf("next handler must not be called")
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+unsignedTokenWithClaims(t, Claims{"iss": "https://unknown.example"}))
	response := httptest.NewRecorder()

	middleware.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestOIDCMiddleware_AnonymousClaimsDoNotContainSubject(t *testing.T) {
	t.Parallel()

	middleware := (&OIDC{
		verifiers: map[string]issuerVerifier{},
		settings: OIDCSettings{
			AllowAnonymous: true,
		},
	}).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := FromContext(r)
		if claims == nil {
			t.Fatal("expected anonymous claims in context")
		}
		if _, ok := claims["sub"]; ok {
			t.Fatal("anonymous claims must not contain sub")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	middleware.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestOIDCMiddleware_AppliesClaimMappingsAndTokenTypeIndicators(t *testing.T) {
	t.Parallel()

	privateKey, issuer := newTestOIDCIssuer(t)
	verifier := newTestAccessTokenVerifier(t, issuer, "basyx-api")

	mappings, err := normalizeClaimMappings([]OIDCClaimMappingSettings{
		{Target: "roles", Mode: "list", Sources: []string{"/roles", "/realm_access/roles"}},
		{Target: "token_type", Mode: "scalar", Sources: []string{"/idtyp", "/token_use"}},
	})
	if err != nil {
		t.Fatalf("normalizeClaimMappings() error = %v", err)
	}

	oidcMiddleware := (&OIDC{
		verifiers: map[string]issuerVerifier{
			issuer: issuerVerifier{
				issuer:        issuer,
				verifier:      verifier,
				scopes:        []string{"access_as_user"},
				scopeClaims:   defaultScopeClaimPointers,
				claimMappings: mappings,
			},
		},
	}).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := FromContext(r)
		if claims == nil {
			t.Fatalf("expected claims in context")
		}

		if got := claims["basyx.token_type"]; got != "app" {
			t.Fatalf("basyx.token_type = %#v, want app", got)
		}

		wantRoles := []string{"viewer", "admin", "editor"}
		if got := claims["basyx.roles"]; !reflect.DeepEqual(got, wantRoles) {
			t.Fatalf("basyx.roles = %#v, want %#v", got, wantRoles)
		}

		wantScopes := []string{"access_as_user", "profile"}
		if got := claims["basyx.scopes"]; !reflect.DeepEqual(got, wantScopes) {
			t.Fatalf("basyx.scopes = %#v, want %#v", got, wantScopes)
		}

		w.WriteHeader(http.StatusNoContent)
	}))

	token := signTestAccessToken(t, privateKey, issuer, Claims{
		"aud":          "basyx-api",
		"scp":          "access_as_user profile",
		"idtyp":        "app",
		"roles":        []any{"viewer", "admin"},
		"realm_access": map[string]any{"roles": []any{"admin", "editor"}},
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	oidcMiddleware.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func unsignedTokenWithClaims(t *testing.T, claims Claims) string {
	t.Helper()

	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}
