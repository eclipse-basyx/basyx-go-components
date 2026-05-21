package descriptors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func contextWithABACDisabled(t *testing.T) context.Context {
	t.Helper()

	cfg := &common.Config{}
	var cfgCtx context.Context
	handler := common.ConfigMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cfgCtx = r.Context()
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if cfgCtx == nil {
		t.Fatal("failed to create config-bearing context")
	}
	return cfgCtx
}

func TestBuildListAssetAdministrationShellDescriptorsQuery_UsesPagedInnerQueryAndPayloadFlags(t *testing.T) {
	ctx := contextWithABACDisabled(t)
	ds, err := buildListAssetAdministrationShellDescriptorsQuery(
		ctx,
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

	ctx := auth.MergeQueryFilter(contextWithABACDisabled(t), grammar.Query{
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

func TestBuildListAssetAdministrationShellDescriptorsQuery_PayloadJoinSkippedWhenPayloadFragmentsAlwaysFalse(t *testing.T) {
	falseVal := false
	falseExpr := grammar.LogicalExpression{Boolean: &falseVal}

	fAdmin := grammar.FragmentStringPattern("$aasdesc#administration")
	fDisplayName := grammar.FragmentStringPattern("$aasdesc#displayName")
	fDescription := grammar.FragmentStringPattern("$aasdesc#description")

	ctx := auth.MergeQueryFilter(contextWithABACDisabled(t), grammar.Query{
		FilterConditions: []grammar.SubFilter{
			{Fragment: &fAdmin, Condition: &falseExpr},
			{Fragment: &fDisplayName, Condition: &falseExpr},
			{Fragment: &fDescription, Condition: &falseExpr},
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

	if strings.Contains(sql, `LEFT JOIN "descriptor_payload"`) {
		t.Fatalf("did not expect descriptor_payload join when all payload fragments are always false: %s", sql)
	}
	for _, want := range []string{
		`NULL AS "raw_admin_payload"`,
		`NULL AS "raw_displayname_payload"`,
		`NULL AS "raw_description_payload"`,
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected SQL to contain %q, got: %s", want, sql)
		}
	}
}

func TestBuildListAssetAdministrationShellDescriptorsQuery_UsesNullProjectionForAlwaysFalsePayloadFragment(t *testing.T) {
	falseVal := false
	falseExpr := grammar.LogicalExpression{Boolean: &falseVal}

	fAdmin := grammar.FragmentStringPattern("$aasdesc#administration")

	ctx := auth.MergeQueryFilter(contextWithABACDisabled(t), grammar.Query{
		FilterConditions: []grammar.SubFilter{
			{Fragment: &fAdmin, Condition: &falseExpr},
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

	if !strings.Contains(sql, `LEFT JOIN "descriptor_payload"`) {
		t.Fatalf("expected descriptor_payload join when at least one payload fragment is not always false: %s", sql)
	}
	if !strings.Contains(sql, `NULL AS "raw_admin_payload"`) {
		t.Fatalf("expected admin payload projection to be NULL when fragment is always false: %s", sql)
	}
	if !strings.Contains(sql, `"descriptor_payload"."displayname_payload"`) {
		t.Fatalf("expected non-false payload columns to keep descriptor_payload access: %s", sql)
	}
}
