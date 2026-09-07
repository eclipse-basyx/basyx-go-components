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

// CloneQueryFilter returns a deep copy of the provided query filter.
func CloneQueryFilter(queryFilter *QueryFilter) (*QueryFilter, error) {
	if queryFilter == nil {
		return nil, nil
	}

	b, err := json.Marshal(queryFilter)
	if err != nil {
		return nil, err
	}

	var cloned QueryFilter
	if err := json.Unmarshal(b, &cloned); err != nil {
		return nil, err
	}
	restoreQueryFilterIndeterminateMarkers(&cloned, queryFilter)
	for fragment, predicate := range queryFilter.Filters {
		clonedPredicate, ok := cloned.Filters[fragment]
		if !ok {
			continue
		}
		clonedPredicate, err = restoreFragmentFilterPredicateMetadata(clonedPredicate, predicate)
		if err != nil {
			return nil, err
		}
		cloned.Filters[fragment] = clonedPredicate
	}

	return &cloned, nil
}

func restoreQueryFilterIndeterminateMarkers(cloned, source *QueryFilter) {
	if cloned.Formula != nil && source.Formula != nil {
		restoreLogicalIndeterminateMarkers(cloned.Formula, *source.Formula)
	}
	for right, sourceFormula := range source.FormulasByRight {
		clonedFormula, ok := cloned.FormulasByRight[right]
		if !ok {
			continue
		}
		restoreLogicalIndeterminateMarkers(&clonedFormula, sourceFormula)
		cloned.FormulasByRight[right] = clonedFormula
	}
}

func restoreLogicalIndeterminateMarkers(cloned *grammar.LogicalExpression, source grammar.LogicalExpression) {
	if source.Indeterminate {
		*cloned = source
		return
	}
	for index := range source.And {
		restoreLogicalIndeterminateMarkers(&cloned.And[index], source.And[index])
	}
	for index := range source.Or {
		restoreLogicalIndeterminateMarkers(&cloned.Or[index], source.Or[index])
	}
	if source.Not != nil && cloned.Not != nil {
		restoreLogicalIndeterminateMarkers(cloned.Not, *source.Not)
	}
	for index := range source.Match {
		restoreMatchIndeterminateMarkers(&cloned.Match[index], source.Match[index])
	}
}

func restoreMatchIndeterminateMarkers(cloned *grammar.MatchExpression, source grammar.MatchExpression) {
	if source.Indeterminate {
		*cloned = source
		return
	}
	for index := range source.Match {
		restoreMatchIndeterminateMarkers(&cloned.Match[index], source.Match[index])
	}
}

func restoreFragmentFilterPredicateMetadata(
	cloned FragmentFilterPredicate,
	source FragmentFilterPredicate,
) (FragmentFilterPredicate, error) {
	if (cloned.Condition == nil) != (source.Condition == nil) ||
		len(cloned.And) != len(source.And) ||
		len(cloned.Or) != len(source.Or) {
		return FragmentFilterPredicate{}, fmt.Errorf("AUTH-CLONEQF-SCOPEMISMATCH fragment predicate shape changed during clone")
	}
	cloned.global = source.global
	if cloned.Condition != nil && source.Condition != nil {
		restoreLogicalIndeterminateMarkers(cloned.Condition, *source.Condition)
	}
	if source.fragment != nil {
		fragmentCopy := *source.fragment
		cloned.fragment = &fragmentCopy
	}
	for i := range cloned.And {
		var err error
		cloned.And[i], err = restoreFragmentFilterPredicateMetadata(cloned.And[i], source.And[i])
		if err != nil {
			return FragmentFilterPredicate{}, err
		}
	}
	for i := range cloned.Or {
		var err error
		cloned.Or[i], err = restoreFragmentFilterPredicateMetadata(cloned.Or[i], source.Or[i])
		if err != nil {
			return FragmentFilterPredicate{}, err
		}
	}
	return cloned, nil
}
