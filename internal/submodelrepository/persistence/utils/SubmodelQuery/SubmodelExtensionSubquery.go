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

func GetExtensionSubqueryForSubmodel(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	extensionSemanticIdSubquery := dialect.From(goqu.T("reference").As("r")).
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
			goqu.T("extension").As("e"),
			goqu.On(goqu.I("e.semantic_id").Eq(goqu.I("r.id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("e.id").Eq(goqu.I("sm_ext.extension_id")))

	extensionSemanticIdReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
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
			goqu.T("extension").As("e"),
			goqu.On(goqu.I("e.semantic_id").Eq(goqu.I("ref.id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("r.id"))),
		).
		Where(
			goqu.I("sm_ext.extension_id").Eq(goqu.I("e.id")),
			goqu.I("r.id").IsNotNull(),
		)

	extensionRefersToSubquery := dialect.From(goqu.T("reference").As("r")).
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
			goqu.T("extension_refers_to").As("ert"),
			goqu.On(goqu.I("ert.extension_id").Eq(goqu.I("sm_ext.extension_id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("ert.reference_id").Eq(goqu.I("r.id")))

	extensionRefersToReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
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
			goqu.T("extension_refers_to").As("ert"),
			goqu.On(goqu.I("ert.extension_id").Eq(goqu.I("sm_ext.extension_id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("ref.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(
			goqu.I("ref.rootreference").Eq(goqu.I("ert.reference_id")),
			goqu.I("ref.id").IsNotNull(),
		)

	extensionSupplementalSemanticIdSubquery := dialect.From(goqu.T("reference").As("r")).
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
			goqu.T("extension_refers_to").As("ert"),
			goqu.On(goqu.I("ert.extension_id").Eq(goqu.I("sm_ext.extension_id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("ert.reference_id").Eq(goqu.I("r.id")))

	extensionSupplementalSemanticIdReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
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
			goqu.T("extension_refers_to").As("ert"),
			goqu.On(goqu.I("ert.extension_id").Eq(goqu.I("sm_ext.extension_id"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("ref.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(
			goqu.I("ref.rootreference").Eq(goqu.I("ert.reference_id")),
			goqu.I("ref.id").IsNotNull(),
		)

	extensionSubquery := dialect.From(goqu.T("submodel_extension").As("sm_ext")).
		Select(goqu.L(`
			jsonb_agg(
					DISTINCT jsonb_build_object(
					'dbId', sm_ext.extension_id,
					'name', e.name,
					'value_type', e.value_type,
					'value', COALESCE(e.value_text, e.value_num::text, e.value_bool::text, e.value_time::text, e.value_datetime::text),
					'semanticIdReferenceRows', ?,
					'semanticIdReferredReferencesRows', ?,
					'refersToReferenceRows', ?,
					'refersToReferredReferencesRows', ?,
					'supplementalSemanticIdReferenceRows', ?,
					'supplementalSemanticIdReferredReferenceRows', ?
					)
			)
		`, extensionSemanticIdSubquery, extensionSemanticIdReferredSubquery, extensionRefersToSubquery, extensionRefersToReferredSubquery, extensionSupplementalSemanticIdSubquery, extensionSupplementalSemanticIdReferredSubquery)).
		Join(
			goqu.T("extension").As("e"),
			goqu.On(goqu.I("sm_ext.extension_id").Eq(goqu.I("e.id"))),
		).
		Where(goqu.I("sm_ext.submodel_id").Eq(goqu.I("s.id")))
	return extensionSubquery
}
