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
	if strings.Contains(strings.ToLower(expr), " and ") {
		lower := strings.ToLower(expr)
		var parts []string
		start := 0
		for {
			idx := strings.Index(lower[start:], " and ")
			if idx < 0 {
				parts = append(parts, expr[start:])
				break
			}
			parts = append(parts, expr[start:start+idx])
			start = start + idx + len(" and ")
		}
		return parts
	}
	return strings.Split(expr, ";")
}

func parseComparison(expr string) (comparison, error) {
	ops := []string{"=out=", "=in=", "!=", "=="}
	for _, op := range ops {
		idx := strings.Index(expr, op)
		if idx < 0 {
			continue
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
	return comparison{}, newQueryError("EVENTFEED-FILTER-MALFORMED", "malformed RSQL filter expression")
}

func parseRSQLValues(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") {
		inner := strings.TrimSpace(raw[1 : len(raw)-1])
		if inner == "" {
			return nil, newQueryError("EVENTFEED-FILTER-MALFORMED", "malformed RSQL filter expression")
		}
		parts := strings.Split(inner, ",")
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

func unquoteRSQL(v string) (string, error) {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1], nil
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
