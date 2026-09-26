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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

const (
	grantedAASUUID      = "0b6c5a3e-6a8f-4b64-9a0c-0d1d4f2a1e01"
	grantedSubmodelUUID = "1c7d6b4f-7b90-4c75-8b1d-1e2e503b2f02"
	grantedCDUUID       = "2d8e7c50-8ca1-4d86-9c2e-2f3f614c3003"
)

func failClosedReadContext(t *testing.T) context.Context {
	t.Helper()
	return WithQueryFilter(t.Context(), failClosedQueryFilter(grammar.RightsEnumREAD))
}

func mustGrantSet(t *testing.T, configure func(*ReBACGrantSet) error, rights ...grammar.RightsEnum) *ReBACGrantSet {
	t.Helper()
	grants := NewReBACGrantSet(rights...)
	if err := configure(grants); err != nil {
		t.Fatalf("configure grant set: %v", err)
	}
	return grants
}

func formulaSQLForRoot(ctx context.Context, t *testing.T, root grammar.CollectorRoot, table string, alias string) string {
	t.Helper()
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(root)
	if err != nil {
		t.Fatalf("create collector: %v", err)
	}
	ds := goqu.Dialect("postgres").From(goqu.T(table).As(alias)).Select(goqu.I(alias + ".id"))
	secured, err := AddFormulaQueryFromContext(ctx, ds, collector)
	if err != nil {
		t.Fatalf("add formula: %v", err)
	}
	sql, _, err := secured.ToSQL()
	if err != nil {
		t.Fatalf("render formula: %v", err)
	}
	return sql
}

func TestReBACGrantWidensOnlyItsOwnResourceKind(t *testing.T) {
	t.Parallel()

	grants := mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowResources(SemanticResourceAAS, []string{grantedAASUUID}, grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
	ctx := WithReBACGrants(failClosedReadContext(t), grants)

	aasSQL := formulaSQLForRoot(ctx, t, grammar.CollectorRootAAS, "aas", "aas")
	if !strings.Contains(aasSQL, "rebac_granted_aas") || !strings.Contains(aasSQL, grantedAASUUID) {
		t.Fatalf("AAS grant must widen the AAS root:\n%s", aasSQL)
	}
	for _, target := range []struct {
		root  grammar.CollectorRoot
		table string
		alias string
	}{
		{grammar.CollectorRootSM, "submodel", "submodel"},
		{grammar.CollectorRootSME, "submodel_element", "sme"},
		{grammar.CollectorRootCD, "concept_description", "concept_description"},
	} {
		sql := formulaSQLForRoot(ctx, t, target.root, target.table, target.alias)
		if strings.Contains(sql, "rebac_granted") || strings.Contains(sql, grantedAASUUID) {
			t.Fatalf("AAS grant leaked into %s root:\n%s", target.root, sql)
		}
	}
}

func TestReBACSubmodelGrantDoesNotWidenAASHierarchyQueries(t *testing.T) {
	t.Parallel()

	grants := mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowResources(SemanticResourceSM, []string{grantedSubmodelUUID}, grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
	ctx := WithReBACGrants(failClosedReadContext(t), grants)
	ctx = grammar.ContextWithAASHierarchyQueries(ctx)

	field := grammar.ModelStringPattern("$sm#idShort")
	value := grammar.StandardString("shared")
	condition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &value}}}
	ctx = mustAuthorizedQueryContext(ctx, t, grammar.Query{Condition: &condition})

	sql := buildAuthorizedAASSelectionSQL(ctx, t)
	if strings.Contains(sql, "rebac_granted") || strings.Contains(sql, grantedSubmodelUUID) {
		t.Fatalf("Submodel grant must not widen an AAS query or its related Submodel view:\n%s", sql)
	}
}

