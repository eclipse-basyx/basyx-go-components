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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	jsoniter "github.com/json-iterator/go"
)

func resolveGlobalToken(name string, now time.Time) (string, bool) {
	switch strings.ToUpper(name) {
	case "UTCNOW":
		return now.UTC().Format(time.RFC3339), true
	case "LOCALNOW":
		return now.In(time.Local).Format(time.RFC3339), true
	case "CLIENTNOW":
		return now.Format(time.RFC3339), true
	case "ANONYMOUS":
		return "ANONYMOUS", true
	default:
		return "", false
	}
}

func evalLE(le grammar.LogicalExpression, claims Claims, now time.Time) bool {
	if le.Boolean != nil {
		return *le.Boolean
	}

	if len(le.Gt) == 2 {
		return numCmp(resolveValue(le.Gt[0], claims, now), resolveValue(le.Gt[1], claims, now), "gt")
	}
	if len(le.Ge) == 2 {
		return numCmp(resolveValue(le.Ge[0], claims, now), resolveValue(le.Ge[1], claims, now), "ge")
	}
	if len(le.Lt) == 2 {
		return numCmp(resolveValue(le.Lt[0], claims, now), resolveValue(le.Lt[1], claims, now), "lt")
	}
	if len(le.Le) == 2 {
		return numCmp(resolveValue(le.Le[0], claims, now), resolveValue(le.Le[1], claims, now), "le")
	}

	if len(le.Eq) == 2 {
		return fmt.Sprint(resolveValue(le.Eq[0], claims, now)) == fmt.Sprint(resolveValue(le.Eq[1], claims, now))
	}
	if len(le.Ne) == 2 {
		return fmt.Sprint(resolveValue(le.Ne[0], claims, now)) != fmt.Sprint(resolveValue(le.Ne[1], claims, now))
	}

	if len(le.Regex) == 2 {
		hay := asString(resolveStringItem(le.Regex[0], claims, now))
		pat := asString(resolveStringItem(le.Regex[1], claims, now))
		re, err := regexp.Compile(pat)
		if err != nil {
			return false
		}
		return re.MatchString(hay)
	}
	if len(le.Contains) == 2 {
		hay := asString(resolveStringItem(le.Contains[0], claims, now))
		needle := asString(resolveStringItem(le.Contains[1], claims, now))
		return strings.Contains(hay, needle)
	}
	if len(le.StartsWith) == 2 {
		hay := asString(resolveStringItem(le.StartsWith[0], claims, now))
		prefix := asString(resolveStringItem(le.StartsWith[1], claims, now))
		return strings.HasPrefix(hay, prefix)
	}
	if len(le.EndsWith) == 2 {
		hay := asString(resolveStringItem(le.EndsWith[0], claims, now))
		suffix := asString(resolveStringItem(le.EndsWith[1], claims, now))
		return strings.HasSuffix(hay, suffix)
	}

	if len(le.And) >= 2 {
		for _, sub := range le.And {
			if !evalLE(sub, claims, now) {
				return false
			}
		}
		return true
	}
	if len(le.Or) >= 2 {
		for _, sub := range le.Or {
			if evalLE(sub, claims, now) {
				return true
			}
		}
		return false
	}
	if le.Not != nil {
		return !evalLE(*le.Not, claims, now)
	}

	if len(le.Match) > 0 {
		for _, m := range le.Match {
			if !evalMatch(m, claims, now) {
				return false
			}
		}
		return true
	}
	return false
}

