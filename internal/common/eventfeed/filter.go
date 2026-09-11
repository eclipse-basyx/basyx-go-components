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
	"github.com/doug-martin/goqu/v9"
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
	Children    []*parsedFilter
	Disjunction bool
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

const maxFilterDepth = 32
const maxFilterLength = 16384

func parseRSQL(expr string) (*parsedFilter, error) {
	if len(expr) > maxFilterLength {
		return nil, malformedFilter("filter expression is too long")
	}
	if err := validateRSQLQuoting(expr); err != nil {
		return nil, err
	}
	return parseRSQLExpression(strings.TrimSpace(expr), 0)
}

func parseRSQLExpression(expr string, depth int) (*parsedFilter, error) {
	if depth > maxFilterDepth {
		return nil, malformedFilter("filter nesting is too deep")
	}
	if expr == "" {
		return nil, malformedFilter("empty filter expression")
	}
	for _, disjunction := range []bool{true, false} {
		parts, err := splitRSQLBoolean(expr, disjunction)
		if err != nil {
			return nil, err
		}
		if len(parts) > 1 {
			return parseRSQLChildren(parts, disjunction, depth+1)
		}
	}
	if expr[0] == '(' {
		if rsqlSkipParen(expr, 0) != len(expr) {
			return nil, malformedFilter("unexpected content after group")
		}
		return parseRSQLExpression(strings.TrimSpace(expr[1:len(expr)-1]), depth+1)
	}
	cmp, err := parseComparison(expr)
	if err != nil {
		return nil, err
	}
	return &parsedFilter{Comparisons: []comparison{cmp}}, nil
}

func parseRSQLChildren(parts []string, disjunction bool, depth int) (*parsedFilter, error) {
	result := &parsedFilter{Disjunction: disjunction}
	for _, part := range parts {
		child, err := parseRSQLExpression(strings.TrimSpace(part), depth)
		if err != nil {
			return nil, err
		}
		if !disjunction && len(child.Children) == 0 {
			result.Comparisons = append(result.Comparisons, child.Comparisons...)
		} else {
			result.Children = append(result.Children, child)
		}
	}
	return result, nil
}

func splitRSQLBoolean(expr string, disjunction bool) ([]string, error) {
	var parts []string
	start, depth := 0, 0
	for i := 0; i < len(expr); {
		if n := rsqlQuotedSpan(expr, i); n > 0 {
			i += n
			continue
		}
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth < 0 {
			return nil, malformedFilter("unmatched closing parenthesis")
		}
		if depth == 0 {
			if n := rsqlBooleanSeparator(expr[i:], disjunction); n > 0 {
				parts = append(parts, expr[start:i])
				i += n
				start = i
				continue
			}
		}
		i++
	}
	if depth != 0 {
		return nil, malformedFilter("unclosed parenthesis")
	}
	return append(parts, expr[start:]), nil
}

func rsqlBooleanSeparator(expr string, disjunction bool) int {
	separator, word := byte(';'), " and "
	if disjunction {
		separator, word = ',', " or "
	}
	if expr[0] == separator {
		return 1
	}
	if len(expr) >= len(word) && strings.EqualFold(expr[:len(word)], word) {
		return len(word)
	}
	return 0
}

func malformedFilter(message string) error {
	return newQueryError("EVENTFEED-FILTER-MALFORMED", message)
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
	values, err := parseRSQLValues(valueRaw, op)
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

func parseRSQLValues(raw, operator string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	list := operator == "=in=" || operator == "=out="
	if list != (strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")")) {
		return nil, malformedFilter("membership operators require a list; equality requires one scalar value")
	}
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

func unquoteRSQL(value string) (string, error) {
	if value == "" {
		return "", malformedFilter("empty unquoted value")
	}
	if value[0] == '\'' || value[0] == '"' {
		length, closed := rsqlQuotedSpanEnd(value, 0)
		if !closed || length != len(value) {
			return "", malformedFilter("unexpected content after quoted value")
		}
		return unescapeRSQL(value[1:len(value)-1], value[0]), nil
	}
	if strings.ContainsAny(value, "()=,!;<>\\\"' \t\r\n") {
		return "", malformedFilter("reserved characters must be quoted")
	}
	return value, nil
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
			_ = b.WriteByte(inner[i])
		case inner[i] == quote && i+1 < len(inner) && inner[i+1] == quote:
			i++
			_ = b.WriteByte(quote)
		default:
			_ = b.WriteByte(inner[i])
		}
	}
	return b.String()
}

func (f *parsedFilter) expression(presentation Presentation) (goqu.Expression, error) {
	expressions := make([]goqu.Expression, 0, len(f.Comparisons)+len(f.Children))
	for _, child := range f.Children {
		expr, err := child.expression(presentation)
		if err != nil {
			return nil, err
		}
		expressions = append(expressions, expr)
	}
	for _, cmp := range f.Comparisons {
		column, err := columnForField(cmp.Field, presentation)
		if err != nil {
			return nil, err
		}
		expr, err := filterExpression(column, cmp)
		if err != nil {
			return nil, err
		}
		expressions = append(expressions, expr)
	}
	if len(expressions) == 1 {
		return expressions[0], nil
	}
	if f.Disjunction {
		return goqu.Or(expressions...), nil
	}
	return goqu.And(expressions...), nil
}
