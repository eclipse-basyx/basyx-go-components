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

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// WithQueryFilter stores the provided query filter in the context.
func WithQueryFilter(ctx context.Context, queryFilter *QueryFilter) context.Context {
	if queryFilter == nil {
		return ctx
	}
	return context.WithValue(ctx, filterKey, queryFilter)
}

// WithoutQueryFilter returns a child context that keeps request metadata but
// removes row-level ABAC filters for technical checks such as existence probes.
func WithoutQueryFilter(ctx context.Context) context.Context {
	return context.WithValue(ctx, filterKey, struct{}{})
}

// CloneQueryFilter returns a deep copy of an already compiled query filter.
// It preserves internal metadata without reapplying external JSON input limits.
func CloneQueryFilter(queryFilter *QueryFilter) (*QueryFilter, error) {
	if queryFilter == nil {
		return nil, nil
	}
	cloned := *queryFilter
	if queryFilter.Formula != nil {
		formula := cloneLogicalExpression(*queryFilter.Formula)
		cloned.Formula = &formula
	}
	if queryFilter.FormulasByRight != nil {
		cloned.FormulasByRight = make(map[grammar.RightsEnum]grammar.LogicalExpression, len(queryFilter.FormulasByRight))
		for right, formula := range queryFilter.FormulasByRight {
			cloned.FormulasByRight[right] = cloneLogicalExpression(formula)
		}
	}
	if queryFilter.Filters != nil {
		cloned.Filters = make(FragmentFilters, len(queryFilter.Filters))
		for fragment, predicate := range queryFilter.Filters {
			cloned.Filters[fragment] = cloneFragmentFilterPredicate(predicate)
		}
	}
	return &cloned, nil
}

func cloneFragmentFilterPredicate(predicate FragmentFilterPredicate) FragmentFilterPredicate {
	cloned := predicate
	if predicate.Condition != nil {
		condition := cloneLogicalExpression(*predicate.Condition)
		cloned.Condition = &condition
	}
	if predicate.fragment != nil {
		fragment := *predicate.fragment
		cloned.fragment = &fragment
	}
	cloned.And = cloneFragmentFilterPredicates(predicate.And)
	cloned.Or = cloneFragmentFilterPredicates(predicate.Or)
	return cloned
}

func cloneFragmentFilterPredicates(predicates []FragmentFilterPredicate) []FragmentFilterPredicate {
	if predicates == nil {
		return nil
	}
	cloned := make([]FragmentFilterPredicate, len(predicates))
	for index, predicate := range predicates {
		cloned[index] = cloneFragmentFilterPredicate(predicate)
	}
	return cloned
}
