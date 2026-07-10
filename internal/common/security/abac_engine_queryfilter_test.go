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

package auth

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestQueryFilter_FilterExpressionsFor_ExactMatch(t *testing.T) {
	b := true
	expr := grammar.LogicalExpression{Boolean: &b}

	q := QueryFilter{Filters: FragmentFilters{
		"$aasdesc#endpoints[2]": expr,
	}}

	entries := q.FilterExpressionEntriesFor("$aasdesc#endpoints[2]")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Fragment != "$aasdesc#endpoints[2]" {
		t.Fatalf("expected fragment %q, got %q", "$aasdesc#endpoints[2]", entries[0].Fragment)
	}
	jEntry, _ := json.Marshal(entries[0].Expression)
	jWantEntry, _ := json.Marshal(expr)
	if string(jEntry) != string(jWantEntry) {
		t.Fatalf("expected %s, got %s", string(jWantEntry), string(jEntry))
	}
}

func TestQueryFilter_FilterExpressionEntriesFor_DoesNotMatchDifferentRoots(t *testing.T) {
	allow := true
	q := QueryFilter{Filters: FragmentFilters{
		"$sm#idShort":  {Boolean: &allow},
		"$sme#idShort": {Boolean: &allow},
	}}

	entries := q.FilterExpressionEntriesFor("$sm#idShort")
	if len(entries) != 1 {
		t.Fatalf("expected one $sm entry, got %d", len(entries))
	}
	if entries[0].Fragment != "$sm#idShort" {
		t.Fatalf("expected $sm fragment, got %q", entries[0].Fragment)
	}
}

func TestWithoutQueryFilterRemovesStoredFilter(t *testing.T) {
	b := true
	ctx := WithQueryFilter(context.Background(), &QueryFilter{
		Formula: &grammar.LogicalExpression{Boolean: &b},
	})
	if GetQueryFilter(ctx) == nil {
		t.Fatal("expected query filter before stripping")
	}

	stripped := WithoutQueryFilter(ctx)
	if got := GetQueryFilter(stripped); got != nil {
		t.Fatalf("expected stripped query filter to be nil, got %#v", got)
	}
}

func TestQueryFilter_FilterExpressionEntriesFor_WildcardIncludesLiteralAndIndexed(t *testing.T) {
	b1 := true
	b2 := false
	b3 := true

	// literal [] + indexed entries
	exprWildcard := grammar.LogicalExpression{Boolean: &b1}
	expr2 := grammar.LogicalExpression{Boolean: &b2}
	expr10 := grammar.LogicalExpression{Boolean: &b3}

	q := QueryFilter{Filters: FragmentFilters{
		"$aasdesc#specificAssetIds[]":   exprWildcard,
		"$aasdesc#specificAssetIds[2]":  expr2,
		"$aasdesc#specificAssetIds[10]": expr10,
	}}

	entries := q.FilterExpressionEntriesFor("$aasdesc#specificAssetIds[]")
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	fragments := map[grammar.FragmentStringPattern]struct{}{}
	for _, e := range entries {
		fragments[e.Fragment] = struct{}{}
	}
	for _, want := range []grammar.FragmentStringPattern{
		"$aasdesc#specificAssetIds[]",
		"$aasdesc#specificAssetIds[2]",
		"$aasdesc#specificAssetIds[10]",
	} {
		if _, ok := fragments[want]; !ok {
			t.Fatalf("expected fragment %q to be present", want)
		}
	}
}

func TestQueryFilter_FilterExpressionEntriesFor_SMEWildcardMatchesIndexedPath(t *testing.T) {
	t.Parallel()

	allow := true
	q := QueryFilter{Filters: FragmentFilters{
		"$sme.List[0]#value": {Boolean: &allow},
	}}

	entries := q.FilterExpressionEntriesFor("$sme.List[]#value")
	if len(entries) != 1 {
		t.Fatalf("expected indexed SME path to match wildcard, got %d entries", len(entries))
	}
	if entries[0].Fragment != "$sme.List[0]#value" {
		t.Fatalf("expected indexed SME fragment, got %q", entries[0].Fragment)
	}
}

func TestQueryFilter_FilterExpressionsFor_WildcardMatchesIndexedAndSorted(t *testing.T) {
	b1 := true
	b3 := true

	expr2 := grammar.LogicalExpression{Boolean: &b1}
	expr10 := grammar.LogicalExpression{Boolean: &b3}

	q := QueryFilter{Filters: FragmentFilters{
		"$aasdesc#endpoints[10]": expr10,
		"$aasdesc#endpoints[2]":  expr2,
		"$aasdesc#other[1]":      expr2,
	}}

	entries := q.FilterExpressionEntriesFor("$aasdesc#endpoints[]")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	fragments := map[grammar.FragmentStringPattern]struct{}{}
	for _, e := range entries {
		fragments[e.Fragment] = struct{}{}
	}
	if _, ok := fragments["$aasdesc#endpoints[2]"]; !ok {
		t.Fatalf("expected fragment %q to be present", "$aasdesc#endpoints[2]")
	}
	if _, ok := fragments["$aasdesc#endpoints[10]"]; !ok {
		t.Fatalf("expected fragment %q to be present", "$aasdesc#endpoints[10]")
	}
}

func TestQueryFilter_FilterExpressionEntriesFor_WildcardSuffixMustMatchPath(t *testing.T) {
	b1 := true
	b2 := false
	b3 := true

	exprLiteral := grammar.LogicalExpression{Boolean: &b1}
	expr2name := grammar.LogicalExpression{Boolean: &b2}
	expr2 := grammar.LogicalExpression{Boolean: &b3}

	q := QueryFilter{Filters: FragmentFilters{
		"$aasdesc#specificAssetIds[]":       exprLiteral,
		"$aasdesc#specificAssetIds[2].name": expr2name,
		"$aasdesc#specificAssetIds[2]":      expr2,
	}}

	entries := q.FilterExpressionEntriesFor("$aasdesc#specificAssetIds[].name")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Fragment != "$aasdesc#specificAssetIds[2].name" {
		t.Fatalf("expected fragment %q, got %q", "$aasdesc#specificAssetIds[2].name", entries[0].Fragment)
	}
}

func TestQueryFilter_FilterExpressionFor_WildcardCombinesOr(t *testing.T) {
	// FilterExpressionFor was removed; FilterExpressionEntriesFor returns the matching entries.
	b1 := true
	b2 := false
	exprA := grammar.LogicalExpression{Boolean: &b1}
	exprB := grammar.LogicalExpression{Boolean: &b2}

	q := QueryFilter{Filters: FragmentFilters{
		"$aasdesc#endpoints[0]": exprA,
		"$aasdesc#endpoints[1]": exprB,
	}}

	entries := q.FilterExpressionEntriesFor("$aasdesc#endpoints[]")
	if len(entries) != 2 {
		j, _ := json.Marshal(entries)
		t.Fatalf("expected 2 entries, got %d: %s", len(entries), string(j))
	}
}

func TestQueryFilter_FilterExpressionEntriesFor_PropagatesMatchFlag(t *testing.T) {
	b := true
	expr := grammar.LogicalExpression{Boolean: &b}

	q := QueryFilter{
		Filters: FragmentFilters{
			"$aasdesc#specificAssetIds[]": expr,
		},
		FilterMatch: FragmentMatchModes{
			"$aasdesc#specificAssetIds[]": true,
		},
	}

	entries := q.FilterExpressionEntriesFor("$aasdesc#specificAssetIds[]")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !entries[0].Match {
		t.Fatalf("expected Match=true for fragment entry")
	}
}
