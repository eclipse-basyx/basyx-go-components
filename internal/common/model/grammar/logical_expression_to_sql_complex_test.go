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
	collector := mustCollectorForRoot(t, "$aasdesc")
	whereExpr, _, err := le.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}
	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).
		InnerJoin(
			goqu.T("aas_descriptor").As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		Select(goqu.V(1)).
		Where(whereExpr)

	ctes, err := BuildResolvedFieldPathFlagCTEsWithCollector(collector, collector.Entries(), nil)
	if err != nil {
		t.Fatalf("BuildResolvedFieldPathFlagCTEsWithCollector returned error: %v", err)
	}
	for _, cte := range ctes {
		ds = ds.With(cte.Alias, cte.Dataset).
			LeftJoin(
				goqu.T(cte.Alias),
				goqu.On(goqu.I(cte.Alias+".root_id").Eq(goqu.I("descriptor.id"))),
			)
	}
	ds = ds.Prepared(true)
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
			wantSQL: []string{"FROM \"descriptor\"", "JOIN \"aas_descriptor\"", "::text", "= ?"},
			wantArgs: []interface{}{
				"shell-short",
			},
			noExists: true,
		},
		{
			name:    "gt number adds implicit guarded ::double precision",
			expr:    LogicalExpression{Gt: ComparisonItems{field("$aasdesc#id"), Value{NumVal: floatPtr(10)}}},
			wantSQL: []string{"FROM \"descriptor\"", "JOIN \"aas_descriptor\"", "CASE WHEN", "::double precision", "> ?"},
			wantArgs: []interface{}{
				float64(10),
			},
			noExists: true,
		},
		{
			name:    "ge number with explicit $numCast is guarded",
			expr:    LogicalExpression{Ge: ComparisonItems{Value{NumCast: valuePtr(field("$aasdesc#id"))}, Value{NumVal: floatPtr(10)}}},
			wantSQL: []string{"FROM \"descriptor\"", "JOIN \"aas_descriptor\"", "CASE WHEN", "::double precision", ">= ?"},
			wantArgs: []interface{}{
				float64(10),
			},
			noExists: true,
		},
		{
			name:    "eq boolean adds implicit guarded ::boolean",
			expr:    LogicalExpression{Eq: ComparisonItems{field("$aasdesc#assetKind"), Value{Boolean: &kind}}},
			wantSQL: []string{"FROM \"descriptor\"", "JOIN \"aas_descriptor\"", "CASE WHEN", "::boolean", "= ?"},
			wantArgs: []interface{}{
				true,
			},
			noExists: true,
		},
		{
			name:    "lt time adds implicit guarded ::time",
			expr:    LogicalExpression{Lt: ComparisonItems{field("$aasdesc#idShort"), Value{TimeVal: &timeVal}}},
			wantSQL: []string{"FROM \"descriptor\"", "JOIN \"aas_descriptor\"", "CASE WHEN", "::time", "< ?"},
			wantArgs: []interface{}{
				string(timeVal),
			},
			noExists: true,
		},
		{
			name:    "eq datetime adds implicit guarded ::timestamptz",
			expr:    LogicalExpression{Eq: ComparisonItems{field("$aasdesc#id"), Value{DateTimeVal: &dtVal}}},
			wantSQL: []string{"FROM \"descriptor\"", "JOIN \"aas_descriptor\"", "CASE WHEN", "::timestamptz", "= ?"},
			wantArgs: []interface{}{
				dt,
			},
			noExists: true,
		},
		{
			name:    "contains uses LIKE and casts field to text",
			expr:    LogicalExpression{Contains: StringItems{strField("$aasdesc#assetType"), strString("blocked")}},
			wantSQL: []string{"FROM \"descriptor\"", "JOIN \"aas_descriptor\"", "LIKE", "::text"},
			wantArgs: []interface{}{
				"blocked",
			},
			noExists: true,
		},
		{
			name:    "regex uses ~ and respects explicit $strCast",
			expr:    LogicalExpression{Regex: StringItems{StringValue{StrCast: valuePtr(field("$aasdesc#assetType"))}, strString("^foo.*")}},
			wantSQL: []string{"FROM \"descriptor\"", "JOIN \"aas_descriptor\"", "~", "::text"},
			wantArgs: []interface{}{
				"^foo.*",
			},
			noExists: true,
		},
		{
			name: "or mixes value-to-value with flag CTE",
			expr: LogicalExpression{Or: []LogicalExpression{
				{Eq: ComparisonItems{strVal("same"), strVal("same")}},
				{Eq: ComparisonItems{field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value"), strVal("WRITTEN_BY_X")}},
			}},
			wantSQL: []string{" OR ", "descriptor_flags_1"},
			wantArgs: []interface{}{
				"same",
				"WRITTEN_BY_X",
				0,
				1,
			},
			noExists: true,
		},
		{
			name: "nested JSON with indexed submodelDescriptors uses flag CTE",
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
			wantSQL: []string{"descriptor_flags_1"},
			wantArgs: []interface{}{
				2,
				0,
				"GlobalReference",
				"urn:example",
			},
			noExists: true,
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
