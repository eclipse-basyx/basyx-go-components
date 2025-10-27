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

/*
type AdministrationRow struct {
	DbId                          int64           `json:"dbId"`
	Version                       string          `json:"version"`
	Revision                      string          `json:"revision"`
	TemplateId                    string          `json:"templateId"`
	Creator                       json.RawMessage `json:"creator"`
	CreatorReferred               json.RawMessage `json:"creatorReferred"`
	EdsDataSpecifications         json.RawMessage `json:"edsDataSpecifications"`
	EdsDataSpecificationsReferred json.RawMessage `json:"edsDataSpecificationsReferred"`
	EdsDataSpecificationIEC61360  json.RawMessage `json:"edsDataSpecificationIEC61360"` //iecRows
}
*/

func GetAdministrationSubqueryForSubmodel(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	administrativeInformationEmbeddedDataSpecificationReferenceSubquery, administrativeInformationEmbeddedDataSpecificationReferenceReferredSubquery, administrativeInformationIEC61360Subquery := GetEmbeddedDataSpecificationSubqueries(dialect, "administrative_information_embedded_data_specification", "administrative_information_id", "s.administration_id")

	administrationCreatorSubquery := dialect.From(goqu.T("administrative_information").As("admi")).
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
			goqu.T("reference").As("r"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("admi.creator"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("admi.id").Eq(goqu.I("s.administration_id")))

	administrationCreatorReferredSubquery := dialect.From(goqu.T("administrative_information").As("admi")).
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
			goqu.On(goqu.I("r.rootreference").Eq(goqu.I("admi.creator"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("r.id"))),
		).
		Where(
			goqu.I("r.id").IsNotNull(),
			goqu.I("admi.id").Eq(goqu.I("s.administration_id")),
		)

	administrativeInformationSubquery := dialect.From(goqu.T("administrative_information").As("ai")).
		Select(goqu.L(`
			jsonb_agg(
					DISTINCT jsonb_build_object(
						'dbId', ai.id,
						'version', ai.version,
						'revision', ai.revision,
						'templateId', ai.templateId,
						'creator', ?,
						'creatorReferred', ?,
						'edsDataSpecifications', ?,
						'edsDataSpecificationsReferred', ?,
						'edsDataSpecificationIEC61360', ?
					)
			)
		`, administrationCreatorSubquery, administrationCreatorReferredSubquery, administrativeInformationEmbeddedDataSpecificationReferenceSubquery, administrativeInformationEmbeddedDataSpecificationReferenceReferredSubquery, administrativeInformationIEC61360Subquery)).
		Where(goqu.I("ai.id").Eq(goqu.I("s.administration_id")))
	return administrativeInformationSubquery
}