func TestReBACGrantIsBoundToTheSelectedRight(t *testing.T) {
	t.Parallel()

	grants := mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowResources(SemanticResourceSM, []string{grantedSubmodelUUID}, grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD, grammar.RightsEnumUPDATE)
	base := WithQueryFilter(t.Context(), failClosedQueryFilter(grammar.RightsEnumREAD, grammar.RightsEnumUPDATE))
	base = WithReBACGrants(base, grants)

	readSQL := formulaSQLForRoot(SelectFormulaForRight(base, grammar.RightsEnumREAD), t, grammar.CollectorRootSM, "submodel", "submodel")
	if !strings.Contains(readSQL, grantedSubmodelUUID) {
		t.Fatalf("READ grant must apply when READ is selected:\n%s", readSQL)
	}
	updateSQL := formulaSQLForRoot(SelectFormulaForRight(base, grammar.RightsEnumUPDATE), t, grammar.CollectorRootSM, "submodel", "submodel")
	if strings.Contains(updateSQL, grantedSubmodelUUID) {
		t.Fatalf("READ grant must not authorize UPDATE:\n%s", updateSQL)
	}
	unselectedSQL := formulaSQLForRoot(base, t, grammar.CollectorRootSM, "submodel", "submodel")
	if strings.Contains(unselectedSQL, grantedSubmodelUUID) {
		t.Fatalf("grant must cover every default route right when no right is selected:\n%s", unselectedSQL)
	}
}

func TestEmptyOrForeignReBACGrantKeepsSQLByteIdentical(t *testing.T) {
	t.Parallel()

	allow := boolExpression(true)
	field := grammar.ModelStringPattern("$aas#idShort")
	value := grammar.StandardString("visible")
	restricted := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &value}}}
	ctx := WithQueryFilter(t.Context(), &QueryFilter{
		Formula:         &restricted,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{grammar.RightsEnumREAD: restricted},
		Filters:         FragmentFilters{"$aas#description": NewFragmentFilterPredicate(allow, false)},
	})
	baseline := formulaSQLForRoot(ctx, t, grammar.CollectorRootAAS, "aas", "aas")

	emptyCtx := WithReBACGrants(ctx, NewReBACGrantSet(grammar.RightsEnumREAD))
	if got := formulaSQLForRoot(emptyCtx, t, grammar.CollectorRootAAS, "aas", "aas"); got != baseline {
		t.Fatalf("empty grant set changed SQL:\nbaseline: %s\ngot:      %s", baseline, got)
	}
	foreign := mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowResources(SemanticResourceCD, []string{grantedCDUUID}, grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
	foreignCtx := WithReBACGrants(ctx, foreign)
	if got := formulaSQLForRoot(foreignCtx, t, grammar.CollectorRootAAS, "aas", "aas"); got != baseline {
		t.Fatalf("grant for another kind changed SQL:\nbaseline: %s\ngot:      %s", baseline, got)
	}

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAAS)
	if err != nil {
		t.Fatal(err)
	}
	baselineMask, _, err := buildFragmentMaskCondition(ctx, "$aas#description", collector)
	if err != nil {
		t.Fatal(err)
	}
	foreignMask, _, err := buildFragmentMaskCondition(foreignCtx, "$aas#description", collector)
	if err != nil {
		t.Fatal(err)
	}
	if renderExpression(t, baselineMask) != renderExpression(t, foreignMask) {
		t.Fatalf("grant for another kind changed a fragment mask")
	}
}

