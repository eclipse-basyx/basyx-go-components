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
	"net/http"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/go-chi/chi/v5"
)

func TestCallerConditionReadsSecurityHiddenFieldThroughGuard(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$aas#idShort")
	secret := grammar.StandardString("secret")
	condition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{
		{Field: &field},
		{StrVal: &secret},
	}}
	ctx := WithQueryFilter(t.Context(), queryFilterHidingField("$aas#idShort"))
	ctx = mustAuthorizedQueryContext(ctx, t, grammar.Query{Condition: &condition})

	sql := buildAuthorizedAASSelectionSQL(ctx, t)
	if strings.Count(sql, "::boolean") < 2 || !strings.Contains(sql, `"aas"."id_short"`) {
		t.Fatalf("hidden caller field must be evaluated with a condition-visible guard:\n%s", sql)
	}
	if strings.Contains(sql, "CASE") {
		t.Fatalf("ordinary caller fields must remain unwrapped and indexable:\n%s", sql)
	}
}

func TestDeniedRelatedSubmodelIsNullUnderPositiveAndNegatedConditions(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$sm#idShort")
	value := grammar.StandardString("secret")
	leaf := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &value}}}
	for _, test := range []struct {
		name      string
		condition grammar.LogicalExpression
	}{
		{name: "positive", condition: leaf},
		{name: "negated", condition: grammar.LogicalExpression{Not: &leaf}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.WithValue(t.Context(), authorizedQueryContextKey{}, &AuthorizedQuery{
				caller: grammar.Query{Condition: &test.condition},
				outer:  SemanticAccessView{resource: SemanticResourceAAS, decision: AccessViewUnrestricted},
				relatedViews: map[SemanticResourceKind]SemanticAccessView{
					SemanticResourceSM: {resource: SemanticResourceSM, decision: AccessViewDenied},
				},
			})
			ctx = grammar.ContextWithAASHierarchyQueries(ctx)
			sql := buildAuthorizedAASSelectionSQL(ctx, t)
			if !strings.Contains(sql, "NULL") {
				t.Fatalf("denied related field must compile to neutral NULL:\n%s", sql)
			}
			if strings.Contains(sql, "submodel") || strings.Contains(sql, "EXISTS") {
				t.Fatalf("denied related data must not become a negatable existence test:\n%s", sql)
			}
		})
	}
}

func TestRelatedSubmodelGrantAndCallerPredicateShareOneExistsWitness(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$sm#idShort")
	allowed := grammar.StandardString("allowed")
	requested := grammar.StandardString("requested")
	grant := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &allowed}}}
	caller := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &requested}}}
	ctx := context.WithValue(t.Context(), authorizedQueryContextKey{}, &AuthorizedQuery{
		caller: grammar.Query{Condition: &caller},
		outer:  SemanticAccessView{resource: SemanticResourceAAS, decision: AccessViewUnrestricted},
		relatedViews: map[SemanticResourceKind]SemanticAccessView{
			SemanticResourceSM: {
				resource: SemanticResourceSM,
				decision: AccessViewRestricted,
				queryFilter: &QueryFilter{
					Formula: &grant,
				},
			},
		},
	})
	ctx = grammar.ContextWithAASHierarchyQueries(ctx)
	sql := buildAuthorizedAASSelectionSQL(ctx, t)
	if count := strings.Count(sql, "EXISTS"); count != 1 {
		t.Fatalf("grant and caller predicate must use one related-row witness, got %d EXISTS clauses:\n%s", count, sql)
	}
	if strings.Count(sql, `"submodel"."id_short"`) < 2 {
		t.Fatalf("related grant is not bound to the caller field read:\n%s", sql)
	}
}

func TestGrantFormulaAndFilterRemainInTheSameAlternative(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$sm#idShort")
	view := SemanticAccessView{
		resource: SemanticResourceSM,
		decision: AccessViewRestricted,
		alternatives: []CompiledGrantAlternative{
			{
				formula: boolExpression(true),
				filters: FragmentFilters{
					"$sm#idShort": NewFragmentFilterPredicate(boolExpression(false), false),
				},
			},
			{formula: boolExpression(false)},
		},
	}
	guard, _, guarded, err := compileConditionVisibilityGuard(view, SemanticAccessTarget{
		Resource: SemanticResourceSM,
		Field:    field,
	})
	if err != nil {
		t.Fatalf("compile alternative guard: %v", err)
	}
	if !guarded {
		t.Fatal("expected restricted alternative guard")
	}
	sql, _, err := goqu.Dialect("postgres").From("submodel").Where(guard).ToSQL()
	if err != nil {
		t.Fatalf("render alternative guard: %v", err)
	}
	if !strings.Contains(sql, "TRUE::boolean AND FALSE::boolean") ||
		!strings.Contains(sql, "OR FALSE::boolean") {
		t.Fatalf("formula and filter were not preserved as complete alternatives:\n%s", sql)
	}
}

