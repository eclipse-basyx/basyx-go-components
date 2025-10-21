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

import "github.com/doug-martin/goqu/v9"

func GetQualifierSubqueryForSubmodel(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	qualifierSemanticIdSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.L(`
		jsonb_agg(
			DISTINCT jsonb_build_object(
				'reference_id', r.id, 
				'reference_type', r.type, 
				'key_id', rk.id, 
				'key_type', rk.type, 
				'key_value', rk.value
			)
		)`)).
		Join(
			goqu.T("qualifier").As("q"),
			goqu.On(goqu.I("q.semantic_id").Eq(goqu.I("r.id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("q.id").Eq(goqu.I("smq.qualifier_id")))

	qualifierSemanticIdReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.L(`jsonb_agg(
			DISTINCT jsonb_build_object(
				'reference_id', r.id, 
				'reference_type', r.type, 
				'parentReference', r.parentreference, 
				'rootReference', r.rootreference, 
				'key_id', rk.id, 
				'key_type', rk.type, 
				'key_value', rk.value
			)
		)`)).
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
			goqu.I("smq.qualifier_id").Eq(goqu.I("q.id")),
			goqu.I("r.id").IsNotNull(),
		)

	qualifierValueIdSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.L(`
		jsonb_agg(
			DISTINCT jsonb_build_object(
				'reference_id', r.id, 
				'reference_type', r.type, 
				'key_id', rk.id, 
				'key_type', rk.type, 
				'key_value', rk.value
			)
		)`)).
		Join(
			goqu.T("qualifier").As("q"),
			goqu.On(goqu.I("q.value_id").Eq(goqu.I("r.id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("q.id").Eq(goqu.I("smq.qualifier_id")))

	qualifierValueIdReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.L(`jsonb_agg(
			DISTINCT jsonb_build_object(
				'reference_id', r.id, 
				'reference_type', r.type, 
				'parentReference', r.parentreference, 
				'rootReference', r.rootreference, 
				'key_id', rk.id, 
				'key_type', rk.type, 
				'key_value', rk.value
			)
		)`)).
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
			goqu.I("smq.qualifier_id").Eq(goqu.I("q.id")),
			goqu.I("r.id").IsNotNull(),
		)

	qualifierSupplementalSemanticIdSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.L(`
		jsonb_agg(
			DISTINCT jsonb_build_object(
				'reference_id', r.id, 
				'reference_type', r.type, 
				'key_id', rk.id, 
				'key_type', rk.type, 
				'key_value', rk.value
			)
		)`)).
		Join(
			goqu.T("qualifier_supplemental_semantic_id").As("qssi"),
			goqu.On(goqu.I("qssi.qualifier_id").Eq(goqu.I("smq.qualifier_id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("qssi.reference_id").Eq(goqu.I("r.id")))

	qualifierSupplementalSemanticIdReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.L(`jsonb_agg(
			DISTINCT jsonb_build_object(
				'reference_id', ref.id, 
				'reference_type', ref.type, 
				'parentReference', ref.parentreference, 
				'rootReference', ref.rootreference, 
				'key_id', rk.id, 
				'key_type', rk.type, 
				'key_value', rk.value
			)
		)`)).
		Join(
			goqu.T("qualifier_supplemental_semantic_id").As("qssi"),
			goqu.On(goqu.I("qssi.qualifier_id").Eq(goqu.I("smq.qualifier_id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("ref.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(
			goqu.I("ref.rootreference").Eq(goqu.I("qssi.reference_id")),
			goqu.I("ref.id").IsNotNull(),
		)

	qualifierSubquery := dialect.From(goqu.T("submodel_qualifier").As("smq")).
		Select(goqu.L(`
			jsonb_agg(
					DISTINCT jsonb_build_object(
					'dbId', smq.qualifier_id,
					'kind', q.kind,
					'type', q.type,
					'value_type', q.value_type,
					'value', COALESCE(q.value_text, q.value_num::text, q.value_bool::text, q.value_time::text, q.value_datetime::text),
					'semanticIdReferenceRows', ?,
					'semanticIdReferredReferencesRows', ?,
					'valueIdReferenceRows', ?,
					'valueIdReferredReferencesRows', ?,
					'supplementalSemanticIdReferenceRows', ?,
					'supplementalSemanticIdReferredReferenceRows', ?
					)
			)
		`, qualifierSemanticIdSubquery, qualifierSemanticIdReferredSubquery, qualifierValueIdSubquery, qualifierValueIdReferredSubquery, qualifierSupplementalSemanticIdSubquery, qualifierSupplementalSemanticIdReferredSubquery)).
		Join(
			goqu.T("qualifier").As("q"),
			goqu.On(goqu.I("smq.qualifier_id").Eq(goqu.I("q.id"))),
		).
		Where(goqu.I("smq.submodel_id").Eq(goqu.I("s.id")))
	return qualifierSubquery
}
