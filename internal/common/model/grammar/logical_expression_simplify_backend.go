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
// Author: Martin Stemmer ( Fraunhofer IESE )

package grammar

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/stringification"
)

// AttributeResolver resolves an AttributeValue to a concrete scalar value.
//
// The grammar package does not define what an attribute *means* (e.g., CLAIM/GLOBAL);
// that interpretation is delegated to the caller.
//
// Return nil when an attribute cannot be resolved.
//
// Implementations should be deterministic for a given request context.
//
// Example:
//
//	resolver := func(attr grammar.AttributeValue) any { /* resolve CLAIM/GLOBAL */ }
//
//	adapted, decision := le.SimplifyForBackendFilter(resolver)
type AttributeResolver func(attr AttributeValue) any

// SimplifyOptions controls how backend simplification treats comparisons.
type SimplifyOptions struct {
	// EnableImplicitCasts wraps field operands in casts to match the other operand's type.
	EnableImplicitCasts bool
}

// DefaultSimplifyOptions returns the default settings used by SimplifyForBackendFilter.
func DefaultSimplifyOptions() SimplifyOptions {
	return SimplifyOptions{EnableImplicitCasts: true}
}

// SimplifyDecision is a tri-state result for SimplifyForBackendFilter.
//
// - SimplifyTrue / SimplifyFalse: the expression is fully decidable without backend context.
//
// When SimplifyTrue/SimplifyFalse is returned, the returned LogicalExpression will be a
// boolean literal.
//
//nolint:revive // Int enum name is fine.
type SimplifyDecision int

const (
	// SimplifyUndecided - the expression still depends on backend-only values (typically $field).
	SimplifyUndecided SimplifyDecision = iota
	// SimplifyTrue - expression is trivial and true
	SimplifyTrue
	// SimplifyFalse - expression is trivial and false
	SimplifyFalse
)

func decisionFromBool(b bool) SimplifyDecision {
	if b {
		return SimplifyTrue
	}
	return SimplifyFalse
}

// deduplicateLogicalExpressions removes duplicates from a slice.
// Two expressions are considered equal if their JSON representations match.
func deduplicateLogicalExpressions(exprs []LogicalExpression) []LogicalExpression {
	if len(exprs) <= 1 {
		return exprs
	}

	seen := make(map[string]struct{})
	result := make([]LogicalExpression, 0, len(exprs))

	for _, expr := range exprs {
		jsonBytes, err := json.Marshal(expr)
		if err != nil {
			// If marshaling fails, keep the expression to be safe.
			result = append(result, expr)
			continue
		}

		key := string(jsonBytes)
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			result = append(result, expr)
		}
	}

	return result
}

// SimplifyForBackendFilter partially evaluates parts of the expression that depend only
// on attributes (as resolved via the provided resolver) into a boolean literal.
//
// Any parts that depend on backend context (e.g., $field values) are preserved so they can
// be evaluated later (e.g., translated to SQL).
//
// The returned decision reports whether the full expression is decidable here:
//   - SimplifyTrue / SimplifyFalse: fully decided without backend evaluation
//   - SimplifyUndecided: still depends on backend ($field)
//
//nolint:revive // Cyclomatic complexity is acceptable here.
func (le LogicalExpression) SimplifyForBackendFilter(resolve AttributeResolver) (LogicalExpression, SimplifyDecision) {
	return le.SimplifyForBackendFilterWithOptions(resolve, DefaultSimplifyOptions())
}

