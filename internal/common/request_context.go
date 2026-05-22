package common

import "context"

type authorizationHeaderContextKey struct{}
type acceptHeaderContextKey struct{}

// WithAuthorizationHeader stores the inbound Authorization header in context.
func WithAuthorizationHeader(ctx context.Context, authorizationHeader string) context.Context {
	if ctx == nil {
		panic("common.WithAuthorizationHeader: ctx must not be nil")
	}

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

// WithAcceptHeader stores the inbound Accept header in context.
func WithAcceptHeader(ctx context.Context, acceptHeader string) context.Context {
	if ctx == nil {
		panic("common.WithAcceptHeader: ctx must not be nil")
	}

	return context.WithValue(ctx, acceptHeaderContextKey{}, acceptHeader)
}

// AcceptHeaderFromContext returns the previously stored Accept header.
func AcceptHeaderFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	value, ok := ctx.Value(acceptHeaderContextKey{}).(string)
	if !ok {
		return ""
	}

	return value
}
