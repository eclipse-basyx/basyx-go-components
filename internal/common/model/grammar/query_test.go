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

// Package grammar defines the data structures for representing queries in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"encoding/json"
	"testing"

	jsoniter "github.com/json-iterator/go"
)

func TestQueryWrapperUnmarshal_SimpleEquality(t *testing.T) {
	jsonStr := `{
		"Query": {
			"$condition": {
				"$eq": [
					{"$field": "$sm#semanticId"},
					{"$strVal": "RootLevel_QL2"}
				]
			}
		}
	}`

	var wrapper QueryWrapper
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	err := json.Unmarshal([]byte(jsonStr), &wrapper)
	if err != nil {
		t.Fatalf("Failed to unmarshal query: %v", err)
	}

	if wrapper.Query.Condition == nil {
		t.Fatal("Expected Condition to be set")
	}

	if len(wrapper.Query.Condition.Eq) != 2 {
		t.Fatalf("Expected 2 operands in $eq, got %d", len(wrapper.Query.Condition.Eq))
	}

	leftOperand := wrapper.Query.Condition.Eq[0]
	if leftOperand.Field == nil {
		t.Fatal("Expected left operand to be a field")
	}
	if *leftOperand.Field != "$sm#semanticId" {
		t.Errorf("Expected field '$sm#semanticId', got '%s'", *leftOperand.Field)
	}

	rightOperand := wrapper.Query.Condition.Eq[1]
	if rightOperand.StrVal == nil {
		t.Fatal("Expected right operand to be a string value")
	}
	if string(*rightOperand.StrVal) != "RootLevel_QL2" {
		t.Errorf("Expected value 'RootLevel_QL2', got '%s'", *rightOperand.StrVal)
	}
}

func TestQueryWrapperUnmarshal_NestedAndOr(t *testing.T) {
	jsonStr := `{
		"Query": {
			"$condition": {
				"$and": [
					{
						"$eq": [
							{"$field": "$sm#idShort"},
							{"$strVal": "TestSubmodel"}
						]
					},
					{
						"$or": [
							{
								"$gt": [
									{"$numCast":{"$field": "$sm#id"}},
									{"$numVal": 100}
								]
							},
							{
								"$eq": [
									{"$field": "$sm#semanticId"},
									{"$strVal": "test.semantic.id"}
								]
							}
						]
					}
				]
			}
		}
	}`

	var wrapper QueryWrapper
	err := json.Unmarshal([]byte(jsonStr), &wrapper)
	if err != nil {
		t.Fatalf("Failed to unmarshal nested query: %v", err)
	}

	if wrapper.Query.Condition == nil {
		t.Fatal("Expected Condition to be set")
	}

	if len(wrapper.Query.Condition.And) != 2 {
		t.Fatalf("Expected 2 AND conditions, got %d", len(wrapper.Query.Condition.And))
	}

	// Check first AND condition (equality)
	firstAnd := wrapper.Query.Condition.And[0]
	if len(firstAnd.Eq) != 2 {
		t.Fatalf("Expected 2 operands in first $eq, got %d", len(firstAnd.Eq))
	}

	// Check second AND condition (OR with 2 conditions)
	secondAnd := wrapper.Query.Condition.And[1]
	if len(secondAnd.Or) != 2 {
		t.Fatalf("Expected 2 OR conditions, got %d", len(secondAnd.Or))
	}

	// Check first OR condition (greater than)
	firstOr := secondAnd.Or[0]
	if len(firstOr.Gt) != 2 {
		t.Fatalf("Expected 2 operands in $gt, got %d", len(firstOr.Gt))
	}
	if firstOr.Gt[1].NumVal == nil || *firstOr.Gt[1].NumVal != 100 {
		t.Error("Expected right operand of $gt to be 100")
	}

	// Check second OR condition (equality)
	secondOr := secondAnd.Or[1]
	if len(secondOr.Eq) != 2 {
		t.Fatalf("Expected 2 operands in second $eq, got %d", len(secondOr.Eq))
	}
}

