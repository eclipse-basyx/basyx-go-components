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

package grammar

import (
	"context"
	"strings"
)

type aasHierarchyQueriesContextKey struct{}

// ContextWithAASHierarchyQueries enables referenced Submodel hierarchy fields
// for AAS collectors created while handling ctx.
func ContextWithAASHierarchyQueries(ctx context.Context) context.Context {
	return context.WithValue(ctx, aasHierarchyQueriesContextKey{}, true)
}

// AASHierarchyQueriesEnabled reports whether ctx enables referenced Submodel
// hierarchy fields for AAS queries.
func AASHierarchyQueriesEnabled(ctx context.Context) bool {
	enabled, _ := ctx.Value(aasHierarchyQueriesContextKey{}).(bool)
	return enabled
}

// FindModelFieldByRoot returns the first model field whose root is in roots.
func FindModelFieldByRoot(query Query, roots ...string) (ModelStringPattern, bool) {
	rootSet := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		rootSet[root] = struct{}{}
	}

	for _, field := range query.Select {
		if modelFieldHasRoot(field, rootSet) {
			return field, true
		}
	}
	if query.Condition != nil {
		if field, found := findLogicalExpressionField(*query.Condition, rootSet); found {
			return field, true
		}
	}
	for _, filter := range query.FilterConditions {
		if filter.Condition == nil {
			continue
		}
		if field, found := findLogicalExpressionField(*filter.Condition, rootSet); found {
			return field, true
		}
	}
	return "", false
}

func findLogicalExpressionField(expression LogicalExpression, roots map[string]struct{}) (ModelStringPattern, bool) {
	for _, child := range append(expression.And, expression.Or...) {
		if field, found := findLogicalExpressionField(child, roots); found {
			return field, true
		}
	}
	if expression.Not != nil {
		if field, found := findLogicalExpressionField(*expression.Not, roots); found {
			return field, true
		}
	}
	for _, match := range expression.Match {
		if field, found := findMatchExpressionField(match, roots); found {
			return field, true
		}
	}
	return findExpressionOperandField(
		[]ComparisonItems{expression.Eq, expression.Ne, expression.Gt, expression.Ge, expression.Lt, expression.Le},
		[]StringItems{expression.Contains, expression.StartsWith, expression.EndsWith, expression.Regex},
		expression.BoolCast,
		roots,
	)
}

func findMatchExpressionField(expression MatchExpression, roots map[string]struct{}) (ModelStringPattern, bool) {
	for _, child := range expression.Match {
		if field, found := findMatchExpressionField(child, roots); found {
			return field, true
		}
	}
	return findExpressionOperandField(
		[]ComparisonItems{expression.Eq, expression.Ne, expression.Gt, expression.Ge, expression.Lt, expression.Le},
		[]StringItems{expression.Contains, expression.StartsWith, expression.EndsWith, expression.Regex},
		nil,
		roots,
	)
}

func findExpressionOperandField(
	comparisonSets []ComparisonItems,
	stringSets []StringItems,
	standaloneValue *Value,
	roots map[string]struct{},
) (ModelStringPattern, bool) {
	if standaloneValue != nil {
		if field, found := findValueField(*standaloneValue, roots); found {
			return field, true
		}
	}
	for _, values := range comparisonSets {
		for _, value := range values {
			if field, found := findValueField(value, roots); found {
				return field, true
			}
		}
	}
	for _, values := range stringSets {
		for _, value := range values {
			if field, found := findStringValueField(value, roots); found {
				return field, true
			}
		}
	}
	return "", false
}

func findValueField(value Value, roots map[string]struct{}) (ModelStringPattern, bool) {
	if value.Field != nil && modelFieldHasRoot(*value.Field, roots) {
		return *value.Field, true
	}
	children := []*Value{
		value.BoolCast, value.DateTimeCast, value.DayOfMonth, value.DayOfWeek,
		value.HexCast, value.Month, value.NumCast, value.StrCast, value.TimeCast, value.Year,
	}
	for _, child := range children {
		if child == nil {
			continue
		}
		if field, found := findValueField(*child, roots); found {
			return field, true
		}
	}
	return "", false
}

func findStringValueField(value StringValue, roots map[string]struct{}) (ModelStringPattern, bool) {
	if value.Field != nil && modelFieldHasRoot(*value.Field, roots) {
		return *value.Field, true
	}
	if value.StrCast != nil {
		return findValueField(*value.StrCast, roots)
	}
	return "", false
}

func modelFieldHasRoot(field ModelStringPattern, roots map[string]struct{}) bool {
	fieldString := string(field)
	separator := strings.IndexAny(fieldString, ".#")
	if separator < 0 {
		return false
	}
	_, found := roots[fieldString[:separator]]
	return found
}
