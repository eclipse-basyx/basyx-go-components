package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/pkg/schemagen"
)

// Keep your public types unchanged elsewhere.
type AccessModel struct {
	gen schemagen.AccessRuleModelSchemaJson
}

// Your existing callsites expect this signature.
func ParseAccessModel(b []byte) (*AccessModel, error) {
	var m schemagen.AccessRuleModelSchemaJson
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse access model: %w", err)
	}
	return &AccessModel{gen: m}, nil
}

// Your existing callsites expect this signature.
type EvalInput struct {
	Method string
	Path   string
	Claims Claims
	NowUTC time.Time
}

type QueryFilter struct {
	Where  string         // e.g. "identifier NOT IN (:ban1,:ban2)"
	Params map[string]any // e.g. {"ban1":"AAS-001","ban2":"AAS-002","tenant":"acme"}
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
			params[key] = claims[key] // may be string/json.Number/etc. (your toFloat handles numerics)
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
		if lexpr != nil && !evalLE(*lexpr, in.Claims, in.NowUTC) {
			continue
		}

		// ---- NEW: handle FILTER ----
		var out *QueryFilter
		if r.FILTER != nil {
			// pick condition: inline CONDITION or USEFORMULA
			var cond *schemagen.LogicalExpression
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
			// apply fragment only if no condition or condition true
			if cond == nil || evalLE(*cond, in.Claims, in.NowUTC) {
				if r.FILTER.FRAGMENT != nil && *r.FILTER.FRAGMENT != "" {
					where, params := renderFragment(*r.FILTER.FRAGMENT, in.Claims, in.NowUTC)
					out = &QueryFilter{Where: where, Params: params}
				}
			}
		}

		switch acl.ACCESS {
		case schemagen.ACLACCESSALLOW:
			return true, "ALLOW by rule", out
		case schemagen.ACLACCESSDISABLED:
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

		// rights (support "ALL")
		if !rightsContains(acl.RIGHTS, right) {
			continue
		}

		// objects (match {"ROUTE":"..."}; "*" wildcard supported)
		if !matchRouteObjectsObjItem(objs, in.Path) {
			continue
		}

		// attributes (inline + USEATTRIBUTES + DEFATTRIBUTES)
		if !attributesSatisfiedAttrs(attrs, in.Claims) {
			continue
		}

		// formula
		if lexpr != nil && !evalLE(*lexpr, in.Claims, in.NowUTC) {
			continue
		}

		switch acl.ACCESS {
		case schemagen.ACLACCESSALLOW:
			return true, "ALLOW by rule"
		case schemagen.ACLACCESSDISABLED:
			return false, "DENY (disabled) by rule"
		default:
			return false, "DENY (unknown access) by rule"
		}
	}

	return false, "no matching rule"
}

// ---------- internals (adapter helpers) ----------

func materialize(all schemagen.AccessRuleModelSchemaJsonAllAccessPermissionRules, r schemagen.AccessPermissionRule) (schemagen.ACL, []schemagen.AttributeItem, []schemagen.ObjectItem, *schemagen.LogicalExpression) {
	// ACL / USEACL
	acl := schemagen.ACL{}
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

	// ATTRIBUTES + USEATTRIBUTES (append)
	var attrs []schemagen.AttributeItem
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

	// OBJECTS + USEOBJECTS (resolve groups; handle one level of nesting)
	var objs []schemagen.ObjectItem
	if len(r.OBJECTS) > 0 {
		objs = append(objs, r.OBJECTS...)
	}
	if len(r.USEOBJECTS) > 0 {
		objs = append(objs, resolveObjects(all, r.USEOBJECTS)...)
	}

	// FORMULA / USEFORMULA
	var f *schemagen.LogicalExpression
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

func resolveObjects(all schemagen.AccessRuleModelSchemaJsonAllAccessPermissionRules, names []string) []schemagen.ObjectItem {
	var out []schemagen.ObjectItem
	for _, name := range names {
		for _, d := range all.DEFOBJECTS {
			if d.Name == name {
				if len(d.Objects) > 0 {
					out = append(out, d.Objects...)
				}
				// one level nested indirection
				if len(d.USEOBJECTS) > 0 {
					out = append(out, resolveObjects(all, d.USEOBJECTS)...)
				}
			}
		}
	}
	return out
}

func mapMethodToRight(meth string) schemagen.RightsEnum {
	switch strings.ToUpper(meth) {
	case http.MethodGet, http.MethodHead:
		return schemagen.RightsEnumREAD
	case http.MethodPost:
		return schemagen.RightsEnumCREATE
	case http.MethodPut, http.MethodPatch:
		return schemagen.RightsEnumUPDATE
	case http.MethodDelete:
		return schemagen.RightsEnumDELETE
	default:
		return schemagen.RightsEnumREAD
	}
}

func rightsContains(hay []schemagen.RightsEnum, needle schemagen.RightsEnum) bool {
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

// Reuse your existing attributesSatisfied logic by converting items to map[string]string.
func attributesSatisfiedAttrs(items []schemagen.AttributeItem, claims Claims) bool {
	if len(items) == 0 {
		return true
	}
	okAny := false
	for _, it := range items {
		m, ok := asStringMap(it)
		if !ok {
			continue
		}
		if g, ok := m["GLOBAL"]; ok {
			switch strings.ToUpper(g) {
			case "ANONYMOUS":
				okAny = true
			case "AUTHENTICATED":
				if claims != nil {
					okAny = true
				}
			}
		}
		if c, ok := m["CLAIM"]; ok {
			if v, exists := claims[c]; exists && fmt.Sprint(v) != "" {
				okAny = true
			}
		}
	}
	return okAny
}

func matchRouteObjectsObjItem(objs []schemagen.ObjectItem, reqPath string) bool {
	if len(objs) == 0 {
		return true
	}
	for _, oi := range objs {
		m, ok := asStringMap(oi)
		if !ok {
			continue
		}
		if pat, ok := m["ROUTE"]; ok {
			if pat == "*" {
				return true
			}
			if ok, _ := path.Match(pat, reqPath); ok {
				return true
			}
		}
	}
	return false
}

// Turn typed LogicalExpression into the generic Cond your evalExpr understands.
type Cond map[string]any

func logicalToCond(le schemagen.LogicalExpression) (Cond, error) {
	b, err := json.Marshal(le)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return Cond(m), nil
}

// Generic converter that tolerates interface{} coming from schemagen types.
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
