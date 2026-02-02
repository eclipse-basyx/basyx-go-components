package digitaltwinregistry

import (
	"context"
	"net/http"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type createdAfterKey struct{}

type createdAfterValue struct {
	value *time.Time
	err   error
}

// CreatedAfterMiddleware parses ?createdAfter=... (RFC3339) and stores it in the request context.
// If parsing fails, the error is stored in context and can be handled by the service.
func CreatedAfterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get("createdAfter")
		ctx := r.Context()
		if raw == "" {
			ctx = context.WithValue(ctx, createdAfterKey{}, createdAfterValue{})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			bad := common.NewErrBadRequest("invalid createdAfter (expected RFC3339)")
			resp := common.NewErrorResponse(bad, http.StatusBadRequest, "DTR", "CreatedAfterMiddleware", "createdAfter")
			_ = model.EncodeJSONResponse(resp.Body, &resp.Code, w)
			return
		}
		val := createdAfterValue{value: &parsed}
		ctx = context.WithValue(ctx, createdAfterKey{}, val)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CreatedAfterFromContext returns the parsed createdAfter value if present.
// If the param was invalid, err will be non-nil.
func CreatedAfterFromContext(ctx context.Context) (*time.Time, error) {
	val, ok := ctx.Value(createdAfterKey{}).(createdAfterValue)
	if !ok {
		return nil, nil
	}
	return val.value, val.err
}
