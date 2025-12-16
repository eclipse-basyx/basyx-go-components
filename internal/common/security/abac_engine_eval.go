/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

package auth

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	jsoniter "github.com/json-iterator/go"
)

func resolveGlobalToken(name string, claims Claims) (any, bool) {
	switch strings.ToUpper(name) {
	case "UTCNOW":
		if val, ok := claims["UTCNOW"]; ok {
			return normalizeClaimScalar(val), true
		}
		return "", false
	case "LOCALNOW":
		if val, ok := claims["LOCALNOW"]; ok {
			return normalizeClaimScalar(val), true
		}
		return "", false
	case "CLIENTNOW":
		if val, ok := claims["CLIENTNOW"]; ok {
			return normalizeClaimScalar(val), true
		}
		return "", false
	case "ANONYMOUS":
		return "ANONYMOUS", true
	default:
		return "", false
	}
}

func evalLE(le grammar.LogicalExpression, claims Claims) bool {
	if le.Boolean != nil {
		return *le.Boolean
	}

	if len(le.Gt) == 2 {
		return orderedCmp(le.Gt[0], le.Gt[1], claims, "gt")
	}
	if len(le.Ge) == 2 {
		return orderedCmp(le.Ge[0], le.Ge[1], claims, "ge")
	}
	if len(le.Lt) == 2 {
		return orderedCmp(le.Lt[0], le.Lt[1], claims, "lt")
	}
	if len(le.Le) == 2 {
		return orderedCmp(le.Le[0], le.Le[1], claims, "le")
	}

	if len(le.Eq) == 2 {
		return eqCmp(le.Eq[0], le.Eq[1], claims, false)
	}
	if len(le.Ne) == 2 {
		return eqCmp(le.Ne[0], le.Ne[1], claims, true)
	}

	if len(le.Regex) == 2 {
		hay := asString(resolveStringItem(le.Regex[0], claims))
		pat := asString(resolveStringItem(le.Regex[1], claims))
		re, err := regexp.Compile(pat)
		if err != nil {
			return false
		}
		return re.MatchString(hay)
	}
	if len(le.Contains) == 2 {
		hay := asString(resolveStringItem(le.Contains[0], claims))
		needle := asString(resolveStringItem(le.Contains[1], claims))
		return strings.Contains(hay, needle)
	}
	if len(le.StartsWith) == 2 {
		hay := asString(resolveStringItem(le.StartsWith[0], claims))
		prefix := asString(resolveStringItem(le.StartsWith[1], claims))
		return strings.HasPrefix(hay, prefix)
	}
	if len(le.EndsWith) == 2 {
		hay := asString(resolveStringItem(le.EndsWith[0], claims))
		suffix := asString(resolveStringItem(le.EndsWith[1], claims))
		return strings.HasSuffix(hay, suffix)
	}

	if len(le.And) >= 2 {
		for _, sub := range le.And {
			if !evalLE(sub, claims) {
				return false
			}
		}
		return true
	}
	if len(le.Or) >= 2 {
		for _, sub := range le.Or {
			if evalLE(sub, claims) {
				return true
			}
		}
		return false
	}
	if le.Not != nil {
		return !evalLE(*le.Not, claims)
	}

	return false
}

