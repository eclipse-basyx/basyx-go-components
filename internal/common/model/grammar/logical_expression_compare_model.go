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

// Package grammar defines the data structures for representing logical expressions in the grammar model.
// Author: Martin ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	builder "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// EvaluateAssetAdministrationShellDescriptor evaluates the logical expression against the supplied
// AssetAdministrationShellDescriptor and returns the boolean result or any error encountered while
// normalizing the descriptor for evaluation.
func (le *LogicalExpression) EvaluateAssetAdministrationShellDescriptor(model model.AssetAdministrationShellDescriptor) (bool, error) {
	data, err := descriptorToMap(model)
	if err != nil {
		return false, err
	}
	return le.evaluateAgainstModel(data)
}

// EvaluateSubmodelDescriptor evaluates the logical expression directly against a SubmodelDescriptor.
// The helper wraps the descriptor in the same map structure that the evaluator uses so `$smdesc`
// paths can be resolved the same way as when the descriptor is embedded inside an AAS descriptor.
func (le *LogicalExpression) EvaluateSubmodelDescriptor(model model.SubmodelDescriptor) (bool, error) {
	data, err := descriptorToMap(model)
	if err != nil {
		return false, err
	}
	return le.evaluateAgainstModel(data)
}

// evaluateAgainstModel can be used for any type of model. You have to add a case in norm normalizeFieldReference function.
func (le *LogicalExpression) evaluateAgainstModel(data map[string]interface{}) (bool, error) {
	if len(le.Eq) > 0 {
		return le.evaluateModelComparison(le.Eq, "$eq", data)
	}
	if len(le.Ne) > 0 {
		return le.evaluateModelComparison(le.Ne, "$ne", data)
	}
	if len(le.Gt) > 0 {
		return le.evaluateModelComparison(le.Gt, "$gt", data)
	}
	if len(le.Ge) > 0 {
		return le.evaluateModelComparison(le.Ge, "$ge", data)
	}
	if len(le.Lt) > 0 {
		return le.evaluateModelComparison(le.Lt, "$lt", data)
	}
	if len(le.Le) > 0 {
		return le.evaluateModelComparison(le.Le, "$le", data)
	}
	if len(le.Regex) > 0 {
		return le.evaluateStringOperation(le.Regex, "$regex", data)
	}
	if len(le.Contains) > 0 {
		return le.evaluateStringOperation(le.Contains, "$contains", data)
	}
	if len(le.StartsWith) > 0 {
		return le.evaluateStringOperation(le.StartsWith, "$starts-with", data)
	}
	if len(le.EndsWith) > 0 {
		return le.evaluateStringOperation(le.EndsWith, "$ends-with", data)
	}
	if len(le.And) > 0 {
		for i, sub := range le.And {
			result, err := sub.evaluateAgainstModel(data)
			if err != nil {
				return false, fmt.Errorf("error evaluating AND condition at index %d: %w", i, err)
			}
			if !result {
				return false, nil
			}
		}
		return true, nil
	}
	if len(le.Or) > 0 {
		for i, sub := range le.Or {
			result, err := sub.evaluateAgainstModel(data)
			if err != nil {
				return false, fmt.Errorf("error evaluating OR condition at index %d: %w", i, err)
			}
			if result {
				return true, nil
			}
		}
		return false, nil
	}
	if le.Not != nil {
		result, err := le.Not.evaluateAgainstModel(data)
		if err != nil {
			return false, fmt.Errorf("error evaluating NOT condition: %w", err)
		}
		return !result, nil
	}
	if le.Boolean != nil {
		return *le.Boolean, nil
	}
	if len(le.Match) > 0 {
		return false, fmt.Errorf("match expressions are not supported for model evaluation yet")
	}
	return false, fmt.Errorf("logical expression has no valid operation")
}

func (le *LogicalExpression) evaluateModelComparison(operands []Value, operation string, data map[string]interface{}) (bool, error) {
	if len(operands) != 2 {
		return false, fmt.Errorf("comparison operation %s requires exactly 2 operands, got %d", operation, len(operands))
	}
	left := &operands[0]
	right := &operands[1]
	normalizeSemanticShorthand(left)
	normalizeSemanticShorthand(right)

	// has to be compatible
	expectedType, err := left.IsComparableTo(*right)
	if err != nil {
		return false, err
	}

	leftValues, err := resolveOperandValue(left, data)
	if err != nil {
		return false, err
	}
	rightValues, err := resolveOperandValue(right, data)
	if err != nil {
		return false, err
	}
	if len(leftValues) == 0 || len(rightValues) == 0 {
		return false, nil
	}

	for _, lv := range leftValues {
		for _, rv := range rightValues {
			match, err := compareValues(operation, lv, rv, expectedType)
			if err != nil {
				return false, err
			}
			if match {
				return true, nil
			}
		}
	}

	return false, nil
}

