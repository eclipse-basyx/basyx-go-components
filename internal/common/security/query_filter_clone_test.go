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
	"reflect"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestCloneQueryFilterPreservesNilFields(t *testing.T) {
	cloned, err := CloneQueryFilter(nil)
	if err != nil || cloned != nil {
		t.Fatalf("nil filter changed: %#v, %v", cloned, err)
	}
	source := &QueryFilter{}
	cloned, err = CloneQueryFilter(source)
	if err != nil || cloned == source || !reflect.DeepEqual(cloned, source) {
		t.Fatalf("empty filter changed or aliases source: %#v, %v", cloned, err)
	}
}

func TestCloneQueryFilterIsolatesCompiledState(t *testing.T) {
	for _, mutateSource := range []bool{false, true} {
		name := "mutate clone"
		if mutateSource {
			name = "mutate source"
		}
		t.Run(name, func(t *testing.T) {
			source := cloneIsolationFilterFixture()
			cloned, err := CloneQueryFilter(source)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(source, cloned) {
				t.Fatal("clone lost expressions, indeterminate markers, or private fragment metadata")
			}
			changed, preserved := cloned, source
			if mutateSource {
				changed, preserved = source, cloned
			}
			mutateCloneIsolationFilter(changed)
			if !reflect.DeepEqual(preserved, cloneIsolationFilterFixture()) {
				t.Fatal("mutating one filter changed the other filter's compiled state")
			}
		})
	}
}

func cloneIsolationFilterFixture() *QueryFilter {
	role := grammar.StandardString("viewer")
	field := grammar.ModelStringPattern("$sme.A#value")
	one := float64(1)
	fragment := grammar.FragmentStringPattern("$sme.A#value")
	trueValue, falseValue := true, false
	formula := grammar.LogicalExpression{And: []grammar.LogicalExpression{
		{Eq: grammar.ComparisonItems{
			{StrCast: &grammar.Value{Attribute: map[string]any{"CLAIM": "role"}}},
			{StrVal: &role},
		}},
		{Match: []grammar.MatchExpression{{Match: []grammar.MatchExpression{{Indeterminate: true}}}}},
	}}
	globalCondition := grammar.LogicalExpression{Boolean: &trueValue}
	scopedCondition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {NumVal: &one}}}
	invalidCondition := grammar.LogicalExpression{Indeterminate: true}
	return &QueryFilter{
		Formula: &formula,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumREAD: {Or: []grammar.LogicalExpression{
				{Boolean: &trueValue}, {Not: &grammar.LogicalExpression{Boolean: &falseValue}},
			}},
			grammar.RightsEnumUPDATE: {BoolCast: &grammar.Value{Attribute: map[string]string{"CLAIM": "active"}}},
		},
		Filters: FragmentFilters{fragment: {
			And: []FragmentFilterPredicate{
				{Condition: &globalCondition, global: true},
				{Or: []FragmentFilterPredicate{
					{Condition: &scopedCondition, Match: true, fragment: &fragment, caller: true},
					{Condition: &invalidCondition},
				}},
			},
		}},
	}
}

func mutateCloneIsolationFilter(filter *QueryFilter) {
	filter.Formula.And[0].Eq[0].StrCast.Attribute.(map[string]any)["CLAIM"] = "changed"
	*filter.Formula.And[0].Eq[1].StrVal = "changed"
	filter.Formula.And[1].Match[0].Match[0].Indeterminate = false
	read := filter.FormulasByRight[grammar.RightsEnumREAD]
	*read.Or[0].Boolean = false
	*read.Or[1].Not.Boolean = true
	update := filter.FormulasByRight[grammar.RightsEnumUPDATE]
	update.BoolCast.Attribute.(map[string]string)["CLAIM"] = "changed"
	delete(filter.FormulasByRight, grammar.RightsEnumUPDATE)
	predicate := filter.Filters["$sme.A#value"]
	*predicate.And[0].Condition.Boolean = false
	predicate.And[0].global = false
	leaf := &predicate.And[1].Or[0]
	*leaf.fragment = "$sme.Other#value"
	*leaf.Condition.Eq[0].Field = "$sme.Other#value"
	*leaf.Condition.Eq[1].NumVal = 2
	leaf.Match, leaf.caller = false, false
	predicate.And[1].Or[1].Condition.Indeterminate = false
	filter.Filters["$sme.New#value"] = FragmentFilterPredicate{global: true}
}

func TestCloneQueryFilterIsolatesAttributeContainers(t *testing.T) {
	for _, mutateSource := range []bool{false, true} {
		name := "mutate clone"
		if mutateSource {
			name = "mutate source"
		}
		t.Run(name, func(t *testing.T) {
			source := cloneAttributeContainerFixture()
			cloned, err := CloneQueryFilter(source)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(source, cloned) {
				t.Fatal("clone changed internal attribute container types or values")
			}
			changed, preserved := cloned, source
			if mutateSource {
				changed, preserved = source, cloned
			}
			attribute := changed.Formula.Contains[0].Attribute.(map[string]any)
			attribute["strings"].([]string)[0] = "changed"
			attribute["objects"].([]map[string]any)[0]["value"] = "changed"
			nested := attribute["array"].([]any)[0].(map[string]any)
			nested["inner"].([]any)[0] = "changed"
			attribute["map"].(map[string]string)["value"] = "changed"
			attribute["extra"] = true
			if !reflect.DeepEqual(preserved, cloneAttributeContainerFixture()) {
				t.Fatal("nested attribute maps or slices alias the other filter")
			}
		})
	}
}

func cloneAttributeContainerFixture() *QueryFilter {
	needle := grammar.StandardString("value")
	attribute := map[string]any{
		"strings": []string{"value"},
		"objects": []map[string]any{{"value": "original"}},
		"array":   []any{map[string]any{"inner": []any{"original"}}},
		"map":     map[string]string{"value": "original"},
	}
	formula := grammar.LogicalExpression{Contains: grammar.StringItems{{Attribute: attribute}, {StrVal: &needle}}}
	return &QueryFilter{Formula: &formula}
}
