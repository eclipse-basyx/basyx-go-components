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
	"regexp"
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
	maskCondition, hasMask, err := buildFragmentMaskCondition(ctx, fragment, collector)
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
	maskCondition, hasMask, err := buildFragmentMaskCondition(ctx, fragment, collector)
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

// BoolOrProjections aggregates each mask flag across rows of the same parent.
func (r *SharedFragmentMaskRuntime) BoolOrProjections(partitionBy ...interface{}) ([]interface{}, error) {
	if r == nil {
		return nil, nil
	}
	projections := make([]interface{}, 0, len(r.projections))
	window := goqu.W().PartitionBy(partitionBy...)
	for _, projection := range r.projections {
		aliased, ok := projection.(exp.AliasedExpression)
		if !ok {
			return nil, fmt.Errorf("SECURITY-MASKPROJ-ASSERTALIASED: mask projection is not aliased")
		}
		projections = append(
			projections,
			goqu.Func("BOOL_OR", aliased.Aliased()).Over(window).As(aliased.GetAs()),
		)
	}
	return projections, nil
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
		return goqu.L("TRUE").As(alias), nil
	}
	return goqu.Case().When(maskCondition, goqu.L("TRUE")).Else(goqu.L("FALSE")).As(alias), nil
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

	filters := p.FilterPredicateEntriesFor(fragment)
	if len(filters) == 0 {
		return nil, false, nil
	}

	wcs := make([]exp.Expression, 0, len(filters))
	for _, filter := range filters {
		wc, err := evaluateFragmentFilterPredicate(ctx, filter.Predicate, filter.Fragment, collector)
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

func evaluateFragmentFilterPredicate(
	ctx context.Context,
	predicate FragmentFilterPredicate,
	fragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
) (exp.Expression, error) {
	if predicate.Condition != nil {
		return evaluateFragmentFilterLeaf(ctx, predicate, fragment, collector)
	}
	if len(predicate.And) > 0 {
		return evaluateFragmentFilterChildren(ctx, predicate.And, fragment, collector, true)
	}
	if len(predicate.Or) > 0 {
		return evaluateFragmentFilterChildren(ctx, predicate.Or, fragment, collector, false)
	}
	return nil, fmt.Errorf("FILTER-EVALPRED-INVALID fragment filter predicate is empty")
}

func evaluateFragmentFilterLeaf(
	ctx context.Context,
	predicate FragmentFilterPredicate,
	fragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
) (exp.Expression, error) {
	if predicate.global {
		whereCondition, _, err := predicate.Condition.EvaluateToExpression(collector.WithoutInlineAliases())
		return whereCondition, err
	}
	fragment = predicate.evaluationFragment(fragment)
	evalCollector := collector.WithoutInlineAliases()
	if predicate.Match {
		if predicate.caller {
			evalCollector = conditionVisibleRowCollector(ctx, collector.ForFragmentMatch(fragment))
		} else {
			evalCollector = collector.ForFragmentMatch(fragment)
		}
	} else if predicate.caller {
		evalCollector = conditionVisibleCollector(ctx, evalCollector)
	}
	whereCondition, _, err := predicate.Condition.EvaluateToExpressionWithNegatedFragments(
		evalCollector,
		[]grammar.FragmentStringPattern{fragment},
	)
	if err != nil {
		return nil, err
	}
	return whereCondition, nil
}

func evaluateFragmentFilterChildren(
	ctx context.Context,
	children []FragmentFilterPredicate,
	fragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
	and bool,
) (exp.Expression, error) {
	expressions := make([]exp.Expression, 0, len(children))
	for _, child := range children {
		expression, err := evaluateFragmentFilterPredicate(ctx, child, fragment, collector)
		if err != nil {
			return nil, err
		}
		expressions = append(expressions, expression)
	}
	if and {
		return goqu.And(expressions...), nil
	}
	return goqu.Or(expressions...), nil
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
	filters := p.FilterPredicateEntriesFor(fragment)
	if len(filters) == 0 {
		return "no-fragment-filter", nil
	}

	parts := make([]string, 0, len(filters))
	for _, filter := range filters {
		exprJSON, err := json.Marshal(filter.Predicate)
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
		parts = append(parts, fmt.Sprintf(
			"%s|%s|%s",
			exprJSON,
			fragmentFilterPredicateScopeSignature(filter.Predicate),
			bindingsJSON,
		))
	}
	sort.Strings(parts)
	return strings.Join(parts, "&&"), nil
}

func fragmentFilterPredicateScopeSignature(predicate FragmentFilterPredicate) string {
	if predicate.Condition != nil {
		if predicate.global {
			return "leaf:global"
		}
		if predicate.caller {
			return "leaf:caller"
		}
		if predicate.fragment == nil {
			return "leaf"
		}
		return "leaf:" + string(*predicate.fragment)
	}
	if len(predicate.And) > 0 {
		return "and(" + fragmentFilterPredicateScopesSignature(predicate.And) + ")"
	}
	return "or(" + fragmentFilterPredicateScopesSignature(predicate.Or) + ")"
}

func fragmentFilterPredicateScopesSignature(predicates []FragmentFilterPredicate) string {
	parts := make([]string, 0, len(predicates))
	for _, predicate := range predicates {
		parts = append(parts, fragmentFilterPredicateScopeSignature(predicate))
	}
	return strings.Join(parts, ",")
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
	if authorized := AuthorizedQueryFromContext(ctx); authorized != nil {
		if authorized.outer.decision == AccessViewDenied {
			return ds.Where(goqu.L("FALSE")), nil
		}
		if authorized.outer.queryFilter != nil && authorized.outer.queryFilter.Formula != nil {
			wc, _, err := authorized.outer.queryFilter.Formula.EvaluateToExpression(collector.WithoutFieldValueDecorator())
			if err != nil {
				return nil, err
			}
			ds = ds.Where(wc)
		}
		caller := authorized.callerForBackend()
		if caller.Condition != nil {
			wc, _, err := caller.Condition.EvaluateToExpression(conditionVisibleCollector(ctx, collector))
			if err != nil {
				return nil, err
			}
			ds = ds.Where(wc)
		}
		return ds, nil
	}

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

func conditionVisibleCollector(
	ctx context.Context,
	collector *grammar.ResolvedFieldPathCollector,
) *grammar.ResolvedFieldPathCollector {
	return decorateConditionVisibleCollector(ctx, collector)
}

func conditionVisibleRowCollector(
	ctx context.Context,
	collector *grammar.ResolvedFieldPathCollector,
) *grammar.ResolvedFieldPathCollector {
	return decorateConditionVisibleCollector(ctx, collector)
}

func decorateConditionVisibleCollector(
	ctx context.Context,
	collector *grammar.ResolvedFieldPathCollector,
) *grammar.ResolvedFieldPathCollector {
	authorized := AuthorizedQueryFromContext(ctx)
	if collector == nil || authorized == nil {
		return collector
	}
	return collector.WithFieldValueDecorator(func(access grammar.SemanticFieldAccess) (grammar.FieldValueDecoration, error) {
		target := SemanticAccessTarget{
			Resource: semanticResourceFromField(access.Field),
			Field:    access.Field,
		}
		view, found := authorized.accessView(target.Resource)
		if !found || view.decision == AccessViewDenied {
			return grammar.FieldValueDecoration{SQLValue: goqu.L("NULL")}, nil
		}
		if view.decision == AccessViewUnrestricted ||
			view.queryFilter == nil && len(view.alternatives) == 0 {
			return grammar.FieldValueDecoration{SQLValue: access.SQLValue, IncludeResolved: true}, nil
		}
		if target.Resource == authorized.outer.resource &&
			fieldVisibleAfterOuterSelection(view, target) {
			return grammar.FieldValueDecoration{SQLValue: access.SQLValue, IncludeResolved: true}, nil
		}

		guard, resolved, guarded, err := compileConditionVisibilityGuard(view, target)
		if err != nil {
			return grammar.FieldValueDecoration{}, err
		}
		if !guarded {
			return grammar.FieldValueDecoration{SQLValue: access.SQLValue, IncludeResolved: true}, nil
		}
		return grammar.FieldValueDecoration{
			SQLValue:           access.SQLValue,
			AdditionalResolved: resolved,
			IncludeResolved:    true,
			VisibilityWitness:  guard,
		}, nil
	})
}

func fieldVisibleAfterOuterSelection(
	view SemanticAccessView,
	target SemanticAccessTarget,
) bool {
	fragment := grammar.FragmentStringPattern(target.Field)
	if len(view.alternatives) == 0 {
		return view.queryFilter != nil &&
			len(conditionVisibilityFilterEntries(view.queryFilter, fragment)) == 0
	}
	for _, alternative := range view.alternatives {
		if len(alternative.coverages) > 0 ||
			len(conditionVisibilityFilterEntries(
				&QueryFilter{Filters: alternative.filters},
				fragment,
			)) > 0 {
			return false
		}
	}
	return true
}

func (q *AuthorizedQuery) accessView(resource SemanticResourceKind) (SemanticAccessView, bool) {
	if q == nil {
		return SemanticAccessView{}, false
	}
	if resource == q.outer.resource {
		return q.outer, true
	}
	view, ok := q.relatedViews[resource]
	if !ok && resource == SemanticResourceSME {
		view, ok = q.relatedViews[SemanticResourceSM]
	}
	return view, ok
}

func semanticResourceFromField(field grammar.ModelStringPattern) SemanticResourceKind {
	value := strings.TrimPrefix(string(field), "$")
	separator := strings.IndexAny(value, ".#")
	if separator >= 0 {
		value = value[:separator]
	}
	return SemanticResourceKind(value)
}

func compileConditionVisibilityGuard(
	view SemanticAccessView,
	target SemanticAccessTarget,
) (exp.Expression, []grammar.ResolvedFieldPath, bool, error) {
	if view.decision == AccessViewDenied {
		return goqu.L("FALSE"), nil, true, nil
	}
	if view.decision == AccessViewUnrestricted ||
		view.queryFilter == nil && len(view.alternatives) == 0 {
		return nil, nil, false, nil
	}
	if len(view.alternatives) > 0 {
		return compileAlternativeVisibilityGuard(view.alternatives, target)
	}

	fragment := grammar.FragmentStringPattern(target.Field)
	filters := conditionVisibilityFilterEntries(view.queryFilter, fragment)
	if len(filters) > 0 {
		expressions := make([]exp.Expression, 0, len(filters))
		var resolved []grammar.ResolvedFieldPath
		for _, filter := range filters {
			expression, paths, err := evaluateConditionVisibilityPredicate(filter.Predicate, filter.Fragment, &target)
			if err != nil {
				return nil, nil, false, err
			}
			expressions = append(expressions, expression)
			resolved = append(resolved, paths...)
		}
		return goqu.And(expressions...), resolved, true, nil
	}

	if view.queryFilter.Formula == nil {
		return nil, nil, false, nil
	}
	guard, resolved, err := view.queryFilter.Formula.EvaluateToExpression(nil)
	if err != nil {
		return nil, nil, false, err
	}
	return guard, resolved, true, nil
}

func compileAlternativeVisibilityGuard(
	alternatives []CompiledGrantAlternative,
	target SemanticAccessTarget,
) (exp.Expression, []grammar.ResolvedFieldPath, bool, error) {
	expressions := make([]exp.Expression, 0, len(alternatives))
	var resolved []grammar.ResolvedFieldPath
	for _, alternative := range alternatives {
		expression, paths, applicable, err := compileGrantAlternativeVisibility(alternative, target)
		if err != nil {
			return nil, nil, false, err
		}
		if !applicable {
			continue
		}
		expressions = append(expressions, expression)
		resolved = append(resolved, paths...)
	}
	if len(expressions) == 0 {
		return goqu.L("FALSE"), nil, true, nil
	}
	return goqu.Or(expressions...), resolved, true, nil
}

func compileGrantAlternativeVisibility(
	alternative CompiledGrantAlternative,
	target SemanticAccessTarget,
) (exp.Expression, []grammar.ResolvedFieldPath, bool, error) {
	formula, resolved, err := alternative.formula.EvaluateToExpression(nil)
	if err != nil {
		return nil, nil, false, fmt.Errorf("SECURITY-CONDITIONVIEW-GRANTFORMULA: %w", err)
	}
	expressions := []exp.Expression{formula}
	filters := conditionVisibilityFilterEntries(
		&QueryFilter{Filters: alternative.filters},
		grammar.FragmentStringPattern(target.Field),
	)
	for _, filter := range filters {
		expression, paths, evaluateErr := evaluateConditionVisibilityPredicate(filter.Predicate, filter.Fragment, &target)
		if evaluateErr != nil {
			return nil, nil, false, evaluateErr
		}
		expressions = append(expressions, expression)
		resolved = append(resolved, paths...)
	}
	coverage, paths, applicable, err := compileSemanticCoverageGuard(alternative.coverages, target)
	if err != nil || !applicable {
		return nil, nil, applicable, err
	}
	if coverage != nil {
		expressions = append(expressions, coverage)
		resolved = append(resolved, paths...)
	}
	return goqu.And(expressions...), resolved, true, nil
}

func compileSemanticCoverageGuard(
	coverages []semanticObjectCoverage,
	target SemanticAccessTarget,
) (exp.Expression, []grammar.ResolvedFieldPath, bool, error) {
	if len(coverages) == 0 {
		return nil, nil, true, nil
	}
	var expressions []exp.Expression
	var resolved []grammar.ResolvedFieldPath
	for _, coverage := range coverages {
		if coverage.resource != target.Resource {
			continue
		}
		expression, paths, applicable, err := compileSemanticObjectCoverage(coverage, target)
		if err != nil {
			return nil, nil, false, err
		}
		if !applicable {
			continue
		}
		expressions = append(expressions, expression)
		resolved = append(resolved, paths...)
	}
	if len(expressions) == 0 {
		return nil, nil, false, nil
	}
	return goqu.Or(expressions...), resolved, true, nil
}

func compileSemanticObjectCoverage(
	coverage semanticObjectCoverage,
	target SemanticAccessTarget,
) (exp.Expression, []grammar.ResolvedFieldPath, bool, error) {
	if target.Resource == SemanticResourceSM {
		return compileSubmodelIdentifierCoverage(coverage)
	}
	if target.Resource == SemanticResourceSME {
		return compileSMEObjectCoverage(coverage, target)
	}
	return nil, nil, false, nil
}

func compileSubmodelIdentifierCoverage(
	coverage semanticObjectCoverage,
) (exp.Expression, []grammar.ResolvedFieldPath, bool, error) {
	if coverage.allSubmodelIDs {
		return goqu.L("TRUE"), nil, true, nil
	}
	field := grammar.ModelStringPattern("$sm#id")
	value := grammar.StandardString(coverage.submodelID)
	condition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &value}}}
	expression, resolved, err := condition.EvaluateToExpression(nil)
	if err != nil {
		return nil, nil, false, fmt.Errorf("SECURITY-CONDITIONVIEW-SUBMODELID: %w", err)
	}
	return expression, resolved, true, nil
}