// SimplifyForBackendFilterWithOptions behaves like SimplifyForBackendFilter but allows
// callers to control implicit casting behavior.
//
//nolint:revive // Cyclomatic complexity is acceptable here.
func (le LogicalExpression) SimplifyForBackendFilterWithOptions(resolve AttributeResolver, opts SimplifyOptions) (LogicalExpression, SimplifyDecision) {
	// Boolean literal stays as-is.
	if le.Boolean != nil {
		return le, decisionFromBool(*le.Boolean)
	}

	rle, rdec := handleComparison(le, resolve, opts)
	if rle != nil {
		return *rle, rdec
	}

	if len(le.Match) > 0 {
		simplified, decision := simplifyMatchExpressionsForBackendFilter(le.Match, resolve, opts)
		switch decision {
		case SimplifyTrue:
			b := true
			return LogicalExpression{Boolean: &b}, SimplifyTrue
		case SimplifyFalse:
			b := false
			return LogicalExpression{Boolean: &b}, SimplifyFalse
		default:
			if len(simplified) == 1 {
				return LogicalExpression{Match: simplified}, SimplifyUndecided
			}
			return LogicalExpression{Match: simplified}, SimplifyUndecided
		}
	}

	// Logical: AND / OR
	if len(le.And) > 0 {
		if len(le.And) == 1 {
			return le.And[0].SimplifyForBackendFilterWithOptions(resolve, opts)
		}
		out := LogicalExpression{}
		anyUnknown := false
		// Short-circuit: if any child becomes false => whole AND is false.
		for _, sub := range le.And {
			t, decision := sub.SimplifyForBackendFilterWithOptions(resolve, opts)
			switch decision {
			case SimplifyFalse:
				b := false
				return LogicalExpression{Boolean: &b}, SimplifyFalse
			case SimplifyTrue:
				// true child is neutral in AND; omit it
				continue
			case SimplifyUndecided:
				// keep expression
			}
			out.And = append(out.And, t)
			anyUnknown = true
		}
		if !anyUnknown {
			// All children were true (or empty after trimming) -> true
			b := true
			return LogicalExpression{Boolean: &b}, SimplifyTrue
		}
		out.And = deduplicateLogicalExpressions(out.And)
		if len(out.And) == 1 {
			return out.And[0], SimplifyUndecided
		}
		return out, SimplifyUndecided
	}

	if len(le.Or) > 0 {
		if len(le.Or) == 1 {
			return le.Or[0].SimplifyForBackendFilterWithOptions(resolve, opts)
		}
		out := LogicalExpression{}
		anyUnknown := false
		// Short-circuit: if any child becomes true => whole OR is true.
		for _, sub := range le.Or {
			t, decision := sub.SimplifyForBackendFilterWithOptions(resolve, opts)
			switch decision {
			case SimplifyTrue:
				b := true
				return LogicalExpression{Boolean: &b}, SimplifyTrue
			case SimplifyFalse:
				// false child is neutral in OR; omit it
				continue
			case SimplifyUndecided:
				// keep expression
			}
			out.Or = append(out.Or, t)
			anyUnknown = true
		}
		if !anyUnknown {
			// All children were false (or empty after trimming) -> false
			b := false
			return LogicalExpression{Boolean: &b}, SimplifyFalse
		}
		out.Or = deduplicateLogicalExpressions(out.Or)
		if len(out.Or) == 1 {
			return out.Or[0], SimplifyUndecided
		}
		return out, SimplifyUndecided
	}

	// Logical: NOT
	if le.Not != nil {
		t, decision := le.Not.SimplifyForBackendFilterWithOptions(resolve, opts)
		switch decision {
		case SimplifyTrue:
			b := false
			return LogicalExpression{Boolean: &b}, SimplifyFalse
		case SimplifyFalse:
			b := true
			return LogicalExpression{Boolean: &b}, SimplifyTrue
		default:
			return LogicalExpression{Not: &t}, SimplifyUndecided
		}
	}

	return le, SimplifyUndecided
}

// SimplifyForBackendFilterNoResolver runs SimplifyForBackendFilter with a no-op resolver.
// Attributes will remain unresolved, so only literal-only subexpressions can be reduced.
func (le LogicalExpression) SimplifyForBackendFilterNoResolver() (LogicalExpression, SimplifyDecision) {
	return le.SimplifyForBackendFilter(func(AttributeValue) any { return nil })
}

func simplifyMatchExpressionsForBackendFilter(match []MatchExpression, resolve AttributeResolver, opts SimplifyOptions) ([]MatchExpression, SimplifyDecision) {
	if len(match) == 0 {
		return nil, SimplifyUndecided
	}

	out := make([]MatchExpression, 0, len(match))
	anyUnknown := false
	for _, m := range match {
		t, decision := simplifyMatchExpressionForBackendFilter(m, resolve, opts)
		switch decision {
		case SimplifyFalse:
			return nil, SimplifyFalse
		case SimplifyTrue:
			continue
		case SimplifyUndecided:
		}
		out = append(out, t)
		anyUnknown = true
	}
	if !anyUnknown {
		return nil, SimplifyTrue
	}
	return out, SimplifyUndecided
}

