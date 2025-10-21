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
func GetQueryWithGoqu(submodelId string) (string, error) {
	dialect := goqu.Dialect("postgres")

	// Build display names subquery
	displayNamesSubquery := dialect.From(goqu.T("lang_string_name_type_reference").As("dn_ref")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('language', dn.language, 'text', dn.text, 'id', dn.id))")).
		Join(
			goqu.T("lang_string_name_type").As("dn"),
			goqu.On(goqu.I("dn.lang_string_name_type_reference_id").Eq(goqu.I("dn_ref.id"))),
		).
		Where(goqu.I("dn_ref.id").Eq(goqu.I("s.displayname_id")))

	// Build descriptions subquery
	descriptionsSubquery := dialect.From(goqu.T("lang_string_text_type_reference").As("dr")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('language', d.language, 'text', d.text, 'id', d.id))")).
		Join(
			goqu.T("lang_string_text_type").As("d"),
			goqu.On(goqu.I("d.lang_string_text_type_reference_id").Eq(goqu.I("dr.id"))),
		).
		Where(goqu.I("dr.id").Eq(goqu.I("s.description_id")))

	// Build semantic_id subquery
	semanticIdSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('reference_id', r.id, 'reference_type', r.type, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value))")).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("r.id"))),
		).
		Where(goqu.I("r.id").Eq(goqu.I("s.semantic_id")))

	// Build semantic_id referred references subquery
	semanticIdReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('reference_id', ref.id, 'reference_type', ref.type, 'parentReference', ref.parentreference, 'rootReference', ref.rootreference, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value))")).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("ref.id"))),
		).
		Where(
			goqu.I("ref.rootreference").Eq(goqu.I("s.semantic_id")),
			goqu.I("ref.id").Neq(goqu.I("s.semantic_id")),
		)

	// Build supplemental semantic ids subquery
	supplementalSemanticIdsSubquery := dialect.From(goqu.T("submodel_supplemental_semantic_id").As("sssi")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('reference_id', ref.id, 'reference_type', ref.type, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value))")).
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
	supplementalSemanticIdsReferredSubquery := dialect.From(goqu.T("submodel_supplemental_semantic_id").As("sssi")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('supplemental_root_reference_id', sssi.reference_id, 'reference_id', ref.id, 'reference_type', ref.type, 'parentReference', ref.parentreference, 'rootReference', ref.rootreference, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value))")).
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
	embeddedDataSpecificationReferenceSubquery, embeddedDataSpecificationReferenceReferredSubquery, iec61360Subquery := GetEmbeddedDataSpecificationSubqueries(dialect)

	qualifierSubquery := GetQualifierSubqueryForSubmodel(dialect)

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
		)

	// Add optional WHERE clause for submodel ID filtering
	if submodelId != "" {
		query = query.Where(goqu.I("s.id").Eq(submodelId))
	}

	// add a field that counts number of submodels to presize slices in calling function
	query = query.SelectAppend(goqu.L("COUNT(s.id) OVER() AS total_submodels"))

	sql, _, err := query.ToSQL()
	if err != nil {
		return "", fmt.Errorf("error generating SQL: %w", err)
	}

	return sql, nil
}
