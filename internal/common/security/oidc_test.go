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
	"testing"
)

func TestOIDCVerifierConfig_UsesClientIDWhenAudienceProvided(t *testing.T) {
	t.Parallel()

	cfg := oidcVerifierConfig("discovery-service")
	if cfg == nil {
		t.Fatalf("expected verifier config, got nil")
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

func unsignedTokenWithClaims(t *testing.T, claims Claims) string {
	t.Helper()

	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}
