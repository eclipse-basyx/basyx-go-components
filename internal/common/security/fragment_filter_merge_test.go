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
	if len(predicate.And) != 3 {
		t.Fatalf("expected rule formula and two filter leaves joined by AND, got %#v", predicate)
	}
	assertRuleFilterLeafMatch(t, predicate, "global-name", false)
	assertRuleFilterLeafMatch(t, predicate, "local-name", true)
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
