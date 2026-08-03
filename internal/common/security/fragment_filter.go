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

import "github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"

// FragmentFilters groups filter predicates by the fragment they control.
type FragmentFilters map[grammar.FragmentStringPattern]FragmentFilterPredicate

// FragmentFilterPredicate preserves the Boolean composition of fragment
// conditions while keeping row-local match mode attached to each leaf.
type FragmentFilterPredicate struct {
	Condition *grammar.LogicalExpression `json:"$condition,omitempty" yaml:"$condition,omitempty" mapstructure:"$condition,omitempty"`
	Match     bool                       `json:"$match,omitempty" yaml:"$match,omitempty" mapstructure:"$match,omitempty"`
	And       []FragmentFilterPredicate  `json:"$and,omitempty" yaml:"$and,omitempty" mapstructure:"$and,omitempty"`
	Or        []FragmentFilterPredicate  `json:"$or,omitempty" yaml:"$or,omitempty" mapstructure:"$or,omitempty"`
}

// FragmentFilterEntry associates a concrete fragment with its predicate.
type FragmentFilterEntry struct {
	Fragment  grammar.FragmentStringPattern
	Predicate FragmentFilterPredicate
}

// NewFragmentFilterPredicate creates a leaf with its evaluation scope.
func NewFragmentFilterPredicate(expression grammar.LogicalExpression, match bool) FragmentFilterPredicate {
	return FragmentFilterPredicate{Condition: &expression, Match: match}
}

// AndFragmentFilterPredicates combines predicates without changing leaf scope.
func AndFragmentFilterPredicates(predicates ...FragmentFilterPredicate) FragmentFilterPredicate {
	return combineFragmentFilterPredicates(true, predicates)
}

// OrFragmentFilterPredicates combines predicates without changing leaf scope.
func OrFragmentFilterPredicates(predicates ...FragmentFilterPredicate) FragmentFilterPredicate {
	return combineFragmentFilterPredicates(false, predicates)
}

func combineFragmentFilterPredicates(and bool, predicates []FragmentFilterPredicate) FragmentFilterPredicate {
	children := make([]FragmentFilterPredicate, 0, len(predicates))
	for _, predicate := range predicates {
		if predicate.isZero() {
			continue
		}
		if and && len(predicate.And) > 0 && predicate.Condition == nil && len(predicate.Or) == 0 {
			children = append(children, predicate.And...)
			continue
		}
		if !and && len(predicate.Or) > 0 && predicate.Condition == nil && len(predicate.And) == 0 {
			children = append(children, predicate.Or...)
			continue
		}
		children = append(children, predicate)
	}
	if len(children) == 1 {
		return children[0]
	}
	if and {
		return FragmentFilterPredicate{And: children}
	}
	return FragmentFilterPredicate{Or: children}
}

func (predicate FragmentFilterPredicate) isZero() bool {
	return predicate.Condition == nil && len(predicate.And) == 0 && len(predicate.Or) == 0
}
