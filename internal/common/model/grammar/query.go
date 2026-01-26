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
	"fmt"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// Query represents a query structure with a condition field
type Query struct {
	// Condition corresponds to the JSON schema field "$condition".
	Condition *LogicalExpression `json:"$condition,omitempty" yaml:"$condition,omitempty" mapstructure:"$condition,omitempty"`
}

// QueryWrapper wraps a Query object
type QueryWrapper struct {
	// Query corresponds to the JSON schema field "Query".
	Query Query `json:"Query" yaml:"Query" mapstructure:"Query"`
}

// UnmarshalJSON implements json.Unmarshaler for QueryWrapper.
func (j *QueryWrapper) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := common.UnmarshalAndDisallowUnknownFields(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["Query"]; raw != nil && !ok {
		return fmt.Errorf("field Query in QueryWrapper: required")
	}
	type Plain QueryWrapper
	var plain Plain
	if err := common.UnmarshalAndDisallowUnknownFields(value, &plain); err != nil {
		return err
	}
	*j = QueryWrapper(plain)
	return nil
}

// AssertQueryRequired checks if the required fields are not zero-ed
func AssertQueryRequired(obj Query) error {
	elements := map[string]interface{}{
		"$condition": obj.Condition,
	}
	for name, el := range elements {
		if isZero := model.IsZeroValue(el); isZero {
			return &model.RequiredError{Field: name}
		}
	}

	if obj.Condition != nil {
		if err := AssertLogicalExpressionRequired(*obj.Condition); err != nil {
			return err
		}
	}
	return nil
}

// AssertQueryConstraints checks if the values respects the defined constraints
func AssertQueryConstraints(obj Query) error {
	if obj.Condition != nil {
		if err := AssertLogicalExpressionConstraints(*obj.Condition); err != nil {
			return err
		}
	}
	return nil
}