func compileSMEObjectCoverage(
	coverage semanticObjectCoverage,
	target SemanticAccessTarget,
) (exp.Expression, []grammar.ResolvedFieldPath, bool, error) {
	targetPath := submodelElementPath(target.Field)
	if !coverageCoversSMEField(coverage, target.Field, targetPath) {
		return nil, nil, false, nil
	}
	expressions := make([]exp.Expression, 0, 2)
	var resolved []grammar.ResolvedFieldPath
	identifier, paths, _, err := compileSubmodelIdentifierCoverage(coverage)
	if err != nil {
		return nil, nil, false, err
	}
	if !coverage.allSubmodelIDs {
		expressions = append(expressions, identifier)
		resolved = append(resolved, paths...)
	}
	if !coverage.allSubmodelElements && targetPath == "" {
		pathExpression, pathResolved, pathErr := submodelElementCoverageExpression(coverage)
		if pathErr != nil {
			return nil, nil, false, pathErr
		}
		expressions = append(expressions, pathExpression)
		resolved = append(resolved, pathResolved...)
	}
	if len(expressions) == 0 {
		return goqu.L("TRUE"), nil, true, nil
	}
	return goqu.And(expressions...), resolved, true, nil
}

func coverageCoversSMEField(
	coverage semanticObjectCoverage,
	field grammar.ModelStringPattern,
	targetPath string,
) bool {
	if coverage.submodelFragment != "" {
		if submodelElementFragment(field) != coverage.submodelFragment {
			return false
		}
		return targetPath == "" || targetPath == coverage.submodelElementPath
	}
	if coverage.allSubmodelElements || targetPath == "" {
		return true
	}
	return submodelElementPathCovered(coverage.submodelElementPath, targetPath)
}