// reduceCmp: try to replace attributes with literals, and
// if both operands become literals, reduce to $boolean using evalLE.
func reduceCmp(le grammar.LogicalExpression, claims Claims, items []grammar.Value, op string) (*grammar.LogicalExpression, bool) {
	if len(items) != 2 {
		return &le, false
	}

	left := replaceAttribute(items[0], claims)
	right := replaceAttribute(items[1], claims)
	isStringOp := op == "$regex" || op == "$contains" || op == "$starts-with" || op == "$ends-with"
	var comparisonType grammar.ComparisonKind
	if isStringOp {
		// String operators work on the string representation regardless of the literal's native type.
		comparisonType = grammar.KindString
	} else {
		var err error
		comparisonType, err = left.IsComparableTo(right)
		if err != nil {
			return &le, false
		}
	}

	left = grammar.WrapCastAroundField(left, comparisonType)
	right = grammar.WrapCastAroundField(right, comparisonType)

	// Construct a new LE with updated operands
	out := grammar.LogicalExpression{}
	switch op {
	case "$eq":
		out.Eq = []grammar.Value{left, right}
	case "$ne":
		out.Ne = []grammar.Value{left, right}
	case "$gt":
		out.Gt = []grammar.Value{left, right}
	case "$ge":
		out.Ge = []grammar.Value{left, right}
	case "$lt":
		out.Lt = []grammar.Value{left, right}
	case "$le":
		out.Le = []grammar.Value{left, right}
	case "$regex":
		out.Regex = []grammar.StringValue{valueToStringValue(left), valueToStringValue(right)}
	case "$contains":
		out.Contains = []grammar.StringValue{valueToStringValue(left), valueToStringValue(right)}
	case "$starts-with":
		out.StartsWith = []grammar.StringValue{valueToStringValue(left), valueToStringValue(right)}
	case "$ends-with":
		out.EndsWith = []grammar.StringValue{valueToStringValue(left), valueToStringValue(right)}
	}

	// If both operands are literals (not fields and not attributes), try to evaluate
	if isLiteral(left) && isLiteral(right) {
		// Evaluate by reusing evalLE on this sub-expression
		b := false
		tmp := out
		if evalLE(tmp, claims) {
			b = true
		}
		return &grammar.LogicalExpression{Boolean: &b}, true
	}
	return &out, false
}

// adaptLEForBackend takes a logical expression and partially evaluates parts
// that depend only on CLAIM or GLOBAL attributes into $boolean true/false.
// The remaining expression is returned for backend evaluation. The second
// return value indicates whether the entire expression became a pure boolean
// expression (i.e., consists only of true/false after reduction).
//
// nolint:revive // Cyclomatic complexity is 22 instead of 20 which is fine for now.
func adaptLEForBackend(le grammar.LogicalExpression, claims Claims) (grammar.LogicalExpression, bool) {
	// Boolean literal stays as-is
	if le.Boolean != nil {
		return le, true
	}

	rle, rbool := handleComparison(le, claims)
	if rle != nil {
		return *rle, rbool
	}

	// Logical: AND / OR
	if len(le.And) > 0 {
		if len(le.And) == 1 {
			return adaptLEForBackend(le.And[0], claims)
		}
		out := grammar.LogicalExpression{}
		anyUnknown := false
		// Short-circuit: if any child becomes false => whole AND is false.
		for _, sub := range le.And {
			t, onlyBool := adaptLEForBackend(sub, claims)
			if onlyBool && t.Boolean != nil {
				if !*t.Boolean {
					b := false
					return grammar.LogicalExpression{Boolean: &b}, true
				}
				// true child is neutral in AND; omit it
				continue
			}
			out.And = append(out.And, t)
			anyUnknown = true
		}
		if !anyUnknown {
			// All children were true (or empty after trimming) -> true
			b := true
			return grammar.LogicalExpression{Boolean: &b}, true
		}
		// Single remaining branch -> remove redundant AND wrapper
		if len(out.And) == 1 {
			return out.And[0], false
		}
		return out, false
	}

	if len(le.Or) > 0 {
		if len(le.Or) == 1 {
			return adaptLEForBackend(le.Or[0], claims)
		}
		out := grammar.LogicalExpression{}
		anyUnknown := false
		// Short-circuit: if any child becomes true => whole OR is true.
		for _, sub := range le.Or {
			t, onlyBool := adaptLEForBackend(sub, claims)
			if onlyBool && t.Boolean != nil {
				if *t.Boolean {
					b := true
					return grammar.LogicalExpression{Boolean: &b}, true
				}
				// false child is neutral in OR; omit it
				continue
			}
			out.Or = append(out.Or, t)
			anyUnknown = true
		}
		if !anyUnknown {
			// All children were false (or empty after trimming) -> false
			b := false
			return grammar.LogicalExpression{Boolean: &b}, true
		}
		// Single remaining branch -> remove redundant OR wrapper
		if len(out.Or) == 1 {
			return out.Or[0], false
		}
		return out, false
	}

	// Logical: NOT
	if le.Not != nil {
		t, onlyBool := adaptLEForBackend(*le.Not, claims)
		if onlyBool && t.Boolean != nil {
			b := !*t.Boolean
			return grammar.LogicalExpression{Boolean: &b}, true
		}
		return grammar.LogicalExpression{Not: &t}, false
	}

	// Unknown or unsupported -> cannot fully decide here
	return le, false
}