func TestQueryWrapperUnmarshal_AllComparisonOperators(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		checkOp func(*LogicalExpression) bool
		opName  string
	}{
		{
			name:    "Equality",
			json:    `{"Query": {"$condition": {"$eq": [{"$numCast":{"$field": "$sm#id"}}, {"$numVal": 1}]}}}`,
			checkOp: func(le *LogicalExpression) bool { return len(le.Eq) == 2 },
			opName:  "$eq",
		},
		{
			name:    "Not Equal",
			json:    `{"Query": {"$condition": {"$ne": [{"$numCast":{"$field": "$sm#id"}}, {"$numVal": 1}]}}}`,
			checkOp: func(le *LogicalExpression) bool { return len(le.Ne) == 2 },
			opName:  "$ne",
		},
		{
			name:    "Greater Than",
			json:    `{"Query": {"$condition": {"$gt": [{"$numCast":{"$field": "$sm#id"}}, {"$numVal": 1}]}}}`,
			checkOp: func(le *LogicalExpression) bool { return len(le.Gt) == 2 },
			opName:  "$gt",
		},
		{
			name:    "Greater or Equal",
			json:    `{"Query": {"$condition": {"$ge": [{"$numCast":{"$field": "$sm#id"}}, {"$numVal": 1}]}}}`,
			checkOp: func(le *LogicalExpression) bool { return len(le.Ge) == 2 },
			opName:  "$ge",
		},
		{
			name:    "Less Than",
			json:    `{"Query": {"$condition": {"$lt": [{"$numCast":{"$field": "$sm#id"}}, {"$numVal": 1}]}}}`,
			checkOp: func(le *LogicalExpression) bool { return len(le.Lt) == 2 },
			opName:  "$lt",
		},
		{
			name:    "Less or Equal",
			json:    `{"Query": {"$condition": {"$le": [{"$numCast":{"$field": "$sm#id"}}, {"$numVal": 1}]}}}`,
			checkOp: func(le *LogicalExpression) bool { return len(le.Le) == 2 },
			opName:  "$le",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wrapper QueryWrapper
			err := json.Unmarshal([]byte(tt.json), &wrapper)
			if err != nil {
				t.Fatalf("Failed to unmarshal %s query: %v", tt.opName, err)
			}

			if wrapper.Query.Condition == nil {
				t.Fatal("Expected Condition to be set")
			}

			if !tt.checkOp(wrapper.Query.Condition) {
				t.Errorf("Expected %s operation to be present with 2 operands", tt.opName)
			}
		})
	}
}

func TestQueryWrapperUnmarshal_DifferentValueTypes(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		checkVal  func(*Value) bool
		valueDesc string
	}{
		{
			name: "String Value",
			json: `{"Query": {"$condition": {"$eq": [{"$field": "$sm#id"}, {"$strVal": "test"}]}}}`,
			checkVal: func(v *Value) bool {
				return v.StrVal != nil && string(*v.StrVal) == "test"
			},
			valueDesc: "string 'test'",
		},
		{
			name: "Numeric Value",
			json: `{"Query": {"$condition": {"$eq": [{"$numCast":{"$field": "$sm#id"}}, {"$numVal": 42}]}}}`,
			checkVal: func(v *Value) bool {
				return v.NumVal != nil && *v.NumVal == 42
			},
			valueDesc: "number 42",
		},
		{
			name: "Float Value",
			json: `{"Query": {"$condition": {"$eq": [{"$numCast":{"$field": "$sm#id"}}, {"$numVal": 3.14}]}}}`,
			checkVal: func(v *Value) bool {
				return v.NumVal != nil && *v.NumVal == 3.14
			},
			valueDesc: "float 3.14",
		},
		{
			name: "Boolean Value",
			json: `{"Query": {"$condition": {"$eq": [{"$boolCast":{"$field": "$sm#id"}}, {"$boolean": true}]}}}`,
			checkVal: func(v *Value) bool {
				return v.Boolean != nil && *v.Boolean == true
			},
			valueDesc: "boolean true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wrapper QueryWrapper
			err := json.Unmarshal([]byte(tt.json), &wrapper)
			if err != nil {
				t.Fatalf("Failed to unmarshal query with %s: %v", tt.valueDesc, err)
			}

			if len(wrapper.Query.Condition.Eq) != 2 {
				t.Fatalf("Expected 2 operands, got %d", len(wrapper.Query.Condition.Eq))
			}

			rightOperand := wrapper.Query.Condition.Eq[1]
			if !tt.checkVal(&rightOperand) {
				t.Errorf("Expected right operand to be %s", tt.valueDesc)
			}
		})
	}
}

func TestQueryWrapperUnmarshal_NotOperation(t *testing.T) {
	jsonStr := `{
		"Query": {
			"$condition": {
				"$not": {
					"$eq": [
						{"$field": "$sm#idShort"},
						{"$strVal": "ExcludedSubmodel"}
					]
				}
			}
		}
	}`

	var wrapper QueryWrapper
	err := json.Unmarshal([]byte(jsonStr), &wrapper)
	if err != nil {
		t.Fatalf("Failed to unmarshal NOT query: %v", err)
	}

	if wrapper.Query.Condition == nil {
		t.Fatal("Expected Condition to be set")
	}

	if wrapper.Query.Condition.Not == nil {
		t.Fatal("Expected NOT condition to be set")
	}

	if len(wrapper.Query.Condition.Not.Eq) != 2 {
		t.Fatalf("Expected 2 operands in nested $eq, got %d", len(wrapper.Query.Condition.Not.Eq))
	}
}

