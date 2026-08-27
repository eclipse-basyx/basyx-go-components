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
	"encoding/json"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestResolveAttributeValue_MissingClaimReturnsNil(t *testing.T) {
	attr := map[string]any{"CLAIM": "clear"}
	claims := Claims{"role": "editor"}

	got := resolveAttributeValue(attr, claims, nil)
	if got != nil {
		t.Fatalf("expected nil for missing claim, got %#v", got)
	}
}

func TestResolveAttributeValueSerializesCompleteClaimValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		want  any
	}{
		{name: "string", value: "editor", want: "editor"},
		{name: "number", value: json.Number("42"), want: "42"},
		{name: "boolean", value: true, want: "true"},
		{name: "array", value: []any{"admin", "manager"}, want: `["admin","manager"]`},
		{
			name: "nested object",
			value: map[string]any{
				"roles": []any{"admin", "manager"},
				"profile": map[string]any{
					"department": "engineering",
				},
			},
			want: `{"profile":{"department":"engineering"},"roles":["admin","manager"]}`,
		},
		{name: "empty array", value: []any{}, want: `[]`},
		{name: "null", value: nil, want: nil},
		{name: "unsupported", value: func() {}, want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := resolveAttributeValue(
				map[string]any{"CLAIM": "claim"},
				Claims{"claim": test.value},
				nil,
			)
			if got != test.want {
				t.Fatalf("resolveAttributeValue() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestSerializedClaimArraySupportsQuotedContains(t *testing.T) {
	t.Parallel()

	claimAttribute := grammar.AttributeValue(map[string]any{"CLAIM": "role"})
	quotedManager := grammar.StandardString(`"manager"`)
	expression := grammar.LogicalExpression{
		Contains: grammar.StringItems{
			{Attribute: claimAttribute},
			{StrVal: &quotedManager},
		},
	}

	tests := []struct {
		name  string
		roles []any
		want  grammar.SimplifyDecision
	}{
		{name: "second element matches", roles: []any{"admin", "manager"}, want: grammar.SimplifyTrue},
		{name: "substring does not match", roles: []any{"supermanager"}, want: grammar.SimplifyFalse},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resolver := func(attribute grammar.AttributeValue) any {
				return resolveAttributeValue(attribute, Claims{"role": test.roles}, nil)
			}
			_, decision := expression.SimplifyForBackendFilter(resolver)
			if decision != test.want {
				t.Fatalf("SimplifyForBackendFilter() decision = %v, want %v", decision, test.want)
			}
		})
	}
}

func TestSerializedClaimArraySupportsElementBoundaryRegex(t *testing.T) {
	t.Parallel()

	const rawExpression = `{
		"$regex": [
			{"$attribute": {"CLAIM": "role"}},
			{"$strVal": "(^|\\[|,)\\s*\"manager\"\\s*(,|\\])"}
		]
	}`

	var expression grammar.LogicalExpression
	if err := json.Unmarshal([]byte(rawExpression), &expression); err != nil {
		t.Fatalf("unmarshal regex expression: %v", err)
	}

	tests := []struct {
		name  string
		value any
		want  grammar.SimplifyDecision
	}{
		{name: "only element matches", value: []any{"manager"}, want: grammar.SimplifyTrue},
		{name: "first element matches", value: []any{"manager", "admin"}, want: grammar.SimplifyTrue},
		{name: "middle element matches", value: []any{"admin", "manager", "reader"}, want: grammar.SimplifyTrue},
		{
			name: "deeply nested array element matches",
			value: map[string]any{
				"realm_access": map[string]any{
					"roles": []any{"admin", "manager"},
				},
			},
			want: grammar.SimplifyTrue,
		},
		{name: "missing element does not match", value: []any{"admin", "reader"}, want: grammar.SimplifyFalse},
		{name: "substring does not match", value: []any{"supermanager"}, want: grammar.SimplifyFalse},
		{name: "escaped suffix does not match", value: []any{`manager"`}, want: grammar.SimplifyFalse},
		{name: "object key does not match", value: map[string]any{"manager": false}, want: grammar.SimplifyFalse},
		{name: "empty array does not match", value: []any{}, want: grammar.SimplifyFalse},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resolver := func(attribute grammar.AttributeValue) any {
				return resolveAttributeValue(attribute, Claims{"role": test.value}, nil)
			}
			_, decision := expression.SimplifyForBackendFilter(resolver)
			if decision != test.want {
				t.Fatalf("SimplifyForBackendFilter() decision = %v, want %v", decision, test.want)
			}
		})
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
