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
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	jsoniter "github.com/json-iterator/go"
)

// AccessModel is an evaluated, in-memory representation of the Access Rule Model
// (ARM) used by the ABAC engine. It holds the generated schema and provides
// evaluation helpers.
type AccessModel struct {
	gen grammar.AccessRuleModelSchemaJSON
}

// ParseAccessModel parses a JSON (or YAML converted to JSON) payload that
// conforms to the Access Rule Model schema and returns a compiled AccessModel.
func ParseAccessModel(b []byte) (*AccessModel, error) {
	var m grammar.AccessRuleModelSchemaJSON
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse access model: %w", err)
	}
	return &AccessModel{gen: m}, nil
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
	right := mapMethodToRight(in.Method)
	all := m.gen.AllAccessPermissionRules

	for _, r := range all.Rules {
		acl, attrs, objs, lexpr := materialize(all, r)

		// Gate 1: rights
		if !rightsContains(acl.RIGHTS, right) {
			fmt.Println("method mismatch")
			continue
		}
		// Gate 2: attributes
		if !attributesSatisfiedAll(attrs, in.Claims) {
			fmt.Println("missing claims")
			continue
		}
		// Gate 3: objects
		accessWithOptinalFilter := matchRouteObjectsObjItem(objs, in.Path)
		if !accessWithOptinalFilter.access {
			fmt.Println("no matching object")
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
		if combinedLE != nil {
			adapted, onlyBool := adaptLEForBackend(*combinedLE, in.Claims, in.IssuedUTC)
			if onlyBool {
				fmt.Println("security only LE")
				// Fully decidable here; evaluate and continue on false
				if !evalLE(adapted, in.Claims, in.IssuedUTC) {
					continue
				}
			} else {
				fmt.Println("got a expression from LE")
				qf = &QueryFilter{Formula: &adapted, Filter: r.FILTER}

			}
		}

		return true, DecisionAllow, qf
	}
	return false, DecisionNoMatch, nil
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

// resolveObjects expands DEFOBJECTS references (including nested USEOBJECTS)
// into a concrete object list.
func resolveObjects(all grammar.AccessRuleModelSchemaJSONAllAccessPermissionRules, names []string) []grammar.ObjectItem {
	var out []grammar.ObjectItem
	for _, name := range names {
		for _, d := range all.DEFOBJECTS {
			if d.Name == name {
				if len(d.Objects) > 0 {
					out = append(out, d.Objects...)
				}
				if len(d.USEOBJECTS) > 0 {
					out = append(out, resolveObjects(all, d.USEOBJECTS)...)
				}
			}
		}
	}
	return out
}

// mapMethodToRight maps an HTTP method into an abstract right used by the
// Access Rule Model (CREATE, READ, UPDATE, DELETE). Unknown methods default to READ.
func mapMethodToRight(meth string) grammar.RightsEnum {
	switch strings.ToUpper(meth) {
	case http.MethodGet, http.MethodHead:
		return grammar.RightsEnumREAD
	case http.MethodPost:
		return grammar.RightsEnumCREATE
	case http.MethodPut, http.MethodPatch:
		return grammar.RightsEnumUPDATE
	case http.MethodDelete:
		return grammar.RightsEnumDELETE
	default:
		return grammar.RightsEnumREAD
	}
}

// rightsContains returns true if the required right is included in the rule's
// rights, or if the rule grants ALL rights.
func rightsContains(hay []grammar.RightsEnum, needle grammar.RightsEnum) bool {
	for _, r := range hay {
		if strings.EqualFold(string(r), "ALL") {
			return true
		}
		if strings.EqualFold(string(r), string(needle)) {
			return true
		}
	}
	return false
}

// attributesSatisfiedAll returns true only if ALL required attributes are satisfied.
// Rules supported:
//   - GLOBAL=ANONYMOUS         → satisfied unconditionally
//   - CLAIM=<claimKey>         → user must have that claim key (presence check)
//
// If items is empty, it returns true. Unknown kinds fail closed (return false).
func attributesSatisfiedAll(items []grammar.AttributeItem, claims Claims) bool {
	// No required attributes → allowed
	if len(items) == 0 {
		return true
	}

	for _, it := range items {
		switch it.Kind {
		case grammar.ATTRGLOBAL:
			// Currently only ANONYMOUS is supported per your comment.
			if it.Value == "ANONYMOUS" {
				// satisfied → continue checking the rest
				continue
			}
			// Unsupported GLOBAL value → fail closed
			return false

		case grammar.ATTRCLAIM:
			// Presence-only check: user must have this claim key
			fmt.Println("it.Value")
			fmt.Println(it.Value)
			fmt.Println(claims)
			if _, ok := claims[it.Value]; !ok {
				return false
			}

		default:
			// Unknown attribute type → fail closed
			return false
		}
	}

	// All attributes satisfied
	return true
}

// normalize cleans route patterns and request paths into a consistent absolute
// form suitable for matching. "*" and "/*" are treated as wildcards.
func normalize(p string) string {
	if p == "" {
		return "/"
	}
	if p != "*" && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = path.Clean(p)
	return p
}

func matchRoute(route string, userPath string) bool {
	path := normalize(userPath)
	pat := normalize(route)

	// Escape regex chars first, keeping '*' as literal "\*"
	regexPattern := regexp.QuoteMeta(pat)

	// Replace ** before * (so we don't double-handle)
	// ** -> .*
	regexPattern = strings.ReplaceAll(regexPattern, `\*\*`, `.*`)
	// *  -> [^/]+  (one segment, no slash)
	regexPattern = strings.ReplaceAll(regexPattern, `\*`, `[^/]+`)

	// Anchor the pattern
	regexPattern = "^" + regexPattern + "$"

	matched, _ := regexp.MatchString(regexPattern, path)
	return matched
}

type RouteWithFilter struct {
	route string
	le    *grammar.LogicalExpression
}

func mapDesciptorValueToRoute(descriptorValue grammar.DescriptorValue) []RouteWithFilter {
	switch descriptorValue.Scope {
	case "$aasdesc":
		if descriptorValue.ID.IsAll {
			return []RouteWithFilter{
				{route: "/shell-descriptors"},
				{route: "/shell-descriptors/*"},
			}
		} else {
			id := descriptorValue.ID.ID
			encodeID := common.EncodeString(id)
			field := grammar.ModelStringPattern("$aasdesc#id")
			standardString := grammar.StandardString(id)

			extra_filter := grammar.LogicalExpression{
				Eq: grammar.ComparisonItems{
					grammar.Value{Field: &field},
					grammar.Value{StrVal: &standardString},
				},
			}

			return []RouteWithFilter{
				{route: "/shell-descriptors", le: &extra_filter},
				{route: "/shell-descriptors/" + encodeID},
			}
		}

	case "$smdesc":
		if descriptorValue.ID.IsAll {
			return []RouteWithFilter{
				{route: "/shell-descriptors/*/submodel-descriptors"},
				{route: "/shell-descriptors/*/submodel-descriptors/*"},
			}
		} else {
			id := descriptorValue.ID.ID
			encodeID := common.EncodeString(id)

			field := grammar.ModelStringPattern("$smdesc#id")
			standardString := grammar.StandardString(id)

			extra_filter := grammar.LogicalExpression{
				Eq: grammar.ComparisonItems{
					grammar.Value{Field: &field},
					grammar.Value{StrVal: &standardString},
				},
			}

			return []RouteWithFilter{
				// collection route + filter on submodel-descriptor id
				{route: "/shell-descriptors/*/submodel-descriptors", le: &extra_filter},
				// direct item route
				{route: "/shell-descriptors/*/submodel-descriptors/" + encodeID},
			}
		}
	}

	return []RouteWithFilter{}
}

type AccessWithLE struct {
	access bool
	le     *grammar.LogicalExpression
}

// matchRouteObjectsObjItem returns true if any ROUTE object matches the request
// path. Supports exact match, prefix match using "/*", and global wildcards.
func matchRouteObjectsObjItem(objs []grammar.ObjectItem, reqPath string) AccessWithLE {

	var locialExpressions []grammar.LogicalExpression
	access := false
	for _, oi := range objs {

		switch oi.Kind {
		case grammar.Route:

			if matchRoute(oi.Route.Route, reqPath) {
				return AccessWithLE{access: true}
			}

		case grammar.Descriptor:
			desc := oi.Descriptor
			if desc != nil {
				for _, routeWithFilter := range mapDesciptorValueToRoute(*desc) {

					if matchRoute(routeWithFilter.route, reqPath) {
						if routeWithFilter.le != nil {
							access = true
							locialExpressions = append(locialExpressions, *routeWithFilter.le)
						} else {
							return AccessWithLE{access: true}
						}
					}
				}
			}
		}

	}

	var objectLogicalExpression *grammar.LogicalExpression
	if len(locialExpressions) > 0 {
		objectLogicalExpression = &grammar.LogicalExpression{
			Or: locialExpressions,
		}
	}
	return AccessWithLE{access: access, le: objectLogicalExpression}
}

// Cond is a helper alias used by logical evaluation and utility functions.
type Cond map[string]any

// asStringMap attempts to normalize arbitrary map-like values into a
// map[string]string, best-effort. Useful when claims or attributes may be
// represented heterogeneously by upstream libraries.
func asStringMap(v any) (map[string]string, bool) {
	switch vv := v.(type) {
	case map[string]string:
		return vv, true
	case map[string]any:
		out := make(map[string]string, len(vv))
		for k, val := range vv {
			out[k] = fmt.Sprint(val)
		}
		return out, true
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var m map[string]any
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, false
		}
		out := make(map[string]string, len(m))
		for k, val := range m {
			out[k] = fmt.Sprint(val)
		}
		return out, true
	}
}