func TestUnrestrictedAlternativeIsNotNarrowedByRestrictedAlternative(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$sm#idShort")
	view := SemanticAccessView{
		resource: SemanticResourceSM,
		decision: AccessViewRestricted,
		alternatives: []CompiledGrantAlternative{
			{formula: boolExpression(true)},
			{
				formula: boolExpression(true),
				filters: FragmentFilters{
					"$sm#idShort": NewFragmentFilterPredicate(boolExpression(false), false),
				},
			},
		},
	}
	guard, _, _, err := compileConditionVisibilityGuard(view, SemanticAccessTarget{
		Resource: SemanticResourceSM,
		Field:    field,
	})
	if err != nil {
		t.Fatalf("compile alternative union: %v", err)
	}
	sql, _, err := goqu.Dialect("postgres").From("submodel").Where(guard).ToSQL()
	if err != nil {
		t.Fatalf("render alternative union: %v", err)
	}
	if !strings.Contains(sql, "TRUE::boolean OR") ||
		!strings.Contains(sql, "TRUE::boolean AND FALSE::boolean") {
		t.Fatalf("restricted alternative narrowed an unrestricted grant:\n%s", sql)
	}
}

func TestNegatedRestrictedRelatedConditionRequiresVisibleWitness(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$sm#idShort")
	allowed := grammar.StandardString("allowed")
	requested := grammar.StandardString("requested")
	grant := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &allowed}}}
	leaf := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &requested}}}
	caller := grammar.LogicalExpression{Not: &leaf}
	ctx := context.WithValue(t.Context(), authorizedQueryContextKey{}, &AuthorizedQuery{
		caller: grammar.Query{Condition: &caller},
		outer:  SemanticAccessView{resource: SemanticResourceAAS, decision: AccessViewUnrestricted},
		relatedViews: map[SemanticResourceKind]SemanticAccessView{
			SemanticResourceSM: {
				resource: SemanticResourceSM,
				decision: AccessViewRestricted,
				queryFilter: &QueryFilter{
					Formula: &grant,
				},
			},
		},
	})
	ctx = grammar.ContextWithAASHierarchyQueries(ctx)
	sql := buildAuthorizedAASSelectionSQL(ctx, t)
	if strings.Count(sql, "EXISTS") != 2 || !strings.Contains(sql, "AND NOT (EXISTS") {
		t.Fatalf("negated related predicate must require a visible witness outside negation:\n%s", sql)
	}
}

func TestReferableGrantBuildsSegmentAwareSMESubtreeView(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	router.Get("/submodels", func(http.ResponseWriter, *http.Request) {})
	model, err := ParseAccessModel([]byte(`{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [{"name":"role","attributes":[{"CLAIM":"role"}]}],
			"DEFOBJECTS": [{"name":"motor","objects":[{"REFERABLE":"$sme(\"urn:sm:motor\").Metrics"}]}],
			"DEFACLS": [{"name":"read","acl":{"USEATTRIBUTES":"role","RIGHTS":["READ"],"ACCESS":"ALLOW"}}],
			"DEFFORMULAS": [{"name":"viewer","formula":{"$eq":[{"$attribute":{"CLAIM":"role"}},{"$strVal":"viewer"}]}}],
			"rules": [{"USEACL":"read","USEOBJECTS":["motor"],"USEFORMULA":"viewer"}]
		}
	}`), router, "")
	if err != nil {
		t.Fatalf("parse access model: %v", err)
	}
	session := newAuthorizationSession(
		model,
		Claims{"role": "viewer"},
		nil,
		grammar.DefaultSimplifyOptions(),
	)
	view := session.semanticView(SemanticResourceSME)
	if view.Decision() == AccessViewDenied {
		t.Fatal("REFERABLE read grant did not produce an SME access view")
	}

	field := grammar.ModelStringPattern("$sme#idShort")
	requested := grammar.StandardString("Temperature")
	caller := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &requested}}}
	ctx := context.WithValue(t.Context(), authorizedQueryContextKey{}, &AuthorizedQuery{
		caller: grammar.Query{Condition: &caller},
		outer:  SemanticAccessView{resource: SemanticResourceAAS, decision: AccessViewUnrestricted},
		relatedViews: map[SemanticResourceKind]SemanticAccessView{
			SemanticResourceSME: view,
		},
	})
	ctx = grammar.ContextWithAASHierarchyQueries(ctx)
	sql := buildAuthorizedAASSelectionSQL(ctx, t)
	if !strings.Contains(sql, `"submodel_element"."idshort_path" LIKE`) ||
		!strings.Contains(sql, `"submodel"."submodel_identifier"`) {
		t.Fatalf("REFERABLE view must bind the submodel and segment-bounded SME subtree:\n%s", sql)
	}
}