// adaptLEForBackend takes a logical expression and partially evaluates parts
// that depend only on CLAIM or GLOBAL attributes into $boolean true/false.
// The remaining expression is returned for backend evaluation. The second
// return value indicates whether the entire expression became a pure boolean
// expression (i.e., consists only of true/false after reduction).
func adaptLEForBackend(le grammar.LogicalExpression, claims Claims, now time.Time) (grammar.LogicalExpression, bool) {
	// Boolean literal stays as-is
	if le.Boolean != nil {
		return le, true
	}

	// Comparison operators: try to replace attributes with literals, and
	// if both operands become literals, reduce to $boolean using evalLE.
	reduceCmp := func(items []grammar.Value, op string) (grammar.LogicalExpression, bool) {
		if len(items) != 2 {
			return le, false
		}

		left := replaceAttribute(items[0], claims, now)
		right := replaceAttribute(items[1], claims, now)

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
			if evalLE(tmp, claims, now) {
				b = true
			}
			return grammar.LogicalExpression{Boolean: &b}, true
		}
		return out, false
	}

	// Handle comparisons
	switch {
	case len(le.Eq) == 2:
		return reduceCmp(le.Eq, "$eq")
	case len(le.Ne) == 2:
		return reduceCmp(le.Ne, "$ne")
	case len(le.Gt) == 2:
		return reduceCmp(le.Gt, "$gt")
	case len(le.Ge) == 2:
		return reduceCmp(le.Ge, "$ge")
	case len(le.Lt) == 2:
		return reduceCmp(le.Lt, "$lt")
	case len(le.Le) == 2:
		return reduceCmp(le.Le, "$le")
	case len(le.Regex) == 2:
		return reduceCmp(stringItemsToValues(le.Regex), "$regex")
	case len(le.Contains) == 2:
		return reduceCmp(stringItemsToValues(le.Contains), "$contains")
	case len(le.StartsWith) == 2:
		return reduceCmp(stringItemsToValues(le.StartsWith), "$starts-with")
	case len(le.EndsWith) == 2:
		return reduceCmp(stringItemsToValues(le.EndsWith), "$ends-with")
	}

	// Logical: AND / OR
	if len(le.And) > 0 {
		if len(le.And) == 1 {
			return adaptLEForBackend(le.And[0], claims, now)
		}
		out := grammar.LogicalExpression{}
		anyUnknown := false
		// Short-circuit: if any child becomes false => whole AND is false.
		for _, sub := range le.And {
			t, onlyBool := adaptLEForBackend(sub, claims, now)
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
			return adaptLEForBackend(le.Or[0], claims, now)
		}
		out := grammar.LogicalExpression{}
		anyUnknown := false
		// Short-circuit: if any child becomes true => whole OR is true.
		for _, sub := range le.Or {
			t, onlyBool := adaptLEForBackend(sub, claims, now)
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
		t, onlyBool := adaptLEForBackend(*le.Not, claims, now)
		if onlyBool && t.Boolean != nil {
			b := !*t.Boolean
			return grammar.LogicalExpression{Boolean: &b}, true
		}
		return grammar.LogicalExpression{Not: &t}, false
	}

	// $match: try to reduce nested matches
	if len(le.Match) > 0 {
		// Semantics of $match in eval: AND over children
		out := grammar.LogicalExpression{}
		anyUnknown := false
		for _, m := range le.Match {
			if t, isBool := adaptMatchForBackend(m, claims, now); isBool {
				if t.Boolean != nil && !*t.Boolean {
					b := false
					return grammar.LogicalExpression{Boolean: &b}, true
				}
				// true is neutral for AND; omit it
				continue
			}
			out.Match = append(out.Match, m)
			anyUnknown = true
		}
		if !anyUnknown {
			b := true
			return grammar.LogicalExpression{Boolean: &b}, true
		}
		return out, false
	}

	// Unknown or unsupported -> cannot fully decide here
	return le, false
}