func submodelElementCoverageExpression(
	coverage semanticObjectCoverage,
) (exp.Expression, []grammar.ResolvedFieldPath, error) {
	if coverage.submodelFragment != "" {
		return submodelElementExactExpression(coverage.submodelElementPath)
	}
	return submodelElementSubtreeExpression(coverage.submodelElementPath)
}

func submodelElementPath(field grammar.ModelStringPattern) string {
	value := string(field)
	if !strings.HasPrefix(value, "$sme") {
		return ""
	}
	hash := strings.IndexByte(value, '#')
	if hash < 0 {
		hash = len(value)
	}
	return strings.TrimPrefix(value[len("$sme"):hash], ".")
}

func submodelElementFragment(field grammar.ModelStringPattern) string {
	value := string(field)
	if !strings.HasPrefix(value, "$sme") {
		return ""
	}
	hash := strings.IndexByte(value, '#')
	if hash < 0 || hash+1 >= len(value) {
		return ""
	}
	return strings.TrimSpace(value[hash+1:])
}

func submodelElementPathCovered(root string, candidate string) bool {
	root = strings.TrimSpace(root)
	candidate = strings.TrimSpace(candidate)
	return candidate == root || strings.HasPrefix(candidate, root+".") || strings.HasPrefix(candidate, root+"[")
}

