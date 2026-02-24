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

func extractExpressions(mappers []ExpressionIdentifiableMapper) []exp.Expression {
	expressions := make([]exp.Expression, 0, len(mappers))

	for _, m := range mappers {
		expressions = append(expressions, m.Exp)
	}

	return expressions
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
