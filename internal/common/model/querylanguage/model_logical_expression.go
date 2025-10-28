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

// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )
package querylanguage

import (
	"encoding/json"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

type LogicalExpression struct {
	And []Condition `json:"$and,omitempty"`
	Or  []Condition `json:"$or,omitempty"`
	Not []Condition `json:"$not,omitempty"`
}

func (le *LogicalExpression) IsCondition() bool {
	return true
}

func (le *LogicalExpression) GetConditionType() string {
	return "LogicalExpression"
}

// UnmarshalJSON implements custom unmarshalling for LogicalExpression
func (le *LogicalExpression) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal into a map to see which logical operator we have
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Handle each logical operator
	for op, value := range raw {
		switch op {
		case "$and":
			if err := le.unmarshalConditions(value, &le.And); err != nil {
				return fmt.Errorf("error unmarshalling $and: %w", err)
			}
		case "$or":
			if err := le.unmarshalConditions(value, &le.Or); err != nil {
				return fmt.Errorf("error unmarshalling $or: %w", err)
			}
		case "$not":
			if err := le.unmarshalConditions(value, &le.Not); err != nil {
				return fmt.Errorf("error unmarshalling $not: %w", err)
			}
		default:
			return fmt.Errorf("unknown logical operator: %s", op)
		}
	}

	return nil
}

// unmarshalConditions handles the conversion of JSON array to Condition slice
func (le *LogicalExpression) unmarshalConditions(data []byte, conditions *[]Condition) error {
	// Parse as array of raw JSON objects
	var conditionArray []json.RawMessage
	if err := json.Unmarshal(data, &conditionArray); err != nil {
		return fmt.Errorf("logical expression value must be an array: %w", err)
	}

	for i, conditionData := range conditionArray {
		// Try to determine if it's a Comparison or LogicalExpression
		var rawCondition map[string]interface{}
		if err := json.Unmarshal(conditionData, &rawCondition); err != nil {
			return fmt.Errorf("failed to unmarshal condition %d: %w", i, err)
		}

		// Check if it's a comparison operation ($eq, $ne, etc.)
		isComparison := false
		for key := range rawCondition {
			if key == "$eq" || key == "$ne" || key == "$gt" || key == "$ge" || key == "$lt" || key == "$le" {
				isComparison = true
				break
			}
		}

		// Check if it's a logical expression ($and, $or, $not)
		isLogical := false
		for key := range rawCondition {
			if key == "$and" || key == "$or" || key == "$not" {
				isLogical = true
				break
			}
		}

		if isComparison {
			var comparison Comparison
			if err := json.Unmarshal(conditionData, &comparison); err != nil {
				return fmt.Errorf("failed to unmarshal comparison at index %d: %w", i, err)
			}
			*conditions = append(*conditions, &comparison)
		} else if isLogical {
			var logicalExpr LogicalExpression
			if err := json.Unmarshal(conditionData, &logicalExpr); err != nil {
				return fmt.Errorf("failed to unmarshal logical expression at index %d: %w", i, err)
			}
			*conditions = append(*conditions, &logicalExpr)
		} else {
			return fmt.Errorf("unknown condition type at index %d", i)
		}
	}

	return nil
}

// GetLogicalOperationType returns the type of logical operation
func (le *LogicalExpression) GetLogicalOperationType() string {
	if len(le.And) > 0 {
		return "$and"
	}
	if len(le.Or) > 0 {
		return "$or"
	}
	if len(le.Not) > 0 {
		return "$not"
	}
	return "unknown"
}

// GetConditions returns the conditions for the active logical operation
func (le *LogicalExpression) GetConditions() []Condition {
	switch le.GetLogicalOperationType() {
	case "$and":
		return le.And
	case "$or":
		return le.Or
	case "$not":
		return le.Not
	default:
		return nil
	}
}