func (le *LogicalExpression) evaluateStringOperation(items []StringValue, operation string, data map[string]interface{}) (bool, error) {
	if len(items) != 2 {
		return false, fmt.Errorf("string operation %s requires exactly 2 operands, got %d", operation, len(items))
	}
	left, err := resolveStringValues(items[0], data)
	if err != nil {
		return false, err
	}
	right, err := resolveStringValues(items[1], data)
	if err != nil {
		return false, err
	}
	if len(left) == 0 || len(right) == 0 {
		return false, nil
	}

	matchFunc := func(a, b string) (bool, error) {
		switch operation {
		case "$contains":
			return strings.Contains(a, b), nil
		case "$starts-with":
			return strings.HasPrefix(a, b), nil
		case "$ends-with":
			return strings.HasSuffix(a, b), nil
		case "$regex":
			re, err := regexp.Compile(b)
			if err != nil {
				return false, err
			}
			return re.MatchString(a), nil
		default:
			return false, fmt.Errorf("unsupported string operation %s", operation)
		}
	}

	for _, l := range left {
		for _, r := range right {
			ok, err := matchFunc(l, r)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
	}
	return false, nil
}

func resolveOperandValue(op *Value, data map[string]interface{}) ([]interface{}, error) {
	if op == nil {
		return nil, fmt.Errorf("operand must not be nil")
	}
	if op.IsField() {
		if op.Field == nil {
			return nil, fmt.Errorf("field operand is missing value")
		}
		values, err := resolveFieldValues(data, string(*op.Field))
		if err != nil {
			return nil, err
		}
		return values, nil
	}
	if op.StrVal != nil {
		return []interface{}{string(*op.StrVal)}, nil
	}
	if op.NumVal != nil {
		return []interface{}{*op.NumVal}, nil
	}
	if op.Boolean != nil {
		return []interface{}{*op.Boolean}, nil
	}
	if op.HexVal != nil {
		return []interface{}{string(*op.HexVal)}, nil
	}
	if op.DateTimeVal != nil {
		return []interface{}{time.Time(*op.DateTimeVal)}, nil
	}
	if op.TimeVal != nil {
		return []interface{}{string(*op.TimeVal)}, nil
	}
	if op.DayOfWeek != nil {
		return []interface{}{*op.DayOfWeek}, nil
	}
	if op.DayOfMonth != nil {
		return []interface{}{*op.DayOfMonth}, nil
	}
	if op.Month != nil {
		return []interface{}{*op.Month}, nil
	}
	if op.Year != nil {
		return []interface{}{time.Time(*op.Year)}, nil
	}
	if op.Attribute != nil {
		return []interface{}{op.Attribute}, nil
	}
	if op.StrCast != nil {
		values, err := resolveOperandValue(op.StrCast, data)
		if err != nil {
			return nil, err
		}
		return castToStrings(values), nil
	}
	if op.HexCast != nil {
		values, err := resolveOperandValue(op.HexCast, data)
		if err != nil {
			return nil, err
		}
		return castToStrings(values), nil
	}
	if op.NumCast != nil {
		values, err := resolveOperandValue(op.NumCast, data)
		if err != nil {
			return nil, err
		}
		var out []interface{}
		for _, v := range values {
			num, ok := toFloat(v)
			if !ok {
				continue
			}
			out = append(out, num)
		}
		return out, nil
	}
	if op.BoolCast != nil {
		values, err := resolveOperandValue(op.BoolCast, data)
		if err != nil {
			return nil, err
		}
		var out []interface{}
		for _, v := range values {
			b, ok := coerceBool(v)
			if !ok {
				continue
			}
			out = append(out, b)
		}
		return out, nil
	}
	if op.TimeCast != nil {
		values, err := resolveOperandValue(op.TimeCast, data)
		if err != nil {
			return nil, err
		}
		return castToStrings(values), nil
	}
	if op.DateTimeCast != nil {
		values, err := resolveOperandValue(op.DateTimeCast, data)
		if err != nil {
			return nil, err
		}
		return castToStrings(values), nil
	}
	return nil, fmt.Errorf("unsupported operand type %s", op.GetValueType())
}

func resolveStringValues(op StringValue, data map[string]interface{}) ([]string, error) {
	if op.Field != nil {
		values, err := resolveFieldValues(data, string(*op.Field))
		if err != nil {
			return nil, err
		}
		return stringifyValues(values), nil
	}
	if op.StrVal != nil {
		return []string{string(*op.StrVal)}, nil
	}
	if op.Attribute != nil {
		return []string{fmt.Sprint(op.Attribute)}, nil
	}
	if op.StrCast != nil {
		values, err := resolveOperandValue(op.StrCast, data)
		if err != nil {
			return nil, err
		}
		return stringifyValues(values), nil
	}
	return nil, nil
}

func resolveFieldValues(data map[string]interface{}, rawField string) ([]interface{}, error) {
	normalized, err := normalizeFieldReference(rawField)
	if err != nil {
		return nil, err
	}
	tokens := builder.TokenizeField(normalized)
	values, err := gatherFieldValues(data, tokens)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	return values, nil
}

func normalizeFieldReference(field string) (string, error) {
	hashIdx := strings.Index(field, "#")
	if hashIdx < 0 {
		return "", fmt.Errorf("invalid field reference %s", field)
	}

	// Normalize common smdesc field casing to match JSON tags
	field = strings.ReplaceAll(field, "protocolinformation", "protocolInformation")
	prefix := field[:hashIdx]
	rest := strings.TrimPrefix(field[hashIdx+1:], ".")

	// TODO add more cases here if you have a different model type
	_, _ = fmt.Println(prefix)
	switch prefix {
	case "$aasdesc":
		return field, nil
	case "$smdesc":

		// normalize to $aasdesc, so rules with $aasdesc and $smdesc work
		base := "$aasdesc#submodelDescriptors[]"
		if rest == "" {
			return base, nil
		}
		return base + "." + rest, nil
	default:
		return "", fmt.Errorf("unsupported field prefix %s for model evaluation", prefix)
	}
}

func gatherFieldValues(current interface{}, tokens []builder.Token) ([]interface{}, error) {
	if current == nil {
		return nil, nil
	}
	if len(tokens) == 0 {
		return []interface{}{current}, nil
	}

	token := tokens[0]
	rest := tokens[1:]
	switch t := token.(type) {
	case builder.SimpleToken:
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, nil
		}
		next, exists := m[t.Name]
		if !exists || next == nil {
			return nil, nil
		}
		return gatherFieldValues(next, rest)
	case builder.ArrayToken:
		var arr []interface{}
		if m, ok := current.(map[string]interface{}); ok {
			next, exists := m[t.Name]
			if !exists || next == nil {
				return nil, nil
			}
			if cast, ok := next.([]interface{}); ok {
				arr = cast
			}
		} else if cast, ok := current.([]interface{}); ok {
			arr = cast
		} else {
			return nil, nil
		}
		if arr == nil {
			return nil, nil
		}
		if t.Index >= 0 {
			if t.Index >= len(arr) {
				return nil, nil
			}
			return gatherFieldValues(arr[t.Index], rest)
		}
		var out []interface{}
		for _, item := range arr {
			values, err := gatherFieldValues(item, rest)
			if err != nil {
				return nil, err
			}
			out = append(out, values...)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported token %T while resolving field %v", t, tokens)
	}
}

func descriptorToMap(subject any) (map[string]interface{}, error) {
	if subject == nil {
		return nil, fmt.Errorf("model must not be nil")
	}

	switch v := subject.(type) {
	case model.SubmodelDescriptor:
		mapped, err := marshalToMap(v)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"submodelDescriptors": []interface{}{mapped},
		}, nil
	case *model.SubmodelDescriptor:
		if v == nil {
			return nil, fmt.Errorf("model must not be nil")
		}
		mapped, err := marshalToMap(v)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"submodelDescriptors": []interface{}{mapped},
		}, nil
	default:
		return marshalToMap(subject)
	}
}