func submodelElementSubtreeExpression(
	path string,
) (exp.Expression, []grammar.ResolvedFieldPath, error) {
	idShortPath, resolved, err := resolveSubmodelElementPathColumn()
	if err != nil {
		return nil, nil, fmt.Errorf("SECURITY-CONDITIONVIEW-REFERABLEPATH: %w", err)
	}
	escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(path)
	return goqu.Or(
		idShortPath.Eq(path),
		goqu.L("? LIKE (? || '.%') ESCAPE '!'", idShortPath, escaped),
		goqu.L("? LIKE (? || '[%]%') ESCAPE '!'", idShortPath, escaped),
	), []grammar.ResolvedFieldPath{resolved}, nil
}

func submodelElementExactExpression(
	path string,
) (exp.Expression, []grammar.ResolvedFieldPath, error) {
	idShortPath, resolved, err := resolveSubmodelElementPathColumn()
	if err != nil {
		return nil, nil, fmt.Errorf("SECURITY-CONDITIONVIEW-FRAGMENTPATH: %w", err)
	}
	return idShortPath.Eq(path), []grammar.ResolvedFieldPath{resolved}, nil
}

func resolveSubmodelElementPathColumn() (exp.IdentifierExpression, grammar.ResolvedFieldPath, error) {
	field := grammar.ModelStringPattern("$sme#idShort")
	resolved, err := grammar.ResolveScalarFieldToSQL(&field)
	if err != nil {
		return nil, grammar.ResolvedFieldPath{}, err
	}
	resolved.Column = "submodel_element.idshort_path"
	return goqu.I(resolved.Column), resolved, nil
}