func TestQueryWrapperUnmarshal_MissingQueryField(t *testing.T) {
	jsonStr := `{
		"$condition": {
			"$eq": [
				{"$field": "$sm#id"},
				{"$strVal": "test"}
			]
		}
	}`

	var wrapper QueryWrapper
	err := json.Unmarshal([]byte(jsonStr), &wrapper)
	if err == nil {
		t.Fatal("Expected error when Query field is missing")
	}

	expectedErrMsg := "field Query in QueryWrapper: required"
	if err.Error() != expectedErrMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrMsg, err.Error())
	}
}

func TestQueryWrapperUnmarshal_EmptyQuery(t *testing.T) {
	jsonStr := `{
		"Query": {}
	}`

	var wrapper QueryWrapper
	err := json.Unmarshal([]byte(jsonStr), &wrapper)
	if err != nil {
		t.Fatalf("Failed to unmarshal empty query: %v", err)
	}

	if wrapper.Query.Condition != nil {
		t.Error("Expected Condition to be nil for empty query")
	}
}

func TestQueryWrapperUnmarshal_FieldToFieldComparison(t *testing.T) {
	jsonStr := `{
		"Query": {
			"$condition": {
				"$eq": [
					{"$field": "$sm#idShort"},
					{"$field": "$sm#id"}
				]
			}
		}
	}`

	var wrapper QueryWrapper
	err := json.Unmarshal([]byte(jsonStr), &wrapper)
	if err != nil {
		t.Fatalf("Failed to unmarshal field-to-field query: %v", err)
	}

	if len(wrapper.Query.Condition.Eq) != 2 {
		t.Fatalf("Expected 2 operands, got %d", len(wrapper.Query.Condition.Eq))
	}

	leftOperand := wrapper.Query.Condition.Eq[0]
	rightOperand := wrapper.Query.Condition.Eq[1]

	if !leftOperand.IsField() || !rightOperand.IsField() {
		t.Error("Expected both operands to be fields")
	}

	if *leftOperand.Field != "$sm#idShort" {
		t.Errorf("Expected left field '$sm#idShort', got '%s'", *leftOperand.Field)
	}

	if *rightOperand.Field != "$sm#id" {
		t.Errorf("Expected right field '$sm#id', got '%s'", *rightOperand.Field)
	}
}

func TestQueryWrapperUnmarshal_ValueToValueComparison(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		checkVals func(*Value, *Value) bool
		desc      string
	}{
		{
			name: "String to String",
			json: `{"Query": {"$condition": {"$eq": [{"$strVal": "value1"}, {"$strVal": "value2"}]}}}`,
			checkVals: func(left, right *Value) bool {
				return left.StrVal != nil && right.StrVal != nil &&
					string(*left.StrVal) == "value1" && string(*right.StrVal) == "value2"
			},
			desc: "two string values",
		},
		{
			name: "Number to Number",
			json: `{"Query": {"$condition": {"$gt": [{"$numVal": 100}, {"$numVal": 50}]}}}`,
			checkVals: func(left, right *Value) bool {
				return left.NumVal != nil && right.NumVal != nil &&
					*left.NumVal == 100 && *right.NumVal == 50
			},
			desc: "two numeric values",
		},
		{
			name: "Boolean to Boolean",
			json: `{"Query": {"$condition": {"$eq": [{"$boolean": true}, {"$boolean": false}]}}}`,
			checkVals: func(left, right *Value) bool {
				return left.Boolean != nil && right.Boolean != nil &&
					*left.Boolean == true && *right.Boolean == false
			},
			desc: "two boolean values",
		},
		{
			name: "Same String Values",
			json: `{"Query": {"$condition": {"$eq": [{"$strVal": "same"}, {"$strVal": "same"}]}}}`,
			checkVals: func(left, right *Value) bool {
				return left.StrVal != nil && right.StrVal != nil &&
					string(*left.StrVal) == string(*right.StrVal)
			},
			desc: "identical string values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wrapper QueryWrapper
			err := json.Unmarshal([]byte(tt.json), &wrapper)
			if err != nil {
				t.Fatalf("Failed to unmarshal value-to-value query with %s: %v", tt.desc, err)
			}

			if wrapper.Query.Condition == nil {
				t.Fatal("Expected Condition to be set")
			}

			// Get the comparison items (could be Eq or Gt)
			var operands []Value
			if len(wrapper.Query.Condition.Eq) > 0 {
				operands = wrapper.Query.Condition.Eq
			} else if len(wrapper.Query.Condition.Gt) > 0 {
				operands = wrapper.Query.Condition.Gt
			}

			if len(operands) != 2 {
				t.Fatalf("Expected 2 operands, got %d", len(operands))
			}

			leftOperand := operands[0]
			rightOperand := operands[1]

			if leftOperand.IsField() || rightOperand.IsField() {
				t.Error("Expected both operands to be values, not fields")
			}

			if !tt.checkVals(&leftOperand, &rightOperand) {
				t.Errorf("Value validation failed for %s", tt.desc)
			}
		})
	}
}
