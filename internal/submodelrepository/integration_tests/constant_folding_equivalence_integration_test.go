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

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/stretchr/testify/require"
)

// foldingCase is a condition without fields. Its XPath result is want; nil means the
// condition has no value, so the candidate is excluded.
type foldingCase struct {
	name, condition string
	want            *bool
}

var (
	xpathTrue  = new(true)
	xpathFalse = new(false)
)

// TestConstantFoldingMatchesPostgreSQL checks that Go constant folding and the SQL PostgreSQL
// evaluates for the unfolded condition both yield the XPath result.
func TestConstantFoldingMatchesPostgreSQL(t *testing.T) {
	cases := []foldingCase{
		{"string of a large number", `{"$eq":[{"$strCast":{"$numVal":1000000}},{"$strVal":"1.0E6"}]}`, xpathTrue},
		{"string of a cast large number", `{"$eq":[{"$strCast":{"$numCast":{"$strVal":"1000000"}}},{"$strVal":"1.0E6"}]}`, xpathTrue},
		{"string of a small number", `{"$eq":[{"$strCast":{"$numCast":{"$strVal":"0.0000001"}}},{"$strVal":"1.0E-7"}]}`, xpathTrue},
		{"string of a decimal", `{"$eq":[{"$strCast":{"$numCast":{"$strVal":"123456.50"}}},{"$strVal":"123456.5"}]}`, xpathTrue},
		{"string of an integral number", `{"$eq":[{"$strCast":{"$numVal":100}},{"$strVal":"100"}]}`, xpathTrue},
		{"string of the smallest decimal", `{"$eq":[{"$strCast":{"$numVal":0.000001}},{"$strVal":"0.000001"}]}`, xpathTrue},
		{"string of a negative exponent", `{"$eq":[{"$strCast":{"$numVal":-1.5e20}},{"$strVal":"-1.5E20"}]}`, xpathTrue},
		{"string of the shortest round-trip digits", `{"$eq":[{"$strCast":{"$numCast":{"$strVal":"0.30000000000000004"}}},{"$strVal":"0.30000000000000004"}]}`, xpathTrue},
		{"string of the smallest subnormal", `{"$eq":[{"$strCast":{"$numVal":5e-324}},{"$strVal":"4.9E-324"}]}`, xpathFalse},
		{"string of negative zero", `{"$eq":[{"$strCast":{"$numCast":{"$strVal":"-0"}}},{"$strVal":"-0"}]}`, xpathTrue},
		{"string of a year", `{"$eq":[{"$strCast":{"$year":{"$dateTimeVal":"2024-01-01T00:00:00Z"}}},{"$strVal":"2024"}]}`, xpathTrue},
		{"string of a negative literal", `{"$eq":[{"$strCast":{"$numVal":-1}},{"$strVal":"-1"}]}`, xpathTrue},
		{"string of a Boolean literal", `{"$eq":[{"$strCast":{"$boolean":false}},{"$strVal":"false"}]}`, xpathTrue},
		{"hex of a Boolean literal", `{"$eq":[{"$hexCast":{"$boolean":true}},{"$hexVal":"16#1"}]}`, nil},
		{"hex of a number literal", `{"$eq":[{"$hexCast":{"$numVal":-1}},{"$hexVal":"16#1"}]}`, nil},
	}
	db := openConstantFoldingDatabase(t)
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			goResult, sqlResults := evaluateConstantCondition(t, db, testCase.condition)
			if goResult.decided {
				require.Equal(t, testCase.want, goResult.value, "Go folding")
			}
			for mode, sqlResult := range sqlResults {
				require.Equal(t, testCase.want, sqlResult, "PostgreSQL, %s", mode)
			}
		})
	}
}

