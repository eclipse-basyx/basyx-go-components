// nolint:all
package common

import (
	"context"
	"net/http"
)

// configKey is an unexported type used as the context key.
type configKey struct{}

// ConfigMiddleware injects the process-wide *Config into each request context.
// This lets downstream handlers fetch configuration without adding parameters.
func ConfigMiddleware(cfg *Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), configKey{}, cfg)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ConfigFromContext retrieves the *Config stored in context. The boolean
// indicates whether a config was present.
func ConfigFromContext(ctx context.Context) (*Config, bool) {
	cfg, ok := ctx.Value(configKey{}).(*Config)
	return cfg, ok
}

// ContextWithConfig returns a context containing the process-wide *Config.
func ContextWithConfig(ctx context.Context, cfg *Config) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, configKey{}, cfg)
}
