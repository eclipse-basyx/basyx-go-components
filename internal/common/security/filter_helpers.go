/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/
// Author: Martin Stemmer ( Fraunhofer IESE )

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// AddFilterQueryFromContext appends the WHERE clause for one fragment filter
// stored in ctx.
//
// The ds parameter is the SELECT dataset to constrain. The fragment parameter
// identifies the fragment whose filter expression should be applied. The
// collector parameter resolves grammar field paths to the SQL aliases used by
// ds. If ctx contains no QueryFilter, or the QueryFilter has no matching
// fragment entry, the original dataset is returned unchanged.
func AddFilterQueryFromContext(
	ctx context.Context,
	ds *goqu.SelectDataset,
	fragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
) (*goqu.SelectDataset, error) {
	maskCondition, hasMask, err := buildFragmentMaskConditionWithOptions(ctx, fragment, collector, true)
	return addFilterCondition(ds, maskCondition, hasMask, err)
}

// AddCorrelatedFilterQueryFromContext appends a fragment filter while keeping
// the collector active for MATCH expressions. Readers use this when a filter
// combines row-local fields with route-level fields that require correlated
// EXISTS queries.
func AddCorrelatedFilterQueryFromContext(
	ctx context.Context,
	ds *goqu.SelectDataset,
	fragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
) (*goqu.SelectDataset, error) {
	maskCondition, hasMask, err := buildFragmentMaskConditionWithOptions(ctx, fragment, collector, false)
	return addFilterCondition(ds, maskCondition, hasMask, err)
}

func addFilterCondition(
	ds *goqu.SelectDataset,
	maskCondition exp.Expression,
	hasMask bool,
	err error,
) (*goqu.SelectDataset, error) {
	if err != nil {
		return nil, err
	}
	if !hasMask {
		return ds, nil
	}
	return ds.Where(maskCondition), nil
}

// AddFilterQueriesFromContext appends WHERE clauses for multiple fragment
// filters stored in ctx.
//
// Duplicate fragment entries and equivalent mask signatures are skipped so the
// resulting SQL does not repeat identical predicates. The returned dataset is
// the original dataset with all applicable fragment predicates applied.
func AddFilterQueriesFromContext(
	ctx context.Context,
	ds *goqu.SelectDataset,
	fragments []grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
) (*goqu.SelectDataset, error) {
	seenFragments := make(map[grammar.FragmentStringPattern]struct{}, len(fragments))
	seenSignatures := make(map[string]struct{}, len(fragments))
	for _, fragment := range fragments {
		if _, ok := seenFragments[fragment]; ok {
			continue
		}
		seenFragments[fragment] = struct{}{}

		signature, err := buildFragmentMaskSignature(ctx, fragment)
		if err != nil {
			return nil, err
		}
		if _, ok := seenSignatures[signature]; ok {
			continue
		}
		seenSignatures[signature] = struct{}{}

		ds, err = AddFilterQueryFromContext(ctx, ds, fragment, collector)
		if err != nil {
			return nil, err
		}
	}
	return ds, nil
}

// AddAllFilterQueriesFromContext appends WHERE clauses for every fragment
// filter stored in ctx.
//
// The collector resolves grammar field paths to SQL aliases. When ctx has no
// QueryFilter, or the QueryFilter has no filters, the original dataset is
// returned unchanged.
func AddAllFilterQueriesFromContext(
	ctx context.Context,
	ds *goqu.SelectDataset,
	collector *grammar.ResolvedFieldPathCollector,
) (*goqu.SelectDataset, error) {
	return AddAllFilterQueriesFromContextExcept(ctx, ds, collector, nil)
}