func conditionVisibilityFilterEntries(
	queryFilter *QueryFilter,
	field grammar.FragmentStringPattern,
) []FragmentFilterEntry {
	if queryFilter == nil {
		return nil
	}
	entries := make([]FragmentFilterEntry, 0, len(queryFilter.Filters))
	for fragment, predicate := range queryFilter.Filters {
		if !fragmentAffectsSemanticField(fragment, field) {
			continue
		}
		entries = append(entries, FragmentFilterEntry{Fragment: fragment, Predicate: predicate})
	}
	sort.Slice(entries, func(first, second int) bool {
		return entries[first].Fragment < entries[second].Fragment
	})
	return entries
}

func fragmentAffectsSemanticField(
	fragment grammar.FragmentStringPattern,
	field grammar.FragmentStringPattern,
) bool {
	if fragmentRoot(fragment) != "$sme" || fragmentRoot(field) != "$sme" {
		return fragmentCoversSemanticField(fragment, field)
	}
	if !smeFragmentSuffixOverlaps(fragment, field) {
		return false
	}
	fragmentPath := submodelElementPath(grammar.ModelStringPattern(fragment))
	fieldPath := submodelElementPath(grammar.ModelStringPattern(field))
	if fragmentPath == "" || fieldPath == "" {
		return true
	}
	fragmentTokens := tokenizeSMEPath(fragmentPath)
	fieldTokens := tokenizeSMEPath(fieldPath)
	if semanticFragmentSuffix(fragment) != "" {
		return len(fragmentTokens) == len(fieldTokens) && semanticTokenPathsOverlap(fragmentTokens, fieldTokens)
	}
	return semanticTokenPathsOverlap(fragmentTokens, fieldTokens)
}