func TestReBACElementGrantCoversOnlyItsSubtree(t *testing.T) {
	t.Parallel()

	grants := mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowSubmodelElement(grantedSubmodelUUID, "list_a[1]", grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
	ctx := WithReBACGrants(failClosedReadContext(t), grants)

	smeSQL := formulaSQLForRoot(ctx, t, grammar.CollectorRootSME, "submodel_element", "sme")
	for _, fragment := range []string{
		`"sme"."idshort_path" = 'list_a[1]'`,
		`'list!_a[1]' || '.%'`,
		`'list!_a[1]' || '[%]%'`,
		grantedSubmodelUUID,
	} {
		if !strings.Contains(smeSQL, fragment) {
			t.Fatalf("element grant must be segment-bounded and escaped (missing %q):\n%s", fragment, smeSQL)
		}
	}
	if strings.Contains(smeSQL, `'list!_a[1]' || '%'`) {
		t.Fatalf("element grant must not match sibling path prefixes:\n%s", smeSQL)
	}

	submodelSQL := formulaSQLForRoot(ctx, t, grammar.CollectorRootSM, "submodel", "submodel")
	if strings.Contains(submodelSQL, "rebac_granted") {
		t.Fatalf("element grant must not expose the containing Submodel:\n%s", submodelSQL)
	}

	for candidate, covered := range map[string]bool{
		"list_a[1]":       true,
		"list_a[1].child": true,
		"list_a[1][0]":    true,
		"list_a[10]":      false,
		"list_a[1]x":      false,
		"list_a":          false,
		"listXa[1]":       false,
	} {
		if submodelElementPathCovered("list_a[1]", candidate) != covered {
			t.Fatalf("path coverage for %q must be %v", candidate, covered)
		}
	}
}

func TestReBACSubmodelGrantCoversItsElementRows(t *testing.T) {
	t.Parallel()

	grants := mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowResources(SemanticResourceSM, []string{grantedSubmodelUUID}, grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
	ctx := WithReBACGrants(failClosedReadContext(t), grants)

	collector, err := grammar.NewResolvedFieldPathCollectorForSMERow("visible_sme_child")
	if err != nil {
		t.Fatal(err)
	}
	grant, ok := reBACGrantPredicate(ctx, collector)
	if !ok {
		t.Fatal("Submodel grant must cover its SubmodelElement rows")
	}
	sql := renderExpression(t, grant)
	if !strings.Contains(sql, `"visible_sme_child"."submodel_id" IN`) || strings.Contains(sql, "idshort_path") {
		t.Fatalf("Submodel grant must be correlated to the row alias without path restriction:\n%s", sql)
	}
}

func TestReBACGrantLiftsSecurityMasksButNeverCallerFilters(t *testing.T) {
	t.Parallel()

	grants := mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowResources(SemanticResourceAAS, []string{grantedAASUUID}, grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAAS)
	if err != nil {
		t.Fatal(err)
	}

	securityCtx := WithReBACGrants(WithQueryFilter(t.Context(), queryFilterHidingField("$aas#idShort")), grants)
	securityMask, hasMask, err := buildFragmentMaskCondition(securityCtx, "$aas#idShort", collector)
	if err != nil || !hasMask {
		t.Fatalf("expected security mask: %v", err)
	}
	if !strings.Contains(renderExpression(t, securityMask), grantedAASUUID) {
		t.Fatalf("ReBAC grant must lift ABAC fragment masks of the granted resource")
	}

	field := grammar.ModelStringPattern("$aas#idShort")
	value := grammar.StandardString("wanted")
	fragment := grammar.FragmentStringPattern("$aas#idShort")
	callerCondition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &value}}}
	allow := boolExpression(true)
	callerCtx := WithQueryFilter(t.Context(), &QueryFilter{
		Formula:         &allow,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{grammar.RightsEnumREAD: allow},
	})
	callerCtx = WithReBACGrants(callerCtx, grants)
	callerCtx = mustAuthorizedQueryContext(callerCtx, t, grammar.Query{
		FilterConditions: []grammar.SubFilter{{Fragment: &fragment, Condition: &callerCondition}},
	})
	callerMask, hasMask, err := buildFragmentMaskCondition(callerCtx, "$aas#idShort", collector)
	if err != nil || !hasMask {
		t.Fatalf("expected caller projection mask: %v", err)
	}
	if strings.Contains(renderExpression(t, callerMask), grantedAASUUID) {
		t.Fatalf("ReBAC grant must never bypass a caller-requested filter")
	}
}

func TestReBACGrantNeverBypassesCallerConditions(t *testing.T) {
	t.Parallel()

	grants := mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowResources(SemanticResourceAAS, []string{grantedAASUUID}, grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
	field := grammar.ModelStringPattern("$aas#idShort")
	value := grammar.StandardString("caller-selected")
	condition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &value}}}
	ctx := WithReBACGrants(failClosedReadContext(t), grants)
	ctx = mustAuthorizedQueryContext(ctx, t, grammar.Query{Condition: &condition})

	sql, args := buildAuthorizedAASSelectionSQLWithArgs(ctx, t)
	if !containsSQLArgument(args, "{"+grantedAASUUID+"}") || !containsSQLArgument(args, "caller-selected") {
		t.Fatalf("expected ReBAC grant and caller condition:\n%s\n%v", sql, args)
	}
	orIndex := strings.Index(sql, " OR ")
	andIndex := strings.Index(sql, ")))) AND ")
	if orIndex < 0 || andIndex < orIndex {
		t.Fatalf("caller condition must be ANDed outside the widened security condition:\n%s", sql)
	}
}

