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

// GetReferenceQueries builds two subqueries for fetching reference data and referred references.
//
// The first subquery returns a jsonb aggregation of the direct reference (by referenceIdCondition)
// along with its keys from the reference_key table.
//
// The second subquery returns a jsonb aggregation of all referred references (those that share
// the same rootreference but are not the root itself), including their parent/root metadata and keys.
//
// Parameters:
//   - dialect: The goqu DialectWrapper for building SQL queries.
//   - referenceIdCondition: The condition to match the reference ID (e.g., goqu.I("tlsme.semantic_id")).
//
// Returns:
//   - First dataset: Direct reference subquery (reference + keys).
//   - Second dataset: Referred references subquery (nested references + keys).
func GetReferenceQueries(dialect goqu.DialectWrapper, referenceIDCondition any) (*goqu.SelectDataset, *goqu.SelectDataset) {
	refSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.L("jsonb_agg(jsonb_build_object('reference_id', r.id, 'reference_type', r.type, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value) ORDER BY rk.position)")).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("r.id"))),
		).
		Where(goqu.I("r.id").Eq(referenceIDCondition))

	refReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.L("jsonb_agg(jsonb_build_object('reference_id', ref.id, 'reference_type', ref.type, 'parentReference', ref.parentreference, 'rootReference', ref.rootreference, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value) ORDER BY rk.position)")).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("ref.id"))),
		).
		Where(
			goqu.I("ref.rootreference").Eq(referenceIDCondition),
			goqu.I("ref.id").Neq(referenceIDCondition),
		)
	return refSubquery, refReferredSubquery
}

// GetSupplementalSemanticIDQueries builds two subqueries for fetching supplemental semantic IDs
// and their referred references from a join table.
//
// The first subquery returns a jsonb aggregation of all direct supplemental semantic references
// linked via the specified join table, along with their keys.
//
// The second subquery returns a jsonb aggregation of all referred references (nested references
// that share the same root as a supplemental semantic ID), including metadata about the
// supplemental root reference and keys.
//
// Parameters:
//   - dialect: The goqu DialectWrapper for building SQL queries.
//   - joinTable: The join table (e.g., goqu.T("submodel_element_supplemental_semantic_id")) that links entities to references.
//   - entityIdColumn: The column name in the join table that references the entity (e.g., "sme_id").
//   - referenceIdColumn: The column name in the join table that references the reference ID (e.g., "reference_id").
//   - entityIdCondition: The condition to match the entity ID (e.g., goqu.I("tlsme.id")).
//
// Returns:
//   - First dataset: Direct supplemental semantic IDs subquery (references + keys).
//   - Second dataset: Referred supplemental semantic IDs subquery (nested references + keys + root ID).
func GetSupplementalSemanticIDQueries(dialect goqu.DialectWrapper, joinTable exp.IdentifierExpression, entityIDColumn string, referenceIDColumn string, entityIDCondition exp.IdentifierExpression) (*goqu.SelectDataset, *goqu.SelectDataset) {
	supplementalSemanticIDsSubquery := dialect.From(joinTable.As("jt")).
		Select(goqu.L("jsonb_agg(jsonb_build_object('reference_id', ref.id, 'reference_type', ref.type, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value) ORDER BY rk.position)")).
		LeftJoin(
			goqu.T("reference").As("ref"),
			goqu.On(goqu.I("ref.id").Eq(goqu.I("jt."+referenceIDColumn))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("ref.id"))),
		).
		Where(goqu.I("jt." + entityIDColumn).Eq(entityIDCondition))

	// Build supplemental semantic ids referred subquery
	supplementalSemanticIDsReferredSubquery := dialect.From(joinTable.As("jt")).
		Select(goqu.L("jsonb_agg(jsonb_build_object('supplemental_root_reference_id', jt."+referenceIDColumn+", 'reference_id', ref.id, 'reference_type', ref.type, 'parentReference', ref.parentreference, 'rootReference', ref.rootreference, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value) ORDER BY rk.position)")).
		LeftJoin(
			goqu.T("reference").As("ref"),
			goqu.On(goqu.I("ref.rootreference").Eq(goqu.I("jt."+referenceIDColumn))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("ref.id"))),
		).
		Where(
			goqu.I("jt."+entityIDColumn).Eq(entityIDCondition),
			goqu.I("ref.id").IsNotNull(),
		)
	return supplementalSemanticIDsSubquery, supplementalSemanticIDsReferredSubquery
}
