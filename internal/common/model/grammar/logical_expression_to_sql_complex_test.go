package grammar

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
)

func toPreparedSQLForDescriptor(t *testing.T, le LogicalExpression) (string, []interface{}) {
	t.Helper()
	whereExpr, err := le.EvaluateToExpression()
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}
	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).Select(goqu.V(1)).Where(whereExpr).Prepared(true)
	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	return sql, args
}

func TestLogicalExpression_ToSQL_ComplexCases(t *testing.T) {
	kind := true
	timeVal := TimeLiteralPattern("12:34")
	dt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	dtVal := DateTimeLiteralPattern(dt)

	tests := []struct {
		name      string
		expr      LogicalExpression
		jsonInput string
		wantSQL   []string
		wantArgs  []interface{}
		noExists  bool
	}{
		{
			name:    "eq string adds implicit ::text",
			expr:    LogicalExpression{Eq: ComparisonItems{field("$aasdesc#idShort"), strVal("shell-short")}},
			wantSQL: []string{"::text", "= ?"},
			wantArgs: []interface{}{
				"shell-short",
			},
			noExists: true,
		},
		{
			name:    "gt number adds implicit ::double precision",
			expr:    LogicalExpression{Gt: ComparisonItems{field("$aasdesc#id"), Value{NumVal: floatPtr(10)}}},
			wantSQL: []string{"::double precision", "> ?"},
			wantArgs: []interface{}{
				float64(10),
			},
			noExists: true,
		},
		{
			name:    "ge number with explicit $numCast",
			expr:    LogicalExpression{Ge: ComparisonItems{Value{NumCast: valuePtr(field("$aasdesc#id"))}, Value{NumVal: floatPtr(10)}}},
			wantSQL: []string{"::double precision", ">= ?"},
			wantArgs: []interface{}{
				float64(10),
			},
			noExists: true,
		},
		{
			name:    "eq boolean adds implicit ::boolean",
			expr:    LogicalExpression{Eq: ComparisonItems{field("$aasdesc#assetKind"), Value{Boolean: &kind}}},
			wantSQL: []string{"::boolean", "= ?"},
			wantArgs: []interface{}{
				true,
			},
			noExists: true,
		},
		{
			name:    "lt time adds implicit ::time",
			expr:    LogicalExpression{Lt: ComparisonItems{field("$aasdesc#idShort"), Value{TimeVal: &timeVal}}},
			wantSQL: []string{"::time", "< ?"},
			wantArgs: []interface{}{
				string(timeVal),
			},
			noExists: true,
		},
		{
			name:    "eq datetime adds implicit ::timestamptz",
			expr:    LogicalExpression{Eq: ComparisonItems{field("$aasdesc#id"), Value{DateTimeVal: &dtVal}}},
			wantSQL: []string{"::timestamptz", "= ?"},
			wantArgs: []interface{}{
				dt,
			},
			noExists: true,
		},
		{
			name:    "contains uses LIKE and casts field to text",
			expr:    LogicalExpression{Contains: StringItems{strField("$aasdesc#assetType"), strString("blocked")}},
			wantSQL: []string{"LIKE", "::text"},
			wantArgs: []interface{}{
				"blocked",
			},
			noExists: true,
		},
		{
			name:    "regex uses ~ and respects explicit $strCast",
			expr:    LogicalExpression{Regex: StringItems{StringValue{StrCast: valuePtr(field("$aasdesc#assetType"))}, strString("^foo.*")}},
			wantSQL: []string{"~", "::text"},
			wantArgs: []interface{}{
				"^foo.*",
			},
			noExists: true,
		},
		{
			name: "or mixes value-to-value with EXISTS",
			expr: LogicalExpression{Or: []LogicalExpression{
				{Eq: ComparisonItems{strVal("same"), strVal("same")}},
				{Eq: ComparisonItems{field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value"), strVal("WRITTEN_BY_X")}},
			}},
			wantSQL: []string{" OR ", "EXISTS"},
			wantArgs: []interface{}{
				"same",
				"WRITTEN_BY_X",
				0,
				1,
			},
		},
		{
			name: "nested JSON with indexed submodelDescriptors uses EXISTS and binds positions",
			jsonInput: `{
				"$and": [
					{"$ne": [
						{"$field": "$aasdesc#submodelDescriptors[2].semanticId.keys[0].type"},
						{"$strVal": "GlobalReference"}
					]},
					{"$eq": [
						{"$field": "$aasdesc#submodelDescriptors[2].semanticId.keys[0].value"},
						{"$strVal": "urn:example"}
					]}
				]
			}`,
			wantSQL: []string{"EXISTS", "FROM \"submodel_descriptor\""},
			wantArgs: []interface{}{
				2,
				0,
				"GlobalReference",
				"urn:example",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			le := tt.expr
			if tt.jsonInput != "" {
				if err := json.Unmarshal([]byte(tt.jsonInput), &le); err != nil {
					t.Fatalf("failed to unmarshal LogicalExpression: %v", err)
				}
			}

			sql, args := toPreparedSQLForDescriptor(t, le)
			fmt.Println(sql, args)

			if tt.noExists && strings.Contains(sql, "EXISTS") {
				t.Fatalf("did not expect EXISTS, got: %s", sql)
			}
			for _, want := range tt.wantSQL {
				if !strings.Contains(sql, want) {
					t.Fatalf("expected SQL to contain %q, got: %s", want, sql)
				}
			}
			for _, wantArg := range tt.wantArgs {
				if !argListContains(args, wantArg) {
					t.Fatalf("expected args to contain %#v, got %#v", wantArg, args)
				}
			}
		})
	}
}

func floatPtr(v float64) *float64 { return &v }

func valuePtr(v Value) *Value { return &v }
