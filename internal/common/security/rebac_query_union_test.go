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
// Author: BaSyx Authors

package auth

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestUnionAuthorizationEvaluationWithReBACReadGrantsUnionsFormulaAndFragments(t *testing.T) {
	abacField := grammar.ModelStringPattern("$sm#id")
	abacValue := grammar.StandardString("abac-only")
	abacFormula := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &abacField}, {StrVal: &abacValue}}}
	fragment := grammar.FragmentStringPattern("$sm#idShort")
	evaluation := AuthorizationEvaluation{
		Allowed: true,
		Reason:  DecisionAllow,
		QueryFilter: &QueryFilter{
			Formula: &abacFormula,
			Filters: FragmentFilters{fragment: NewFragmentFilterPredicate(abacFormula, false)},
		},
	}

	result, err := UnionAuthorizationEvaluationWithReBACReadGrants(SemanticResourceSM, evaluation, ReBACReadGrantSet{Identifiers: []string{"rebac-one", "rebac-two"}})
	if err != nil {
		t.Fatalf("union error: %v", err)
	}
	if !result.Allowed || result.QueryFilter == nil || result.QueryFilter.Formula == nil {
		t.Fatalf("grant union did not produce an allowed filter: %#v", result)
	}
	if len(result.QueryFilter.Formula.Or) != 2 {
		t.Fatalf("formula is not ABAC OR ReBAC: %#v", result.QueryFilter.Formula)
	}
	if got := result.QueryFilter.Filters[fragment]; len(got.Or) != 2 {
		t.Fatalf("fragment filter did not retain ABAC OR grant branches: %#v", got)
	}
	if len(result.alternatives) != 1 || result.alternatives[0].ruleID != "rebac-read-grant" {
		t.Fatalf("missing full-field ReBAC witness: %#v", result.alternatives)
	}
	if evaluation.QueryFilter.Formula != &abacFormula || len(evaluation.QueryFilter.Filters[fragment].Or) != 0 {
		t.Fatal("union mutated the existing ABAC evaluation")
	}
}

func TestUnionSemanticAccessViewWithReBACReadGrantsPreservesABACAlternative(t *testing.T) {
	field := grammar.ModelStringPattern("$sm#id")
	value := grammar.StandardString("abac-only")
	abacFormula := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &value}}}
	view := SemanticAccessView{
		resource:     SemanticResourceSM,
		decision:     AccessViewRestricted,
		queryFilter:  &QueryFilter{Formula: &abacFormula},
		alternatives: []CompiledGrantAlternative{{ruleID: "abac-rule", formula: abacFormula}},
	}

	result, err := UnionSemanticAccessViewWithReBACReadGrants(view, ReBACReadGrantSet{Identifiers: []string{"rebac-only"}})
	if err != nil {
		t.Fatalf("union error: %v", err)
	}
	if result.decision != AccessViewRestricted || len(result.alternatives) != 2 {
		t.Fatalf("semantic view lost its ABAC witness or grant witness: %#v", result)
	}
	if result.alternatives[0].ruleID != "abac-rule" || result.alternatives[1].ruleID != "rebac-read-grant" {
		t.Fatalf("unexpected alternatives: %#v", result.alternatives)
	}
}

func TestUnionReBACElementReadGrantsUsesRowScopedFormula(t *testing.T) {
	visibleField := grammar.ModelStringPattern("$sme.visible#idShort")
	visibleValue := grammar.StandardString("visible")
	visibleOnly := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &visibleField}, {StrVal: &visibleValue}}}
	evaluation := AuthorizationEvaluation{Allowed: true, Reason: DecisionAllow, QueryFilter: &QueryFilter{Formula: &visibleOnly}}
	result, err := UnionAuthorizationEvaluationWithReBACReadGrants(
		SemanticResourceSME,
		evaluation,
		ReBACReadGrantSet{Elements: []ReBACElementReadGrant{{SubmodelID: "sm-one", ElementPath: "component.temperature"}}},
	)
	if err != nil {
		t.Fatalf("SME union error = %v", err)
	}
	if result.QueryFilter == nil || result.QueryFilter.SMERowFormula == nil {
		t.Fatalf("missing row-correlated SME grant: %#v", result)
	}
	if !reflect.DeepEqual(result.QueryFilter.Formula, &visibleOnly) {
		t.Fatalf("SME union changed the existing ABAC formula: %#v", result)
	}
}