// EvaluateToExpression iteratively evaluates the logical expression tree
// and returns a goqu expression that can be used in SQL WHERE clauses.
//
// The method handles:
// - AND operations: all conditions must be true (uses goqu.And)
// - OR operations: at least one condition must be true (uses goqu.Or)
// - NOT operations: negates the result using L() with SQL NOT
// - Nested LogicalExpressions and Comparisons
//
// Returns:
//   - exp.Expression: The evaluated goqu expression
//   - error: An error if evaluation fails
func (le *LogicalExpression) EvaluateToExpression() (exp.Expression, error) {
	operationType := le.GetLogicalOperationType()
	conditions := le.GetConditions()

	if len(conditions) == 0 {
		return nil, fmt.Errorf("logical expression has no conditions")
	}

	// Evaluate each condition to get expressions
	var expressions []exp.Expression

	for i, condition := range conditions {
		var expr exp.Expression
		var err error

		switch condition.GetConditionType() {
		case "Comparison":
			expr, err = le.evaluateComparison(condition.(*Comparison))
			if err != nil {
				return nil, fmt.Errorf("error evaluating comparison at index %d: %w", i, err)
			}

		case "LogicalExpression":
			// Recursive call for nested logical expressions
			nestedLogicalExpr := condition.(*LogicalExpression)
			expr, err = nestedLogicalExpr.EvaluateToExpression()
			if err != nil {
				return nil, fmt.Errorf("error evaluating nested logical expression at index %d: %w", i, err)
			}

		default:
			return nil, fmt.Errorf("unsupported condition type: %s at index %d", condition.GetConditionType(), i)
		}

		expressions = append(expressions, expr)
	}

	// Apply the logical operation to all expressions
	switch operationType {
	case "$and":
		if len(expressions) == 1 {
			return expressions[0], nil
		}
		return goqu.And(expressions...), nil

	case "$or":
		if len(expressions) == 1 {
			return expressions[0], nil
		}
		return goqu.Or(expressions...), nil

	case "$not":
		if len(expressions) != 1 {
			return nil, fmt.Errorf("NOT operation requires exactly 1 condition, got %d", len(expressions))
		}
		// Since goqu doesn't have a direct NOT function, we'll use L() with SQL NOT
		return goqu.L("NOT (?)", expressions[0]), nil

	default:
		return nil, fmt.Errorf("unsupported logical operation type: %s", operationType)
	}
}

// evaluateComparison evaluates a Comparison condition to a goqu expression
func (le *LogicalExpression) evaluateComparison(comparison *Comparison) (exp.Expression, error) {
	operation := comparison.GetOperationType()
	operationObj := comparison.GetOperation()

	if operationObj == nil {
		return nil, fmt.Errorf("comparison operation is nil")
	}

	operands := operationObj.GetOperands()
	if len(operands) != 2 {
		return nil, fmt.Errorf("comparison operation requires exactly 2 operands, got %d", len(operands))
	}

	leftOperand := operands[0]
	rightOperand := operands[1]

	// Handle different operand type combinations
	if leftOperand.GetOperandType() == "$field" && rightOperand.GetOperandType() != "$field" {
		return HandleFieldToValueComparison(leftOperand, rightOperand, operation)
	} else if leftOperand.GetOperandType() != "$field" && rightOperand.GetOperandType() == "$field" {
		return HandleValueToFieldComparison(leftOperand, rightOperand, operation)
	} else if leftOperand.GetOperandType() == "$field" && rightOperand.GetOperandType() == "$field" {
		return HandleFieldToFieldComparison(leftOperand, rightOperand, operation)
	} else if leftOperand.GetOperandType() != "$field" && rightOperand.GetOperandType() != "$field" {
		return HandleValueToValueComparison(leftOperand, rightOperand, operation)
	} else {
		return nil, fmt.Errorf("unsupported operand combination: left=%s, right=%s",
			leftOperand.GetOperandType(), rightOperand.GetOperandType())
	}
}
