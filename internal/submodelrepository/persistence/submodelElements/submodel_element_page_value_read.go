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

package submodelelements

import (
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const submodelElementPageValuesCTE = "submodel_element_page_values"

func buildSubmodelElementPageValueDataset(
	dialect goqu.DialectWrapper,
	includeBlobValue bool,
) *goqu.SelectDataset {
	elementIDs := dialect.From("selected_submodel_elements").Select("element_id")
	datasets := buildSubmodelElementPageValueDatasets(dialect, elementIDs, includeBlobValue)
	values := datasets[0]
	for _, dataset := range datasets[1:] {
		values = values.UnionAll(dataset)
	}
	return values
}

func buildSubmodelElementPageValueDatasets(
	dialect goqu.DialectWrapper,
	elementIDs *goqu.SelectDataset,
	includeBlobValue bool,
) []*goqu.SelectDataset {
	return []*goqu.SelectDataset{
		buildAnnotatedRelationshipPageValues(dialect, elementIDs),
		buildBasicEventPageValues(dialect, elementIDs),
		buildBlobPageValues(dialect, elementIDs, includeBlobValue),
		buildEntityPageValues(dialect, elementIDs),
		buildFilePageValues(dialect, elementIDs),
		buildSubmodelElementListPageValues(dialect, elementIDs),
		buildMultiLanguagePropertyPageValues(dialect, elementIDs),
		buildOperationPageValues(dialect, elementIDs, includeBlobValue),
		buildPropertyPageValues(dialect, elementIDs),
		buildRangePageValues(dialect, elementIDs),
		buildReferenceElementPageValues(dialect, elementIDs),
		buildRelationshipElementPageValues(dialect, elementIDs),
	}
}

func buildSimpleSubmodelElementPageValues(
	dialect goqu.DialectWrapper,
	table string,
	alias string,
	elementIDs *goqu.SelectDataset,
	value exp.Expression,
) *goqu.SelectDataset {
	source := goqu.T(table).As(alias)
	return dialect.From(source).
		Select(
			source.Col("id").As("element_id"),
			exp.NewAliasExpression(value, "value_payload"),
		).
		Where(source.Col("id").In(elementIDs))
}

func buildAnnotatedRelationshipPageValues(dialect goqu.DialectWrapper, elementIDs *goqu.SelectDataset) *goqu.SelectDataset {
	const alias = "page_annotated_relationship"
	source := goqu.T(alias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("first"), source.Col("first"),
		common.PostgreSQLTextLiteral("second"), source.Col("second"),
	)
	return buildSimpleSubmodelElementPageValues(dialect, "annotated_relationship_element", alias, elementIDs, value)
}

func buildBasicEventPageValues(dialect goqu.DialectWrapper, elementIDs *goqu.SelectDataset) *goqu.SelectDataset {
	const alias = "page_basic_event"
	source := goqu.T(alias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("direction"), source.Col("direction"),
		common.PostgreSQLTextLiteral("state"), source.Col("state"),
		common.PostgreSQLTextLiteral("message_topic"), source.Col("message_topic"),
		common.PostgreSQLTextLiteral("last_update"), source.Col("last_update"),
		common.PostgreSQLTextLiteral("min_interval"), source.Col("min_interval"),
		common.PostgreSQLTextLiteral("max_interval"), source.Col("max_interval"),
		common.PostgreSQLTextLiteral("observed"), source.Col("observed"),
		common.PostgreSQLTextLiteral("message_broker"), source.Col("message_broker"),
	)
	return buildSimpleSubmodelElementPageValues(dialect, "basic_event_element", alias, elementIDs, value)
}

func buildBlobPageValues(dialect goqu.DialectWrapper, elementIDs *goqu.SelectDataset, includeBlobValue bool) *goqu.SelectDataset {
	const alias = "page_blob"
	source := goqu.T(alias)
	fields := []interface{}{
		common.PostgreSQLTextLiteral("content_type"), source.Col("content_type"),
	}
	if includeBlobValue {
		fields = append(fields, common.PostgreSQLTextLiteral("value"), source.Col("value"))
	}
	return buildSimpleSubmodelElementPageValues(
		dialect,
		"blob_element",
		alias,
		elementIDs,
		goqu.Func("jsonb_build_object", fields...),
	)
}

func buildEntityPageValues(dialect goqu.DialectWrapper, elementIDs *goqu.SelectDataset) *goqu.SelectDataset {
	const alias = "page_entity"
	source := goqu.T(alias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("entity_type"), source.Col("entity_type"),
		common.PostgreSQLTextLiteral("global_asset_id"), source.Col("global_asset_id"),
		common.PostgreSQLTextLiteral("specific_asset_ids"), source.Col("specific_asset_ids"),
	)
	return buildSimpleSubmodelElementPageValues(dialect, "entity_element", alias, elementIDs, value)
}

func buildFilePageValues(dialect goqu.DialectWrapper, elementIDs *goqu.SelectDataset) *goqu.SelectDataset {
	const alias = "page_file"
	source := goqu.T(alias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("value"), source.Col("value"),
		common.PostgreSQLTextLiteral("content_type"), source.Col("content_type"),
	)
	return buildSimpleSubmodelElementPageValues(dialect, "file_element", alias, elementIDs, value)
}

func buildSubmodelElementListPageValues(dialect goqu.DialectWrapper, elementIDs *goqu.SelectDataset) *goqu.SelectDataset {
	const alias = "page_submodel_element_list"
	source := goqu.T(alias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("order_relevant"), source.Col("order_relevant"),
		common.PostgreSQLTextLiteral("type_value_list_element"), source.Col("type_value_list_element"),
		common.PostgreSQLTextLiteral("value_type_list_element"), source.Col("value_type_list_element"),
		common.PostgreSQLTextLiteral("semantic_id_list_element"), source.Col("semantic_id_list_element"),
	)
	return buildSimpleSubmodelElementPageValues(dialect, "submodel_element_list", alias, elementIDs, value)
}

func buildMultiLanguagePropertyPageValues(dialect goqu.DialectWrapper, elementIDs *goqu.SelectDataset) *goqu.SelectDataset {
	const (
		elementAlias = "page_multilanguage_element"
		payloadAlias = "page_multilanguage_payload"
		valuesAlias  = "page_multilanguage_values"
		valueAlias   = "page_multilanguage_value"
	)
	valueRow := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("language"), goqu.I(valueAlias+".language"),
		common.PostgreSQLTextLiteral("text"), goqu.I(valueAlias+".text"),
		common.PostgreSQLTextLiteral("id"), goqu.I(valueAlias+".id"),
	)
	orderedValue := goqu.L(
		"? ORDER BY ?",
		valueRow,
		goqu.I(valueAlias+".id"),
	)
	values := dialect.From(goqu.T("multilanguage_property_value").As(valueAlias)).
		Select(
			goqu.I(valueAlias+".submodel_element_id").As("element_id"),
			goqu.Func("jsonb_agg", orderedValue).As("value_payload"),
		).
		Where(goqu.I(valueAlias + ".submodel_element_id").In(elementIDs)).
		GroupBy(goqu.I(valueAlias + ".submodel_element_id"))

	element := goqu.T("submodel_element").As(elementAlias)
	payload := goqu.T("multilanguage_property_payload").As(payloadAlias)
	aggregatedValues := goqu.T(valuesAlias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("value_id"),
		goqu.COALESCE(payload.Col("value_id_payload"), goqu.L("'[]'::jsonb")),
		common.PostgreSQLTextLiteral("value_id_referred"), goqu.L("'[]'::jsonb"),
		common.PostgreSQLTextLiteral("value"), aggregatedValues.Col("value_payload"),
	)
	return dialect.From(element).
		LeftJoin(payload, goqu.On(payload.Col("submodel_element_id").Eq(element.Col("id")))).
		LeftJoin(values.As(valuesAlias), goqu.On(aggregatedValues.Col("element_id").Eq(element.Col("id")))).
		Select(
			element.Col("id").As("element_id"),
			value.As("value_payload"),
		).
		Where(element.Col("id").In(elementIDs))
}