func TestSubmodelQueryBuildsRelatedSMEAccessView(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$sme#idShort")
	value := grammar.StandardString("Temperature")
	query := grammar.Query{Condition: &grammar.LogicalExpression{Eq: grammar.ComparisonItems{
		{Field: &field},
		{StrVal: &value},
	}}}
	related := queryRelatedResources(query, SemanticResourceSM)
	if len(related) != 1 || related[0] != SemanticResourceSME {
		t.Fatalf("submodel query did not request its SME access view: %#v", related)
	}
}

func TestAuthorizedQueryNormalizesEnumLiteralWithoutMutatingInput(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$sm#semanticId.type")
	value := grammar.StandardString("ExternalReference")
	query := grammar.Query{Condition: &grammar.LogicalExpression{Eq: grammar.ComparisonItems{
		{Field: &field},
		{StrVal: &value},
	}}}
	ctx, err := WithAuthorizedQuery(t.Context(), SemanticResourceSM, query)
	if err != nil {
		t.Fatalf("create authorized query: %v", err)
	}
	authorized := AuthorizedQueryFromContext(ctx)
	if authorized == nil || authorized.callerForBackend().Condition == nil {
		t.Fatal("authorized query did not retain the caller condition")
	}
	backendCondition := authorized.callerForBackend().Condition
	if backendCondition.Eq[1].NumVal == nil || backendCondition.Eq[1].StrVal != nil {
		t.Fatalf("authorized query did not normalize the enum literal: %#v", backendCondition.Eq[1])
	}
	if query.Condition.Eq[1].StrVal == nil || query.Condition.Eq[1].NumVal != nil {
		t.Fatalf("authorized query normalization mutated its input: %#v", query.Condition.Eq[1])
	}
}

func TestCallerSMEConditionUsesCorrelatedAliasInSMERowQuery(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$sme#idShort")
	value := grammar.StandardString("InstanceId")
	condition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &value}}}
	ctx := context.WithValue(t.Context(), authorizedQueryContextKey{}, &AuthorizedQuery{
		caller: grammar.Query{Condition: &condition},
		outer:  SemanticAccessView{resource: SemanticResourceSM, decision: AccessViewUnrestricted},
		relatedViews: map[SemanticResourceKind]SemanticAccessView{
			SemanticResourceSME: {resource: SemanticResourceSME, decision: AccessViewUnrestricted},
		},
	})
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if err != nil {
		t.Fatalf("create SME collector: %v", err)
	}
	dataset := goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.id"))
	secured, err := AddFormulaQueryFromContext(ctx, dataset, collector)
	if err != nil {
		t.Fatalf("add authorized SME condition: %v", err)
	}
	sql, _, err := secured.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("render authorized SME query: %v", err)
	}
	if !strings.Contains(sql, "EXISTS") ||
		!strings.Contains(sql, `"submodel_element"."submodel_id" = "sme"."submodel_id"`) {
		t.Fatalf("caller SME condition was not safely correlated to the SME row alias:\n%s", sql)
	}
}

