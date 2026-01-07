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
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE ), Martin Stemmer ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"fmt"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

type existsJoinRule struct {
	Alias string
	Deps  []string
	Apply func(ds *goqu.SelectDataset) *goqu.SelectDataset
}

// extractFieldOperandAndCast walks through cast wrappers to find the underlying field operand
// and returns the outermost cast target type (if any).
func extractFieldOperandAndCast(v *Value) (*Value, string) {
	cur := v
	castType := ""
	for cur != nil {
		// Record only the outermost cast.
		if castType == "" {
			switch {
			case cur.StrCast != nil:
				castType = "text"
			case cur.NumCast != nil:
				castType = "double precision"
			case cur.BoolCast != nil:
				castType = "boolean"
			case cur.TimeCast != nil:
				castType = "time"
			case cur.DateTimeCast != nil:
				castType = "timestamptz"
			case cur.HexCast != nil:
				castType = "text"
			}
		}

		if cur.Field != nil {
			return cur, castType
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
			return nil, ""
		}
	}
	return nil, ""
}

func toSQLResolvedFieldOrValue(operand *Value, explicitCastType string, position string) (interface{}, *ResolvedFieldPath, error) {
	fieldOperand, _ := extractFieldOperandAndCast(operand)
	if fieldOperand == nil || fieldOperand.Field == nil {
		val, err := toSQLComponent(operand, position)
		return val, nil, err
	}
	fieldStr := string(*fieldOperand.Field)
	f := ModelStringPattern(fieldStr)
	resolved, err := ResolveScalarFieldToSQL(&f)
	if err != nil {
		return nil, nil, err
	}
	ident := goqu.I(resolved.Column)
	if explicitCastType != "" {
		return safeCastSQLValue(ident, explicitCastType), &resolved, nil
	}
	return ident, &resolved, nil
}

func anyResolvedHasBindings(resolved []ResolvedFieldPath) bool {
	for _, r := range resolved {
		if len(r.ArrayBindings) > 0 {
			return true
		}
	}
	return false
}

func collectResolvedFieldPaths(a, b *ResolvedFieldPath) []ResolvedFieldPath {
	var out []ResolvedFieldPath
	if a != nil {
		out = append(out, *a)
	}
	if b != nil {
		out = append(out, *b)
	}
	return out
}

func sqlTypeForOperand(v *Value) string {
	if v == nil {
		return ""
	}
	switch {
	case v.StrVal != nil:
		return "text"
	case v.NumVal != nil:
		return "double precision"
	case v.Boolean != nil:
		return "boolean"
	case v.TimeVal != nil:
		return "time"
	case v.DateTimeVal != nil:
		return "timestamptz"
	default:
		return ""
	}
}

func leadingAlias(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", false
	}
	idx := strings.Index(expr, ".")
	if idx <= 0 {
		return "", false
	}
	return expr[:idx], true
}

func requiredAliasesFromResolved(resolved []ResolvedFieldPath) (map[string]struct{}, error) {
	req := map[string]struct{}{}
	for _, r := range resolved {
		if strings.TrimSpace(r.Column) != "" {
			a, ok := leadingAlias(r.Column)
			if !ok {
				return nil, fmt.Errorf("cannot extract alias from column %q", r.Column)
			}
			req[a] = struct{}{}
		}
		for _, b := range r.ArrayBindings {
			a, ok := leadingAlias(b.Alias)
			if !ok {
				return nil, fmt.Errorf("cannot extract alias from binding alias %q", b.Alias)
			}
			req[a] = struct{}{}
		}
	}
	return req, nil
}

