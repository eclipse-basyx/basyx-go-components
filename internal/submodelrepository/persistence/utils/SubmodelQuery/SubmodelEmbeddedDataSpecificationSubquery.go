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

func GetEmbeddedDataSpecificationSubqueries(dialect goqu.DialectWrapper, joinTable string, joinTableIdField string, compareField string) (*goqu.SelectDataset, *goqu.SelectDataset, *goqu.SelectDataset) {
	// Build the jsonb object for embedded data specification references
	edsReferenceObj := goqu.Func("jsonb_build_object",
		goqu.V("eds_id"), goqu.I("jt.embedded_data_specification_id"),
		goqu.V("reference_id"), goqu.I("data_spec_reference.id"),
		goqu.V("reference_type"), goqu.I("data_spec_reference.type"),
		goqu.V("key_id"), goqu.I("data_spec_reference_key.id"),
		goqu.V("key_type"), goqu.I("data_spec_reference_key.type"),
		goqu.V("key_value"), goqu.I("data_spec_reference_key.value"),
	)

	embeddedDataSpecificationReferenceSubquery := dialect.From(goqu.T(joinTable).As("jt")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", edsReferenceObj))).
		LeftJoin(
			goqu.T("data_specification").As("data_spec"),
			goqu.On(goqu.I("data_spec.id").Eq(goqu.I("jt.embedded_data_specification_id"))),
		).
		LeftJoin(
			goqu.T("reference").As("data_spec_reference"),
			goqu.On(goqu.I("data_spec.data_specification").Eq(goqu.I("data_spec_reference.id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("data_spec_reference_key"),
			goqu.On(goqu.I("data_spec_reference.id").Eq(goqu.I("data_spec_reference_key.reference_id"))),
		).
		Where(goqu.I("jt." + joinTableIdField).Eq(goqu.I(compareField)))

	// Build semantic_id referred references subquery
	edsReferenceReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("ref.id"),
		goqu.V("reference_type"), goqu.I("ref.type"),
		goqu.V("parentReference"), goqu.I("ref.parentreference"),
		goqu.V("rootReference"), goqu.I("ref.rootreference"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	embeddedDataSpecificationReferenceReferredSubquery := dialect.From(goqu.T(joinTable).As("jt")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", edsReferenceReferredObj))).
		LeftJoin(
			goqu.T("data_specification").As("data_spec"),
			goqu.On(goqu.I("data_spec.id").Eq(goqu.I("jt.embedded_data_specification_id"))),
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
			goqu.I("jt."+joinTableIdField).Eq(goqu.I(compareField)),
			goqu.I("ref.id").IsNotNull(),
		)

	// Build preferred name subquery
	preferredNameObj := goqu.Func("jsonb_build_object",
		goqu.V("language"), goqu.I("pn.language"),
		goqu.V("text"), goqu.I("pn.text"),
		goqu.V("id"), goqu.I("pn.id"),
	)

	preferredNameSubquery := dialect.From(goqu.T("lang_string_text_type_reference").As("pn_ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", preferredNameObj))).
		Join(
			goqu.T("lang_string_text_type").As("pn"),
			goqu.On(goqu.I("pn.lang_string_text_type_reference_id").Eq(goqu.I("pn_ref.id"))),
		).
		Where(goqu.I("pn_ref.id").Eq(goqu.I("iec.preferred_name_id")))

	// Build short name subquery
	shortNameObj := goqu.Func("jsonb_build_object",
		goqu.V("language"), goqu.I("sn.language"),
		goqu.V("text"), goqu.I("sn.text"),
		goqu.V("id"), goqu.I("sn.id"),
	)

	shortNameSubquery := dialect.From(goqu.T("lang_string_text_type_reference").As("sn_ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", shortNameObj))).
		Join(
			goqu.T("lang_string_text_type").As("sn"),
			goqu.On(goqu.I("sn.lang_string_text_type_reference_id").Eq(goqu.I("sn_ref.id"))),
		).
		Where(goqu.I("sn_ref.id").Eq(goqu.I("iec.short_name_id")))

	// Build definition subquery
	definitionObj := goqu.Func("jsonb_build_object",
		goqu.V("language"), goqu.I("df.language"),
		goqu.V("text"), goqu.I("df.text"),
		goqu.V("id"), goqu.I("df.id"),
	)

	definitionSubquery := dialect.From(goqu.T("lang_string_text_type_reference").As("df_ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", definitionObj))).
		Join(
			goqu.T("lang_string_text_type").As("df"),
			goqu.On(goqu.I("df.lang_string_text_type_reference_id").Eq(goqu.I("df_ref.id"))),
		).
		Where(goqu.I("df_ref.id").Eq(goqu.I("iec.definition_id")))

	// Build unit reference keys subquery
	unitReferenceKeysObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("dsu.id"),
		goqu.V("reference_type"), goqu.I("dsu.type"),
		goqu.V("key_id"), goqu.I("dsk.id"),
		goqu.V("key_type"), goqu.I("dsk.type"),
		goqu.V("key_value"), goqu.I("dsk.value"),
	)

	unitReferenceKeysSubquery := dialect.From(goqu.T("reference").As("dsu")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", unitReferenceKeysObj))).
		LeftJoin(
			goqu.T("reference_key").As("dsk"),
			goqu.On(goqu.I("dsk.reference_id").Eq(goqu.I("dsu.id"))),
		).
		Where(goqu.I("dsu.id").Eq(goqu.I("iec.unit_id")))

	// Build unit reference referred subquery
	unitReferenceReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("rootReference"), goqu.I("dsu2.rootreference"),
		goqu.V("parentReference"), goqu.I("dsu2.parentreference"),
		goqu.V("reference_type"), goqu.I("dsu2.type"),
		goqu.V("reference_id"), goqu.I("dsu2.id"),
		goqu.V("key_id"), goqu.I("dsk2.id"),
		goqu.V("key_type"), goqu.I("dsk2.type"),
		goqu.V("key_value"), goqu.I("dsk2.value"),
	)

	unitReferenceReferredSubquery := dialect.From(goqu.T("reference").As("dsu2")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", unitReferenceReferredObj))).
		LeftJoin(
			goqu.T("reference_key").As("dsk2"),
			goqu.On(goqu.I("dsk2.reference_id").Eq(goqu.I("dsu2.id"))),
		).
		Where(
			goqu.I("dsu2.rootreference").Eq(goqu.I("iec.unit_id")),
			goqu.I("dsu2.id").Neq(goqu.I("iec.unit_id")),
		)

	// Build value list value reference pair reference subquery
	vlvrpReferenceObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("vlvrp_ref.id"),
		goqu.V("reference_type"), goqu.I("vlvrp_ref.type"),
		goqu.V("key_id"), goqu.I("vlvrp_ref_rk.id"),
		goqu.V("key_type"), goqu.I("vlvrp_ref_rk.type"),
		goqu.V("key_value"), goqu.I("vlvrp_ref_rk.value"),
	)

	vlvrpReferenceSubquery := dialect.From(goqu.T("reference").As("vlvrp_ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", vlvrpReferenceObj))).
		LeftJoin(
			goqu.T("reference_key").As("vlvrp_ref_rk"),
			goqu.On(goqu.I("vlvrp_ref_rk.reference_id").Eq(goqu.I("vlvrp_ref.id"))),
		).
		Where(goqu.I("vlvrp_ref.id").Eq(goqu.I("vlvrp.value_id")))

	// Build value list value reference pair reference referred subquery
	vlvrpReferenceReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("rootReference"), goqu.I("vlvrp_referred_ref.rootreference"),
		goqu.V("parentReference"), goqu.I("vlvrp_referred_ref.parentreference"),
		goqu.V("reference_type"), goqu.I("vlvrp_referred_ref.type"),
		goqu.V("reference_id"), goqu.I("vlvrp_referred_ref.id"),
		goqu.V("key_id"), goqu.I("vlvrp_ref_rk.id"),
		goqu.V("key_type"), goqu.I("vlvrp_ref_rk.type"),
		goqu.V("key_value"), goqu.I("vlvrp_ref_rk.value"),
	)

	vlvrpReferenceReferredSubquery := dialect.From(goqu.T("reference").As("vlvrp_referred_ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", vlvrpReferenceReferredObj))).
		LeftJoin(
			goqu.T("reference_key").As("vlvrp_ref_rk"),
			goqu.On(goqu.I("vlvrp_ref_rk.reference_id").Eq(goqu.I("vlvrp_referred_ref.id"))),
		).
		Where(
			goqu.I("vlvrp_referred_ref.rootreference").Eq(goqu.I("vlvrp.value_id")),
			goqu.I("vlvrp_referred_ref.id").Neq(goqu.I("vlvrp.value_id")),
		)

	// Build value list entries subquery
	valueListEntryObj := goqu.Func("jsonb_build_object",
		goqu.V("value_reference_pair_id"), goqu.I("vlvrp.id"),
		goqu.V("value_pair_value"), goqu.I("vlvrp.value"),
		goqu.V("reference_rows"), vlvrpReferenceSubquery,
		goqu.V("referred_reference_rows"), vlvrpReferenceReferredSubquery,
	)

	valueListEntriesSubquery := dialect.From(goqu.T("value_list").As("vl")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", valueListEntryObj))).
		Join(
			goqu.T("value_list_value_reference_pair").As("vlvrp"),
			goqu.On(goqu.I("vl.id").Eq(goqu.I("vlvrp.value_list_id"))),
		).
		LeftJoin(
			goqu.T("reference").As("vlref"),
			goqu.On(goqu.I("vlvrp.value_id").Eq(goqu.I("vlref.id"))),
		).
		Where(goqu.I("vl.id").Eq(goqu.I("iec.value_list_id")))

	// Build level type subquery
	levelTypeSubquery := dialect.From(goqu.T("level_type").As("leveltype")).
		Select(goqu.Func("jsonb_build_object",
			goqu.V("min"), goqu.I("leveltype.min"),
			goqu.V("max"), goqu.I("leveltype.max"),
			goqu.V("nom"), goqu.I("leveltype.nom"),
			goqu.V("typ"), goqu.I("leveltype.typ"),
		)).
		Where(goqu.I("leveltype.id").Eq(goqu.I("iec.level_type_id")))

	// Build the main IEC61360 jsonb object
	iec61360Obj := goqu.Func("jsonb_build_object",
		goqu.V("eds_id"), goqu.I("jt.embedded_data_specification_id"),
		goqu.V("iec_id"), goqu.I("iec.id"),
		goqu.V("unit"), goqu.I("iec.unit"),
		goqu.V("source_of_definition"), goqu.I("iec.source_of_definition"),
		goqu.V("symbol"), goqu.I("iec.symbol"),
		goqu.V("data_type"), goqu.I("iec.data_type"),
		goqu.V("value_format"), goqu.I("iec.value_format"),
		goqu.V("value"), goqu.I("iec.value"),
		goqu.V("level_type_id"), goqu.I("iec.level_type_id"),
		goqu.V("preferred_name"), preferredNameSubquery,
		goqu.V("short_name"), shortNameSubquery,
		goqu.V("definition"), definitionSubquery,
		goqu.V("unit_reference_keys"), unitReferenceKeysSubquery,
		goqu.V("unit_reference_referred"), unitReferenceReferredSubquery,
		goqu.V("value_list_entries"), valueListEntriesSubquery,
		goqu.V("level_type"), levelTypeSubquery,
	)

	iec61360Subquery := dialect.From(goqu.T(joinTable).As("jt")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", iec61360Obj))).
		Join(
			goqu.T("data_specification").As("ds"),
			goqu.On(goqu.I("ds.id").Eq(goqu.I("jt.embedded_data_specification_id"))),
		).
		Join(
			goqu.T("data_specification_content").As("dsc"),
			goqu.On(goqu.I("dsc.id").Eq(goqu.I("ds.data_specification_content"))),
		).
		Join(
			goqu.T("data_specification_iec61360").As("iec"),
			goqu.On(goqu.I("iec.id").Eq(goqu.I("dsc.id"))),
		).
		Where(goqu.I("jt." + joinTableIdField).Eq(goqu.I(compareField)))
	return embeddedDataSpecificationReferenceSubquery, embeddedDataSpecificationReferenceReferredSubquery, iec61360Subquery
}
