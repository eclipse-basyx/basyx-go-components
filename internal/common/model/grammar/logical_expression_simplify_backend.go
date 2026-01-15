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
	// SimplifyTrue - expression is trival and true
	SimplifyTrue
	// SimplifyFalse - expression is trival and false
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
	// Boolean literal stays as-is.
	if le.Boolean != nil {
		return le, decisionFromBool(*le.Boolean)
	}

	rle, rdec := handleComparison(le, resolve)
	if rle != nil {
		return *rle, rdec
	}

	// Logical: AND / OR
	if len(le.And) > 0 {
		if len(le.And) == 1 {
			return le.And[0].SimplifyForBackendFilter(resolve)
		}
		out := LogicalExpression{}
		anyUnknown := false
		// Short-circuit: if any child becomes false => whole AND is false.
		for _, sub := range le.And {
			t, decision := sub.SimplifyForBackendFilter(resolve)
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
			return le.Or[0].SimplifyForBackendFilter(resolve)
		}
		out := LogicalExpression{}
		anyUnknown := false
		// Short-circuit: if any child becomes true => whole OR is true.
		for _, sub := range le.Or {
			t, decision := sub.SimplifyForBackendFilter(resolve)
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
		t, decision := le.Not.SimplifyForBackendFilter(resolve)
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

func handleComparison(le LogicalExpression, resolve AttributeResolver) (*LogicalExpression, SimplifyDecision) {
	switch {
	case len(le.Eq) == 2:
		return reduceCmp(le, resolve, le.Eq, "$eq")
	case len(le.Ne) == 2:
		return reduceCmp(le, resolve, le.Ne, "$ne")
	case len(le.Gt) == 2:
		return reduceCmp(le, resolve, le.Gt, "$gt")
	case len(le.Ge) == 2:
		return reduceCmp(le, resolve, le.Ge, "$ge")
	case len(le.Lt) == 2:
		return reduceCmp(le, resolve, le.Lt, "$lt")
	case len(le.Le) == 2:
		return reduceCmp(le, resolve, le.Le, "$le")
	case len(le.Regex) == 2:
		return reduceCmp(le, resolve, stringItemsToValues(le.Regex), "$regex")
	case len(le.Contains) == 2:
		return reduceCmp(le, resolve, stringItemsToValues(le.Contains), "$contains")
	case len(le.StartsWith) == 2:
		return reduceCmp(le, resolve, stringItemsToValues(le.StartsWith), "$starts-with")
	case len(le.EndsWith) == 2:
		return reduceCmp(le, resolve, stringItemsToValues(le.EndsWith), "$ends-with")
	}
	return nil, SimplifyUndecided
}

func reduceCmp(le LogicalExpression, resolve AttributeResolver, items []Value, op string) (*LogicalExpression, SimplifyDecision) {
	if len(items) != 2 {
		return &le, SimplifyUndecided
	}

	left := replaceAttribute(items[0], resolve)
	right := replaceAttribute(items[1], resolve)
	isStringOp := op == "$regex" || op == "$contains" || op == "$starts-with" || op == "$ends-with"
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

	left = WrapCastAroundField(left, comparisonType)
	right = WrapCastAroundField(right, comparisonType)

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

func replaceAttribute(v Value, resolve AttributeResolver) Value {
	if valueContainsField(v) {
		return v
	}

	if valueContainsAttribute(v) {
		resolved := resolveValue(v, resolve)
		if lit, ok := literalValueFromAny(resolved); ok {
			return lit
		}
	}

	if !valueContainsAttribute(v) && !valueContainsField(v) {
		if lit, ok := literalValueFromAny(resolveValue(v, resolve)); ok {
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
	case time.Time:
		dt := DateTimeLiteralPattern(t)
		return Value{DateTimeVal: &dt}, true
	case string:
		if hv, ok := normalizeHexString(t); ok {
			hex := HexLiteralPattern(hv)
			return Value{HexVal: &hex}, true
		}
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			dt := DateTimeLiteralPattern(parsed)
			return Value{DateTimeVal: &dt}, true
		}
		if _, ok := toTimeOfDaySeconds(t); ok {
			tv := TimeLiteralPattern(t)
			return Value{TimeVal: &tv}, true
		}
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return Value{NumVal: &f}, true
		}
		s := StandardString(t)
		return Value{StrVal: &s}, true
	default:
		s := StandardString(fmt.Sprint(x))
		return Value{StrVal: &s}, true
	}
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
			return t.Format("15:04:05")
		}
		if _, ok := toTimeOfDaySeconds(inner); ok {
			return fmt.Sprint(inner)
		}
		return fmt.Sprint(inner)
	case v.DateTimeCast != nil:
		return fmt.Sprint(resolveValue(*v.DateTimeCast, resolve))
	case v.HexCast != nil:
		if hv, ok := normalizeHexAny(resolveValue(*v.HexCast, resolve)); ok {
			return hv
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
