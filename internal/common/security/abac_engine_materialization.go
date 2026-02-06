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
	"fmt"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

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
	filterList := r.FILTERLIST
	if r.FILTER != nil {
		filterList = append(filterList, *r.FILTER)
	}
	resolvedFilters := make([]grammar.AccessPermissionRuleFILTER, 0, len(filterList))
	for i, filter := range filterList {
		if filter.FRAGMENT == nil {
			return materializedRule{}, fmt.Errorf("FILTERLIST[%d]: FRAGMENT is required", i+1)
		}

		useFormulaName := ""
		if filter.USEFORMULA != nil {
			useFormulaName = strings.TrimSpace(*filter.USEFORMULA)
		}

		if filter.CONDITION != nil && useFormulaName != "" {
			return materializedRule{}, fmt.Errorf("FILTERLIST[%d]: only one of CONDITION or USEFORMULA may be defined", i+1)
		}

		if filter.CONDITION == nil {
			if useFormulaName == "" {
				return materializedRule{}, fmt.Errorf("FILTERLIST[%d]: CONDITION or USEFORMULA is required", i+1)
			}
			f, ok := index.formulas[useFormulaName]
			if !ok {
				return materializedRule{}, fmt.Errorf("FILTERLIST[%d]: USEFORMULA %q not found", i+1, useFormulaName)
			}
			tmp := f
			filter.CONDITION = &tmp
		}

		filter.USEFORMULA = nil
		resolvedFilters = append(resolvedFilters, filter)
	}

	mr := materializedRule{filterList: resolvedFilters}

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
