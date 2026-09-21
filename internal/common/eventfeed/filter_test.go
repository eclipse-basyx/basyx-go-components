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

package eventfeed

import (
	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestParseFilterOK(t *testing.T) {
	f, err := parseFilterParam("rsql:event.type==io.admin-shell.aas.created.v1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Comparisons) != 1 || f.Comparisons[0].Operator != "==" {
		t.Fatalf("unexpected %+v", f)
	}
}

func TestParseFilterIn(t *testing.T) {
	f, err := parseFilterParam("rsql:event.type=in=(io.admin-shell.aas.created.v1,io.admin-shell.aas.updated.v1)")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Comparisons[0].Values) != 2 {
		t.Fatalf("values=%v", f.Comparisons[0].Values)
	}
}

func TestParseFilterRejectsDataField(t *testing.T) {
	_, err := parseFilterParam("rsql:data.semanticId==x")
	if !IsQueryError(err) {
		t.Fatalf("expected query error, got %v", err)
	}
}

func TestParseFilterRejectsUnknownPrefix(t *testing.T) {
	_, err := parseFilterParam("jq:event.type==x")
	if !IsQueryError(err) {
		t.Fatalf("expected query error, got %v", err)
	}
}

func TestParseFilterAnd(t *testing.T) {
	f, err := parseFilterParam("rsql:event.type==a and event.subject==b")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Comparisons) != 2 {
		t.Fatalf("comparisons=%d", len(f.Comparisons))
	}
}

func TestParseFilterQuotedSemicolonAndComma(t *testing.T) {
	f, err := parseFilterParam("rsql:event.subject=='urn:example:sm;v1'")
	if err != nil {
		t.Fatalf("parse quoted subject: %v", err)
	}
	if len(f.Comparisons) != 1 || f.Comparisons[0].Values[0] != "urn:example:sm;v1" {
		t.Fatalf("subject=%+v", f.Comparisons)
	}

	f, err = parseFilterParam("rsql:event.subject=in=('a,b','c,d')")
	if err != nil {
		t.Fatalf("parse quoted in: %v", err)
	}
	if len(f.Comparisons[0].Values) != 2 || f.Comparisons[0].Values[0] != "a,b" || f.Comparisons[0].Values[1] != "c,d" {
		t.Fatalf("in values=%v", f.Comparisons[0].Values)
	}
}

func TestParseFilterQuotedDelimitersInsideValues(t *testing.T) {
	cases := []struct {
		name  string
		expr  string
		field string
		op    string
		want  []string
	}{
		{
			name:  "semicolon inside quoted subject",
			expr:  "rsql:event.subject=='urn:example:sm;v1'",
			field: "event.subject",
			op:    "==",
			want:  []string{"urn:example:sm;v1"},
		},
		{
			name:  "not-equals operator inside quoted subject",
			expr:  "rsql:event.subject=='urn:example:sm!=v1'",
			field: "event.subject",
			op:    "==",
			want:  []string{"urn:example:sm!=v1"},
		},
		{
			name:  "in operator inside quoted subject",
			expr:  "rsql:event.subject=='urn:example:sm=in=v1'",
			field: "event.subject",
			op:    "==",
			want:  []string{"urn:example:sm=in=v1"},
		},
		{
			name:  "and separator inside quoted subject",
			expr:  "rsql:event.subject=='urn:example:a and b'",
			field: "event.subject",
			op:    "==",
			want:  []string{"urn:example:a and b"},
		},
		{
			name:  "commas inside quoted list values",
			expr:  "rsql:event.subject=in=('urn:example:sm,a','urn:example:sm,b')",
			field: "event.subject",
			op:    "=in=",
			want:  []string{"urn:example:sm,a", "urn:example:sm,b"},
		},
		{
			name:  "doubled quote escape",
			expr:  "rsql:event.subject=='urn:example:it''s'",
			field: "event.subject",
			op:    "==",
			want:  []string{"urn:example:it's"},
		},
		{
			name:  "backslash quote escape",
			expr:  `rsql:event.subject=='urn:example:it\'s'`,
			field: "event.subject",
			op:    "==",
			want:  []string{"urn:example:it's"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := parseFilterParam(tc.expr)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(f.Comparisons) != 1 {
				t.Fatalf("comparisons=%d want 1: %+v", len(f.Comparisons), f.Comparisons)
			}
			cmp := f.Comparisons[0]
			if cmp.Field != tc.field || cmp.Operator != tc.op {
				t.Fatalf("field=%s operator=%s", cmp.Field, cmp.Operator)
			}
			if len(cmp.Values) != len(tc.want) {
				t.Fatalf("values=%v want %v", cmp.Values, tc.want)
			}
			for i, want := range tc.want {
				if cmp.Values[i] != want {
					t.Fatalf("values[%d]=%q want %q", i, cmp.Values[i], want)
				}
			}
		})
	}
}

func TestParseFilterQuotedValueDoesNotBreakConjunction(t *testing.T) {
	f, err := parseFilterParam("rsql:event.subject=='urn:example:sm;v1';event.type=='" + TypeSubmodelUpdated + "'")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Comparisons) != 2 {
		t.Fatalf("comparisons=%d want 2: %+v", len(f.Comparisons), f.Comparisons)
	}
	if f.Comparisons[0].Values[0] != "urn:example:sm;v1" {
		t.Fatalf("subject=%v", f.Comparisons[0].Values)
	}
	if f.Comparisons[1].Values[0] != TypeSubmodelUpdated {
		t.Fatalf("type=%v", f.Comparisons[1].Values)
	}
}

func TestParseFilterUnterminatedQuoteIsRejected(t *testing.T) {
	if _, err := parseFilterParam("rsql:event.subject=='urn:example:sm"); err == nil {
		t.Fatal("expected an error for an unterminated quoted value")
	}
}

func TestFilterBooleanExpressions(t *testing.T) {
	cases := []struct{ name, input, expected string }{
		{"or", "rsql:event.subject==a,event.subject==b", `(("subject" = 'a') OR ("subject" = 'b'))`},
		{"and precedence", "rsql:event.subject==a,event.subject==b;event.type==created", `(("subject" = 'a') OR (("subject" = 'b') AND ("event_type" = 'created')))`},
		{"group precedence", "rsql:(event.subject==a,event.subject==b);event.type==created", `((("subject" = 'a') OR ("subject" = 'b')) AND ("event_type" = 'created'))`},
		{"quoted comma", "rsql:event.subject=='a,b'", `("subject" = 'a,b')`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := parseFilterParam(tc.input)
			require.NoError(t, err)
			expr, err := f.expression(PresentationRegular)
			require.NoError(t, err)
			query, _, err := goqu.Dialect("postgres").From("feed_events").Select("id").Where(expr).ToSQL()
			require.NoError(t, err)
			require.Equal(t, `SELECT "id" FROM "feed_events" WHERE `+tc.expected, query)
		})
	}
}

func TestFilterRejectsMalformedSyntax(t *testing.T) {
	for _, input := range []string{
		"event.subject==a;", ";event.subject==a", "event.subject==a,,event.subject==b",
		"(event.subject==a", "event.subject==a)", "event.subject==a=b",
		"event.subject==a b", "event.subject=='a'junk", "event.subject=in=a", "event.subject==(a,b)",
		"event.subject=in=(a,)", "event.subject=in=(a,(b))", "event.subject=gt=a",
	} {
		t.Run(input, func(t *testing.T) {
			_, err := parseFilterParam("rsql:" + input)
			require.Error(t, err)
			require.True(t, IsQueryError(err))
		})
	}
}
