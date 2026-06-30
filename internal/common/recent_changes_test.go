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
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
)

func TestNormalizeRecentChangesLimitDoesNotImposeUndocumentedMaximum(t *testing.T) {
	got, err := NormalizeRecentChangesLimit(1001)
	if err != nil {
		t.Fatalf("NormalizeRecentChangesLimit returned unexpected error: %v", err)
	}
	if got != 1001 {
		t.Fatalf("expected limit 1001, got %d", got)
	}
}

func TestNormalizeRecentChangesLimitDefaultsMissingLimit(t *testing.T) {
	got, err := NormalizeRecentChangesLimit(0)
	if err != nil {
		t.Fatalf("NormalizeRecentChangesLimit returned unexpected error: %v", err)
	}
	if got != DefaultRecentChangesLimit {
		t.Fatalf("expected default limit %d, got %d", DefaultRecentChangesLimit, got)
	}
}

func TestRecentChangeTimestampsRequireValidAdministrationTimestamps(t *testing.T) {
	createdAt := "2026-01-02T03:04:05Z"
	updatedAt := "2026-01-02T03:04:06Z"
	administration := types.NewAdministrativeInformation()
	administration.SetCreatedAt(&createdAt)
	administration.SetUpdatedAt(&updatedAt)

	gotCreatedAt, gotUpdatedAt, ok := RecentChangeTimestamps(administration)
	if !ok {
		t.Fatal("expected valid timestamps")
	}
	if gotCreatedAt != createdAt {
		t.Fatalf("expected createdAt %q, got %q", createdAt, gotCreatedAt)
	}
	if gotUpdatedAt != updatedAt {
		t.Fatalf("expected updatedAt %q, got %q", updatedAt, gotUpdatedAt)
	}
}

func TestRecentChangeTimestampsRejectMissingOrInvalidTimestamps(t *testing.T) {
	valid := "2026-01-02T03:04:05Z"
	invalid := "not-a-date-time"

	tests := []struct {
		name      string
		createdAt *string
		updatedAt *string
	}{
		{name: "nil-administration"},
		{name: "missing-createdAt", updatedAt: &valid},
		{name: "missing-updatedAt", createdAt: &valid},
		{name: "invalid-createdAt", createdAt: &invalid, updatedAt: &valid},
		{name: "invalid-updatedAt", createdAt: &valid, updatedAt: &invalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var administration types.IAdministrativeInformation
			if tt.name != "nil-administration" {
				candidate := types.NewAdministrativeInformation()
				candidate.SetCreatedAt(tt.createdAt)
				candidate.SetUpdatedAt(tt.updatedAt)
				administration = candidate
			}

			if _, _, ok := RecentChangeTimestamps(administration); ok {
				t.Fatal("expected timestamps to be rejected")
			}
		})
	}
}
