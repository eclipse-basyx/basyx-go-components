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
package submodelQueries

import "github.com/doug-martin/goqu/v9"

func GetQualifierSubqueryForSubmodel(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	// Build the jsonb object for semantic ID references
	semanticIdObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	qualifierSemanticIdSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", semanticIdObj))).
		Join(
			goqu.T("qualifier").As("q"),
			goqu.On(goqu.I("q.semantic_id").Eq(goqu.I("r.id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("q.id").Eq(goqu.I("smq.qualifier_id")))

	// Build the jsonb object for semantic ID referred references
	semanticIdReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("parentReference"), goqu.I("r.parentreference"),
		goqu.V("rootReference"), goqu.I("r.rootreference"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	qualifierSemanticIdReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", semanticIdReferredObj))).
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

	// Build the jsonb object for value ID references
	valueIdObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	qualifierValueIdSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", valueIdObj))).
		Join(
			goqu.T("qualifier").As("q"),
			goqu.On(goqu.I("q.value_id").Eq(goqu.I("r.id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("q.id").Eq(goqu.I("smq.qualifier_id")))

	// Build the jsonb object for value ID referred references
	valueIdReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("parentReference"), goqu.I("r.parentreference"),
		goqu.V("rootReference"), goqu.I("r.rootreference"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	qualifierValueIdReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", valueIdReferredObj))).
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

	// Build the jsonb object for supplemental semantic ID references
	supplementalSemanticIdObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	qualifierSupplementalSemanticIdSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", supplementalSemanticIdObj))).
		Join(
			goqu.T("qualifier_supplemental_semantic_id").As("qssi"),
			goqu.On(goqu.I("qssi.qualifier_id").Eq(goqu.I("smq.qualifier_id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("qssi.reference_id").Eq(goqu.I("r.id")))

	// Build the jsonb object for supplemental semantic ID referred references
	supplementalSemanticIdReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("ref.id"),
		goqu.V("reference_type"), goqu.I("ref.type"),
		goqu.V("parentReference"), goqu.I("ref.parentreference"),
		goqu.V("rootReference"), goqu.I("ref.rootreference"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	qualifierSupplementalSemanticIdReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", supplementalSemanticIdReferredObj))).
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

	// Build the main qualifier jsonb object
	qualifierObj := goqu.Func("jsonb_build_object",
		goqu.V("dbId"), goqu.I("smq.qualifier_id"),
		goqu.V("kind"), goqu.I("q.kind"),
		goqu.V("type"), goqu.I("q.type"),
		goqu.V("value_type"), goqu.I("q.value_type"),
		goqu.V("value"), goqu.COALESCE(
			goqu.I("q.value_text"),
			goqu.L("?::text", goqu.I("q.value_num")),
			goqu.L("?::text", goqu.I("q.value_bool")),
			goqu.L("?::text", goqu.I("q.value_time")),
			goqu.L("?::text", goqu.I("q.value_datetime")),
		),
		goqu.V("semanticIdReferenceRows"), qualifierSemanticIdSubquery,
		goqu.V("semanticIdReferredReferencesRows"), qualifierSemanticIdReferredSubquery,
		goqu.V("valueIdReferenceRows"), qualifierValueIdSubquery,
		goqu.V("valueIdReferredReferencesRows"), qualifierValueIdReferredSubquery,
		goqu.V("supplementalSemanticIdReferenceRows"), qualifierSupplementalSemanticIdSubquery,
		goqu.V("supplementalSemanticIdReferredReferenceRows"), qualifierSupplementalSemanticIdReferredSubquery,
	)

	qualifierSubquery := dialect.From(goqu.T("submodel_qualifier").As("smq")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", qualifierObj))).
		Join(
			goqu.T("qualifier").As("q"),
			goqu.On(goqu.I("smq.qualifier_id").Eq(goqu.I("q.id"))),
		).
		Where(goqu.I("smq.submodel_id").Eq(goqu.I("s.id")))
	return qualifierSubquery
}
