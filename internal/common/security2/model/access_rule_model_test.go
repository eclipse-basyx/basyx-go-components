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

package acm

import (
	"encoding/json"
	"testing"
)

func TestQueryWrapperUnmarshal(t *testing.T) {
	jsonData := `{
    "Query": {
        "$condition": {
            "$eq": [
                {
                    "$field": "$sm#semanticId"
                },
                {
                    "$strVal": "RootLevel_QL2"
                }
            ]
        }
    }
}`

	var queryWrapper QueryWrapper
	err := json.Unmarshal([]byte(jsonData), &queryWrapper)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Verify the structure
	if queryWrapper.Query.Condition == nil {
		t.Fatal("Expected condition to be non-nil")
	}

	if len(queryWrapper.Query.Condition.Eq) != 2 {
		t.Fatalf("Expected 2 operands in $eq, got %d", len(queryWrapper.Query.Condition.Eq))
	}

	// Verify first operand is a field
	firstOperand := &queryWrapper.Query.Condition.Eq[0]
	if !firstOperand.IsField() {
		t.Error("Expected first operand to be a field")
	}
	if firstOperand.Field == nil {
		t.Fatal("Expected field to be non-nil")
	}
	if string(*firstOperand.Field) != "$sm#semanticId" {
		t.Errorf("Expected field to be '$sm#semanticId', got '%s'", string(*firstOperand.Field))
	}

	// Verify second operand is a string value
	secondOperand := &queryWrapper.Query.Condition.Eq[1]
	if secondOperand.IsField() {
		t.Error("Expected second operand to be a value, not a field")
	}
	if secondOperand.StrVal == nil {
		t.Fatal("Expected strVal to be non-nil")
	}
	if string(*secondOperand.StrVal) != "RootLevel_QL2" {
		t.Errorf("Expected strVal to be 'RootLevel_QL2', got '%s'", string(*secondOperand.StrVal))
	}
}

func TestQueryWrapperEvaluateToExpression(t *testing.T) {
	jsonData := `{
    "Query": {
        "$condition": {
            "$eq": [
                {
                    "$field": "$sm#semanticId"
                },
                {
                    "$strVal": "RootLevel_QL2"
                }
            ]
        }
    }
}`

	var queryWrapper QueryWrapper
	err := json.Unmarshal([]byte(jsonData), &queryWrapper)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Evaluate to SQL expression
	expr, err := queryWrapper.Query.Condition.EvaluateToExpression()
	if err != nil {
		t.Fatalf("Failed to evaluate to expression: %v", err)
	}

	if expr == nil {
		t.Fatal("Expected expression to be non-nil")
	}

	t.Logf("Generated expression: %v", expr)
}
