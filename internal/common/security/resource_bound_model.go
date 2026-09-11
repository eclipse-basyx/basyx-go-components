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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/go-chi/chi/v5"
)

// ResourceBoundPolicy is the resource-bound representation proposed in Part 4 PR 108.
type ResourceBoundPolicy struct {
	Resource   grammar.ObjectItem                                                           `json:"RESOURCE"`
	Attributes []grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem `json:"DEFATTRIBUTES,omitempty"`
	ACLs       []grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem       `json:"DEFACLS,omitempty"`
	Formulas   []grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem   `json:"DEFFORMULAS,omitempty"`
	Rules      []json.RawMessage                                                            `json:"rules"`
}

// ParseResourceBoundDocument validates bindings and compiles every model independently.
func ParseResourceBoundDocument(data []byte) ([]ResourceBoundPolicy, error) {
	var doc struct {
		Models []ResourceBoundPolicy `json:"ResourceBoundAccessRuleModels"`
	}
	if err := common.UnmarshalAndDisallowUnknownFields(data, &doc); err != nil {
		return nil, fmt.Errorf("REBAC-PARSEDOC-DECODE %w", err)
	}
	if len(doc.Models) == 0 {
		return nil, fmt.Errorf("REBAC-PARSEDOC-EMPTY collection must not be empty")
	}
	seen := make(map[string]bool, len(doc.Models))
	for _, model := range doc.Models {
		key, err := ResourceBoundKey(model.Resource)
		if err != nil {
			return nil, err
		}
		if seen[key] {
			return nil, fmt.Errorf("REBAC-PARSEDOC-DUPLICATE duplicate resource %s", key)
		}
		seen[key] = true
		if _, err = CompileResourceBoundPolicy(model, nil, ""); err != nil {
			return nil, err
		}
	}
	return doc.Models, nil
}

// UnmarshalJSON validates mandatory fields without accepting object-based definitions.
func (p *ResourceBoundPolicy) UnmarshalJSON(data []byte) error {
	type plain ResourceBoundPolicy
	var value plain
	if err := common.UnmarshalAndDisallowUnknownFields(data, &value); err != nil {
		return fmt.Errorf("REBAC-PARSEPOLICY-DECODE %w", err)
	}
	if err := validateBoundDefinitionArrays(data); err != nil {
		return err
	}
	if value.Rules == nil {
		return fmt.Errorf("REBAC-PARSEPOLICY-RULES rules must be an array")
	}
	if _, err := ResourceBoundKey(value.Resource); err != nil {
		return err
	}
	*p = ResourceBoundPolicy(value)
	return nil
}

func validateBoundDefinitionArrays(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("REBAC-PARSEPOLICY-DEFINITIONS %w", err)
	}
	for _, name := range []string{"DEFATTRIBUTES", "DEFACLS", "DEFFORMULAS"} {
		if raw, present := fields[name]; present && string(raw) == "null" {
			return fmt.Errorf("REBAC-PARSEPOLICY-DEFINITIONS %s must be an array", name)
		}
	}
	return nil
}

// ResourceBoundKey normalizes the supported concrete resource bindings.
func ResourceBoundKey(object grammar.ObjectItem) (string, error) {
	switch object.Kind {
	case grammar.Route:
		return boundRouteKey(object.Route)
	case grammar.Identifiable:
		if object.Identifiable != nil && !object.Identifiable.ID.IsAll && object.Identifiable.ID.ID != "" {
			if object.Identifiable.Scope == "$aas" || object.Identifiable.Scope == "$sm" || object.Identifiable.Scope == "$cd" {
				return object.Identifiable.Scope + ":" + object.Identifiable.ID.ID, nil
			}
		}
	case grammar.Referable:
		return boundReferableKey(object.Referable)
	case grammar.Descriptor:
		if object.Descriptor != nil && !object.Descriptor.ID.IsAll && object.Descriptor.ID.ID != "" && (object.Descriptor.Scope == "$aasdesc" || object.Descriptor.Scope == "$smdesc") {
			return object.Descriptor.Scope + ":" + object.Descriptor.ID.ID, nil
		}
	}
	return "", fmt.Errorf("REBAC-RESOURCEKEY-UNSUPPORTED expected a concrete supported resource")
}