// AddAllFilterQueriesFromContextExcept appends WHERE clauses for every
// fragment filter stored in ctx except fragments matching excluded patterns.
//
// Excluded patterns are matched by fragment shape, including array segments, so
// callers can omit a fragment family while still applying the remaining ABAC
// filters. The returned dataset is sorted by fragment name before predicates
// are applied to keep generated SQL deterministic.
func AddAllFilterQueriesFromContextExcept(
	ctx context.Context,
	ds *goqu.SelectDataset,
	collector *grammar.ResolvedFieldPathCollector,
	excluded []grammar.FragmentStringPattern,
) (*goqu.SelectDataset, error) {
	queryFilter := GetQueryFilter(ctx)
	if queryFilter == nil || len(queryFilter.Filters) == 0 {
		return ds, nil
	}

	fragments := make([]grammar.FragmentStringPattern, 0, len(queryFilter.Filters))
	for fragment := range queryFilter.Filters {
		if matchesAnyFragmentPattern(fragment, excluded) {
			continue
		}
		fragments = append(fragments, fragment)
	}
	sort.Slice(fragments, func(i, j int) bool {
		return fragments[i] < fragments[j]
	})

	return AddFilterQueriesFromContext(ctx, ds, fragments, collector)
}

// FilterColumnSpec describes a selectable expression and its optional fragment
// mask.
//
// Exp is the SQL expression to select. Fragment identifies the ABAC fragment
// that controls whether Exp is exposed by GetColumnSelectStatement.
type FilterColumnSpec struct {
	Exp      exp.Expression
	Fragment *grammar.FragmentStringPattern
}

// Column builds a selectable column spec that is always exposed.
//
// The iexp parameter is returned as the column expression. Because no fragment
// is attached, GetColumnSelectStatement never wraps it in a mask CASE
// expression.
func Column(iexp exp.Expression) FilterColumnSpec {
	return FilterColumnSpec{Exp: iexp}
}

// MaskedColumn builds a selectable column spec controlled by a fragment filter.
//
// The iexp parameter is the SQL expression to expose when the fragment filter
// matches. The fragment parameter identifies the ABAC fragment whose condition
// decides whether the value is selected or replaced with NULL.
func MaskedColumn(iexp exp.Expression, fragment grammar.FragmentStringPattern) FilterColumnSpec {
	f := fragment
	return FilterColumnSpec{
		Exp:      iexp,
		Fragment: &f,
	}
}

// MaskedInnerColumnSpec describes one masked column in an inner/outer query
// pattern.
//
// Fragment identifies the ABAC fragment that controls visibility. FlagAlias is
// the inner SELECT alias for the computed boolean mask. RawAlias is the inner
// SELECT alias for the unmasked value that the outer query may expose.
type MaskedInnerColumnSpec struct {
	Fragment  grammar.FragmentStringPattern
	FlagAlias string
	RawAlias  string
}

// SharedFragmentMaskRuntime stores a reusable mask plan for an inner/outer
// reader query.
//
// It keeps the fragment list, generated inner SELECT projections, and the
// fragment-to-flag aliases needed to build outer CASE expressions without
// recalculating equivalent predicates.
type SharedFragmentMaskRuntime struct {
	fragments         []grammar.FragmentStringPattern
	projections       []interface{}
	aliasesByFragment map[grammar.FragmentStringPattern]string
}