func TestSemanticReadIndexRecognizesQueryOnlySubmodelRoute(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	router.Post("/query/submodels", func(http.ResponseWriter, *http.Request) {})
	model, err := ParseAccessModel([]byte(`{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [{"name":"role","attributes":[{"CLAIM":"role"}]}],
			"DEFOBJECTS": [{"name":"query","objects":[{"ROUTE":"/query/submodels"}]}],
			"DEFACLS": [{"name":"read","acl":{"USEATTRIBUTES":"role","RIGHTS":["READ"],"ACCESS":"ALLOW"}}],
			"DEFFORMULAS": [{"name":"viewer","formula":{"$eq":[{"$attribute":{"CLAIM":"role"}},{"$strVal":"viewer"}]}}],
			"rules": [{"USEACL":"read","USEOBJECTS":["query"],"USEFORMULA":"viewer"}]
		}
	}`), router, "")
	if err != nil {
		t.Fatalf("parse access model: %v", err)
	}
	session := newAuthorizationSession(model, Claims{"role": "viewer"}, nil, grammar.DefaultSimplifyOptions())
	if session.semanticView(SemanticResourceSM).Decision() == AccessViewDenied {
		t.Fatal("query-only Submodel route was absent from the semantic READ index")
	}
	if session.semanticView(SemanticResourceSME).Decision() == AccessViewDenied {
		t.Fatal("query-only Submodel route did not cover its SME condition view")
	}
}

func TestReferableSubtreeCoverageUsesSegmentBoundaries(t *testing.T) {
	t.Parallel()

	if !submodelElementPathCovered("Metrics", "Metrics.Temperature") {
		t.Fatal("expected child path to be covered")
	}
	if !submodelElementPathCovered("Measurements", "Measurements[0].Value") {
		t.Fatal("expected indexed child path to be covered")
	}
	if submodelElementPathCovered("Metrics", "MetricsPrivate.Temperature") {
		t.Fatal("string prefix without an SME segment boundary widened access")
	}
}

func TestAuthorizedQueryCompositionRetainsEarlierSemanticRoot(t *testing.T) {
	t.Parallel()

	bdField := grammar.ModelStringPattern("$bd#specificAssetIds[].value")
	bdValue := grammar.StandardString("asset-link")
	first, err := WithAuthorizedQuery(t.Context(), SemanticResourceBD, grammar.Query{
		Condition: &grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &bdField}, {StrVal: &bdValue}}},
	})
	if err != nil {
		t.Fatalf("build first authorized query: %v", err)
	}
	descriptorField := grammar.ModelStringPattern("$aasdesc#assetKind")
	descriptorValue := grammar.StandardString("Instance")
	second, err := WithAuthorizedQuery(first, SemanticResourceAASDesc, grammar.Query{
		Condition: &grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &descriptorField}, {StrVal: &descriptorValue}}},
	})
	if err != nil {
		t.Fatalf("compose authorized query: %v", err)
	}
	authorized := AuthorizedQueryFromContext(second)
	if _, found := authorized.accessView(SemanticResourceBD); !found {
		t.Fatal("composing a descriptor selector discarded the earlier basic-discovery access view")
	}
}

func TestGenericSMEFragmentMaskProtectsPathSpecificCallerField(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$sme.Metrics.Temperature#value")
	value := grammar.StandardString("secret")
	target := SemanticAccessTarget{Resource: SemanticResourceSME, Field: field}
	view := SemanticAccessView{
		resource: SemanticResourceSME,
		decision: AccessViewRestricted,
		queryFilter: &QueryFilter{Filters: FragmentFilters{
			"$sme#value": NewFragmentFilterPredicate(boolExpression(false), false),
		}},
	}
	guard, _, guarded, err := compileConditionVisibilityGuard(view, target)
	if err != nil {
		t.Fatalf("compile visibility guard: %v", err)
	}
	if !guarded {
		t.Fatal("generic SME value mask did not cover a path-specific SME value")
	}
	sql, _, err := goqu.Dialect("postgres").From("submodel_element").Where(
		goqu.And(guard, goqu.V(string(value)).Eq(string(value))),
	).Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("render visibility guard: %v", err)
	}
	if !strings.Contains(sql, "false") && !strings.Contains(sql, "$1") {
		t.Fatalf("expected denying visibility guard, got:\n%s", sql)
	}
}

