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

// Package submodelsubqueries provides functions to build subqueries for retrieving submodel elements from the database.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package submodelsubqueries

import (
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/queries"
)

// SubmodelElementSubmodelFilter represents a filter for submodel elements based on submodel ID.
// It is used to restrict submodel element queries to a specific submodel.
type SubmodelElementSubmodelFilter struct {
	// SubmodelIDFilter contains the submodel ID to filter by.
	// This can be any type that represents a valid submodel identifier.
	SubmodelIDFilter any
}

// SubmodelElementIDShortPathFilter represents a filter for submodel elements based on their idShort path.
// It is used to query submodel elements at a specific path within the submodel hierarchy.
type SubmodelElementIDShortPathFilter struct {
	// SubmodelElementIDShortPath contains the idShort path to filter by.
	// The path can represent a single element or a hierarchical path to nested elements.
	SubmodelElementIDShortPath string
}

// SubmodelElementFilter combines multiple filter criteria for querying submodel elements.
// It allows filtering by both submodel ID and idShort path.
type SubmodelElementFilter struct {
	// SubmodelFilter restricts the query to elements of a specific submodel.
	// If nil, no submodel filtering is applied (though this may result in an error).
	SubmodelFilter *SubmodelElementSubmodelFilter

	// SubmodelElementIDShortPathFilter restricts the query to elements at a specific idShort path.
	// If nil, no path filtering is applied.
	SubmodelElementIDShortPathFilter *SubmodelElementIDShortPathFilter
}

// HasSubmodelFilter returns true if a submodel filter is configured.
// This indicates whether the query should be restricted to a specific submodel.
func (s *SubmodelElementFilter) HasSubmodelFilter() bool {
	return s.SubmodelFilter != nil
}

// HasIDShortPathFilter returns true if an idShort path filter is configured.
// This indicates whether the query should be restricted to a specific path.
func (s *SubmodelElementFilter) HasIDShortPathFilter() bool {
	return s.SubmodelElementIDShortPathFilter != nil
}

// GetSubmodelElementsQuery builds a subquery to retrieve submodel elements for a given submodel.
func GetSubmodelElementsQuery(filter SubmodelElementFilter, cursor string, limit int, valueOnly bool) (*goqu.SelectDataset, error) {
	dialect := goqu.Dialect("postgres")

	var semanticIDSubquery, semanticIDReferredSubquery, qualifierSubquery, displayNamesSubquery, descriptionsSubquery *goqu.SelectDataset

	if valueOnly {
		// For value-only mode, return empty arrays for metadata fields
		semanticIDSubquery = dialect.From(goqu.L("(SELECT '[]'::jsonb)")).Select(goqu.L("'[]'::jsonb"))
		semanticIDReferredSubquery = dialect.From(goqu.L("(SELECT '[]'::jsonb)")).Select(goqu.L("'[]'::jsonb"))
		qualifierSubquery = dialect.From(goqu.L("(SELECT '[]'::jsonb)")).Select(goqu.L("'[]'::jsonb"))
		displayNamesSubquery = dialect.From(goqu.L("(SELECT '[]'::jsonb)")).Select(goqu.L("'[]'::jsonb"))
		descriptionsSubquery = dialect.From(goqu.L("(SELECT '[]'::jsonb)")).Select(goqu.L("'[]'::jsonb"))
	} else {
		semanticIDSubquery, semanticIDReferredSubquery = queries.GetReferenceQueries(dialect, goqu.I("sme.semantic_id"))
		qualifierSubquery = queries.GetQualifierSubquery(dialect, goqu.T("submodel_element_qualifier"), "sme_id", "qualifier_id", goqu.I("sme.id"))
		displayNamesSubquery = queries.GetDisplayNamesQuery(dialect, "sme.displayname_id")
		descriptionsSubquery = queries.GetDescriptionQuery(dialect, "sme.description_id")
	}

	valueByType := getValueSubquery(dialect)

	query := dialect.From(goqu.T("submodel_element").As("sme")).Select(
		goqu.I("sme.id").As("db_id"),
		goqu.I("sme.parent_sme_id").As("parent_id"),
		goqu.I("sme.root_sme_id").As("root_id"),
		goqu.I("sme.id_short").As("id_short"),
		goqu.I("sme.idshort_path").As("idshort_path"),
		goqu.I("sme.category").As("category"),
		goqu.I("sme.model_type").As("model_type"),
		goqu.I("sme.position").As("position"),
		goqu.L("CASE WHEN ? THEN '[]'::jsonb ELSE sme.embedded_data_specification END", valueOnly).As("embeddedDataSpecifications"),
		goqu.L("CASE WHEN ? THEN '[]'::jsonb ELSE sme.supplemental_semantic_ids END", valueOnly).As("supplementalSemanticIds"),
		goqu.L("CASE WHEN ? THEN '[]'::jsonb ELSE sme.extensions END", valueOnly).As("extensions"),
		displayNamesSubquery.As("displayNames"),
		descriptionsSubquery.As("descriptions"),
		valueByType.As("value"),
		semanticIDSubquery.As("semanticId"),
		semanticIDReferredSubquery.As("semanticIdReferred"),
		qualifierSubquery.As("qualifiers"),
	)

	if !filter.HasSubmodelFilter() {
		_ = fmt.Errorf("no SubmodelFilter provided for SubmodelElement Query, but SubmodelElements always belong to a Submodel - consider defining a SubmodelID Filter in your GetSubmodelElementsSubquery call")
		return nil, common.NewInternalServerError("unable to fetch SubmodelElements. See console for details")
	}
	query = query.Where(
		goqu.I("sme.submodel_id").Eq(filter.SubmodelFilter.SubmodelIDFilter),
	)

	if filter.HasIDShortPathFilter() {
		query = query.Where(
			goqu.Or(
				goqu.I("sme.idshort_path").Eq(filter.SubmodelElementIDShortPathFilter.SubmodelElementIDShortPath),
				goqu.I("sme.idshort_path").Like(
					goqu.L("? || '.%'", filter.SubmodelElementIDShortPathFilter.SubmodelElementIDShortPath),
				),
				goqu.I("sme.idshort_path").Like(
					goqu.L("? || '[%'", filter.SubmodelElementIDShortPathFilter.SubmodelElementIDShortPath),
				),
			),
		)
	}

	// Order by idShortPath to ensure consistent pagination
	query = query.Order(
		goqu.I("sme.idshort_path").Asc(),
	)

	// Pagination with peeking ahead on root elements only
	if (limit > 0 || cursor != "") && limit != -1 {
		rootQuery := dialect.From(goqu.T("submodel_element").As("root_sme")).
			Select(goqu.I("root_sme.id")).
			Where(goqu.I("root_sme.parent_sme_id").IsNull()).
			Where(goqu.I("root_sme.submodel_id").Eq(filter.SubmodelFilter.SubmodelIDFilter)).
			Order(goqu.I("root_sme.idshort_path").Asc())

		if cursor != "" {
			rootQuery = rootQuery.Where(goqu.I("root_sme.idshort_path").Gte(cursor))
		}

		if limit > 0 {
			// Ensure limit is non-negative before converting to uint to avoid integer overflow
			limitPlusOne := limit + 1
			if limitPlusOne < 0 {
				return nil, common.NewErrBadRequest("limit value causes integer overflow")
			}
			rootQuery = rootQuery.Limit(uint(limitPlusOne)) // Fetch one extra to check for more results
		}

		query = query.Where(goqu.I("sme.root_sme_id").In(rootQuery))
	}

	return query, nil
}