func simplifyMatchExpressionForBackendFilter(me MatchExpression, resolve AttributeResolver, opts SimplifyOptions) (MatchExpression, SimplifyDecision) {
	if me.Boolean != nil {
		return me, decisionFromBool(*me.Boolean)
	}

	switch {
	case len(me.Eq) == 2:
		out, decision := reduceMatchCmp(me, resolve, me.Eq, "$eq", opts)
		return derefMatch(out, decision)
	case len(me.Ne) == 2:
		out, decision := reduceMatchCmp(me, resolve, me.Ne, "$ne", opts)
		return derefMatch(out, decision)
	case len(me.Gt) == 2:
		out, decision := reduceMatchCmp(me, resolve, me.Gt, "$gt", opts)
		return derefMatch(out, decision)
	case len(me.Ge) == 2:
		out, decision := reduceMatchCmp(me, resolve, me.Ge, "$ge", opts)
		return derefMatch(out, decision)
	case len(me.Lt) == 2:
		out, decision := reduceMatchCmp(me, resolve, me.Lt, "$lt", opts)
		return derefMatch(out, decision)
	case len(me.Le) == 2:
		out, decision := reduceMatchCmp(me, resolve, me.Le, "$le", opts)
		return derefMatch(out, decision)
	case len(me.Regex) == 2:
		out, decision := reduceMatchCmp(me, resolve, stringItemsToValues(me.Regex), "$regex", opts)
		return derefMatch(out, decision)
	case len(me.Contains) == 2:
		out, decision := reduceMatchCmp(me, resolve, stringItemsToValues(me.Contains), "$contains", opts)
		return derefMatch(out, decision)
	case len(me.StartsWith) == 2:
		out, decision := reduceMatchCmp(me, resolve, stringItemsToValues(me.StartsWith), "$starts-with", opts)
		return derefMatch(out, decision)
	case len(me.EndsWith) == 2:
		out, decision := reduceMatchCmp(me, resolve, stringItemsToValues(me.EndsWith), "$ends-with", opts)
		return derefMatch(out, decision)
	}

	if len(me.Match) > 0 {
		simplified, decision := simplifyMatchExpressionsForBackendFilter(me.Match, resolve, opts)
		switch decision {
		case SimplifyTrue:
			b := true
			return MatchExpression{Boolean: &b}, SimplifyTrue
		case SimplifyFalse:
			b := false
			return MatchExpression{Boolean: &b}, SimplifyFalse
		default:
			return MatchExpression{Match: simplified}, SimplifyUndecided
		}
	}

	return me, SimplifyUndecided
}

func derefMatch(me *MatchExpression, decision SimplifyDecision) (MatchExpression, SimplifyDecision) {
	if me == nil {
		return MatchExpression{}, decision
	}
	return *me, decision
}

func handleComparison(le LogicalExpression, resolve AttributeResolver, opts SimplifyOptions) (*LogicalExpression, SimplifyDecision) {
	switch {
	case len(le.Eq) == 2:
		return reduceCmp(le, resolve, le.Eq, "$eq", opts)
	case len(le.Ne) == 2:
		return reduceCmp(le, resolve, le.Ne, "$ne", opts)
	case len(le.Gt) == 2:
		return reduceCmp(le, resolve, le.Gt, "$gt", opts)
	case len(le.Ge) == 2:
		return reduceCmp(le, resolve, le.Ge, "$ge", opts)
	case len(le.Lt) == 2:
		return reduceCmp(le, resolve, le.Lt, "$lt", opts)
	case len(le.Le) == 2:
		return reduceCmp(le, resolve, le.Le, "$le", opts)
	case len(le.Regex) == 2:
		return reduceCmp(le, resolve, stringItemsToValues(le.Regex), "$regex", opts)
	case len(le.Contains) == 2:
		return reduceCmp(le, resolve, stringItemsToValues(le.Contains), "$contains", opts)
	case len(le.StartsWith) == 2:
		return reduceCmp(le, resolve, stringItemsToValues(le.StartsWith), "$starts-with", opts)
	case len(le.EndsWith) == 2:
		return reduceCmp(le, resolve, stringItemsToValues(le.EndsWith), "$ends-with", opts)
	}
	return nil, SimplifyUndecided
}

func reduceMatchCmp(me MatchExpression, resolve AttributeResolver, items []Value, op string, opts SimplifyOptions) (*MatchExpression, SimplifyDecision) {
	if len(items) != 2 {
		return &me, SimplifyUndecided
	}

	left := replaceAttribute(items[0], resolve)
	right := replaceAttribute(items[1], resolve)
	isStringOp := op == "$regex" || op == "$contains" || op == "$starts-with" || op == "$ends-with"
	if !isStringOp {
		left, right = convertEnumLiteralIfNeeded(left, right)
	}
	var comparisonType ComparisonKind
	if isStringOp {
		comparisonType = KindString
	} else {
		var err error
		comparisonType, err = left.IsComparableTo(right)
		if err != nil {
			return &me, SimplifyUndecided
		}
	}

	if opts.EnableImplicitCasts {
		left = WrapCastAroundField(left, comparisonType)
		right = WrapCastAroundField(right, comparisonType)
	}

	out := MatchExpression{}
	leOut := LogicalExpression{}
	switch op {
	case "$eq":
		out.Eq = []Value{left, right}
		leOut.Eq = []Value{left, right}
	case "$ne":
		out.Ne = []Value{left, right}
		leOut.Ne = []Value{left, right}
	case "$gt":
		out.Gt = []Value{left, right}
		leOut.Gt = []Value{left, right}
	case "$ge":
		out.Ge = []Value{left, right}
		leOut.Ge = []Value{left, right}
	case "$lt":
		out.Lt = []Value{left, right}
		leOut.Lt = []Value{left, right}
	case "$le":
		out.Le = []Value{left, right}
		leOut.Le = []Value{left, right}
	case "$regex":
		out.Regex = []StringValue{valueToStringValue(left), valueToStringValue(right)}
		leOut.Regex = out.Regex
	case "$contains":
		out.Contains = []StringValue{valueToStringValue(left), valueToStringValue(right)}
		leOut.Contains = out.Contains
	case "$starts-with":
		out.StartsWith = []StringValue{valueToStringValue(left), valueToStringValue(right)}
		leOut.StartsWith = out.StartsWith
	case "$ends-with":
		out.EndsWith = []StringValue{valueToStringValue(left), valueToStringValue(right)}
		leOut.EndsWith = out.EndsWith
	}

	if isLiteral(left) && isLiteral(right) {
		b := evalComparisonOnly(leOut, resolve)
		return &MatchExpression{Boolean: &b}, decisionFromBool(b)
	}

	return &out, SimplifyUndecided
}