func TestUnionReBACReadGrantsRejectsUnsupportedSMEIdentity(t *testing.T) {
	_, err := UnionAuthorizationEvaluationWithReBACReadGrants(
		SemanticResourceSME,
		AuthorizationEvaluation{},
		ReBACReadGrantSet{Identifiers: []string{"sm-one"}},
	)
	if !errors.Is(err, ErrReBACReadGrantInvalid) {
		t.Fatalf("SME identity union error = %v", err)
	}
}

func TestSMEReadGrantSQLBindsTheCurrentVisibleElementRow(t *testing.T) {
	evaluation := AuthorizationEvaluation{Allowed: false}
	result, err := UnionAuthorizationEvaluationWithReBACReadGrants(
		SemanticResourceSME,
		evaluation,
		ReBACReadGrantSet{Elements: []ReBACElementReadGrant{{SubmodelID: "sm-one", ElementPath: "visible.temperature"}}},
	)
	if err != nil {
		t.Fatalf("union error: %v", err)
	}
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if err != nil {
		t.Fatalf("collector error: %v", err)
	}
	dataset := goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.idshort_path"))
	dataset, err = AddSMEFormulaQueryFromContext(WithQueryFilter(t.Context(), result.QueryFilter), dataset, collector, "sme")
	if err != nil {
		t.Fatalf("row formula error: %v", err)
	}
	sqlQuery, args, err := dataset.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("SQL build error: %v", err)
	}
	if !strings.Contains(sqlQuery, `"submodel_element"."id" = "sme"."id"`) {
		t.Fatalf("grant is not correlated to the selected SME row: %s", sqlQuery)
	}
	if strings.Contains(strings.Join(stringifyReBACQueryArgs(args), ","), "hidden.sibling") {
		t.Fatalf("hidden sibling was included in the row grant: %v", args)
	}
	if !strings.Contains(strings.Join(stringifyReBACQueryArgs(args), ","), "visible.temperature") {
		t.Fatalf("visible element path is absent from the grant SQL: %v", args)
	}
}

func stringifyReBACQueryArgs(values []any) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = fmt.Sprint(value)
	}
	return result
}

func TestReBACReadGrantFormulaUsesTheResourceIdentifierField(t *testing.T) {
	tests := []struct {
		resource SemanticResourceKind
		field    grammar.ModelStringPattern
	}{
		{SemanticResourceAAS, "$aas#id"},
		{SemanticResourceSM, "$sm#id"},
		{SemanticResourceAASDesc, "$aasdesc#id"},
		{SemanticResourceCD, "$cd#id"},
	}
	for _, testCase := range tests {
		t.Run(string(testCase.resource), func(t *testing.T) {
			formula, _, ok, err := rebacReadGrantFormula(testCase.resource, ReBACReadGrantSet{Identifiers: []string{"granted"}})
			if err != nil || !ok || string(*formula.Eq[0].Field) != string(testCase.field) {
				t.Fatalf("grant formula = %#v, %t, %v", formula, ok, err)
			}
		})
	}
}