func getValueSubquery(dialect goqu.DialectWrapper) exp.CaseExpression {
	valueByType := goqu.Case().
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeAnnotatedRelationshipElement),
			getAnnotatedRelationshipElementSubquery(dialect),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeBasicEventElement),
			getBasicEventElementSubquery(dialect),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeBlob),
			getBlobSubquery(dialect),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeEntity),
			getEntitySubquery(dialect),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeFile),
			getFileSubquery(dialect),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeSubmodelElementList),
			getSubmodelElementListSubquery(dialect),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeMultiLanguageProperty),
			getMultiLanguagePropertySubquery(dialect),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeOperation),
			getOperationSubquery(dialect),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeProperty),
			getPropertySubquery(dialect),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeRange),
			getRangeSubquery(dialect),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeReferenceElement),
			getReferenceElementSubquery(dialect),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeRelationshipElement),
			getRelationshipElementSubquery(dialect),
		).
		Else(goqu.V(nil))
	return valueByType
}

func getBasicEventElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(goqu.T("basic_event_element").As("bee")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("direction"), goqu.I("bee.direction"),
				goqu.V("state"), goqu.I("bee.state"),
				goqu.V("message_topic"), goqu.I("bee.message_topic"),
				goqu.V("last_update"), goqu.I("bee.last_update"),
				goqu.V("min_interval"), goqu.I("bee.min_interval"),
				goqu.V("max_interval"), goqu.I("bee.max_interval"),
				goqu.V("observed"), goqu.I("bee.observed"),
				goqu.V("message_broker"), goqu.I("bee.message_broker"),
			),
		).
		Where(goqu.I("bee.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}

func getBlobSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	selectFunc := goqu.Func("jsonb_build_object",
		goqu.V("content_type"), goqu.I("be.content_type"),
		goqu.V("value"), goqu.L("be.value"),
	)

	return dialect.From(goqu.T("blob_element").As("be")).
		Select(
			selectFunc,
		).
		Where(goqu.I("be.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}

func getEntitySubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(goqu.T("entity_element").As("ee")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("entity_type"), goqu.I("ee.entity_type"),
				goqu.V("global_asset_id"), goqu.I("ee.global_asset_id"),
				goqu.V("specific_asset_ids"), goqu.I("ee.specific_asset_ids"),
			),
		).
		Where(goqu.I("ee.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}

func getFileSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(goqu.T("file_element").As("fe")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("value"), goqu.I("fe.value"),
				goqu.V("content_type"), goqu.I("fe.content_type"),
			),
		).
		Where(goqu.I("fe.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}

func getSubmodelElementListSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(goqu.T("submodel_element_list").As("list")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("order_relevant"), goqu.I("list.order_relevant"),
				goqu.V("type_value_list_element"), goqu.I("list.type_value_list_element"),
				goqu.V("value_type_list_element"), goqu.I("list.value_type_list_element"),
				goqu.V("semantic_id_list_element"), goqu.I("list.semantic_id_list_element"),
			),
		).
		Where(goqu.I("list.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}

func getMultiLanguagePropertySubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	valueIDSubquery, valueIDReferredSubquery := queries.GetReferenceQueries(dialect, goqu.I("mlp.value_id"))

	mlpValueObject := goqu.Func("jsonb_build_object",
		goqu.V("language"), goqu.I("mlpval.language"),
		goqu.V("text"), goqu.I("mlpval.text"),
		goqu.V("id"), goqu.I("mlpval.id"),
	)

	mlpValueSubquery := dialect.From(goqu.T("multilanguage_property_value").As("mlpval")).
		Select(goqu.Func("jsonb_agg", goqu.L("? ORDER BY mlpval.language, mlpval.text, mlpval.id", mlpValueObject))).
		Where(goqu.I("mlpval.mlp_id").Eq(goqu.I("sme.id")))

	return dialect.From(goqu.T("multilanguage_property").As("mlp")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("value_id"), valueIDSubquery,
				goqu.V("value_id_referred"), valueIDReferredSubquery,
				goqu.V("value"), mlpValueSubquery,
			),
		).
		Where(goqu.I("mlp.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}

func getOperationSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(goqu.T("operation_element").As("oe")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("input_variables"), goqu.I("oe.input_variables"),
				goqu.V("output_variables"), goqu.I("oe.output_variables"),
				goqu.V("inoutput_variables"), goqu.I("oe.inoutput_variables"),
			),
		).
		Where(goqu.I("oe.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}

func getPropertySubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	valueIDSubquery, valueIDReferredSubquery := queries.GetReferenceQueries(dialect, goqu.I("pr.value_id"))

	return dialect.From(goqu.T("property_element").As("pr")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("value"), goqu.COALESCE(
					goqu.I("pr.value_text"),
					goqu.L("?::text", goqu.I("pr.value_num")),
					goqu.L("?::text", goqu.I("pr.value_bool")),
					goqu.L("?::text", goqu.I("pr.value_time")),
					goqu.L("?::text", goqu.I("pr.value_date")),
					goqu.L("?::text", goqu.I("pr.value_datetime")),
				),
				goqu.V("value_type"), goqu.I("pr.value_type"),
				goqu.V("value_id"), valueIDSubquery,
				goqu.V("value_id_referred"), valueIDReferredSubquery,
			),
		).
		Where(goqu.I("pr.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}

func getRangeSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(goqu.T("range_element").As("range")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("value_type"), goqu.I("range.value_type"),
				goqu.V("min"), goqu.COALESCE(
					goqu.I("range.min_text"),
					goqu.L("?::text", goqu.I("range.min_num")),
					goqu.L("?::text", goqu.I("range.min_time")),
					goqu.L("?::text", goqu.I("range.min_date")),
					goqu.L("?::text", goqu.I("range.min_datetime")),
				),
				goqu.V("max"), goqu.COALESCE(
					goqu.I("range.max_text"),
					goqu.L("?::text", goqu.I("range.max_num")),
					goqu.L("?::text", goqu.I("range.max_time")),
					goqu.L("?::text", goqu.I("range.max_date")),
					goqu.L("?::text", goqu.I("range.max_datetime")),
				),
			),
		).
		Where(goqu.I("range.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}

func getReferenceElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(goqu.T("reference_element").As("re")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("value"), goqu.I("re.value"),
			),
		).
		Where(goqu.I("re.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}

func getRelationshipElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(goqu.T("relationship_element").As("re")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("first"), goqu.I("re.first"),
				goqu.V("second"), goqu.I("re.second"),
			),
		).
		Where(goqu.I("re.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}

func getAnnotatedRelationshipElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(goqu.T("annotated_relationship_element").As("are")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("first"), goqu.I("are.first"),
				goqu.V("second"), goqu.I("are.second"),
			),
		).
		Where(goqu.I("are.id").Eq(goqu.I("sme.id"))).
		Limit(1)
}