func buildExistsForResolvedFieldPaths(resolved []ResolvedFieldPath, predicate exp.Expression) (exp.Expression, error) {
	required, err := requiredAliasesFromResolved(resolved)
	if err != nil {
		return nil, err
	}

	// Choose a base alias with a direct correlation to outer descriptor.
	base := ""
	if _, ok := required["specific_asset_id"]; ok {
		base = "specific_asset_id"
	} else if _, ok := required["aas_descriptor_endpoint"]; ok {
		base = "aas_descriptor_endpoint"
	} else if _, ok := required["submodel_descriptor"]; ok {
		base = "submodel_descriptor"
	} else {
		// Best-effort fallback: pick any alias.
		for a := range required {
			base = a
			break
		}
	}
	if base == "" {
		return nil, fmt.Errorf("cannot build EXISTS: no aliases required")
	}

	rules := existsJoinRulesForAASDescriptors()
	baseTable, ok := existsTableForAlias(base)
	if !ok {
		return nil, fmt.Errorf("cannot build EXISTS: no table mapping for alias %q", base)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T(baseTable).As(base)).Select(goqu.V(1))

	applied := map[string]struct{}{base: {}}
	visiting := map[string]struct{}{}

	var ensure func(alias string) error
	ensure = func(alias string) error {
		if alias == "" {
			return nil
		}
		if _, ok := applied[alias]; ok {
			return nil
		}
		if _, ok := visiting[alias]; ok {
			return fmt.Errorf("cyclic EXISTS join dependency for alias %q", alias)
		}
		rule, ok := rules[alias]
		if !ok {
			return fmt.Errorf("no EXISTS join rule registered for alias %q", alias)
		}
		visiting[alias] = struct{}{}
		for _, dep := range rule.Deps {
			if err := ensure(dep); err != nil {
				return err
			}
		}
		delete(visiting, alias)
		ds = rule.Apply(ds)
		applied[alias] = struct{}{}
		return nil
	}

	for alias := range required {
		if alias == base {
			continue
		}
		if err := ensure(alias); err != nil {
			return nil, err
		}
	}

	where := []exp.Expression{predicate}
	if corr := existsCorrelationForAlias(base); corr != nil {
		where = append(where, corr)
	}

	// Add all binding constraints.
	for _, r := range resolved {
		for _, b := range r.ArrayBindings {
			// Only apply non-nil binding values.
			if b.Index.intValue != nil {
				where = append(where, goqu.I(b.Alias).Eq(*b.Index.intValue))
			}
			if b.Index.stringValue != nil {
				where = append(where, goqu.I(b.Alias).Eq(*b.Index.stringValue))
			}
		}
	}

	ds = ds.Where(goqu.And(where...))
	return goqu.L("EXISTS (?)", ds), nil
}

func andBindingsForResolvedFieldPaths(resolved []ResolvedFieldPath, predicate exp.Expression) exp.Expression {
	where := []exp.Expression{predicate}
	for _, r := range resolved {
		for _, b := range r.ArrayBindings {
			if b.Index.intValue != nil {
				where = append(where, goqu.I(b.Alias).Eq(*b.Index.intValue))
			}
			if b.Index.stringValue != nil {
				where = append(where, goqu.I(b.Alias).Eq(*b.Index.stringValue))
			}
		}
	}
	return goqu.And(where...)
}

func existsTableForAlias(alias string) (string, bool) {
	switch alias {
	case "specific_asset_id":
		return "specific_asset_id", true
	case "external_subject_reference":
		return "reference", true
	case "external_subject_reference_key":
		return "reference_key", true
	case "aas_descriptor_endpoint":
		return "aas_descriptor_endpoint", true
	case "submodel_descriptor":
		return "submodel_descriptor", true
	case "submodel_descriptor_endpoint":
		return "aas_descriptor_endpoint", true
	case "aasdesc_submodel_descriptor_semantic_id_reference":
		return "reference", true
	case "aasdesc_submodel_descriptor_semantic_id_reference_key":
		return "reference_key", true
	default:
		return "", false
	}
}

func existsCorrelationForAlias(base string) exp.Expression {
	switch base {
	case "specific_asset_id":
		return goqu.I("specific_asset_id.descriptor_id").Eq(goqu.I("descriptor.id"))
	case "aas_descriptor_endpoint":
		return goqu.I("aas_descriptor_endpoint.descriptor_id").Eq(goqu.I("descriptor.id"))
	case "submodel_descriptor":
		// submodel_descriptor.aas_descriptor_id points to the AAS descriptor (descriptor.id)
		return goqu.I("submodel_descriptor.aas_descriptor_id").Eq(goqu.I("descriptor.id"))
	default:
		return nil
	}
}

