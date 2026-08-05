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

package auth

import (
	"testing"
	"time"
)

func TestResolveAttributeValue_MissingClaimReturnsNil(t *testing.T) {
	attr := map[string]any{"CLAIM": "clear"}
	claims := Claims{"role": "editor"}

	got := resolveAttributeValue(attr, claims, nil)
	if got != nil {
		t.Fatalf("expected nil for missing claim, got %#v", got)
	}
}

func TestResolveAttributeValue_ClaimArrayUnwrapsFirstElement(t *testing.T) {
	attr := map[string]any{"CLAIM": "role"}
	claims := Claims{"role": []any{"editor"}}

	got := resolveAttributeValue(attr, claims, nil)
	if got != "editor" {
		t.Fatalf("expected editor, got %#v", got)
	}
}

func TestGlobalAttributesForEvaluationProvidesServerTimeWithoutClaims(t *testing.T) {
	currentTime := time.Date(2026, time.August, 5, 12, 30, 0, 0, time.FixedZone("UTC+2", 2*60*60))

	globals := globalAttributesForEvaluation(nil, nil, currentTime)

	if got := globals["UTCNOW"]; got != "2026-08-05T10:30:00Z" {
		t.Fatalf("UTCNOW = %#v, want 2026-08-05T10:30:00Z", got)
	}
	if got := globals["LOCALNOW"]; got != currentTime.In(time.Local).Format(time.RFC3339) {
		t.Fatalf("LOCALNOW = %#v, want server local time", got)
	}
	if _, exists := globals["CLIENTNOW"]; exists {
		t.Fatal("CLIENTNOW must not be generated for anonymous requests")
	}
}

func TestGlobalAttributesForEvaluationUsesAuthenticatedClientTime(t *testing.T) {
	clientNow := "2026-08-05T12:30:00+02:00"
	claims := Claims{"CLIENTNOW": clientNow}
	configured := GlobalAttributes{"CLIENTNOW": "untrusted"}

	globals := globalAttributesForEvaluation(configured, claims, time.Time{})

	if got := globals["CLIENTNOW"]; got != clientNow {
		t.Fatalf("CLIENTNOW = %#v, want %s", got, clientNow)
	}
}

func TestGlobalAttributesForEvaluationRejectsUnauthenticatedClientTime(t *testing.T) {
	configured := GlobalAttributes{"CLIENTNOW": "2026-08-05T12:30:00+02:00"}

	globals := globalAttributesForEvaluation(configured, nil, time.Time{})

	if _, exists := globals["CLIENTNOW"]; exists {
		t.Fatal("CLIENTNOW must not be accepted without an authenticated claim")
	}
}

func TestResolveAttributeValueKeepsGlobalsSeparateFromClaims(t *testing.T) {
	globals := GlobalAttributes{"UTCNOW": "2026-08-05T10:30:00Z"}

	globalValue := resolveAttributeValue(map[string]any{"GLOBAL": "UTCNOW"}, nil, globals)
	if globalValue != "2026-08-05T10:30:00Z" {
		t.Fatalf("GLOBAL UTCNOW = %#v, want 2026-08-05T10:30:00Z", globalValue)
	}

	claimValue := resolveAttributeValue(map[string]any{"CLAIM": "UTCNOW"}, nil, globals)
	if claimValue != nil {
		t.Fatalf("CLAIM UTCNOW = %#v, want nil", claimValue)
	}
}