func TestStructuralSMEMaskProtectsDescendantCallerField(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$sme.NewTestList[0]#value")
	target := SemanticAccessTarget{Resource: SemanticResourceSME, Field: field}
	view := SemanticAccessView{
		resource: SemanticResourceSME,
		decision: AccessViewRestricted,
		queryFilter: &QueryFilter{Filters: FragmentFilters{
			"$sme.NewTestList": NewFragmentFilterPredicate(boolExpression(false), false),
		}},
	}
	guard, _, guarded, err := compileConditionVisibilityGuard(view, target)
	if err != nil {
		t.Fatalf("compile structural SME visibility guard: %v", err)
	}
	if !guarded {
		t.Fatal("structural SME mask did not cover a descendant list item")
	}
	sql, _, err := goqu.Dialect("postgres").From("submodel_element").Where(guard).ToSQL()
	if err != nil {
		t.Fatalf("render structural SME visibility guard: %v", err)
	}
	if !strings.Contains(strings.ToLower(sql), "false") {
		t.Fatalf("expected denying structural visibility guard, got:\n%s", sql)
	}
}

func TestSMEArrayMaskOverlapRespectsFixedAndWildcardIndexes(t *testing.T) {
	t.Parallel()

	wildcardFilter := grammar.FragmentStringPattern("$sme.List[]#value")
	fixedZeroField := grammar.FragmentStringPattern("$sme.List[0]#value")
	if !fragmentAffectsSemanticField(wildcardFilter, fixedZeroField) {
		t.Fatal("wildcard SME mask did not protect a fixed list occurrence")
	}

	fixedZeroFilter := grammar.FragmentStringPattern("$sme.List[0]#value")
	wildcardField := grammar.FragmentStringPattern("$sme.List[]#value")
	if !fragmentAffectsSemanticField(fixedZeroFilter, wildcardField) {
		t.Fatal("fixed SME mask did not protect its occurrence in a wildcard field read")
	}

	fixedOneField := grammar.FragmentStringPattern("$sme.List[1]#value")
	if fragmentAffectsSemanticField(fixedZeroFilter, fixedOneField) {
		t.Fatal("fixed SME mask widened to a different list occurrence")
	}
}

func TestSemanticRouteIndexRejectsSubmodelPrefixCollision(t *testing.T) {
	t.Parallel()

	coverage, covered := semanticSubmodelRouteCoverage("/submodels-private", SemanticResourceSM, "")
	if covered || coverage.resource != "" {
		t.Fatal("non-segment route prefix was classified as Submodel READ coverage")
	}
}

func TestMetadataOnlyRouteDoesNotGrantSMEConditionAccess(t *testing.T) {
	t.Parallel()

	if _, covered := semanticSubmodelRouteCoverage("/submodels/$metadata", SemanticResourceSME, ""); covered {
		t.Fatal("Submodel metadata collection route granted access to SME conditions")
	}
	encodedID := "dXJuOnNtOm1ldGFkYXRh"
	if _, covered := semanticSubmodelRouteCoverage("/submodels/"+encodedID+"/$metadata", SemanticResourceSME, ""); covered {
		t.Fatal("Submodel metadata item route granted access to SME conditions")
	}
}

func TestAuthorizationSessionPinsClaimsGlobalsAndPolicy(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	router.Get("/submodels", func(http.ResponseWriter, *http.Request) {})
	model, err := ParseAccessModel([]byte(`{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [{"name":"role","attributes":[{"CLAIM":"role"}]}],
			"DEFOBJECTS": [{"name":"submodels","objects":[{"ROUTE":"/submodels"}]}],
			"DEFACLS": [{"name":"read","acl":{"USEATTRIBUTES":"role","RIGHTS":["READ"],"ACCESS":"ALLOW"}}],
			"DEFFORMULAS": [{"name":"viewer","formula":{"$eq":[{"$attribute":{"CLAIM":"role"}},{"$strVal":"viewer"}]}}],
			"rules": [{"USEACL":"read","USEOBJECTS":["submodels"],"USEFORMULA":"viewer"}]
		}
	}`), router, "")
	if err != nil {
		t.Fatalf("parse access model: %v", err)
	}
	model.WithPolicyID("policy-a")
	claims := Claims{"role": "viewer", "nested": map[string]any{"value": "original"}}
	session := newAuthorizationSession(model, claims, nil, grammar.DefaultSimplifyOptions())
	claims["role"] = "denied"
	claims["nested"].(map[string]any)["value"] = "mutated"
	model.WithPolicyID("policy-b")

	view := session.semanticView(SemanticResourceSM)
	if view.Decision() == AccessViewDenied {
		t.Fatal("session observed claims mutated after request authorization")
	}
	if session.PolicyID() != "policy-a" {
		t.Fatalf("session policy changed after publication: %q", session.PolicyID())
	}
	if len(view.alternatives) != 1 {
		t.Fatalf("expected one complete grant alternative, got %d", len(view.alternatives))
	}
}

