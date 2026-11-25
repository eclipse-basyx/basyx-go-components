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

package grammar

import (
	"math"
	"strconv"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

// XPath casts an xs:double whose absolute value lies in [xsdDoubleDecimalMin, xsdDoubleDecimalMax)
// to xs:string as an xs:decimal and every other finite, non-zero value in scientific notation.
const (
	xsdDoubleDecimalMin = 1e-6
	xsdDoubleDecimalMax = 1e6
)

// canonicalDoubleString casts an xs:double to xs:string as XPath does, using the shortest
// digits that read back as the same value, for example "1.5", "1.0E6" and "-1.25E-7".
func canonicalDoubleString(value float64) string {
	switch {
	case math.IsNaN(value):
		return "NaN"
	case math.IsInf(value, 1):
		return "INF"
	case math.IsInf(value, -1):
		return "-INF"
	case value == 0 && math.Signbit(value):
		return "-0"
	case value == 0:
		return "0"
	}
	if magnitude := math.Abs(value); magnitude >= xsdDoubleDecimalMin && magnitude < xsdDoubleDecimalMax {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	mantissa, exponent, _ := strings.Cut(strconv.FormatFloat(value, 'E', -1, 64), "E")
	if !strings.Contains(mantissa, ".") {
		mantissa += ".0"
	}
	power, _ := strconv.Atoi(exponent)
	return mantissa + "E" + strconv.Itoa(power)
}

// canonicalDoubleSQLText casts an SQL double precision value to xs:string the way
// canonicalDoubleString does. PostgreSQL prints a double with digits that read back as the same
// value but, unlike Go, never with a decimal that lies exactly halfway to a neighbouring double,
// for example "9.999999999999999e+22" for 1e23. The shortest digits are therefore searched among
// the decimals next to the printed value: for each precision, the value truncated to it and the
// next larger decimal. The first that reads back as the double wins, the closer one within a
// precision. The operand is bound once in a subquery fenced with OFFSET 0.
func canonicalDoubleSQLText(sqlValue interface{}) exp.Expression {
	return goqu.L(canonicalDoubleSQL, xsdDoubleDecimalMin, xsdDoubleDecimalMax, sqlValue)
}

// xsdDoubleOverflowSQL is the smallest decimal that rounds to infinity as a double. PostgreSQL
// raises an error when it converts this or a larger numeric to double precision. Numeric powers of
// two with integer exponents are exact.
const xsdDoubleOverflowSQL = "(power(2::numeric, 1024) - power(2::numeric, 970))"

var canonicalDoubleSQL = strings.Join([]string{
	"(SELECT CASE",
	"WHEN d = 'NaN'::double precision THEN 'NaN'",
	"WHEN d = 'Infinity'::double precision THEN 'INF'",
	"WHEN d = '-Infinity'::double precision THEN '-INF'",
	"WHEN d = 0 THEN CASE WHEN d::text LIKE '-%' THEN '-0' ELSE '0' END",
	"ELSE CASE WHEN d < 0 THEN '-' ELSE '' END ||",
	"CASE WHEN abs(d) >= ? AND abs(d) < ? THEN shortest_text",
	"ELSE left(shortest_parts.digits, 1) || '.' || COALESCE(NULLIF(substr(shortest_parts.digits, 2), ''), '0')",
	"|| 'E' || shortest_parts.exponent10::text END",
	"END",
	"FROM (SELECT ?::double precision AS d OFFSET 0) AS xsd_double,",
	"LATERAL (SELECT CASE WHEN d IN ('NaN'::double precision, 'Infinity'::double precision, '-Infinity'::double precision) OR d = 0",
	"THEN NULL ELSE trim_scale(abs(d)::text::numeric) END AS printed) AS xsd_double_printed,",
	decimalPartsSQL("printed::text", "printed_parts") + ",",
	"LATERAL (SELECT (SELECT trim_scale(candidate)::text",
	"FROM generate_series(1, 17) AS digit_count,",
	"LATERAL (SELECT trunc(printed, digit_count - 1 - printed_parts.exponent10) AS truncated) AS xsd_double_truncated,",
	"LATERAL (VALUES (truncated), (truncated + ('1e' || (printed_parts.exponent10 - digit_count + 1))::numeric)) AS xsd_double_candidate(candidate)",
	"WHERE candidate > 0 AND candidate < " + xsdDoubleOverflowSQL + " AND candidate::double precision = abs(d)",
	"ORDER BY digit_count, abs(candidate - printed), candidate LIMIT 1) AS shortest_text) AS xsd_double_shortest,",
	decimalPartsSQL("shortest_text", "shortest_parts") + ")",
}, " ")

// decimalPartsSQL splits the text of a positive numeric into its significant digits and the
// decimal exponent of its first significant digit.
func decimalPartsSQL(text, alias string) string {
	return "LATERAL (SELECT rtrim(ltrim(replace(" + text + ", '.', ''), '0'), '0') AS digits," +
		" CASE WHEN " + text + " LIKE '0%' THEN 1 - length(" + text + ") + length(ltrim(" + text + ", '0.'))" +
		" ELSE strpos(" + text + " || '.', '.') - 2 END AS exponent10) AS " + alias
}
