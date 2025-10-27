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
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"
)

type AccessModel struct {
	gen AccessRuleModelSchemaJson
}

func ParseAccessModel(b []byte) (*AccessModel, error) {
	var m AccessRuleModelSchemaJson
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse access model: %w", err)
	}
	return &AccessModel{gen: m}, nil
}

type EvalInput struct {
	Method    string
	Path      string
	Claims    Claims
	IssuedUTC time.Time
}

type QueryFilter struct {
	// todo: not implemented because DiscoveryService does not need a Query Filter
}

func (m *AccessModel) AuthorizeWithFilter(in EvalInput) (ok bool, reason string, qf *QueryFilter) {
	right := mapMethodToRight(in.Method)
	all := m.gen.AllAccessPermissionRules

	fmt.Println(in.Claims)

	for _, r := range all.Rules {
		acl, attrs, objs, lexpr := materialize(all, r)

		fmt.Println("rule: ", r.USEOBJECTS)
		if !rightsContains(acl.RIGHTS, right) {
			fmt.Println("missing rights")
			continue
		}
		if !matchRouteObjectsObjItem(objs, in.Path) {
			fmt.Println("missing object")
			continue
		}
		if !attributesSatisfiedAttrs(attrs, in.Claims) {
			fmt.Println("missing attributes")
			continue
		}
		if lexpr != nil && !evalLE(*lexpr, in.Claims, in.IssuedUTC) {
			fmt.Println("formula fails")
			continue
		}

		// todo
		qf = &QueryFilter{}

		switch acl.ACCESS {
		case ACLACCESSALLOW:
			return true, "ALLOW by rule", qf
		case ACLACCESSDISABLED:
			return false, "DENY (disabled) by rule", nil
		default:
			return false, "DENY (unknown access) by rule", nil
		}
	}
	return false, "no matching rule", nil
}

func (m *AccessModel) Authorize(in EvalInput) (bool, string) {
	right := mapMethodToRight(in.Method)

	all := m.gen.AllAccessPermissionRules
	for _, r := range all.Rules {
		acl, attrs, objs, lexpr := materialize(all, r)

		if !rightsContains(acl.RIGHTS, right) {
			continue
		}

		// todo: currently rejects access if no route is defined
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
		case ACLACCESSALLOW:
			return true, "ALLOW by rule"
		case ACLACCESSDISABLED:
			return false, "DENY (disabled) by rule"
		default:
			return false, "DENY (unknown access) by rule"
		}
	}

	return false, "no matching rule"
}

func materialize(all AccessRuleModelSchemaJsonAllAccessPermissionRules, r AccessPermissionRule) (ACL, []AttributeItem, []ObjectItem, *LogicalExpression) {
	// ACL / USEACL
	acl := ACL{}
	if r.ACL != nil {
		acl = *r.ACL
	} else if r.USEACL != nil {
		use := *r.USEACL
		for _, d := range all.DEFACLS {
			if d.Name == use {
				acl = d.Acl
				break
			}
		}
	}

	var attrs []AttributeItem
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

	var objs []ObjectItem
	if len(r.OBJECTS) > 0 {
		objs = append(objs, r.OBJECTS...)
	}
	if len(r.USEOBJECTS) > 0 {
		objs = append(objs, resolveObjects(all, r.USEOBJECTS)...)
	}

	var f *LogicalExpression
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

func resolveObjects(all AccessRuleModelSchemaJsonAllAccessPermissionRules, names []string) []ObjectItem {
	var out []ObjectItem
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

func mapMethodToRight(meth string) RightsEnum {
	switch strings.ToUpper(meth) {
	case http.MethodGet, http.MethodHead:
		return RightsEnumREAD
	case http.MethodPost:
		return RightsEnumCREATE
	case http.MethodPut, http.MethodPatch:
		return RightsEnumUPDATE
	case http.MethodDelete:
		return RightsEnumDELETE
	default:
		return RightsEnumREAD
	}
}

func rightsContains(hay []RightsEnum, needle RightsEnum) bool {
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
func attributesSatisfiedAttrs(items []AttributeItem, claims Claims) bool {

	for _, it := range items {

		switch it.Kind {
		case ATTRGLOBAL:
			if it.Value == "ANONYMOUS" {
				return true
			}
		case ATTRCLAIM:
			for key, _ := range claims {
				if it.Value == key {
					return true
				}
			}
		}
	}
	return false
}

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

func matchRouteObjectsObjItem(objs []ObjectItem, reqPath string) bool {
	req := normalize(reqPath)

	for _, oi := range objs {
		if oi.Kind != Route {
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

type Cond map[string]any

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