func smeFragmentSuffixOverlaps(
	fragment grammar.FragmentStringPattern,
	field grammar.FragmentStringPattern,
) bool {
	fragmentSuffix := semanticFragmentSuffix(fragment)
	if fragmentSuffix == "" {
		return true
	}
	fragmentTokens := builder.TokenizeField("$sme#" + fragmentSuffix)
	fieldTokens := builder.TokenizeField("$sme#" + semanticFragmentSuffix(field))
	return semanticTokenPathsOverlap(fragmentTokens, fieldTokens)
}

func semanticFragmentSuffix(fragment grammar.FragmentStringPattern) string {
	value := string(fragment)
	hash := strings.IndexByte(value, '#')
	if hash < 0 || hash+1 >= len(value) {
		return ""
	}
	return value[hash+1:]
}

func tokenizeSMEPath(path string) []builder.Token {
	return builder.TokenizeField("$sme#" + path)
}

func semanticTokenPathsOverlap(first []builder.Token, second []builder.Token) bool {
	limit := min(len(first), len(second))
	for index := 0; index < limit; index++ {
		if !semanticTokensOverlap(first[index], second[index]) {
			return false
		}
	}
	return true
}

func semanticTokenPathCovers(cover []builder.Token, candidate []builder.Token) bool {
	if len(cover) > len(candidate) {
		return false
	}
	for index := range cover {
		if !semanticTokenCovers(cover[index], candidate[index]) {
			return false
		}
	}
	return true
}

func semanticTokensOverlap(first builder.Token, second builder.Token) bool {
	if first.GetName() != second.GetName() {
		return false
	}
	firstArray, firstIsArray := first.(builder.ArrayToken)
	secondArray, secondIsArray := second.(builder.ArrayToken)
	if firstIsArray != secondIsArray {
		return true
	}
	return !firstIsArray || firstArray.Index < 0 || secondArray.Index < 0 || firstArray.Index == secondArray.Index
}

func semanticTokenCovers(cover builder.Token, candidate builder.Token) bool {
	if cover.GetName() != candidate.GetName() {
		return false
	}
	coverArray, coverIsArray := cover.(builder.ArrayToken)
	candidateArray, candidateIsArray := candidate.(builder.ArrayToken)
	if !coverIsArray {
		return true
	}
	if !candidateIsArray {
		return false
	}
	if coverArray.Index < 0 {
		return true
	}
	return candidateArray.Index >= 0 && coverArray.Index == candidateArray.Index
}

func fragmentCoversSemanticField(
	fragment grammar.FragmentStringPattern,
	field grammar.FragmentStringPattern,
) bool {
	if !fragmentRootsEqual(fragment, field) {
		return false
	}
	fragmentTokens := builder.TokenizeField(string(fragment))
	fieldTokens := builder.TokenizeField(string(field))
	if len(fragmentTokens) > len(fieldTokens) {
		return false
	}
	for index := range fragmentTokens {
		if fragmentTokens[index].GetName() != fieldTokens[index].GetName() {
			return false
		}
		_, fragmentArray := fragmentTokens[index].(builder.ArrayToken)
		_, fieldArray := fieldTokens[index].(builder.ArrayToken)
		if fragmentArray != fieldArray {
			return false
		}
	}
	return true
}

func evaluateConditionVisibilityPredicate(
	predicate FragmentFilterPredicate,
	fragment grammar.FragmentStringPattern,
	target *SemanticAccessTarget,
) (exp.Expression, []grammar.ResolvedFieldPath, error) {
	if predicate.Condition != nil {
		if predicate.global {
			return predicate.Condition.EvaluateToExpression(nil)
		}
		fragment = predicate.evaluationFragment(fragment)
		if target != nil && target.Resource == SemanticResourceSME {
			return evaluateSMEConditionVisibilityPredicate(*predicate.Condition, fragment, *target)
		}
		return predicate.Condition.EvaluateToExpressionWithNegatedFragments(
			nil,
			[]grammar.FragmentStringPattern{fragment},
		)
	}
	children := predicate.Or
	combine := goqu.Or
	if len(predicate.And) > 0 {
		children = predicate.And
		combine = goqu.And
	}
	if len(children) == 0 {
		return nil, nil, fmt.Errorf("SECURITY-CONDITIONVIEW-EMPTYPREDICATE fragment filter predicate is empty")
	}
	expressions := make([]exp.Expression, 0, len(children))
	var resolved []grammar.ResolvedFieldPath
	for _, child := range children {
		expression, paths, err := evaluateConditionVisibilityPredicate(child, fragment, target)
		if err != nil {
			return nil, nil, err
		}
		expressions = append(expressions, expression)
		resolved = append(resolved, paths...)
	}
	return combine(expressions...), resolved, nil
}

