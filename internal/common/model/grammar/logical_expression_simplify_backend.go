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
	"math"
	"math/big"
	"reflect"
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

// ClaimValue preserves the JSON type of a value resolved from a verified token.
// It lets casts enforce the stricter claim rules without changing global or
// backend value handling.
type ClaimValue struct {
	Value any
}

// SimplifyOptions controls how backend simplification treats comparisons.
type SimplifyOptions struct {
	// EnableImplicitCasts wraps field operands in casts to match the other operand's type.
	EnableImplicitCasts bool
}

// DefaultSimplifyOptions returns the default settings used by SimplifyForBackendFilter.
func DefaultSimplifyOptions() SimplifyOptions {
	return SimplifyOptions{EnableImplicitCasts: true}
}

// SimplifyDecision describes the result of SimplifyForBackendFilter.
//
// - SimplifyTrue / SimplifyFalse: the expression is fully decidable without backend context.
// - SimplifyIndeterminate: evaluation failed and the complete expression must deny access.
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
	// SimplifyIndeterminate - runtime evaluation is indeterminate and fails closed
	SimplifyIndeterminate
)

func decisionFromBool(b bool) SimplifyDecision {
	if b {
		return SimplifyTrue
	}
	return SimplifyFalse
}

func invalidLogicalExpression() (LogicalExpression, SimplifyDecision) {
	return LogicalExpression{Indeterminate: true}, SimplifyIndeterminate
}

func invalidMatchExpression() (MatchExpression, SimplifyDecision) {
	return MatchExpression{Indeterminate: true}, SimplifyIndeterminate
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
//   - SimplifyIndeterminate: evaluation failed and the returned expression is false
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
	if le.Indeterminate {
		return le, SimplifyIndeterminate
	}
	// Boolean literal stays as-is.
	if le.Boolean != nil {
		return le, decisionFromBool(*le.Boolean)
	}

	if le.BoolCast != nil {
		if valueContainsField(*le.BoolCast) {
			return le, SimplifyUndecided
		}
		if b, ok := resolveBoolValue(Value{BoolCast: le.BoolCast}, resolve); ok {
			return LogicalExpression{Boolean: &b}, decisionFromBool(b)
		}
		return invalidLogicalExpression()
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
		case SimplifyIndeterminate:
			return invalidLogicalExpression()
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
		anyFalse := false
		anyInvalid := false
		for _, sub := range le.And {
			t, decision := sub.SimplifyForBackendFilterWithOptions(resolve, opts)
			switch decision {
			case SimplifyFalse:
				anyFalse = true
			case SimplifyTrue:
			case SimplifyIndeterminate:
				anyInvalid = true
			case SimplifyUndecided:
				out.And = append(out.And, t)
			}
		}
		if anyFalse {
			b := false
			return LogicalExpression{Boolean: &b}, SimplifyFalse
		}
		if anyInvalid {
			if len(out.And) == 0 {
				return invalidLogicalExpression()
			}
			out.And = append(out.And, LogicalExpression{Indeterminate: true})
		}
		if len(out.And) == 0 {
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
		anyTrue := false
		anyInvalid := false
		for _, sub := range le.Or {
			t, decision := sub.SimplifyForBackendFilterWithOptions(resolve, opts)
			switch decision {
			case SimplifyTrue:
				anyTrue = true
			case SimplifyFalse:
			case SimplifyIndeterminate:
				anyInvalid = true
			case SimplifyUndecided:
				out.Or = append(out.Or, t)
			}
		}
		if anyTrue {
			b := true
			return LogicalExpression{Boolean: &b}, SimplifyTrue
		}
		if anyInvalid {
			if len(out.Or) == 0 {
				return invalidLogicalExpression()
			}
			out.Or = append(out.Or, LogicalExpression{Indeterminate: true})
		}
		if len(out.Or) == 0 {
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
		case SimplifyIndeterminate:
			return invalidLogicalExpression()
		default:
			return LogicalExpression{Not: &t}, SimplifyUndecided
		}
	}

	return le, SimplifyUndecided
}

// SimplifyForBackendFilterNoResolver runs SimplifyForBackendFilter with a no-op resolver.
// Attribute operands are invalid, while field-dependent expressions remain undecided.
func (le LogicalExpression) SimplifyForBackendFilterNoResolver() (LogicalExpression, SimplifyDecision) {
	return le.SimplifyForBackendFilter(func(AttributeValue) any { return nil })
}

func simplifyMatchExpressionsForBackendFilter(match []MatchExpression, resolve AttributeResolver, opts SimplifyOptions) ([]MatchExpression, SimplifyDecision) {
	if len(match) == 0 {
		return nil, SimplifyUndecided
	}

	out := make([]MatchExpression, 0, len(match))
	anyFalse := false
	anyInvalid := false
	for _, m := range match {
		t, decision := simplifyMatchExpressionForBackendFilter(m, resolve, opts)
		switch decision {
		case SimplifyFalse:
			anyFalse = true
		case SimplifyTrue:
		case SimplifyIndeterminate:
			anyInvalid = true
		case SimplifyUndecided:
			out = append(out, t)
		}
	}
	if anyFalse {
		return nil, SimplifyFalse
	}
	if anyInvalid {
		if len(out) == 0 {
			return nil, SimplifyIndeterminate
		}
		out = append(out, MatchExpression{Indeterminate: true})
	}
	if len(out) == 0 {
		return nil, SimplifyTrue
	}
	return out, SimplifyUndecided
}

func simplifyMatchExpressionForBackendFilter(me MatchExpression, resolve AttributeResolver, opts SimplifyOptions) (MatchExpression, SimplifyDecision) {
	if me.Indeterminate {
		return me, SimplifyIndeterminate
	}
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
		case SimplifyIndeterminate:
			return invalidMatchExpression()
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

	left, right, attributesResolved := resolveComparisonAttributes(items[0], items[1], op, resolve)
	if !attributesResolved {
		invalid, decision := invalidMatchExpression()
		return &invalid, decision
	}
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
			invalid, decision := invalidMatchExpression()
			return &invalid, decision
		}
	}

	if opts.EnableImplicitCasts {
		left = WrapCastAroundField(left, comparisonType)
		right = WrapCastAroundField(right, comparisonType)
	}

	out, leOut := buildMatchComparisonExpressions(op, left, right)

	if isLiteral(left) && isLiteral(right) ||
		(!valueContainsField(left) && !valueContainsField(right) && (valueContainsAttribute(left) || valueContainsAttribute(right))) {
		b, valid := evalComparisonOnly(leOut, resolve)
		if !valid {
			invalid, decision := invalidMatchExpression()
			return &invalid, decision
		}
		return &MatchExpression{Boolean: &b}, decisionFromBool(b)
	}

	return &out, SimplifyUndecided
}

