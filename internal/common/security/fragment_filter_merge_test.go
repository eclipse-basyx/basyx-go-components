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
	"encoding/json"
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestAuthorizeMultipleABACRulesPreservesMatchModePerAlternative(t *testing.T) {
	model := mustParseAASRegistryAccessModel(t, mixedMatchRulesModelJSON)
	result := model.AuthorizeWithFilterWithOptions(EvalInput{
		Method: "GET",
		Path:   "/shell-descriptors",
	}, grammar.DefaultSimplifyOptions())
	if !result.Allowed || result.QueryFilter == nil {
		t.Fatalf("expected filtered authorization, got %#v", result)
	}

	fragment := grammar.FragmentStringPattern("$aasdesc#specificAssetIds[]")
	predicate := result.QueryFilter.Filters[fragment]
	if len(predicate.Or) != 2 {
		t.Fatalf("expected one OR branch per ABAC rule, got %#v", predicate)
	}
	assertRuleFilterLeafMatch(t, predicate.Or[0], "global-name", false)
	assertRuleFilterLeafMatch(t, predicate.Or[1], "local-name", true)
}

func TestAuthorizeSingleABACRulePreservesMatchModePerFilter(t *testing.T) {
	model := mustParseAASRegistryAccessModel(t, mixedMatchRulesModelJSON)
	if len(model.rules) != 2 {
		t.Fatalf("expected two materialized test rules, got %d", len(model.rules))
	}
	model.rules[0].filterList = append(model.rules[0].filterList, model.rules[1].filterList...)
	model.rules = model.rules[:1]

	result := model.AuthorizeWithFilterWithOptions(EvalInput{
		Method: "GET",
		Path:   "/shell-descriptors",
	}, grammar.DefaultSimplifyOptions())
	if !result.Allowed || result.QueryFilter == nil {
		t.Fatalf("expected filtered authorization, got %#v", result)
	}

	fragment := grammar.FragmentStringPattern("$aasdesc#specificAssetIds[]")
	predicate := result.QueryFilter.Filters[fragment]
	if len(predicate.And) != 2 {
		t.Fatalf("expected the true rule formula to be removed from the two filter leaves, got %#v", predicate)
	}
	assertRuleFilterLeafMatch(t, predicate, "global-name", false)
	assertRuleFilterLeafMatch(t, predicate, "local-name", true)
}

func TestAuthorizeUnrestrictedRuleRemovesEquivalentFragmentRestriction(t *testing.T) {
	model := mustParseAASRegistryAccessModel(t, mixedMatchRulesModelJSON)
	model.rules[1].filterList = nil

	result := model.AuthorizeWithFilterWithOptions(EvalInput{
		Method: "GET",
		Path:   "/shell-descriptors",
	}, grammar.DefaultSimplifyOptions())
	if !result.Allowed || result.QueryFilter == nil {
		t.Fatalf("expected authorization result, got %#v", result)
	}

	fragment := grammar.FragmentStringPattern("$aasdesc#specificAssetIds[]")
	if _, restricted := result.QueryFilter.Filters[fragment]; restricted {
		t.Fatalf("unrestricted rule must remove the equivalent fragment restriction: %#v", result.QueryFilter.Filters)
	}
}

func TestAuthorizeMultipleABACRulesEquivalentFragmentsDoNotExposeRestrictedIndex(t *testing.T) {
	model := mustParseAASRegistryAccessModel(t, mixedMatchRulesModelJSON)
	falseCondition := boolExpression(false)
	indexedFragment := grammar.FragmentStringPattern("$aasdesc#specificAssetIds[0]")
	model.rules[0].filterList[0].CONDITION = &falseCondition
	model.rules[1].filterList[0].CONDITION = &falseCondition
	model.rules[1].filterList[0].FRAGMENT = &indexedFragment

	result := model.AuthorizeWithFilterWithOptions(EvalInput{
		Method: "GET",
		Path:   "/shell-descriptors",
	}, grammar.DefaultSimplifyOptions())
	if !result.Allowed || result.QueryFilter == nil {
		t.Fatalf("expected filtered authorization, got %#v", result)
	}

	cloned, err := CloneQueryFilter(result.QueryFilter)
	if err != nil {
		t.Fatalf("failed to clone query filter: %v", err)
	}
	indexedFilters := cloned.FilterPredicateEntriesFor(indexedFragment)
	if len(indexedFilters) == 0 {
		t.Fatal("expected fragment filters for indexed position 0")
	}
	positionVisible, constant := constantFragmentFilterEntriesValue(indexedFilters, indexedFragment)
	if !constant {
		t.Fatalf("expected constant regression predicates, got %#v", indexedFilters)
	}
	if positionVisible {
		t.Fatal("position 0 is visible although neither permitting rule's applicable fragment restriction passes")
	}

	otherFragment := grammar.FragmentStringPattern("$aasdesc#specificAssetIds[1]")
	otherFilters := cloned.FilterPredicateEntriesFor(otherFragment)
	positionVisible, constant = constantFragmentFilterEntriesValue(otherFilters, otherFragment)
	if !constant {
		t.Fatalf("expected constant regression predicates, got %#v", otherFilters)
	}
	if !positionVisible {
		t.Fatal("position 1 is hidden although the indexed restriction applies only to position 0")
	}
}

