package querybuilder

import (
	"fmt"
	"strings"
)

// Package querybuilder provides a tiny, ORM-less SQL builder tailored to the
// BaSyx submodel repository schema. It focuses on explicit, predictable SQL
// generation with parameter placeholders ($1, $2, ...) and accumulated args.
//
// Goals:
// - Readable builder API (Select/From/Join/Where/OrderBy/Limit)
// - Deterministic SQL output (stable ordering of clauses)
// - Safe argument handling via placeholders
// - No runtime reflection, no magic, easy to unit test

// SelectBuilder builds SELECT statements with a fluent API.
// It is intentionally minimal and explicit.
type SelectBuilder struct {
	columns    []string
	table      string
	joins      []string
	wheres     []string
	havings    []string
	orderBy    []string
	limit      *int
	offset     *int
	distinct   bool
	distinctOn []string
	groupBy    []string
	args       []interface{}
}

// NewSelect creates a new SelectBuilder.
func NewSelect(columns ...string) *SelectBuilder {
	return &SelectBuilder{columns: dedupe(columns)}
}

// From sets the base table (optionally with alias) for the query.
func (b *SelectBuilder) From(table string) *SelectBuilder {
	b.table = table
	return b
}

// Join appends a JOIN clause; pass a full join expression (e.g., "LEFT JOIN x ON ...").
func (b *SelectBuilder) Join(joinExpr string) *SelectBuilder {
	b.joins = append(b.joins, joinExpr)
	return b
}

// Where adds a WHERE predicate with placeholders and values.
// Use $ placeholders, values will be appended to the args in call order.
func (b *SelectBuilder) Where(predicate string, values ...interface{}) *SelectBuilder {
	b.wheres = append(b.wheres, predicate)
	b.args = append(b.args, values...)
	return b
}

// WhereIn adds a WHERE col IN ($n, $n+1, ...) predicate and appends values in order.
// If values is empty, it will add a predicate that is always false (1=0).
func (b *SelectBuilder) WhereIn(column string, values ...interface{}) *SelectBuilder {
	if len(values) == 0 {
		b.wheres = append(b.wheres, "1=0")
		return b
	}
	// Generate placeholder numbers starting from current arg count + 1
	start := len(b.args) + 1
	ph := make([]string, len(values))
	for i := range values {
		ph[i] = fmt.Sprintf("$%d", start+i)
	}
	b.wheres = append(b.wheres, fmt.Sprintf("%s IN (%s)", column, strings.Join(ph, ", ")))
	b.args = append(b.args, values...)
	return b
}

// OrderBy adds an ORDER BY expression.
func (b *SelectBuilder) OrderBy(expr string) *SelectBuilder {
	b.orderBy = append(b.orderBy, expr)
	return b
}

// Limit sets a LIMIT.
func (b *SelectBuilder) Limit(n int) *SelectBuilder {
	b.limit = &n
	return b
}

// Offset sets an OFFSET.
func (b *SelectBuilder) Offset(n int) *SelectBuilder {
	b.offset = &n
	return b
}

// Distinct adds DISTINCT to SELECT.
func (b *SelectBuilder) Distinct() *SelectBuilder {
	b.distinct = true
	return b
}

// DistinctOn adds DISTINCT ON (columns...) to SELECT (PostgreSQL-specific).
func (b *SelectBuilder) DistinctOn(columns ...string) *SelectBuilder {
	b.distinctOn = dedupe(columns)
	return b
}

// GroupBy adds GROUP BY columns.
func (b *SelectBuilder) GroupBy(columns ...string) *SelectBuilder {
	b.groupBy = append(b.groupBy, dedupe(columns)...)
	return b
}

// Having adds a HAVING predicate with args appended.
func (b *SelectBuilder) Having(predicate string, values ...interface{}) *SelectBuilder {
	b.havings = append(b.havings, predicate)
	b.args = append(b.args, values...)
	return b
}

// Args returns the accumulated argument values in placeholder order.
func (b *SelectBuilder) Args() []interface{} { return b.args }

