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
	"reflect"
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

func TestResolveAttributeValueAcceptsScalarAndStringArrayClaims(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		want  any
	}{
		{name: "string", value: "editor", want: "editor"},
		{name: "number", value: json.Number("42"), want: "42"},
		{name: "boolean", value: true, want: "true"},
		{name: "single-item array", value: []any{"admin"}, want: []string{"admin"}},
		{name: "multi-item array", value: []any{"admin", "manager"}, want: []string{"admin", "manager"}},
		{
			name: "nested object",
			value: map[string]any{
				"roles": []any{"admin", "manager"},
				"profile": map[string]any{
					"department": "engineering",
				},
			},
			want: nil,
		},
		{name: "empty array", value: []any{}, want: nil},
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
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("resolveAttributeValue() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestResolveAttributeValueClaimPathSelectsOnlyRequestedValue(t *testing.T) {
	t.Parallel()

	claims := Claims{
		"realm_access": map[string]any{"roles": []any{"reader", "admin"}},
		"profile":      map[string]any{"tags": []any{"admin"}},
	}
	got := resolveAttributeValue(map[string]any{"CLAIMPATH": "/realm_access/roles"}, claims, nil)
	if want := []string{"reader", "admin"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveAttributeValue() = %#v, want %#v", got, want)
	}
}

func TestResolveAttributeValueClaimPathSupportsJSONPointerEscapes(t *testing.T) {
	t.Parallel()

	claims := Claims{"realm/access": map[string]any{"role~name": "admin"}}
	got := resolveAttributeValue(map[string]any{"CLAIMPATH": "/realm~1access/role~0name"}, claims, nil)
	if got != "admin" {
		t.Fatalf("resolveAttributeValue() = %#v, want admin", got)
	}
}

func TestStringArrayClaimUsesInForExactMembership(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		roles []any
		want  grammar.SimplifyDecision
	}{
		{name: "exact member", roles: []any{"reader", "admin"}, want: grammar.SimplifyTrue},
		{name: "substring", roles: []any{"administrator"}, want: grammar.SimplifyFalse},
		{name: "escaped delimiter", roles: []any{`x","admin`}, want: grammar.SimplifyFalse},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			expression := grammar.LogicalExpression{In: []grammar.Value{
				{StrVal: stringValue("admin")},
				{Attribute: map[string]any{"CLAIMPATH": "/realm_access/roles"}},
			}}
			resolver := func(attribute grammar.AttributeValue) any {
				return resolveAttributeValue(attribute, Claims{
					"realm_access": map[string]any{"roles": test.roles},
					"profile":      map[string]any{"tags": []any{"admin"}},
				}, nil)
			}
			_, decision := expression.SimplifyForBackendFilter(resolver)
			if decision != test.want {
				t.Fatalf("SimplifyForBackendFilter() decision = %v, want %v", decision, test.want)
			}
		})
	}
}

func TestEqualityAndStringOperatorsRejectArrayClaims(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expression string
	}{
		{name: "equality", expression: `{"$eq":[{"$attribute":{"CLAIM":"roles"}},{"$strVal":"admin"}]}`},
		{name: "contains", expression: `{"$contains":[{"$attribute":{"CLAIM":"roles"}},{"$strVal":"admin"}]}`},
		{name: "regex", expression: `{"$regex":[{"$attribute":{"CLAIM":"roles"}},{"$strVal":"admin"}]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var expression grammar.LogicalExpression
			if err := json.Unmarshal([]byte(test.expression), &expression); err != nil {
				t.Fatalf("unmarshal expression: %v", err)
			}
			resolver := func(attribute grammar.AttributeValue) any {
				return resolveAttributeValue(attribute, Claims{"roles": []any{"reader", "admin"}}, nil)
			}
			_, decision := expression.SimplifyForBackendFilter(resolver)
			if decision != grammar.SimplifyInvalid {
				t.Fatalf("SimplifyForBackendFilter() decision = %v, want %v", decision, grammar.SimplifyInvalid)
			}
		})
	}
}

func stringValue(value string) *grammar.StandardString {
	pattern := grammar.StandardString(value)
	return &pattern
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