func handleComparison(le grammar.LogicalExpression, claims Claims) (*grammar.LogicalExpression, bool) {
	switch {
	case len(le.Eq) == 2:
		return reduceCmp(le, claims, le.Eq, "$eq")
	case len(le.Ne) == 2:
		return reduceCmp(le, claims, le.Ne, "$ne")
	case len(le.Gt) == 2:
		return reduceCmp(le, claims, le.Gt, "$gt")
	case len(le.Ge) == 2:
		return reduceCmp(le, claims, le.Ge, "$ge")
	case len(le.Lt) == 2:
		return reduceCmp(le, claims, le.Lt, "$lt")
	case len(le.Le) == 2:
		return reduceCmp(le, claims, le.Le, "$le")
	case len(le.Regex) == 2:
		return reduceCmp(le, claims, stringItemsToValues(le.Regex), "$regex")
	case len(le.Contains) == 2:
		return reduceCmp(le, claims, stringItemsToValues(le.Contains), "$contains")
	case len(le.StartsWith) == 2:
		return reduceCmp(le, claims, stringItemsToValues(le.StartsWith), "$starts-with")
	case len(le.EndsWith) == 2:
		return reduceCmp(le, claims, stringItemsToValues(le.EndsWith), "$ends-with")
	}
	return nil, false
}

// replaceAttribute resolves a Value that is deterministic from CLAIM/GLOBAL attributes
// (including via casts) to a literal when possible. Values that reference $field
// remain untouched because they cannot be evaluated without backend context.
func replaceAttribute(v grammar.Value, claims Claims) grammar.Value {
	if valueContainsField(v) {
		return v
	}

	if valueContainsAttribute(v) {
		// Use existing resolver to get a concrete value that may depend on claims
		resolved := resolveValue(v, claims)
		if lit, ok := literalValueFromAny(resolved); ok {
			return lit
		}
	}

	// Pure literal (including casts around literals): collapse to a literal value
	if !valueContainsAttribute(v) && !valueContainsField(v) {
		if lit, ok := literalValueFromAny(resolveValue(v, claims)); ok {
			return lit
		}
	}

	return v
}

func valueContainsField(v grammar.Value) bool {
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

func valueContainsAttribute(v grammar.Value) bool {
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

func valueChildren(v grammar.Value) []*grammar.Value {
	return []*grammar.Value{
		v.BoolCast,
		v.DateTimeCast,
		v.HexCast,
		v.NumCast,
		v.StrCast,
		v.TimeCast,
	}
}

// isLiteral returns true if the Value represents a literal (not a field and not an attribute)
func isLiteral(v grammar.Value) bool {
	return v.IsValue() && !v.IsField() && v.Attribute == nil
}

// literalValueFromAny converts a Go value into a grammar.Value with a literal.
func literalValueFromAny(x any) (grammar.Value, bool) {
	switch t := x.(type) {
	case nil:
		return grammar.Value{}, false
	case bool:
		b := t
		return grammar.Value{Boolean: &b}, true
	case float64:
		f := t
		return grammar.Value{NumVal: &f}, true
	case float32:
		f := float64(t)
		return grammar.Value{NumVal: &f}, true
	case int:
		f := float64(t)
		return grammar.Value{NumVal: &f}, true
	case int32:
		f := float64(t)
		return grammar.Value{NumVal: &f}, true
	case int64:
		f := float64(t)
		return grammar.Value{NumVal: &f}, true
	case grammar.TimeLiteralPattern:
		tv := t
		return grammar.Value{TimeVal: &tv}, true
	case time.Time:
		dt := grammar.DateTimeLiteralPattern(t)
		return grammar.Value{DateTimeVal: &dt}, true
	case string:
		if hv, ok := normalizeHexString(t); ok {
			hex := grammar.HexLiteralPattern(hv)
			return grammar.Value{HexVal: &hex}, true
		}
		// Try RFC3339 datetime first
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			dt := grammar.DateTimeLiteralPattern(parsed)
			return grammar.Value{DateTimeVal: &dt}, true
		}
		// Then try time-of-day
		if _, ok := toTimeOfDaySeconds(t); ok {
			tv := grammar.TimeLiteralPattern(t)
			return grammar.Value{TimeVal: &tv}, true
		}
		// Then try numeric
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return grammar.Value{NumVal: &f}, true
		}
		s := grammar.StandardString(t)
		return grammar.Value{StrVal: &s}, true
	default:
		// Fallback to string representation
		s := grammar.StandardString(fmt.Sprint(x))
		return grammar.Value{StrVal: &s}, true
	}
}

