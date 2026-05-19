package auth

import "context"

// WithClaims stores claims in the provided context.
func WithClaims(ctx context.Context, claims Claims) context.Context {
	if ctx == nil {
		ctx = context.TODO()
	}

	return context.WithValue(ctx, claimsKey, claims)
}
