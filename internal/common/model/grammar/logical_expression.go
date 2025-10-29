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

// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package grammar

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

type LogicalExpression struct {
	// And corresponds to the JSON schema field "$and".
	And []LogicalExpression `json:"$and,omitempty" yaml:"$and,omitempty" mapstructure:"$and,omitempty"`

	// Boolean corresponds to the JSON schema field "$boolean".
	Boolean *bool `json:"$boolean,omitempty" yaml:"$boolean,omitempty" mapstructure:"$boolean,omitempty"`

	// Contains corresponds to the JSON schema field "$contains".
	Contains StringItems `json:"$contains,omitempty" yaml:"$contains,omitempty" mapstructure:"$contains,omitempty"`

	// EndsWith corresponds to the JSON schema field "$ends-with".
	EndsWith StringItems `json:"$ends-with,omitempty" yaml:"$ends-with,omitempty" mapstructure:"$ends-with,omitempty"`

	// Eq corresponds to the JSON schema field "$eq".
	Eq ComparisonItems `json:"$eq,omitempty" yaml:"$eq,omitempty" mapstructure:"$eq,omitempty"`

	// Ge corresponds to the JSON schema field "$ge".
	Ge ComparisonItems `json:"$ge,omitempty" yaml:"$ge,omitempty" mapstructure:"$ge,omitempty"`

	// Gt corresponds to the JSON schema field "$gt".
	Gt ComparisonItems `json:"$gt,omitempty" yaml:"$gt,omitempty" mapstructure:"$gt,omitempty"`

	// Le corresponds to the JSON schema field "$le".
	Le ComparisonItems `json:"$le,omitempty" yaml:"$le,omitempty" mapstructure:"$le,omitempty"`

	// Lt corresponds to the JSON schema field "$lt".
	Lt ComparisonItems `json:"$lt,omitempty" yaml:"$lt,omitempty" mapstructure:"$lt,omitempty"`

	// Match corresponds to the JSON schema field "$match".
	Match []MatchExpression `json:"$match,omitempty" yaml:"$match,omitempty" mapstructure:"$match,omitempty"`

	// Ne corresponds to the JSON schema field "$ne".
	Ne ComparisonItems `json:"$ne,omitempty" yaml:"$ne,omitempty" mapstructure:"$ne,omitempty"`

	// Not corresponds to the JSON schema field "$not".
	Not *LogicalExpression `json:"$not,omitempty" yaml:"$not,omitempty" mapstructure:"$not,omitempty"`

	// Or corresponds to the JSON schema field "$or".
	Or []LogicalExpression `json:"$or,omitempty" yaml:"$or,omitempty" mapstructure:"$or,omitempty"`

	// Regex corresponds to the JSON schema field "$regex".
	Regex StringItems `json:"$regex,omitempty" yaml:"$regex,omitempty" mapstructure:"$regex,omitempty"`

	// StartsWith corresponds to the JSON schema field "$starts-with".
	StartsWith StringItems `json:"$starts-with,omitempty" yaml:"$starts-with,omitempty" mapstructure:"$starts-with,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *LogicalExpression) UnmarshalJSON(value []byte) error {
	type Plain LogicalExpression
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	if plain.And != nil && len(plain.And) < 2 {
		return fmt.Errorf("field %s length: must be >= %d", "$and", 2)
	}
	if plain.Match != nil && len(plain.Match) < 1 {
		return fmt.Errorf("field %s length: must be >= %d", "$match", 1)
	}
	if plain.Or != nil && len(plain.Or) < 2 {
		return fmt.Errorf("field %s length: must be >= %d", "$or", 2)
	}
	*j = LogicalExpression(plain)
	return nil
}

// EvaluateToExpression evaluates the logical expression tree and returns a goqu expression
// that can be used in SQL WHERE clauses.
//
// The method handles:
// - Comparison operations: $eq, $ne, $gt, $ge, $lt, $le
// - AND operations: all conditions must be true (uses goqu.And)
// - OR operations: at least one condition must be true (uses goqu.Or)
// - NOT operations: negates the result
// - Nested LogicalExpressions
//
// Returns:
//   - exp.Expression: The evaluated goqu expression
//   - error: An error if evaluation fails
func (le *LogicalExpression) EvaluateToExpression() (exp.Expression, error) {
	// Handle comparison operations
	if len(le.Eq) > 0 {
		return le.evaluateComparison(le.Eq, "$eq")
	}
	if len(le.Ne) > 0 {
		return le.evaluateComparison(le.Ne, "$ne")
	}
	if len(le.Gt) > 0 {
		return le.evaluateComparison(le.Gt, "$gt")
	}
	if len(le.Ge) > 0 {
		return le.evaluateComparison(le.Ge, "$ge")
	}
	if len(le.Lt) > 0 {
		return le.evaluateComparison(le.Lt, "$lt")
	}
	if len(le.Le) > 0 {
		return le.evaluateComparison(le.Le, "$le")
	}

	// Handle logical operations
	if len(le.And) > 0 {
		var expressions []exp.Expression
		for i, nestedExpr := range le.And {
			expr, err := nestedExpr.EvaluateToExpression()
			if err != nil {
				return nil, fmt.Errorf("error evaluating AND condition at index %d: %w", i, err)
			}
			expressions = append(expressions, expr)
		}
		return goqu.And(expressions...), nil
	}

	if len(le.Or) > 0 {
		var expressions []exp.Expression
		for i, nestedExpr := range le.Or {
			expr, err := nestedExpr.EvaluateToExpression()
			if err != nil {
				return nil, fmt.Errorf("error evaluating OR condition at index %d: %w", i, err)
			}
			expressions = append(expressions, expr)
		}
		return goqu.Or(expressions...), nil
	}

	if le.Not != nil {
		expr, err := le.Not.EvaluateToExpression()
		if err != nil {
			return nil, fmt.Errorf("error evaluating NOT condition: %w", err)
		}
		return goqu.L("NOT (?)", expr), nil
	}

	// Handle boolean literal
	if le.Boolean != nil {
		return goqu.L("?", *le.Boolean), nil
	}

	return nil, fmt.Errorf("logical expression has no valid operation")
}

// evaluateComparison evaluates a comparison operation with the given operands
func (le *LogicalExpression) evaluateComparison(operands []Value, operation string) (exp.Expression, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("comparison operation %s requires exactly 2 operands, got %d", operation, len(operands))
	}

	leftOperand := &operands[0]
	rightOperand := &operands[1]

	return HandleComparison(leftOperand, rightOperand, operation)
}

func ParseAASQLFieldToSQLColumn(field string) string {
	switch field {
	case "$sm#idShort":
		return "s.id_short"
	case "$sm#id":
		return "s.id"
	case "$sm#semanticId":
		return "semantic_id_reference_key.value"
	case "$sm#semanticId.type":
		return "semantic_id_reference.type"
	case "$sm#semanticId.keys[].value":
		return "semantic_id_reference_key.value"
	case "$sm#semanticId.keys[].type":
		return "semantic_id_reference_key.type"
	}

	if strings.HasPrefix(field, "$sm#semanticId.keys[") && strings.HasSuffix(field, "].value") {
		return "semantic_id_reference_key.value"
	}
	if strings.HasPrefix(field, "$sm#semanticId.keys[") && strings.HasSuffix(field, "].type") {
		return "semantic_id_reference_key.type"
	}

	return field
}

// HandleComparison builds a SQL comparison expression from two Value operands.
// It handles all combinations: field-to-field, field-to-value, value-to-field, and value-to-value.
func HandleComparison(leftOperand, rightOperand *Value, operation string) (exp.Expression, error) {

	// Convert both operands
	leftSQL, err := toSQLComponent(leftOperand, "left")
	if err != nil {
		return nil, err
	}

	rightSQL, err := toSQLComponent(rightOperand, "right")
	if err != nil {
		return nil, err
	}

	// Validate value-to-value comparisons have matching types
	if !leftOperand.IsField() && !rightOperand.IsField() {
		if leftOperand.GetValueType() != rightOperand.GetValueType() {
			return nil, fmt.Errorf("cannot compare different value types: %s and %s",
				leftOperand.GetValueType(), rightOperand.GetValueType())
		}
	}

	// Check if either operand is $sm#semanticId field
	isLeftShorthandSemanticId := isSemanticIdShorthandField(leftOperand)
	isRightShorthandSemanticId := isSemanticIdShorthandField(rightOperand)

	isLeftSpecificKeyValueSemanticId := isSemanticIdSpecificKeyValueField(leftOperand, false)
	isRightSpecificKeyValueSemanticId := isSemanticIdSpecificKeyValueField(rightOperand, false)

	isLeftSpecificKeyTypeSemanticId := isSemanticIdSpecificKeyValueField(leftOperand, true)
	isRightSpecificKeyTypeSemanticId := isSemanticIdSpecificKeyValueField(rightOperand, true)

	// Build the comparison expression
	comparisonExpr, err := buildComparisonExpression(leftSQL, rightSQL, operation)
	if err != nil {
		return nil, err
	}

	// If semantic_id is involved, add position = 0 constraint
	if isLeftShorthandSemanticId || isRightShorthandSemanticId {
		positionConstraint := goqu.I("semantic_id_reference_key.position").Eq(0)
		return goqu.And(comparisonExpr, positionConstraint), nil
	} else if (isLeftSpecificKeyValueSemanticId || isRightSpecificKeyValueSemanticId) || (isLeftSpecificKeyTypeSemanticId || isRightSpecificKeyTypeSemanticId) {

		operandToUse := leftOperand
		if isRightSpecificKeyValueSemanticId || isRightSpecificKeyTypeSemanticId {
			operandToUse = rightOperand
		}

		start, end := getStartAndEndIndicesOfBrackets(operandToUse)
		if isNotWildcardAndValidIndices(start, end) {
			positionStrOnError, position, err := getPositionAsInteger(operandToUse, start, end)
			if err == nil {
				positionConstraint := goqu.I("semantic_id_reference_key.position").Eq(position)
				return goqu.And(comparisonExpr, positionConstraint), nil
			} else {
				return nil, fmt.Errorf("invalid position in semanticId key field: %s", positionStrOnError)
			}
		}
	}
	return comparisonExpr, nil
}

func getPositionAsInteger(operandToUse *Value, start int, end int) (string, int, error) {
	positionStr := string(*operandToUse.Field)[start+1 : end]
	position, err := strconv.Atoi(positionStr)
	return positionStr, position, err
}

func isNotWildcardAndValidIndices(start, end int) bool {
	return start != -1 && end != -1 && start < end && (end-start > 1)
}

func getStartAndEndIndicesOfBrackets(operandToUse *Value) (int, int) {
	start := strings.Index(string(*operandToUse.Field), "[")
	end := strings.Index(string(*operandToUse.Field), "]")
	return start, end
}

func isSemanticIdShorthandField(operand *Value) bool {
	return operand.IsField() && operand.Field != nil && string(*operand.Field) == "$sm#semanticId"
}

func isSemanticIdSpecificKeyValueField(operand *Value, isTypeCheck bool) bool {
	suffix := "value"
	if isTypeCheck {
		suffix = "type"
	}
	if !operand.IsField() || operand.Field == nil {
		return false
	}
	field := string(*operand.Field)
	return strings.HasPrefix(field, "$sm#semanticId.keys[") && strings.HasSuffix(field, "]."+suffix)
}

func toSQLComponent(operand *Value, position string) (interface{}, error) {
	if operand.IsField() {
		if operand.Field == nil {
			return nil, fmt.Errorf("%s operand is not a valid field", position)
		}
		fieldName := string(*operand.Field)
		fieldName = ParseAASQLFieldToSQLColumn(fieldName)
		return goqu.I(fieldName), nil
	}
	return goqu.V(operand.GetValue()), nil
}

// buildComparisonExpression is a helper function to build comparison expressions
func buildComparisonExpression(left interface{}, right interface{}, operation string) (exp.Expression, error) {
	switch operation {
	case "$eq":
		return exp.NewLiteralExpression("? = ?", left, right), nil
	case "$ne":
		return exp.NewLiteralExpression("? != ?", left, right), nil
	case "$gt":
		return exp.NewLiteralExpression("? > ?", left, right), nil
	case "$ge":
		return exp.NewLiteralExpression("? >= ?", left, right), nil
	case "$lt":
		return exp.NewLiteralExpression("? < ?", left, right), nil
	case "$le":
		return exp.NewLiteralExpression("? <= ?", left, right), nil
	default:
		return nil, fmt.Errorf("unsupported comparison operation: %s", operation)
	}
}