// adaptMatchForBackend reduces a MatchExpression similarly to adaptLEForBackend.
func adaptMatchForBackend(me grammar.MatchExpression, claims Claims, now time.Time) (grammar.MatchExpression, bool) {
	if me.Boolean != nil {
		return me, true
	}
	reduceCmp := func(items []grammar.Value, op string) (grammar.MatchExpression, bool) {
		if len(items) != 2 {
			return me, false
		}
		left := replaceAttribute(items[0], claims, now)
		right := replaceAttribute(items[1], claims, now)
		out := grammar.MatchExpression{}
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
		if isLiteral(left) && isLiteral(right) {
			b := false
			tmp := grammar.LogicalExpression{}
			switch op {
			case "$eq":
				tmp.Eq = out.Eq
			case "$ne":
				tmp.Ne = out.Ne
			case "$gt":
				tmp.Gt = out.Gt
			case "$ge":
				tmp.Ge = out.Ge
			case "$lt":
				tmp.Lt = out.Lt
			case "$le":
				tmp.Le = out.Le
			case "$regex":
				tmp.Regex = out.Regex
			case "$contains":
				tmp.Contains = out.Contains
			case "$starts-with":
				tmp.StartsWith = out.StartsWith
			case "$ends-with":
				tmp.EndsWith = out.EndsWith
			}
			if evalLE(tmp, claims, now) {
				b = true
			}
			return grammar.MatchExpression{Boolean: &b}, true
		}
		return out, false
	}

	switch {
	case len(me.Eq) == 2:
		return reduceCmp(me.Eq, "$eq")
	case len(me.Ne) == 2:
		return reduceCmp(me.Ne, "$ne")
	case len(me.Gt) == 2:
		return reduceCmp(me.Gt, "$gt")
	case len(me.Ge) == 2:
		return reduceCmp(me.Ge, "$ge")
	case len(me.Lt) == 2:
		return reduceCmp(me.Lt, "$lt")
	case len(me.Le) == 2:
		return reduceCmp(me.Le, "$le")
	case len(me.Regex) == 2:
		return reduceCmp(stringItemsToValues(me.Regex), "$regex")
	case len(me.Contains) == 2:
		return reduceCmp(stringItemsToValues(me.Contains), "$contains")
	case len(me.StartsWith) == 2:
		return reduceCmp(stringItemsToValues(me.StartsWith), "$starts-with")
	case len(me.EndsWith) == 2:
		return reduceCmp(stringItemsToValues(me.EndsWith), "$ends-with")
	case len(me.Match) > 0:
		allBool := true
		out := grammar.MatchExpression{}
		for _, sub := range me.Match {
			t, onlyBool := adaptMatchForBackend(sub, claims, now)
			out.Match = append(out.Match, t)
			if !onlyBool {
				allBool = false
			}
		}
		if allBool {
			accum := true
			for _, sub := range out.Match {
				if sub.Boolean == nil || !*sub.Boolean {
					accum = false
					break
				}
			}
			return grammar.MatchExpression{Boolean: &accum}, true
		}
		return out, false
	}
	return me, false
}

