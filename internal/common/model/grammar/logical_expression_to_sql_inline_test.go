package grammar

import (
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func TestResolvedFieldPathCollectorCanEvaluateAllowedWildcardPathInline(t *testing.T) {
	field := ModelStringPattern("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value")
	resolved, err := ResolveScalarFieldToSQL(&field)
	if err != nil {
		t.Fatalf("ResolveScalarFieldToSQL returned error: %v", err)
	}

	collector, err := NewResolvedFieldPathCollectorForRoot(CollectorRootAASDesc)
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForRoot returned error: %v", err)
	}
	collector.AllowInlineAliases(
		"descriptor",
		"aas_descriptor",
		"specific_asset_id",
		"external_subject_reference",
		"external_subject_reference_key",
	)

	if !collector.canEvaluateInline([]ResolvedFieldPath{resolved}) {
		t.Fatalf("expected resolved path to be inline-compatible: %#v", resolved)
	}
}

func TestSMECollectorCorrelatesExistsToSubmodelElementAlias(t *testing.T) {
	field := ModelStringPattern("$sm#supplementalSemanticIds[].keys[].value")
	value := StandardString("PUBLIC_READABLE")
	expression := LogicalExpression{
		Eq: ComparisonItems{
			{Field: &field},
			{StrVal: &value},
		},
	}
	collector, err := NewResolvedFieldPathCollectorForRoot(CollectorRootSME)
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForRoot returned error: %v", err)
	}
	collector.SetRootJoinKey("submodel_element", "submodel_id")

	where, _, err := expression.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}
	sql, _, err := goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("submodel_element")).
		Select(goqu.V(1)).
		Where(where).
		ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	if strings.Contains(sql, `"sme".`) {
		t.Fatalf("SME correlation must not reference an unjoined sme alias: %s", sql)
	}
	if !strings.Contains(sql, `"submodel_element"."submodel_id"`) {
		t.Fatalf("expected submodel_element correlation in SQL: %s", sql)
	}
}