func boundRouteKey(route *grammar.RouteValue) (string, error) {
	if route == nil {
		return "", fmt.Errorf("REBAC-RESOURCEKEY-ROUTE missing route")
	}
	if isBoundCollection(strings.TrimPrefix(route.Route, "/")) {
		return "", fmt.Errorf("REBAC-RESOURCEKEY-ROUTE collection bindings are ABAC-only")
	}
	target, err := parseBoundTarget(route.Route, "")
	if err != nil {
		return "", err
	}
	if target.Access || target.Suffix != "" {
		return "", fmt.Errorf("REBAC-RESOURCEKEY-ROUTE expected a resource root")
	}
	if target.Kind == "discovery" {
		return "discovery:" + target.AAS, nil
	}
	return ResourceBoundKey(target.object())
}

func boundReferableKey(value *grammar.ReferableValue) (string, error) {
	if value == nil || value.ID.IsAll || value.ID.ID == "" || value.IDShortPath == "" || value.Scope != "$sme" {
		return "", fmt.Errorf("REBAC-RESOURCEKEY-REFERABLE expected a concrete SME")
	}
	encoded, err := json.Marshal([]string{value.ID.ID, value.IDShortPath})
	if err != nil {
		return "", fmt.Errorf("REBAC-RESOURCEKEY-ENCODE %w", err)
	}
	return "$sme:" + string(encoded), nil
}

// CompileResourceBoundPolicy reuses ABAC compilation after independent binding selection.
func CompileResourceBoundPolicy(policy ResourceBoundPolicy, router *chi.Mux, basePath string) (*AccessModel, error) {
	all := grammar.AccessRuleModelSchemaJSONAllAccessPermissionRules{DEFATTRIBUTES: policy.Attributes, DEFACLS: policy.ACLs, DEFFORMULAS: policy.Formulas}
	if _, err := ResourceBoundKey(policy.Resource); err != nil {
		return nil, err
	}
	if policy.Rules == nil {
		return nil, fmt.Errorf("REBAC-COMPILE-RULES rules must be an array")
	}
	for _, raw := range policy.Rules {
		rule, err := compileBoundRule(raw)
		if err != nil {
			return nil, err
		}
		all.Rules = append(all.Rules, rule)
	}
	rules, err := materializeRules(all)
	if err != nil {
		return nil, fmt.Errorf("REBAC-COMPILE-MATERIALIZE %w", err)
	}
	return &AccessModel{gen: grammar.AccessRuleModelSchemaJSON{AllAccessPermissionRules: all}, apiRouter: router, rules: rules, basePath: basePath}, nil
}

func compileBoundRule(raw json.RawMessage) (grammar.AccessPermissionRule, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return grammar.AccessPermissionRule{}, fmt.Errorf("REBAC-COMPILERULE-DECODE %w", err)
	}
	if fields == nil {
		return grammar.AccessPermissionRule{}, fmt.Errorf("REBAC-COMPILERULE-NULL rule must be an object")
	}
	for _, forbidden := range []string{"OBJECTS", "USEOBJECTS"} {
		if _, ok := fields[forbidden]; ok {
			return grammar.AccessPermissionRule{}, fmt.Errorf("REBAC-COMPILERULE-BINDING %s is not permitted", forbidden)
		}
	}
	fields["OBJECTS"] = json.RawMessage(`[{"ROUTE":"/*"}]`)
	data, err := json.Marshal(fields)
	if err != nil {
		return grammar.AccessPermissionRule{}, fmt.Errorf("REBAC-COMPILERULE-ENCODE %w", err)
	}
	var rule grammar.AccessPermissionRule
	if err = json.Unmarshal(data, &rule); err != nil {
		return rule, fmt.Errorf("REBAC-COMPILERULE-VALIDATE %w", err)
	}
	return rule, nil
}

func decodeBoundPolicy(raw []byte) (*ResourceBoundPolicy, error) {
	var policy ResourceBoundPolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return nil, fmt.Errorf("REBAC-DECODE-POLICY %w", err)
	}
	return &policy, nil
}
