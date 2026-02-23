package descriptors

import (
	"context"
	"strings"
	"testing"
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
		`AS "flag_admin"`,
		`AS "flag_displayname"`,
		`AS "flag_description"`,
		`"aas_list_data"."flag_admin"`,
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
