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

package queries

import "github.com/doug-martin/goqu/v9"

/*
type AdministrationRow struct {
	DbID                          int64           `json:"dbId"`
	Version                       string          `json:"version"`
	Revision                      string          `json:"revision"`
	TemplateID                    string          `json:"templateId"`
	Creator                       json.RawMessage `json:"creator"`
	CreatorReferred               json.RawMessage `json:"creatorReferred"`
	EdsDataSpecifications         json.RawMessage `json:"edsDataSpecifications"`
	EdsDataSpecificationsReferred json.RawMessage `json:"edsDataSpecificationsReferred"`
	EdsDataSpecificationIEC61360  json.RawMessage `json:"edsDataSpecificationIEC61360"` //iecRows
}
*/

// GetAdministrationSubquery constructs a complex SQL subquery for retrieving
// administrative information related to a submodel or other AAS elements. This function
// builds a comprehensive query that includes:
//   - Administrative information (version, revision, templateId)
//   - Creator references with their keys
//   - Creator referred references (hierarchical references)
//   - Embedded data specifications and their references
//   - IEC 61360 data specifications
//
// The function creates multiple nested subqueries to aggregate related data into JSONB objects,
// allowing for efficient retrieval of all administrative information in a single query.
//
// Parameters:
//   - dialect: The goqu dialect wrapper for database-specific SQL generation
//   - joinConditionColumn: The column name to use for joining with the administrative_information
//     table. This should be a fully qualified column name (e.g., "s.administration_id")
//     that references the administrative information ID in the parent query context.
//
// Returns:
//   - *goqu.SelectDataset: A configured select dataset that can be used as a subquery
//     in larger queries to retrieve administrative information. Returns a JSONB aggregation
//     of all matching administrative records.
//
// Example usage:
//
//	adminSubquery := GetAdministrationSubquery(dialect, "s.administration_id")
//	mainQuery := dialect.From("submodel").As("s").
//	    Select(..., goqu.L("?", adminSubquery).As("administration"))
func GetAdministrationSubquery(dialect goqu.DialectWrapper, joinConditionColumn string) *goqu.SelectDataset {
	administrativeInformationEmbeddedDataSpecificationReferenceSubquery, administrativeInformationEmbeddedDataSpecificationReferenceReferredSubquery, administrativeInformationIEC61360Subquery := GetEmbeddedDataSpecificationSubqueries(dialect, "administrative_information_embedded_data_specification", "administrative_information_id", joinConditionColumn)

	// Build the jsonb object for administration creator references
	creatorObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	administrationCreatorSubquery := dialect.From(goqu.T("administrative_information").As("admi")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", creatorObj))).
		Join(
			goqu.T("reference").As("r"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("admi.creator"))),
		).
		Join(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("r.id").Eq(goqu.I("rk.reference_id"))),
		).
		Where(goqu.I("admi.id").Eq(goqu.I(joinConditionColumn)))

	// Build the jsonb object for administration creator referred references
	creatorReferredObj := goqu.Func("jsonb_build_object",
		goqu.V("reference_id"), goqu.I("r.id"),
		goqu.V("reference_type"), goqu.I("r.type"),
		goqu.V("parentReference"), goqu.I("r.parentreference"),
		goqu.V("rootReference"), goqu.I("r.rootreference"),
		goqu.V("key_id"), goqu.I("rk.id"),
		goqu.V("key_type"), goqu.I("rk.type"),
		goqu.V("key_value"), goqu.I("rk.value"),
	)

	administrationCreatorReferredSubquery := dialect.From(goqu.T("administrative_information").As("admi")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", creatorReferredObj))).
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
			goqu.I("admi.id").Eq(goqu.I(joinConditionColumn)),
		)

	administrativeInformationObj := goqu.Func("jsonb_build_object",
		goqu.V("dbId"), goqu.C("id").Table("ai"),
		goqu.V("version"), goqu.C("version").Table("ai"),
		goqu.V("revision"), goqu.C("revision").Table("ai"),
		goqu.V("templateId"), goqu.C("templateid").Table("ai"),
		goqu.V("creator"), goqu.L("?", administrationCreatorSubquery),
		goqu.V("creatorReferred"), goqu.L("?", administrationCreatorReferredSubquery),
		goqu.V("edsDataSpecifications"), goqu.L("?", administrativeInformationEmbeddedDataSpecificationReferenceSubquery),
		goqu.V("edsDataSpecificationsReferred"), goqu.L("?", administrativeInformationEmbeddedDataSpecificationReferenceReferredSubquery),
		goqu.V("edsDataSpecificationIEC61360"), goqu.L("?", administrativeInformationIEC61360Subquery),
	)

	administrativeInformationSubquery := dialect.From(goqu.T("administrative_information").As("ai")).
		Select(goqu.Func("jsonb_agg", administrativeInformationObj)).
		Where(goqu.I("ai.id").Eq(goqu.I(joinConditionColumn)))
	return administrativeInformationSubquery
}
