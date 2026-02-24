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
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// AddFilterQueryFromContext appends the WHERE clause for the given fragment
// identifier if a QueryFilter is present in the context. When no filter is
// available or the fragment is not defined, the original dataset is returned
// unchanged.
func AddFilterQueryFromContext(
	ctx context.Context,
	ds *goqu.SelectDataset,
	fragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
) (*goqu.SelectDataset, error) {
	maskCondition, hasMask, err := buildFragmentMaskCondition(ctx, fragment, collector)
	if err != nil {
		return nil, err
	}
	if !hasMask {
		return ds, nil
	}
	return ds.Where(maskCondition), nil
}

// AddFilterQueriesFromContext appends WHERE clauses for multiple fragments
// while skipping duplicate fragment entries.
func AddFilterQueriesFromContext(
	ctx context.Context,
	ds *goqu.SelectDataset,
	fragments []grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
) (*goqu.SelectDataset, error) {
	seenFragments := make(map[grammar.FragmentStringPattern]struct{}, len(fragments))
	var err error
	for _, fragment := range fragments {
		if _, ok := seenFragments[fragment]; ok {
			continue
		}
		seenFragments[fragment] = struct{}{}
		ds, err = AddFilterQueryFromContext(ctx, ds, fragment, collector)
		if err != nil {
			return nil, err
		}
	}
	return ds, nil
}

// FilterColumnSpec describes a selectable expression with an optional fragment
// used for ABAC masking/filtering.
type FilterColumnSpec struct {
	Exp      exp.Expression
	Fragment *grammar.FragmentStringPattern
}

// Column builds a plain selectable column spec without fragment masking.
func Column(iexp exp.Expression) FilterColumnSpec {
	return FilterColumnSpec{Exp: iexp}
}

// MaskedColumn builds a selectable column spec that is controlled by the given
// fragment's filter condition.
func MaskedColumn(iexp exp.Expression, fragment grammar.FragmentStringPattern) FilterColumnSpec {
	f := fragment
	return FilterColumnSpec{
		Exp:      iexp,
		Fragment: &f,
	}
}

// MaskedInnerColumnSpec describes one masked column in an inner/outer query
// pattern: a fragment controls visibility of a raw inner-column alias.
type MaskedInnerColumnSpec struct {
	Fragment  grammar.FragmentStringPattern
	FlagAlias string
	RawAlias  string
}

// SharedFragmentMaskRuntime wraps a shared mask plan together with the source
// specs and convenience helpers for reader query construction.
type SharedFragmentMaskRuntime struct {
	fragments         []grammar.FragmentStringPattern
	projections       []interface{}
	aliasesByFragment map[grammar.FragmentStringPattern]string
}

// BuildSharedFragmentMaskRuntime creates shared boolean mask flag projections
// and returns a runtime helper that can apply fragment filters and build outer
// CASE projections against an inner derived table.
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

// Projections returns the inner SELECT projections for shared boolean flags.
func (r *SharedFragmentMaskRuntime) Projections() []interface{} {
	if r == nil {
		return nil
	}
	return r.projections
}

// ApplyFilters appends fragment filters for the runtime's mask specs.
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

// FlagAlias returns the projected flag alias for a fragment.
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

// MaskedInnerAliasExpr returns CASE WHEN <flag> THEN <inner alias> ELSE NULL.
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

// MaskedInnerAliasExprs builds CASE expressions for a set of masked inner
// columns against the same derived-table alias, preserving order.
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
		wc, _, err := filter.Expression.EvaluateToExpressionWithNegatedFragments(
			collector,
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

// GetColumnSelectStatement builds the list of SELECT expressions while honoring
// fragment filters stored in the context. Filterable expressions are wrapped
// in CASE projections so their values are only exposed when the other
// fragment filters succeed; otherwise the raw expressions are returned.
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

// AddFormulaQueryFromContext appends the Formula-based WHERE clause found in
// the context's QueryFilter to the provided dataset. When no filter formula is
// present, the dataset is returned unchanged; errors from expression building
// are propagated.
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
