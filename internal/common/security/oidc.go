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
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi"
)

// OIDC wraps a token verifier and related settings.
type OIDC struct {
	verifiers map[string]issuerVerifier
	settings  OIDCSettings
}

type issuerVerifier struct {
	issuer        string
	verifier      *accessTokenVerifier
	scopes        []string
	scopeClaims   []string
	claimMappings []claimMapping
}

// OIDCSettings configures OIDC token verification.
//
// Providers: issuers with optional audience checks (and scopes) allowed by this service.
// AllowAnonymous: if true, requests without a Bearer token are treated as
//
//	anonymous instead of being rejected.
type OIDCSettings struct {
	Providers      []OIDCProviderSettings
	AllowAnonymous bool
}

// OIDCProviderSettings configures a single issuer and scopes, with optional
// audience verification.
type OIDCProviderSettings struct {
	Issuer        string
	Audience      string
	Scopes        []string
	DiscoveryURL  string
	ScopeClaims   []string
	ClaimMappings []OIDCClaimMappingSettings
}

// OIDCClaimMappingSettings maps provider claims into the reserved basyx.* namespace.
type OIDCClaimMappingSettings struct {
	Target  string
	Mode    string
	Sources []string
}

// NewOIDC initializes an OIDC verifier from the given settings.
func NewOIDC(ctx context.Context, s OIDCSettings) (*OIDC, error) {
	log.Printf("🔐 Initializing OIDC verifier…")

	verifiers := make(map[string]issuerVerifier, len(s.Providers))
	for _, p := range s.Providers {
		issuer := strings.TrimSpace(p.Issuer)
		audience := strings.TrimSpace(p.Audience)
		if issuer == "" {
			return nil, fmt.Errorf("COMMON-OIDC-VALIDATEISSUER issuer must not be empty")
		}
		if _, ok := verifiers[issuer]; ok {
			return nil, fmt.Errorf("COMMON-OIDC-VALIDATEISSUER duplicate issuer configured: %s", issuer)
		}

		scopeClaims, err := normalizeScopeClaimPointers(p.ScopeClaims)
		if err != nil {
			return nil, err
		}
		claimMappings, err := normalizeClaimMappings(p.ClaimMappings)
		if err != nil {
			return nil, err
		}

		provider, err := newOIDCProvider(ctx, issuer, p.DiscoveryURL)
		if err != nil {
			return nil, err
		}

		verifierCfg := oidcVerifierConfig(audience)
		v := provider.VerifierContext(oidcHTTPContext(ctx), verifierCfg)
		if v == nil {
			return nil, fmt.Errorf("COMMON-OIDC-CREATEVERIFIER failed to construct OIDC verifier")
		}

		verifiers[issuer] = issuerVerifier{
			issuer:        issuer,
			verifier:      &accessTokenVerifier{verifier: v},
			scopes:        p.Scopes,
			scopeClaims:   scopeClaims,
			claimMappings: claimMappings,
		}
		if verifierCfg.SkipClientIDCheck {
			log.Printf("⚠️ OIDC verifier created without audience validation. Issuer=%s", issuer)
		} else {
			log.Printf("✅ OIDC verifier created. Issuer=%s Audience=%s", issuer, verifierCfg.ClientID)
		}
	}

	return &OIDC{verifiers: verifiers, settings: s}, nil
}

func oidcVerifierConfig(audience string) *oidc.Config {
	if strings.TrimSpace(audience) == "" {
		return &oidc.Config{SkipClientIDCheck: true}
	}
	return &oidc.Config{ClientID: strings.TrimSpace(audience)}
}

type ctxKey string

const (
	// ClaimsKey is the context key used to store JWT claims.
	ClaimsKey ctxKey = "jwtClaims"
)

// FromContext retrieves Claims previously stored by the middleware.
func FromContext(r *http.Request) Claims {
	if v := r.Context().Value(ClaimsKey); v != nil {
		if c, ok := v.(Claims); ok {
			return c
		}
	}
	return nil
}

// ClaimsFromContext retrieves Claims from a context.Context.
func ClaimsFromContext(ctx context.Context) Claims {
	if v := ctx.Value(ClaimsKey); v != nil {
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
				ctx := context.WithValue(r.Context(), ClaimsKey, anon)
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
			log.Printf("❌ unknown token issuer")
			respondOIDCError(w)
			return
		}

		c, err := verifier.verifier.Verify(r.Context(), raw)
		if err != nil {
			log.Printf("❌ Token verification failed: %v", err)
			respondOIDCError(w)
			return
		}

		if err := normalizeVerifiedClaims(c, verifier.scopeClaims, verifier.claimMappings); err != nil {
			log.Printf("❌ Failed to normalize token claims: %v", err)
			respondOIDCError(w)
			return
		}

		if !hasAllScopes(c, verifier.scopes) {
			log.Printf("❌ missing required scopes: %v", verifier.scopes)
			respondOIDCStatus(w, http.StatusForbidden)
			return
		}

		// add time claims sourced from the current request context
		currTime := time.Now()
		c["CLIENTNOW"] = currTime.Format(time.RFC3339)
		c["LOCALNOW"] = currTime.In(time.Local).Format(time.RFC3339)
		c["UTCNOW"] = currTime.UTC().Format(time.RFC3339)

		r = r.WithContext(context.WithValue(r.Context(), ClaimsKey, c))
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

// respondOIDCError writes a structured error response with the provided code
// and message using the common BaSyx error format.
func respondOIDCError(w http.ResponseWriter) {
	respondOIDCStatus(w, http.StatusUnauthorized)
}

func respondOIDCStatus(w http.ResponseWriter, status int) {
	resp := common.NewErrorResponse(errors.New("access denied"), status, "Middleware", "Rules", "Denied")
	err := openapi.EncodeJSONResponse(resp.Body, &resp.Code, w)
	if err != nil {
		log.Printf("❌ Failed to encode error response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func extractIssuer(rawToken string) (string, error) {
	if err := validateCompactSignedJWT(rawToken); err != nil {
		return "", err
	}
	parts := strings.Split(rawToken, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("COMMON-OIDC-DECODETOKENPAYLOAD decode token payload: %w", err)
	}
	var claims struct {
		Issuer string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("COMMON-OIDC-PARSETOKENCLAIMS parse token claims: %w", err)
	}
	if strings.TrimSpace(claims.Issuer) == "" {
		return "", fmt.Errorf("COMMON-OIDC-VALIDATEISSUER token missing issuer")
	}
	return claims.Issuer, nil
}

func validateCompactSignedJWT(rawToken string) error {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return fmt.Errorf("COMMON-OIDC-VALIDATETOKENFORMAT token must be a compact signed JWT")
	}
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("COMMON-OIDC-VALIDATETOKENFORMAT token must be a compact signed JWT")
		}
	}
	return nil
}
