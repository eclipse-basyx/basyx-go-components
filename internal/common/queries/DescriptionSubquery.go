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
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )

package queries

import "github.com/doug-martin/goqu/v9"

// GetDescriptionQuery builds a subquery to retrieve localized description strings for an AAS element.
//
// This function constructs a SQL subquery that fetches all description entries (LangStringTextType)
// associated with a specific element through the lang_string_text_type_reference table.
// The descriptions are aggregated into a JSON array containing objects with language, text, and id fields.
//
// The query performs the following:
//   - Joins the lang_string_text_type_reference table (aliased as "dr") with lang_string_text_type (aliased as "d")
//   - Filters by the provided joinConditionColumn to match the parent element's description reference ID
//   - Aggregates all matching description entries into a JSONB array
//   - Each description object includes: language code, text content, and database ID
//
// Parameters:
//   - dialect: The SQL dialect wrapper for generating database-specific queries
//   - joinConditionColumn: The column identifier used to join with the parent element's description reference.
//     This should be a qualified column name (e.g., "sm.description_id") that references
//     the lang_string_text_type_reference.id field.
//
// Returns:
//   - *goqu.SelectDataset: A subquery that returns a JSONB array of description objects.
//     Returns an empty array if no descriptions are found for the given join condition.
//
// Example usage:
//
//	descQuery := GetDescriptionQuery(dialect, "sm.description_id")
//	// Use as part of a larger query to include descriptions in the result set
func GetDescriptionQuery(dialect goqu.DialectWrapper, joinConditionColumn string) *goqu.SelectDataset {
	descriptionObj := goqu.Func("jsonb_build_object",
		goqu.V("language"), goqu.I("d.language"),
		goqu.V("text"), goqu.I("d.text"),
		goqu.V("id"), goqu.I("d.id"),
	)

	descriptionsSubquery := dialect.From(goqu.T("lang_string_text_type_reference").As("dr")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", descriptionObj))).
		Join(
			goqu.T("lang_string_text_type").As("d"),
			goqu.On(goqu.I("d.lang_string_text_type_reference_id").Eq(goqu.I("dr.id"))),
		).
		Where(goqu.I("dr.id").Eq(goqu.I(joinConditionColumn)))
	return descriptionsSubquery
}
