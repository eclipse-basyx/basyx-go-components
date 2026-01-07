package grammar

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func TestQueryWrapper_SMECondition_ToSQL(t *testing.T) {
	jsonStr := `{
		"Query": {
			"$condition": {
				"$eq": [
					{"$field": "$sme.temperature#value"},
					{"$strVal": "100"}
				]
			}
		}
	}`

	var wrapper QueryWrapper
	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
		t.Fatalf("Failed to unmarshal SME query: %v", err)
	}
	if wrapper.Query.Condition == nil {
		t.Fatal("Expected Condition to be set")
	}

	whereExpr, err := wrapper.Query.Condition.EvaluateToExpression()
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	// Use aliases matching the resolver output.
	ds := d.From(goqu.T("submodel_element").As("submodel_element")).
		LeftJoin(goqu.T("property_element").As("property_element"), goqu.On(goqu.I("property_element.id").Eq(goqu.I("submodel_element.id")))).
		Select(goqu.V(1)).
		Where(whereExpr).
		Prepared(true)

	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	// We expect the idShortPath binding to become a plain AND constraint (no EXISTS join graph for SME).
	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS for SME query, got: %s", sql)
	}
	if !strings.Contains(sql, "\"submodel_element\".\"idshort_path\"") {
		t.Fatalf("expected idshort_path constraint in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "property_element") {
		t.Fatalf("expected SME value expression to reference property_element, got: %s", sql)
	}

	if !argListContains(args, "temperature") {
		t.Fatalf("expected args to contain %q, got %#v", "temperature", args)
	}
	if !argListContains(args, "100") {
		t.Fatalf("expected args to contain %q, got %#v", "100", args)
	}
}