// BuildSharedFragmentMaskRuntime creates reusable boolean mask projections for
// an inner/outer query.
//
// The ctx parameter supplies the QueryFilter, collector resolves grammar field
// paths to SQL aliases, and columns describes each raw inner-column alias and
// its controlling fragment. The returned runtime can append the matching WHERE
// filters to the inner query and build outer CASE projections against the
// derived table. Equivalent fragment masks share one flag projection.
func BuildSharedFragmentMaskRuntime(
	ctx context.Context,
	collector *grammar.ResolvedFieldPathCollector,
	columns []MaskedInnerColumnSpec,
) (*SharedFragmentMaskRuntime, error) {
	runtime := &SharedFragmentMaskRuntime{
		fragments:         make([]grammar.FragmentStringPattern, 0, len(columns)),
		projections:       make([]interface{}, 0, len(columns)),
		aliasesByFragment: make(map[grammar.FragmentStringPattern]string, len(columns)),
	}
	signatureToAlias := make(map[string]string, len(columns))
	usedAliases := make(map[string]struct{}, len(columns))
	seenFragments := make(map[grammar.FragmentStringPattern]struct{}, len(columns))

	for _, c := range columns {
		if _, ok := seenFragments[c.Fragment]; !ok {
			runtime.fragments = append(runtime.fragments, c.Fragment)
			seenFragments[c.Fragment] = struct{}{}
		}
		signature, err := buildFragmentMaskSignature(ctx, c.Fragment)
		if err != nil {
			return nil, err
		}
		if alias, ok := signatureToAlias[signature]; ok {
			runtime.aliasesByFragment[c.Fragment] = alias
			continue
		}
		if strings.TrimSpace(c.FlagAlias) == "" {
			return nil, fmt.Errorf("mask flag alias for fragment %q must not be empty", c.Fragment)
		}
		if _, exists := usedAliases[c.FlagAlias]; exists {
			return nil, fmt.Errorf("duplicate mask flag alias %q", c.FlagAlias)
		}
		proj, err := buildFragmentMaskFlagProjection(ctx, c.Fragment, collector, c.FlagAlias)
		if err != nil {
			return nil, err
		}
		runtime.projections = append(runtime.projections, proj)
		runtime.aliasesByFragment[c.Fragment] = c.FlagAlias
		signatureToAlias[signature] = c.FlagAlias
		usedAliases[c.FlagAlias] = struct{}{}
	}
	return runtime, nil
}

// Projections returns the inner SELECT projections for shared boolean mask
// flags.
//
// The returned slice can be appended to an inner SELECT list. A nil runtime
// returns nil so callers can safely use optional masking.
func (r *SharedFragmentMaskRuntime) Projections() []interface{} {
	if r == nil {
		return nil
	}
	return r.projections
}

// ApplyFilters appends WHERE predicates for the runtime's fragment masks.
//
// The ctx parameter supplies the QueryFilter and collector resolves grammar
// field paths to SQL aliases. A nil runtime returns the original dataset
// unchanged.
func (r *SharedFragmentMaskRuntime) ApplyFilters(
	ctx context.Context,
	ds *goqu.SelectDataset,
	collector *grammar.ResolvedFieldPathCollector,
) (*goqu.SelectDataset, error) {
	if r == nil {
		return ds, nil
	}
	return AddFilterQueriesFromContext(ctx, ds, r.fragments, collector)
}

// FlagAlias returns the projected boolean flag alias for a fragment.
//
// An error is returned when the runtime is nil or no flag was registered for
// the requested fragment.
func (r *SharedFragmentMaskRuntime) FlagAlias(fragment grammar.FragmentStringPattern) (string, error) {
	if r == nil {
		return "", fmt.Errorf("shared fragment mask runtime is nil")
	}
	alias, ok := r.aliasesByFragment[fragment]
	if !ok {
		return "", fmt.Errorf("missing shared mask alias for %q", fragment)
	}
	return alias, nil
}

// MaskedInnerAliasExpr builds the outer projection for one masked inner alias.
//
// The dataAlias parameter is the alias of the inner derived table, fragment
// identifies the controlling mask flag, and rawAlias is the unmasked inner
// column alias. The returned expression has the form CASE WHEN flag THEN value
// ELSE NULL.
func (r *SharedFragmentMaskRuntime) MaskedInnerAliasExpr(
	dataAlias string,
	fragment grammar.FragmentStringPattern,
	rawAlias string,
) (exp.Expression, error) {
	flagAlias, err := r.FlagAlias(fragment)
	if err != nil {
		return nil, err
	}
	return goqu.Case().
		When(goqu.I(dataAlias+"."+flagAlias), goqu.I(dataAlias+"."+rawAlias)).
		Else(nil), nil
}