func existsJoinRulesForAASDescriptors() map[string]existsJoinRule {
	return map[string]existsJoinRule{
		"external_subject_reference": {
			Alias: "external_subject_reference",
			Deps:  []string{"specific_asset_id"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("reference").As("external_subject_reference"),
					goqu.On(goqu.I("external_subject_reference.id").Eq(goqu.I("specific_asset_id.external_subject_ref"))),
				)
			},
		},
		"external_subject_reference_key": {
			Alias: "external_subject_reference_key",
			Deps:  []string{"external_subject_reference"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("reference_key").As("external_subject_reference_key"),
					goqu.On(goqu.I("external_subject_reference_key.reference_id").Eq(goqu.I("external_subject_reference.id"))),
				)
			},
		},
		"submodel_descriptor_endpoint": {
			Alias: "submodel_descriptor_endpoint",
			Deps:  []string{"submodel_descriptor"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("aas_descriptor_endpoint").As("submodel_descriptor_endpoint"),
					goqu.On(goqu.I("submodel_descriptor_endpoint.descriptor_id").Eq(goqu.I("submodel_descriptor.descriptor_id"))),
				)
			},
		},
		"aasdesc_submodel_descriptor_semantic_id_reference": {
			Alias: "aasdesc_submodel_descriptor_semantic_id_reference",
			Deps:  []string{"submodel_descriptor"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("reference").As("aasdesc_submodel_descriptor_semantic_id_reference"),
					goqu.On(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference.id").Eq(goqu.I("submodel_descriptor.semantic_id"))),
				)
			},
		},
		"aasdesc_submodel_descriptor_semantic_id_reference_key": {
			Alias: "aasdesc_submodel_descriptor_semantic_id_reference_key",
			Deps:  []string{"aasdesc_submodel_descriptor_semantic_id_reference"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("reference_key").As("aasdesc_submodel_descriptor_semantic_id_reference_key"),
					goqu.On(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference_key.reference_id").Eq(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference.id"))),
				)
			},
		},
	}
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

	leftField, leftCastType := extractFieldOperandAndCast(leftOperand)
	rightField, rightCastType := extractFieldOperandAndCast(rightOperand)

	// Field-to-field comparisons are forbidden by the query language.
	// We can safely assume comparisons have either 0 or 1 field operands.
	if leftField != nil && rightField != nil {
		return nil, fmt.Errorf("field-to-field comparisons are not supported")
	}

	// Fast-path: both are values (no FieldIdentifiers involved).
	if leftField == nil && rightField == nil {
		leftSQL, err := toSQLComponent(leftOperand, "left")
		if err != nil {
			return nil, err
		}
		rightSQL, err := toSQLComponent(rightOperand, "right")
		if err != nil {
			return nil, err
		}
		// has to be compatible
		_, err = leftOperand.IsComparableTo(*rightOperand)
		if err != nil {
			return nil, err
		}
		return buildComparisonExpression(leftSQL, rightSQL, operation)
	}

	leftSQL, leftResolved, err := toSQLResolvedFieldOrValue(leftOperand, leftCastType, "left")
	if err != nil {
		return nil, err
	}
	rightSQL, rightResolved, err := toSQLResolvedFieldOrValue(rightOperand, rightCastType, "right")
	if err != nil {
		return nil, err
	}

	// Cast the field side to the non-field operand's type (unless already explicitly casted).
	if leftResolved != nil && rightResolved == nil && leftCastType == "" {
		if t := sqlTypeForOperand(rightOperand); t != "" {
			leftSQL = safeCastSQLValue(goqu.I(leftResolved.Column), t)
		}
	}
	if rightResolved != nil && leftResolved == nil && rightCastType == "" {
		if t := sqlTypeForOperand(leftOperand); t != "" {
			rightSQL = safeCastSQLValue(goqu.I(rightResolved.Column), t)
		}
	}

	// has to be compatible
	_, err = leftOperand.IsComparableTo(*rightOperand)
	if err != nil {
		return nil, err
	}

	comparisonExpr, err := buildComparisonExpression(leftSQL, rightSQL, operation)
	if err != nil {
		return nil, err
	}

	resolved := collectResolvedFieldPaths(leftResolved, rightResolved)
	// No resolved fields (should not happen due to earlier fast-path), fall back.
	if len(resolved) == 0 {
		return comparisonExpr, nil
	}

	// If any resolved path has bindings, build a correlated EXISTS with joins + constraints.
	if anyResolvedHasBindings(resolved) {
		if existsExpr, err := buildExistsForResolvedFieldPaths(resolved, comparisonExpr); err == nil {
			return existsExpr, nil
		}
		// Fallback: if we cannot build an EXISTS join graph for the involved aliases,
		// apply bindings as plain AND constraints.
		return andBindingsForResolvedFieldPaths(resolved, comparisonExpr), nil
	}
	return comparisonExpr, nil
}

