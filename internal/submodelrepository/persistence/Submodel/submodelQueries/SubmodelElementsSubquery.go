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

// Package submodelsubqueries provides functions to build subqueries for retrieving submodel elements from the database.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package submodelsubqueries

import (
	"fmt"

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

// GetSubmodelElementsSubquery builds a subquery to retrieve submodel elements for a given submodel.
func GetSubmodelElementsSubquery(dialect goqu.DialectWrapper, rootSubmodelElements bool, filter SubmodelElementFilter) (*goqu.SelectDataset, error) {
	semanticIDSubquery, semanticIDReferredSubquery := queries.GetReferenceQueries(dialect, goqu.I("tlsme.semantic_id"))
	supplSemanticIDSubquery, supplSemanticIDReferredSubquery := queries.GetSupplementalSemanticIDQueries(dialect, goqu.T("submodel_element_supplemental_semantic_id"), "submodel_element_id", "reference_id", goqu.I("tlsme.id"))
	qualifierSubquery := queries.GetQualifierSubquery(dialect, goqu.T("submodel_element_qualifier"), "sme_id", "qualifier_id", goqu.I("tlsme.id"))
	displayNamesSubquery := queries.GetDisplayNamesQuery(dialect, "tlsme.displayname_id")
	descriptionsSubquery := queries.GetDescriptionQuery(dialect, "tlsme.description_id")

	valueByType := getValueSubquery(dialect)

	obj := goqu.Func("jsonb_build_object",
		goqu.V("db_id"), goqu.I("tlsme.id"),
		goqu.V("parent_id"), goqu.I("tlsme.parent_sme_id"),
		goqu.V("root_id"), goqu.I("tlsme.root_sme_id"),
		goqu.V("id_short"), goqu.I("tlsme.id_short"),
		goqu.V("id_short_path"), goqu.I("tlsme.idshort_path"),
		goqu.V("category"), goqu.I("tlsme.category"),
		goqu.V("model_type"), goqu.I("tlsme.model_type"),
		goqu.V("position"), goqu.I("tlsme.position"),
		goqu.V("embeddedDataSpecifications"), goqu.I("tlsme.embedded_data_specification"),
		goqu.V("displayNames"), displayNamesSubquery,
		goqu.V("descriptions"), descriptionsSubquery,
		goqu.V("value"), valueByType,
		goqu.V("semanticId"), semanticIDSubquery,
		goqu.V("semanticIdReferred"), semanticIDReferredSubquery,
		goqu.V("supplementalSemanticIdReferenceRows"), supplSemanticIDSubquery,
		goqu.V("supplementalSemanticIdReferredReferenceRows"), supplSemanticIDReferredSubquery,
		goqu.V("qualifiers"), qualifierSubquery,
	)

	smeSubquery := dialect.From(goqu.T("submodel_element").As("tlsme")).
		Select(goqu.Func("jsonb_agg", obj))

	if filter.HasSubmodelFilter() {
		smeSubquery = smeSubquery.Where(
			goqu.I("tlsme.submodel_id").Eq(filter.SubmodelFilter.SubmodelIDFilter),
		)
	} else {
		_ = fmt.Errorf("no SubmodelFilter provided for SubmodelElement Query, but SubmodelElements always belong to a Submodel - consider defining a SubmodelID Filter in your GetSubmodelELementsSubquery call")
		return nil, common.NewInternalServerError("unable to fetch SubmodelElements. See console for details")
	}

	if filter.HasIDShortPathFilter() {
		smeSubquery = smeSubquery.Where(
			// (idshort_path = $2 OR idshort_path LIKE $2 || '.%' OR idshort_path LIKE $2 || '[%')
			goqu.Or(
				goqu.I("tlsme.idshort_path").Eq(filter.SubmodelElementIDShortPathFilter.SubmodelElementIDShortPath),
				goqu.I("tlsme.idshort_path").Like(
					goqu.L("? || '.%'", filter.SubmodelElementIDShortPathFilter.SubmodelElementIDShortPath),
				),
				goqu.I("tlsme.idshort_path").Like(
					goqu.L("? || '[%'", filter.SubmodelElementIDShortPathFilter.SubmodelElementIDShortPath),
				),
			),
		)
	}

	if rootSubmodelElements {
		smeSubquery = smeSubquery.Where(
			goqu.I("tlsme.parent_sme_id").IsNull(),
		)
	} else {
		smeSubquery = smeSubquery.Where(
			goqu.I("tlsme.parent_sme_id").IsNotNull(),
		)
	}

	return smeSubquery, nil
}

func getValueSubquery(dialect goqu.DialectWrapper) exp.CaseExpression {
	valueByType := goqu.Case().
		// When(
		// 	goqu.I("tlsme.model_type").Eq("AnnotatedRelationshipElement"),
		// 	getAnnotatedRelationshipElementSubquery(dialect),
		// ).
		When(
			goqu.I("tlsme.model_type").Eq("BasicEventElement"),
			getBasicEventElementSubquery(dialect),
		).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("Blob"),
		// 	getBlobSubquery(dialect),
		// ).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("Capability"),
		// 	getCapabilitySubquery(dialect),
		// ).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("Entity"),
		// 	getEntitySubquery(dialect),
		// ).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("File"),
		// 	getFileSubquery(dialect),
		// ).
		When(
			goqu.I("tlsme.model_type").Eq("SubmodelElementList"),
			getSubmodelElementListSubquery(dialect),
		).
		When(
			goqu.I("tlsme.model_type").Eq("MultiLanguageProperty"),
			getMultiLanguagePropertySubquery(dialect),
		).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("Operation"),
		// 	getOperationSubquery(dialect),
		// ).
		When(
			goqu.I("tlsme.model_type").Eq("Property"),
			getPropertySubquery(dialect),
		).
		When(
			goqu.I("tlsme.model_type").Eq("Range"),
			getRangeSubquery(dialect),
		).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("ReferenceElement"),
		// 	getReferenceElementSubquery(dialect),
		// ).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("RelationshipElement"),
		// 	getRelationshipElementSubquery(dialect),
		// ).
		Else(goqu.V(nil))
	return valueByType
}

// func getAnnotatedRelationshipElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

func getBasicEventElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	observedRef, observedRefReferred := queries.GetReferenceQueries(dialect, goqu.I("bee.observed_ref"))
	messageBrokerRef, messageBrokerRefReferred := queries.GetReferenceQueries(dialect, goqu.I("bee.message_broker_ref"))

	return dialect.From(goqu.T("basic_event_element").As("bee")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("direction"), goqu.I("bee.direction"),
				goqu.V("state"), goqu.I("bee.state"),
				goqu.V("message_topic"), goqu.I("bee.message_topic"),
				goqu.V("last_update"), goqu.I("bee.last_update"),
				goqu.V("min_interval"), goqu.I("bee.min_interval"),
				goqu.V("max_interval"), goqu.I("bee.max_interval"),
				goqu.V("observed_ref"), observedRef,
				goqu.V("observed_ref_referred"), observedRefReferred,
				goqu.V("message_broker_ref"), messageBrokerRef,
				goqu.V("message_broker_ref_referred"), messageBrokerRefReferred,
			),
		).
		Where(goqu.I("bee.id").Eq(goqu.I("tlsme.id"))).
		Limit(1)
}

