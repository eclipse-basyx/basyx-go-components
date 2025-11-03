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

// Package submodel_query provides functions to build subqueries for retrieving submodel elements from the database.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package submodelQueries

import (
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/queries"
)

// GetSubmodelElementsSubquery builds a subquery to retrieve submodel elements for a given submodel.
func GetSubmodelElementsSubquery(dialect goqu.DialectWrapper, rootSubmodelElements bool) *goqu.SelectDataset {
	semanticIDSubquery, semanticIDReferredSubquery := queries.GetReferenceQueries(dialect, goqu.I("tlsme.semantic_id"))
	supplSemanticIDSubquery, supplSemanticIDReferredSubquery := queries.GetSupplementalSemanticIdQueries(dialect, goqu.T("submodel_element_supplemental_semantic_id"), "submodel_element_id", "reference_id", goqu.I("tlsme.id"))

	valueByType := goqu.Case().
		// When(
		// 	goqu.I("tlsme.model_type").Eq("AnnotatedRelationshipElement"),
		// 	getAnnotatedRelationshipElementSubquery(dialect),
		// ).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("BasicEventElement"),
		// 	getBasicEventElementSubquery(dialect),
		// ).
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
		// 	goqu.I("tlsme.model_type").Eq("EventElement"),
		// 	getEventElementSubquery(dialect),
		// ).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("File"),
		// 	getFileSubquery(dialect),
		// ).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("SubmodelElementList"),
		// 	getSubmodelElementListSubquery(dialect),
		// ).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("MultiLanguageProperty"),
		// 	getMultiLanguagePropertySubquery(dialect),
		// ).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("Operation"),
		// 	getOperationSubquery(dialect),
		// ).
		When(
			goqu.I("tlsme.model_type").Eq("Property"),
			getPropertySubquery(dialect),
		).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("Range"),
		// 	getRangeSubquery(dialect),
		// ).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("ReferenceElement"),
		// 	getReferenceElementSubquery(dialect),
		// ).
		// When(
		// 	goqu.I("tlsme.model_type").Eq("RelationshipElement"),
		// 	getRelationshipElementSubquery(dialect),
		// ).
		Else(goqu.V(nil))

	obj := goqu.Func("jsonb_build_object",
		goqu.V("db_id"), goqu.I("tlsme.id"),
		goqu.V("parent_id"), goqu.I("tlsme.parent_sme_id"),
		goqu.V("id_short"), goqu.I("tlsme.id_short"),
		goqu.V("category"), goqu.I("tlsme.category"),
		goqu.V("model_type"), goqu.I("tlsme.model_type"),
		goqu.V("value"), valueByType,
		goqu.V("semanticId"), semanticIDSubquery,
		goqu.V("semanticIdReferred"), semanticIDReferredSubquery,
		goqu.V("supplSemanticId"), supplSemanticIDSubquery,
		goqu.V("supplSemanticIdReferred"), supplSemanticIDReferredSubquery,
	)

	smeSubquery := dialect.From(goqu.T("submodel_element").As("tlsme")).
		Select(goqu.Func("jsonb_agg", obj))

	if rootSubmodelElements {
		smeSubquery = smeSubquery.Where(
			goqu.I("tlsme.submodel_id").Eq(goqu.I("s.id")),
			goqu.I("tlsme.parent_sme_id").IsNull(),
		)
	} else {
		smeSubquery = smeSubquery.Where(
			goqu.I("tlsme.submodel_id").Eq(goqu.I("s.id")),
			goqu.I("tlsme.parent_sme_id").IsNotNull(),
		)
	}

	return smeSubquery
}

// func getAnnotatedRelationshipElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

// func getBasicEventElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

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

// func getEventElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

// func getSubmodelElementListSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

// func getSubmodelElementCollectionSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return dialect.From(goqu.T("property_element").As("pr")).
// 		Select(
// 			goqu.Func("jsonb_build_object",
// 				goqu.V("value"), goqu.COALESCE(
// 					goqu.I("pr.value_text"),
// 					goqu.L("?::text", goqu.I("pr.value_num")),
// 					goqu.L("?::text", goqu.I("pr.value_bool")),
// 					goqu.L("?::text", goqu.I("pr.value_time")),
// 					goqu.L("?::text", goqu.I("pr.value_datetime")),
// 				),
// 				goqu.V("value_type"), goqu.I("pr.value_type"),
// 			),
// 		).
// 		Where(goqu.I("pr.id").Eq(goqu.I("tlsme.id"))).
// 		Limit(1)
// }

// func getMultiLanguagePropertySubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

// func getOperationSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

func getPropertySubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
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
			),
		).
		Where(goqu.I("pr.id").Eq(goqu.I("tlsme.id"))).
		Limit(1)
}

// func getRangeSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

// func getReferenceElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }

// func getRelationshipElementSubquery(dialect goqu.DialectWrapper) *goqu.SelectDataset {
// 	return nil
// }
