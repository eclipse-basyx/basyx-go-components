package descriptors

import (
	"context"
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func TestBuildListAssetAdministrationShellDescriptorsQuery_UsesPagedInnerQueryAndPayloadFlags(t *testing.T) {
	ds, err := buildListAssetAdministrationShellDescriptorsQuery(
		context.Background(),
		2,
		"",
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("buildListAssetAdministrationShellDescriptorsQuery returned error: %v", err)
	}

	sql, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	for _, want := range []string{
		`FROM (SELECT`,
		`AS "aas_page"`,
		`LIMIT $`,
		`AS "flag_`,
		`"aas_list_data"."flag_`,
		`"aas_list_data"."raw_admin_payload"`,
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected SQL to contain %q, got: %s", want, sql)
		}
	}
	hasLimitArg := false
	for _, arg := range args {
		if v, ok := arg.(int64); ok && v == 2 {
			hasLimitArg = true
			break
		}
		if v, ok := arg.(int); ok && v == 2 {
			hasLimitArg = true
			break
		}
	}
	if !hasLimitArg {
		t.Fatalf("expected prepared args to contain limit 2, got: %#v", args)
	}
}

func TestBuildListAssetAdministrationShellDescriptorsQuery_ReusesSameMaskConditionAcrossFragments(t *testing.T) {
	field := grammar.ModelStringPattern("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value")
	lit := grammar.StandardString("PUBLIC_READABLE")
	cond := grammar.LogicalExpression{
		Eq: []grammar.Value{
			{Field: &field},
			{StrVal: &lit},
		},
	}

	fAssetKind := grammar.FragmentStringPattern("$aasdesc#assetKind")
	fAssetType := grammar.FragmentStringPattern("$aasdesc#assetType")
	fDescription := grammar.FragmentStringPattern("$aasdesc#description")

	ctx := auth.MergeQueryFilter(context.Background(), grammar.Query{
		FilterConditions: []grammar.SubFilter{
			{Fragment: &fAssetKind, Condition: &cond},
			{Fragment: &fAssetType, Condition: &cond},
			{Fragment: &fDescription, Condition: &cond},
		},
	})

	ds, err := buildListAssetAdministrationShellDescriptorsQuery(ctx, 2, "", "", "", "")
	if err != nil {
		t.Fatalf("buildListAssetAdministrationShellDescriptorsQuery returned error: %v", err)
	}
	sql, _, err := ds.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if got := strings.Count(sql, "EXISTS ("); got != 1 {
		t.Fatalf("expected exactly 1 EXISTS for shared fragment condition, got %d: %s", got, sql)
	}
}