// MaskedInnerAliasExprs builds outer CASE projections for multiple masked
// inner aliases.
//
// The dataAlias parameter is the alias of the inner derived table. The returned
// expressions preserve the order of columns and return an error if any column's
// controlling fragment has no registered flag alias.
func (r *SharedFragmentMaskRuntime) MaskedInnerAliasExprs(
	dataAlias string,
	columns []MaskedInnerColumnSpec,
) ([]exp.Expression, error) {
	expressions := make([]exp.Expression, 0, len(columns))
	for _, c := range columns {
		iexp, err := r.MaskedInnerAliasExpr(dataAlias, c.Fragment, c.RawAlias)
		if err != nil {
			return nil, err
		}
		expressions = append(expressions, iexp)
	}
	return expressions, nil
}

func extractExpressions(columns []FilterColumnSpec) []exp.Expression {
	expressions := make([]exp.Expression, 0, len(columns))

	for _, c := range columns {
		expressions = append(expressions, c.Exp)
	}

	return expressions
}

// buildFragmentMaskFlagProjection builds a boolean flag projection for one
// fragment-specific mask condition.
func buildFragmentMaskFlagProjection(
	ctx context.Context,
	fragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
	alias string,
) (exp.Expression, error) {
	maskCondition, hasMask, err := buildFragmentMaskCondition(ctx, fragment, collector)
	if err != nil {
		return nil, err
	}
	if !hasMask {
		return goqu.V(true).As(alias), nil
	}
	return goqu.Case().When(maskCondition, true).Else(false).As(alias), nil
}

func buildFragmentMaskCondition(
	ctx context.Context,
	fragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
) (exp.Expression, bool, error) {
	return buildFragmentMaskConditionWithOptions(ctx, fragment, collector, false)
}

func buildFragmentMaskConditionWithOptions(
	ctx context.Context,
	fragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
	inlineArrayEndedFragments bool,
) (exp.Expression, bool, error) {
	p := GetQueryFilter(ctx)
	if p == nil {
		return nil, false, nil
	}

	filters := p.FilterExpressionEntriesFor(fragment)
	if len(filters) == 0 {
		return nil, false, nil
	}

	wcs := make([]exp.Expression, 0, len(filters))
	for _, filter := range filters {
		evalCollector := collector
		if inlineArrayEndedFragments && filter.Match && fragmentEndsWithWildcardArraySegment(filter.Fragment) {
			// Array-ended fragments must be evaluated against the current row context
			// instead of descriptor-wide EXISTS correlation.
			evalCollector = nil
		}
		wc, _, err := filter.Expression.EvaluateToExpressionWithNegatedFragments(
			evalCollector,
			[]grammar.FragmentStringPattern{grammar.FragmentStringPattern(filter.Fragment)},
		)
		if err != nil {
			return nil, false, err
		}
		wcs = append(wcs, wc)
	}
	if len(wcs) == 1 {
		return wcs[0], true, nil
	}
	return goqu.And(wcs...), true, nil
}

func fragmentEndsWithWildcardArraySegment(fragment grammar.FragmentStringPattern) bool {
	tokens := builder.TokenizeField(string(fragment))
	if len(tokens) == 0 {
		return false
	}
	arrayToken, isArray := tokens[len(tokens)-1].(builder.ArrayToken)
	return isArray && arrayToken.Index < 0
}

func fragmentEndsWithArraySegment(fragment grammar.FragmentStringPattern) bool {
	tokens := builder.TokenizeField(string(fragment))
	if len(tokens) == 0 {
		return false
	}
	_, isArray := tokens[len(tokens)-1].(builder.ArrayToken)
	return isArray
}

func matchesAnyFragmentPattern(fragment grammar.FragmentStringPattern, patterns []grammar.FragmentStringPattern) bool {
	for _, pattern := range patterns {
		if fragmentPathMatches(fragment, pattern) {
			return true
		}
	}
	return false
}

