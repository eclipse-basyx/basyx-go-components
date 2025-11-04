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
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
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
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse access model: %w", err)
	}
	return &AccessModel{gen: m}, nil
}

// EvalInput is the minimal set of request properties the ABAC engine needs to
// evaluate a decision. IssuedUTC should be in UTC.
type EvalInput struct {
	Method    string
	Path      string
	Claims    Claims
	IssuedUTC time.Time
}

// QueryFilter captures optional, fine-grained restrictions produced by a rule
// even when ACCESS=ALLOW. Controllers can use it to restrict rows, constrain
// mutations, or redact fields. The Discovery Service currently does not require
// a concrete filter structure; extend this struct when needed.
type QueryFilter struct {
	// TODO: not implemented because DiscoveryService does not need a concrete QueryFilter yet.
}

// AuthorizeWithFilter evaluates the request against the model rules in order.
// It returns whether access is allowed, a human-readable reason, and an optional
// QueryFilter for controllers to enforce (e.g., tenant scoping, redactions).
func (m *AccessModel) AuthorizeWithFilter(in EvalInput) (ok bool, reason string, qf *QueryFilter) {
	right := mapMethodToRight(in.Method)
	all := m.gen.AllAccessPermissionRules

	for _, r := range all.Rules {
		acl, attrs, objs, lexpr := materialize(all, r)

		fmt.Println("rule: ", r.USEOBJECTS)
		// Gate 1: rights
		if !rightsContains(acl.RIGHTS, right) {
			continue
		}
		// Gate 2: route
		if !matchRouteObjectsObjItem(objs, in.Path) {
			continue
		}
		// Gate 3: attributes
		if !attributesSatisfiedAttrs(attrs, in.Claims) {
			continue
		}
		// Gate 4: formula
		if lexpr != nil && !evalLE(*lexpr, in.Claims, in.IssuedUTC) {
			continue
		}

		// Optional data-level restrictions (to be defined by the product)
		qf = &QueryFilter{}

		switch acl.ACCESS {
		case grammar.ACLACCESSALLOW:
			return true, "ALLOW by rule", qf
		case grammar.ACLACCESSDISABLED:
			return false, "DENY (disabled) by rule", nil
		default:
			return false, "DENY (unknown access) by rule", nil
		}
	}
	return false, "no matching rule", nil
}

// Authorize evaluates the request and returns an allow/deny boolean and a
// reason, without producing a QueryFilter. Prefer AuthorizeWithFilter in new code.
func (m *AccessModel) Authorize(in EvalInput) (bool, string) {
	right := mapMethodToRight(in.Method)

	all := m.gen.AllAccessPermissionRules
	for _, r := range all.Rules {
		acl, attrs, objs, lexpr := materialize(all, r)

		if !rightsContains(acl.RIGHTS, right) {
			continue
		}
		// Note: currently rejects access if no route is defined.
		if !matchRouteObjectsObjItem(objs, in.Path) {
			continue
		}
		if !attributesSatisfiedAttrs(attrs, in.Claims) {
			continue
		}
		if lexpr != nil && !evalLE(*lexpr, in.Claims, in.IssuedUTC) {
			continue
		}

		switch acl.ACCESS {
		case grammar.ACLACCESSALLOW:
			return true, "ALLOW by rule"
		case grammar.ACLACCESSDISABLED:
			return false, "DENY (disabled) by rule"
		default:
			return false, "DENY (unknown access) by rule"
		}
	}

	return false, "no matching rule"
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

// attributesSatisfiedAttrs returns true if the provided claims satisfy at least
// one of the required attributes. Currently supports GLOBAL=ANONYMOUS and
// CLAIM=<claimKey> checks.
func attributesSatisfiedAttrs(items []grammar.AttributeItem, claims Claims) bool {
	for _, it := range items {
		switch it.Kind {
		case grammar.ATTRGLOBAL:
			if it.Value == "ANONYMOUS" {
				return true
			}
		case grammar.ATTRCLAIM:
			for key := range claims {
				if it.Value == key {
					return true
				}
			}
		}
	}
	return false
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

// matchRouteObjectsObjItem returns true if any ROUTE object matches the request
// path. Supports exact match, prefix match using "/*", and global wildcards.
func matchRouteObjectsObjItem(objs []grammar.ObjectItem, reqPath string) bool {
	req := normalize(reqPath)

	for _, oi := range objs {
		if oi.Kind != grammar.Route {
			continue
		}
		pat := normalize(oi.Value)

		if pat == "*" || pat == "/*" {
			return true
		}

		if strings.HasSuffix(pat, "/*") {
			base := strings.TrimSuffix(pat, "/*")
			if base != "" && strings.HasPrefix(req, base+"/") {
				return true
			}
			continue
		}

		if pat == req {
			return true
		}
	}
	return false
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