func reduceCmp(le LogicalExpression, resolve AttributeResolver, items []Value, op string, opts SimplifyOptions) (*LogicalExpression, SimplifyDecision) {
	if len(items) != 2 {
		return &le, SimplifyUndecided
	}

	left := replaceAttribute(items[0], resolve)
	right := replaceAttribute(items[1], resolve)
	isStringOp := op == "$regex" || op == "$contains" || op == "$starts-with" || op == "$ends-with"
	if !isStringOp {
		left, right = convertEnumLiteralIfNeeded(left, right)
	}
	var comparisonType ComparisonKind
	if isStringOp {
		comparisonType = KindString
	} else {
		var err error
		comparisonType, err = left.IsComparableTo(right)
		if err != nil {
			return &le, SimplifyUndecided
		}
	}

	if opts.EnableImplicitCasts {
		left = WrapCastAroundField(left, comparisonType)
		right = WrapCastAroundField(right, comparisonType)
	}

	out := LogicalExpression{}
	switch op {
	case "$eq":
		out.Eq = []Value{left, right}
	case "$ne":
		out.Ne = []Value{left, right}
	case "$gt":
		out.Gt = []Value{left, right}
	case "$ge":
		out.Ge = []Value{left, right}
	case "$lt":
		out.Lt = []Value{left, right}
	case "$le":
		out.Le = []Value{left, right}
	case "$regex":
		out.Regex = []StringValue{valueToStringValue(left), valueToStringValue(right)}
	case "$contains":
		out.Contains = []StringValue{valueToStringValue(left), valueToStringValue(right)}
	case "$starts-with":
		out.StartsWith = []StringValue{valueToStringValue(left), valueToStringValue(right)}
	case "$ends-with":
		out.EndsWith = []StringValue{valueToStringValue(left), valueToStringValue(right)}
	}

	if isLiteral(left) && isLiteral(right) {
		b := evalComparisonOnly(out, resolve)
		if b {
			return &LogicalExpression{Boolean: &b}, SimplifyTrue
		}
		return &LogicalExpression{Boolean: &b}, SimplifyFalse
	}

	return &out, SimplifyUndecided
}

func evalComparisonOnly(le LogicalExpression, resolve AttributeResolver) bool {
	if len(le.Gt) == 2 {
		return orderedCmp(le.Gt[0], le.Gt[1], resolve, "gt")
	}
	if len(le.Ge) == 2 {
		return orderedCmp(le.Ge[0], le.Ge[1], resolve, "ge")
	}
	if len(le.Lt) == 2 {
		return orderedCmp(le.Lt[0], le.Lt[1], resolve, "lt")
	}
	if len(le.Le) == 2 {
		return orderedCmp(le.Le[0], le.Le[1], resolve, "le")
	}

	if len(le.Eq) == 2 {
		return eqCmp(le.Eq[0], le.Eq[1], resolve, false)
	}
	if len(le.Ne) == 2 {
		return eqCmp(le.Ne[0], le.Ne[1], resolve, true)
	}

	if len(le.Regex) == 2 {
		hay := asString(resolveStringItem(le.Regex[0], resolve))
		pat := asString(resolveStringItem(le.Regex[1], resolve))
		re, err := regexp.Compile(pat)
		if err != nil {
			return false
		}
		return re.MatchString(hay)
	}
	if len(le.Contains) == 2 {
		hay := asString(resolveStringItem(le.Contains[0], resolve))
		needle := asString(resolveStringItem(le.Contains[1], resolve))
		return strings.Contains(hay, needle)
	}
	if len(le.StartsWith) == 2 {
		hay := asString(resolveStringItem(le.StartsWith[0], resolve))
		prefix := asString(resolveStringItem(le.StartsWith[1], resolve))
		return strings.HasPrefix(hay, prefix)
	}
	if len(le.EndsWith) == 2 {
		hay := asString(resolveStringItem(le.EndsWith[0], resolve))
		suffix := asString(resolveStringItem(le.EndsWith[1], resolve))
		return strings.HasSuffix(hay, suffix)
	}

	return false
}

func convertEnumLiteralIfNeeded(left, right Value) (Value, Value) {
	if field, _ := extractFieldOperandAndCast(&left); field != nil && right.StrVal != nil {
		if converted, ok := convertEnumLiteralForField(*field, right); ok {
			right = converted
		}
		return left, right
	}
	if field, _ := extractFieldOperandAndCast(&right); field != nil && left.StrVal != nil {
		if converted, ok := convertEnumLiteralForField(*field, left); ok {
			left = converted
		}
	}
	return left, right
}

