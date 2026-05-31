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
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
)

const testOIDCKeyID = "basyx-test-key"

func TestAccessTokenVerifier_AcceptsLegacyMissingAudience(t *testing.T) {
	t.Parallel()

	privateKey, issuer := newTestOIDCIssuer(t)
	verifier := newTestAccessTokenVerifier(t, issuer, "")
	token := signTestAccessToken(t, privateKey, issuer, Claims{})

	if _, err := verifier.Verify(context.Background(), token); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestAccessTokenVerifier_EnforcesAudience(t *testing.T) {
	t.Parallel()

	privateKey, issuer := newTestOIDCIssuer(t)
	verifier := newTestAccessTokenVerifier(t, issuer, "basyx-api")

	if _, err := verifier.Verify(context.Background(), signTestAccessToken(t, privateKey, issuer, Claims{"aud": "other-api"})); err == nil {
		t.Fatalf("expected wrong audience error")
	}
	if _, err := verifier.Verify(context.Background(), signTestAccessToken(t, privateKey, issuer, Claims{"aud": "basyx-api"})); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestAccessTokenVerifier_RejectsExpiredToken(t *testing.T) {
	t.Parallel()

	privateKey, issuer := newTestOIDCIssuer(t)
	verifier := newTestAccessTokenVerifier(t, issuer, "")
	token := signTestAccessToken(t, privateKey, issuer, Claims{"exp": time.Now().Add(-time.Minute).Unix()})

	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatalf("expected expired token error")
	}
}

func newTestOIDCIssuer(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	handler := &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{{
			PublicKey: privateKey.Public(),
			KeyID:     testOIDCKeyID,
			Algorithm: oidc.RS256,
		}},
	}
	server := httptest.NewServer(handler)
	handler.SetIssuer(server.URL)
	t.Cleanup(server.Close)
	return privateKey, server.URL
}

func newTestAccessTokenVerifier(t *testing.T, issuer string, audience string) *accessTokenVerifier {
	t.Helper()

	provider, err := newOIDCProvider(context.Background(), issuer, "")
	if err != nil {
		t.Fatalf("newOIDCProvider() error = %v", err)
	}
	return &accessTokenVerifier{
		verifier: provider.VerifierContext(context.Background(), oidcVerifierConfig(audience)),
	}
}

func signTestAccessToken(t *testing.T, privateKey *rsa.PrivateKey, issuer string, overrides Claims) string {
	t.Helper()

	claims := Claims{
		"iss": issuer,
		"sub": "test-subject",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	for key, value := range overrides {
		claims[key] = value
	}
	rawClaims, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return oidctest.SignIDToken(privateKey, testOIDCKeyID, oidc.RS256, string(rawClaims))
}
