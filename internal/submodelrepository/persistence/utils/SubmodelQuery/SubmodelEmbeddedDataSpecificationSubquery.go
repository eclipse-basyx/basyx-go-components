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

func GetEmbeddedDataSpecificationSubqueries(dialect goqu.DialectWrapper) (*goqu.SelectDataset, *goqu.SelectDataset, *goqu.SelectDataset) {
	embeddedDataSpecificationReferenceSubquery := dialect.From(goqu.T("submodel_embedded_data_specification").As("seds")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('eds_id', seds.embedded_data_specification_id, 'reference_id', data_spec_reference.id, 'reference_type', data_spec_reference.type, 'key_id', data_spec_reference_key.id, 'key_type', data_spec_reference_key.type, 'key_value', data_spec_reference_key.value))")).
		LeftJoin(
			goqu.T("data_specification").As("data_spec"),
			goqu.On(goqu.I("data_spec.id").Eq(goqu.I("seds.embedded_data_specification_id"))),
		).
		LeftJoin(
			goqu.T("reference").As("data_spec_reference"),
			goqu.On(goqu.I("data_spec.data_specification").Eq(goqu.I("data_spec_reference.id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("data_spec_reference_key"),
			goqu.On(goqu.I("data_spec_reference.id").Eq(goqu.I("data_spec_reference_key.reference_id"))),
		).
		Where(goqu.I("seds.submodel_id").Eq(goqu.I("s.id")))

	// Build semantic_id referred references subquery
	embeddedDataSpecificationReferenceReferredSubquery := dialect.From(goqu.T("submodel_embedded_data_specification").As("seds")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('reference_id', ref.id, 'reference_type', ref.type, 'parentReference', ref.parentreference, 'rootReference', ref.rootreference, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value))")).
		LeftJoin(
			goqu.T("data_specification").As("data_spec"),
			goqu.On(goqu.I("data_spec.id").Eq(goqu.I("seds.embedded_data_specification_id"))),
		).
		LeftJoin(
			goqu.T("reference").As("data_spec_reference"),
			goqu.On(goqu.I("data_spec.data_specification").Eq(goqu.I("data_spec_reference.id"))),
		).
		LeftJoin(
			goqu.T("reference").As("ref"),
			goqu.On(goqu.I("ref.rootreference").Eq(goqu.I("data_spec_reference.id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("ref.id"))),
		).
		LeftJoin(
			goqu.T("reference").As("dsr"),
			goqu.On(goqu.I("dsr.id").Eq(goqu.I("data_spec_reference.id"))),
		).
		Where(
			goqu.I("seds.submodel_id").Eq(goqu.I("s.id")),
			goqu.I("ref.id").IsNotNull(),
		)

	preferredNameSubquery := dialect.From(goqu.T("lang_string_text_type_reference").As("pn_ref")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('language', pn.language, 'text', pn.text, 'id', pn.id))")).
		Join(
			goqu.T("lang_string_text_type").As("pn"),
			goqu.On(goqu.I("pn.lang_string_text_type_reference_id").Eq(goqu.I("pn_ref.id"))),
		).
		Where(goqu.I("pn_ref.id").Eq(goqu.I("iec.preferred_name_id")))

	shortNameSubquery := dialect.From(goqu.T("lang_string_text_type_reference").As("sn_ref")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('language', sn.language, 'text', sn.text, 'id', sn.id))")).
		Join(
			goqu.T("lang_string_text_type").As("sn"),
			goqu.On(goqu.I("sn.lang_string_text_type_reference_id").Eq(goqu.I("sn_ref.id"))),
		).
		Where(goqu.I("sn_ref.id").Eq(goqu.I("iec.short_name_id")))

	definitionSubquery := dialect.From(goqu.T("lang_string_text_type_reference").As("df_ref")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('language', df.language, 'text', df.text, 'id', df.id))")).
		Join(
			goqu.T("lang_string_text_type").As("df"),
			goqu.On(goqu.I("df.lang_string_text_type_reference_id").Eq(goqu.I("df_ref.id"))),
		).
		Where(goqu.I("df_ref.id").Eq(goqu.I("iec.definition_id")))

	unitReferenceKeysSubquery := dialect.From(goqu.T("reference").As("dsu")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('reference_id', dsu.id,'reference_type', dsu.type, 'key_id', dsk.id, 'key_type', dsk.type, 'key_value', dsk.value))")).
		LeftJoin(
			goqu.T("reference_key").As("dsk"),
			goqu.On(goqu.I("dsk.reference_id").Eq(goqu.I("dsu.id"))),
		).
		Where(goqu.I("dsu.id").Eq(goqu.I("iec.unit_id")))

	unitReferenceReferredSubquery := dialect.From(goqu.T("reference").As("dsu2")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('rootReference', dsu2.rootreference,'parentReference', dsu2.parentreference,'reference_type', dsu2.type, 'reference_id', dsu2.id, 'key_id', dsk2.id, 'key_type', dsk2.type, 'key_value', dsk2.value))")).
		LeftJoin(
			goqu.T("reference_key").As("dsk2"),
			goqu.On(goqu.I("dsk2.reference_id").Eq(goqu.I("dsu2.id"))),
		).
		Where(
			goqu.I("dsu2.rootreference").Eq(goqu.I("iec.unit_id")),
			goqu.I("dsu2.id").Neq(goqu.I("iec.unit_id")),
		)

	vlvrpReferenceSubquery := dialect.From(goqu.T("reference").As("vlvrp_ref")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('reference_id', vlvrp_ref.id,'reference_type', vlvrp_ref.type, 'key_id', vlvrp_ref_rk.id, 'key_type', vlvrp_ref_rk.type, 'key_value', vlvrp_ref_rk.value))")).
		LeftJoin(
			goqu.T("reference_key").As("vlvrp_ref_rk"),
			goqu.On(goqu.I("vlvrp_ref_rk.reference_id").Eq(goqu.I("vlvrp_ref.id"))),
		).
		Where(goqu.I("vlvrp_ref.id").Eq(goqu.I("vlvrp.value_id")))

	vlvrpReferenceReferredSubquery := dialect.From(goqu.T("reference").As("vlvrp_referred_ref")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('rootReference', vlvrp_referred_ref.rootreference,'parentReference', vlvrp_referred_ref.parentreference,'reference_type', vlvrp_referred_ref.type, 'reference_id', vlvrp_referred_ref.id, 'key_id', vlvrp_ref_rk.id, 'key_type', vlvrp_ref_rk.type, 'key_value', vlvrp_ref_rk.value))")).
		LeftJoin(
			goqu.T("reference_key").As("vlvrp_ref_rk"),
			goqu.On(goqu.I("vlvrp_ref_rk.reference_id").Eq(goqu.I("vlvrp_referred_ref.id"))),
		).
		Where(
			goqu.I("vlvrp_referred_ref.rootreference").Eq(goqu.I("vlvrp.value_id")),
			goqu.I("vlvrp_referred_ref.id").Neq(goqu.I("vlvrp.value_id")),
		)

	valueListEntriesSubquery := dialect.From(goqu.T("value_list").As("vl")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('value_reference_pair_id', vlvrp.id,'value_pair_value', vlvrp.value, 'reference_rows', ?, 'referred_reference_rows', ?))", vlvrpReferenceSubquery, vlvrpReferenceReferredSubquery)).
		Join(
			goqu.T("value_list_value_reference_pair").As("vlvrp"),
			goqu.On(goqu.I("vl.id").Eq(goqu.I("vlvrp.value_list_id"))),
		).
		LeftJoin(
			goqu.T("reference").As("vlref"),
			goqu.On(goqu.I("vlvrp.value_id").Eq(goqu.I("vlref.id"))),
		).
		Where(goqu.I("vl.id").Eq(goqu.I("iec.value_list_id")))

	levelTypeSubquery := dialect.From(goqu.T("level_type").As("leveltype")).
		Select(goqu.L("jsonb_build_object('min',leveltype.min, 'max', leveltype.max, 'nom', leveltype.nom,'typ', leveltype.typ)")).
		Where(goqu.I("leveltype.id").Eq(goqu.I("iec.level_type_id")))

	iec61360Subquery := dialect.From(goqu.T("submodel_embedded_data_specification").As("seds")).
		Select(
			goqu.L(
				`jsonb_agg(
					DISTINCT jsonb_build_object(
						'eds_id', seds.embedded_data_specification_id,
						'iec_id', iec.id,
						'unit', iec.unit,
						'source_of_definition', iec.source_of_definition, 
						'symbol', iec.symbol, 
						'data_type', iec.data_type, 
						'value_format', iec.value_format, 
						'value', iec.value, 
						'level_type_id', iec.level_type_id, 
						'preferred_name', ?, 
						'short_name', ?, 
						'definition', ?, 
						'unit_reference_keys', ?, 
						'unit_reference_referred', ?, 
						'value_list_entries', ?,
						'level_type', ?
					)
				)`,
				preferredNameSubquery, shortNameSubquery, definitionSubquery, unitReferenceKeysSubquery, unitReferenceReferredSubquery, valueListEntriesSubquery, levelTypeSubquery)).
		Join(
			goqu.T("data_specification").As("ds"),
			goqu.On(goqu.I("ds.id").Eq(goqu.I("seds.embedded_data_specification_id"))),
		).
		Join(
			goqu.T("data_specification_content").As("dsc"),
			goqu.On(goqu.I("dsc.id").Eq(goqu.I("ds.data_specification_content"))),
		).
		Join(
			goqu.T("data_specification_iec61360").As("iec"),
			goqu.On(goqu.I("iec.id").Eq(goqu.I("dsc.id"))),
		).
		Where(goqu.I("seds.submodel_id").Eq(goqu.I("s.id")))
	return embeddedDataSpecificationReferenceSubquery, embeddedDataSpecificationReferenceReferredSubquery, iec61360Subquery
}
