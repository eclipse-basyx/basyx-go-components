package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"regexp"
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
	Where  string
	Params map[string]any
}

var paramRe = regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_]*)`)

func renderFragment(tpl string, claims Claims, now time.Time) (string, map[string]any) {
	params := map[string]any{}
	where := paramRe.ReplaceAllStringFunc(tpl, func(ph string) string {
		key := ph[1:]
		switch key {
		case "UTCNOW":
			params[key] = now.Format(time.RFC3339)
		default:
			params[key] = claims[key]
		}
		return ":" + key
	})
	return where, params
}

func (m *AccessModel) AuthorizeWithFilter(in EvalInput) (ok bool, reason string, qf *QueryFilter) {
	right := mapMethodToRight(in.Method)
	all := m.gen.AllAccessPermissionRules

	for _, r := range all.Rules {
		acl, attrs, objs, lexpr := materialize(all, r)

		if !rightsContains(acl.RIGHTS, right) {
			continue
		}
		if !matchRouteObjectsObjItem(objs, in.Path) {
			continue
		}
		if !attributesSatisfiedAttrs(attrs, in.Claims) {
			continue
		}
		if lexpr != nil && !evalLE(*lexpr, in.Claims, in.IssuedUTC) {
			continue
		}

		var out *QueryFilter
		if r.FILTER != nil {

			var cond *LogicalExpression
			switch {
			case r.FILTER.CONDITION != nil:
				cond = r.FILTER.CONDITION
			case r.FILTER.USEFORMULA != nil:
				use := *r.FILTER.USEFORMULA
				for _, d := range all.DEFFORMULAS {
					if d.Name == use {
						tmp := d.Formula
						cond = &tmp
						break
					}
				}
			}

			if cond == nil || evalLE(*cond, in.Claims, in.IssuedUTC) {
				if r.FILTER.FRAGMENT != nil && *r.FILTER.FRAGMENT != "" {
					where, params := renderFragment(*r.FILTER.FRAGMENT, in.Claims, in.IssuedUTC)
					out = &QueryFilter{Where: where, Params: params}
				}
			}
		}

		switch acl.ACCESS {
		case ACLACCESSALLOW:
			return true, "ALLOW by rule", out
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