// func getBlobSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

// func getCapabilitySubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

// func getEntitySubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

// func getFileSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

func getSubmodelElementListSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	semanticIDListElement, semanticIDListElementReferred := queries.GetReferenceQueries(dialect, goqu.I("list.semantic_id_list_element"))

	return dialect.From(goqu.T("submodel_element_list").As("list")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("order_relevant"), goqu.I("list.order_relevant"),
				goqu.V("type_value_list_element"), goqu.I("list.type_value_list_element"),
				goqu.V("value_type_list_element"), goqu.I("list.value_type_list_element"),
				goqu.V("semantic_id_list_element"), semanticIDListElement,
				goqu.V("semantic_id_list_element_referred"), semanticIDListElementReferred,
			),
		).
		Where(goqu.I("list.id").Eq(goqu.I("tlsme.id"))).
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
		Select(goqu.Func("jsonb_agg", goqu.L("?", mlpValueObject))).
		Where(goqu.I("mlpval.id").Eq(goqu.I("tlsme.id")))

	return dialect.From(goqu.T("multilanguage_property").As("mlp")).
		Select(
			goqu.Func("jsonb_build_object",
				goqu.V("value_id"), valueIDSubquery,
				goqu.V("value_id_referred"), valueIDReferredSubquery,
				goqu.V("value"), mlpValueSubquery,
			),
		).
		Where(goqu.I("mlp.id").Eq(goqu.I("tlsme.id"))).
		Limit(1)
}

// func getOperationSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

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
					goqu.L("?::text", goqu.I("pr.value_datetime")),
				),
				goqu.V("value_type"), goqu.I("pr.value_type"),
				goqu.V("value_id"), valueIDSubquery,
				goqu.V("value_id_referred"), valueIDReferredSubquery,
			),
		).
		Where(goqu.I("pr.id").Eq(goqu.I("tlsme.id"))).
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
					goqu.L("?::text", goqu.I("range.min_datetime")),
				),
				goqu.V("max"), goqu.COALESCE(
					goqu.I("range.max_text"),
					goqu.L("?::text", goqu.I("range.max_num")),
					goqu.L("?::text", goqu.I("range.max_time")),
					goqu.L("?::text", goqu.I("range.max_datetime")),
				),
			),
		).
		Where(goqu.I("range.id").Eq(goqu.I("tlsme.id"))).
		Limit(1)
}

// func getReferenceElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

// func getRelationshipElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }
