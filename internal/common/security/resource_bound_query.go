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
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

const boundRequestKey ctxKey = "resourceBoundRequest"

type boundPolicyDecision struct {
	id         int64
	evaluation AuthorizationEvaluation
}
type boundRequest struct {
	aasContext          exp.Expression
	revision            int64
	policyID            string
	creationTarget      *boundTarget
	repo                *resourceBoundRepository
	target              boundTarget
	input               EvalInput
	right               grammar.RightsEnum
	policies            []boundPolicyDecision
	actorPrincipals     []common.AccessPrincipal
	fallback            AuthorizationEvaluation
	collectionAdmission AuthorizationEvaluation
}

func boundRequestFromContext(ctx context.Context) *boundRequest {
	value, _ := ctx.Value(boundRequestKey).(*boundRequest)
	return value
}

type boundCompiledPolicy struct {
	id    int64
	model *AccessModel
}
type boundCompiledSnapshot struct {
	revision int64
	policies []boundCompiledPolicy
}

func (repo *resourceBoundRepository) requestState(ctx context.Context, db boundQueryer, target boundTarget, input EvalInput, right grammar.RightsEnum, revision int64) (*boundRequest, error) {
	state := &boundRequest{repo: repo, target: target, input: input, right: right, revision: revision, policyID: "resource-bound-first:" + repo.scope, actorPrincipals: repo.boundPrincipals(input.Claims)}
	opts := grammar.DefaultSimplifyOptions()
	opts.EnableImplicitCasts = repo.implicitCasts
	if repo.fallback != nil {
		if model := repo.fallback.ActiveAccessModel(); model != nil {
			if isBoundCollection(target.Kind) {
				state.collectionAdmission, state.fallback = authorizeABACCollection(model, input, opts)
			} else {
				state.fallback = authorizeABACFallback(model, input, opts)
			}
		}
	}
	snapshot := repo.compiled.Load()
	if snapshot == nil || snapshot.revision != revision {
		var err error
		snapshot, err = repo.compileSnapshot(ctx, db, revision)
		if err != nil {
			return nil, err
		}
		repo.compiled.Store(snapshot)
	}
	policyInput := input
	policyInput.Claims = withBoundGroupClaims(input.Claims, state.actorPrincipals)
	for _, policy := range snapshot.policies {
		state.policies = append(state.policies, boundPolicyDecision{id: policy.id, evaluation: authorizeBoundModel(policy.model, policyInput, right, opts, nil)})
	}
	return state, nil
}

func authorizeABACFallback(model *AccessModel, input EvalInput, opts grammar.SimplifyOptions) AuthorizationEvaluation {
	return model.AuthorizeWithFilterWithOptions(input, opts)
}

func authorizeABACCollection(model *AccessModel, input EvalInput, opts grammar.SimplifyOptions) (AuthorizationEvaluation, AuthorizationEvaluation) {
	admission := authorizeABACWithObjectSelector(model, input, opts, func(object grammar.ObjectItem) bool {
		return object.Kind == grammar.Route
	})
	resources := authorizeABACWithObjectSelector(model, input, opts, func(object grammar.ObjectItem) bool {
		return object.Kind != grammar.Route
	})
	return admission, resources
}

func authorizeABACWithObjectSelector(model *AccessModel, input EvalInput, opts grammar.SimplifyOptions, include func(grammar.ObjectItem) bool) AuthorizationEvaluation {
	if model == nil {
		return AuthorizationEvaluation{Reason: DecisionNoMatch}
	}
	selected := *model
	selected.rules = make([]materializedRule, 0, len(model.rules))
	for _, source := range model.rules {
		rule := source
		rule.objs = make([]grammar.ObjectItem, 0, len(source.objs))
		for _, object := range source.objs {
			if include(object) {
				rule.objs = append(rule.objs, object)
			}
		}
		if len(rule.objs) > 0 {
			selected.rules = append(selected.rules, rule)
		}
	}
	return selected.AuthorizeWithFilterWithOptions(input, opts)
}

func authorizeBoundModel(model *AccessModel, input EvalInput, right grammar.RightsEnum, opts grammar.SimplifyOptions, include func(grammar.ObjectItem) bool) AuthorizationEvaluation {
	if model == nil {
		return AuthorizationEvaluation{Reason: DecisionNoMatch}
	}
	selected := *model
	selected.rules = make([]materializedRule, 0, len(model.rules))
	for _, source := range model.rules {
		if !boundRuleAllowsRight(source.acl.RIGHTS, right) {
			continue
		}
		rule := source
		rule.acl.RIGHTS = []grammar.RightsEnum{grammar.RightsEnumALL}
		if include != nil {
			rule.objs = make([]grammar.ObjectItem, 0, len(source.objs))
			for _, object := range source.objs {
				if include(object) {
					rule.objs = append(rule.objs, object)
				}
			}
		}
		if len(rule.objs) > 0 {
			selected.rules = append(selected.rules, rule)
		}
	}
	return selected.AuthorizeWithFilterWithOptions(input, opts)
}