// replaceAttribute resolves a Value that is deterministic from CLAIM/GLOBAL attributes
// (including via casts) to a literal when possible. Values that reference $field
// remain untouched because they cannot be evaluated without backend context.
func replaceAttribute(v grammar.Value, claims Claims, now time.Time) grammar.Value {
	if valueContainsAttribute(v) && !valueContainsField(v) {
		// Use existing resolver to get a concrete value
		resolved := resolveValue(v, claims, now)
		if lit, ok := literalValueFromAny(resolved); ok {
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

// isConstantLiteralVal returns true if the value is a direct literal constant
// (not a field, not an attribute), so comparisons with it can be reduced when the
// other side is resolvable.
func isConstantLiteralVal(v grammar.Value) bool {
	if v.Field != nil || v.Attribute != nil {
		return false
	}
	if v.StrVal != nil || v.NumVal != nil || v.Boolean != nil || v.DateTimeVal != nil || v.TimeVal != nil || v.Year != nil || v.Month != nil || v.DayOfMonth != nil || v.DayOfWeek != nil || v.HexVal != nil {
		return true
	}
	// Casts produce runtime literals via resolveValue; treat as not a constant here.
	return false
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
	case string:
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

func evalMatch(me grammar.MatchExpression, claims Claims, now time.Time) bool {
	if me.Boolean != nil {
		return *me.Boolean
	}
	if len(me.Gt) == 2 {
		return numCmp(resolveValue(me.Gt[0], claims, now), resolveValue(me.Gt[1], claims, now), "gt")
	}
	if len(me.Ge) == 2 {
		return numCmp(resolveValue(me.Ge[0], claims, now), resolveValue(me.Ge[1], claims, now), "ge")
	}
	if len(me.Lt) == 2 {
		return numCmp(resolveValue(me.Lt[0], claims, now), resolveValue(me.Lt[1], claims, now), "lt")
	}
	if len(me.Le) == 2 {
		return numCmp(resolveValue(me.Le[0], claims, now), resolveValue(me.Le[1], claims, now), "le")
	}
	if len(me.Eq) == 2 {
		return fmt.Sprint(resolveValue(me.Eq[0], claims, now)) == fmt.Sprint(resolveValue(me.Eq[1], claims, now))
	}
	if len(me.Ne) == 2 {
		return fmt.Sprint(resolveValue(me.Ne[0], claims, now)) != fmt.Sprint(resolveValue(me.Ne[1], claims, now))
	}
	if len(me.Regex) == 2 {
		hay := asString(resolveStringItem(me.Regex[0], claims, now))
		pat := asString(resolveStringItem(me.Regex[1], claims, now))
		re, err := regexp.Compile(pat)
		if err != nil {
			return false
		}
		return re.MatchString(hay)
	}
	if len(me.Contains) == 2 {
		hay := asString(resolveStringItem(me.Contains[0], claims, now))
		needle := asString(resolveStringItem(me.Contains[1], claims, now))
		return strings.Contains(hay, needle)
	}
	if len(me.StartsWith) == 2 {
		hay := asString(resolveStringItem(me.StartsWith[0], claims, now))
		prefix := asString(resolveStringItem(me.StartsWith[1], claims, now))
		return strings.HasPrefix(hay, prefix)
	}
	if len(me.EndsWith) == 2 {
		hay := asString(resolveStringItem(me.EndsWith[0], claims, now))
		suffix := asString(resolveStringItem(me.EndsWith[1], claims, now))
		return strings.HasSuffix(hay, suffix)
	}
	if len(me.Match) > 0 {
		for _, sub := range me.Match {
			if !evalMatch(sub, claims, now) {
				return false
			}
		}
		return true
	}
	return false
}

func resolveValue(v grammar.Value, claims Claims, now time.Time) any {

	if v.Attribute != nil {
		return resolveAttributeValue(v.Attribute, claims, now)
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
		return stringValueFromDate(v)
	}
	if v.HexVal != nil {
		return string(*v.HexVal)
	}

	if v.Field != nil {
		return ""
	}

	if v.StrCast != nil {
		return fmt.Sprint(resolveValue(*v.StrCast, claims, now))
	}
	if v.NumCast != nil {
		x := resolveValue(*v.NumCast, claims, now)
		if f, ok := toFloat(x); ok {
			return f
		}
		return x
	}
	if v.BoolCast != nil {
		return castToBool(resolveValue(*v.BoolCast, claims, now))
	}

	if v.TimeCast != nil {
		inner := resolveValue(*v.TimeCast, claims, now)

		if t, ok := toDateTime(inner); ok {
			return t.Format("15:04:05")
		}

		if _, ok := toTimeOfDaySeconds(inner); ok {
			return fmt.Sprint(inner)
		}

		return fmt.Sprint(inner)
	}
	if v.DateTimeCast != nil {

		return fmt.Sprint(resolveValue(*v.DateTimeCast, claims, now))
	}
	if v.HexCast != nil {
		return fmt.Sprint(resolveValue(*v.HexCast, claims, now))
	}

	return nil
}

func resolveStringItem(s grammar.StringValue, claims Claims, now time.Time) string {
	if s.Attribute != nil {
		return asString(resolveAttributeValue(s.Attribute, claims, now))
	}

	if s.StrVal != nil {
		return string(*s.StrVal)
	}

	if s.StrCast != nil {
		return fmt.Sprint(resolveValue(*s.StrCast, claims, now))
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
func resolveAttributeValue(attr grammar.AttributeValue, claims Claims, now time.Time) any {
	m, ok := asStringMap(attr)
	if !ok {
		return nil
	}
	if c := m["CLAIM"]; c != "" {
		return normalizeClaimScalar(claims[c])
	}
	if g := m["GLOBAL"]; g != "" {
		if val, ok := resolveGlobalToken(g, now); ok {
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
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
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

func numCmp(a, b any, op string) bool {
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			switch op {
			case "gt":
				return af > bf
			case "ge":
				return af >= bf
			case "lt":
				return af < bf
			case "le":
				return af <= bf
			default:
				return false
			}
		}
	}
	if as, aok := toTimeOfDaySeconds(a); aok {
		if bs, bok := toTimeOfDaySeconds(b); bok {
			switch op {
			case "gt":
				return as > bs
			case "ge":
				return as >= bs
			case "lt":
				return as < bs
			case "le":
				return as <= bs
			default:
				return false
			}
		}
	}

	if at, aok := toDateTime(a); aok {
		if bt, bok := toDateTime(b); bok {
			switch op {
			case "gt":
				return at.After(bt)
			case "ge":
				return at.After(bt) || at.Equal(bt)
			case "lt":
				return at.Before(bt)
			case "le":
				return at.Before(bt) || at.Equal(bt)
			default:
				return false
			}
		}
	}

	return false
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
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, false
		}
		out := make(map[string]string, len(m))
		for k, val := range m {
			out[k] = fmt.Sprint(val)
		}
		return out, true
	}
}