func buildMatchComparisonExpressions(op string, left, right Value) (MatchExpression, LogicalExpression) {
	values := []Value{left, right}
	stringValues := []StringValue{valueToStringValue(left), valueToStringValue(right)}
	out := MatchExpression{}
	leOut := LogicalExpression{}
	switch op {
	case "$eq":
		out.Eq, leOut.Eq = values, values
	case "$ne":
		out.Ne, leOut.Ne = values, values
	case "$gt":
		out.Gt, leOut.Gt = values, values
	case "$ge":
		out.Ge, leOut.Ge = values, values
	case "$lt":
		out.Lt, leOut.Lt = values, values
	case "$le":
		out.Le, leOut.Le = values, values
	case "$regex":
		out.Regex, leOut.Regex = stringValues, stringValues
	case "$contains":
		out.Contains, leOut.Contains = stringValues, stringValues
	case "$starts-with":
		out.StartsWith, leOut.StartsWith = stringValues, stringValues
	case "$ends-with":
		out.EndsWith, leOut.EndsWith = stringValues, stringValues
	}
	return out, leOut
}

func reduceCmp(le LogicalExpression, resolve AttributeResolver, items []Value, op string, opts SimplifyOptions) (*LogicalExpression, SimplifyDecision) {
	if len(items) != 2 {
		return &le, SimplifyUndecided
	}

	left, right, attributesResolved := resolveComparisonAttributes(items[0], items[1], op, resolve)
	if !attributesResolved {
		invalid, decision := invalidLogicalExpression()
		return &invalid, decision
	}
	isStringOp := isStringComparisonOperator(op)
	if !isStringOp {
		left, right = convertEnumLiteralIfNeeded(left, right)
	}
	comparisonType, err := comparisonKind(left, right, isStringOp)
	if err != nil {
		invalid, decision := invalidLogicalExpression()
		return &invalid, decision
	}

	if opts.EnableImplicitCasts {
		left = WrapCastAroundField(left, comparisonType)
		right = WrapCastAroundField(right, comparisonType)
	}

	out := comparisonExpression(op, left, right)

	if isLiteral(left) && isLiteral(right) ||
		(!valueContainsField(left) && !valueContainsField(right) && (valueContainsAttribute(left) || valueContainsAttribute(right))) {
		b, valid := evalComparisonOnly(out, resolve)
		if !valid {
			invalid, decision := invalidLogicalExpression()
			return &invalid, decision
		}
		if b {
			return &LogicalExpression{Boolean: &b}, SimplifyTrue
		}
		return &LogicalExpression{Boolean: &b}, SimplifyFalse
	}

	return &out, SimplifyUndecided
}