func TestUnionReBACSubmodelDescriptorReadGrantsSeparateStandaloneAndEmbeddedScopes(t *testing.T) {
	result, err := UnionAuthorizationEvaluationWithReBACReadGrants(
		SemanticResourceSMDesc,
		AuthorizationEvaluation{},
		ReBACReadGrantSet{
			Identifiers: []string{"standalone"},
			EmbeddedSubmodelDescriptors: []ReBACEmbeddedSubmodelDescriptorReadGrant{{
				AASDescriptorID: "aas-one",
				SubmodelID:      "embedded-sm",
			}},
		},
	)
	if err != nil {
		t.Fatalf("submodel descriptor union error = %v", err)
	}
	if result.QueryFilter == nil || result.QueryFilter.SMDescStandaloneFormula == nil || result.QueryFilter.SMDescEmbeddedFormula == nil {
		t.Fatalf("missing descriptor grant scope formulas: %#v", result)
	}
	standaloneSQL := rebacSubmodelDescriptorScopeSQL(t, result.QueryFilter, false)
	if !strings.Contains(standaloneSQL, "standalone") || strings.Contains(standaloneSQL, "aas-one") || strings.Contains(standaloneSQL, "embedded-sm") {
		t.Fatalf("standalone descriptor query cross-applied embedded grants: %s", standaloneSQL)
	}
	embeddedSQL := rebacSubmodelDescriptorScopeSQL(t, result.QueryFilter, true)
	if !strings.Contains(embeddedSQL, "aas-one") || !strings.Contains(embeddedSQL, "embedded-sm") || strings.Contains(embeddedSQL, "standalone") {
		t.Fatalf("embedded descriptor query cross-applied standalone grants: %s", embeddedSQL)
	}
}

func rebacSubmodelDescriptorScopeSQL(t *testing.T, filter *QueryFilter, embedded bool) string {
	t.Helper()
	var collector *grammar.ResolvedFieldPathCollector
	var err error
	if embedded {
		collector, err = grammar.NewResolvedFieldPathCollectorForNestedSMDesc()
	} else {
		collector, err = grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSMDesc)
	}
	if err != nil {
		t.Fatalf("collector error: %v", err)
	}
	dataset := goqu.Dialect("postgres").
		From(goqu.T("submodel_descriptor").As("submodel_descriptor")).
		Select(goqu.I("submodel_descriptor.id"))
	dataset, err = AddSubmodelDescriptorFormulaQueryFromContext(WithQueryFilter(t.Context(), filter), dataset, collector, embedded)
	if err != nil {
		t.Fatalf("scope query error: %v", err)
	}
	sqlQuery, args, err := dataset.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("scope SQL error: %v", err)
	}
	return sqlQuery + " " + strings.Join(stringifyReBACQueryArgs(args), ",")
}

func TestUnionReBACDiscoveryReadGrantsUsesDiscoveryRowAASID(t *testing.T) {
	result, err := UnionAuthorizationEvaluationWithReBACReadGrants(
		SemanticResourceBD,
		AuthorizationEvaluation{},
		ReBACReadGrantSet{Identifiers: []string{"aas-id"}},
	)
	if err != nil {
		t.Fatalf("discovery union error = %v", err)
	}
	if result.QueryFilter == nil || result.QueryFilter.Formula == nil || string(*result.QueryFilter.Formula.Eq[0].Field) != "$bd#aasId" {
		t.Fatalf("discovery grant is not bound to the discovery row AAS ID: %#v", result)
	}
}

func TestDescriptorGrantDoesNotAuthorizeOppositeRepresentation(t *testing.T) {
	for _, test := range []struct {
		name     string
		grants   ReBACReadGrantSet
		embedded bool
	}{
		{"standalone cannot expose embedded", ReBACReadGrantSet{Identifiers: []string{"shared-id"}}, true},
		{"embedded cannot expose standalone", ReBACReadGrantSet{EmbeddedSubmodelDescriptors: []ReBACEmbeddedSubmodelDescriptorReadGrant{{AASDescriptorID: "aas", SubmodelID: "shared-id"}}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			evaluation, err := UnionAuthorizationEvaluationWithReBACReadGrants(SemanticResourceSMDesc, AuthorizationEvaluation{}, test.grants)
			if err != nil {
				t.Fatal(err)
			}
			query := rebacSubmodelDescriptorScopeSQL(t, evaluation.QueryFilter, test.embedded)
			if !strings.Contains(strings.ToUpper(query), "FALSE") || strings.Contains(query, "shared-id") {
				t.Fatalf("opposite representation was not denied: %s", query)
			}
			if len(evaluation.QueryFilter.FormulasByRight) == 0 {
				t.Fatal("row filter would not be enforced")
			}
		})
	}
}