// stringItemsToValues is a small helper to treat StringItems like a 2-Value slice
func stringItemsToValues(items []grammar.StringValue) []grammar.Value {
	out := make([]grammar.Value, 0, len(items))
	for _, it := range items {
		// Convert StringValue to Value
		if it.Field != nil {
			out = append(out, grammar.Value{Field: it.Field})
			continue
		}
		if it.StrVal != nil {
			out = append(out, grammar.Value{StrVal: it.StrVal})
			continue
		}
		if it.Attribute != nil {
			// Leave as attribute inside Value for further resolution
			out = append(out, grammar.Value{Attribute: it.Attribute})
			continue
		}
		if it.StrCast != nil {
			out = append(out, grammar.Value{StrCast: it.StrCast})
			continue
		}
		out = append(out, grammar.Value{})
	}
	return out
}

// valueToStringValue converts a Value into a StringValue best-effort for string operations
func valueToStringValue(v grammar.Value) grammar.StringValue {
	if v.DateTimeVal != nil || v.TimeVal != nil || v.Year != nil || v.Month != nil || v.DayOfMonth != nil || v.DayOfWeek != nil {
		s := grammar.StandardString(stringValueFromDate(v))
		return grammar.StringValue{StrVal: &s}
	}
	if v.Field != nil {
		return grammar.StringValue{Field: v.Field}
	}
	if v.StrVal != nil {
		return grammar.StringValue{StrVal: v.StrVal}
	}
	if v.Attribute != nil {
		return grammar.StringValue{Attribute: v.Attribute}
	}
	if v.StrCast != nil {
		return grammar.StringValue{StrCast: v.StrCast}
	}
	// Fallback: stringify
	s := grammar.StandardString(fmt.Sprint(v.GetValue()))
	return grammar.StringValue{StrVal: &s}
}

//nolint:revive // Cyclomatic complexity is acceptable here as the function is still readable and compact.
func resolveValue(v grammar.Value, claims Claims) any {
	if v.Attribute != nil {
		return resolveAttributeValue(v.Attribute, claims)
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
		// Preserve semantic types instead of stringifying so downstream type checks work.
		switch {
		case v.DateTimeVal != nil:
			return time.Time(*v.DateTimeVal)
		case v.TimeVal != nil:
			tv := grammar.TimeLiteralPattern(*v.TimeVal)
			return tv
		case v.Year != nil:
			return float64(time.Time(*v.Year).Year())
		case v.Month != nil:
			return float64(int(time.Time(*v.Month).Month()))
		case v.DayOfMonth != nil:
			return float64(time.Time(*v.DayOfMonth).Day())
		case v.DayOfWeek != nil:
			return float64(int(time.Time(*v.DayOfWeek).Weekday()))
		}
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

	if v.StrCast != nil {
		return fmt.Sprint(resolveValue(*v.StrCast, claims))
	}
	if v.NumCast != nil {
		x := resolveValue(*v.NumCast, claims)
		if f, ok := toFloat(x); ok {
			return f
		}
		if s, ok := x.(string); ok {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f
			}
		}
		return x
	}
	if v.BoolCast != nil {
		return castToBool(resolveValue(*v.BoolCast, claims))
	}

	if v.TimeCast != nil {
		inner := resolveValue(*v.TimeCast, claims)

		if t, ok := toDateTime(inner); ok {
			return t.Format("15:04:05")
		}

		if _, ok := toTimeOfDaySeconds(inner); ok {
			return fmt.Sprint(inner)
		}

		return fmt.Sprint(inner)
	}
	if v.DateTimeCast != nil {
		return fmt.Sprint(resolveValue(*v.DateTimeCast, claims))
	}
	if v.HexCast != nil {
		if hv, ok := normalizeHexAny(resolveValue(*v.HexCast, claims)); ok {
			return hv
		}
		return fmt.Sprint(resolveValue(*v.HexCast, claims))
	}

	return nil
}

