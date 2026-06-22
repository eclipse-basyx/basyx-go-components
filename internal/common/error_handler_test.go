/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package common

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
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

func TestIsPostgresUniqueViolationSupportsPGX(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "pgx", err: &pgconn.PgError{Code: "23505"}, want: true},
		{name: "wrapped pgx", err: fmt.Errorf("insert failed: %w", &pgconn.PgError{Code: "23505"}), want: true},
		{name: "different state", err: &pgconn.PgError{Code: "23503"}, want: false},
		{name: "ordinary error", err: errors.New("failed"), want: false},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := IsPostgresUniqueViolation(testCase.err); got != testCase.want {
				t.Fatalf("IsPostgresUniqueViolation() = %t, want %t", got, testCase.want)
			}
		})
	}
}
