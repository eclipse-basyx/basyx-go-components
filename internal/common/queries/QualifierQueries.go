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
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )

package queries

import (
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

// GetQualifierSubquery constructs a complex SQL subquery that retrieves all qualifiers
// associated with an AAS element (such as a submodel or submodel element), including their
// complete reference hierarchies.
//
// This function builds a comprehensive query that aggregates qualifier data into JSONB format,
// including:
//   - Basic qualifier attributes (kind, type, value_type, value, position)
//   - Semantic ID references and their nested referred references
//   - Value ID references and their nested referred references
//   - Supplemental semantic ID references and their nested referred references
//
// The function constructs multiple nested subqueries to handle the complex reference hierarchies
// present in AAS qualifiers. Each reference type (semantic ID, value ID, supplemental semantic ID)
// is queried separately with both direct references and their transitively referred references.
//
// Parameters:
//   - dialect: A goqu.DialectWrapper that provides database-specific SQL generation capabilities
//   - joinTable: The identifier expression for the join table that links entities to qualifiers
//     (e.g., "submodel_qualifier", "submodel_element_qualifier")
//   - entityIDColumn: The column name in the join table that references the parent entity
//     (e.g., "submodel_id", "submodel_element_id")
//   - qualifierIDColumn: The column name in the join table that references the qualifier
//     (typically "qualifier_id")
//   - entityIDCondition: The identifier expression for the entity ID to match against
//     (e.g., goqu.I("s.id") for matching submodels)
//
// Returns:
//   - *goqu.SelectDataset: A select query that returns a JSONB array of qualifier objects.
//     Each qualifier object contains all its attributes and associated reference hierarchies.
//
// Query Structure:
// The returned query aggregates data from the following tables:
//   - Join table (aliased as "jt"): Links entities to qualifiers
//   - qualifier (aliased as "q"): Contains qualifier metadata and values
//   - reference (aliased as "r" or "ref"): Contains semantic references
//   - reference_key (aliased as "rk"): Contains the keys within each reference
//   - qualifier_supplemental_semantic_id (aliased as "qssi"): Links qualifiers to supplemental semantic IDs
//
// The value field in the returned JSONB uses COALESCE to select the appropriate value column
// based on the qualifier's value_type (text, numeric, boolean, time, or datetime).
//
// Example usage:
//
//	qualifierSubquery := GetQualifierSubquery(
//	    dialect,
//	    goqu.T("submodel_qualifier"),
//	    "submodel_id",
//	    "qualifier_id",
//	    goqu.I("s.id"),
//	)
func GetQualifierSubquery(dialect goqu.DialectWrapper, joinTable exp.IdentifierExpression, entityIDColumn string, qualifierIDColumn string, entityIDCondition exp.IdentifierExpression) *goqu.SelectDataset {
	// Build the jsonb object for semantic ID references
	semanticIDObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	qualifierSemanticIDSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", semanticIDObj))).
		Join(
			goqu.T("qualifier").As("q"),
			goqu.On(goqu.I("q.semantic_id").Eq(goqu.I("r.id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("q.id").Eq(goqu.I("jt." + qualifierIDColumn)))

	// Build the jsonb object for semantic ID referred references
	semanticIDReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("parentReference"), goqu.I("r.parentreference"),
		goqu.V("rootReference"), goqu.I("r.rootreference"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	qualifierSemanticIDReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", semanticIDReferredObj))).
		Join(
			goqu.T("reference").As("r"),
			goqu.On(goqu.I("ref.id").Eq(goqu.I("r.rootreference"))),
		).
		Join(
			goqu.T("qualifier").As("q"),
			goqu.On(goqu.I("q.semantic_id").Eq(goqu.I("ref.id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("r.id"))),
		).
		Where(
			goqu.I("jt."+qualifierIDColumn).Eq(goqu.I("q.id")),
			goqu.I("r.id").IsNotNull(),
		)

	// Build the jsonb object for value ID references
	valueIDObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	qualifierValueIDSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", valueIDObj))).
		Join(
			goqu.T("qualifier").As("q"),
			goqu.On(goqu.I("q.value_id").Eq(goqu.I("r.id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("q.id").Eq(goqu.I("jt." + qualifierIDColumn)))

	// Build the jsonb object for value ID referred references
	valueIDReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("parentReference"), goqu.I("r.parentreference"),
		goqu.V("rootReference"), goqu.I("r.rootreference"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	qualifierValueIDReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", valueIDReferredObj))).
		Join(
			goqu.T("reference").As("r"),
			goqu.On(goqu.I("ref.id").Eq(goqu.I("r.rootreference"))),
		).
		Join(
			goqu.T("qualifier").As("q"),
			goqu.On(goqu.I("q.value_id").Eq(goqu.I("ref.id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("r.id"))),
		).
		Where(
			goqu.I("jt."+qualifierIDColumn).Eq(goqu.I("q.id")),
			goqu.I("r.id").IsNotNull(),
		)

	// Build the jsonb object for supplemental semantic ID references
	supplementalSemanticIDObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	qualifierSupplementalSemanticIDSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", supplementalSemanticIDObj))).
		Join(
			goqu.T("qualifier_supplemental_semantic_id").As("qssi"),
			goqu.On(goqu.I("qssi.qualifier_id").Eq(goqu.I("jt."+qualifierIDColumn))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("qssi.reference_id").Eq(goqu.I("r.id")))

	// Build the jsonb object for supplemental semantic ID referred references
	supplementalSemanticIDReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("ref.id"),
		goqu.V("reference_type"), goqu.I("ref.type"),
		goqu.V("parentReference"), goqu.I("ref.parentreference"),
		goqu.V("rootReference"), goqu.I("ref.rootreference"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	qualifierSupplementalSemanticIDReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", supplementalSemanticIDReferredObj))).
		Join(
			goqu.T("qualifier_supplemental_semantic_id").As("qssi"),
			goqu.On(goqu.I("qssi.qualifier_id").Eq(goqu.I("jt."+qualifierIDColumn))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("ref.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(
			goqu.I("ref.rootreference").Eq(goqu.I("qssi.reference_id")),
			goqu.I("ref.id").IsNotNull(),
		)

	// Build the main qualifier jsonb object
	qualifierObj := goqu.Func("jsonb_build_object",
		goqu.V("dbId"), goqu.I("jt."+qualifierIDColumn),
		goqu.V("kind"), goqu.I("q.kind"),
		goqu.V("type"), goqu.I("q.type"),
		goqu.V("position"), goqu.I("q.position"),
		goqu.V("value_type"), goqu.I("q.value_type"),
		goqu.V("value"), goqu.COALESCE(
			goqu.I("q.value_text"),
			goqu.L("?::text", goqu.I("q.value_num")),
			goqu.L("?::text", goqu.I("q.value_bool")),
			goqu.L("?::text", goqu.I("q.value_time")),
			goqu.L("?::text", goqu.I("q.value_datetime")),
		),
		goqu.V("semanticIdReferenceRows"), qualifierSemanticIDSubquery,
		goqu.V("semanticIdReferredReferencesRows"), qualifierSemanticIDReferredSubquery,
		goqu.V("valueIdReferenceRows"), qualifierValueIDSubquery,
		goqu.V("valueIdReferredReferencesRows"), qualifierValueIDReferredSubquery,
		goqu.V("supplementalSemanticIdReferenceRows"), qualifierSupplementalSemanticIDSubquery,
		goqu.V("supplementalSemanticIdReferredReferenceRows"), qualifierSupplementalSemanticIDReferredSubquery,
	)

	qualifierSubquery := dialect.From(joinTable.As("jt")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", qualifierObj))).
		Join(
			goqu.T("qualifier").As("q"),
			goqu.On(goqu.I("jt."+qualifierIDColumn).Eq(goqu.I("q.id"))),
		).
		Where(goqu.I("jt." + entityIDColumn).Eq(entityIDCondition))
	return qualifierSubquery
}