func TestAuthorizedQueryDeepCopiesCallerLiteralPointers(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$aas#idShort")
	value := grammar.StandardString("original")
	condition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &value}}}
	ctx := mustAuthorizedQueryContext(t.Context(), t, grammar.Query{Condition: &condition})

	field = "$aas#id"
	value = "mutated"
	authorized := AuthorizedQueryFromContext(ctx)
	if got := *authorized.caller.Condition.Eq[0].Field; got != "$aas#idShort" {
		t.Fatalf("authorized query observed caller field mutation: %q", got)
	}
	if got := *authorized.caller.Condition.Eq[1].StrVal; got != "original" {
		t.Fatalf("authorized query observed caller literal mutation: %q", got)
	}
}

func TestNegatedCallerConditionCannotMatchSecurityHiddenField(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$aas#idShort")
	secret := grammar.StandardString("secret")
	leaf := grammar.LogicalExpression{Eq: grammar.ComparisonItems{
		{Field: &field},
		{StrVal: &secret},
	}}
	condition := grammar.LogicalExpression{Not: &leaf}
	ctx := WithQueryFilter(t.Context(), queryFilterHidingField("$aas#idShort"))
	ctx = mustAuthorizedQueryContext(ctx, t, grammar.Query{Condition: &condition})

	sql := buildAuthorizedAASSelectionSQL(ctx, t)
	if !strings.Contains(sql, "AND NOT") || strings.Count(sql, "::boolean") < 2 {
		t.Fatalf("trusted visibility must stay outside caller negation:\n%s", sql)
	}
}

func TestCallerResponseFilterConditionCannotReadAnotherHiddenField(t *testing.T) {
	t.Parallel()

	hiddenField := grammar.ModelStringPattern("$aas#idShort")
	secret := grammar.StandardString("secret")
	condition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{
		{Field: &hiddenField},
		{StrVal: &secret},
	}}
	responseFragment := grammar.FragmentStringPattern("$aas#assetInformation.globalAssetId")
	ctx := WithQueryFilter(t.Context(), queryFilterHidingField("$aas#idShort"))
	ctx = mustAuthorizedQueryContext(ctx, t, grammar.Query{FilterConditions: []grammar.SubFilter{{
		Condition: &condition,
		Fragment:  &responseFragment,
	}}})

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAAS)
	if err != nil {
		t.Fatalf("create collector: %v", err)
	}
	mask, hasMask, err := buildFragmentMaskCondition(ctx, responseFragment, collector)
	if err != nil {
		t.Fatalf("build response filter: %v", err)
	}
	if !hasMask {
		t.Fatal("expected caller response filter mask")
	}
	sql, _, err := goqu.Dialect("postgres").From("aas").Where(mask).ToSQL()
	if err != nil {
		t.Fatalf("render response filter: %v", err)
	}
	if !strings.Contains(sql, "AND") || !strings.Contains(sql, `"aas"."id_short"`) {
		t.Fatalf("caller response-filter condition omitted the hidden-field guard:\n%s", sql)
	}
}

func TestCallerMatchResponseFilterRetainsHiddenFieldGuard(t *testing.T) {
	t.Parallel()

	hiddenField := grammar.ModelStringPattern("$aas#assetInformation.specificAssetIds[].value")
	secret := grammar.StandardString("secret")
	condition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{
		{Field: &hiddenField},
		{StrVal: &secret},
	}}
	responseFragment := grammar.FragmentStringPattern("$aas#assetInformation.specificAssetIds[]")
	match := true
	ctx := WithQueryFilter(t.Context(), queryFilterHidingField(responseFragment))
	ctx = mustAuthorizedQueryContext(ctx, t, grammar.Query{FilterConditions: []grammar.SubFilter{{
		Condition: &condition,
		Fragment:  &responseFragment,
		Match:     &match,
	}}})

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAAS)
	if err != nil {
		t.Fatalf("create collector: %v", err)
	}
	mask, hasMask, err := buildFragmentMaskCondition(ctx, responseFragment, collector)
	if err != nil {
		t.Fatalf("build matched response filter: %v", err)
	}
	if !hasMask {
		t.Fatal("expected matched caller response filter mask")
	}
	sql, _, err := goqu.Dialect("postgres").From("aas").Where(mask).ToSQL()
	if err != nil {
		t.Fatalf("render matched response filter: %v", err)
	}
	if !strings.Contains(strings.ToLower(sql), "false") || !strings.Contains(sql, `"specific_asset_id"."value"`) {
		t.Fatalf("matched caller response filter omitted its hidden-field guard:\n%s", sql)
	}
}

