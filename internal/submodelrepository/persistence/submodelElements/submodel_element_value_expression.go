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
// Author: Jannik Fried (Fraunhofer IESE), Aaron Zielstorff (Fraunhofer IESE)

package submodelelements

import (
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

func getSMEValueExpressionForRead(dialect goqu.DialectWrapper) exp.CaseExpression {
	return goqu.Case().
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeAnnotatedRelationshipElement),
			dialect.From(goqu.T("annotated_relationship_element").As("are")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("first"), goqu.I("are.first"),
					goqu.V("second"), goqu.I("are.second"),
				)).
				Where(goqu.I("are.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeBasicEventElement),
			dialect.From(goqu.T("basic_event_element").As("bee")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("direction"), goqu.I("bee.direction"),
					goqu.V("state"), goqu.I("bee.state"),
					goqu.V("message_topic"), goqu.I("bee.message_topic"),
					goqu.V("last_update"), goqu.I("bee.last_update"),
					goqu.V("min_interval"), goqu.I("bee.min_interval"),
					goqu.V("max_interval"), goqu.I("bee.max_interval"),
					goqu.V("observed"), goqu.I("bee.observed"),
					goqu.V("message_broker"), goqu.I("bee.message_broker"),
				)).
				Where(goqu.I("bee.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeBlob),
			dialect.From(goqu.T("blob_element").As("be")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("content_type"), goqu.I("be.content_type"),
					goqu.V("value"), goqu.I("be.value"),
				)).
				Where(goqu.I("be.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeEntity),
			dialect.From(goqu.T("entity_element").As("ee")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("entity_type"), goqu.I("ee.entity_type"),
					goqu.V("global_asset_id"), goqu.I("ee.global_asset_id"),
					goqu.V("specific_asset_ids"), goqu.I("ee.specific_asset_ids"),
				)).
				Where(goqu.I("ee.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeFile),
			dialect.From(goqu.T("file_element").As("fe")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("value"), goqu.I("fe.value"),
					goqu.V("content_type"), goqu.I("fe.content_type"),
				)).
				Where(goqu.I("fe.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeSubmodelElementList),
			dialect.From(goqu.T("submodel_element_list").As("sel")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("order_relevant"), goqu.I("sel.order_relevant"),
					goqu.V("type_value_list_element"), goqu.I("sel.type_value_list_element"),
					goqu.V("value_type_list_element"), goqu.I("sel.value_type_list_element"),
					goqu.V("semantic_id_list_element"), goqu.I("sel.semantic_id_list_element"),
				)).
				Where(goqu.I("sel.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeMultiLanguageProperty),
			dialect.From(goqu.T("multilanguage_property").As("mlp")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("value_id"), goqu.COALESCE(goqu.I("mlp.value_id_payload"), goqu.L("'[]'::jsonb")),
					goqu.V("value_id_referred"), goqu.L("'[]'::jsonb"),
					goqu.V("value"),
					dialect.From(goqu.T("multilanguage_property_value").As("mlpv")).
						Select(goqu.Func("jsonb_agg", goqu.Func("jsonb_build_object",
							goqu.V("language"), goqu.I("mlpv.language"),
							goqu.V("text"), goqu.I("mlpv.text"),
							goqu.V("id"), goqu.I("mlpv.id"),
						))).
						Where(goqu.I("mlpv.mlp_id").Eq(goqu.I("sme.id"))),
				)).
				Where(goqu.I("mlp.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeOperation),
			dialect.From(goqu.T("operation_element").As("oe")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("input_variables"), goqu.I("oe.input_variables"),
					goqu.V("output_variables"), goqu.I("oe.output_variables"),
					goqu.V("inoutput_variables"), goqu.I("oe.inoutput_variables"),
				)).
				Where(goqu.I("oe.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeProperty),
			dialect.From(goqu.T("property_element").As("pe")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("value"), goqu.COALESCE(
						goqu.I("pe.value_text"),
						goqu.L("?::text", goqu.I("pe.value_num")),
						goqu.L("?::text", goqu.I("pe.value_bool")),
						goqu.L("?::text", goqu.I("pe.value_time")),
						goqu.L("?::text", goqu.I("pe.value_date")),
						goqu.L("?::text", goqu.I("pe.value_datetime")),
					),
					goqu.V("value_type"), goqu.I("pe.value_type"),
					goqu.V("value_id"), goqu.COALESCE(
						dialect.From(goqu.T("property_element_payload").As("pep")).
							Select(goqu.I("pep.value_id_payload")).
							Where(goqu.I("pep.property_element_id").Eq(goqu.I("sme.id"))).
							Limit(1),
						goqu.L("'[]'::jsonb"),
					),
					goqu.V("value_id_referred"), goqu.L("'[]'::jsonb"),
				)).
				Where(goqu.I("pe.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeRange),
			dialect.From(goqu.T("range_element").As("re")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("value_type"), goqu.I("re.value_type"),
					goqu.V("min"), goqu.COALESCE(
						goqu.I("re.min_text"),
						goqu.L("?::text", goqu.I("re.min_num")),
						goqu.L("?::text", goqu.I("re.min_time")),
						goqu.L("?::text", goqu.I("re.min_date")),
						goqu.L("?::text", goqu.I("re.min_datetime")),
					),
					goqu.V("max"), goqu.COALESCE(
						goqu.I("re.max_text"),
						goqu.L("?::text", goqu.I("re.max_num")),
						goqu.L("?::text", goqu.I("re.max_time")),
						goqu.L("?::text", goqu.I("re.max_date")),
						goqu.L("?::text", goqu.I("re.max_datetime")),
					),
				)).
				Where(goqu.I("re.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeReferenceElement),
			dialect.From(goqu.T("reference_element").As("refe")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("value"), goqu.I("refe.value"),
				)).
				Where(goqu.I("refe.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeRelationshipElement),
			dialect.From(goqu.T("relationship_element").As("rle")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("first"), goqu.I("rle.first"),
					goqu.V("second"), goqu.I("rle.second"),
				)).
				Where(goqu.I("rle.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		Else(goqu.V(nil))
}
