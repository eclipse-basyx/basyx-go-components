package querybuilder

import (
	"strings"
	"testing"
)

func TestSelectBuilderBasic(t *testing.T) {
	q, args := NewSelect("id", "name").From("public.submodel s").Build()
	if len(args) != 0 {
		t.Fatalf("expected 0 args, got %d", len(args))
	}
	expectedContains := []string{
		"SELECT id, name",
		"FROM public.submodel s",
	}
	for _, frag := range expectedContains {
		if !strings.Contains(q, frag) {
			t.Fatalf("query missing fragment %q; got: %s", frag, q)
		}
	}
}

func TestSelectBuilderWhereAndArgs(t *testing.T) {
	b := NewSelect("s.id", "s.identification").
		From("public.submodel s").
		Where("s.id = $1", 42).
		Where("s.identification = $2", "abc")

	q, args := b.Build()
	if want, got := 2, len(args); want != got {
		t.Fatalf("want %d args, got %d", want, got)
	}
	if args[0] != 42 || args[1] != "abc" {
		t.Fatalf("unexpected args: %#v", args)
	}
	if !strings.Contains(q, "WHERE s.id = $1 AND s.identification = $2") {
		t.Fatalf("unexpected WHERE: %s", q)
	}
}

func TestSelectBuilderJoinOrderLimit(t *testing.T) {
	b := NewSelect("s.id", "s.identification").
		From("public.submodel s").
		Join("LEFT JOIN public.reference r ON r.id = s.semantic_id").
		OrderBy("s.id ASC").
		Limit(10)

	q, _ := b.Build()
	if !strings.Contains(q, "LEFT JOIN public.reference r ON r.id = s.semantic_id") {
		t.Fatalf("JOIN missing: %s", q)
	}
	if !strings.Contains(q, "ORDER BY s.id ASC") {
		t.Fatalf("ORDER BY missing: %s", q)
	}
	if !strings.HasSuffix(strings.TrimSpace(q), "LIMIT 10") {
		t.Fatalf("LIMIT missing or misplaced: %s", q)
	}
}

func TestSelectBuilderDedupeColumns(t *testing.T) {
	b := NewSelect("s.id", "s.id", "s.identification").From("public.submodel s")
	q, _ := b.Build()
	// Parse the SELECT list and count exact token matches for s.id
	fromIdx := strings.Index(q, " FROM ")
	if fromIdx == -1 {
		t.Fatalf("unexpected query shape, missing FROM: %s", q)
	}
	selectList := strings.TrimPrefix(q[:fromIdx], "SELECT ")
	parts := strings.Split(selectList, ", ")
	count := 0
	for _, p := range parts {
		if p == "s.id" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one s.id column, got %d in: %s", count, q)
	}
}

func TestSelectBuilderWhereInAndOffset(t *testing.T) {
	b := NewSelect("id").
		From("t").
		WhereIn("id", 1, 2, 3).
		OrderBy("id").
		Limit(5).
		Offset(10)
	q, args := b.Build()
	if !strings.Contains(q, "WHERE id IN ($1, $2, $3)") {
		t.Fatalf("expected IN clause, got: %s", q)
	}
	if len(args) != 3 || args[0] != 1 || args[1] != 2 || args[2] != 3 {
		t.Fatalf("unexpected args: %#v", args)
	}
	if !strings.HasSuffix(strings.TrimSpace(q), "LIMIT 5\nOFFSET 10") {
		t.Fatalf("expected LIMIT then OFFSET at end, got: %q", q)
	}
}

func TestSelectBuilderDistinctGroupHaving(t *testing.T) {
	b := NewSelect("id", "count(*)").
		From("t").
		Distinct().
		GroupBy("id").
		Having("count(*) > $1", 10)
	q, args := b.Build()
	if !strings.HasPrefix(q, "SELECT DISTINCT ") {
		t.Fatalf("expected DISTINCT, got: %s", q)
	}
	if !strings.Contains(q, "GROUP BY id") || !strings.Contains(q, "HAVING count(*) > $1") {
		t.Fatalf("expected GROUP BY and HAVING, got: %s", q)
	}
	if len(args) != 1 || args[0] != 10 {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestSelectBuilderDistinctOn(t *testing.T) {
	b := NewSelect("id", "ts").
		From("t").
		DistinctOn("id")
	q, _ := b.Build()
	if !strings.HasPrefix(q, "SELECT DISTINCT ON (id) ") {
		t.Fatalf("expected DISTINCT ON, got: %s", q)
	}
}

func TestInsertBuilderBasic(t *testing.T) {
	b := NewInsert("t").
		Columns("id", "name").
		Values(1, "foo").
		Values(2, "bar")
	q, args := b.Build()
	if !strings.HasPrefix(q, "INSERT INTO t (id, name) VALUES ") {
		t.Fatalf("unexpected query start: %s", q)
	}
	expectedValues := []string{"($1, $2)", "($3, $4)"}
	for _, ev := range expectedValues {
		if !strings.Contains(q, ev) {
			t.Fatalf("missing values fragment %q in query: %s", ev, q)
		}
	}
	if len(args) != 4 ||
		args[0] != 1 || args[1] != "foo" ||
		args[2] != 2 || args[3] != "bar" {
		t.Fatalf("unexpected args: %#v", args)
	}
}
