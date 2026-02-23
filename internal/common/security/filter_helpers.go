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
	p := GetQueryFilter(ctx)
	if p == nil {
		return ds, nil
	}

	filters := p.FilterExpressionEntriesFor(fragment)
	if len(filters) == 0 {
		return ds, nil
	}
	for _, filter := range filters {
		wc, _, err := filter.Expression.EvaluateToExpressionWithNegatedFragments(collector, []grammar.FragmentStringPattern{grammar.FragmentStringPattern(filter.Fragment)})

		if err != nil {
			return nil, err
		}
		ds = ds.Where(wc)
	}

	return ds, nil
}

// ExpressionIdentifiableMapper links a selectable expression with an optional
// identifier name used for ABAC fragment filtering; canBeFiltered controls
// whether the expression participates in filter-based projections.
type ExpressionIdentifiableMapper struct {
	Exp      exp.Expression
	Fragment *grammar.FragmentStringPattern
}

// FragmentMaskFlagSpec describes a fragment whose effective mask condition
// should be materialized as a boolean projection (flag column).
type FragmentMaskFlagSpec struct {
	Fragment grammar.FragmentStringPattern
	Alias    string
}

// SharedFragmentMaskPlan contains reusable boolean flag projections and a
// fragment-to-alias mapping for later outer-query CASE projections.
type SharedFragmentMaskPlan struct {
	Projections       []interface{}
	aliasesByFragment map[grammar.FragmentStringPattern]string
}

// FlagAliasFor returns the flag alias assigned to the fragment.
func (p *SharedFragmentMaskPlan) FlagAliasFor(fragment grammar.FragmentStringPattern) (string, bool) {
	if p == nil {
		return "", false
	}
	alias, ok := p.aliasesByFragment[fragment]
	return alias, ok
}

func extractExpressions(mappers []ExpressionIdentifiableMapper) []exp.Expression {
	expressions := make([]exp.Expression, 0, len(mappers))

	for _, m := range mappers {
		expressions = append(expressions, m.Exp)
	}

	return expressions
}

// BuildSharedFragmentMaskPlan builds reusable boolean flag projections for the
// provided fragments. Fragments with identical effective conditions (including
// fragment guard bindings) share a single projected flag alias.
func BuildSharedFragmentMaskPlan(
	ctx context.Context,
	collector *grammar.ResolvedFieldPathCollector,
	specs []FragmentMaskFlagSpec,
) (*SharedFragmentMaskPlan, error) {
	plan := &SharedFragmentMaskPlan{
		Projections:       make([]interface{}, 0, len(specs)),
		aliasesByFragment: make(map[grammar.FragmentStringPattern]string, len(specs)),
	}
	signatureToAlias := make(map[string]string, len(specs))
	usedAliases := make(map[string]struct{}, len(specs))

	for _, spec := range specs {
		signature, err := buildFragmentMaskSignature(ctx, spec.Fragment)
		if err != nil {
			return nil, err
		}
		if alias, ok := signatureToAlias[signature]; ok {
			plan.aliasesByFragment[spec.Fragment] = alias
			continue
		}
		if strings.TrimSpace(spec.Alias) == "" {
			return nil, fmt.Errorf("mask flag alias for fragment %q must not be empty", spec.Fragment)
		}
		if _, exists := usedAliases[spec.Alias]; exists {
			return nil, fmt.Errorf("duplicate mask flag alias %q", spec.Alias)
		}

		proj, err := BuildFragmentMaskFlagProjection(ctx, spec.Fragment, collector, spec.Alias)
		if err != nil {
			return nil, err
		}
		plan.Projections = append(plan.Projections, proj)
		plan.aliasesByFragment[spec.Fragment] = spec.Alias
		signatureToAlias[signature] = spec.Alias
		usedAliases[spec.Alias] = struct{}{}
	}

	return plan, nil
}

// BuildFragmentMaskFlagProjection builds a boolean flag projection for one
// fragment-specific mask condition.
func BuildFragmentMaskFlagProjection(
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
func GetColumnSelectStatement(ctx context.Context, expressionMappers []ExpressionIdentifiableMapper, collector *grammar.ResolvedFieldPathCollector) ([]exp.Expression, error) {
	defaultReturn := extractExpressions(expressionMappers)
	p := GetQueryFilter(ctx)
	if p == nil {
		return defaultReturn, nil
	}

	var ok = false
	result := []exp.Expression{}
	for _, expMapper := range expressionMappers {
		if expMapper.Fragment != nil {
			filters := p.FilterExpressionEntriesFor(*expMapper.Fragment)
			if len(filters) != 0 {
				ok = true

				wcs := make([]exp.Expression, 0, len(filters))
				for _, filter := range filters {
					wc, _, err := filter.Expression.EvaluateToExpressionWithNegatedFragments(
						collector,
						[]grammar.FragmentStringPattern{grammar.FragmentStringPattern(filter.Fragment)},
					)
					if err != nil {
						return nil, err
					}
					wcs = append(wcs, wc)
				}

				combined := wcs[0]
				if len(wcs) > 1 {
					combined = goqu.And(wcs...)
				}
				result = append(result, caseWhenColumn(combined, expMapper.Exp))
			} else {
				result = append(result, expMapper.Exp)
			}
		} else {
			result = append(result, expMapper.Exp)
		}
	}
	if !ok {
		return defaultReturn, nil
	}

	return result, nil
}

func caseWhenColumn(wc exp.Expression, iexp exp.Expression) exp.CaseExpression {
	return goqu.Case().
		When(
			wc,
			iexp,
		).
		Else(nil)
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

		ds = ds.Where(

			wc,
		)
	}
	return ds, nil
}