// TestTimestampFieldsCompareAsText checks that a TIMESTAMPTZ field, read as its xs:dateTime
// text in UTC, works with string operands and operators regardless of the session TimeZone.
func TestTimestampFieldsCompareAsText(t *testing.T) {
	cases := []foldingCase{
		{"equal text", `{"$eq":[{"$field":"$aasdesc#createdAt"},{"$strVal":"2024-01-01T08:00:00+00:00"}]}`, xpathTrue},
		{"a date is other text", `{"$eq":[{"$field":"$aasdesc#createdAt"},{"$strVal":"2024-01-01"}]}`, xpathFalse},
		{"local text is other text", `{"$eq":[{"$field":"$aasdesc#createdAt"},{"$strVal":"2024-01-01 22:00:00"}]}`, xpathFalse},
		{"text that is no timestamp", `{"$ne":[{"$field":"$aasdesc#createdAt"},{"$strVal":"yesterday"}]}`, xpathTrue},
		{"string order", `{"$gt":[{"$field":"$aasdesc#createdAt"},{"$strVal":"2024-01-01T07"}]}`, xpathTrue},
		{"contains", `{"$contains":[{"$field":"$aasdesc#createdAt"},{"$strVal":"T08:00"}]}`, xpathTrue},
		{"starts with", `{"$starts-with":[{"$field":"$aasdesc#createdAt"},{"$strVal":"2024-01-01T08"}]}`, xpathTrue},
		{"regex", `{"$regex":[{"$field":"$aasdesc#createdAt"},{"$strVal":"^2024-01-01T08:00:00"}]}`, xpathTrue},
		{"string cast", `{"$eq":[{"$strCast":{"$field":"$aasdesc#createdAt"}},{"$strVal":"2024-01-01T08:00:00+00:00"}]}`, xpathTrue},
		{"date-time cast keeps the instant", `{"$eq":[{"$field":"$aasdesc#createdAt"},{"$dateTimeVal":"2024-01-01T10:00:00+02:00"}]}`, xpathTrue},
		{"year in UTC", `{"$eq":[{"$year":{"$dateTimeCast":{"$field":"$aasdesc#createdAt"}}},{"$numVal":2024}]}`, xpathTrue},
	}
	db := openConstantFoldingDatabase(t)
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.ExecContext(t.Context(), "SET TIME ZONE 'Pacific/Kiritimati'")
	require.NoError(t, err)
	defer func() { _, _ = conn.ExecContext(t.Context(), "RESET TIME ZONE") }()

	row := goqu.L("(VALUES (?::timestamptz)) AS aas_descriptor(created_at)", "2024-01-01T08:00:00Z")
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var expression grammar.LogicalExpression
			require.NoError(t, json.Unmarshal([]byte(testCase.condition), &expression))
			sqlExpression, _, err := expression.EvaluateToExpression(nil)
			require.NoError(t, err)
			for _, prepared := range []bool{false, true} {
				query, args, err := goqu.Dialect(common.Dialect).From(row).Select(goqu.L("(?)", sqlExpression)).Prepared(prepared).ToSQL()
				require.NoError(t, err)
				var value sql.NullBool
				require.NoError(t, conn.QueryRowContext(t.Context(), query, args...).Scan(&value), query)
				require.Equal(t, testCase.want, nullableBool(value), query)
			}
		})
	}
}

// TestRandomNumberStringsMatchPostgreSQL compares Go folding and PostgreSQL for $strCast of
// random numbers, literal and cast from text.
func TestRandomNumberStringsMatchPostgreSQL(t *testing.T) {
	db := openConstantFoldingDatabase(t)
	random := rand.New(rand.NewPCG(917, 2026)) //nolint:gosec // Reproducible test inputs, not secrets.
	numbers := []float64{1e23, -1.529906903274676e17, 9.720726937680768e16, 5e-324, 2.2250738585072014e-308, math.MaxFloat64, 999999.9999999999, 1e6, 1e-6, 9.999999999999999e-7}
	for range 400 {
		numbers = append(numbers, randomFiniteDouble(random))
	}
	for _, number := range numbers {
		literal, err := json.Marshal(number)
		require.NoError(t, err)
		text := strconv.FormatFloat(number, 'g', -1, 64)
		for _, operand := range []string{
			fmt.Sprintf(`{"$numVal":%s}`, literal),
			fmt.Sprintf(`{"$numCast":{"$strVal":%q}}`, text),
		} {
			condition := fmt.Sprintf(`{"$strCast":%s}`, operand)
			requireConstantStringsAgree(t, db, condition)
		}
	}
}