func convertEnumLiteralForField(field Value, lit Value) (Value, bool) {
	if field.Field == nil || lit.StrVal == nil {
		return Value{}, false
	}
	fieldName := string(*field.Field)
	f := ModelStringPattern(fieldName)
	resolved, err := ResolveScalarFieldToSQL(&f)
	if err != nil {
		return Value{}, false
	}
	if !strings.Contains(strings.ToLower(resolved.Column), "value_type") {
		return Value{}, false
	}
	if enumVal, ok := stringification.DataTypeDefXSDFromString(string(*lit.StrVal)); ok {
		if converted, ok := enumValueToValue(enumVal); ok {
			return converted, true
		}
	}
	return Value{}, false
}

func enumValueToValue(enumVal interface{}) (Value, bool) {
	v, ok := enumVal.(int)
	if !ok {
		return Value{}, false
	}
	f := float64(v)
	return Value{NumVal: &f}, true
}

func replaceAttribute(v Value, resolve AttributeResolver) Value {
	if valueContainsField(v) {
		return v
	}

	if valueContainsAttribute(v) {
		resolved := resolveValue(v, resolve)
		if lit, ok := literalValueFromAnyWithHint(resolved, v.EffectiveTypeWithCast()); ok {
			return lit
		}
	}

	if !valueContainsAttribute(v) && !valueContainsField(v) {
		if lit, ok := literalValueFromAnyWithHint(resolveValue(v, resolve), v.EffectiveTypeWithCast()); ok {
			return lit
		}
	}

	return v
}

func valueContainsField(v Value) bool {
	if v.Field != nil {
		return true
	}
	for _, child := range valueChildren(v) {
		if child != nil && valueContainsField(*child) {
			return true
		}
	}
	return false
}

func valueContainsAttribute(v Value) bool {
	if v.Attribute != nil {
		return true
	}
	for _, child := range valueChildren(v) {
		if child != nil && valueContainsAttribute(*child) {
			return true
		}
	}
	return false
}

func valueChildren(v Value) []*Value {
	return []*Value{
		v.BoolCast,
		v.DateTimeCast,
		v.HexCast,
		v.NumCast,
		v.StrCast,
		v.TimeCast,
	}
}

func isLiteral(v Value) bool {
	return v.IsValue() && !v.IsField() && v.Attribute == nil
}

func literalValueFromAny(x any) (Value, bool) {
	switch t := x.(type) {
	case nil:
		return Value{}, false
	case bool:
		b := t
		return Value{Boolean: &b}, true
	case float64:
		f := t
		return Value{NumVal: &f}, true
	case float32:
		f := float64(t)
		return Value{NumVal: &f}, true
	case int:
		f := float64(t)
		return Value{NumVal: &f}, true
	case int32:
		f := float64(t)
		return Value{NumVal: &f}, true
	case int64:
		f := float64(t)
		return Value{NumVal: &f}, true
	case TimeLiteralPattern:
		tv := t
		return Value{TimeVal: &tv}, true
	case HexLiteralPattern:
		hv := t
		return Value{HexVal: &hv}, true
	case *HexLiteralPattern:
		if t == nil {
			return Value{}, false
		}
		hv := *t
		return Value{HexVal: &hv}, true
	case time.Time:
		dt := DateTimeLiteralPattern(t)
		return Value{DateTimeVal: &dt}, true
	case string:
		s := StandardString(t)
		return Value{StrVal: &s}, true
	default:
		s := StandardString(fmt.Sprint(x))
		return Value{StrVal: &s}, true
	}
}

func literalValueFromAnyWithHint(x any, hint ComparisonKind) (Value, bool) {
	switch hint {
	case KindDateTime:
		if dt, ok := toDateTime(x); ok {
			pat := DateTimeLiteralPattern(dt)
			return Value{DateTimeVal: &pat}, true
		}
	case KindTime:
		s := strings.TrimSpace(fmt.Sprint(x))
		if s != "" {
			if _, ok := toTimeOfDaySeconds(s); ok {
				pat := TimeLiteralPattern(s)
				return Value{TimeVal: &pat}, true
			}
		}
	case KindHex:
		if hv, ok := normalizeHexAny(x); ok {
			pat := HexLiteralPattern(hv)
			return Value{HexVal: &pat}, true
		}
	case KindNumber:
		if f, ok := toFloat(x); ok {
			return Value{NumVal: &f}, true
		}
	case KindBool:
		switch v := x.(type) {
		case bool:
			b := v
			return Value{Boolean: &b}, true
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true", "1", "yes", "y", "on":
				b := true
				return Value{Boolean: &b}, true
			case "false", "0", "no", "n", "off":
				b := false
				return Value{Boolean: &b}, true
			}
		}
	}
	return literalValueFromAny(x)
}