func TestCallerMatchResponseFilterKeepsCurrentSMERowCorrelation(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$sme.a[].b[]#value")
	value := grammar.StandardString("allowed")
	condition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{
		{Field: &field},
		{StrVal: &value},
	}}
	fragment := grammar.FragmentStringPattern("$sme.a[].b[]#idShort")
	match := true
	ctx, err := WithAuthorizedQuery(t.Context(), SemanticResourceSM, grammar.Query{
		FilterConditions: []grammar.SubFilter{{
			Condition: &condition,
			Fragment:  &fragment,
			Match:     &match,
		}},
	})
	if err != nil {
		t.Fatalf("create authorized query: %v", err)
	}
	collector, err := grammar.NewResolvedFieldPathCollectorForSMERow("sme")
	if err != nil {
		t.Fatalf("create SME row collector: %v", err)
	}
	mask, hasMask, err := buildFragmentMaskCondition(ctx, fragment, collector)
	if err != nil {
		t.Fatalf("build matched response filter: %v", err)
	}
	if !hasMask {
		t.Fatal("expected matched caller response filter mask")
	}
	sql, _, err := goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("sme")).
		Where(mask).
		ToSQL()
	if err != nil {
		t.Fatalf("render matched response filter: %v", err)
	}
	if !strings.Contains(sql, `"submodel_element"."id" = "sme"."id"`) ||
		!strings.Contains(sql, `"property_element"`) {
		t.Fatalf("caller match lost current-row correlation:\n%s", sql)
	}
}

func TestParentArrayMaskProtectsCallerReadOfChildField(t *testing.T) {
	t.Parallel()

	field := grammar.ModelStringPattern("$aas#assetInformation.specificAssetIds[].value")
	value := grammar.StandardString("secret")
	condition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &value}}}
	ctx := WithQueryFilter(t.Context(), queryFilterHidingField("$aas#assetInformation.specificAssetIds[]"))
	ctx = mustAuthorizedQueryContext(ctx, t, grammar.Query{Condition: &condition})

	sql := buildAuthorizedAASSelectionSQL(ctx, t)
	if !strings.Contains(sql, "::boolean") || !strings.Contains(sql, "specific_asset_id") {
		t.Fatalf("parent fragment mask did not protect its child field:\n%s", sql)
	}
}

func queryFilterHidingField(fragment grammar.FragmentStringPattern) *QueryFilter {
	allow := boolExpression(true)
	deny := boolExpression(false)
	return &QueryFilter{
		Formula: &allow,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumREAD: allow,
		},
		Filters: FragmentFilters{
			fragment: NewFragmentFilterPredicate(deny, false),
		},
	}
}

func buildAuthorizedAASSelectionSQL(ctx context.Context, t *testing.T) string {
	t.Helper()
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAAS)
	if err != nil {
		t.Fatalf("create collector: %v", err)
	}
	ds := goqu.Dialect("postgres").From(goqu.T("aas").As("aas")).Select(goqu.I("aas.id"))
	secured, err := AddFormulaQueryFromContext(ctx, ds, collector)
	if err != nil {
		t.Fatalf("add authorized condition: %v", err)
	}
	sql, _, err := secured.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("render authorized query: %v", err)
	}
	return sql
}

func mustAuthorizedQueryContext(ctx context.Context, t *testing.T, query grammar.Query) context.Context {
	t.Helper()
	authorized, err := WithAuthorizedQuery(ctx, SemanticResourceAAS, query)
	if err != nil {
		t.Fatalf("create authorized query: %v", err)
	}
	return authorized
}