func resolveStringItem(s grammar.StringValue, claims Claims) string {
	if s.Attribute != nil {
		return asString(resolveAttributeValue(s.Attribute, claims))
	}

	if s.StrVal != nil {
		return string(*s.StrVal)
	}

	if s.StrCast != nil {
		return fmt.Sprint(resolveValue(*s.StrCast, claims))
	}

	if s.Field != nil {
		return ""
	}
	return ""
}

func asString(v any) string {
	return fmt.Sprint(v)
}

// resolveAttributeValue resolves a grammar.AttributeValue to a concrete literal using claims/globals.
// It also normalizes common claim container shapes (e.g., single-element arrays from Keycloak).
func resolveAttributeValue(attr grammar.AttributeValue, claims Claims) any {
	m, ok := asStringMap(attr)
	if !ok {
		return nil
	}
	if c := m["CLAIM"]; c != "" {
		return fmt.Sprint(normalizeClaimScalar(claims[c]))
	}
	if g := m["GLOBAL"]; g != "" {
		if val, ok := resolveGlobalToken(g, claims); ok {
			return val
		}
	}
	return nil
}

// normalizeClaimScalar unwraps common container formats so operators see a scalar.
func normalizeClaimScalar(v any) any {
	switch val := v.(type) {
	case []any:
		if len(val) == 0 {
			return ""
		}
		return normalizeClaimScalar(val[0])
	case []string:
		if len(val) == 0 {
			return ""
		}
		return val[0]
	default:
		return v
	}
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

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	default:
		return 0, false
	}
}

