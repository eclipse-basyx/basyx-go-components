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
	"testing"
)

func TestParseArrayOperands(t *testing.T) {
	// Test the new array-based query structure
	jsonData := `{
		"Query": {
			"$condition": {
				"$eq": [
					{ "$field": "$sm#idShort" },
					{ "$field": "$sm#idShort" }
				]
			}
		}
	}`

	var queryObj QueryObj
	err := json.Unmarshal([]byte(jsonData), &queryObj)
	if err != nil {
		t.Fatalf("Error unmarshalling JSON: %v", err)
	}

	// Verify the structure
	if queryObj.Query.Condition == nil {
		t.Fatal("Condition should not be nil")
	}

	comparison, ok := queryObj.Query.Condition.(*Comparison)
	if !ok {
		t.Fatal("Condition should be a Comparison")
	}

	if comparison.GetOperationType() != "$eq" {
		t.Errorf("Expected operation type '$eq', got '%s'", comparison.GetOperationType())
	}

	operation := comparison.GetOperation()
	if operation == nil {
		t.Fatal("Operation should not be nil")
	}

	operands := operation.GetOperands()
	if len(operands) != 2 {
		t.Errorf("Expected 2 operands, got %d", len(operands))
	}

	// Check first operand
	if operands[0].GetOperandType() != "$field" {
		t.Errorf("Expected first operand type '$field', got '%s'", operands[0].GetOperandType())
	}
	if operands[0].GetValue() != "$sm#idShort" {
		t.Errorf("Expected first operand value '$sm#idShort', got '%s'", operands[0].GetValue())
	}

	// Check second operand
	if operands[1].GetOperandType() != "$field" {
		t.Errorf("Expected second operand type '$field', got '%s'", operands[1].GetOperandType())
	}
	if operands[1].GetValue() != "$sm#idShort" {
		t.Errorf("Expected second operand value '$sm#idShort', got '%s'", operands[1].GetValue())
	}
}

func TestParseMixedOperands(t *testing.T) {
	// Test with different operand types
	jsonData := `{
		"Query": {
			"$condition": {
				"$eq": [
					{ "$field": "$sm#idShort" },
					{ "$strVal": "testValue" }
				]
			}
		}
	}`

	var queryObj QueryObj
	err := json.Unmarshal([]byte(jsonData), &queryObj)
	if err != nil {
		t.Fatalf("Error unmarshalling JSON: %v", err)
	}

	comparison, ok := queryObj.Query.Condition.(*Comparison)
	if !ok {
		t.Fatal("Condition should be a Comparison")
	}

	operands := comparison.GetOperation().GetOperands()
	if len(operands) != 2 {
		t.Errorf("Expected 2 operands, got %d", len(operands))
	}

	// Check field operand
	if operands[0].GetOperandType() != "$field" {
		t.Errorf("Expected first operand type '$field', got '%s'", operands[0].GetOperandType())
	}
	if operands[0].GetValue() != "$sm#idShort" {
		t.Errorf("Expected first operand value '$sm#idShort', got '%s'", operands[0].GetValue())
	}

	// Check string operand
	if operands[1].GetOperandType() != "$strVal" {
		t.Errorf("Expected second operand type '$strVal', got '%s'", operands[1].GetOperandType())
	}
	if operands[1].GetValue() != "testValue" {
		t.Errorf("Expected second operand value 'testValue', got '%s'", operands[1].GetValue())
	}
}

