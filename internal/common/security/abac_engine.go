/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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
	"fmt"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	api "github.com/go-chi/chi/v5"
	jsoniter "github.com/json-iterator/go"
)

// AccessModel is an evaluated, in-memory representation of the Access Rule Model
// (ARM) used by the ABAC engine. It holds the generated schema and provides
// evaluation helpers.
type AccessModel struct {
	gen       grammar.AccessRuleModelSchemaJSON
	apiRouter *api.Mux
	rctx      *api.Context
	rules     []materializedRule
}

type materializedRule struct {
	acl        grammar.ACL
	attrs      []grammar.AttributeItem
	objs       []grammar.ObjectItem
	lexpr      *grammar.LogicalExpression
	filterList []grammar.AccessPermissionRuleFILTER
}

// ParseAccessModel parses a JSON (or YAML converted to JSON) payload that
// conforms to the Access Rule Model schema and returns a compiled AccessModel.
func ParseAccessModel(b []byte, apiRouter *api.Mux) (*AccessModel, error) {
	var m grammar.AccessRuleModelSchemaJSON
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse access model: %w", err)
	}

	rules, err := materializeRules(m.AllAccessPermissionRules)
	if err != nil {
		return nil, fmt.Errorf("parse access model: %w", err)
	}

	return &AccessModel{
		gen:       m,
		apiRouter: apiRouter,
		rctx:      api.NewRouteContext(),
		rules:     rules,
	}, nil
}

// FilterConditionParts holds the main and optional logical expressions that
// together define a fragment-level filter.
type FilterConditionParts struct {
	MainPart     grammar.LogicalExpression `json:"mainPart,omitempty" yaml:"mainPart,omitempty" mapstructure:"mainPart,omitempty"`
	OptionalPart grammar.LogicalExpression `json:"optionalPart,omitempty" yaml:"optionalPart,omitempty" mapstructure:"optionalPart,omitempty"`
}

// FragmentFilters groups conditional parts by fragment name so callers can pick
// the subset relevant to the resource they are processing.
type FragmentFilters map[string]FilterConditionParts

// QueryFilter captures optional, fine-grained restrictions produced by a rule
// even when ACCESS=ALLOW. Controllers can use it to restrict rows, constrain
// mutations, or redact fields. The Discovery Service currently does not require
// a concrete filter structure; extend this struct when needed.
type QueryFilter struct {
	Formula *grammar.LogicalExpression `json:"Formula,omitempty" yaml:"Formula,omitempty" mapstructure:"Formula,omitempty"`
	Filters FragmentFilters            `json:"Filters,omitempty" yaml:"Filters,omitempty" mapstructure:"Filters,omitempty"`
}

// DecisionCode represents the result of an authorization check.
// It is serialized as a JSON string for consistent use in controller
// responses and API payloads.
type DecisionCode string

const (
	// DecisionAllow indicates that the authorization check succeeded
	// and the requested action is permitted.
	DecisionAllow DecisionCode = "ALLOW"

	// DecisionNoMatch indicates that no matching rule or policy was found
	// for the authorization check, resulting in a neutral or deny outcome.
	DecisionNoMatch DecisionCode = "NO_MATCH"
)

