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

// Package auth contains authentication and authorization helpers, including
// OIDC verification, ABAC evaluation, and shared types used by the HTTP
// middleware and OpenAPI controllers.
//
// This file implements the ABAC "engine": parsing an access model, materializing
// rules, and evaluating requests against those rules to yield an allow/deny
// decision and an optional QueryFilter.
//
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
}

// ParseAccessModel parses a JSON (or YAML converted to JSON) payload that
// conforms to the Access Rule Model schema and returns a compiled AccessModel.
func ParseAccessModel(b []byte, apiRouter *api.Mux) (*AccessModel, error) {
	var m grammar.AccessRuleModelSchemaJSON
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse access model: %w", err)
	}
	return &AccessModel{gen: m, apiRouter: apiRouter, rctx: api.NewRouteContext()}, nil
}

// QueryFilter captures optional, fine-grained restrictions produced by a rule
// even when ACCESS=ALLOW. Controllers can use it to restrict rows, constrain
// mutations, or redact fields. The Discovery Service currently does not require
// a concrete filter structure; extend this struct when needed.
type QueryFilter struct {
	Formula *grammar.LogicalExpression          `json:"Formula,omitempty" yaml:"Formula,omitempty" mapstructure:"Formula,omitempty"`
	Filter  *grammar.AccessPermissionRuleFILTER `json:"Filter,omitempty" yaml:"Filter,omitempty" mapstructure:"Filter,omitempty"`
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
func (m *AccessModel) AuthorizeWithFilter(in EvalInput) (ok bool, code DecisionCode, qf *QueryFilter) {
	rights := m.mapMethodAndPathToRights(in)
	all := m.gen.AllAccessPermissionRules

	var ruleExprs []grammar.LogicalExpression
	var filter *grammar.AccessPermissionRuleFILTER

	for _, r := range all.Rules {
		acl, attrs, objs, lexpr := materialize(all, r)

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
			// rule has no formula → grants full access
			return true, DecisionAllow, nil
		}

		adapted, onlyBool := adaptLEForBackend(*combinedLE, in.Claims, in.IssuedUTC)
		if onlyBool {
			// Fully decidable here; evaluate and continue on false
			if !evalLE(adapted, in.Claims, in.IssuedUTC) {
				continue
			}
			return true, DecisionAllow, nil
		}

		if filter == nil {
			filter = r.FILTER
		}
		ruleExprs = append(ruleExprs, adapted)
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

	simplified, onlyBool := adaptLEForBackend(combined, in.Claims, in.IssuedUTC)
	if onlyBool {
		if evalLE(simplified, in.Claims, in.IssuedUTC) {
			return true, DecisionAllow, nil
		}
		return false, DecisionNoMatch, nil
	}

	return true, DecisionAllow, &QueryFilter{Formula: &simplified, Filter: filter}
}

// materialize resolves a rule's references (USEACL, USEOBJECTS, USEFORMULA) into
// concrete ACL, attributes, objects, and an optional logical expression.
func materialize(all grammar.AccessRuleModelSchemaJSONAllAccessPermissionRules, r grammar.AccessPermissionRule) (grammar.ACL, []grammar.AttributeItem, []grammar.ObjectItem, *grammar.LogicalExpression) {
	// ACL / USEACL
	acl := grammar.ACL{}
	if r.ACL != nil {
		acl = *r.ACL
	} else if r.USEACL != nil {
		use := *r.USEACL
		for _, d := range all.DEFACLS {
			if d.Name == use {
				acl = d.ACL
				break
			}
		}
	}

	// Attributes: inline + referenced
	var attrs []grammar.AttributeItem
	if acl.ATTRIBUTES != nil {
		attrs = append(attrs, acl.ATTRIBUTES...)
	}
	if acl.USEATTRIBUTES != nil {
		use := *acl.USEATTRIBUTES
		for _, d := range all.DEFATTRIBUTES {
			if d.Name == use {
				attrs = append(attrs, d.Attributes...)
				break
			}
		}
	}

	// Objects: inline + referenced (with recursive resolution)
	var objs []grammar.ObjectItem
	if len(r.OBJECTS) > 0 {
		objs = append(objs, r.OBJECTS...)
	}
	if len(r.USEOBJECTS) > 0 {
		objs = append(objs, resolveObjects(all, r.USEOBJECTS)...)
	}

	// Formula: inline or referenced
	var f *grammar.LogicalExpression
	if r.FORMULA != nil {
		f = r.FORMULA
	} else if r.USEFORMULA != nil {
		use := *r.USEFORMULA
		for _, d := range all.DEFFORMULAS {
			if d.Name == use {
				tmp := d.Formula
				f = &tmp
				break
			}
		}
	}

	return acl, attrs, objs, f
}