func stringItemsToValues(items []StringValue) []Value {
	out := make([]Value, 0, len(items))
	for _, it := range items {
		if it.Field != nil {
			out = append(out, Value{Field: it.Field})
			continue
		}
		if it.StrVal != nil {
			out = append(out, Value{StrVal: it.StrVal})
			continue
		}
		if it.Attribute != nil {
			out = append(out, Value{Attribute: it.Attribute})
			continue
		}
		if it.StrCast != nil {
			out = append(out, Value{StrCast: it.StrCast})
			continue
		}
		out = append(out, Value{})
	}
	return out
}

func valueToStringValue(v Value) StringValue {
	if v.DateTimeVal != nil || v.TimeVal != nil || v.Year != nil || v.Month != nil || v.DayOfMonth != nil || v.DayOfWeek != nil {
		s := StandardString(stringValueFromDate(v))
		return StringValue{StrVal: &s}
	}
	if v.Field != nil {
		return StringValue{Field: v.Field}
	}
	if v.StrVal != nil {
		return StringValue{StrVal: v.StrVal}
	}
	if v.Attribute != nil {
		return StringValue{Attribute: v.Attribute}
	}
	if v.StrCast != nil {
		return StringValue{StrCast: v.StrCast}
	}
	s := StandardString(fmt.Sprint(v.GetValue()))
	return StringValue{StrVal: &s}
}

func resolveDateTimeLiteral(v Value) any {
	switch {
	case v.DateTimeVal != nil:
		return time.Time(*v.DateTimeVal)
	case v.TimeVal != nil:
		tv := TimeLiteralPattern(*v.TimeVal)
		return tv
	case v.Year != nil:
		return float64(time.Time(*v.Year).Year())
	case v.Month != nil:
		return float64(int(time.Time(*v.Month).Month()))
	case v.DayOfMonth != nil:
		return float64(time.Time(*v.DayOfMonth).Day())
	case v.DayOfWeek != nil:
		return float64(int(time.Time(*v.DayOfWeek).Weekday()))
	default:
		return nil
	}
}

func resolveCastValue(v Value, resolve AttributeResolver) any {
	switch {
	case v.StrCast != nil:
		return fmt.Sprint(resolveValue(*v.StrCast, resolve))
	case v.NumCast != nil:
		x := resolveValue(*v.NumCast, resolve)
		if f, ok := toFloat(x); ok {
			return f
		}
		if s, ok := x.(string); ok {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f
			}
		}
		return x
	case v.BoolCast != nil:
		return castToBool(resolveValue(*v.BoolCast, resolve))
	case v.TimeCast != nil:
		inner := resolveValue(*v.TimeCast, resolve)
		if t, ok := toDateTime(inner); ok {
			return TimeLiteralPattern(t.Format("15:04:05"))
		}
		if s := fmt.Sprint(inner); s != "" {
			if _, ok := toTimeOfDaySeconds(s); ok {
				return TimeLiteralPattern(s)
			}
		}
		return fmt.Sprint(inner)
	case v.DateTimeCast != nil:
		inner := resolveValue(*v.DateTimeCast, resolve)
		if t, ok := toDateTime(inner); ok {
			return t
		}
		return fmt.Sprint(inner)
	case v.HexCast != nil:
		if hv, ok := normalizeHexAny(resolveValue(*v.HexCast, resolve)); ok {
			return HexLiteralPattern(hv)
		}
		return fmt.Sprint(resolveValue(*v.HexCast, resolve))
	default:
		return nil
	}
}

func resolveValue(v Value, resolve AttributeResolver) any {
	if v.Attribute != nil {
		return resolve(v.Attribute)
	}

	if v.StrVal != nil {
		return string(*v.StrVal)
	}
	if v.NumVal != nil {
		return *v.NumVal
	}
	if v.Boolean != nil {
		return *v.Boolean
	}

	if v.DateTimeVal != nil || v.TimeVal != nil || v.Year != nil || v.Month != nil || v.DayOfMonth != nil || v.DayOfWeek != nil {
		return resolveDateTimeLiteral(v)
	}

	if v.HexVal != nil {
		if hv, ok := normalizeHexString(string(*v.HexVal)); ok {
			return hv
		}
		return string(*v.HexVal)
	}

	if v.Field != nil {
		return ""
	}

	return resolveCastValue(v, resolve)
}

func resolveStringItem(s StringValue, resolve AttributeResolver) string {
	if s.Attribute != nil {
		return asString(resolve(s.Attribute))
	}
	if s.StrVal != nil {
		return string(*s.StrVal)
	}
	if s.StrCast != nil {
		return fmt.Sprint(resolveValue(*s.StrCast, resolve))
	}
	if s.Field != nil {
		return ""
	}
	return ""
}

func asString(v any) string {
	return fmt.Sprint(v)
}

func castToBool(v any) bool {
	switch strings.ToLower(fmt.Sprint(v)) {
	case "true", "1", "yes", "y", "on":
		return true
	case "false", "0", "no", "n", "off", "":
		return false
	default:
		return false
	}
}