// Build assembles the final SQL string with placeholders as provided.
func (b *SelectBuilder) Build() (string, []interface{}) {
	if b.table == "" {
		panic("querybuilder: From(table) must be specified before Build()")
	}

	var sb strings.Builder
	sb.Grow(2048)

	// SELECT
	sb.WriteString("SELECT ")
	if len(b.distinctOn) > 0 {
		sb.WriteString("DISTINCT ON (")
		sb.WriteString(strings.Join(b.distinctOn, ", "))
		sb.WriteString(") ")
	} else if b.distinct {
		sb.WriteString("DISTINCT ")
	}
	if len(b.columns) == 0 {
		sb.WriteString("*")
	} else {
		sb.WriteString(strings.Join(b.columns, ", "))
	}

	// FROM
	sb.WriteString(" FROM ")
	sb.WriteString(b.table)

	// JOINS
	for _, j := range b.joins {
		sb.WriteString("\n")
		sb.WriteString(j)
	}

	// WHERE
	if len(b.wheres) > 0 {
		sb.WriteString("\nWHERE ")
		sb.WriteString(strings.Join(b.wheres, " AND "))
	}

	// GROUP BY
	if len(b.groupBy) > 0 {
		sb.WriteString("\nGROUP BY ")
		sb.WriteString(strings.Join(b.groupBy, ", "))
	}

	// HAVING
	if len(b.havings) > 0 {
		sb.WriteString("\nHAVING ")
		sb.WriteString(strings.Join(b.havings, " AND "))
	}

	// ORDER BY
	if len(b.orderBy) > 0 {
		sb.WriteString("\nORDER BY ")
		sb.WriteString(strings.Join(b.orderBy, ", "))
	}

	// LIMIT
	if b.limit != nil {
		sb.WriteString(fmt.Sprintf("\nLIMIT %d", *b.limit))
	}

	// OFFSET
	if b.offset != nil {
		sb.WriteString(fmt.Sprintf("\nOFFSET %d", *b.offset))
	}

	return sb.String(), append([]interface{}(nil), b.args...)
}

// Helper: dedupe removes duplicates while preserving original order.
func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

type InsertBuilder struct {
	table     string
	cols      []string
	args      []interface{}
	returning string // Optional RETURNING clause
}

// NewInsert creates a new InsertBuilder for the given table.
func NewInsert(table string) *InsertBuilder {
	return &InsertBuilder{table: table}
}

// Columns sets the columns to insert into.
func (b *InsertBuilder) Columns(cols ...string) *InsertBuilder {
	b.cols = dedupe(cols)
	return b
}

// Values appends a set of values for the insert; must match columns count.
func (b *InsertBuilder) Values(vals ...interface{}) *InsertBuilder {
	if len(vals) != len(b.cols) {
		panic("querybuilder: Values count must match Columns count")
	}
	b.args = append(b.args, vals...)
	return b
}

// Returning sets a RETURNING clause (PostgreSQL-specific).
func (b *InsertBuilder) Returning(expr string) *InsertBuilder {
	b.returning = expr
	return b
}

// Args returns the accumulated argument values in placeholder order.
func (b *InsertBuilder) Args() []interface{} { return b.args }

// Build assembles the final SQL INSERT statement with placeholders.
func (b *InsertBuilder) Build() (string, []interface{}) {
	if b.table == "" {
		panic("querybuilder: table must be specified before Build()")
	}
	if len(b.cols) == 0 {
		panic("querybuilder: at least one column must be specified before Build()")
	}
	if len(b.args) == 0 || len(b.args)%len(b.cols) != 0 {
		panic("querybuilder: values count must be a multiple of columns count before Build()")
	}

	var sb strings.Builder
	sb.Grow(512)

	sb.WriteString("INSERT INTO ")
	sb.WriteString(b.table)
	sb.WriteString(" (")
	sb.WriteString(strings.Join(b.cols, ", "))
	sb.WriteString(") VALUES ")

	numRows := len(b.args) / len(b.cols)
	phIdx := 1
	for r := 0; r < numRows; r++ {
		if r > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("(")
		for c := range b.cols {
			if c > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("$%d", phIdx))
			phIdx++
		}
		sb.WriteString(")")
	}

	if b.returning != "" {
		sb.WriteString(fmt.Sprintf(" RETURNING %s", b.returning))
	}

	return sb.String(), append([]interface{}(nil), b.args...)
}
