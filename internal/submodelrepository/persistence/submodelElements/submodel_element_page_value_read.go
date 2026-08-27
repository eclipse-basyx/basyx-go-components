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
	"context"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

var submodelElementValueModelTypes = []types.ModelType{
	types.ModelTypeAnnotatedRelationshipElement,
	types.ModelTypeBasicEventElement,
	types.ModelTypeBlob,
	types.ModelTypeEntity,
	types.ModelTypeFile,
	types.ModelTypeSubmodelElementList,
	types.ModelTypeMultiLanguageProperty,
	types.ModelTypeOperation,
	types.ModelTypeProperty,
	types.ModelTypeRange,
	types.ModelTypeReferenceElement,
	types.ModelTypeRelationshipElement,
}

func populateSubmodelElementPageValues(
	ctx context.Context,
	db DBQueryer,
	rows []loadedSMERow,
	includeBlobValue bool,
) error {
	idsByModelType, visibleIDs := collectSubmodelElementPageValueIDs(rows)
	if !hasSubmodelElementPageValueIDs(idsByModelType) {
		return nil
	}

	query, args, err := buildSubmodelElementPageValueQuery(idsByModelType, visibleIDs, includeBlobValue)
	if err != nil {
		return common.NewInternalServerError("SMREPO-GETSMEPAGE-VALUES-BUILDQ " + err.Error())
	}
	valueRows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return common.NewInternalServerError("SMREPO-GETSMEPAGE-VALUES-EXECQ " + err.Error())
	}
	defer func() { _ = valueRows.Close() }()

	valuesByID := make(map[int64][]byte, len(rows))
	for valueRows.Next() {
		var elementID int64
		var payload []byte
		if err := valueRows.Scan(&elementID, &payload); err != nil {
			return common.NewInternalServerError("SMREPO-GETSMEPAGE-VALUES-SCANROW " + err.Error())
		}
		valuesByID[elementID] = payload
	}
	if err := valueRows.Err(); err != nil {
		return common.NewInternalServerError("SMREPO-GETSMEPAGE-VALUES-ROWSERR " + err.Error())
	}

	for index := range rows {
		if !rows[index].row.DbID.Valid {
			continue
		}
		payload, exists := valuesByID[rows[index].row.DbID.Int64]
		if exists {
			rows[index].row.Value = bytesToRawMessagePtr(payload)
		}
	}
	return nil
}

func collectSubmodelElementPageValueIDs(rows []loadedSMERow) (map[types.ModelType][]int64, []int64) {
	idsByModelType := make(map[types.ModelType][]int64, len(submodelElementValueModelTypes))
	visibleIDs := make([]int64, 0, len(rows))
	for _, item := range rows {
		if !item.row.DbID.Valid {
			continue
		}
		elementID := item.row.DbID.Int64
		modelType := types.ModelType(item.row.ModelType)
		idsByModelType[modelType] = append(idsByModelType[modelType], elementID)
		if item.valueVisible {
			visibleIDs = append(visibleIDs, elementID)
		}
	}
	return idsByModelType, visibleIDs
}

func hasSubmodelElementPageValueIDs(idsByModelType map[types.ModelType][]int64) bool {
	for _, modelType := range submodelElementValueModelTypes {
		if len(idsByModelType[modelType]) > 0 {
			return true
		}
	}
	return false
}

func buildSubmodelElementPageValueQuery(
	idsByModelType map[types.ModelType][]int64,
	visibleIDs []int64,
	includeBlobValue bool,
) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	datasets := buildSubmodelElementPageValueDatasets(dialect, idsByModelType, includeBlobValue)
	values := datasets[0]
	for _, dataset := range datasets[1:] {
		values = values.UnionAll(dataset)
	}

	const valueAlias = "submodel_element_page_value"
	value := goqu.T(valueAlias)
	maskedValue := buildMaskedSMEValuePayloadExpr(valueAlias + ".value_payload")
	query := dialect.From(values.As(valueAlias)).
		Select(
			value.Col("element_id"),
			goqu.Case().
				When(common.PostgreSQLBigIntArrayContains(value.Col("element_id"), visibleIDs), value.Col("value_payload")).
				Else(maskedValue),
		).
		Prepared(true)
	return query.ToSQL()
}

