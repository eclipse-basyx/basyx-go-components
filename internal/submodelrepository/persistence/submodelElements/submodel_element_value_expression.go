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
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func temporalColumnAsText(column exp.IdentifierExpression) exp.LiteralExpression {
	return goqu.L(`trim(both '"' from to_json(?)::text)`, column)
}

func operationValueExpressionForRead(dialect goqu.DialectWrapper, includeBlobValue bool) exp.Expression {
	var payload exp.Expression = goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("input_variables"), goqu.I("oe.input_variables"),
		common.PostgreSQLTextLiteral("output_variables"), goqu.I("oe.output_variables"),
		common.PostgreSQLTextLiteral("inoutput_variables"), goqu.I("oe.inoutput_variables"),
	)
	if !includeBlobValue {
		payload = withoutEmbeddedBlobValues(dialect, payload)
	}

	return dialect.From(goqu.T("operation_element").As("oe")).
		Select(payload).
		Where(goqu.I("oe.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}

func withoutEmbeddedBlobValues(dialect goqu.DialectWrapper, payload exp.Expression) exp.Expression {
	node := goqu.I("parent.node")
	objectNode := goqu.Case().
		When(goqu.Func("jsonb_typeof", node).Eq("object"), node).
		Else(goqu.L("'{}'::jsonb"))
	arrayNode := goqu.Case().
		When(goqu.Func("jsonb_typeof", node).Eq("array"), node).
		Else(goqu.L("'[]'::jsonb"))

	objectChildren := dialect.
		From(goqu.Func("jsonb_each", objectNode).As("object_child")).
		Select(
			goqu.I("object_child.key"),
			goqu.I("object_child.value"),
		)
	arrayIndex := goqu.I("array_child.array_index")
	arrayChildren := dialect.
		From(goqu.L(
			"? AS array_child(array_index)",
			goqu.Func(
				"generate_series",
				0,
				goqu.L("? - 1", goqu.Func("jsonb_array_length", arrayNode)),
			),
		)).
		Select(
			goqu.Cast(arrayIndex, "text"),
			goqu.L("? -> ?", node, arrayIndex),
		)
	children := objectChildren.UnionAll(arrayChildren).As("child")

	jsonNodes := dialect.
		Select(
			goqu.L("ARRAY[]::text[]"),
			payload,
		).
		UnionAll(
			dialect.
				From(
					goqu.T("operation_json_nodes").As("parent"),
					goqu.Lateral(children),
				).
				Select(
					goqu.L("? || ARRAY[?]", goqu.I("parent.path"), goqu.I("child.key")),
					goqu.I("child.value"),
				),
		)
	blobValuePaths := dialect.
		From("operation_json_nodes").
		Select(
			goqu.ROW_NUMBER().Over(goqu.W().OrderBy(goqu.I("operation_json_nodes.path").Asc())),
			goqu.L("? || ARRAY[?]", goqu.I("operation_json_nodes.path"), common.PostgreSQLTextLiteral("value")),
		).
		Where(goqu.L(
			"? ->> ? = ?",
			goqu.I("operation_json_nodes.node"),
			common.PostgreSQLTextLiteral("modelType"),
			common.PostgreSQLTextLiteral("Blob"),
		))
	strippedPayload := dialect.
		Select(
			goqu.L("0::bigint"),
			payload,
		).
		UnionAll(
			dialect.
				From(goqu.T("stripped_operation_payload").As("current")).
				Join(
					goqu.T("blob_value_paths").As("target"),
					goqu.On(goqu.I("target.position").Eq(goqu.L("? + 1", goqu.I("current.position")))),
				).
				Select(
					goqu.L("? + 1", goqu.I("current.position")),
					goqu.L("? #- ?", goqu.I("current.payload"), goqu.I("target.path")),
				),
		)

	return dialect.
		From("stripped_operation_payload").
		Select("payload").
		WithRecursive("operation_json_nodes(path, node)", jsonNodes).
		With("blob_value_paths(position, path)", blobValuePaths).
		With("stripped_operation_payload(position, payload)", strippedPayload).
		Order(goqu.I("position").Desc()).
		Limit(1)
}

func getSMEValueExpressionForRead(dialect goqu.DialectWrapper, includeBlobValue bool) exp.CaseExpression {
	blobPayload := []interface{}{
		common.PostgreSQLTextLiteral("content_type"), goqu.I("be.content_type"),
	}
	if includeBlobValue {
		blobPayload = append(blobPayload, common.PostgreSQLTextLiteral("value"), goqu.I("be.value"))
	}

	return goqu.Case().
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeAnnotatedRelationshipElement),
			dialect.From(goqu.T("annotated_relationship_element").As("are")).
				Select(goqu.Func("jsonb_build_object",
					common.PostgreSQLTextLiteral("first"), goqu.I("are.first"),
					common.PostgreSQLTextLiteral("second"), goqu.I("are.second"),
				)).
				Where(goqu.I("are.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeBasicEventElement),
			dialect.From(goqu.T("basic_event_element").As("bee")).
				Select(goqu.Func("jsonb_build_object",
					common.PostgreSQLTextLiteral("direction"), goqu.I("bee.direction"),
					common.PostgreSQLTextLiteral("state"), goqu.I("bee.state"),
					common.PostgreSQLTextLiteral("message_topic"), goqu.I("bee.message_topic"),
					common.PostgreSQLTextLiteral("last_update"), goqu.I("bee.last_update"),
					common.PostgreSQLTextLiteral("min_interval"), goqu.I("bee.min_interval"),
					common.PostgreSQLTextLiteral("max_interval"), goqu.I("bee.max_interval"),
					common.PostgreSQLTextLiteral("observed"), goqu.I("bee.observed"),
					common.PostgreSQLTextLiteral("message_broker"), goqu.I("bee.message_broker"),
				)).
				Where(goqu.I("bee.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeBlob),
			dialect.From(goqu.T("blob_element").As("be")).
				Select(goqu.Func("jsonb_build_object", blobPayload...)).
				Where(goqu.I("be.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeEntity),
			dialect.From(goqu.T("entity_element").As("ee")).
				Select(goqu.Func("jsonb_build_object",
					common.PostgreSQLTextLiteral("entity_type"), goqu.I("ee.entity_type"),
					common.PostgreSQLTextLiteral("global_asset_id"), goqu.I("ee.global_asset_id"),
					common.PostgreSQLTextLiteral("specific_asset_ids"), goqu.I("ee.specific_asset_ids"),
				)).
				Where(goqu.I("ee.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeFile),
			dialect.From(goqu.T("file_element").As("fe")).
				Select(goqu.Func("jsonb_build_object",
					common.PostgreSQLTextLiteral("value"), goqu.I("fe.value"),
					common.PostgreSQLTextLiteral("content_type"), goqu.I("fe.content_type"),
				)).
				Where(goqu.I("fe.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeSubmodelElementList),
			dialect.From(goqu.T("submodel_element_list").As("sel")).
				Select(goqu.Func("jsonb_build_object",
					common.PostgreSQLTextLiteral("order_relevant"), goqu.I("sel.order_relevant"),
					common.PostgreSQLTextLiteral("type_value_list_element"), goqu.I("sel.type_value_list_element"),
					common.PostgreSQLTextLiteral("value_type_list_element"), goqu.I("sel.value_type_list_element"),
					common.PostgreSQLTextLiteral("semantic_id_list_element"), goqu.I("sel.semantic_id_list_element"),
				)).
				Where(goqu.I("sel.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeMultiLanguageProperty),
			goqu.Func("jsonb_build_object",
				common.PostgreSQLTextLiteral("value_id"), goqu.COALESCE(
					dialect.From(goqu.T("multilanguage_property_payload").As("mlpp")).
						Select(goqu.I("mlpp.value_id_payload")).
						Where(goqu.I("mlpp.submodel_element_id").Eq(goqu.I("sme.id"))).
						Limit(1),
					goqu.L("'[]'::jsonb"),
				),
				common.PostgreSQLTextLiteral("value_id_referred"), goqu.L("'[]'::jsonb"),
				common.PostgreSQLTextLiteral("value"),
				dialect.From(goqu.T("multilanguage_property_value").As("mlpv")).
					Select(goqu.Func("jsonb_agg", goqu.Func("jsonb_build_object",
						common.PostgreSQLTextLiteral("language"), goqu.I("mlpv.language"),
						common.PostgreSQLTextLiteral("text"), goqu.I("mlpv.text"),
						common.PostgreSQLTextLiteral("id"), goqu.I("mlpv.id"),
					))).
					Where(goqu.I("mlpv.submodel_element_id").Eq(goqu.I("sme.id"))),
			),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeOperation),
			operationValueExpressionForRead(dialect, includeBlobValue),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeProperty),
			dialect.From(goqu.T("property_element").As("pe")).
				Select(goqu.Func("jsonb_build_object",
					common.PostgreSQLTextLiteral("value"), goqu.COALESCE(
						goqu.I("pe.value_text"),
						goqu.L("?::text", goqu.I("pe.value_num")),
						goqu.L("?::text", goqu.I("pe.value_bool")),
						temporalColumnAsText(goqu.I("pe.value_time")),
						temporalColumnAsText(goqu.I("pe.value_date")),
						temporalColumnAsText(goqu.I("pe.value_datetime")),
					),
					common.PostgreSQLTextLiteral("value_type"), goqu.I("pe.value_type"),
					common.PostgreSQLTextLiteral("value_id"), goqu.COALESCE(
						dialect.From(goqu.T("property_element_payload").As("pep")).
							Select(goqu.I("pep.value_id_payload")).
							Where(goqu.I("pep.property_element_id").Eq(goqu.I("sme.id"))).
							Limit(1),
						goqu.L("'[]'::jsonb"),
					),
					common.PostgreSQLTextLiteral("value_id_referred"), goqu.L("'[]'::jsonb"),
				)).
				Where(goqu.I("pe.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeRange),
			dialect.From(goqu.T("range_element").As("re")).
				Select(goqu.Func("jsonb_build_object",
					common.PostgreSQLTextLiteral("value_type"), goqu.I("re.value_type"),
					common.PostgreSQLTextLiteral("min"), goqu.COALESCE(
						goqu.I("re.min_text"),
						goqu.L("?::text", goqu.I("re.min_num")),
						temporalColumnAsText(goqu.I("re.min_time")),
						temporalColumnAsText(goqu.I("re.min_date")),
						temporalColumnAsText(goqu.I("re.min_datetime")),
					),
					common.PostgreSQLTextLiteral("max"), goqu.COALESCE(
						goqu.I("re.max_text"),
						goqu.L("?::text", goqu.I("re.max_num")),
						temporalColumnAsText(goqu.I("re.max_time")),
						temporalColumnAsText(goqu.I("re.max_date")),
						temporalColumnAsText(goqu.I("re.max_datetime")),
					),
				)).
				Where(goqu.I("re.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeReferenceElement),
			dialect.From(goqu.T("reference_element").As("refe")).
				Select(goqu.Func("jsonb_build_object",
					common.PostgreSQLTextLiteral("value"), goqu.I("refe.value"),
				)).
				Where(goqu.I("refe.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeRelationshipElement),
			dialect.From(goqu.T("relationship_element").As("rle")).
				Select(goqu.Func("jsonb_build_object",
					common.PostgreSQLTextLiteral("first"), goqu.I("rle.first"),
					common.PostgreSQLTextLiteral("second"), goqu.I("rle.second"),
				)).
				Where(goqu.I("rle.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		Else(goqu.V(nil))
}