func randomFiniteDouble(random *rand.Rand) float64 {
	for {
		var number float64
		switch random.IntN(3) {
		case 0:
			number = math.Float64frombits(random.Uint64())
		case 1:
			number = float64(random.IntN(2_000_000)-1_000_000) * math.Pow10(random.IntN(13)-9)
		default:
			number = (random.Float64() - 0.5) * math.Pow10(random.IntN(40)-20)
		}
		if !math.IsNaN(number) && !math.IsInf(number, 0) {
			return number
		}
	}
}

// requireConstantStringsAgree checks that a string-valued operand folds in Go to the text that
// PostgreSQL computes for it.
func requireConstantStringsAgree(t *testing.T, db *sql.DB, operand string) {
	t.Helper()
	var value grammar.Value
	require.NoError(t, json.Unmarshal([]byte(operand), &value), operand)
	var sqlText sql.NullString
	query, args := constantSelectSQL(t, goqu.L("(?)", stringOperandSQL(t, value)), true)
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&sqlText), operand)
	require.True(t, sqlText.Valid, operand)
	condition := fmt.Sprintf(`{"$eq":[%s,{"$strVal":%q}]}`, operand, sqlText.String)
	goResult, _ := evaluateConstantCondition(t, db, condition)
	require.True(t, goResult.decided, condition)
	require.Equal(t, xpathTrue, goResult.value, condition)
}

func stringOperandSQL(t *testing.T, value grammar.Value) exp.Expression {
	t.Helper()
	condition := grammar.LogicalExpression{Eq: []grammar.Value{value, {StrVal: new(grammar.StandardString(""))}}}
	expression, _, err := condition.EvaluateToExpression(nil)
	require.NoError(t, err)
	literal, ok := expression.(exp.LiteralExpression)
	require.True(t, ok)
	return literal.Args()[0].(exp.Expression)
}

type foldedResult struct {
	decided bool
	value   *bool
}

// evaluateConstantCondition folds a condition in Go and evaluates the unfolded condition in
// PostgreSQL with interpolated and with bound arguments.
func evaluateConstantCondition(t *testing.T, db *sql.DB, condition string) (foldedResult, map[string]*bool) {
	t.Helper()
	var expression grammar.LogicalExpression
	require.NoError(t, json.Unmarshal([]byte(condition), &expression), condition)
	sqlExpression, _, err := expression.EvaluateToExpression(nil)
	require.NoError(t, err, condition)

	results := map[string]*bool{}
	for mode, prepared := range map[string]bool{"interpolated": false, "prepared": true} {
		query, args := constantSelectSQL(t, sqlExpression, prepared)
		var value sql.NullBool
		require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&value), "%s: %s", mode, query)
		results[mode] = nullableBool(value)
	}
	_, decision := expression.SimplifyForBackendFilterNoResolver()
	return foldedResultOf(decision), results
}

func constantSelectSQL(t *testing.T, expression exp.Expression, prepared bool) (string, []any) {
	t.Helper()
	query, args, err := goqu.Dialect(common.Dialect).Select(goqu.L("(?)", expression)).Prepared(prepared).ToSQL()
	require.NoError(t, err)
	return query, args
}

func foldedResultOf(decision grammar.SimplifyDecision) foldedResult {
	switch decision {
	case grammar.SimplifyTrue:
		return foldedResult{decided: true, value: xpathTrue}
	case grammar.SimplifyFalse:
		return foldedResult{decided: true, value: xpathFalse}
	case grammar.SimplifyIndeterminate:
		return foldedResult{decided: true}
	default:
		return foldedResult{}
	}
}

func nullableBool(value sql.NullBool) *bool {
	if !value.Valid {
		return nil
	}
	return new(value.Bool)
}

func openConstantFoldingDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
