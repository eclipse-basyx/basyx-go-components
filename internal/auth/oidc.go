package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc"
)

type OIDC struct {
	verifier *oidc.IDTokenVerifier
}

type OIDCSettings struct {
	Issuer   string
	Audience string
}

func NewOIDC(ctx context.Context, s OIDCSettings) (*OIDC, error) {
	log.Printf("🔐 Initializing OIDC verifier...")
	provider, err := oidc.NewProvider(ctx, s.Issuer)
	if err != nil {
		return nil, err
	}
	v := provider.Verifier(&oidc.Config{
		ClientID: s.Audience,
	})
	log.Printf("✅ OIDC verifier created. Issuer=%s Audience=%s", s.Issuer, s.Audience)
	return &OIDC{verifier: v}, nil
}

type Claims map[string]any

type ctxKey string

const claimsKey ctxKey = "jwtClaims"

func FromContext(r *http.Request) Claims {
	if v := r.Context().Value(claimsKey); v != nil {
		if c, ok := v.(Claims); ok {
			return c
		}
	}
	return nil
}

func (o *OIDC) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			http.Error(w, "missing or invalid Authorization header", http.StatusUnauthorized)
			return
		}
		raw := strings.TrimPrefix(authz, "Bearer ")

		idToken, err := o.verifier.Verify(r.Context(), raw)
		if err != nil {
			log.Printf("❌ Token verification failed: %v", err)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		var rm json.RawMessage
		if err := idToken.Claims(&rm); err != nil { /* handle */
		}

		dec := json.NewDecoder(bytes.NewReader(rm))
		dec.UseNumber()

		var c Claims
		if err := dec.Decode(&c); err != nil {
			log.Printf("❌ Failed to parse claims: %v", err)
			http.Error(w, "invalid claims", http.StatusUnauthorized)
			return
		}

		// 1) Ensure it’s a Bearer access token (optional but good hygiene)
		if typ, _ := c.GetString("typ"); typ != "" && !strings.EqualFold(typ, "Bearer") {
			log.Printf("❌ unexpected token typ: %q", typ)
			http.Error(w, "invalid token type", http.StatusUnauthorized)
			return
		}

		// 3) Require scopes for endpoint (example)
		required := []string{"profile"} // tailor per route
		if !hasAllScopes(c, required) {
			log.Printf("❌ missing required scopes: %v", required)
			http.Error(w, "insufficient scope", http.StatusForbidden)
			return
		}

		log.Printf("✅ Token verified successfully for subject: %v", c["sub"])
		ctx := context.WithValue(r.Context(), claimsKey, c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (c Claims) GetString(key string) (string, bool) {
	v, ok := c[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func (c Claims) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any(c))
}

func hasAllScopes(c Claims, need []string) bool {
	s, _ := c.GetString("scope") // e.g. "profile email"
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
