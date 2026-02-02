package auth

import (
	"context"
	"net/http"
	"strings"
)

// EdcBpnHeaderMiddleware injects the Edc-Bpn header value into JWT claims
// when security is enabled. The claim key is "edc_bpn".
func EdcBpnHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bpn := strings.TrimSpace(r.Header.Get("Edc-Bpn"))
		if bpn == "" {
			next.ServeHTTP(w, r)
			return
		}

		claims := FromContext(r)
		if claims == nil {
			next.ServeHTTP(w, r)
			return
		}

		claims["Edc-Bpn"] = bpn
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