func evaluateSMEConditionVisibilityPredicate(
	condition grammar.LogicalExpression,
	fragment grammar.FragmentStringPattern,
	target SemanticAccessTarget,
) (exp.Expression, []grammar.ResolvedFieldPath, error) {
	conditionExpression, resolved, err := evaluateSMEConditionWithFragmentScope(condition, fragment)
	if err != nil {
		return nil, nil, err
	}
	fragmentPath := submodelElementPath(grammar.ModelStringPattern(fragment))
	if fragmentPath == "" || smeFragmentPathCoversField(fragment, target.Field) {
		return conditionExpression, resolved, nil
	}
	rowScope, scopeResolved, err := smeFragmentRowScopeExpression(fragment)
	if err != nil {
		return nil, nil, err
	}
	return goqu.Or(conditionExpression, goqu.L("NOT (?)", rowScope)), append(resolved, scopeResolved...), nil
}

func evaluateSMEConditionWithFragmentScope(
	condition grammar.LogicalExpression,
	fragment grammar.FragmentStringPattern,
) (exp.Expression, []grammar.ResolvedFieldPath, error) {
	suffix := semanticFragmentSuffix(fragment)
	if suffix == "" {
		return condition.EvaluateToExpression(nil)
	}
	suffixFragment := grammar.FragmentStringPattern("$sme#" + suffix)
	expression, resolved, err := condition.EvaluateToExpressionWithNegatedFragments(
		nil,
		[]grammar.FragmentStringPattern{suffixFragment},
	)
	if err != nil {
		return nil, nil, err
	}
	bindings, err := grammar.ResolveFragmentFieldToSQL(&suffixFragment)
	if err != nil {
		return nil, nil, err
	}
	if len(bindings) > 0 {
		resolved = append(resolved, grammar.ResolvedFieldPath{ArrayBindings: bindings})
	}
	return expression, resolved, nil
}

func smeFragmentPathCoversField(
	fragment grammar.FragmentStringPattern,
	field grammar.ModelStringPattern,
) bool {
	fragmentPath := submodelElementPath(grammar.ModelStringPattern(fragment))
	fieldPath := submodelElementPath(field)
	if fragmentPath == "" {
		return true
	}
	if fieldPath == "" {
		return false
	}
	fragmentTokens := tokenizeSMEPath(fragmentPath)
	fieldTokens := tokenizeSMEPath(fieldPath)
	if semanticFragmentSuffix(fragment) != "" && len(fragmentTokens) != len(fieldTokens) {
		return false
	}
	return semanticTokenPathCovers(fragmentTokens, fieldTokens)
}

func smeFragmentRowScopeExpression(
	fragment grammar.FragmentStringPattern,
) (exp.Expression, []grammar.ResolvedFieldPath, error) {
	path := submodelElementPath(grammar.ModelStringPattern(fragment))
	field := grammar.ModelStringPattern("$sme#idShort")
	resolved, err := grammar.ResolveScalarFieldToSQL(&field)
	if err != nil {
		return nil, nil, fmt.Errorf("SECURITY-CONDITIONVIEW-SMEFILTERSCOPE: %w", err)
	}
	resolved.Column = "submodel_element.idshort_path"
	pathColumn := goqu.I(resolved.Column)
	if strings.Contains(path, "[]") {
		pattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(path), regexp.QuoteMeta("[]"), `\[[0-9]+\]`) + "$"
		return goqu.L("? ~ ?", pathColumn, pattern), []grammar.ResolvedFieldPath{resolved}, nil
	}
	if semanticFragmentSuffix(fragment) != "" {
		return pathColumn.Eq(path), []grammar.ResolvedFieldPath{resolved}, nil
	}
	escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(path)
	return goqu.Or(
		pathColumn.Eq(path),
		goqu.L("? LIKE (? || '.%') ESCAPE '!'", pathColumn, escaped),
		goqu.L("? LIKE (? || '[%]%') ESCAPE '!'", pathColumn, escaped),
	), []grammar.ResolvedFieldPath{resolved}, nil
}
