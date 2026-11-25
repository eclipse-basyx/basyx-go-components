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
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/require"
)

func TestCanonicalDoubleString(t *testing.T) {
	cases := map[float64]string{
		0:                        "0",
		math.Copysign(0, -1):     "-0",
		100:                      "100",
		-1.5:                     "-1.5",
		0.000001:                 "0.000001",
		999999.5:                 "999999.5",
		1e6:                      "1.0E6",
		-1.5e20:                  "-1.5E20",
		1e-7:                     "1.0E-7",
		1e23:                     "1.0E23",
		5e-324:                   "5.0E-324",
		0.30000000000000004:      "0.30000000000000004",
		math.MaxFloat64:          "1.7976931348623157E308",
		math.Inf(1):              "INF",
		math.Inf(-1):             "-INF",
		-1.529906903274676e17:    "-1.529906903274676E17",
		1.2345678901234568e+17:   "1.2345678901234568E17",
		123456.78901234567:       "123456.78901234567",
		0.0000012345678901234567: "0.0000012345678901234567",
	}
	for value, want := range cases {
		require.Equal(t, want, canonicalDoubleString(value), "%v", value)
	}
	require.Equal(t, "NaN", canonicalDoubleString(math.NaN()))
}

func TestCastOfNumberLiteralBindsText(t *testing.T) {
	condition := LogicalExpression{Eq: []Value{
		{StrCast: &Value{NumVal: new(-1.0)}},
		{StrVal: new(StandardString("-1"))},
	}}
	expression, _, err := condition.EvaluateToExpression(nil)
	require.NoError(t, err)

	interpolated, _, err := goqu.Dialect("postgres").Select(expression).ToSQL()
	require.NoError(t, err)
	require.Contains(t, interpolated, `'-1'::text`)

	_, args, err := goqu.Dialect("postgres").Select(expression).Prepared(true).ToSQL()
	require.NoError(t, err)
	require.Equal(t, []any{"-1", "-1"}, args)
}