// HandleStringOperation builds SQL expressions for string-specific operators.
func HandleStringOperation(leftOperand, rightOperand *Value, operation string) (exp.Expression, error) {
	normalizeSemanticShorthand(leftOperand)
	normalizeSemanticShorthand(rightOperand)

	leftField, leftCastType := extractFieldOperandAndCast(leftOperand)
	rightField, rightCastType := extractFieldOperandAndCast(rightOperand)

	// Field-to-field string operations are forbidden by the query language.
	// We can safely assume string operations have either 0 or 1 field operands.
	if leftField != nil && rightField != nil {
		return nil, fmt.Errorf("field-to-field string operations are not supported")
	}

	// Fast-path: no FieldIdentifiers involved.
	if leftField == nil && rightField == nil {
		leftSQL, err := toSQLComponent(leftOperand, "left")
		if err != nil {
			return nil, err
		}
		rightSQL, err := toSQLComponent(rightOperand, "right")
		if err != nil {
			return nil, err
		}
		return buildStringOperationExpression(leftSQL, rightSQL, operation)
	}

	leftSQL, leftResolved, err := toSQLResolvedFieldOrValue(leftOperand, leftCastType, "left")
	if err != nil {
		return nil, err
	}
	rightSQL, rightResolved, err := toSQLResolvedFieldOrValue(rightOperand, rightCastType, "right")
	if err != nil {
		return nil, err
	}

	if leftResolved != nil && rightResolved == nil && leftCastType == "" {
		if t := sqlTypeForOperand(rightOperand); t != "" {
			leftSQL = safeCastSQLValue(goqu.I(leftResolved.Column), t)
		}
	}
	if rightResolved != nil && leftResolved == nil && rightCastType == "" {
		if t := sqlTypeForOperand(leftOperand); t != "" {
			rightSQL = safeCastSQLValue(goqu.I(rightResolved.Column), t)
		}
	}

	stringExpr, err := buildStringOperationExpression(leftSQL, rightSQL, operation)
	if err != nil {
		return nil, err
	}

	resolved := collectResolvedFieldPaths(leftResolved, rightResolved)
	if len(resolved) == 0 {
		return stringExpr, nil
	}
	if anyResolvedHasBindings(resolved) {
		if existsExpr, err := buildExistsForResolvedFieldPaths(resolved, stringExpr); err == nil {
			return existsExpr, nil
		}
		return andBindingsForResolvedFieldPaths(resolved, stringExpr), nil
	}
	return stringExpr, nil
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

// normalizeSemanticShorthand expands known shorthand fields to their explicit keys[0].value form.
func normalizeSemanticShorthand(operand *Value) {
	inner, _ := extractFieldOperandAndCast(operand)
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
		f := ModelStringPattern(fieldName)
		resolved, err := ResolveScalarFieldToSQL(&f)
		if err != nil {
			return nil, err
		}
		return goqu.I(resolved.Column), nil
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

// safeCastSQLValue applies a PostgreSQL cast to the provided SQL value.
//
// For types that can raise runtime errors (e.g. timestamptz, time, numeric, boolean), the cast is guarded
// so non-castable inputs yield NULL instead of a PostgreSQL cast error.
// This is critical for security rules: a failed cast should simply cause the predicate to not match.
func safeCastSQLValue(sqlValue interface{}, targetType string) exp.Expression {
	switch targetType {
	case "timestamptz":
		return goqu.L("CASE WHEN ?::text ~ ? THEN (?::timestamptz) END", sqlValue, `^[0-9]{4}-[0-9]{2}-[0-9]{2}T`, sqlValue)
	case "time":
		return goqu.L("CASE WHEN ?::text ~ ? THEN (?::time) END", sqlValue, `^[0-9]{2}:[0-9]{2}(:[0-9]{2})?$`, sqlValue)
	case "double precision":
		return goqu.L("CASE WHEN ?::text ~ ? THEN (?::double precision) END", sqlValue, `^\s*-?[0-9]+(\.[0-9]+)?\s*$`, sqlValue)
	case "boolean":
		return goqu.L("CASE WHEN lower(?::text) IN ('true','false','1','0','yes','no') THEN (?::boolean) END", sqlValue, sqlValue)
	default:
		// text/hex casts are always safe
		return goqu.L("?::"+targetType, sqlValue)
	}
}

// castOperandToSQLType recursively converts an operand to SQL and applies a PostgreSQL cast.
func castOperandToSQLType(inner *Value, position string, targetType string) (exp.Expression, error) {
	sqlValue, err := toSQLComponent(inner, position)
	if err != nil {
		return nil, err
	}
	return safeCastSQLValue(sqlValue, targetType), nil
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

func normalizeTime(t time.Time) time.Time {
	// Ensure location is set to avoid goqu encoding errors.
	if t.Location() == nil {
		return time.Unix(0, t.UnixNano()).UTC()
	}
	return t.UTC()
}