func toTimeOfDaySeconds(v any) (int, bool) {
	s := strings.TrimSpace(fmt.Sprint(v))
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

func toDateTime(v any) (time.Time, bool) {
	switch val := v.(type) {
	case time.Time:
		return val, true
	case grammar.DateTimeLiteralPattern:
		return time.Time(val), true
	case *grammar.DateTimeLiteralPattern:
		if val == nil {
			return time.Time{}, false
		}
		return time.Time(*val), true
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
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

func orderedCmp(left, right grammar.Value, claims Claims, op string) bool {
	comparisonType, err := left.IsComparableTo(right)
	if err != nil {
		return false
	}

	switch comparisonType {
	case grammar.KindNumber:
		lv, lok := resolveNumberValue(left, claims)
		rv, rok := resolveNumberValue(right, claims)
		if !lok || !rok {
			return false
		}
		return compareFloats(lv, rv, op)
	case grammar.KindTime:
		lv, lok := resolveTimeValue(left, claims)
		rv, rok := resolveTimeValue(right, claims)
		if !lok || !rok {
			return false
		}
		return compareInts(lv, rv, op)
	case grammar.KindDateTime:
		lv, lok := resolveDateTimeValue(left, claims)
		rv, rok := resolveDateTimeValue(right, claims)
		if !lok || !rok {
			return false
		}
		return compareTimes(lv, rv, op)
	case grammar.KindHex:
		lv, lok := resolveHexValue(left, claims)
		rv, rok := resolveHexValue(right, claims)
		if !lok || !rok {
			return false
		}
		return compareHex(lv, rv, op)
	default:
		return false
	}
}

//nolint:revive // cyclomatic complexity is acceptable here as it is still readable
func eqCmp(left, right grammar.Value, claims Claims, negate bool) bool {
	comparisonType, err := left.IsComparableTo(right)
	if err != nil {
		return negate
	}

	equal := false
	switch comparisonType {
	case grammar.KindNumber:
		lv, lok := resolveNumberValue(left, claims)
		rv, rok := resolveNumberValue(right, claims)
		equal = lok && rok && lv == rv
	case grammar.KindDateTime:
		lv, lok := resolveDateTimeValue(left, claims)
		rv, rok := resolveDateTimeValue(right, claims)
		equal = lok && rok && lv.Equal(rv)
	case grammar.KindTime:
		lv, lok := resolveTimeValue(left, claims)
		rv, rok := resolveTimeValue(right, claims)
		equal = lok && rok && lv == rv
	case grammar.KindHex:
		lv, lok := resolveHexValue(left, claims)
		rv, rok := resolveHexValue(right, claims)
		equal = lok && rok && lv == rv
	case grammar.KindBool:
		lv, lok := resolveBoolValue(left, claims)
		rv, rok := resolveBoolValue(right, claims)
		equal = lok && rok && lv == rv
	case grammar.KindString:
		lv, lok := resolveStringValue(left, claims)
		rv, rok := resolveStringValue(right, claims)
		equal = lok && rok && lv == rv
	default:
		equal = false
	}

	if negate {
		return !equal
	}
	return equal
}

func resolveNumberValue(v grammar.Value, claims Claims) (float64, bool) {
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
		raw := resolveValue(*v.NumCast, claims)
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
		raw := resolveValue(v, claims)
		if f, ok := toFloat(raw); ok {
			return f, true
		}
	}
	return 0, false
}

func resolveDateTimeValue(v grammar.Value, claims Claims) (time.Time, bool) {
	switch {
	case v.DateTimeVal != nil:
		return time.Time(*v.DateTimeVal), true
	case v.DateTimeCast != nil:
		return toDateTime(resolveValue(*v.DateTimeCast, claims))
	default:
		return toDateTime(resolveValue(v, claims))
	}
}

func resolveTimeValue(v grammar.Value, claims Claims) (int, bool) {
	switch {
	case v.TimeVal != nil:
		return toTimeOfDaySeconds(*v.TimeVal)
	case v.TimeCast != nil:
		raw := resolveValue(*v.TimeCast, claims)
		if dt, ok := toDateTime(raw); ok {
			return dt.Hour()*3600 + dt.Minute()*60 + dt.Second(), true
		}
		return toTimeOfDaySeconds(raw)
	default:
		return toTimeOfDaySeconds(resolveValue(v, claims))
	}
}

func resolveHexValue(v grammar.Value, claims Claims) (string, bool) {
	switch {
	case v.HexVal != nil:
		return normalizeHexString(string(*v.HexVal))
	case v.HexCast != nil:
		return normalizeHexAny(resolveValue(*v.HexCast, claims))
	default:
		return normalizeHexAny(resolveValue(v, claims))
	}
}

func resolveBoolValue(v grammar.Value, claims Claims) (bool, bool) {
	switch {
	case v.Boolean != nil:
		return *v.Boolean, true
	case v.BoolCast != nil:
		return castToBool(resolveValue(*v.BoolCast, claims)), true
	default:
		return false, false
	}
}

func resolveStringValue(v grammar.Value, claims Claims) (string, bool) {
	switch {
	case v.StrVal != nil:
		return string(*v.StrVal), true
	case v.StrCast != nil:
		return fmt.Sprint(resolveValue(*v.StrCast, claims)), true
	case v.Attribute != nil:
		return asString(resolveAttributeValue(v.Attribute, claims)), true
	case v.Field != nil:
		return "", false
	default:
		return asString(resolveValue(v, claims)), true
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
	if op == "eq" || op == "ne" {
		if op == "eq" {
			return a == b
		}
		return a != b
	}

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
	case grammar.HexLiteralPattern:
		return normalizeHexString(string(h))
	case *grammar.HexLiteralPattern:
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

func stringValueFromDate(v grammar.Value) string {
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

// asStringMap attempts to normalize arbitrary map-like values into a
// map[string]string, best-effort. Useful when claims or attributes may be
// represented heterogeneously by upstream libraries.
func asStringMap(v any) (map[string]string, bool) {
	switch vv := v.(type) {
	case map[string]string:
		return vv, true
	case map[string]any:
		out := make(map[string]string, len(vv))
		for k, val := range vv {
			out[k] = fmt.Sprint(val)
		}
		return out, true
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var m map[string]any
		var jsonMarshaller = jsoniter.ConfigCompatibleWithStandardLibrary
		if err := jsonMarshaller.Unmarshal(b, &m); err != nil {
			return nil, false
		}
		out := make(map[string]string, len(m))
		for k, val := range m {
			out[k] = fmt.Sprint(val)
		}
		return out, true
	}
}