func orderedCmp(left, right Value, resolve AttributeResolver, op string) bool {
	comparisonType, err := left.IsComparableTo(right)
	if err != nil {
		return false
	}

	switch comparisonType {
	case KindNumber:
		lv, lok := resolveNumberValue(left, resolve)
		rv, rok := resolveNumberValue(right, resolve)
		if !lok || !rok {
			return false
		}
		return compareFloats(lv, rv, op)
	case KindTime:
		lv, lok := resolveTimeValue(left, resolve)
		rv, rok := resolveTimeValue(right, resolve)
		if !lok || !rok {
			return false
		}
		return compareInts(lv, rv, op)
	case KindDateTime:
		lv, lok := resolveDateTimeValue(left, resolve)
		rv, rok := resolveDateTimeValue(right, resolve)
		if !lok || !rok {
			return false
		}
		return compareTimes(lv, rv, op)
	case KindHex:
		lv, lok := resolveHexValue(left, resolve)
		rv, rok := resolveHexValue(right, resolve)
		if !lok || !rok {
			return false
		}
		return compareHex(lv, rv, op)
	default:
		return false
	}
}

func eqCmp(left, right Value, resolve AttributeResolver, negate bool) bool {
	comparisonType, err := left.IsComparableTo(right)
	if err != nil {
		return negate
	}

	equal := false
	switch comparisonType {
	case KindNumber:
		lv, lok := resolveNumberValue(left, resolve)
		rv, rok := resolveNumberValue(right, resolve)
		equal = lok && rok && lv == rv
	case KindDateTime:
		lv, lok := resolveDateTimeValue(left, resolve)
		rv, rok := resolveDateTimeValue(right, resolve)
		equal = lok && rok && lv.Equal(rv)
	case KindTime:
		lv, lok := resolveTimeValue(left, resolve)
		rv, rok := resolveTimeValue(right, resolve)
		equal = lok && rok && lv == rv
	case KindHex:
		lv, lok := resolveHexValue(left, resolve)
		rv, rok := resolveHexValue(right, resolve)
		equal = lok && rok && lv == rv
	case KindBool:
		lv, lok := resolveBoolValue(left, resolve)
		rv, rok := resolveBoolValue(right, resolve)
		equal = lok && rok && lv == rv
	case KindString:
		lv, lok := resolveStringValue(left, resolve)
		rv, rok := resolveStringValue(right, resolve)
		equal = lok && rok && lv == rv
	default:
		equal = false
	}

	if negate {
		return !equal
	}
	return equal
}

func resolveNumberValue(v Value, resolve AttributeResolver) (float64, bool) {
	switch {
	case v.NumVal != nil:
		return *v.NumVal, true
	case v.Year != nil:
		return float64(time.Time(*v.Year).Year()), true
	case v.Month != nil:
		return float64(int(time.Time(*v.Month).Month())), true
	case v.DayOfMonth != nil:
		return float64(time.Time(*v.DayOfMonth).Day()), true
	case v.DayOfWeek != nil:
		return float64(int(time.Time(*v.DayOfWeek).Weekday())), true
	case v.NumCast != nil:
		raw := resolveValue(*v.NumCast, resolve)
		if f, ok := toFloat(raw); ok {
			return f, true
		}
		if s, ok := raw.(string); ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
				return f, true
			}
		}
		return 0, false
	default:
		raw := resolveValue(v, resolve)
		if f, ok := toFloat(raw); ok {
			return f, true
		}
	}
	return 0, false
}

func resolveDateTimeValue(v Value, resolve AttributeResolver) (time.Time, bool) {
	switch {
	case v.DateTimeVal != nil:
		return time.Time(*v.DateTimeVal), true
	case v.DateTimeCast != nil:
		return toDateTime(resolveValue(*v.DateTimeCast, resolve))
	default:
		return toDateTime(resolveValue(v, resolve))
	}
}

func resolveTimeValue(v Value, resolve AttributeResolver) (int, bool) {
	switch {
	case v.TimeVal != nil:
		return toTimeOfDaySeconds(*v.TimeVal)
	case v.TimeCast != nil:
		raw := resolveValue(*v.TimeCast, resolve)
		if dt, ok := toDateTime(raw); ok {
			return dt.Hour()*3600 + dt.Minute()*60 + dt.Second(), true
		}
		return toTimeOfDaySeconds(raw)
	default:
		return toTimeOfDaySeconds(resolveValue(v, resolve))
	}
}

func resolveHexValue(v Value, resolve AttributeResolver) (string, bool) {
	switch {
	case v.HexVal != nil:
		return normalizeHexString(string(*v.HexVal))
	case v.HexCast != nil:
		return normalizeHexAny(resolveValue(*v.HexCast, resolve))
	default:
		return normalizeHexAny(resolveValue(v, resolve))
	}
}