func TestParseInvalidOperandCount(t *testing.T) {
	// Test with invalid number of operands
	jsonData := `{
		"Query": {
			"$condition": {
				"$eq": [
					{ "$field": "$sm#idShort" }
				]
			}
		}
	}`

	var queryObj QueryObj
	err := json.Unmarshal([]byte(jsonData), &queryObj)
	if err == nil {
		t.Fatal("Expected error for invalid operand count, got nil")
	}

	expectedError := "error unmarshalling condition as comparison: error unmarshalling $eq: exactly 2 operands are required per operation, got 1"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestLogicalExpressionAND(t *testing.T) {
	// Test AND logical expression
	jsonData := `{
		"Query": {
			"$condition": {
				"$and": [
					{
						"$eq": [
							{ "$field": "$sm#idShort" },
							{ "$strVal": "testValue1" }
						]
					},
					{
						"$ne": [
							{ "$field": "$sm#category" },
							{ "$strVal": "testValue2" }
						]
					}
				]
			}
		}
	}`

	var queryObj QueryObj
	err := json.Unmarshal([]byte(jsonData), &queryObj)
	if err != nil {
		t.Fatalf("Error unmarshalling JSON: %v", err)
	}

	// Verify the structure
	if queryObj.Query.Condition == nil {
		t.Fatal("Condition should not be nil")
	}

	logicalExpr, ok := queryObj.Query.Condition.(*LogicalExpression)
	if !ok {
		t.Fatal("Condition should be a LogicalExpression")
	}

	if logicalExpr.GetConditionType() != "LogicalExpression" {
		t.Errorf("Expected condition type 'LogicalExpression', got '%s'", logicalExpr.GetConditionType())
	}

	if logicalExpr.GetLogicalOperationType() != "$and" {
		t.Errorf("Expected logical operation type '$and', got '%s'", logicalExpr.GetLogicalOperationType())
	}

	conditions := logicalExpr.GetConditions()
	if len(conditions) != 2 {
		t.Errorf("Expected 2 conditions, got %d", len(conditions))
	}

	// Test evaluation
	expr, err := logicalExpr.EvaluateToExpression()
	if err != nil {
		t.Errorf("Error evaluating logical expression: %v", err)
	}
	if expr == nil {
		t.Error("Expression should not be nil")
	}
}

func TestLogicalExpressionOR(t *testing.T) {
	// Test OR logical expression
	jsonData := `{
		"Query": {
			"$condition": {
				"$or": [
					{
						"$eq": [
							{ "$field": "$sm#idShort" },
							{ "$strVal": "testValue1" }
						]
					},
					{
						"$eq": [
							{ "$field": "$sm#idShort" },
							{ "$strVal": "testValue2" }
						]
					}
				]
			}
		}
	}`

	var queryObj QueryObj
	err := json.Unmarshal([]byte(jsonData), &queryObj)
	if err != nil {
		t.Fatalf("Error unmarshalling JSON: %v", err)
	}

	logicalExpr, ok := queryObj.Query.Condition.(*LogicalExpression)
	if !ok {
		t.Fatal("Condition should be a LogicalExpression")
	}

	if logicalExpr.GetLogicalOperationType() != "$or" {
		t.Errorf("Expected logical operation type '$or', got '%s'", logicalExpr.GetLogicalOperationType())
	}

	// Test evaluation
	expr, err := logicalExpr.EvaluateToExpression()
	if err != nil {
		t.Errorf("Error evaluating logical expression: %v", err)
	}
	if expr == nil {
		t.Error("Expression should not be nil")
	}
}

func TestLogicalExpressionNOT(t *testing.T) {
	// Test NOT logical expression
	jsonData := `{
		"Query": {
			"$condition": {
				"$not": [
					{
						"$eq": [
							{ "$field": "$sm#idShort" },
							{ "$strVal": "testValue" }
						]
					}
				]
			}
		}
	}`

	var queryObj QueryObj
	err := json.Unmarshal([]byte(jsonData), &queryObj)
	if err != nil {
		t.Fatalf("Error unmarshalling JSON: %v", err)
	}

	logicalExpr, ok := queryObj.Query.Condition.(*LogicalExpression)
	if !ok {
		t.Fatal("Condition should be a LogicalExpression")
	}

	if logicalExpr.GetLogicalOperationType() != "$not" {
		t.Errorf("Expected logical operation type '$not', got '%s'", logicalExpr.GetLogicalOperationType())
	}

	conditions := logicalExpr.GetConditions()
	if len(conditions) != 1 {
		t.Errorf("Expected 1 condition for NOT, got %d", len(conditions))
	}

	// Test evaluation
	expr, err := logicalExpr.EvaluateToExpression()
	if err != nil {
		t.Errorf("Error evaluating logical expression: %v", err)
	}
	if expr == nil {
		t.Error("Expression should not be nil")
	}
}

func TestNestedLogicalExpression(t *testing.T) {
	// Test nested logical expressions
	jsonData := `{
		"Query": {
			"$condition": {
				"$and": [
					{
						"$eq": [
							{ "$field": "$sm#idShort" },
							{ "$strVal": "testValue1" }
						]
					},
					{
						"$or": [
							{
								"$eq": [
									{ "$field": "$sm#category" },
									{ "$strVal": "category1" }
								]
							},
							{
								"$eq": [
									{ "$field": "$sm#category" },
									{ "$strVal": "category2" }
								]
							}
						]
					}
				]
			}
		}
	}`

	var queryObj QueryObj
	err := json.Unmarshal([]byte(jsonData), &queryObj)
	if err != nil {
		t.Fatalf("Error unmarshalling JSON: %v", err)
	}

	logicalExpr, ok := queryObj.Query.Condition.(*LogicalExpression)
	if !ok {
		t.Fatal("Condition should be a LogicalExpression")
	}

	if logicalExpr.GetLogicalOperationType() != "$and" {
		t.Errorf("Expected logical operation type '$and', got '%s'", logicalExpr.GetLogicalOperationType())
	}

	conditions := logicalExpr.GetConditions()
	if len(conditions) != 2 {
		t.Errorf("Expected 2 conditions, got %d", len(conditions))
	}

	// Check that the second condition is also a LogicalExpression
	if conditions[1].GetConditionType() != "LogicalExpression" {
		t.Errorf("Expected second condition to be LogicalExpression, got '%s'", conditions[1].GetConditionType())
	}

	// Test evaluation
	expr, err := logicalExpr.EvaluateToExpression()
	if err != nil {
		t.Errorf("Error evaluating nested logical expression: %v", err)
	}
	if expr == nil {
		t.Error("Expression should not be nil")
	}
}

func TestCompleteLogicalExpressionWorkflow(t *testing.T) {
	// Test a comprehensive logical expression that demonstrates the tree evaluation
	jsonData := `{
		"Query": {
			"$condition": {
				"$and": [
					{
						"$eq": [
							{ "$field": "$sm#idShort" },
							{ "$strVal": "SensorData" }
						]
					},
					{
						"$or": [
							{
								"$eq": [
									{ "$field": "$sm#category" },
									{ "$strVal": "PARAMETER" }
								]
							},
							{
								"$eq": [
									{ "$field": "$sm#category" },
									{ "$strVal": "VARIABLE" }
								]
							}
						]
					},
					{
						"$not": [
							{
								"$eq": [
									{ "$field": "$sm#kind" },
									{ "$strVal": "Template" }
								]
							}
						]
					}
				]
			}
		}
	}`

	var queryObj QueryObj
	err := json.Unmarshal([]byte(jsonData), &queryObj)
	if err != nil {
		t.Fatalf("Error unmarshalling complex JSON: %v", err)
	}

	// Verify the structure
	logicalExpr, ok := queryObj.Query.Condition.(*LogicalExpression)
	if !ok {
		t.Fatal("Root condition should be a LogicalExpression")
	}

	if logicalExpr.GetLogicalOperationType() != "$and" {
		t.Errorf("Expected root operation '$and', got '%s'", logicalExpr.GetLogicalOperationType())
	}

	conditions := logicalExpr.GetConditions()
	if len(conditions) != 3 {
		t.Errorf("Expected 3 root conditions, got %d", len(conditions))
	}

	// Verify first condition is a comparison
	if conditions[0].GetConditionType() != "Comparison" {
		t.Errorf("First condition should be Comparison, got '%s'", conditions[0].GetConditionType())
	}

	// Verify second condition is a logical expression (OR)
	if conditions[1].GetConditionType() != "LogicalExpression" {
		t.Errorf("Second condition should be LogicalExpression, got '%s'", conditions[1].GetConditionType())
	}

	nestedOr := conditions[1].(*LogicalExpression)
	if nestedOr.GetLogicalOperationType() != "$or" {
		t.Errorf("Nested logical expression should be '$or', got '%s'", nestedOr.GetLogicalOperationType())
	}

	// Verify third condition is a logical expression (NOT)
	if conditions[2].GetConditionType() != "LogicalExpression" {
		t.Errorf("Third condition should be LogicalExpression, got '%s'", conditions[2].GetConditionType())
	}

	nestedNot := conditions[2].(*LogicalExpression)
	if nestedNot.GetLogicalOperationType() != "$not" {
		t.Errorf("Nested logical expression should be '$not', got '%s'", nestedNot.GetLogicalOperationType())
	}

	// Test the complete tree evaluation
	expr, err := logicalExpr.EvaluateToExpression()
	if err != nil {
		t.Fatalf("Error evaluating complete logical expression tree: %v", err)
	}
	if expr == nil {
		t.Fatal("Final expression should not be nil")
	}

	t.Logf("Successfully evaluated complex logical expression tree with nested AND, OR, and NOT operations")
}