func TestDeniedOuterViewIsWidenedOnlyByTheGrant(t *testing.T) {
	t.Parallel()

	grants := mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowResources(SemanticResourceAAS, []string{grantedAASUUID}, grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
	denied := context.WithValue(t.Context(), authorizedQueryContextKey{}, &AuthorizedQuery{
		outer: SemanticAccessView{resource: SemanticResourceAAS, decision: AccessViewDenied},
	})
	withoutGrant := buildAuthorizedAASSelectionSQL(denied, t)
	if !strings.Contains(withoutGrant, "FALSE") || strings.Contains(withoutGrant, "rebac_granted") {
		t.Fatalf("denied view without grant must stay FALSE:\n%s", withoutGrant)
	}
	withGrant, args := buildAuthorizedAASSelectionSQLWithArgs(WithReBACGrants(denied, grants), t)
	if !strings.Contains(withGrant, "FALSE OR") || !containsSQLArgument(args, "{"+grantedAASUUID+"}") {
		t.Fatalf("denied view must be widened by the ReBAC grant:\n%s", withGrant)
	}
}

func TestQueriedReBACGrantsStayOnTheirRootAndRight(t *testing.T) {
	t.Parallel()

	granted := goqu.Dialect("postgres").From("granted_aas_marker").Select(goqu.C("object_uuid"))
	grants := mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowQueriedResources(SemanticResourceAAS, granted, grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
	ctx := WithReBACGrants(failClosedReadContext(t), grants)

	aasSQL := formulaSQLForRoot(ctx, t, grammar.CollectorRootAAS, "aas", "aas")
	if !strings.Contains(aasSQL, `"granted_aas_marker"`) || !strings.Contains(aasSQL, `"rebac_granted_aas"."auth_uuid" IN`) {
		t.Fatalf("queried grant must select AAS rows by authorization UUID:\n%s", aasSQL)
	}
	for _, target := range []struct {
		root  grammar.CollectorRoot
		table string
		alias string
	}{
		{grammar.CollectorRootSM, "submodel", "submodel"},
		{grammar.CollectorRootSME, "submodel_element", "sme"},
		{grammar.CollectorRootCD, "concept_description", "concept_description"},
	} {
		if sql := formulaSQLForRoot(ctx, t, target.root, target.table, target.alias); strings.Contains(sql, "granted_aas_marker") {
			t.Fatalf("queried AAS grant leaked into %s:\n%s", target.root, sql)
		}
	}
	updateCtx := SelectFormulaForRight(WithReBACGrants(WithQueryFilter(t.Context(),
		failClosedQueryFilter(grammar.RightsEnumREAD, grammar.RightsEnumUPDATE)), grants), grammar.RightsEnumUPDATE)
	if sql := formulaSQLForRoot(updateCtx, t, grammar.CollectorRootAAS, "aas", "aas"); strings.Contains(sql, "granted_aas_marker") {
		t.Fatalf("queried READ grant must not authorize UPDATE:\n%s", sql)
	}
}

func TestRegistryGrantsStayOnTheirDescriptorRoot(t *testing.T) {
	t.Parallel()

	marker := func(name string) *goqu.SelectDataset {
		return goqu.Dialect("postgres").From(name).Select(goqu.C("object_uuid"))
	}
	grants := mustGrantSet(t, func(set *ReBACGrantSet) error {
		if err := set.AllowQueriedResources(SemanticResourceAASDesc, marker("granted_shell_descriptor"), grammar.RightsEnumREAD); err != nil {
			return err
		}
		return set.AllowQueriedResources(SemanticResourceBD, marker("granted_discovery_entry"), grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
	ctx := WithReBACGrants(failClosedReadContext(t), grants)

	shellSQL := formulaSQLForRoot(ctx, t, grammar.CollectorRootAASDesc, "descriptor", "descriptor")
	if !strings.Contains(shellSQL, `"granted_shell_descriptor"`) || !strings.Contains(shellSQL, `"rebac_granted_descriptor"."auth_uuid" IN`) {
		t.Fatalf("shell descriptor grant must select descriptors by authorization UUID:\n%s", shellSQL)
	}
	if strings.Contains(shellSQL, "granted_discovery_entry") {
		t.Fatalf("discovery grant leaked into shell descriptors:\n%s", shellSQL)
	}
	submodelSQL := formulaSQLForRoot(ctx, t, grammar.CollectorRootSMDesc, "submodel_descriptor", "submodel_descriptor")
	if strings.Contains(submodelSQL, "granted_shell_descriptor") || strings.Contains(submodelSQL, "granted_discovery_entry") {
		t.Fatalf("grants leaked into standalone Submodel descriptors:\n%s", submodelSQL)
	}
	discoverySQL := formulaSQLForRoot(ctx, t, grammar.CollectorRootBD, "aas_identifier", "aas_identifier")
	if !strings.Contains(discoverySQL, `"rebac_granted_aas_identifier"."auth_uuid" IN`) || strings.Contains(discoverySQL, "granted_shell_descriptor") {
		t.Fatalf("discovery grant must stay on discovery entries:\n%s", discoverySQL)
	}

	nested, err := grammar.NewResolvedFieldPathCollectorForNestedSMDesc()
	if err != nil {
		t.Fatalf("create nested collector: %v", err)
	}
	ds := goqu.Dialect("postgres").From(goqu.T("descriptor")).Select(goqu.I("descriptor.id"))
	secured, err := AddFormulaQueryFromContext(ctx, ds, nested)
	if err != nil {
		t.Fatalf("add formula: %v", err)
	}
	nestedSQL, _, err := secured.ToSQL()
	if err != nil {
		t.Fatalf("render formula: %v", err)
	}
	if !strings.Contains(nestedSQL, `"granted_shell_descriptor"`) {
		t.Fatalf("embedded Submodel descriptors must follow their shell descriptor grant:\n%s", nestedSQL)
	}
}

func TestQueriedElementGrantsAreCorrelatedAndEscaped(t *testing.T) {
	t.Parallel()

	granted := goqu.Dialect("postgres").From("granted_element_marker").
		Select(goqu.C("submodel_uuid"), goqu.C("element_path"))
	grants := mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowQueriedSubmodelElements(granted, grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
	ctx := WithReBACGrants(failClosedReadContext(t), grants)

	smeSQL := formulaSQLForRoot(ctx, t, grammar.CollectorRootSME, "submodel_element", "sme")
	for _, fragment := range []string{
		`"granted_element_marker"`,
		`("rebac_granted_submodel"."id" = "sme"."submodel_id")`,
		`"sme"."idshort_path" = "rebac_granted_element"."element_path"`,
		`replace(replace(replace("rebac_granted_element"."element_path", '!', '!!'), '%', '!%'), '_', '!_')`,
		`|| '.%') ESCAPE '!'`,
		`|| '[%]%') ESCAPE '!'`,
	} {
		if !strings.Contains(smeSQL, fragment) {
			t.Fatalf("queried element grant must be correlated, segment-bounded and escaped (missing %q):\n%s", fragment, smeSQL)
		}
	}
	if sql := formulaSQLForRoot(ctx, t, grammar.CollectorRootSM, "submodel", "submodel"); strings.Contains(sql, "granted_element_marker") {
		t.Fatalf("element grants must never expose the containing Submodel:\n%s", sql)
	}
}

func TestReBACGrantRejectsUnsafeIdentifiers(t *testing.T) {
	t.Parallel()

	grants := NewReBACGrantSet(grammar.RightsEnumREAD)
	for _, value := range []string{"x' OR 1=1 --", "", "{" + grantedAASUUID + "}," + grantedCDUUID} {
		if err := grants.AllowResources(SemanticResourceAAS, []string{value}, grammar.RightsEnumREAD); err == nil {
			t.Fatalf("unsafe authorization UUID %q must be rejected", value)
		}
	}
	if err := grants.AllowResources(SemanticResourceSME, []string{grantedAASUUID}, grammar.RightsEnumREAD); err == nil {
		t.Fatal("element grants must use AllowSubmodelElement")
	}
	if err := grants.AllowSubmodelElement(grantedSubmodelUUID, " ", grammar.RightsEnumREAD); err == nil {
		t.Fatal("element grants require an idShortPath")
	}
	if !grants.IsEmpty() {
		t.Fatal("rejected grants must not be recorded")
	}
}

func TestPublishedReBACGrantsAreImmutable(t *testing.T) {
	t.Parallel()

	grants := mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowResources(SemanticResourceAAS, []string{grantedAASUUID}, grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
	ctx := WithReBACGrants(t.Context(), grants)
	if err := grants.AllowAllOfKind(SemanticResourceAAS, grammar.RightsEnumREAD); err != nil {
		t.Fatal(err)
	}
	published := ReBACGrantsFromContext(ctx)
	if len(published.entries) != 1 || published.entries[0].allOfKind {
		t.Fatalf("published grant set must not observe later mutations: %#v", published.entries)
	}
}

func renderExpression(t *testing.T, expression interface{}) string {
	t.Helper()
	sql, _, err := goqu.Dialect("postgres").From("t").Select(goqu.V(1)).Where(expression.(goqu.Expression)).ToSQL()
	if err != nil {
		t.Fatalf("render expression: %v", err)
	}
	return sql
}