func buildOperationPageValues(dialect goqu.DialectWrapper, elementIDs *goqu.SelectDataset, includeBlobValue bool) *goqu.SelectDataset {
	const alias = "page_operation"
	return buildSimpleSubmodelElementPageValues(
		dialect,
		"operation_element",
		alias,
		elementIDs,
		operationPayloadExpressionForRead(dialect, goqu.T(alias), includeBlobValue),
	)
}

func buildPropertyPageValues(dialect goqu.DialectWrapper, elementIDs *goqu.SelectDataset) *goqu.SelectDataset {
	const (
		elementAlias = "page_property"
		payloadAlias = "page_property_payload"
	)
	element := goqu.T("property_element").As(elementAlias)
	payload := goqu.T("property_element_payload").As(payloadAlias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("value"), goqu.COALESCE(
			element.Col("value_text"),
			goqu.Cast(element.Col("value_num"), "text"),
			goqu.Cast(element.Col("value_bool"), "text"),
			temporalColumnAsText(element.Col("value_time")),
			temporalColumnAsText(element.Col("value_date")),
			temporalColumnAsText(element.Col("value_datetime")),
		),
		common.PostgreSQLTextLiteral("value_type"), element.Col("value_type"),
		common.PostgreSQLTextLiteral("value_id"),
		goqu.COALESCE(payload.Col("value_id_payload"), goqu.L("'[]'::jsonb")),
		common.PostgreSQLTextLiteral("value_id_referred"), goqu.L("'[]'::jsonb"),
	)
	return dialect.From(element).
		LeftJoin(payload, goqu.On(payload.Col("property_element_id").Eq(element.Col("id")))).
		Select(
			element.Col("id").As("element_id"),
			value.As("value_payload"),
		).
		Where(element.Col("id").In(elementIDs))
}

