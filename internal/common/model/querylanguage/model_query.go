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
package querylanguage

import (
	"encoding/json"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

type QueryObj struct {
	Query Query `json:"Query"`
}

type Query struct {
	Select    string    `json:"$select"`
	Condition Condition `json:"$condition"`
}

// UnmarshalJSON implements custom unmarshalling for Query.Condition
func (q *Query) UnmarshalJSON(data []byte) error {
	// Define a temporary structure to unmarshal the basic fields
	type TempQuery struct {
		Select    string          `json:"$select"`
		Condition json.RawMessage `json:"$condition"`
	}

	var temp TempQuery
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	q.Select = temp.Select

	// If there's a condition, determine its type and unmarshal accordingly
	if len(temp.Condition) > 0 {
		// Try to determine if it's a comparison or logical expression
		var rawCondition map[string]interface{}
		if err := json.Unmarshal(temp.Condition, &rawCondition); err != nil {
			return fmt.Errorf("error unmarshalling condition structure: %w", err)
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
			if err := json.Unmarshal(temp.Condition, &comparison); err != nil {
				return fmt.Errorf("error unmarshalling condition as comparison: %w", err)
			}
			q.Condition = &comparison
		} else if isLogical {
			var logicalExpr LogicalExpression
			if err := json.Unmarshal(temp.Condition, &logicalExpr); err != nil {
				return fmt.Errorf("error unmarshalling condition as logical expression: %w", err)
			}
			q.Condition = &logicalExpr
		} else {
			return fmt.Errorf("unknown condition type")
		}
	}

	return nil
}

func HandleFieldToValueComparison(leftOperand, rightOperand Operand, operation string) (exp.Expression, error) {
	var condition exp.Expression
	fieldName, ok := leftOperand.GetValue().(string)
	if !ok {
		return nil, fmt.Errorf("left operand is not a string field")
	}
	if len(fieldName) > 4 && fieldName[:4] == "$sm#" {
		fieldName = ParseAASQLFieldToSQLColumn(fieldName)
	}
	leftCol := goqu.I(fieldName)
	rightVal := goqu.V(rightOperand.GetValue())

	switch operation {
	case "$eq":
		condition = leftCol.Eq(rightVal)
	case "$ne":
		condition = leftCol.Neq(rightVal)
	case "$gt":
		condition = leftCol.Gt(rightVal)
	case "$ge":
		condition = leftCol.Gte(rightVal)
	case "$lt":
		condition = leftCol.Lt(rightVal)
	case "$le":
		condition = leftCol.Lte(rightVal)
	default:
		return nil, fmt.Errorf("unsupported comparison operation: %s", operation)
	}
	return condition, nil
}

func HandleValueToFieldComparison(leftOperand, rightOperand Operand, operation string) (exp.Expression, error) {
	var condition exp.Expression
	fieldName, ok := rightOperand.GetValue().(string)
	if !ok {
		return nil, fmt.Errorf("right operand is not a string field")
	}
	if len(fieldName) > 4 && fieldName[:4] == "$sm#" {
		fieldName = ParseAASQLFieldToSQLColumn(fieldName)
	}
	rightCol := goqu.I(fieldName)
	leftVal := goqu.V(leftOperand.GetValue())

	switch operation {
	case "$eq":
		condition = leftVal.Eq(rightCol)
	case "$ne":
		condition = leftVal.Neq(rightCol)
	case "$gt":
		condition = leftVal.Gt(rightCol)
	case "$ge":
		condition = leftVal.Gte(rightCol)
	case "$lt":
		condition = leftVal.Lt(rightCol)
	case "$le":
		condition = leftVal.Lte(rightCol)
	default:
		return nil, fmt.Errorf("unsupported comparison operation: %s", operation)
	}
	return condition, nil
}

func HandleFieldToFieldComparison(leftOperand, rightOperand Operand, operation string) (exp.Expression, error) {
	var condition exp.Expression
	leftFieldName, ok := leftOperand.GetValue().(string)
	if !ok {
		return nil, fmt.Errorf("left operand is not a string field")
	}
	if len(leftFieldName) > 4 && leftFieldName[:4] == "$sm#" {
		leftFieldName = ParseAASQLFieldToSQLColumn(leftFieldName)
	}
	rightFieldName, ok := rightOperand.GetValue().(string)
	if !ok {
		return nil, fmt.Errorf("right operand is not a string field")
	}
	if len(rightFieldName) > 4 && rightFieldName[:4] == "$sm#" {
		rightFieldName = ParseAASQLFieldToSQLColumn(rightFieldName)
	}
	leftCol := goqu.I(leftFieldName)
	rightCol := goqu.I(rightFieldName)

	switch operation {
	case "$eq":
		condition = leftCol.Eq(rightCol)
	case "$ne":
		condition = leftCol.Neq(rightCol)
	case "$gt":
		condition = leftCol.Gt(rightCol)
	case "$ge":
		condition = leftCol.Gte(rightCol)
	case "$lt":
		condition = leftCol.Lt(rightCol)
	case "$le":
		condition = leftCol.Lte(rightCol)
	default:
		return nil, fmt.Errorf("unsupported comparison operation: %s", operation)
	}
	return condition, nil
}

func HandleValueToValueComparison(leftOperand, rightOperand Operand, operation string) (exp.Expression, error) {
	var condition exp.Expression

	if leftOperand.GetOperandType() != rightOperand.GetOperandType() {
		return nil, fmt.Errorf("cannot compare different operand types: %s and %s", leftOperand.GetOperandType(), rightOperand.GetOperandType())
	}

	leftVal := goqu.V(leftOperand.GetValue())
	rightVal := goqu.V(rightOperand.GetValue())

	switch operation {
	case "$eq":
		condition = leftVal.Eq(rightVal)
	case "$ne":
		condition = leftVal.Neq(rightVal)
	case "$gt":
		condition = leftVal.Gt(rightVal)
	case "$ge":
		condition = leftVal.Gte(rightVal)
	case "$lt":
		condition = leftVal.Lt(rightVal)
	case "$le":
		condition = leftVal.Lte(rightVal)
	default:
		return nil, fmt.Errorf("unsupported comparison operation: %s", operation)
	}
	return condition, nil
}

func ParseAASQLFieldToSQLColumn(field string) string {
	switch field {
	case "$sm#idShort":
		return "s.id_short"
	case "$sm#id":
		return "s.id"
	case "$sm#semanticId":
		return "semantic_id_reference_key.value"
	}
	return field
}
