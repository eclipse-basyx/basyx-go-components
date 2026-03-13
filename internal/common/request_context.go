package common

import "context"

type authorizationHeaderContextKey struct{}

// WithAuthorizationHeader stores the inbound Authorization header in context.
func WithAuthorizationHeader(ctx context.Context, authorizationHeader string) context.Context {
	return context.WithValue(ctx, authorizationHeaderContextKey{}, authorizationHeader)
}

// AuthorizationHeaderFromContext returns the previously stored Authorization header.
func AuthorizationHeaderFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	value, ok := ctx.Value(authorizationHeaderContextKey{}).(string)
	if !ok {
		return ""
	}

	return value
}
