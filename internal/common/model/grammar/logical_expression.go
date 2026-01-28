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
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE ), Martin Stemmer ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"fmt"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// LogicalExpression represents a logical expression tree for AAS access control rules.
//
// This structure supports complex logical conditions that can be evaluated against AAS elements.
// It combines comparison operations (eq, ne, gt, ge, lt, le), string operations (contains,
// starts-with, ends-with, regex), and logical operators (AND, OR, NOT) to form sophisticated
// access control rules. Expressions can be nested to create complex conditional logic.
//
// Only one operation field should be set per LogicalExpression instance. The structure can
// be converted to SQL WHERE clauses using the EvaluateToExpression method.
//
// Logical operators:
//   - $and: All conditions must be true (requires at least 2 expressions)
//   - $or: At least one condition must be true (requires at least 2 expressions)
//   - $not: Negates the nested expression
//
// Comparison operators: $eq, $ne, $gt, $ge, $lt, $le
// String operators: $contains, $starts-with, $ends-with, $regex
// Boolean: Direct boolean value evaluation
//
// Example JSON:
//
//	{"$and": [
//	  {"$eq": ["$sm#idShort", "MySubmodel"]},
//	  {"$gt": ["$sme.temperature#value", "100"]}
//	]}
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

// UnmarshalJSON implements the json.Unmarshaler interface for LogicalExpression.
//
// This custom unmarshaler validates that logical operator arrays contain the required
// minimum number of elements:
//   - $and requires at least 2 expressions
//   - $or requires at least 2 expressions
//   - $match requires at least 1 expression
//
// These constraints ensure that logical operations are meaningful and properly formed.
//
// Parameters:
//   - value: JSON byte slice containing the logical expression to unmarshal
//
// Returns:
//   - error: An error if the JSON is invalid or if array constraints are violated.
//     Returns nil on successful unmarshaling and validation.
func (le *LogicalExpression) UnmarshalJSON(value []byte) error {
	type Plain LogicalExpression
	var plain Plain

	if err := common.UnmarshalAndDisallowUnknownFields(value, &plain); err != nil {
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

	// Enforce matching operand types for comparison operators (when both are known)
	if err := validateComparisonItems(plain.Eq, "$eq"); err != nil {
		return err
	}
	if err := validateComparisonItems(plain.Ne, "$ne"); err != nil {
		return err
	}
	if err := validateComparisonItems(plain.Gt, "$gt"); err != nil {
		return err
	}
	if err := validateComparisonItems(plain.Ge, "$ge"); err != nil {
		return err
	}
	if err := validateComparisonItems(plain.Lt, "$lt"); err != nil {
		return err
	}
	if err := validateComparisonItems(plain.Le, "$le"); err != nil {
		return err
	}
	*le = LogicalExpression(plain)
	return nil
}

// validateComparisonItems ensures that comparison operands have matching types when both are known.
// It is intentionally lenient for dynamic operands (fields/attributes), as their runtime type is unknown.
func validateComparisonItems(items ComparisonItems, op string) error {
	if len(items) == 0 {
		return nil
	}
	if len(items) != 2 {
		return fmt.Errorf("comparison %s requires exactly 2 operands, got %d", op, len(items))
	}
	_, err := items[0].IsComparableTo(items[1])
	return err
}

// AssertLogicalExpressionRequired checks if the required fields are not zero-ed
func AssertLogicalExpressionRequired(obj LogicalExpression) error {
	for _, el := range obj.And {
		if err := AssertLogicalExpressionRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Match {
		if err := AssertMatchExpressionRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Or {
		if err := AssertLogicalExpressionRequired(el); err != nil {
			return err
		}
	}
	if obj.Not != nil {
		if err := AssertLogicalExpressionRequired(*obj.Not); err != nil {
			return err
		}
	}
	for _, el := range obj.Eq {
		if err := AssertValueRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Ne {
		if err := AssertValueRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Gt {
		if err := AssertValueRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Ge {
		if err := AssertValueRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Lt {
		if err := AssertValueRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Le {
		if err := AssertValueRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Contains {
		if err := AssertStringValueRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.StartsWith {
		if err := AssertStringValueRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.EndsWith {
		if err := AssertStringValueRequired(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Regex {
		if err := AssertStringValueRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertLogicalExpressionConstraints checks if the values respects the defined constraints
func AssertLogicalExpressionConstraints(obj LogicalExpression) error {
	for _, el := range obj.And {
		if err := AssertLogicalExpressionConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Match {
		if err := AssertMatchExpressionConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Or {
		if err := AssertLogicalExpressionConstraints(el); err != nil {
			return err
		}
	}
	if obj.Not != nil {
		if err := AssertLogicalExpressionConstraints(*obj.Not); err != nil {
			return err
		}
	}
	for _, el := range obj.Eq {
		if err := AssertValueConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Ne {
		if err := AssertValueConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Gt {
		if err := AssertValueConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Ge {
		if err := AssertValueConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Lt {
		if err := AssertValueConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Le {
		if err := AssertValueConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Contains {
		if err := AssertStringValueConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.StartsWith {
		if err := AssertStringValueConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.EndsWith {
		if err := AssertStringValueConstraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.Regex {
		if err := AssertStringValueConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