func boundRuleAllowsRight(rights []grammar.RightsEnum, required grammar.RightsEnum) bool {
	if ruleAllowsRight(rights, required) {
		return true
	}
	return required == grammar.RightsEnumVIEW && ruleAllowsRight(rights, grammar.RightsEnumREAD)
}

func (repo *resourceBoundRepository) compileSnapshot(ctx context.Context, db boundQueryer, revision int64) (*boundCompiledSnapshot, error) {
	query, args, err := goqu.Dialect("postgres").From("rebac_access").Select("id", "revision", "policy").Where(goqu.Ex{"scope": repo.scope}, goqu.C("collection").IsNull(), goqu.C("policy").IsNotNull()).Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-STATE-BUILD %w", err)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-STATE-QUERY %w", err)
	}
	defer func() { _ = rows.Close() }()
	snapshot := &boundCompiledSnapshot{revision: revision}
	for rows.Next() {
		var id, policyRevision int64
		var raw []byte
		if err = rows.Scan(&id, &policyRevision, &raw); err != nil {
			return nil, fmt.Errorf("REBAC-STATE-SCAN %w", err)
		}
		policy, err := decodeBoundPolicy(raw)
		if err != nil {
			return nil, err
		}
		model, err := CompileResourceBoundPolicy(*policy, repo.router, repo.basePath)
		if err != nil {
			return nil, err
		}
		model.WithPolicyID(fmt.Sprintf("rebac:%s:%d:%d", repo.scope, id, policyRevision))
		snapshot.policies = append(snapshot.policies, boundCompiledPolicy{id: id, model: model})
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("REBAC-STATE-ROWS %w", err)
	}
	return snapshot, nil
}

func (state *boundRequest) queryFilter() *QueryFilter {
	formula := boolExpression(true)
	qf := &QueryFilter{Formula: &formula, FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{state.right: boolExpression(true)}, Filters: FragmentFilters{}}
	qf.Filters["$sme"] = NewFragmentFilterPredicate(boolExpression(true), true)
	decisions := append([]boundPolicyDecision(nil), state.policies...)
	decisions = append(decisions, boundPolicyDecision{evaluation: state.fallback})
	for _, decision := range decisions {
		if filter := decision.evaluation.QueryFilter; filter != nil {
			for fragment := range filter.Filters {
				qf.Filters[fragment] = NewFragmentFilterPredicate(boolExpression(true), true)
			}
		}
	}
	return qf
}

func (state *boundRequest) selectedPolicy(kind string, key exp.Expression) *goqu.SelectDataset {
	dialect := goqu.Dialect("postgres")
	binding := state.selectedAccessBinding(kind, key)
	var aas exp.Expression = goqu.L("NULL::BIGINT")
	if state.target.AAS != "" {
		aas = dialect.From("aas").Select("id").Where(goqu.Ex{"aas_id": state.target.AAS})
	}
	if state.aasContext != nil {
		aas = state.aasContext
	}
	return dialect.From("rebac_access").Select(goqu.Func("rebac_effective_policy", goqu.C("id"), aas)).Where(goqu.Ex{"scope": state.repo.scope}, binding)
}

func (state *boundRequest) selectedAccess(kind string, key exp.Expression) *goqu.SelectDataset {
	return goqu.Dialect("postgres").From("rebac_access").Select("id").Where(
		goqu.Ex{"scope": state.repo.scope},
		state.selectedAccessBinding(kind, key),
	)
}

func (state *boundRequest) selectedAccessBinding(kind string, key exp.Expression) exp.Expression {
	column := boundForeignKey(kind)
	if kind == "sme" {
		column = "sme_id"
	}
	condition := goqu.C(column).Eq(key)
	var binding exp.Expression = condition
	if state.creationTarget != nil {
		var err error
		binding, err = state.creationTarget.condition()
		if err != nil {
			binding = goqu.L("FALSE")
		}
	}
	return binding
}

func (state *boundRequest) ownerGrant(kind string, key exp.Expression) exp.Expression {
	if len(state.actorPrincipals) == 0 {
		return goqu.L("FALSE")
	}
	owner := goqu.T("rebac_principal").As("rebac_owner")
	principals := make([]exp.Expression, 0, len(state.actorPrincipals))
	for _, principal := range state.actorPrincipals {
		principals = append(principals, goqu.Ex{
			"rebac_owner.principal_type": principal.NormalizedType(),
			"rebac_owner.issuer":         principal.Issuer,
			"rebac_owner.subject":        principal.Subject,
		})
	}
	query := goqu.Dialect("postgres").From(owner).Select(goqu.L("1")).Where(
		goqu.I("rebac_owner.access_id").In(state.selectedAccess(kind, key)),
		goqu.Ex{"rebac_owner.relation": "owner"},
		goqu.Or(principals...),
	)
	return goqu.L("EXISTS ?", query)
}