func buildSubmodelElementPageValueDatasets(
	dialect goqu.DialectWrapper,
	idsByModelType map[types.ModelType][]int64,
	includeBlobValue bool,
) []*goqu.SelectDataset {
	return []*goqu.SelectDataset{
		buildAnnotatedRelationshipPageValues(dialect, idsByModelType[types.ModelTypeAnnotatedRelationshipElement]),
		buildBasicEventPageValues(dialect, idsByModelType[types.ModelTypeBasicEventElement]),
		buildBlobPageValues(dialect, idsByModelType[types.ModelTypeBlob], includeBlobValue),
		buildEntityPageValues(dialect, idsByModelType[types.ModelTypeEntity]),
		buildFilePageValues(dialect, idsByModelType[types.ModelTypeFile]),
		buildSubmodelElementListPageValues(dialect, idsByModelType[types.ModelTypeSubmodelElementList]),
		buildMultiLanguagePropertyPageValues(dialect, idsByModelType[types.ModelTypeMultiLanguageProperty]),
		buildOperationPageValues(dialect, idsByModelType[types.ModelTypeOperation], includeBlobValue),
		buildPropertyPageValues(dialect, idsByModelType[types.ModelTypeProperty]),
		buildRangePageValues(dialect, idsByModelType[types.ModelTypeRange]),
		buildReferenceElementPageValues(dialect, idsByModelType[types.ModelTypeReferenceElement]),
		buildRelationshipElementPageValues(dialect, idsByModelType[types.ModelTypeRelationshipElement]),
	}
}

func buildSimpleSubmodelElementPageValues(
	dialect goqu.DialectWrapper,
	table string,
	alias string,
	elementIDs []int64,
	value exp.Expression,
) *goqu.SelectDataset {
	source := goqu.T(table).As(alias)
	return dialect.From(source).
		Select(
			source.Col("id").As("element_id"),
			exp.NewAliasExpression(value, "value_payload"),
		).
		Where(common.PostgreSQLBigIntArrayContains(source.Col("id"), elementIDs))
}

func buildAnnotatedRelationshipPageValues(dialect goqu.DialectWrapper, elementIDs []int64) *goqu.SelectDataset {
	const alias = "page_annotated_relationship"
	source := goqu.T(alias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("first"), source.Col("first"),
		common.PostgreSQLTextLiteral("second"), source.Col("second"),
	)
	return buildSimpleSubmodelElementPageValues(dialect, "annotated_relationship_element", alias, elementIDs, value)
}

func buildBasicEventPageValues(dialect goqu.DialectWrapper, elementIDs []int64) *goqu.SelectDataset {
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

func buildBlobPageValues(dialect goqu.DialectWrapper, elementIDs []int64, includeBlobValue bool) *goqu.SelectDataset {
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

func buildEntityPageValues(dialect goqu.DialectWrapper, elementIDs []int64) *goqu.SelectDataset {
	const alias = "page_entity"
	source := goqu.T(alias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("entity_type"), source.Col("entity_type"),
		common.PostgreSQLTextLiteral("global_asset_id"), source.Col("global_asset_id"),
		common.PostgreSQLTextLiteral("specific_asset_ids"), source.Col("specific_asset_ids"),
	)
	return buildSimpleSubmodelElementPageValues(dialect, "entity_element", alias, elementIDs, value)
}

func buildFilePageValues(dialect goqu.DialectWrapper, elementIDs []int64) *goqu.SelectDataset {
	const alias = "page_file"
	source := goqu.T(alias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("value"), source.Col("value"),
		common.PostgreSQLTextLiteral("content_type"), source.Col("content_type"),
	)
	return buildSimpleSubmodelElementPageValues(dialect, "file_element", alias, elementIDs, value)
}

func buildSubmodelElementListPageValues(dialect goqu.DialectWrapper, elementIDs []int64) *goqu.SelectDataset {
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

func buildMultiLanguagePropertyPageValues(dialect goqu.DialectWrapper, elementIDs []int64) *goqu.SelectDataset {
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
		Where(common.PostgreSQLBigIntArrayContains(goqu.I(valueAlias+".submodel_element_id"), elementIDs)).
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
		Where(common.PostgreSQLBigIntArrayContains(element.Col("id"), elementIDs))
}

func buildOperationPageValues(dialect goqu.DialectWrapper, elementIDs []int64, includeBlobValue bool) *goqu.SelectDataset {
	const alias = "page_operation"
	return buildSimpleSubmodelElementPageValues(
		dialect,
		"operation_element",
		alias,
		elementIDs,
		operationPayloadExpressionForRead(dialect, goqu.T(alias), includeBlobValue),
	)
}

func buildPropertyPageValues(dialect goqu.DialectWrapper, elementIDs []int64) *goqu.SelectDataset {
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
		Where(common.PostgreSQLBigIntArrayContains(element.Col("id"), elementIDs))
}

func buildRangePageValues(dialect goqu.DialectWrapper, elementIDs []int64) *goqu.SelectDataset {
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

func buildReferenceElementPageValues(dialect goqu.DialectWrapper, elementIDs []int64) *goqu.SelectDataset {
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

func buildRelationshipElementPageValues(dialect goqu.DialectWrapper, elementIDs []int64) *goqu.SelectDataset {
	const alias = "page_relationship"
	source := goqu.T(alias)
	value := goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("first"), source.Col("first"),
		common.PostgreSQLTextLiteral("second"), source.Col("second"),
	)
	return buildSimpleSubmodelElementPageValues(dialect, "relationship_element", alias, elementIDs, value)
}
