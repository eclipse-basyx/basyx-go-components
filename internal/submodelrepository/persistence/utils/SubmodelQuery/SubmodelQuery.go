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
package submodel_query

import (
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/querylanguage"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/queries"
)

// getQueryWithGoqu constructs a comprehensive SQL query for retrieving submodel data.
//
// This function builds a complex PostgreSQL query using the goqu query builder that:
//   - Aggregates nested data structures (display names, descriptions, references) as JSON
//   - Handles hierarchical reference structures with parent-child relationships
//   - Retrieves embedded data specifications including IEC 61360 content
//   - Manages supplemental semantic IDs and their referred references
//   - Optimally joins multiple tables while avoiding duplication
//
// The query structure includes multiple subqueries for:
//   - Display names and descriptions (multi-language support)
//   - Semantic IDs and their referred references
//   - Supplemental semantic IDs and their hierarchies
//   - Embedded data specifications with IEC 61360 content
//   - Value lists and level types
//
// Parameters:
//   - submodelId: Optional filter for a specific submodel ID. Empty string retrieves all submodels.
//
// Returns:
//   - string: The complete SQL query string ready for execution
//   - error: An error if query generation fails
//
// The function uses COALESCE to ensure empty arrays ('[]'::jsonb) instead of NULL values,
// which simplifies downstream JSON parsing. It also includes a total count window function
// for efficient result set pagination and slice pre-sizing.
func GetQueryWithGoqu(submodelId string, limit int64, cursor string, aasQuery *querylanguage.QueryObj) (string, error) {
	dialect := goqu.Dialect("postgres")

	// Build display names subquery
	displayNameObj := goqu.Func("jsonb_build_object",
		goqu.V("language"), goqu.I("dn.language"),
		goqu.V("text"), goqu.I("dn.text"),
		goqu.V("id"), goqu.I("dn.id"),
	)

	displayNamesSubquery := dialect.From(goqu.T("lang_string_name_type_reference").As("dn_ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", displayNameObj))).
		Join(
			goqu.T("lang_string_name_type").As("dn"),
			goqu.On(goqu.I("dn.lang_string_name_type_reference_id").Eq(goqu.I("dn_ref.id"))),
		).
		Where(goqu.I("dn_ref.id").Eq(goqu.I("s.displayname_id")))

	// Build descriptions subquery
	descriptionObj := goqu.Func("jsonb_build_object",
		goqu.V("language"), goqu.I("d.language"),
		goqu.V("text"), goqu.I("d.text"),
		goqu.V("id"), goqu.I("d.id"),
	)

	descriptionsSubquery := dialect.From(goqu.T("lang_string_text_type_reference").As("dr")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", descriptionObj))).
		Join(
			goqu.T("lang_string_text_type").As("d"),
			goqu.On(goqu.I("d.lang_string_text_type_reference_id").Eq(goqu.I("dr.id"))),
		).
		Where(goqu.I("dr.id").Eq(goqu.I("s.description_id")))

	semanticIdSubquery, semanticIdReferredSubquery := queries.GetReferenceQueries(dialect, goqu.I("s.semantic_id"))

	// Build supplemental semantic ids subquery
	supplementalSemanticIdObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("ref.id"),
		goqu.V("reference_type"), goqu.I("ref.type"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	supplementalSemanticIdsSubquery := dialect.From(goqu.T("submodel_supplemental_semantic_id").As("sssi")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", supplementalSemanticIdObj))).
		LeftJoin(
			goqu.T("reference").As("ref"),
			goqu.On(goqu.I("ref.id").Eq(goqu.I("sssi.reference_id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("ref.id"))),
		).
		Where(goqu.I("sssi.submodel_id").Eq(goqu.I("s.id")))

	// Build supplemental semantic ids referred subquery
	supplementalSemanticIdReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("supplemental_root_reference_id"), goqu.I("sssi.reference_id"),
		goqu.V("reference_id"), goqu.I("ref.id"),
		goqu.V("reference_type"), goqu.I("ref.type"),
		goqu.V("parentReference"), goqu.I("ref.parentreference"),
		goqu.V("rootReference"), goqu.I("ref.rootreference"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	supplementalSemanticIdsReferredSubquery := dialect.From(goqu.T("submodel_supplemental_semantic_id").As("sssi")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", supplementalSemanticIdReferredObj))).
		LeftJoin(
			goqu.T("reference").As("ref"),
			goqu.On(goqu.I("ref.rootreference").Eq(goqu.I("sssi.reference_id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("ref.id"))),
		).
		Where(
			goqu.I("sssi.submodel_id").Eq(goqu.I("s.id")),
			goqu.I("ref.id").IsNotNull(),
		)
	// Build embedded data specifications subquery
	embeddedDataSpecificationReferenceSubquery, embeddedDataSpecificationReferenceReferredSubquery, iec61360Subquery := GetEmbeddedDataSpecificationSubqueries(dialect, "submodel_embedded_data_specification", "submodel_id", "s.id")

	// Build qualifier subquery
	qualifierSubquery := GetQualifierSubqueryForSubmodel(dialect)

	// Build extension subquery
	extensionSubquery := GetExtensionSubqueryForSubmodel(dialect)

	// Build AdministrativeInformation subquery
	administrationSubquery := GetAdministrationSubqueryForSubmodel(dialect)

	// Main query
	query := dialect.From(goqu.T("submodel").As("s")).
		Select(
			goqu.I("s.id").As("submodel_id"),
			goqu.I("s.id_short").As("submodel_id_short"),
			goqu.I("s.category").As("submodel_category"),
			goqu.I("s.kind").As("submodel_kind"),
			goqu.L("COALESCE((?), '[]'::jsonb)", displayNamesSubquery).As("submodel_display_names"),
			goqu.L("COALESCE((?), '[]'::jsonb)", descriptionsSubquery).As("submodel_descriptions"),
			goqu.L("COALESCE((?), '[]'::jsonb)", semanticIdSubquery).As("submodel_semantic_id"),
			goqu.L("COALESCE((?), '[]'::jsonb)", semanticIdReferredSubquery).As("submodel_semantic_id_referred"),
			goqu.L("COALESCE((?), '[]'::jsonb)", supplementalSemanticIdsSubquery).As("submodel_supplemental_semantic_ids"),
			goqu.L("COALESCE((?), '[]'::jsonb)", supplementalSemanticIdsReferredSubquery).As("submodel_supplemental_semantic_id_referred"),
			goqu.L("COALESCE((?), '[]'::jsonb)", embeddedDataSpecificationReferenceSubquery).As("submodel_eds_data_specification"),
			goqu.L("COALESCE((?), '[]'::jsonb)", embeddedDataSpecificationReferenceReferredSubquery).As("submodel_eds_data_specification_referred"),
			goqu.L("COALESCE((?), '[]'::jsonb)", iec61360Subquery).As("submodel_data_spec_iec61360"),
			goqu.L("COALESCE((?), '[]'::jsonb)", qualifierSubquery).As("submodel_qualifiers"),
			goqu.L("COALESCE((?), '[]'::jsonb)", extensionSubquery).As("submodel_extensions"),
			goqu.L("COALESCE((?), '[]'::jsonb)", administrationSubquery).As("submodel_administrative_information"),
			goqu.L("COALESCE((?), '[]'::jsonb)", GetSubmodelElementsSubquery(dialect, true)).As("submodel_root_submodel_elements"),
			goqu.L("COALESCE((?), '[]'::jsonb)", GetSubmodelElementsSubquery(dialect, false)).As("submodel_child_submodel_elements"),
		)

	// Add optional WHERE clause for submodel ID filtering
	if submodelId != "" {
		query = addSubmodelIdFilterToQuery(query, submodelId)
	}

	// Add optional AAS QueryLanguage filtering
	if aasQuery != nil {
		query = addJoinsToQueryForAASQL(query)

		var err error
		query, err = applyAASQuery(aasQuery, query)
		if err != nil {
			return "", fmt.Errorf("error applying AAS QueryLanguage filtering: %w", err)
		}
	}

	query = addSubmodelCountToQuery(query)
	query = addGroupBySubmodelId(query)

	// Add pagination if limit or cursor is specified
	query = addPaginationToQuery(query, limit, cursor)

	sql, _, err := query.ToSQL()
	if err != nil {
		return "", fmt.Errorf("error generating SQL: %w", err)
	}

	return sql, nil
}

func addGroupBySubmodelId(query *goqu.SelectDataset) *goqu.SelectDataset {
	query = query.GroupBy(goqu.I("s.id"))
	return query
}

func addSubmodelCountToQuery(query *goqu.SelectDataset) *goqu.SelectDataset {
	query = query.SelectAppend(goqu.L("COUNT(s.id) OVER() AS total_submodels"))
	return query
}

func addSubmodelIdFilterToQuery(query *goqu.SelectDataset, submodelId string) *goqu.SelectDataset {
	query = query.Where(goqu.I("s.id").Eq(submodelId))
	return query
}

func addJoinsToQueryForAASQL(query *goqu.SelectDataset) *goqu.SelectDataset {
	query = query.Join(goqu.T("reference").As("semantic_id_reference"), goqu.On(goqu.I("s.semantic_id").Eq(goqu.I("semantic_id_reference.id"))))
	query = query.Join(goqu.T("reference_key").As("semantic_id_reference_key"), goqu.On(goqu.I("semantic_id_reference.id").Eq(goqu.I("semantic_id_reference_key.reference_id"))))
	return query
}

func applyAASQuery(aasQuery *querylanguage.QueryObj, query *goqu.SelectDataset) (*goqu.SelectDataset, error) {
	ql := *aasQuery
	switch ql.Query.Condition.GetConditionType() {
	case "Comparison":
		comp := ql.Query.Condition.(*querylanguage.Comparison)
		operation := comp.GetOperationType()
		operands := comp.GetOperation().GetOperands()
		if len(operands) != 2 {
			return nil, fmt.Errorf("comparison operation requires exactly 2 operands, got %d", len(operands))
		}

		leftOperand := operands[0]
		rightOperand := operands[1]

		// if (leftOperand.GetOperandType() == "$field" && leftOperand.GetValue().(string)[4:] == "semanticId") ||
		// 	(rightOperand.GetOperandType() == "$field" && rightOperand.GetValue().(string)[4:] == "semanticId") {
		// }

		// Handle the case where left is field and right is value
		if leftOperand.GetOperandType() == "$field" && rightOperand.GetOperandType() != "$field" {
			exp, err := querylanguage.HandleFieldToValueComparison(leftOperand, rightOperand, operation)
			if err != nil {
				return nil, fmt.Errorf("error handling field-to-value comparison: %w", err)
			}
			query = query.Where(exp)
		} else if leftOperand.GetOperandType() != "$field" && rightOperand.GetOperandType() == "$field" {
			exp, err := querylanguage.HandleValueToFieldComparison(leftOperand, rightOperand, operation)
			if err != nil {
				return nil, fmt.Errorf("error handling value-to-field comparison: %w", err)
			}
			query = query.Where(exp)
		} else if leftOperand.GetOperandType() == "$field" && rightOperand.GetOperandType() == "$field" {
			exp, err := querylanguage.HandleFieldToFieldComparison(leftOperand, rightOperand, operation)
			if err != nil {
				return nil, fmt.Errorf("error handling value-to-field comparison: %w", err)
			}
			query = query.Where(exp)
		} else if leftOperand.GetOperandType() != "$field" && rightOperand.GetOperandType() != "$field" {
			exp, err := querylanguage.HandleValueToValueComparison(leftOperand, rightOperand, operation)
			if err != nil {
				return nil, fmt.Errorf("error handling value-to-value comparison: %w", err)
			}
			query = query.Where(exp)
		} else {
			return nil, fmt.Errorf("unsupported operand combination: left=%s, right=%s",
				leftOperand.GetOperandType(), rightOperand.GetOperandType())
		}

	case "LogicalExpression":
		logicalExpr := ql.Query.Condition.(*querylanguage.LogicalExpression)
		exp, err := logicalExpr.EvaluateToExpression()
		if err != nil {
			return nil, fmt.Errorf("error evaluating logical expression: %w", err)
		}
		query = query.Where(exp)
	case "Match":
		return nil, fmt.Errorf("unsupported query condition type: %s", ql.Query.Condition.GetConditionType())
	default:
		return nil, fmt.Errorf("unsupported query condition type: %s", ql.Query.Condition.GetConditionType())
	}
	return query, nil
}

// addPaginationToQuery adds cursor-based pagination to the query.
//
// Parameters:
//   - query: The goqu query to add pagination to
//   - limit: Maximum number of results to return (0 means no limit)
//   - cursor: The submodel ID to start pagination from (empty string means start from beginning)
//
// Returns:
//   - *goqu.SelectDataset: The query with pagination applied
//
// The function implements cursor-based pagination where:
//   - cursor is the submodel ID to start from (exclusive - starts after the cursor)
//   - limit controls the maximum number of results returned
//   - Results are ALWAYS ordered by submodel ID for consistent pagination
//   - Uses "peek ahead" pattern (limit + 1) to determine if there are more pages
func addPaginationToQuery(query *goqu.SelectDataset, limit int64, cursor string) *goqu.SelectDataset {
	// Add ordering by submodel ID for consistent pagination (ALWAYS when this function is called)
	query = query.Order(goqu.I("s.id").Asc())

	// Add cursor filtering if provided (start after the cursor)
	if cursor != "" {
		query = query.Where(goqu.I("s.id").Gt(cursor))
	}

	// Add limit if provided (use peek ahead pattern)
	if limit > 0 {
		// Add 1 to limit for peek ahead to determine if there are more results
		peekLimit := limit + 1
		query = query.Limit(uint(peekLimit))
	}

	return query
}