func boundFormula(evaluation AuthorizationEvaluation, collector *grammar.ResolvedFieldPathCollector) (exp.Expression, error) {
	if !evaluation.Allowed {
		return goqu.L("FALSE"), nil
	}
	if evaluation.QueryFilter == nil || evaluation.QueryFilter.Formula == nil {
		return goqu.L("TRUE"), nil
	}
	expression, _, err := evaluation.QueryFilter.Formula.EvaluateToExpression(collector)
	if err != nil {
		return nil, fmt.Errorf("REBAC-FORMULA-EVALUATE %w", err)
	}
	return goqu.COALESCE(expression, false), nil
}

func boundFragment(ctx context.Context, evaluation AuthorizationEvaluation, fragment grammar.FragmentStringPattern, collector *grammar.ResolvedFieldPathCollector) (exp.Expression, error) {
	if fragment == "" || evaluation.QueryFilter == nil {
		return goqu.L("TRUE"), nil
	}
	predicates := evaluation.QueryFilter.FilterPredicateEntriesFor(fragment)
	expressions := make([]exp.Expression, 0, len(predicates))
	for _, predicate := range predicates {
		expression, err := evaluateFragmentFilterPredicate(ctx, predicate.Predicate, predicate.Fragment, collector)
		if err != nil {
			return nil, fmt.Errorf("REBAC-FRAGMENT-EVALUATE %w", err)
		}
		expressions = append(expressions, expression)
	}
	if len(expressions) == 0 {
		return goqu.L("TRUE"), nil
	}
	return goqu.And(expressions...), nil
}

func (state *boundRequest) expression(ctx context.Context, collector *grammar.ResolvedFieldPathCollector, fragment grammar.FragmentStringPattern) (exp.Expression, error) {
	kind, key, err := collector.AuthorizationResource()
	if err != nil {
		return nil, err
	}
	if kind == "submodel" && state.target.Kind == "sme" {
		return goqu.L("TRUE"), nil
	}

	selected := goqu.COALESCE(state.selectedPolicy(kind, key), int64(0))
	visible := make([]exp.Expression, 0, len(state.policies)+2)
	visible = append(visible, state.ownerGrant(kind, key))
	for _, policy := range state.policies {
		if !policy.evaluation.Allowed {
			continue
		}
		formula, err := boundFormula(policy.evaluation, collector)
		if err != nil {
			return nil, err
		}
		matches := goqu.And(goqu.L("? = ?", selected, policy.id), formula)
		filter, err := boundFragment(ctx, policy.evaluation, fragment, collector)
		if err != nil {
			return nil, err
		}
		visible = append(visible, goqu.And(matches, filter))
	}
	fallback, err := boundFormula(state.fallback, collector)
	if err != nil {
		return nil, err
	}
	filter, err := boundFragment(ctx, state.fallback, fragment, collector)
	if err != nil {
		return nil, err
	}
	visible = append(visible, goqu.And(fallback, filter))
	return boundOr(visible), nil
}
func boundOr(expressions []exp.Expression) exp.Expression {
	if len(expressions) == 0 {
		return goqu.L("FALSE")
	}
	return goqu.Or(expressions...)
}

func (state *boundRequest) fragmentSignature(fragment grammar.FragmentStringPattern) string {
	for _, policy := range state.policies {
		if policy.evaluation.QueryFilter != nil && len(policy.evaluation.QueryFilter.FilterPredicateEntriesFor(fragment)) > 0 {
			return "rebac:fragment:" + string(fragment)
		}
	}
	if state.fallback.QueryFilter != nil && len(state.fallback.QueryFilter.FilterPredicateEntriesFor(fragment)) > 0 {
		return "rebac:fragment:" + string(fragment)
	}
	return "rebac:resource"
}

func (state *boundRequest) provenance(collector *grammar.ResolvedFieldPathCollector) (exp.Expression, error) {
	kind, key, err := collector.AuthorizationResource()
	if err != nil {
		return nil, err
	}
	selected := goqu.COALESCE(state.selectedPolicy(kind, key), int64(0))
	result := goqu.Case().When(state.ownerGrant(kind, key), "rebac-owner:"+state.repo.scope)
	matched := true
	for _, policy := range state.policies {
		if !policy.evaluation.Allowed {
			continue
		}
		formula, err := boundFormula(policy.evaluation, collector)
		if err != nil {
			return nil, err
		}
		matched = true
		result = result.When(goqu.And(goqu.L("? = ?", selected, policy.id), formula), policy.evaluation.PolicyID)
	}
	if !matched {
		return goqu.Cast(goqu.V(state.fallback.PolicyID), "TEXT"), nil
	}
	return result.Else(state.fallback.PolicyID), nil
}
