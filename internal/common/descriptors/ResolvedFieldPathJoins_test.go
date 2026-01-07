package descriptors

import (
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestGetJoinTablesForResolvedFieldPath_BaseOnly(t *testing.T) {
	d := goqu.Dialect("postgres")
	resolved := grammar.ResolvedFieldPath{Column: "aas_descriptor.id_short"}

	ds, err := GetJoinTablesForResolvedFieldPath(d, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sql, _, err := ds.Select(goqu.Star()).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL error: %v", err)
	}
	if !strings.Contains(sql, "JOIN \"aas_descriptor\"") {
		t.Fatalf("expected join to aas_descriptor, got SQL: %s", sql)
	}
	if strings.Contains(sql, "specific_asset_id") {
		t.Fatalf("did not expect specific_asset_id join, got SQL: %s", sql)
	}
}

func TestGetJoinTablesForResolvedFieldPath_ExternalSubjectKeyChain(t *testing.T) {
	d := goqu.Dialect("postgres")
	resolved := grammar.ResolvedFieldPath{
		Column: "external_subject_reference_key.value",
		ArrayBindings: []grammar.ArrayIndexBinding{
			{Alias: "specific_asset_id.position", Index: grammar.NewArrayIndexPosition(2)},
			{Alias: "external_subject_reference_key.position", Index: grammar.NewArrayIndexPosition(5)},
		},
	}

	ds, err := GetJoinTablesForResolvedFieldPath(d, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sql, _, err := ds.Select(goqu.Star()).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL error: %v", err)
	}

	want := []string{
		"JOIN \"specific_asset_id\" AS \"specific_asset_id\"",
		"JOIN \"reference\" AS \"external_subject_reference\"",
		"JOIN \"reference_key\" AS \"external_subject_reference_key\"",
	}
	for _, w := range want {
		if !strings.Contains(sql, w) {
			t.Fatalf("expected SQL to contain %q, got: %s", w, sql)
		}
	}
}

func TestGetJoinTablesForResolvedFieldPath_SubmodelDescriptorEndpoint(t *testing.T) {
	d := goqu.Dialect("postgres")
	resolved := grammar.ResolvedFieldPath{Column: "submodel_descriptor_endpoint.href"}

	ds, err := GetJoinTablesForResolvedFieldPath(d, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sql, _, err := ds.Select(goqu.Star()).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL error: %v", err)
	}

	if !strings.Contains(sql, "JOIN \"submodel_descriptor\" AS \"submodel_descriptor\"") {
		t.Fatalf("expected join to submodel_descriptor, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "JOIN \"aas_descriptor_endpoint\" AS \"submodel_descriptor_endpoint\"") {
		t.Fatalf("expected join to submodel_descriptor_endpoint alias, got SQL: %s", sql)
	}
}
