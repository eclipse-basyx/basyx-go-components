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

// FragmentFilters groups conditional parts by fragment name so callers can pick
// the subset relevant to the resource they are processing.
type FragmentFilters map[string]grammar.LogicalExpression

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
//
//nolint:revive // i will refactor this function at some point
func (m *AccessModel) AuthorizeWithFilter(in EvalInput) (bool, DecisionCode, *QueryFilter) {
	rights, mapped := m.mapMethodAndPathToRights(in)
	if !mapped {
		return false, DecisionNoMatch, nil
	}

	var ruleExprs []QueryFilter
	var allFragments []string

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
		fragments := make(map[string]grammar.LogicalExpression)
		for _, filter := range r.filterList {
			fragment := string(*filter.FRAGMENT)

			if existing, ok := fragments[fragment]; ok {
				existing.And = append(existing.And, *filter.CONDITION)
				fragments[fragment] = existing
			} else {
				fragments[fragment] = grammar.LogicalExpression{
					And: []grammar.LogicalExpression{
						adapted,
						*filter.CONDITION,
					},
				}
			}
			allFragments = append(allFragments, fragment)
		}

		// Deduplicate And expressions in fragments
		for fragment, expr := range fragments {
			if len(expr.And) > 0 {
				expr.And = deduplicateLogicalExpressions(expr.And)
				fragments[fragment] = expr
			}
		}

		ruleExprs = append(ruleExprs, QueryFilter{&adapted, fragments})

	}

	if len(ruleExprs) == 0 {
		return false, DecisionNoMatch, nil
	}

	combined := grammar.LogicalExpression{Or: []grammar.LogicalExpression{}}
	combinedFragments := make(map[string]grammar.LogicalExpression)
	for _, qfr := range ruleExprs {
		combined.Or = append(combined.Or, *qfr.Formula)

		// filter
		for _, fragment := range allFragments {
			cur := combinedFragments[fragment]
			if existing, ok := qfr.Filters[fragment]; ok {
				cur.Or = append(cur.Or, existing)

			} else {
				cur.Or = append(cur.Or, *qfr.Formula)
			}
			combinedFragments[fragment] = cur
		}
	}

	// Deduplicate Or expressions in combined and fragments
	combined.Or = deduplicateLogicalExpressions(combined.Or)
	for fragment, expr := range combinedFragments {
		if len(expr.Or) > 0 {
			expr.Or = deduplicateLogicalExpressions(expr.Or)
			combinedFragments[fragment] = expr
		}
	}

	for fragment, le := range combinedFragments {
		simpleFilter, onlyBool := adaptLEForBackend(le, in.Claims)
		if onlyBool {
			delete(combinedFragments, fragment)
		} else {
			combinedFragments[fragment] = simpleFilter
		}
	}

	simplified, onlyBool := adaptLEForBackend(combined, in.Claims)

	hasFormula := true
	if onlyBool {
		if !evalLE(simplified, in.Claims) {
			return false, DecisionNoMatch, nil
		}
		hasFormula = false
	}

	var qf *QueryFilter
	if hasFormula || len(combinedFragments) > 0 {
		qf = &QueryFilter{}
		if hasFormula {
			qf.Formula = &simplified
		}
		if len(combinedFragments) > 0 {
			qf.Filters = combinedFragments
		}
	}

	return true, DecisionAllow, qf
}

// FilterExpressionFor returns the logical expression associated with the
// fragment `key` from the QueryFilter's `Filters` map. If the map is nil or
// no entry exists for `key`, nil is returned.
//
// Note: map values are returned by value, so this method returns the address
// of a copy of the stored `grammar.LogicalExpression`. Callers should not rely
// on mutating the returned value to modify the original map entry.
func (q *QueryFilter) FilterExpressionFor(key string) *grammar.LogicalExpression {
	frag, ok := q.Filters[key]
	if !ok {
		return nil
	}
	return &frag
}
