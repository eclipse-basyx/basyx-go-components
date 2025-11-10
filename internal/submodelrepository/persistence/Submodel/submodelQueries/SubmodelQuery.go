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

// Package submodelsubqueries provides functions to construct SQL queries for retrieving submodel data.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package submodelsubqueries

import (
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/queries"
)

// GetQueryWithGoqu constructs a comprehensive SQL query for retrieving submodel data.
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
//   - submodelID: Optional filter for a specific submodel ID. Empty string retrieves all submodels.
//
// Returns:
//   - string: The complete SQL query string ready for execution
//   - error: An error if query generation fails
//
// The function uses COALESCE to ensure empty arrays ('[]'::jsonb) instead of NULL values,
// which simplifies downstream JSON parsing. It also includes a total count window function
// for efficient result set pagination and slice pre-sizing.
func GetQueryWithGoqu(submodelID string, limit int64, cursor string, aasQuery *grammar.QueryWrapper, onlyIds bool) (string, error) {
	dialect := goqu.Dialect("postgres")

	// Build display names subquery
	displayNamesSubquery := queries.GetDisplayNamesQuery(dialect, "s.displayname_id")

	// Build descriptions subquery
	descriptionsSubquery := queries.GetDescriptionQuery(dialect, "s.description_id")

	semanticIDSubquery, semanticIDReferredSubquery := queries.GetReferenceQueries(dialect, goqu.I("s.semantic_id"))

	// Build qualifier subquery
	qualifierSubquery := queries.GetQualifierSubquery(dialect, goqu.T("submodel_qualifier"), "submodel_id", "qualifier_id", goqu.I("s.id"))

	// Build AdministrativeInformation subquery
	administrationSubquery := queries.GetAdministrationSubquery(dialect, "s.administration_id")

	// Main query
	query := dialect.From(goqu.T("submodel").As("s")).
		Select(
			goqu.I("s.id").As("submodel_id"),
			goqu.I("s.id_short").As("submodel_id_short"),
			goqu.I("s.category").As("submodel_category"),
			goqu.I("s.kind").As("submodel_kind"),
			goqu.I("s.embedded_data_specification").As("embedded_data_specification"),
			goqu.I("s.supplemental_semantic_ids").As("supplemental_semantic_ids"),
			goqu.I("s.extensions").As("extensions"),
			goqu.L("COALESCE((?), '[]'::jsonb)", displayNamesSubquery).As("submodel_display_names"),
			goqu.L("COALESCE((?), '[]'::jsonb)", descriptionsSubquery).As("submodel_descriptions"),
			goqu.L("COALESCE((?), '[]'::jsonb)", semanticIDSubquery).As("submodel_semantic_id"),
			goqu.L("COALESCE((?), '[]'::jsonb)", semanticIDReferredSubquery).As("submodel_semantic_id_referred"),
			goqu.L("COALESCE((?), '[]'::jsonb)", qualifierSubquery).As("submodel_qualifiers"),
			goqu.L("COALESCE((?), '[]'::jsonb)", administrationSubquery).As("submodel_administrative_information"),
		)

	if onlyIds {
		// If only IDs are requested, adjust the selection to only include the submodel ID
		query = dialect.From(goqu.T("submodel").As("s")).
			Select(
				goqu.I("s.id").As("submodel_id"),
			)
	}

	// Add optional WHERE clause for submodel ID filtering
	if submodelID != "" {
		query = addSubmodelIDFilterToQuery(query, submodelID)
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

	// query = addSubmodelCountToQuery(query)
	query = addGroupBySubmodelID(query)

	// Add pagination if limit or cursor is specified
	shouldPeekAhead := true
	if onlyIds {
		shouldPeekAhead = false
	}
	query = addPaginationToQuery(query, limit, cursor, shouldPeekAhead)

	sql, _, err := query.ToSQL()
	if err != nil {
		return "", fmt.Errorf("error generating SQL: %w", err)
	}

	return sql, nil
}

func addGroupBySubmodelID(query *goqu.SelectDataset) *goqu.SelectDataset {
	query = query.GroupBy(goqu.I("s.id"))
	return query
}

func addSubmodelCountToQuery(query *goqu.SelectDataset) *goqu.SelectDataset {
	query = query.SelectAppend(goqu.L("COUNT(s.id) OVER() AS total_submodels"))
	return query
}

func addSubmodelIDFilterToQuery(query *goqu.SelectDataset, submodelID string) *goqu.SelectDataset {
	query = query.Where(goqu.I("s.id").Eq(submodelID))
	return query
}

func addJoinsToQueryForAASQL(query *goqu.SelectDataset) *goqu.SelectDataset {
	query = query.Join(goqu.T("reference").As("semantic_id_reference"), goqu.On(goqu.I("s.semantic_id").Eq(goqu.I("semantic_id_reference.id"))))
	query = query.Join(goqu.T("reference_key").As("semantic_id_reference_key"), goqu.On(goqu.I("semantic_id_reference.id").Eq(goqu.I("semantic_id_reference_key.reference_id"))))
	return query
}

func applyAASQuery(aasQuery *grammar.QueryWrapper, query *goqu.SelectDataset) (*goqu.SelectDataset, error) {
	if aasQuery == nil || aasQuery.Query.Condition == nil {
		return query, nil
	}

	// Evaluate the logical expression to a SQL expression
	expr, err := aasQuery.Query.Condition.EvaluateToExpression()
	if err != nil {
		return nil, fmt.Errorf("error evaluating query condition: %w", err)
	}

	// Apply the expression as a WHERE clause
	query = query.Where(expr)
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
func addPaginationToQuery(query *goqu.SelectDataset, limit int64, cursor string, peekAhead bool) *goqu.SelectDataset {
	// Add ordering by submodel ID for consistent pagination (ALWAYS when this function is called)
	query = query.Order(goqu.I("s.id").Asc())

	// Add cursor filtering if provided (start after the cursor)
	if cursor != "" {
		query = query.Where(goqu.I("s.id").Gt(cursor))
	}

	// Add limit if provided (use peek ahead pattern)
	if limit > 0 {
		// Add 1 to limit for peek ahead to determine if there are more results
		if peekAhead {
			limit += 1
		}
		query = query.Limit(uint(limit))
	}

	return query
}