func marshalToMap(value any) (map[string]interface{}, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func compareValues(operation string, left, right interface{}, expectedType ComparisonKind) (bool, error) {
	switch operation {
	case "$eq":
		if left == nil && right == nil {
			return true, nil
		}
		if left == nil || right == nil {
			return false, nil
		}
		return compareEquality(operation, left, right, expectedType)
	case "$ne":
		eq, err := compareValues("$eq", left, right, expectedType)
		if err != nil {
			return false, nil
		}
		return !eq, nil
	case "$gt", "$ge", "$lt", "$le":
		return compareOrderedValues(operation, left, right, expectedType)
	default:
		return false, fmt.Errorf("unsupported comparison operation %s", operation)
	}
}

func compareOrderedValues(op string, left, right interface{}, expectedType ComparisonKind) (bool, error) {
	// Honor expected types when known to avoid unexpected coercions.
	switch expectedType {
	case KindNumber:
		lf, lok := toFloat(left)
		rf, rok := toFloat(right)
		if !lok || !rok {
			return false, nil
		}
		return compareFloat(op, lf, rf)
	case KindTime:
		lt, lok := toTimeOfDaySeconds(left)
		rt, rok := toTimeOfDaySeconds(right)
		if !lok || !rok {
			return false, nil
		}
		return compareInt(op, lt, rt)
	case KindDateTime:
		lt, lok := toDateTime(left)
		rt, rok := toDateTime(right)
		if !lok || !rok {
			return false, nil
		}
		return compareTime(op, lt, rt)
	case KindString, KindHex:
		// Ordered comparisons are not defined for string/hex
		return false, nil
	case KindBool:
		return false, nil
	}

	// Fallback to permissive coercion when type is unknown.
	if lf, lok := toFloat(left); lok {
		if rf, rok := toFloat(right); rok {
			return compareFloat(op, lf, rf)
		}
	}
	if lt, lok := toTimeOfDaySeconds(left); lok {
		if rt, rok := toTimeOfDaySeconds(right); rok {
			return compareInt(op, lt, rt)
		}
	}
	if lt, lok := toDateTime(left); lok {
		if rt, rok := toDateTime(right); rok {
			return compareTime(op, lt, rt)
		}
	}
	return false, fmt.Errorf("cannot compare %T (%v) and %T (%v) with operator %s", left, left, right, right, op)
}

func compareEquality(op string, left, right interface{}, expectedType ComparisonKind) (bool, error) {
	switch expectedType {
	case KindBool:
		lb, lok := left.(bool)
		rb, rok := right.(bool)
		if !lok || !rok {
			return false, fmt.Errorf("cannot compare non-bool values with %s", op)
		}
		return lb == rb, nil
	case KindNumber:
		lf, lok := toFloat(left)
		rf, rok := toFloat(right)
		if !lok || !rok {
			return false, fmt.Errorf("cannot compare non-number values with %s", op)
		}
		return lf == rf, nil
	case KindDateTime:
		lt, lok := toDateTime(left)
		rt, rok := toDateTime(right)
		if !lok || !rok {
			return false, fmt.Errorf("cannot compare non-datetime values with %s", op)
		}
		return lt.Equal(rt), nil
	case KindTime:
		lt, lok := toTimeOfDaySeconds(left)
		rt, rok := toTimeOfDaySeconds(right)
		if !lok || !rok {
			return false, fmt.Errorf("cannot compare non-time values with %s", op)
		}
		return lt == rt, nil
	case KindHex, KindString:
		return fmt.Sprint(left) == fmt.Sprint(right), nil
	}

	// Fallback to permissive coercion if type is unknown.
	if lb, lok := left.(bool); lok {
		if rb, rok := right.(bool); rok {
			return lb == rb, nil
		}
	}
	if lf, lok := toFloat(left); lok {
		if rf, rok := toFloat(right); rok {
			return lf == rf, nil
		}
	}
	if lt, lok := toDateTime(left); lok {
		if rt, rok := toDateTime(right); rok {
			return lt.Equal(rt), nil
		}
	}
	return fmt.Sprint(left) == fmt.Sprint(right), nil
}

func compareFloat(op string, lf, rf float64) (bool, error) {
	switch op {
	case "$gt":
		return lf > rf, nil
	case "$ge":
		return lf >= rf, nil
	case "$lt":
		return lf < rf, nil
	case "$le":
		return lf <= rf, nil
	default:
		return false, fmt.Errorf("unsupported op %s for float comparison", op)
	}
}

func compareInt(op string, l, r int) (bool, error) {
	switch op {
	case "$gt":
		return l > r, nil
	case "$ge":
		return l >= r, nil
	case "$lt":
		return l < r, nil
	case "$le":
		return l <= r, nil
	default:
		return false, fmt.Errorf("unsupported op %s for int comparison", op)
	}
}

func compareTime(op string, l, r time.Time) (bool, error) {
	switch op {
	case "$gt":
		return l.After(r), nil
	case "$ge":
		return l.After(r) || l.Equal(r), nil
	case "$lt":
		return l.Before(r), nil
	case "$le":
		return l.Before(r) || l.Equal(r), nil
	default:
		return false, fmt.Errorf("unsupported op %s for datetime comparison", op)
	}
}

func castToStrings(values []interface{}) []interface{} {
	out := make([]interface{}, 0, len(values))
	for _, v := range values {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

func stringifyValues(values []interface{}) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

func coerceBool(value interface{}) (bool, bool) {
	if value == nil {
		return false, false
	}
	if b, ok := value.(bool); ok {
		return b, true
	}
	if f, ok := toFloat(value); ok {
		return f != 0, true
	}
	s := strings.TrimSpace(fmt.Sprint(value))
	switch strings.ToLower(s) {
	case "true", "1", "yes":
		return true, true
	case "false", "0", "no":
		return false, true
	default:
		return false, false
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