func resolveBoolValue(v Value, resolve AttributeResolver) (bool, bool) {
	switch {
	case v.Boolean != nil:
		return *v.Boolean, true
	case v.BoolCast != nil:
		return castToBool(resolveValue(*v.BoolCast, resolve)), true
	default:
		return false, false
	}
}

func resolveStringValue(v Value, resolve AttributeResolver) (string, bool) {
	switch {
	case v.StrVal != nil:
		return string(*v.StrVal), true
	case v.StrCast != nil:
		return fmt.Sprint(resolveValue(*v.StrCast, resolve)), true
	case v.Attribute != nil:
		return asString(resolve(v.Attribute)), true
	case v.Field != nil:
		return "", false
	default:
		return asString(resolveValue(v, resolve)), true
	}
}

func compareFloats(a, b float64, op string) bool {
	switch op {
	case "gt":
		return a > b
	case "ge":
		return a >= b
	case "lt":
		return a < b
	case "le":
		return a <= b
	default:
		return false
	}
}

func compareInts(a, b int, op string) bool {
	switch op {
	case "gt":
		return a > b
	case "ge":
		return a >= b
	case "lt":
		return a < b
	case "le":
		return a <= b
	default:
		return false
	}
}

func compareTimes(a, b time.Time, op string) bool {
	switch op {
	case "gt":
		return a.After(b)
	case "ge":
		return a.After(b) || a.Equal(b)
	case "lt":
		return a.Before(b)
	case "le":
		return a.Before(b) || a.Equal(b)
	default:
		return false
	}
}

func compareHex(a, b, op string) bool {
	ai, aok := hexToBigInt(a)
	bi, bok := hexToBigInt(b)
	if !aok || !bok {
		return false
	}

	cmp := ai.Cmp(bi)
	switch op {
	case "gt":
		return cmp > 0
	case "ge":
		return cmp >= 0
	case "lt":
		return cmp < 0
	case "le":
		return cmp <= 0
	default:
		return false
	}
}

var hexLiteralRegex = regexp.MustCompile(`(?i)^16#[0-9a-f]+$`)

func normalizeHexString(raw string) (string, bool) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" || !hexLiteralRegex.MatchString(s) {
		return "", false
	}
	return s, true
}

func normalizeHexAny(v any) (string, bool) {
	switch h := v.(type) {
	case HexLiteralPattern:
		return normalizeHexString(string(h))
	case *HexLiteralPattern:
		if h == nil {
			return "", false
		}
		return normalizeHexString(string(*h))
	default:
		return normalizeHexString(fmt.Sprint(v))
	}
}

func hexToBigInt(hex string) (*big.Int, bool) {
	if hex == "" {
		return nil, false
	}
	s := strings.TrimPrefix(strings.ToLower(hex), "16#")
	if s == "" {
		return nil, false
	}
	i := new(big.Int)
	if _, ok := i.SetString(s, 16); !ok {
		return nil, false
	}
	return i, true
}

func stringValueFromDate(v Value) string {
	switch {
	case v.DateTimeVal != nil:
		return time.Time(*v.DateTimeVal).Format(time.RFC3339)
	case v.TimeVal != nil:
		return string(*v.TimeVal)
	case v.Year != nil:
		return time.Time(*v.Year).Format("2006")
	case v.Month != nil:
		return time.Time(*v.Month).Format("01")
	case v.DayOfMonth != nil:
		return time.Time(*v.DayOfMonth).Format("02")
	case v.DayOfWeek != nil:
		return time.Time(*v.DayOfWeek).Weekday().String()
	default:
		return ""
	}
}

func toFloat(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func toTimeOfDaySeconds(value interface{}) (int, bool) {
	s := strings.TrimSpace(fmt.Sprint(value))
	if s == "" {
		return 0, false
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	sec := 0
	var errS error
	if len(parts) == 3 {
		sec, errS = strconv.Atoi(parts[2])
	}
	if errH != nil || errM != nil || (len(parts) == 3 && errS != nil) {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 || sec < 0 || sec > 59 {
		return 0, false
	}
	return h*3600 + m*60 + sec, true
}

func toDateTime(value interface{}) (time.Time, bool) {
	switch v := value.(type) {
	case time.Time:
		return v, true
	case DateTimeLiteralPattern:
		return time.Time(v), true
	case *DateTimeLiteralPattern:
		if v == nil {
			return time.Time{}, false
		}
		return time.Time(*v), true
	default:
		s := strings.TrimSpace(fmt.Sprint(value))
		if s == "" {
			return time.Time{}, false
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}, false
		}
		return t, true
	}
}

func field(value string) Value {
	p := ModelStringPattern(value)
	return Value{Field: &p}
}

func strField(value string) StringValue {
	p := ModelStringPattern(value)
	return StringValue{Field: &p}
}

func strVal(value string) Value {
	s := StandardString(value)
	return Value{StrVal: &s}
}

func strString(value string) StringValue {
	s := StandardString(value)
	return StringValue{StrVal: &s}
}