func buildRangePageValues(dialect goqu.DialectWrapper, elementIDs *goqu.SelectDataset) *goqu.SelectDataset {
	const alias = "page_range"
	source := goqu.T(alias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("value_type"), source.Col("value_type"),
		common.PostgreSQLTextLiteral("min"), goqu.COALESCE(
			source.Col("min_text"),
			goqu.Cast(source.Col("min_num"), "text"),
			temporalColumnAsText(source.Col("min_time")),
			temporalColumnAsText(source.Col("min_date")),
			temporalColumnAsText(source.Col("min_datetime")),
		),
		common.PostgreSQLTextLiteral("max"), goqu.COALESCE(
			source.Col("max_text"),
			goqu.Cast(source.Col("max_num"), "text"),
			temporalColumnAsText(source.Col("max_time")),
			temporalColumnAsText(source.Col("max_date")),
			temporalColumnAsText(source.Col("max_datetime")),
		),
	)
	return buildSimpleSubmodelElementPageValues(dialect, "range_element", alias, elementIDs, value)
}

func buildReferenceElementPageValues(dialect goqu.DialectWrapper, elementIDs *goqu.SelectDataset) *goqu.SelectDataset {
	const alias = "page_reference"
	source := goqu.T(alias)
	return buildSimpleSubmodelElementPageValues(
		dialect,
		"reference_element",
		alias,
		elementIDs,
		goqu.Func("jsonb_build_object", common.PostgreSQLTextLiteral("value"), source.Col("value")),
	)
}

func buildRelationshipElementPageValues(dialect goqu.DialectWrapper, elementIDs *goqu.SelectDataset) *goqu.SelectDataset {
	const alias = "page_relationship"
	source := goqu.T(alias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("first"), source.Col("first"),
		common.PostgreSQLTextLiteral("second"), source.Col("second"),
	)
	return buildSimpleSubmodelElementPageValues(dialect, "relationship_element", alias, elementIDs, value)
}