func TestCloneQueryFilterPreservesIndeterminateMarkers(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$sm#id")
	value := grammar.StandardString("allowed")
	filter := &QueryFilter{Formula: &grammar.LogicalExpression{And: []grammar.LogicalExpression{
		{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &value}}},
		{Indeterminate: true},
	}}}

	cloned, err := CloneQueryFilter(filter)
	if err != nil {
		t.Fatalf("CloneQueryFilter() error = %v", err)
	}
	if cloned.Formula == nil || len(cloned.Formula.And) != 2 || !cloned.Formula.And[1].Indeterminate {
		t.Fatalf("CloneQueryFilter() lost indeterminate marker: %#v", cloned)
	}
}

func constantFragmentFilterEntriesValue(entries []FragmentFilterEntry, target grammar.FragmentStringPattern) (bool, bool) {
	visible := true
	for _, entry := range entries {
		value, constant := constantFragmentFilterPredicateValue(entry.Predicate, entry.Fragment, target)
		if !constant {
			return false, false
		}
		visible = visible && value
	}
	return visible, true
}

func constantFragmentFilterPredicateValue(
	predicate FragmentFilterPredicate,
	fallback grammar.FragmentStringPattern,
	target grammar.FragmentStringPattern,
) (bool, bool) {
	if predicate.Condition != nil {
		if predicate.Condition.Boolean == nil {
			return false, false
		}
		if !predicate.global && !fragmentScopeAppliesToTarget(predicate.evaluationFragment(fallback), target) {
			return true, true
		}
		return *predicate.Condition.Boolean, true
	}
	if len(predicate.And) > 0 {
		return constantFragmentFilterChildrenValue(predicate.And, fallback, target, true)
	}
	if len(predicate.Or) > 0 {
		return constantFragmentFilterChildrenValue(predicate.Or, fallback, target, false)
	}
	return false, false
}

func constantFragmentFilterChildrenValue(
	children []FragmentFilterPredicate,
	fallback grammar.FragmentStringPattern,
	target grammar.FragmentStringPattern,
	and bool,
) (bool, bool) {
	result := and
	for _, child := range children {
		value, constant := constantFragmentFilterPredicateValue(child, fallback, target)
		if !constant {
			return false, false
		}
		if and {
			result = result && value
		} else {
			result = result || value
		}
	}
	return result, true
}

func fragmentScopeAppliesToTarget(scope grammar.FragmentStringPattern, target grammar.FragmentStringPattern) bool {
	if !fragmentPathMatches(scope, target) {
		return false
	}
	scopeTokens := builder.TokenizeField(string(scope))
	targetTokens := builder.TokenizeField(string(target))
	for i, scopeToken := range scopeTokens {
		scopeArray, isArray := scopeToken.(builder.ArrayToken)
		if !isArray || scopeArray.Index < 0 {
			continue
		}
		targetArray := targetTokens[i].(builder.ArrayToken)
		if scopeArray.Index != targetArray.Index {
			return false
		}
	}
	return true
}

func assertRuleFilterLeafMatch(t *testing.T, predicate FragmentFilterPredicate, value string, expectedMatch bool) {
	t.Helper()
	for _, leaf := range fragmentFilterLeaves(predicate) {
		serialized, err := json.Marshal(leaf.Condition)
		if err != nil {
			t.Fatalf("failed to marshal filter leaf: %v", err)
		}
		if strings.Contains(string(serialized), value) {
			if leaf.Match != expectedMatch {
				t.Fatalf("filter %q has match=%t, want %t", value, leaf.Match, expectedMatch)
			}
			return
		}
	}
	t.Fatalf("filter leaf %q not found in %#v", value, predicate)
}

func fragmentFilterLeaves(predicate FragmentFilterPredicate) []FragmentFilterPredicate {
	if predicate.Condition != nil {
		return []FragmentFilterPredicate{predicate}
	}
	leaves := make([]FragmentFilterPredicate, 0, len(predicate.And)+len(predicate.Or))
	for _, child := range predicate.And {
		leaves = append(leaves, fragmentFilterLeaves(child)...)
	}
	for _, child := range predicate.Or {
		leaves = append(leaves, fragmentFilterLeaves(child)...)
	}
	return leaves
}

const mixedMatchRulesModelJSON = `{
  "AllAccessPermissionRules": {
    "DEFATTRIBUTES": [
      { "name": "anonymous", "attributes": [ { "GLOBAL": "ANONYMOUS" } ] }
    ],
    "DEFOBJECTS": [
      { "name": "shells", "objects": [ { "DESCRIPTOR": "$aasdesc(\"*\")" } ] }
    ],
    "DEFACLS": [
      { "name": "read", "acl": { "USEATTRIBUTES": "anonymous", "RIGHTS": ["READ"], "ACCESS": "ALLOW" } }
    ],
    "DEFFORMULAS": [
      { "name": "always_true", "formula": { "$boolean": true } }
    ],
    "rules": [
      {
        "USEACL": "read",
        "USEOBJECTS": ["shells"],
        "USEFORMULA": "always_true",
        "FILTER": {
          "FRAGMENT": "$aasdesc#specificAssetIds[]",
          "CONDITION": {
            "$eq": [
              { "$field": "$aasdesc#specificAssetIds[].name" },
              { "$strVal": "global-name" }
            ]
          }
        }
      },
      {
        "USEACL": "read",
        "USEOBJECTS": ["shells"],
        "USEFORMULA": "always_true",
        "FILTER": {
          "FRAGMENT": "$aasdesc#specificAssetIds[]",
          "MATCH": true,
          "CONDITION": {
            "$eq": [
              { "$field": "$aasdesc#specificAssetIds[].name" },
              { "$strVal": "local-name" }
            ]
          }
        }
      }
    ]
  }
}`
