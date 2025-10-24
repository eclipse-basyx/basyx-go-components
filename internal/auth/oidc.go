package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi"
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

const (
	claimsKey   ctxKey = "jwtClaims"
	issuedAtKey ctxKey = "tokenIssuedAt"
)

func FromContext(r *http.Request) Claims {
	if v := r.Context().Value(claimsKey); v != nil {
		if c, ok := v.(Claims); ok {
			return c
		}
	}
	return nil
}

func IssuedAtFromContext(r *http.Request) (time.Time, bool) {
	if v := r.Context().Value(issuedAtKey); v != nil {
		if t, ok := v.(time.Time); ok {
			return t, true
		}
	}
	return time.Time{}, false
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
			resp := common.NewErrorResponse(errors.New("invalid token"), http.StatusForbidden, "Middleware", "Rules", "Denied")
			openapi.EncodeJSONResponse(resp.Body, &resp.Code, w)
			return
		}
		var rm json.RawMessage
		if err := idToken.Claims(&rm); err != nil {
		}

		dec := json.NewDecoder(bytes.NewReader(rm))
		dec.UseNumber()

		var c Claims
		if err := dec.Decode(&c); err != nil {
			log.Printf("❌ Failed to parse claims: %v", err)

			resp := common.NewErrorResponse(errors.New("invalid claims"), http.StatusForbidden, "Middleware", "Rules", "Denied")
			openapi.EncodeJSONResponse(resp.Body, &resp.Code, w)
			return
		}

		var issuedAt time.Time
		if n, ok := c["iat"].(json.Number); ok {
			sec, _ := n.Int64()
			issuedAt = time.Unix(sec, 0)
			log.Printf("🕓 Token issued at %v", issuedAt)
		} else {
			log.Printf("⚠️ Token missing 'iat' claim")
		}

		if typ, _ := c.GetString("typ"); typ != "" && !strings.EqualFold(typ, "Bearer") {
			log.Printf("❌ unexpected token typ: %q", typ)

			resp := common.NewErrorResponse(errors.New("invalid token type"), http.StatusForbidden, "Middleware", "Rules", "Denied")
			openapi.EncodeJSONResponse(resp.Body, &resp.Code, w)
			return
		}

		required := []string{"profile"}
		if !hasAllScopes(c, required) {
			log.Printf("❌ missing required scopes: %v", required)

			resp := common.NewErrorResponse(errors.New("insufficient scope"), http.StatusForbidden, "Middleware", "Rules", "Denied")
			openapi.EncodeJSONResponse(resp.Body, &resp.Code, w)
			return
		}

		log.Printf("✅ Token verified successfully for subject: %v", c["sub"])
		ctx := context.WithValue(r.Context(), claimsKey, c)
		ctx = context.WithValue(ctx, issuedAtKey, issuedAt)
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
