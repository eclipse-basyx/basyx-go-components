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

// Package grammar defines the data structures for representing logical expressions in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package grammar

import (
	"fmt"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	builder "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
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

// EvaluateToExpression converts the logical expression tree into a goqu SQL expression.
//
// This method traverses the logical expression tree and constructs a corresponding SQL
// WHERE clause expression that can be used with the goqu query builder. It handles all
// supported comparison operations, logical operators (AND, OR, NOT), and nested expressions.
//
// The method supports special handling for AAS-specific fields, particularly semantic IDs,
// where additional constraints (like position = 0) may be added to the generated SQL.
//
// Supported operations:
//   - Comparison: $eq, $ne, $gt, $ge, $lt, $le
//   - Logical: $and (all true), $or (any true), $not (negation)
//   - Boolean: Direct boolean literal evaluation
//
// Returns:
//   - exp.Expression: A goqu expression that can be used in SQL WHERE clauses
//   - error: An error if the expression is invalid, has no valid operation, or if
//     evaluation of nested expressions fails
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

	// Handle string operations
	if len(le.Contains) > 0 {
		return le.evaluateStringOperationSQL(le.Contains, "$contains")
	}
	if len(le.StartsWith) > 0 {
		return le.evaluateStringOperationSQL(le.StartsWith, "$starts-with")
	}
	if len(le.EndsWith) > 0 {
		return le.evaluateStringOperationSQL(le.EndsWith, "$ends-with")
	}
	if len(le.Regex) > 0 {
		return le.evaluateStringOperationSQL(le.Regex, "$regex")
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

// evaluateStringOperationSQL builds SQL expressions for string operators like $contains, $starts-with, $ends-with, and $regex.
func (le *LogicalExpression) evaluateStringOperationSQL(items []StringValue, operation string) (exp.Expression, error) {
	if len(items) != 2 {
		return nil, fmt.Errorf("string operation %s requires exactly 2 operands, got %d", operation, len(items))
	}

	leftOperand := stringValueToValue(items[0])
	rightOperand := stringValueToValue(items[1])

	return HandleStringOperation(&leftOperand, &rightOperand, operation)
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

// ParseAASQLFieldToSQLColumn translates AAS query language field names to SQL column names.
//
// This function maps AAS-specific field references (like $sm#idShort, $sm#semanticId) to their
// corresponding database column names used in SQL queries. It handles both exact matches and
// pattern-based field references (e.g., semanticId.keys[].value).
//
// Supported field mappings:
//   - $sm#idShort -> s.id_short
//   - $sm#id -> s.id
//   - $sm#semanticId -> semantic_id_reference_key.value
//   - $sm#semanticId.type -> semantic_id_reference.type
//   - $sm#semanticId.keys[].value -> semantic_id_reference_key.value
//   - $sm#semanticId.keys[].type -> semantic_id_reference_key.type
//   - $sm#semanticId.keys[N].value -> semantic_id_reference_key.value (with position constraint)
//   - $sm#semanticId.keys[N].type -> semantic_id_reference_key.type (with position constraint)
//
// Parameters:
//   - field: AAS query language field reference string
//
// Returns:
//   - string: The corresponding SQL column name, or the original field if no mapping exists
func ParseAASQLFieldToSQLColumn(field string) string {

	// TIPP: DO NOT ADD SHORTHANDS HERE!
	// They are already normalized
	switch field {
	case "$sm#idShort":
		return "s.id_short"
	case "$sm#id":
		return "s.id"
	case "$sm#semanticId.type":
		return "semantic_id_reference.type"
	case "$sm#semanticId.keys[].value":
		return "semantic_id_reference_key.value"
	case "$sm#semanticId.keys[].type":
		return "semantic_id_reference_key.type"
	case "$aasdesc#idShort":
		return "aas_descriptor.id_short"
	case "$aasdesc#id":
		return "aas_descriptor.id"
	case "$aasdesc#assetKind":
		return "aas_descriptor.asset_kind"
	case "$aasdesc#assetType":
		return "aas_descriptor.asset_type"
	case "$aasdesc#globalAssetId":
		return "aas_descriptor.global_asset_id"
	case "$aasdesc#specificAssetIds[].name":
		return "specific_asset_id.name"
	case "$aasdesc#specificAssetIds[].value":
		return "specific_asset_id.value"
	case "$aasdesc#specificAssetIds[].externalSubjectId.keys[].value":
		return "external_subject_reference_key.value"
	case "$aasdesc#specificAssetIds[].externalSubjectId.keys[].type":
		return "external_subject_reference_key.type"
	case "$aasdesc#endpoints[].protocolinformation.href":
		return "aas_descriptor_endpoint.href"
	case "$aasdesc#endpoints[].interface":
		return "aas_descriptor_endpoint.interface"
	case "$aasdesc#submodelDescriptors[].idShort":
		return "submodel_descriptor.id_short"
	case "$aasdesc#submodelDescriptors[].id":
		return "submodel_descriptor.id"
	case "$aasdesc#submodelDescriptors[].semanticId.value":
		return "aasdesc_submodel_descriptor_semantic_id_reference.value"
	case "$aasdesc#submodelDescriptors[].semanticId.type":
		return "aasdesc_submodel_descriptor_semantic_id_reference.type"
	case "$aasdesc#submodelDescriptors[].semanticId.keys[].value":
		return "aasdesc_submodel_descriptor_semantic_id_reference_key.value"
	case "$aasdesc#submodelDescriptors[].semanticId.keys[].type":
		return "aasdesc_submodel_descriptor_semantic_id_reference_key.type"
	case "$aasdesc#submodelDescriptors[].endpoints[].interface":
		return "submodel_descriptor_endpoint.interface"
	case "$aasdesc#submodelDescriptors[].endpoints[].protocolinformation.href":
		return "submodel_descriptor_endpoint.href"
	case "$smdesc#idShort":
		return "submodel_descriptor.id_short"
	case "$smdesc#id":
		return "submodel_descriptor.id"
	case "$smdesc#semanticId.value":
		return "aasdesc_submodel_descriptor_semantic_id_reference.value"
	case "$smdesc#semanticId.type":
		return "aasdesc_submodel_descriptor_semantic_id_reference.type"
	case "$smdesc#semanticId.keys[].value":
		return "aasdesc_submodel_descriptor_semantic_id_reference_key.value"
	case "$smdesc#semanticId.keys[].type":
		return "aasdesc_submodel_descriptor_semantic_id_reference_key.type"
	case "$smdesc#endpoints[].interface":
		return "submodel_descriptor_endpoint.interface"
	case "$smdesc#endpoints[].protocolinformation.href":
		return "submodel_descriptor_endpoint.href"

	}

	if strings.HasPrefix(field, "$sm#semanticId.keys[") && strings.HasSuffix(field, "].value") {
		return "semantic_id_reference_key.value"
	}
	if strings.HasPrefix(field, "$sm#semanticId.keys[") && strings.HasSuffix(field, "].type") {
		return "semantic_id_reference_key.type"
	}

	if strings.HasPrefix(field, "$aasdesc#specificAssetIds[") && strings.HasSuffix(field, "].name") {
		return "specific_asset_id.name"
	}
	if strings.HasPrefix(field, "$aasdesc#specificAssetIds[") && strings.HasSuffix(field, "].value") {
		return "specific_asset_id.value"
	}
	if strings.HasPrefix(field, "$aasdesc#specificAssetIds") && strings.Contains(field, ".externalSubjectId.keys[") && strings.HasSuffix(field, "].value") {
		return "external_subject_reference_key.value"
	}
	if strings.HasPrefix(field, "$aasdesc#specificAssetIds") && strings.Contains(field, ".externalSubjectId.keys[") && strings.HasSuffix(field, "].type") {
		return "external_subject_reference_key.type"
	}

	if strings.HasPrefix(field, "$smdesc#semanticId.keys[") && strings.HasSuffix(field, "].value") {
		return "aasdesc_submodel_descriptor_semantic_id_reference_key.value"
	}
	if strings.HasPrefix(field, "$smdesc#semanticId.keys[") && strings.HasSuffix(field, "].type") {
		return "aasdesc_submodel_descriptor_semantic_id_reference_key.type"
	}
	if strings.HasPrefix(field, "$aasdesc#submodelDescriptors") && strings.Contains(field, ".semanticId.keys[") && strings.HasSuffix(field, "].value") {
		return "aasdesc_submodel_descriptor_semantic_id_reference_key.value"
	}
	if strings.HasPrefix(field, "$aasdesc#submodelDescriptors") && strings.Contains(field, ".semanticId.keys[") && strings.HasSuffix(field, "].type") {
		return "aasdesc_submodel_descriptor_semantic_id_reference_key.type"
	}

	// Handle indexed endpoints for aas_descriptor
	if strings.HasPrefix(field, "$aasdesc#endpoints[") && strings.Contains(field, "].interface") {
		return "aas_descriptor_endpoint.interface"
	}
	if strings.HasPrefix(field, "$aasdesc#endpoints[") && strings.Contains(field, "].protocolinformation.href") {
		return "aas_descriptor_endpoint.href"
	}

	// Handle indexed submodelDescriptors fields
	if strings.HasPrefix(field, "$aasdesc#submodelDescriptors[") && strings.Contains(field, "].idShort") {
		return "submodel_descriptor.id_short"
	}
	if strings.HasPrefix(field, "$aasdesc#submodelDescriptors[") && strings.Contains(field, "].id") {
		return "submodel_descriptor.id"
	}
	if strings.HasPrefix(field, "$aasdesc#submodelDescriptors[") && strings.Contains(field, "].semanticId.type") {
		return "aasdesc_submodel_descriptor_semantic_id_reference_key.type"
	}
	if strings.HasPrefix(field, "$aasdesc#submodelDescriptors[") && strings.Contains(field, "].semanticId.value") {
		return "aasdesc_submodel_descriptor_semantic_id_reference_key.value"
	}

	// Handle indexed submodelDescriptors endpoints
	if strings.HasPrefix(field, "$aasdesc#submodelDescriptors[") && strings.Contains(field, "].endpoints[") && strings.Contains(field, "].interface") {
		return "submodel_descriptor_endpoint.interface"
	}
	if strings.HasPrefix(field, "$aasdesc#submodelDescriptors[") && strings.Contains(field, "].endpoints[") && strings.Contains(field, "].protocolinformation.href") {
		return "submodel_descriptor_endpoint.href"
	}

	// Handle indexed smdesc endpoints
	if strings.HasPrefix(field, "$smdesc#endpoints[") && strings.Contains(field, "].interface") {
		return "submodel_descriptor_endpoint.interface"
	}
	if strings.HasPrefix(field, "$smdesc#endpoints[") && strings.Contains(field, "].protocolinformation.href") {
		return "submodel_descriptor_endpoint.href"
	}

	return field
}

// HandleComparison builds a SQL comparison expression from two Value operands.
//
// This function handles all combinations of operand types: field-to-field, field-to-value,
// value-to-field, and value-to-value comparisons. It validates that value-to-value comparisons
// have matching types and adds special constraints for AAS semantic ID fields, such as position
// constraints for specific key indices.
//
// Special handling for semantic IDs:
//   - Shorthand references ($sm#semanticId) add position = 0 constraint
//   - Specific key references ($sm#semanticId.keys[N].value) add position = N constraint
//   - Wildcard references ($sm#semanticId.keys[].value) match any position
//
// Parameters:
//   - leftOperand: The left side of the comparison (field or value)
//   - rightOperand: The right side of the comparison (field or value)
//   - operation: The comparison operator ($eq, $ne, $gt, $ge, $lt, $le)
//
// Returns:
//   - exp.Expression: A goqu expression representing the comparison with any necessary constraints
//   - error: An error if the operands are invalid, types don't match, or the operation is unsupported
func HandleComparison(leftOperand, rightOperand *Value, operation string) (exp.Expression, error) {

	// Normalize shorthand semanticId / descriptor shorthand fields to explicit keys[0].value
	// (e.g. $aasdesc#specificAssetIds[].externalSubjectId ->
	//  $aasdesc#specificAssetIds[].externalSubjectId.keys[0].value)
	normalizeSemanticShorthand(leftOperand)
	normalizeSemanticShorthand(rightOperand)

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
		lType := effectiveTypeWithCast(leftOperand)
		rType := effectiveTypeWithCast(rightOperand)
		if lType != "" && rType != "" && lType != rType {
			return nil, fmt.Errorf("cannot compare different value types: %s and %s",
				lType, rType)
		}
	}

	// Build the comparison expression
	comparisonExpr, err := buildComparisonExpression(leftSQL, rightSQL, operation)
	if err != nil {
		return nil, err
	}

	return applyArrayPositionConstraints(leftOperand, rightOperand, comparisonExpr)
}

// HandleStringOperation builds SQL expressions for string-specific operators.
func HandleStringOperation(leftOperand, rightOperand *Value, operation string) (exp.Expression, error) {
	normalizeSemanticShorthand(leftOperand)
	normalizeSemanticShorthand(rightOperand)

	leftSQL, err := toSQLComponent(leftOperand, "left")
	if err != nil {
		return nil, err
	}
	rightSQL, err := toSQLComponent(rightOperand, "right")
	if err != nil {
		return nil, err
	}

	stringExpr, err := buildStringOperationExpression(leftSQL, rightSQL, operation)
	if err != nil {
		return nil, err
	}

	return applyArrayPositionConstraints(leftOperand, rightOperand, stringExpr)
}

// stringValueToValue normalizes a StringValue into a Value so existing helpers can be reused.
func stringValueToValue(item StringValue) Value {
	switch {
	case item.Field != nil:
		return Value{Field: item.Field}
	case item.StrVal != nil:
		return Value{StrVal: item.StrVal}
	case item.Attribute != nil:
		return Value{Attribute: item.Attribute}
	case item.StrCast != nil:
		return Value{StrCast: item.StrCast}
	default:
		return Value{}
	}
}

// buildStringOperationExpression maps string operations to SQL expressions.
func buildStringOperationExpression(left interface{}, right interface{}, operation string) (exp.Expression, error) {
	switch operation {
	case "$contains":
		return goqu.L("? LIKE '%' || ? || '%'", left, right), nil
	case "$starts-with":
		return goqu.L("? LIKE ? || '%'", left, right), nil
	case "$ends-with":
		return goqu.L("? LIKE '%' || ?", left, right), nil
	case "$regex":
		// PostgreSQL regex match (case-sensitive). Use ~* if you need case-insensitive semantics.
		return goqu.L("? ~ ?", left, right), nil
	default:
		return nil, fmt.Errorf("unsupported string operation: %s", operation)
	}
}

// applyArrayPositionConstraints appends position constraints for array-indexed fields.
func applyArrayPositionConstraints(leftOperand, rightOperand *Value, baseExpr exp.Expression) (exp.Expression, error) {
	// Handle position constraints for fields with array indices
	// This applies to: semanticId.keys[], externalSubjectId.keys[], endpoints[], specificAssetIds[], submodelDescriptors[]

	// Check if either operand is a semanticId / descriptor specific key field
	isLeftSpecificKeyValueSemanticID := isSemanticIDSpecificKeyValueField(leftOperand, false)
	isRightSpecificKeyValueSemanticID := isSemanticIDSpecificKeyValueField(rightOperand, false)

	isLeftSpecificKeyTypeSemanticID := isSemanticIDSpecificKeyValueField(leftOperand, true)
	isRightSpecificKeyTypeSemanticID := isSemanticIDSpecificKeyValueField(rightOperand, true)

	// SpecificAssetId.externalSubjectId keys
	isLeftSpecificAssetExternalValue := isSpecificAssetExternalSubjectField(leftOperand, false)
	isRightSpecificAssetExternalValue := isSpecificAssetExternalSubjectField(rightOperand, false)
	isLeftSpecificAssetExternalType := isSpecificAssetExternalSubjectField(leftOperand, true)
	isRightSpecificAssetExternalType := isSpecificAssetExternalSubjectField(rightOperand, true)

	// Check if either operand has array indices (including inside casts)
	hasArrayIndex := isArrayFieldWithIndex(leftOperand) || isArrayFieldWithIndex(rightOperand)

	if !hasArrayIndex {
		// No arrays involved, return simple comparison
		return baseExpr, nil
	}

	// Determine which operand to use for extracting position
	operandToUse := unwrapFieldOperand(leftOperand)
	if isRightSpecificKeyValueSemanticID || isRightSpecificKeyTypeSemanticID || isRightSpecificAssetExternalValue || isRightSpecificAssetExternalType || isArrayFieldWithIndex(rightOperand) {
		operandToUse = unwrapFieldOperand(rightOperand)
	}

	if operandToUse == nil || operandToUse.Field == nil {
		return baseExpr, nil
	}
	fieldStr := string(*operandToUse.Field)

	// Unified array position constraint logic
	var positionConstraints []exp.Expression

	tokens := builder.TokenizeField(fieldStr)
	for _, token := range tokens {
		if arrayToken, ok := token.(builder.ArrayToken); ok {
			if arrayToken.Index >= 0 {
				var alias string
				if arrayToken.Name == "keys" && ((isLeftSpecificKeyValueSemanticID || isRightSpecificKeyValueSemanticID) ||
					(isLeftSpecificKeyTypeSemanticID || isRightSpecificKeyTypeSemanticID) ||
					(isLeftSpecificAssetExternalValue || isRightSpecificAssetExternalValue) ||
					(isLeftSpecificAssetExternalType || isRightSpecificAssetExternalType)) {
					alias = getReferenceKeyAlias(fieldStr)
				} else {
					alias = getArrayFieldAlias(fieldStr)
				}
				if alias != "" {
					positionConstraints = append(positionConstraints, goqu.I(alias+".position").Eq(arrayToken.Index))
				} else {
					return nil, fmt.Errorf("unknown array alias for field: %s", fieldStr)
				}
			}
		}
	}

	// Add all position constraints
	if len(positionConstraints) > 0 {
		pos := extractArrayIndex(tokens)
		if pos < 0 {
			allConstraints := append([]exp.Expression{baseExpr}, positionConstraints...)
			return goqu.And(allConstraints...), nil
		}
		existsExpr, ok, err := buildArrayExists(fieldStr, pos, baseExpr)
		if err != nil {
			return nil, err
		}
		if ok {
			return existsExpr, nil
		}
		// Fallback to previous AND behaviour if no EXISTS plan
		allConstraints := append([]exp.Expression{baseExpr}, positionConstraints...)
		return goqu.And(allConstraints...), nil
	}

	return baseExpr, nil
}

// normalizeSemanticShorthand expands known shorthand fields to their explicit keys[0].value form.
// This ensures later logic can uniformly handle position constraints on reference_key.position.
func normalizeSemanticShorthand(operand *Value) {
	inner := unwrapFieldOperand(operand)
	if inner == nil || inner.Field == nil {
		return
	}
	field := string(*inner.Field)
	// Already explicit -> nothing to do
	if strings.Contains(field, ".keys[") {
		return
	}
	if strings.HasSuffix(field, ".semanticId") || strings.HasSuffix(field, ".externalSubjectId") {
		field += ".keys[0].value"
		*inner.Field = ModelStringPattern(field)
	}

}

// getReferenceKeyAlias returns the alias used for the reference_key table depending on the field context.
// The alias must match the aliases used when joining the reference_key table elsewhere in the query builder.
func getReferenceKeyAlias(field string) string {
	if strings.Contains(field, "specificAssetIds") && strings.Contains(field, "externalSubjectId") {
		return "external_subject_reference_key"
	}
	if strings.HasPrefix(field, "$smdesc#") {
		return "smdesc_semantic_id_reference_key"
	}
	if strings.Contains(field, "submodelDescriptors") {
		return "aasdesc_submodel_descriptor_semantic_id_reference_key"
	}
	if strings.Contains(field, "$sm#semanticId") {
		return "semantic_id_reference_key"
	}
	// default for submodel semanticId and similar
	return "semantic_id_reference_key"
}

// isSpecificAssetExternalSubjectField checks for specificAssetId.externalSubjectId.keys[...] patterns
func isSpecificAssetExternalSubjectField(operand *Value, isTypeCheck bool) bool {
	suffix := "value"
	if isTypeCheck {
		suffix = "type"
	}
	inner := unwrapFieldOperand(operand)
	if inner == nil || inner.Field == nil {
		return false
	}
	field := string(*inner.Field)
	return strings.HasPrefix(field, "$aasdesc#specificAssetIds") && strings.Contains(field, ".externalSubjectId.keys[") && strings.HasSuffix(field, "]."+suffix)
}

// isArrayFieldWithIndex checks if a field contains an array with a specific index or wildcard
func isArrayFieldWithIndex(operand *Value) bool {
	inner := unwrapFieldOperand(operand)
	if inner == nil || inner.Field == nil {
		return false
	}
	field := string(*inner.Field)
	return strings.Contains(field, "[")
}

// getArrayFieldAlias returns the appropriate alias for position constraints based on the array field type
func getArrayFieldAlias(field string) string {
	// Handle endpoints arrays
	if strings.Contains(field, "#endpoints[") {
		if strings.HasPrefix(field, "$aasdesc#endpoints[") {
			return "aas_descriptor_endpoint"
		}
		if strings.HasPrefix(field, "$smdesc#endpoints[") {
			return "submodel_descriptor_endpoint"
		}
		if strings.Contains(field, "submodelDescriptors") && strings.Contains(field, ".endpoints[") {
			return "submodel_descriptor_endpoint"
		}
	}

	// Handle submodelDescriptors semanticId reference keys
	if strings.Contains(field, "submodelDescriptors") && strings.Contains(field, ".semanticId.keys[") {
		// Use the same alias as the reference_key join for submodel descriptor semanticIds
		return "aasdesc_submodel_descriptor_semantic_id_reference_key"
	}
	// TODO: extend here if new array-based fields are added to descriptors

	// Handle specificAssetIds arrays
	if strings.Contains(field, "specificAssetIds[") {
		return "specific_asset_id"
	}

	// Handle submodelDescriptors arrays
	if strings.Contains(field, "submodelDescriptors[") {
		return "submodel_descriptor"
	}

	return ""
}

func isSemanticIDSpecificKeyValueField(operand *Value, isTypeCheck bool) bool {
	suffix := "value"
	if isTypeCheck {
		suffix = "type"
	}
	inner := unwrapFieldOperand(operand)
	if inner == nil || inner.Field == nil {
		return false
	}
	field := string(*inner.Field)
	return strings.HasPrefix(field, "$sm#semanticId.keys[") && strings.HasSuffix(field, "]."+suffix)
}

func toSQLComponent(operand *Value, position string) (interface{}, error) {
	if operand == nil {
		return nil, fmt.Errorf("%s operand is nil", position)
	}
	if operand.Attribute != nil {
		return nil, fmt.Errorf("attribute operands are not supported in SQL evaluation")
	}

	// Handle casts first so they take precedence over any accidentally set literal/field.
	if operand.StrCast != nil {
		return castOperandToSQLType(operand.StrCast, position, "text")
	}
	if operand.NumCast != nil {
		return castOperandToSQLType(operand.NumCast, position, "double precision")
	}
	if operand.BoolCast != nil {
		return castOperandToSQLType(operand.BoolCast, position, "boolean")
	}
	if operand.TimeCast != nil {
		return castOperandToSQLType(operand.TimeCast, position, "time")
	}
	if operand.DateTimeCast != nil {
		return castOperandToSQLType(operand.DateTimeCast, position, "timestamptz")
	}
	if operand.HexCast != nil {
		return castOperandToSQLType(operand.HexCast, position, "text")
	}

	if operand.IsField() {
		if operand.Field == nil {
			return nil, fmt.Errorf("%s operand is not a valid field", position)
		}
		fieldName := string(*operand.Field)
		fieldName = ParseAASQLFieldToSQLColumn(fieldName)
		return goqu.I(fieldName), nil
	}

	return goqu.V(normalizeLiteralForSQL(operand.GetValue())), nil
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

// effectiveTypeWithCast prefers the target type of an explicit cast over the raw EffectiveType.
// This keeps type validation in sync with the SQL that will actually be generated.
func effectiveTypeWithCast(v *Value) string {
	if v == nil {
		return ""
	}
	switch {
	case v.NumCast != nil:
		return "number"
	case v.BoolCast != nil:
		return "bool"
	case v.TimeCast != nil:
		return "time"
	case v.DateTimeCast != nil:
		return "datetime"
	case v.HexCast != nil:
		return "hex"
	case v.StrCast != nil:
		return "string"
	default:
		return v.EffectiveType()
	}
}

// castOperandToSQLType recursively converts an operand to SQL and applies a PostgreSQL cast.
func castOperandToSQLType(inner *Value, position string, targetType string) (exp.Expression, error) {
	sqlValue, err := toSQLComponent(inner, position)
	if err != nil {
		return nil, err
	}
	// Guard casts so malformed input yields NULL instead of a PostgreSQL cast error.
	switch targetType {
	case "timestamptz":
		return goqu.L("CASE WHEN ?::text ~ ? THEN (?::timestamptz) END", sqlValue, `^[0-9]{4}-[0-9]{2}-[0-9]{2}T`, sqlValue), nil
	case "time":
		return goqu.L("CASE WHEN ?::text ~ ? THEN (?::time) END", sqlValue, `^[0-9]{2}:[0-9]{2}(:[0-9]{2})?$`, sqlValue), nil
	case "double precision":
		return goqu.L("CASE WHEN ?::text ~ ? THEN (?::double precision) END", sqlValue, `^\s*-?[0-9]+(\.[0-9]+)?\s*$`, sqlValue), nil
	case "boolean":
		return goqu.L("CASE WHEN lower(?::text) IN ('true','false','1','0','yes','no') THEN (?::boolean) END", sqlValue, sqlValue), nil
	default:
		// text/hex casts are always safe
		return goqu.L("?::"+targetType, sqlValue), nil
	}
}

// normalizeLiteralForSQL converts grammar literals to safe SQL encodable values.
func normalizeLiteralForSQL(v interface{}) interface{} {
	switch t := v.(type) {
	case DateTimeLiteralPattern:
		return normalizeTime(time.Time(t))
	case time.Time:
		return normalizeTime(t)
	default:
		return v
	}
}

// extractArrayIndex returns the first explicit array index in the token list, or -1 if none.
func extractArrayIndex(tokens []builder.Token) int {
	for _, token := range tokens {
		if at, ok := token.(builder.ArrayToken); ok && at.Index >= 0 {
			return at.Index
		}
	}
	return -1
}

// buildArrayExists converts an indexed array field predicate into a correlated EXISTS where possible.
// Returns (expr, true, nil) when handled, (nil, false, nil) when no EXISTS plan is available.
func buildArrayExists(fieldStr string, position int, predicate exp.Expression) (exp.Expression, bool, error) {
	switch {
	case strings.Contains(fieldStr, "specificAssetIds["):
		alias := "specific_asset_id"
		sub := goqu.From(goqu.T(alias)).
			Where(
				goqu.I(alias+".descriptor_id").Eq(goqu.I("descriptor.id")),
				goqu.I(alias+".position").Eq(position),
				predicate,
			).
			Select(goqu.V(1))
		return goqu.L("EXISTS (?)", sub), true, nil

	case strings.Contains(fieldStr, "#endpoints[") && strings.HasPrefix(fieldStr, "$aasdesc#"):
		alias := "aas_descriptor_endpoint"
		sub := goqu.From(goqu.T(alias)).
			Where(
				goqu.I(alias+".descriptor_id").Eq(goqu.I("descriptor.id")),
				goqu.I(alias+".position").Eq(position),
				predicate,
			).
			Select(goqu.V(1))
		return goqu.L("EXISTS (?)", sub), true, nil

	case strings.Contains(fieldStr, "$aasdesc#submodelDescriptors[") && !strings.Contains(fieldStr, ".endpoints["):
		alias := "submodel_descriptor"
		sub := goqu.From(goqu.T(alias)).
			Where(
				goqu.I(alias+".aas_descriptor_id").Eq(goqu.I("descriptor.id")),
				goqu.I(alias+".position").Eq(position),
				predicate,
			).
			Select(goqu.V(1))
		return goqu.L("EXISTS (?)", sub), true, nil

	case strings.Contains(fieldStr, "$aasdesc#submodelDescriptors[") && strings.Contains(fieldStr, ".endpoints["):
		// Correlate endpoint to its submodel descriptor, then to outer descriptor
		epAlias := "submodel_descriptor_endpoint"
		smdAlias := "submodel_descriptor"
		sub := goqu.From(goqu.T(epAlias)).
			Join(
				goqu.T(smdAlias),
				goqu.On(
					goqu.I(smdAlias+".descriptor_id").Eq(goqu.I(epAlias+".descriptor_id")),
				),
			).
			Where(
				goqu.I(smdAlias+".aas_descriptor_id").Eq(goqu.I("descriptor.id")),
				goqu.I(epAlias+".position").Eq(position),
				predicate,
			).
			Select(goqu.V(1))
		return goqu.L("EXISTS (?)", sub), true, nil

	case strings.Contains(fieldStr, "$aasdesc#submodelDescriptors") && strings.Contains(fieldStr, ".semanticId.keys["):
		// Reference keys of submodel descriptor semanticId
		refKey := "aasdesc_submodel_descriptor_semantic_id_reference_key"
		smdAlias := "submodel_descriptor"
		sub := goqu.From(goqu.T(refKey)).
			Join(
				goqu.T(smdAlias),
				goqu.On(
					goqu.I(smdAlias+".semantic_id").Eq(goqu.I(refKey+".reference_id")),
				),
			).
			Where(
				goqu.I(smdAlias+".aas_descriptor_id").Eq(goqu.I("descriptor.id")),
				goqu.I(refKey+".position").Eq(position),
				predicate,
			).
			Select(goqu.V(1))
		return goqu.L("EXISTS (?)", sub), true, nil

	case strings.HasPrefix(fieldStr, "$sm#semanticId.keys["):
		refKey := "semantic_id_reference_key"
		ref := "semantic_id_reference"
		sub := goqu.From(goqu.T(refKey)).
			Join(
				goqu.T(ref),
				goqu.On(goqu.I(ref+".id").Eq(goqu.I(refKey+".reference_id"))),
			).
			Where(
				goqu.I(ref+".id").Eq(goqu.I("descriptor.semantic_id")),
				goqu.I(refKey+".position").Eq(position),
				predicate,
			).
			Select(goqu.V(1))
		return goqu.L("EXISTS (?)", sub), true, nil

	case strings.Contains(fieldStr, "specificAssetIds") && strings.Contains(fieldStr, ".externalSubjectId.keys["):
		// externalSubjectId keys are keyed off the specificAssetId's external_subject_ref
		refKey := "external_subject_reference_key"
		sai := "specific_asset_id"
		sub := goqu.From(goqu.T(refKey)).
			Join(
				goqu.T(sai),
				goqu.On(goqu.I(sai+".external_subject_ref").Eq(goqu.I(refKey+".reference_id"))),
			).
			Where(
				goqu.I(sai+".descriptor_id").Eq(goqu.I("descriptor.id")),
				goqu.I(sai+".position").Eq(position),
				goqu.I(refKey+".position").Eq(position),
				predicate,
			).
			Select(goqu.V(1))
		return goqu.L("EXISTS (?)", sub), true, nil
	}

	return nil, false, nil
}

// unwrapFieldOperand walks through cast wrappers to find the underlying field operand.
// This allows array position logic to work even when a field is wrapped in $numCast/$timeCast/etc.
func unwrapFieldOperand(v *Value) *Value {
	cur := v
	for cur != nil {
		if cur.Field != nil {
			return cur
		}
		switch {
		case cur.StrCast != nil:
			cur = cur.StrCast
		case cur.NumCast != nil:
			cur = cur.NumCast
		case cur.BoolCast != nil:
			cur = cur.BoolCast
		case cur.TimeCast != nil:
			cur = cur.TimeCast
		case cur.DateTimeCast != nil:
			cur = cur.DateTimeCast
		case cur.HexCast != nil:
			cur = cur.HexCast
		default:
			return nil
		}
	}
	return nil
}

func normalizeTime(t time.Time) time.Time {
	// Ensure location is set to avoid goqu encoding errors.
	if t.Location() == nil {
		return time.Unix(0, t.UnixNano()).UTC()
	}
	return t.UTC()
}
