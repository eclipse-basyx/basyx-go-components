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
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

const boundRequestKey ctxKey = "resourceBoundRequest"

type boundPolicyDecision struct {
	id         int64
	evaluation AuthorizationEvaluation
}
type boundRequest struct {
	aasContext     exp.Expression
	revision       int64
	policyID       string
	creationTarget *boundTarget
	repo           *resourceBoundRepository
	target         boundTarget
	input          EvalInput
	right          grammar.RightsEnum
	policies       []boundPolicyDecision
	fallback       AuthorizationEvaluation
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
	state := &boundRequest{repo: repo, target: target, input: input, right: right, revision: revision, policyID: "resource-bound-first:" + repo.scope}
	input.RequiredRight = right
	opts := grammar.DefaultSimplifyOptions()
	opts.EnableImplicitCasts = repo.implicitCasts
	if repo.fallback != nil {
		if model := repo.fallback.ActiveAccessModel(); model != nil {
			if model.gen.AllAccessPermissionRules.Rules == nil {
				return nil, fmt.Errorf("REBAC-FALLBACK-DOCUMENT object-based rules must be an array")
			}
			state.fallback = model.AuthorizeWithFilterWithOptions(input, opts)
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
	for _, policy := range snapshot.policies {
		state.policies = append(state.policies, boundPolicyDecision{id: policy.id, evaluation: policy.model.AuthorizeWithFilterWithOptions(input, opts)})
	}
	return state, nil
}

func (repo *resourceBoundRepository) compileSnapshot(ctx context.Context, db boundQueryer, revision int64) (*boundCompiledSnapshot, error) {
	query, args, err := goqu.Dialect("postgres").From("rebac_access").Select("id", "revision", "policy").Where(goqu.Ex{"scope": repo.scope}, goqu.C("policy").IsNotNull()).Prepared(true).ToSQL()
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
	var aas exp.Expression = goqu.L("NULL::BIGINT")
	if state.target.AAS != "" {
		aas = dialect.From("aas").Select("id").Where(goqu.Ex{"aas_id": state.target.AAS})
	}
	if state.aasContext != nil {
		aas = state.aasContext
	}
	return dialect.From("rebac_access").Select(goqu.Func("rebac_effective_policy", goqu.C("id"), aas)).Where(goqu.Ex{"scope": state.repo.scope}, binding)
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

func boundFragment(evaluation AuthorizationEvaluation, fragment grammar.FragmentStringPattern, collector *grammar.ResolvedFieldPathCollector) (exp.Expression, error) {
	if fragment == "" || evaluation.QueryFilter == nil {
		return goqu.L("TRUE"), nil
	}
	predicates := evaluation.QueryFilter.FilterPredicateEntriesFor(fragment)
	expressions := make([]exp.Expression, 0, len(predicates))
	for _, predicate := range predicates {
		expression, err := evaluateFragmentFilterPredicate(predicate.Predicate, predicate.Fragment, collector)
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

func (state *boundRequest) expression(collector *grammar.ResolvedFieldPathCollector, fragment grammar.FragmentStringPattern) (exp.Expression, error) {
	kind, key, err := collector.AuthorizationResource()
	if err != nil {
		return nil, err
	}
	if kind == "submodel" && state.target.Kind == "sme" {
		return goqu.L("TRUE"), nil
	}

	selected := goqu.COALESCE(state.selectedPolicy(kind, key), int64(0))
	grants := make([]exp.Expression, 0, len(state.policies))
	visible := make([]exp.Expression, 0, len(state.policies))
	for _, policy := range state.policies {
		if !policy.evaluation.Allowed {
			continue
		}
		formula, err := boundFormula(policy.evaluation, collector)
		if err != nil {
			return nil, err
		}
		matches := goqu.And(goqu.L("? = ?", selected, policy.id), formula)
		grants = append(grants, matches)
		filter, err := boundFragment(policy.evaluation, fragment, collector)
		if err != nil {
			return nil, err
		}
		visible = append(visible, goqu.And(matches, filter))
	}
	granted := boundOr(grants)
	fallback, err := boundFormula(state.fallback, collector)
	if err != nil {
		return nil, err
	}
	filter, err := boundFragment(state.fallback, fragment, collector)
	if err != nil {
		return nil, err
	}
	visible = append(visible, goqu.And(goqu.L("NOT (?)", granted), fallback, filter))
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
	result := goqu.Case()
	matched := false
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