func resolveComparisonAttributes(
	left Value,
	right Value,
	op string,
	resolve AttributeResolver,
) (Value, Value, bool) {
	directClaimPathContains := op == "$contains" && valueIsDirectClaimPath(left)
	if !directClaimPathContains {
		left = replaceAttribute(left, resolve)
	}
	right = replaceAttribute(right, resolve)
	if valueContainsAttribute(right) || !directClaimPathContains && valueContainsAttribute(left) {
		return left, right, false
	}
	return left, right, true
}

func isStringComparisonOperator(op string) bool {
	return op == "$regex" || op == "$contains" || op == "$starts-with" || op == "$ends-with"
}

func comparisonKind(left, right Value, stringOperator bool) (ComparisonKind, error) {
	if stringOperator {
		return KindString, nil
	}
	return left.IsComparableTo(right)
}

func comparisonExpression(op string, left, right Value) LogicalExpression {
	values := []Value{left, right}
	switch op {
	case "$eq":
		return LogicalExpression{Eq: values}
	case "$ne":
		return LogicalExpression{Ne: values}
	case "$gt":
		return LogicalExpression{Gt: values}
	case "$ge":
		return LogicalExpression{Ge: values}
	case "$lt":
		return LogicalExpression{Lt: values}
	case "$le":
		return LogicalExpression{Le: values}
	case "$regex":
		return LogicalExpression{Regex: stringComparisonValues(left, right)}
	case "$contains":
		return LogicalExpression{Contains: stringComparisonValues(left, right)}
	case "$starts-with":
		return LogicalExpression{StartsWith: stringComparisonValues(left, right)}
	case "$ends-with":
		return LogicalExpression{EndsWith: stringComparisonValues(left, right)}
	default:
		return LogicalExpression{}
	}
}

func stringComparisonValues(left, right Value) []StringValue {
	return []StringValue{valueToStringValue(left), valueToStringValue(right)}
}

func evalComparisonOnly(le LogicalExpression, resolve AttributeResolver) (bool, bool) {
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
		hay, hayOK := resolveStringItem(le.Regex[0], resolve)
		pat, patOK := resolveStringItem(le.Regex[1], resolve)
		if !hayOK || !patOK {
			return false, false
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return false, false
		}
		return re.MatchString(hay), true
	}
	if len(le.Contains) == 2 {
		if attributeIsClaimPath(le.Contains[0].Attribute) {
			return claimPathContains(le.Contains[0], le.Contains[1], resolve)
		}
		hay, hayOK := resolveStringItem(le.Contains[0], resolve)
		needle, needleOK := resolveStringItem(le.Contains[1], resolve)
		if !hayOK || !needleOK {
			return false, false
		}
		return strings.Contains(hay, needle), true
	}
	if len(le.StartsWith) == 2 {
		hay, hayOK := resolveStringItem(le.StartsWith[0], resolve)
		prefix, prefixOK := resolveStringItem(le.StartsWith[1], resolve)
		if !hayOK || !prefixOK {
			return false, false
		}
		return strings.HasPrefix(hay, prefix), true
	}
	if len(le.EndsWith) == 2 {
		hay, hayOK := resolveStringItem(le.EndsWith[0], resolve)
		suffix, suffixOK := resolveStringItem(le.EndsWith[1], resolve)
		if !hayOK || !suffixOK {
			return false, false
		}
		return strings.HasSuffix(hay, suffix), true
	}

	return false, false
}

