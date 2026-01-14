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

// GetDisplayNamesQuery builds a subquery to retrieve localized display names for an AAS element.
//
// This function constructs a SQL subquery that fetches all display name entries (LangStringNameType)
// associated with a specific element through the lang_string_name_type_reference table.
// The display names are aggregated into a JSON array containing objects with language, text, and id fields.
//
// The query performs the following:
//   - Joins the lang_string_name_type_reference table (aliased as "dn_ref") with lang_string_name_type (aliased as "dn")
//   - Filters by the provided joinConditionColumn to match the parent element's display name reference ID
//   - Aggregates all matching display name entries into a JSONB array
//   - Each display name object includes: language code, text content, and database ID
//
// Parameters:
//   - dialect: The SQL dialect wrapper for generating database-specific queries
//   - joinConditionColumn: The column identifier used to join with the parent element's display name reference.
//     This should be a qualified column name (e.g., "sm.display_name_id") that references
//     the lang_string_name_type_reference.id field.
//
// Returns:
//   - *goqu.SelectDataset: A subquery that returns a JSONB array of display name objects.
//     Returns an empty array if no display names are found for the given join condition.
//
// Example usage:
//
//	displayQuery := GetDisplayNamesQuery(dialect, "sm.display_name_id")
//	// Use as part of a larger query to include display names in the result set
func GetDisplayNamesQuery(dialect goqu.DialectWrapper, joinConditionColumn string) *goqu.SelectDataset {
	displayNameObj := goqu.Func("jsonb_build_object",
		goqu.V("language"), goqu.I("dn.language"),
		goqu.V("text"), goqu.I("dn.text"),
		goqu.V("id"), goqu.I("dn.id"),
	)

	displayNamesSubquery := dialect.From(goqu.T("lang_string_name_type_reference").As("dn_ref")).
		Select(goqu.Func("jsonb_agg", goqu.L("?", displayNameObj))).
		Join(
			goqu.T("lang_string_name_type").As("dn"),
			goqu.On(goqu.I("dn.lang_string_name_type_reference_id").Eq(goqu.I("dn_ref.id"))),
		).
		Where(goqu.I("dn_ref.id").Eq(goqu.I(joinConditionColumn)))
	return displayNamesSubquery
}
