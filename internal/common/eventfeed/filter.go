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
	"fmt"
	"strings"
)

const rsqlPrefix = "rsql:"

var filterableFields = map[string]string{
	"event.type":       "event_type",
	"event.subject":    "subject",
	"event.source":     "source",
	"event.dataschema": "dataschema",
}

type comparison struct {
	Field    string
	Operator string
	Values   []string
}

type parsedFilter struct {
	Comparisons []comparison
}

func parseFilterParam(raw string) (*parsedFilter, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !strings.HasPrefix(raw, rsqlPrefix) {
		return nil, newQueryError("EVENTFEED-FILTER-PREFIX", "unknown filter prefix; only 'rsql:' is supported")
	}
	expr := strings.TrimSpace(strings.TrimPrefix(raw, rsqlPrefix))
	if expr == "" {
		return nil, newQueryError("EVENTFEED-FILTER-BLANK", "filter expression must not be blank")
	}
	return parseRSQL(expr)
}

func parseRSQL(expr string) (*parsedFilter, error) {
	if err := validateRSQLQuoting(expr); err != nil {
		return nil, err
	}
	parts := splitRSQLAnd(expr)
	out := &parsedFilter{Comparisons: make([]comparison, 0, len(parts))}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		cmp, err := parseComparison(part)
		if err != nil {
			return nil, err
		}
		out.Comparisons = append(out.Comparisons, cmp)
	}
	if len(out.Comparisons) == 0 {
		return nil, newQueryError("EVENTFEED-FILTER-MALFORMED", "malformed RSQL filter expression")
	}
	return out, nil
}

func splitRSQLAnd(expr string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(expr); {
		if n := rsqlQuotedSpan(expr, i); n > 0 {
			i += n
			continue
		}
		if expr[i] == '(' {
			i = rsqlSkipParen(expr, i)
			continue
		}
		if expr[i] == ';' {
			parts = append(parts, expr[start:i])
			i++
			start = i
			continue
		}
		if rsqlHasAndSeparator(expr, i) {
			parts = append(parts, expr[start:i])
			i += len(" and ")
			start = i
			continue
		}
		i++
	}
	parts = append(parts, expr[start:])
	return parts
}

func rsqlHasAndSeparator(expr string, i int) bool {
	const sep = " and "
	if i+len(sep) > len(expr) {
		return false
	}
	return strings.EqualFold(expr[i:i+len(sep)], sep)
}

// rsqlQuotedSpan returns the length of the quoted value starting at i, or 0
// when s[i] does not open one. An unterminated quote spans the rest of s;
// parseRSQL rejects those up front through validateRSQLQuoting.
func rsqlQuotedSpan(s string, i int) int {
	n, _ := rsqlQuotedSpanEnd(s, i)
	return n
}

// rsqlQuotedSpanEnd additionally reports whether the quote was closed. Both
// doubled quotes (”) and backslash escapes (\') escape a quote inside a
// quoted value.
func rsqlQuotedSpanEnd(s string, i int) (int, bool) {
	if i >= len(s) {
		return 0, true
	}
	quote := s[i]
	if quote != '\'' && quote != '"' {
		return 0, true
	}
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case quote:
			if j+1 < len(s) && s[j+1] == quote {
				j++
				continue
			}
			return j - i + 1, true
		}
	}
	return len(s) - i, false
}

func validateRSQLQuoting(expr string) error {
	for i := 0; i < len(expr); {
		n, closed := rsqlQuotedSpanEnd(expr, i)
		if n == 0 {
			i++
			continue
		}
		if !closed {
			return newQueryError("EVENTFEED-FILTER-MALFORMED", "unterminated quoted value in RSQL filter expression")
		}
		i += n
	}
	return nil
}

func rsqlSkipParen(s string, i int) int {
	depth := 0
	for i < len(s) {
		if n := rsqlQuotedSpan(s, i); n > 0 {
			i += n
			continue
		}
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}
	return i
}