func convertEnumLiteralIfNeeded(left, right Value) (Value, Value) {
	if field, _ := extractFieldOperandAndCast(&left); field != nil && right.StrVal != nil {
		if converted, ok, isEnumField := convertEnumLiteralForField(*field, right); ok {
			right = converted
		} else if isEnumField {
			// Keep comparisons against enum-backed fields SQL-safe even when
			// implicit casts are disabled and the enum literal is invalid.
			left = WrapCastAroundField(left, KindString)
		}
		return left, right
	}
	if field, _ := extractFieldOperandAndCast(&right); field != nil && left.StrVal != nil {
		if converted, ok, isEnumField := convertEnumLiteralForField(*field, left); ok {
			left = converted
		} else if isEnumField {
			right = WrapCastAroundField(right, KindString)
		}
	}
	return left, right
}

type enumColumnKind int

const (
	enumColumnUnknown enumColumnKind = iota
	enumColumnDataTypeDefXSD
	enumColumnReferenceType
	enumColumnKeyType
	enumColumnAssetKind
)

func convertEnumLiteralForField(field Value, lit Value) (Value, bool, bool) {
	if field.Field == nil || lit.StrVal == nil {
		return Value{}, false, false
	}
	fieldName := string(*field.Field)
	f := ModelStringPattern(fieldName)
	resolved, err := ResolveScalarFieldToSQL(&f)
	if err != nil {
		return Value{}, false, false
	}
	columnKind := enumKindForResolvedColumn(resolved.Column)
	if columnKind == enumColumnUnknown {
		return Value{}, false, false
	}
	converted, ok := convertEnumLiteralValue(columnKind, string(*lit.StrVal))
	if !ok {
		return Value{}, false, true
	}
	return converted, true, true
}

func enumKindForResolvedColumn(column string) enumColumnKind {
	col := strings.ToLower(strings.TrimSpace(column))
	switch {
	case col == "value_type" || strings.HasSuffix(col, ".value_type"):
		return enumColumnDataTypeDefXSD
	case strings.HasSuffix(col, ".asset_kind"):
		return enumColumnAssetKind
	case strings.HasSuffix(col, "_key.type"):
		return enumColumnKeyType
	case strings.HasSuffix(col, ".type"):
		return enumColumnReferenceType
	default:
		return enumColumnUnknown
	}
}

func convertEnumLiteralValue(columnKind enumColumnKind, literal string) (Value, bool) {
	switch columnKind {
	case enumColumnDataTypeDefXSD:
		if enumVal, ok := stringification.DataTypeDefXSDFromString(literal); ok {
			return enumValueToValue(enumVal)
		}
	case enumColumnAssetKind:
		if enumVal, ok := stringification.AssetKindFromString(literal); ok {
			return enumValueToValue(enumVal)
		}
	case enumColumnReferenceType:
		if enumVal, ok := stringification.ReferenceTypesFromString(literal); ok {
			return enumValueToValue(enumVal)
		}
	case enumColumnKeyType:
		if enumVal, ok := stringification.KeyTypesFromString(literal); ok {
			return enumValueToValue(enumVal)
		}
	}
	return Value{}, false
}

func enumValueToValue(enumVal interface{}) (Value, bool) {
	switch v := enumVal.(type) {
	case int:
		f := float64(v)
		return Value{NumVal: &f}, true
	default:
		rv := reflect.ValueOf(enumVal)
		if rv.Kind() != reflect.Int {
			return Value{}, false
		}
		f := float64(rv.Int())
		return Value{NumVal: &f}, true
	}
}