// AuthorizeWithFilter evaluates the request against the model rules in order.
// It returns whether access is allowed, a human-readable reason, and an optional
// QueryFilter for controllers to enforce (e.g., tenant scoping, redactions).
// It is important that this function does not throw errors. It either gives access or no access.
func (m *AccessModel) AuthorizeWithFilter(in EvalInput) (bool, DecisionCode, *QueryFilter) {
	rights, mapped := m.mapMethodAndPathToRights(in)
	if !mapped {
		return false, DecisionNoMatch, nil
	}

	var ruleExprs []grammar.LogicalExpression
	fragfilters := make(map[string][]grammar.LogicalExpression)
	noFilters := make(map[string][]grammar.LogicalExpression)

	for _, r := range m.rules {
		acl, attrs, objs, lexpr := r.acl, r.attrs, r.objs, r.lexpr
		// Gate 0: check disabled
		if acl.ACCESS == grammar.ACLACCESSDISABLED {
			continue
		}
		// Gate 1: rights
		if !rightsContainsAll(acl.RIGHTS, rights) {
			continue
		}
		// Gate 2: attributes
		if !attributesSatisfiedAll(attrs, in.Claims) {
			continue
		}
		// Gate 3: objects
		accessWithOptinalFilter := matchRouteObjectsObjItem(objs, in.Path)
		if !accessWithOptinalFilter.access {
			continue
		}

		combinedLE := lexpr
		if accessWithOptinalFilter.le != nil {
			if combinedLE == nil {
				// no rule formula -> use route formula as-is
				combinedLE = accessWithOptinalFilter.le
			} else {
				// wrap both in an AND
				andExpr := grammar.LogicalExpression{
					And: []grammar.LogicalExpression{
						*combinedLE,
						*accessWithOptinalFilter.le,
					},
				}
				combinedLE = &andExpr
			}
		}

		// Gate 4: formula → adapt for backend filtering
		if combinedLE == nil {
			// rule has no formula: should not happen -> deny access
			return false, DecisionNoMatch, nil
		}

		adapted, onlyBool := adaptLEForBackend(*combinedLE, in.Claims)
		if onlyBool {
			// Fully decidable here; evaluate and continue on false
			if !evalLE(adapted, in.Claims) {
				continue
			}
		}

		ruleExprs = append(ruleExprs, adapted)

		filterConditions := make(map[string][]grammar.LogicalExpression)

		for _, filter := range r.filterList {
			if filter.FRAGMENT != nil && filter.CONDITION != nil {
				fragment := string(*filter.FRAGMENT)
				filterConditions[fragment] =
					append(filterConditions[fragment], *filter.CONDITION)
			}
		}
		for fragment, conditions := range filterConditions {
			if len(conditions) > 0 {
				filterCondRaw := grammar.LogicalExpression{
					And: append(conditions, adapted),
				}

				fragfilters[fragment] = append(fragfilters[fragment], filterCondRaw)
				noFilters[fragment] = append(noFilters[fragment], adapted)
			} else {
				noFilters["ignore"] = append(noFilters["ignore"], adapted)
			}
		}

	}

	if len(ruleExprs) == 0 {
		return false, DecisionNoMatch, nil
	}

	var combined grammar.LogicalExpression
	if len(ruleExprs) == 1 {
		combined = ruleExprs[0]
	} else {
		combined = grammar.LogicalExpression{Or: ruleExprs}
	}

	simplified, onlyBool := adaptLEForBackend(combined, in.Claims)
	hasFormula := true
	if onlyBool {
		if !evalLE(simplified, in.Claims) {
			return false, DecisionNoMatch, nil
		}
		hasFormula = false
	}

	combinedFiltersMap := make(FragmentFilters, len(fragfilters))

	for fragment, conds := range fragfilters {
		falseBool := false
		expr := grammar.LogicalExpression{Boolean: &falseBool}

		for fragment2, conds2 := range noFilters {
			if fragment != fragment2 {
				// Append noFilter into OR
				expr = grammar.LogicalExpression{
					Or: append(expr.Or, conds2...),
				}
			}
		}
		expr, _ = adaptLEForBackend(expr, in.Claims)

		combinedFiltersMap[fragment] = FilterConditionParts{MainPart: grammar.LogicalExpression{Or: conds}, OptionalPart: expr}

	}
	var qf *QueryFilter
	if hasFormula || len(combinedFiltersMap) > 0 {
		qf = &QueryFilter{}
		if hasFormula {
			qf.Formula = &simplified
		}
		if len(combinedFiltersMap) > 0 {
			qf.Filters = combinedFiltersMap
		}
	}
	return true, DecisionAllow, qf
}

// FilterExpressionFor returns the logical expression that enforces the fragment
// filter identified by key. If negateMainPart is true, the main part is wrapped
// in a NOT before being combined with the optional part. The boolean indicates
// whether the fragment exists; when it does not exist, a false expression is
// returned so callers can reject the lookup.
func (q *QueryFilter) FilterExpressionFor(key string, negateMainPart bool) (bool, grammar.LogicalExpression) {
	filter, ok := q.Filters[key]
	if ok {
		var mainPart grammar.LogicalExpression
		if negateMainPart {
			mainPart = grammar.LogicalExpression{Not: &filter.MainPart}
		} else {
			mainPart = filter.MainPart
		}
		return true, grammar.LogicalExpression{Or: []grammar.LogicalExpression{mainPart, filter.OptionalPart}}
	}
	falseBool := false
	return ok, grammar.LogicalExpression{Boolean: &falseBool}
}

// ExistsExpressionFor builds a logical expression that evaluates to true when
// the fragment filter identified by key is satisfied. If negateMainPart is
// true, the main part is wrapped in a NOT before being combined with the
// optional part. The boolean indicates whether the fragment exists; when it
// does not, a true expression is returned so callers can skip additional
// filtering.
func (q *QueryFilter) ExistsExpressionFor(key string, negateMainPart bool) (bool, grammar.LogicalExpression) {
	filter, ok := q.Filters[key]
	if ok {
		var mainPart grammar.LogicalExpression
		if negateMainPart {
			mainPart = grammar.LogicalExpression{Not: &filter.MainPart}
		} else {
			mainPart = filter.MainPart
		}
		return true, grammar.LogicalExpression{Or: []grammar.LogicalExpression{mainPart, filter.OptionalPart}}
	}
	trueBool := true
	return ok, grammar.LogicalExpression{Boolean: &trueBool}
}
