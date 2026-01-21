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
// Author: Martin Stemmer ( Fraunhofer IESE )

package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi"
)

// OIDC wraps an ID token verifier and related settings.
type OIDC struct {
	verifiers map[string]issuerVerifier
	settings  OIDCSettings
}

type issuerVerifier struct {
	issuer   string
	verifier *oidc.IDTokenVerifier
	scopes   []string
}

// OIDCSettings configures OIDC token verification.
//
// Providers: issuer/audience pairs (and scopes) allowed by this service.
// AllowAnonymous: if true, requests without a Bearer token are treated as
//
//	anonymous instead of being rejected.
type OIDCSettings struct {
	Providers      []OIDCProviderSettings
	AllowAnonymous bool
}

// OIDCProviderSettings configures a single issuer/audience pair and scopes.
type OIDCProviderSettings struct {
	Issuer   string
	Audience string
	Scopes   []string
}

// NewOIDC initializes an OIDC verifier from the given settings.
func NewOIDC(ctx context.Context, s OIDCSettings) (*OIDC, error) {
	log.Printf("🔐 Initializing OIDC verifier…")

	if len(s.Providers) == 0 {
		return nil, fmt.Errorf("at least one OIDC provider must be configured")
	}

	verifiers := make(map[string]issuerVerifier, len(s.Providers))
	for _, p := range s.Providers {
		issuer := strings.TrimSpace(p.Issuer)
		audience := strings.TrimSpace(p.Audience)
		if issuer == "" {
			return nil, fmt.Errorf("issuer must not be empty")
		}
		if audience == "" {
			return nil, fmt.Errorf("audience must not be empty")
		}
		if _, ok := verifiers[issuer]; ok {
			return nil, fmt.Errorf("duplicate issuer configured: %s", issuer)
		}

		provider, err := oidc.NewProvider(ctx, issuer)
		if err != nil {
			return nil, fmt.Errorf("create OIDC provider: %w", err)
		}

		v := provider.Verifier(&oidc.Config{ClientID: audience})
		if v == nil {
			return nil, fmt.Errorf("failed to construct OIDC verifier")
		}

		verifiers[issuer] = issuerVerifier{
			issuer:   issuer,
			verifier: v,
			scopes:   p.Scopes,
		}
		log.Printf("✅ OIDC verifier created. Issuer=%s Audience=%s", issuer, audience)
	}

	return &OIDC{verifiers: verifiers, settings: s}, nil
}

type ctxKey string

const (
	claimsKey   ctxKey = "jwtClaims"
	issuedAtKey ctxKey = "tokenIssuedAt"
)

// FromContext retrieves Claims previously stored by the middleware.
func FromContext(r *http.Request) Claims {
	if v := r.Context().Value(claimsKey); v != nil {
		if c, ok := v.(Claims); ok {
			return c
		}
	}
	return nil
}

// Middleware validates a Bearer token (if present) and injects claims.
//
// Behavior:
//   - If Authorization header is missing or not Bearer:
//   - If AllowAnonymous is true → inject anonymous claims and continue.
//   - Otherwise → 401 Unauthorized.
//   - If Bearer is present → verify the token, parse claims, check scopes,
//     and store claims and iat in the request context.
func (o *OIDC) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			if o.settings.AllowAnonymous {
				anon := Claims{"sub": "anonymous", "scope": ""}
				ctx := context.WithValue(r.Context(), claimsKey, anon)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			respondOIDCError(w)
			return
		}

		raw := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
		if raw == "" {
			respondOIDCError(w)
			return
		}

		issuer, err := extractIssuer(raw)
		if err != nil {
			log.Printf("❌ Failed to read token issuer: %v", err)
			respondOIDCError(w)
			return
		}

		verifier, ok := o.verifiers[issuer]
		if !ok {
			log.Printf("❌ unknown token issuer: %s", issuer)
			respondOIDCError(w)
			return
		}

		idToken, err := verifier.verifier.Verify(r.Context(), raw)
		if err != nil {
			log.Printf("❌ Token verification failed: %v", err)
			respondOIDCError(w)
			return
		}

		var rm json.RawMessage
		if err := idToken.Claims(&rm); err != nil {
			log.Printf("❌ Failed to fetch raw claims: %v", err)
			respondOIDCError(w)
			return
		}

		dec := json.NewDecoder(bytes.NewReader(rm))
		dec.UseNumber()

		var c Claims
		if err := dec.Decode(&c); err != nil {
			log.Printf("❌ Failed to decode claims: %v", err)
			respondOIDCError(w)
			return
		}

		if typ, _ := c.GetString("typ"); typ != "" && !strings.EqualFold(typ, "Bearer") {
			log.Printf("❌ unexpected token typ: %q", typ)
			respondOIDCError(w)
			return
		}

		// Enforce minimal scopes (kept as-is; extend if needed).
		if !hasAllScopes(c, verifier.scopes) {
			log.Printf("❌ missing required scopes: %v", verifier.scopes)
			respondOIDCError(w)
			return
		}

		// add time claims sourced from the current request context
		currTime := time.Now()
		c["CLIENTNOW"] = currTime.Format(time.RFC3339)
		c["LOCALNOW"] = currTime.In(time.Local).Format(time.RFC3339)
		c["UTCNOW"] = currTime.UTC().Format(time.RFC3339)

		log.Printf("✅ Token verified successfully for subject: %v", c["sub"])
		r = r.WithContext(context.WithValue(r.Context(), claimsKey, c))
		next.ServeHTTP(w, r)
	})
}

// GetString returns a string claim value and a boolean indicating presence.
func (c Claims) GetString(key string) (string, bool) {
	v, ok := c[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// MarshalJSON allows Claims to be serialized as a JSON object.
func (c Claims) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any(c))
}

// hasAllScopes reports whether all required scopes are present in the
// space-delimited "scope" claim.
func hasAllScopes(c Claims, need []string) bool {
	s, _ := c.GetString("scope")
	have := map[string]struct{}{}
	for _, sc := range strings.Fields(s) {
		have[sc] = struct{}{}
	}
	for _, n := range need {
		if _, ok := have[n]; !ok {
			return false
		}
	}
	return true
}

// respondOIDCError writes a structured error response with the provided code
// and message using the common BaSyx error format.
func respondOIDCError(w http.ResponseWriter) {
	resp := common.NewErrorResponse(errors.New("access denied"), http.StatusUnauthorized, "Middleware", "Rules", "Denied")
	err := openapi.EncodeJSONResponse(resp.Body, &resp.Code, w)
	if err != nil {
		log.Printf("❌ Failed to encode error response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func extractIssuer(rawToken string) (string, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("token has invalid format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode token payload: %w", err)
	}
	var claims struct {
		Issuer string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse token claims: %w", err)
	}
	if strings.TrimSpace(claims.Issuer) == "" {
		return "", fmt.Errorf("token missing issuer")
	}
	return claims.Issuer, nil
}
