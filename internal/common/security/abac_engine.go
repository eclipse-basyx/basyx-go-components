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
	"strings"

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
	acl    grammar.ACL
	attrs  []grammar.AttributeItem
	objs   []grammar.ObjectItem
	lexpr  *grammar.LogicalExpression
	filter *grammar.AccessPermissionRuleFILTER
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
	rights, mapped := m.mapMethodAndPathToRights(in)
	if !mapped {
		return false, DecisionNoMatch, nil
	}

	var ruleExprs []grammar.LogicalExpression
	var filter *grammar.AccessPermissionRuleFILTER

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
			filter = r.filter
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

// definitionIndex caches definitions for fast lookup during materialization.
type definitionIndex struct {
	acls     map[string]grammar.ACL
	attrs    map[string][]grammar.AttributeItem
	formulas map[string]grammar.LogicalExpression
	objects  map[string]grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem
}

// materializeRules resolves all references in the model up-front so
// AuthorizeWithFilter can work with fully expanded data and invalid references
// fail fast during startup instead of at request time.
func materializeRules(all grammar.AccessRuleModelSchemaJSONAllAccessPermissionRules) ([]materializedRule, error) {
	index, err := buildDefinitionIndex(all)
	if err != nil {
		return nil, err
	}

	rules := make([]materializedRule, 0, len(all.Rules))
	for i, r := range all.Rules {
		mr, err := materializeRule(index, r)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i+1, err)
		}
		rules = append(rules, mr)
	}

	return rules, nil
}

func buildDefinitionIndex(all grammar.AccessRuleModelSchemaJSONAllAccessPermissionRules) (definitionIndex, error) {
	trim := func(s string) (string, error) {
		out := strings.TrimSpace(s)
		if out == "" {
			return "", fmt.Errorf("definition name must not be empty")
		}
		return out, nil
	}

	index := definitionIndex{
		acls:     make(map[string]grammar.ACL),
		attrs:    make(map[string][]grammar.AttributeItem),
		formulas: make(map[string]grammar.LogicalExpression),
		objects:  make(map[string]grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem),
	}

	for _, d := range all.DEFACLS {
		name, err := trim(d.Name)
		if err != nil {
			return index, fmt.Errorf("DEFACLS: %w", err)
		}
		if _, exists := index.acls[name]; exists {
			return index, fmt.Errorf("DEFACLS: duplicate name %q", name)
		}
		index.acls[name] = d.ACL
	}

	for _, d := range all.DEFATTRIBUTES {
		name, err := trim(d.Name)
		if err != nil {
			return index, fmt.Errorf("DEFATTRIBUTES: %w", err)
		}
		if _, exists := index.attrs[name]; exists {
			return index, fmt.Errorf("DEFATTRIBUTES: duplicate name %q", name)
		}
		index.attrs[name] = d.Attributes
	}

	for _, d := range all.DEFFORMULAS {
		name, err := trim(d.Name)
		if err != nil {
			return index, fmt.Errorf("DEFFORMULAS: %w", err)
		}
		if _, exists := index.formulas[name]; exists {
			return index, fmt.Errorf("DEFFORMULAS: duplicate name %q", name)
		}
		index.formulas[name] = d.Formula
	}

	for _, d := range all.DEFOBJECTS {
		name, err := trim(d.Name)
		if err != nil {
			return index, fmt.Errorf("DEFOBJECTS: %w", err)
		}
		if _, exists := index.objects[name]; exists {
			return index, fmt.Errorf("DEFOBJECTS: duplicate name %q", name)
		}
		index.objects[name] = d
	}

	return index, nil
}

// materializeRule resolves a rule's references (USEACL, USEOBJECTS, USEFORMULA)
// into concrete ACL, attributes, objects, and an optional logical expression.
// It returns an error when a referenced definition is missing.
func materializeRule(index definitionIndex, r grammar.AccessPermissionRule) (materializedRule, error) {
	mr := materializedRule{filter: r.FILTER}

	// ACL / USEACL
	switch {
	case r.ACL != nil:
		mr.acl = *r.ACL
	case r.USEACL != nil:
		name := strings.TrimSpace(*r.USEACL)
		acl, ok := index.acls[name]
		if !ok {
			return mr, fmt.Errorf("USEACL %q not found", name)
		}
		mr.acl = acl
	default:
		return mr, fmt.Errorf("ACL is required")
	}

	// Attributes: inline + referenced
	if mr.acl.ATTRIBUTES != nil {
		mr.attrs = append(mr.attrs, mr.acl.ATTRIBUTES...)
	}
	if mr.acl.USEATTRIBUTES != nil {
		name := strings.TrimSpace(*mr.acl.USEATTRIBUTES)
		attrs, ok := index.attrs[name]
		if !ok {
			return mr, fmt.Errorf("USEATTRIBUTES %q not found", name)
		}
		mr.attrs = append(mr.attrs, attrs...)
	}

	// Objects: inline + referenced (with recursive resolution)
	if len(r.OBJECTS) > 0 {
		mr.objs = append(mr.objs, r.OBJECTS...)
	}
	if len(r.USEOBJECTS) > 0 {
		resolved, err := resolveObjects(index, r.USEOBJECTS, map[string]bool{})
		if err != nil {
			return mr, err
		}
		mr.objs = append(mr.objs, resolved...)
	}

	// Formula: inline or referenced
	switch {
	case r.FORMULA != nil:
		mr.lexpr = r.FORMULA
	case r.USEFORMULA != nil:
		name := strings.TrimSpace(*r.USEFORMULA)
		f, ok := index.formulas[name]
		if !ok {
			return mr, fmt.Errorf("USEFORMULA %q not found", name)
		}
		tmp := f
		mr.lexpr = &tmp
	default:
		return mr, fmt.Errorf("FORMULA is required")
	}

	return mr, nil
}

func resolveObjects(index definitionIndex, names []string, seen map[string]bool) ([]grammar.ObjectItem, error) {
	var out []grammar.ObjectItem

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("USEOBJECTS reference must not be empty")
		}

		if seen[name] {
			return nil, fmt.Errorf("circular USEOBJECTS reference involving %q", name)
		}

		def, ok := index.objects[name]
		if !ok {
			return nil, fmt.Errorf("USEOBJECTS %q not found", name)
		}

		if len(def.Objects) > 0 {
			out = append(out, def.Objects...)
		}

		if len(def.USEOBJECTS) > 0 {
			seen[name] = true
			nested, err := resolveObjects(index, def.USEOBJECTS, seen)
			delete(seen, name)
			if err != nil {
				return nil, err
			}
			out = append(out, nested...)
		}
	}

	return out, nil
}