var rsqlOperators = []string{"=out=", "=in=", "!=", "=="}

func parseComparison(expr string) (comparison, error) {
	idx, op := rsqlOperatorIndex(expr)
	if idx < 0 {
		return comparison{}, newQueryError("EVENTFEED-FILTER-MALFORMED", "malformed RSQL filter expression")
	}
	field := strings.TrimSpace(expr[:idx])
	valueRaw := strings.TrimSpace(expr[idx+len(op):])
	if field == "" || valueRaw == "" {
		return comparison{}, newQueryError("EVENTFEED-FILTER-MALFORMED", "malformed RSQL filter expression")
	}
	if strings.HasPrefix(field, "data.") {
		return comparison{}, newQueryError("EVENTFEED-FILTER-DATA", "filter on 'data.*' fields is not supported in v1")
	}
	if _, ok := filterableFields[field]; !ok {
		return comparison{}, newQueryError("EVENTFEED-FILTER-FIELD", fmt.Sprintf("filter on field '%s' is not supported", field))
	}
	values, err := parseRSQLValues(valueRaw)
	if err != nil {
		return comparison{}, err
	}
	return comparison{Field: field, Operator: op, Values: values}, nil
}

// rsqlOperatorIndex returns the position and text of the first comparison
// operator that starts outside a quoted value, so an operator-looking sequence
// inside a quoted identifier is not mistaken for the comparison operator.
func rsqlOperatorIndex(expr string) (int, string) {
	for i := 0; i < len(expr); {
		if n := rsqlQuotedSpan(expr, i); n > 0 {
			i += n
			continue
		}
		for _, op := range rsqlOperators {
			if strings.HasPrefix(expr[i:], op) {
				return i, op
			}
		}
		i++
	}
	return -1, ""
}

func parseRSQLValues(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") {
		inner := strings.TrimSpace(raw[1 : len(raw)-1])
		if inner == "" {
			return nil, newQueryError("EVENTFEED-FILTER-MALFORMED", "malformed RSQL filter expression")
		}
		parts := splitRSQLList(inner)
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			v, err := unquoteRSQL(strings.TrimSpace(p))
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	v, err := unquoteRSQL(raw)
	if err != nil {
		return nil, err
	}
	return []string{v}, nil
}

func splitRSQLList(inner string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(inner); {
		if n := rsqlQuotedSpan(inner, i); n > 0 {
			i += n
			continue
		}
		if inner[i] == ',' {
			parts = append(parts, inner[start:i])
			i++
			start = i
			continue
		}
		i++
	}
	parts = append(parts, inner[start:])
	return parts
}

func unquoteRSQL(v string) (string, error) {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return unescapeRSQL(v[1:len(v)-1], v[0]), nil
		}
	}
	if v == "" {
		return "", newQueryError("EVENTFEED-FILTER-MALFORMED", "malformed RSQL filter expression")
	}
	return v, nil
}

func columnForField(field string, presentation Presentation) (string, error) {
	col, ok := filterableFields[field]
	if !ok {
		return "", newQueryError("EVENTFEED-FILTER-FIELD", fmt.Sprintf("filter on field '%s' is not supported", field))
	}
	if col == "dataschema" {
		if presentation == PresentationCompact {
			return "dataschema_compact", nil
		}
		return "dataschema_full", nil
	}
	return col, nil
}

// unescapeRSQL resolves the two escape forms accepted inside a quoted value:
// a doubled quote (”) and a backslash escape (\', \" or \\).
func unescapeRSQL(inner string, quote byte) string {
	var b strings.Builder
	b.Grow(len(inner))
	for i := 0; i < len(inner); i++ {
		switch {
		case inner[i] == '\\' && i+1 < len(inner):
			i++
			b.WriteByte(inner[i])
		case inner[i] == quote && i+1 < len(inner) && inner[i+1] == quote:
			i++
			b.WriteByte(quote)
		default:
			b.WriteByte(inner[i])
		}
	}
	return b.String()
}