func fragmentPathMatches(fragment grammar.FragmentStringPattern, pattern grammar.FragmentStringPattern) bool {
	if !fragmentRootsEqual(fragment, pattern) {
		return false
	}
	fragmentTokens := builder.TokenizeField(string(fragment))
	patternTokens := builder.TokenizeField(string(pattern))
	if len(fragmentTokens) != len(patternTokens) {
		return false
	}
	for i := range fragmentTokens {
		if fragmentTokens[i].GetName() != patternTokens[i].GetName() {
			return false
		}
		_, fragmentIsArray := fragmentTokens[i].(builder.ArrayToken)
		_, patternIsArray := patternTokens[i].(builder.ArrayToken)
		if fragmentIsArray != patternIsArray {
			return false
		}
	}
	return true
}

func buildFragmentMaskSignature(ctx context.Context, fragment grammar.FragmentStringPattern) (string, error) {
	p := GetQueryFilter(ctx)
	if p == nil {
		return "no-query-filter", nil
	}
	filters := p.FilterExpressionEntriesFor(fragment)
	if len(filters) == 0 {
		return "no-fragment-filter", nil
	}

	parts := make([]string, 0, len(filters))
	for _, filter := range filters {
		exprJSON, err := json.Marshal(filter.Expression)
		if err != nil {
			return "", err
		}
		bindings, err := grammar.ResolveFragmentFieldToSQL((*grammar.FragmentStringPattern)(&filter.Fragment))
		if err != nil {
			return "", err
		}
		bindingsJSON, err := json.Marshal(bindings)
		if err != nil {
			return "", err
		}
		parts = append(parts, string(exprJSON)+"|"+string(bindingsJSON))
	}
	sort.Strings(parts)
	return strings.Join(parts, "&&"), nil
}

// GetColumnSelectStatement builds SELECT expressions while honoring fragment
// filters stored in ctx.
//
// The columns parameter contains raw SELECT expressions and optional fragment
// masks. The collector parameter resolves grammar field paths to SQL aliases.
// When a column has an applicable fragment mask, the expression is wrapped in a
// CASE projection that returns NULL unless the fragment filter matches. When no
// QueryFilter or applicable masks exist, the raw column expressions are
// returned unchanged.
func GetColumnSelectStatement(ctx context.Context, columns []FilterColumnSpec, collector *grammar.ResolvedFieldPathCollector) ([]exp.Expression, error) {
	defaultReturn := extractExpressions(columns)
	p := GetQueryFilter(ctx)
	if p == nil {
		return defaultReturn, nil
	}

	var ok = false
	result := []exp.Expression{}
	for _, column := range columns {
		if column.Fragment != nil {
			maskCondition, hasMask, err := buildFragmentMaskCondition(ctx, *column.Fragment, collector)
			if err != nil {
				return nil, err
			}
			if hasMask {
				ok = true
				result = append(result, goqu.Case().When(maskCondition, column.Exp).Else(nil))
				continue
			}
		}
		result = append(result, column.Exp)
	}
	if !ok {
		return defaultReturn, nil
	}

	return result, nil
}

// AddFormulaQueryFromContext appends the formula-based WHERE clause stored in
// ctx to ds.
//
// The collector parameter resolves grammar field paths to SQL aliases. When ctx
// has no QueryFilter or no formula, the original dataset is returned unchanged.
// Errors from grammar expression evaluation are propagated to the caller.
func AddFormulaQueryFromContext(ctx context.Context, ds *goqu.SelectDataset, collector *grammar.ResolvedFieldPathCollector) (*goqu.SelectDataset, error) {
	p := GetQueryFilter(ctx)
	if p != nil && p.Formula != nil {
		wc, _, err := p.Formula.EvaluateToExpression(collector)
		if err != nil {
			return nil, err
		}
		ds = ds.Where(wc)
	}
	return ds, nil
}
