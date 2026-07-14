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

package sequences

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsRc01UpgradeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "pgx error",
			err:  newRc01PgError(),
			want: true,
		},
		{
			name: "wrapped pgx error",
			err:  fmt.Errorf("schema upload failed: %w", newRc01PgError()),
			want: true,
		},
		{
			name: "wrong SQL state",
			err: &pgconn.PgError{
				Severity: "ERROR",
				Code:     "23505",
				Message:  `column "db_created_at" does not exist`,
			},
			want: false,
		},
		{
			name: "wrong column",
			err: &pgconn.PgError{
				Severity: "ERROR",
				Code:     "42703",
				Message:  `column "other_column" does not exist`,
			},
			want: false,
		},
		{
			name: "legacy pq error text",
			err:  errors.New(errCaseRc01UpgradeFieldErr),
			want: true,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertRc01UpgradeError(t, test.err, test.want)
		})
	}
}

func assertRc01UpgradeError(t *testing.T, err error, want bool) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("isRc01UpgradeError() panicked: %v", recovered)
		}
	}()
	if got := isRc01UpgradeError(err); got != want {
		t.Fatalf("isRc01UpgradeError() = %t, want %t", got, want)
	}
}

func newRc01PgError() error {
	return &pgconn.PgError{
		Severity: "ERROR",
		Code:     "42703",
		Message:  `column "db_created_at" does not exist`,
	}
}
