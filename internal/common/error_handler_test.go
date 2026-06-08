package common

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestErrorClassifiers_RecognizeWrappedErrors(t *testing.T) {
	testCases := []struct {
		name   string
		err    error
		assert func(error) bool
	}{
		{
			name:   "not found",
			err:    fmt.Errorf("outer: %w", NewErrNotFound("x")),
			assert: IsErrNotFound,
		},
		{
			name:   "bad request",
			err:    fmt.Errorf("outer: %w", NewErrBadRequest("x")),
			assert: IsErrBadRequest,
		},
		{
			name:   "internal server",
			err:    fmt.Errorf("outer: %w", NewInternalServerError("x")),
			assert: IsInternalServerError,
		},
		{
			name:   "service unavailable",
			err:    fmt.Errorf("outer: %w", NewErrServiceUnavailable("x")),
			assert: IsErrServiceUnavailable,
		},
		{
			name:   "conflict",
			err:    fmt.Errorf("outer: %w", NewErrConflict("x")),
			assert: IsErrConflict,
		},
		{
			name:   "denied",
			err:    fmt.Errorf("outer: %w", NewErrDenied("x")),
			assert: IsErrDenied,
		},
		{
			name:   "method not allowed",
			err:    fmt.Errorf("outer: %w", NewErrMethodNotAllowed("x")),
			assert: IsErrMethodNotAllowed,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if !tc.assert(tc.err) {
				t.Fatalf("expected classifier to match wrapped error: %v", tc.err)
			}
		})
	}
}

func TestNewErrorResponsePreservesExplicitServiceUnavailable(t *testing.T) {
	response := NewErrorResponse(
		errors.New("503 Service Unavailable: object storage unavailable"),
		http.StatusInternalServerError,
		"SMREPO",
		"PatchSubmodelElement",
		"EvidenceStore",
	)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.Code)
	}
}