func replaceAttribute(v Value, resolve AttributeResolver) Value {
	if valueContainsField(v) {
		return v
	}

	if valueContainsAttribute(v) {
		resolved := resolveValue(v, resolve)
		if isStructuredValue(resolved) {
			return v
		}
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

func isStructuredValue(value any) bool {
	if claim, ok := value.(ClaimValue); ok {
		value = claim.Value
	}
	switch value.(type) {
	case []any, []string, map[string]any, map[string]string:
		return true
	default:
		return false
	}
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
		v.DayOfMonth,
		v.DayOfWeek,
		v.HexCast,
		v.Month,
		v.NumCast,
		v.StrCast,
		v.TimeCast,
		v.Year,
	}
}

func isLiteral(v Value) bool {
	return v.IsValue() && !valueContainsField(v) && v.Attribute == nil
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
	if claim, ok := x.(ClaimValue); ok {
		return literalClaimValueWithHint(claim.Value, hint)
	}
	switch hint {
	case KindDateTime:
		if dt, ok := toDateTime(x); ok {
			pat := DateTimeLiteralPattern(dt)
			return Value{DateTimeVal: &pat}, true
		}
	case KindTime:
		s := strings.Trim(fmt.Sprint(x), xsdWhitespace)
		if _, ok := parseXSDTime(s); ok {
			pat := TimeLiteralPattern(s)
			return Value{TimeVal: &pat}, true
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

func literalClaimValueWithHint(value any, hint ComparisonKind) (Value, bool) {
	switch hint {
	case KindString:
		text, ok := value.(string)
		if !ok {
			return Value{}, false
		}
		return literalValueFromAny(text)
	case KindNumber:
		if number, ok := strictClaimNumber(value); ok {
			return literalValueFromAny(number)
		}
	case KindBool:
		if boolean, ok := value.(bool); ok {
			return literalValueFromAny(boolean)
		}
	case KindDateTime:
		if text, ok := value.(string); ok {
			if parsed, valid := toDateTime(text); valid {
				return literalValueFromAny(parsed)
			}
		}
	case KindTime:
		if text, ok := value.(string); ok {
			if _, valid := parseXSDTime(text); valid {
				pattern := TimeLiteralPattern(strings.Trim(text, xsdWhitespace))
				return Value{TimeVal: &pattern}, true
			}
		}
	case KindHex:
		if text, ok := value.(string); ok {
			if normalized, valid := normalizeHexString(text); valid {
				pattern := HexLiteralPattern(normalized)
				return Value{HexVal: &pattern}, true
			}
		}
	}
	return Value{}, false
}

func strictClaimNumber(value any) (float64, bool) {
	const maxSafeInteger = float64(1<<53 - 1)

	var parsed float64
	switch number := value.(type) {
	case json.Number:
		var err error
		parsed, err = number.Float64()
		if err != nil {
			return 0, false
		}
	case float64:
		parsed = number
	case float32:
		parsed = float64(number)
	case int:
		parsed = float64(number)
	case int32:
		parsed = float64(number)
	case int64:
		parsed = float64(number)
	default:
		return 0, false
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	if math.Trunc(parsed) == parsed && math.Abs(parsed) > maxSafeInteger {
		return 0, false
	}
	return parsed, true
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

func resolveDateTimeLiteral(v Value, resolve AttributeResolver) any {
	switch {
	case v.DateTimeVal != nil:
		return time.Time(*v.DateTimeVal)
	case v.TimeVal != nil:
		return TimeLiteralPattern(*v.TimeVal)
	}
	if part, ok := resolveDatePart(v, resolve); ok {
		return part
	}
	return nil
}

// isDatePart reports whether a value is a $year, $month, $dayOfMonth, or $dayOfWeek operator.
func isDatePart(v Value) bool {
	return v.Year != nil || v.Month != nil || v.DayOfMonth != nil || v.DayOfWeek != nil
}

// resolveDatePart evaluates $year, $month, $dayOfMonth, or $dayOfWeek in the value's own
// timezone, as XSD defines them, matching the SQL generated for field operands.
func resolveDatePart(v Value, resolve AttributeResolver) (float64, bool) {
	operand, part, ok := datePartOperand(v)
	if !ok {
		return 0, false
	}
	t, ok := resolveDateTimeInstant(*operand, resolve)
	if !ok {
		return 0, false
	}
	return extractDatePart(t, part)
}

// datePartOperand returns the operand of a date-part operator and its PostgreSQL EXTRACT field.
func datePartOperand(v Value) (*Value, string, bool) {
	switch {
	case v.Year != nil:
		return v.Year, "YEAR", true
	case v.Month != nil:
		return v.Month, "MONTH", true
	case v.DayOfMonth != nil:
		return v.DayOfMonth, "DAY", true
	case v.DayOfWeek != nil:
		return v.DayOfWeek, "DOW", true
	default:
		return nil, "", false
	}
}

func resolveCastValue(v Value, resolve AttributeResolver) any {
	if literal, isClaim := resolveClaimCast(v, resolve); isClaim {
		return literal
	}
	switch {
	case v.StrCast != nil:
		return resolveStrCast(*v.StrCast, resolve)
	case v.NumCast != nil:
		return resolveNumCast(resolveValue(*v.NumCast, resolve))
	case v.BoolCast != nil:
		return valueOrNil(boolOperandValue(resolveValue(*v.BoolCast, resolve)))
	case v.TimeCast != nil:
		return resolveTimeCastOperand(*v.TimeCast, resolve)
	case v.DateTimeCast != nil:
		return valueOrNil(toDateTime(resolveValue(*v.DateTimeCast, resolve)))
	case v.HexCast != nil:
		hex, ok := normalizeHexAny(resolveValue(*v.HexCast, resolve))
		return valueOrNil(HexLiteralPattern(hex), ok)
	default:
		return nil
	}
}

// resolveClaimCast casts a claim operand with the strict claim typing; isClaim reports whether
// the operand was a claim.
func resolveClaimCast(v Value, resolve AttributeResolver) (literal any, isClaim bool) {
	kind, operand, ok := castOperand(v)
	if !ok {
		return nil, false
	}
	claim, isClaim := resolveValue(*operand, resolve).(ClaimValue)
	if !isClaim {
		return nil, false
	}
	value, valid := literalClaimValueWithHint(claim.Value, kind)
	if !valid {
		return nil, true
	}
	return resolveValue(value, resolve), true
}

// resolveStrCast casts an operand to xs:string. Time globals such as LOCALNOW are xs:dateTime
// values and take their XPath canonical form.
func resolveStrCast(operand Value, resolve AttributeResolver) any {
	raw := resolveValue(operand, resolve)
	if !isDateTimeAttributeOperand(operand) {
		return valueOrNil(stringOperandValue(raw))
	}
	instant, ok := toDateTime(raw)
	if !ok {
		return nil
	}
	return valueOrNil(canonicalDateTimeString(instant))
}

// resolveNumCast casts a resolved operand to a number. A date-time or time, and any value that
// is not a number, yields nil, like the PostgreSQL cast in SQL generation.
func resolveNumCast(raw any) any {
	if isTemporalValue(raw) {
		return nil
	}
	if f, ok := toFloat(raw); ok {
		return f
	}
	return nil
}

// valueOrNil returns value when ok is true and nil otherwise.
func valueOrNil[T any](value T, ok bool) any {
	if !ok {
		return nil
	}
	return value
}

// resolveTimeCastOperand casts an operand to xs:time: a date-time keeps its clock time and
// timezone, and a string must be in the xs:time lexical space.
func resolveTimeCastOperand(v Value, resolve AttributeResolver) any {
	raw := resolveValue(v, resolve)
	if isDateTimeOperand(v) {
		if t, ok := toDateTime(raw); ok {
			return TimeLiteralPattern(canonicalTimeString(timeOfDateTime(t)))
		}
		return nil
	}
	text := strings.Trim(fmt.Sprint(raw), xsdWhitespace)
	if _, ok := parseXSDTime(text); ok {
		return TimeLiteralPattern(text)
	}
	return nil
}

// isTemporalValue reports whether a resolved operand is an xs:dateTime or xs:time value, which
// XPath cannot cast to a number.
func isTemporalValue(value any) bool {
	switch value.(type) {
	case time.Time, DateTimeLiteralPattern, *DateTimeLiteralPattern, TimeLiteralPattern:
		return true
	default:
		return false
	}
}

func castOperand(value Value) (ComparisonKind, *Value, bool) {
	switch {
	case value.StrCast != nil:
		return KindString, value.StrCast, true
	case value.NumCast != nil:
		return KindNumber, value.NumCast, true
	case value.BoolCast != nil:
		return KindBool, value.BoolCast, true
	case value.TimeCast != nil:
		return KindTime, value.TimeCast, true
	case value.DateTimeCast != nil:
		return KindDateTime, value.DateTimeCast, true
	case value.HexCast != nil:
		return KindHex, value.HexCast, true
	default:
		return KindUnknown, nil, false
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
		return resolveDateTimeLiteral(v, resolve)
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

func resolveStringItem(s StringValue, resolve AttributeResolver) (string, bool) {
	if s.Attribute != nil {
		value := resolve(s.Attribute)
		if claim, ok := value.(ClaimValue); ok {
			text, valid := claim.Value.(string)
			return text, valid
		}
		return stringOperandValue(value)
	}
	if s.StrVal != nil {
		return string(*s.StrVal), true
	}
	if s.StrCast != nil {
		return resolveStringValue(Value{StrCast: s.StrCast}, resolve)
	}
	if s.Field != nil {
		return "", false
	}
	return "", false
}

func claimPathContains(haystack, needle StringValue, resolve AttributeResolver) (bool, bool) {
	resolved := resolve(haystack.Attribute)
	claim, ok := resolved.(ClaimValue)
	if !ok {
		return false, false
	}
	items, ok := claimStringArrayItems(claim.Value)
	if !ok {
		return false, false
	}
	wanted, ok := resolveStringItem(needle, resolve)
	if !ok {
		return false, false
	}
	found := false
	for _, item := range items {
		text, stringItem := item.(string)
		if !stringItem {
			return false, false
		}
		if text == wanted {
			found = true
		}
	}
	return found, true
}

func claimStringArrayItems(value any) ([]any, bool) {
	if items, ok := value.([]any); ok {
		return items, true
	}
	stringItems, ok := value.([]string)
	if !ok {
		return nil, false
	}
	items := make([]any, len(stringItems))
	for index, item := range stringItems {
		items[index] = item
	}
	return items, true
}

func boolOperandValue(value any) (bool, bool) {
	if boolean, ok := value.(bool); ok {
		return boolean, true
	}
	text, ok := value.(string)
	if !ok {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "true", "1", "yes", "y", "on":
		return true, true
	case "false", "0", "no", "n", "off":
		return false, true
	default:
		return false, false
	}
}

func orderedCmp(left, right Value, resolve AttributeResolver, op string) (bool, bool) {
	comparisonType, err := left.IsComparableTo(right)
	if err != nil {
		return false, false
	}

	switch comparisonType {
	case KindNumber:
		lv, lok := resolveNumberValue(left, resolve)
		rv, rok := resolveNumberValue(right, resolve)
		if !lok || !rok {
			return false, false
		}
		return compareFloats(lv, rv, op), true
	case KindTime:
		lv, lok := resolveTimeValue(left, resolve)
		rv, rok := resolveTimeValue(right, resolve)
		if !lok || !rok {
			return false, false
		}
		return compareTimes(lv, rv, op), true
	case KindDateTime:
		lv, lok := resolveDateTimeValue(left, resolve)
		rv, rok := resolveDateTimeValue(right, resolve)
		if !lok || !rok {
			return false, false
		}
		return compareTimes(lv, rv, op), true
	case KindHex:
		lv, lok := resolveHexValue(left, resolve)
		rv, rok := resolveHexValue(right, resolve)
		if !lok || !rok {
			return false, false
		}
		return compareHex(lv, rv, op), true
	default:
		return false, false
	}
}

func eqCmp(left, right Value, resolve AttributeResolver, negate bool) (bool, bool) {
	comparisonType, err := left.IsComparableTo(right)
	if err != nil {
		return false, false
	}
	if containsStringArray(left, resolve) || containsStringArray(right, resolve) {
		return false, false
	}

	equal := false
	valid := false
	switch comparisonType {
	case KindNumber:
		lv, lok := resolveNumberValue(left, resolve)
		rv, rok := resolveNumberValue(right, resolve)
		valid = lok && rok
		equal = valid && lv == rv
	case KindDateTime:
		lv, lok := resolveDateTimeValue(left, resolve)
		rv, rok := resolveDateTimeValue(right, resolve)
		valid = lok && rok
		equal = valid && lv.Equal(rv)
	case KindTime:
		lv, lok := resolveTimeValue(left, resolve)
		rv, rok := resolveTimeValue(right, resolve)
		valid = lok && rok
		equal = valid && lv.Equal(rv)
	case KindHex:
		lv, lok := resolveHexValue(left, resolve)
		rv, rok := resolveHexValue(right, resolve)
		valid = lok && rok
		equal = valid && lv == rv
	case KindBool:
		lv, lok := resolveBoolValue(left, resolve)
		rv, rok := resolveBoolValue(right, resolve)
		valid = lok && rok
		equal = valid && lv == rv
	case KindString:
		lv, lok := resolveStringValue(left, resolve)
		rv, rok := resolveStringValue(right, resolve)
		valid = lok && rok
		equal = valid && lv == rv
	default:
		return false, false
	}
	if !valid {
		return false, false
	}

	if negate {
		return !equal, true
	}
	return equal, true
}

func resolveNumberValue(v Value, resolve AttributeResolver) (float64, bool) {
	switch {
	case v.NumVal != nil:
		return *v.NumVal, true
	case isDatePart(v):
		return resolveDatePart(v, resolve)
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
	t, ok := resolveDateTimeInstant(v, resolve)
	return truncateToMicroseconds(t), ok
}

// resolveDateTimeInstant resolves a date-time operand without truncating it to microseconds.
func resolveDateTimeInstant(v Value, resolve AttributeResolver) (time.Time, bool) {
	switch {
	case v.DateTimeVal != nil:
		return time.Time(*v.DateTimeVal), true
	case v.DateTimeCast != nil:
		return toDateTime(resolveCastValue(v, resolve))
	default:
		return toDateTime(resolveValue(v, resolve))
	}
}

// resolveTimeValue returns an xs:time operand as an instant on the XPath reference date.
func resolveTimeValue(v Value, resolve AttributeResolver) (time.Time, bool) {
	var t time.Time
	var ok bool
	switch raw := resolveValue(v, resolve).(type) {
	case TimeLiteralPattern:
		t, ok = parseTimeLiteral(string(raw))
	default:
		t, ok = parseXSDTime(fmt.Sprint(raw))
	}
	return truncateToMicroseconds(t), ok
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
		boolean, ok := resolveCastValue(v, resolve).(bool)
		return boolean, ok
	default:
		return false, false
	}
}

func resolveStringValue(v Value, resolve AttributeResolver) (string, bool) {
	switch {
	case v.StrVal != nil:
		return string(*v.StrVal), true
	case v.StrCast != nil:
		text, ok := resolveCastValue(v, resolve).(string)
		return text, ok
	case v.Attribute != nil:
		return stringOperandValue(resolve(v.Attribute))
	case v.Field != nil:
		return "", false
	default:
		return stringOperandValue(resolveValue(v, resolve))
	}
}

func containsStringArray(value Value, resolve AttributeResolver) bool {
	_, isArray, _ := stringArrayOperand(value, resolve)
	return isArray
}

func stringArrayOperand(value Value, resolve AttributeResolver) ([]string, bool, bool) {
	raw := resolveValue(value, resolve)
	switch items := raw.(type) {
	case []string:
		return items, true, len(items) > 0
	case []any:
		values := make([]string, len(items))
		for index, item := range items {
			text, ok := item.(string)
			if !ok {
				return nil, true, false
			}
			values[index] = text
		}
		return values, true, len(values) > 0
	case nil:
		return nil, false, true
	default:
		return nil, false, true
	}
}

func stringOperandValue(value any) (string, bool) {
	switch temporal := value.(type) {
	case time.Time:
		return canonicalDateTimeString(temporal)
	case DateTimeLiteralPattern:
		return canonicalDateTimeString(time.Time(temporal))
	case TimeLiteralPattern:
		t, ok := parseTimeLiteral(string(temporal))
		if !ok {
			return "", false
		}
		return canonicalTimeString(t), true
	}
	if value == nil || isStructuredValue(value) {
		return "", false
	}
	return fmt.Sprint(value), true
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

// hexWhitespace lists the characters trimmed around a value cast to hex, in Go and in SQL.
const hexWhitespace = " \t\n\v\f\r"

func normalizeHexString(raw string) (string, bool) {
	s := strings.ToUpper(strings.Trim(raw, hexWhitespace))
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
	}
	if part, ok := resolveDatePart(v, func(AttributeValue) any { return nil }); ok {
		return strconv.FormatFloat(part, 'f', -1, 64)
	}
	return ""
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
		return parseXSDDateTime(fmt.Sprint(value))
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
