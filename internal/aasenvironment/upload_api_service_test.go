package aasenvironment

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func TestUploadProcessingStatus_PropagatesWrappedCommonErrors(t *testing.T) {
	testCases := []struct {
		name   string
		err    error
		status int
	}{
		{
			name:   "bad request",
			err:    fmt.Errorf("wrap: %w", common.NewErrBadRequest("x")),
			status: http.StatusBadRequest,
		},
		{
			name:   "method not allowed",
			err:    fmt.Errorf("wrap: %w", common.NewErrMethodNotAllowed("x")),
			status: http.StatusMethodNotAllowed,
		},
		{
			name:   "not found",
			err:    fmt.Errorf("wrap: %w", common.NewErrNotFound("x")),
			status: http.StatusNotFound,
		},
		{
			name:   "denied",
			err:    fmt.Errorf("wrap: %w", common.NewErrDenied("x")),
			status: http.StatusForbidden,
		},
		{
			name:   "conflict",
			err:    fmt.Errorf("wrap: %w", common.NewErrConflict("x")),
			status: http.StatusConflict,
		},
		{
			name:   "internal",
			err:    fmt.Errorf("wrap: %w", common.NewInternalServerError("x")),
			status: http.StatusInternalServerError,
		},
		{
			name:   "unknown defaults to internal",
			err:    fmt.Errorf("random"),
			status: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := uploadProcessingStatus(tc.err)
			if got != tc.status {
				t.Fatalf("expected status %d, got %d for %v", tc.status, got, tc.err)
			}
		})
	}
}
