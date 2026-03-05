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

// Package grammar defines the data structures for representing object items in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE ), Martin Stemmer ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// Route is an OBJECTTYPE value for route objects in the grammar model.
// Route items typically represent paths or endpoints within an AAS structure.
const Route OBJECTTYPE = "ROUTE"

// Identifiable is an OBJECTTYPE value for identifiable objects in the grammar model.
// Identifiable items have a globally meaningful identifier (e.g., AAS, submodel, or concept description).
const Identifiable OBJECTTYPE = "IDENTIFIABLE"

// Referable is an OBJECTTYPE value for referable objects in the grammar model.
// Referable items can be addressed via an idShort path within a scope but are not necessarily globally unique.
const Referable OBJECTTYPE = "REFERABLE"

// Fragment is an OBJECTTYPE value for fragment objects in the grammar model.
// Fragment items refer to addressed elements plus one or more string fragments
// (e.g., specific parts or segments of a representation).
const Fragment OBJECTTYPE = "FRAGMENT"

// Descriptor is an OBJECTTYPE value for descriptor objects in the grammar model.
// Descriptor items capture metadata about AAS or submodel descriptors.
const Descriptor OBJECTTYPE = "DESCRIPTOR"

// ObjectItem is a tagged union that holds exactly one of the supported grammar
// kinds (ROUTE, IDENTIFIABLE, REFERABLE, FRAGMENT, DESCRIPTOR). The Kind field
// indicates which variant is set, and exactly one of the pointer fields is non-nil.
//
// JSON forms supported:
//  1. Compact string grammar, e.g.:
//     {"ROUTE": "/api/submodels/123"}
//     {"IDENTIFIABLE": "$aas(\"someId\")"}
//     {"REFERABLE": "$sme(\"id\").path.to.sme"}
//     {"FRAGMENT": "$sme(\"id\").path \"frag1\" \"frag2\""}
//     {"DESCRIPTOR": "$smdesc(\"*\")"}
//  2. Structured objects, e.g.:
//     {"ROUTE": {"Route": "/api/submodels/123"}}
type ObjectItem struct {
	// Kind is the OBJECTTYPE of the contained item.
	Kind OBJECTTYPE `json:"-"`

	// Route holds the ROUTE value when Kind == Route.
	Route *RouteValue `json:"-"`

	// Identifiable holds the IDENTIFIABLE value when Kind == Identifiable.
	Identifiable *IdentifiableValue `json:"-"`

	// Referable holds the REFERABLE value when Kind == Referable.
	Referable *ReferableValue `json:"-"`

	// Fragment holds the FRAGMENT value when Kind == Fragment.
	Fragment *FragmentValue `json:"-"`

	// Descriptor holds the DESCRIPTOR value when Kind == Descriptor.
	Descriptor *DescriptorValue `json:"-"`
}

// Identifier represents an identifier reference used by IDENTIFIABLE,
// REFERABLE, FRAGMENT, and DESCRIPTOR values. If All is true, the value
// corresponds to the wildcard "*" in the compact grammar; otherwise ID
// contains the concrete identifier string.
type Identifier struct {
	// IsAll indicates that the identifier is the wildcard "*" (i.e., select all).
	IsAll bool

	// ID is the concrete identifier when All is false.
	ID string
}

// RouteValue is the typed representation of a ROUTE item. Route contains the
// literal route string as provided in the compact grammar.
type RouteValue struct {
	// Route is the literal route value (e.g., "/api/submodels/123").
	Route string
}

// IdentifiableValue is the typed representation of an IDENTIFIABLE item.
// Scope indicates the namespace ("$aas", "$sm", or "$cd"), and ID carries
// either a wildcard or concrete identifier.
type IdentifiableValue struct {
	// Scope is one of "$aas", "$sm", or "$cd".
	Scope string

	// ID is either a wildcard ("*") or a concrete identifier string.
	ID Identifier
}

// ReferableValue is the typed representation of a REFERABLE item.
// Scope must be "$sme". ID is a wildcard or concrete identifier, and
// IdShortPath is the dotted idShort path to the target element.
type ReferableValue struct {
	// Scope is "$sme".
	Scope string

	// ID is either a wildcard ("*") or a concrete identifier string.
	ID Identifier

	// IdShortPath is a dotted idShort path (e.g., "sub.a.b").
	IDShortPath string
}

// FragmentValue is the typed representation of a FRAGMENT item.
// Scope must be "$sme". ID is a wildcard or concrete identifier.
// IdShortPath is the addressed element path and Fragments contains one or
// more trailing string fragments from the compact grammar.
type FragmentValue struct {
	// Scope is "$sme".
	Scope string

	// ID is either a wildcard ("*") or a concrete identifier string.
	ID Identifier

	// IdShortPath is a dotted idShort path (e.g., "sub.a.b").
	iDShortPath string

	// Fragments are one or more trailing string fragments.
	Fragments []string
}

// DescriptorValue is the typed representation of a DESCRIPTOR item.
// Scope is "$aasdesc" or "$smdesc" and ID is a wildcard or concrete
// identifier string.
type DescriptorValue struct {
	// Scope is "$aasdesc" or "$smdesc".
	Scope string

	// ID is either a wildcard ("*") or a concrete identifier string.
	ID Identifier
}

// UnmarshalJSON implements json.Unmarshaler for ObjectItem. It accepts either
// the compact string grammar or a structured object for each OBJECTTYPE key.
// Exactly one key must be present, and it must be one of the supported kinds
// (ROUTE, IDENTIFIABLE, REFERABLE, FRAGMENT, DESCRIPTOR).
//
// Examples (compact form):
//
//	{"ROUTE": "/api/submodels/123"}
//	{"IDENTIFIABLE": "$aas(\"id\")"}
//	{"REFERABLE": "$sme(\"id\").path.to.sme"}
//	{"FRAGMENT": "$sme(\"id\").path \"frag1\" \"frag2\""}
//	{"DESCRIPTOR": "$smdesc(\"*\")"}
//
// Examples (structured form):
//
//	{"ROUTE": {"Route": "/api/submodels/123"}}
func (o *ObjectItem) UnmarshalJSON(value []byte) error {
	var raw map[string]json.RawMessage
	if err := common.UnmarshalAndDisallowUnknownFields(value, &raw); err != nil {
		return err
	}
	if len(raw) != 1 {
		return fmt.Errorf("ObjectItem: expected exactly one key, got %d", len(raw))
	}

	for k, v := range raw {
		kind := OBJECTTYPE(k)
		if !isAllowedKind(kind) {
			return fmt.Errorf("ObjectItem: invalid key %q (allowed: ROUTE, IDENTIFIABLE, REFERABLE, FRAGMENT, DESCRIPTOR)", k)
		}
		// Value can be a string (grammar) or already an object; we accept both.
		var s string

		if err := common.UnmarshalAndDisallowUnknownFields(v, &s); err == nil {
			// parse from string grammar
			switch kind {
			case Route:
				rv, err := parseRoute(s)
				if err != nil {
					return wrap(kind, err)
				}
				o.Kind, o.Route = Route, rv
			case Identifiable:
				iv, err := parseIdentifiable(s)
				if err != nil {
					return wrap(kind, err)
				}
				o.Kind, o.Identifiable = Identifiable, iv
			case Referable:
				rv, err := parseReferable(s)
				if err != nil {
					return wrap(kind, err)
				}
				o.Kind, o.Referable = Referable, rv
			case Fragment:
				fv, err := parseFragment(s)
				if err != nil {
					return wrap(kind, err)
				}
				o.Kind, o.Fragment = Fragment, fv
			case Descriptor:
				dv, err := parseDescriptor(s)
				if err != nil {
					return wrap(kind, err)
				}
				o.Kind, o.Descriptor = Descriptor, dv
			}
			return nil
		}

		// Value wasn’t a string—try structured object fallback (nice for tests/migrations)
		switch kind {
		case Route:
			var rv RouteValue
			if err := common.UnmarshalAndDisallowUnknownFields(v, &rv); err != nil {
				return err
			}
			o.Kind, o.Route = Route, &rv
		case Identifiable:
			var iv IdentifiableValue
			if err := common.UnmarshalAndDisallowUnknownFields(v, &iv); err != nil {
				return err
			}
			o.Kind, o.Identifiable = Identifiable, &iv
		case Referable:
			var rv ReferableValue
			if err := common.UnmarshalAndDisallowUnknownFields(v, &rv); err != nil {
				return err
			}
			o.Kind, o.Referable = Referable, &rv
		case Fragment:
			var fv FragmentValue
			if err := common.UnmarshalAndDisallowUnknownFields(v, &fv); err != nil {
				return err
			}
			o.Kind, o.Fragment = Fragment, &fv
		case Descriptor:
			var dv DescriptorValue
			if err := common.UnmarshalAndDisallowUnknownFields(v, &dv); err != nil {
				return err
			}
			o.Kind, o.Descriptor = Descriptor, &dv
		}
		return nil
	}
	return errors.New("ObjectItem: unreachable")
}

// MarshalJSON renders the compact schema form, e.g. {"ROUTE":"/x"} or {"DESCRIPTOR":"$smdesc(\"*\")"}.
func (o ObjectItem) MarshalJSON() ([]byte, error) {
	var (
		kind  string
		value string
		err   error
	)
	switch o.Kind {
	case Route:
		kind = string(Route)
		if o.Route == nil {
			return nil, errors.New("ObjectItem: ROUTE value is nil")
		}
		value = o.Route.Route
	case Identifiable:
		kind = string(Identifiable)
		if o.Identifiable == nil {
			return nil, errors.New("ObjectItem: IDENTIFIABLE value is nil")
		}
		value, err = formatIdentifiableValue(o.Identifiable.Scope, o.Identifiable.ID)
	case Referable:
		kind = string(Referable)
		if o.Referable == nil {
			return nil, errors.New("ObjectItem: REFERABLE value is nil")
		}
		value, err = formatReferableValue(o.Referable.Scope, o.Referable.ID, o.Referable.IDShortPath)
	case Fragment:
		kind = string(Fragment)
		if o.Fragment == nil {
			return nil, errors.New("ObjectItem: FRAGMENT value is nil")
		}
		value, err = formatFragmentValue(o.Fragment.Scope, o.Fragment.ID, o.Fragment.iDShortPath, o.Fragment.Fragments)
	case Descriptor:
		kind = string(Descriptor)
		if o.Descriptor == nil {
			return nil, errors.New("ObjectItem: DESCRIPTOR value is nil")
		}
		value, err = formatIdentifiableValue(o.Descriptor.Scope, o.Descriptor.ID)
	default:
		return nil, fmt.Errorf("ObjectItem: unsupported kind %q", o.Kind)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{kind: value})
}

// --- helpers ---

func isAllowedKind(k OBJECTTYPE) bool {
	switch k {
	case Route, Identifiable, Referable, Fragment, Descriptor:
		return true
	default:
		return false
	}
}

func wrap(kind OBJECTTYPE, err error) error {
	return fmt.Errorf("%s: %w", kind, err)
}

// --- Parsing according to your grammar ---

// ROUTE <ws> <RouteLiteral>
// For JSON, we expect just the <RouteLiteral> (no leading "ROUTE" token in the string).
func parseRoute(s string) (*RouteValue, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty route")
	}
	return &RouteValue{Route: s}, nil
}

// IDENTIFIABLE <ws> ("$aas" | "$sm" | "$cd") <IdentifierInstanceOrAll>
// Example accepted strings: `$aas("id")`, `$sm("*")`, `$cd("abc")`
var reIdentifiable = regexp.MustCompile(`^\s*(\$(?:aas|sm|cd))\s*\(\s*"(\*|[^"]+)"\s*\)\s*$`)

func parseIdentifiable(s string) (*IdentifiableValue, error) {
	m := reIdentifiable.FindStringSubmatch(s)
	if m == nil {
		return nil, fmt.Errorf("expected IDENTIFIABLE like `$aas(\"id\")` or `$sm(\"*\")`, got %q", s)
	}
	id := Identifier{}
	if m[2] == "*" {
		id.IsAll = true
	} else {
		id.ID = m[2]
	}
	return &IdentifiableValue{Scope: m[1], ID: id}, nil
}

// REFERABLE <ws> "$sme" <IdentifierInstanceOrAll> "." <idShortPath>
var reReferable = regexp.MustCompile(`^\s*(\$sme)\s*\(\s*"(\*|[^"]+)"\s*\)\s*\.\s*([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)\s*$`)

func parseReferable(s string) (*ReferableValue, error) {
	m := reReferable.FindStringSubmatch(s)
	if m == nil {
		return nil, fmt.Errorf("expected REFERABLE like `$sme(\"id\").path.to.sme` or `$sme(\"*\").x`, got %q", s)
	}
	id := Identifier{}
	if m[2] == "*" {
		id.IsAll = true
	} else {
		id.ID = m[2]
	}
	return &ReferableValue{Scope: m[1], ID: id, IDShortPath: m[3]}, nil
}

// FRAGMENT <ws> "$sme" <IdentifierInstanceOrAll> "." <idShortPath> ( <ws> <StringLiteral> )+
// We expect: `$sme("id").path "frag1" "frag2" ...`
var reFragmentHead = regexp.MustCompile(`^\s*(\$sme)\s*\(\s*"(\*|[^"]+)"\s*\)\s*\.\s*([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)`)

// capture trailing "..." segments
var reStringLit = regexp.MustCompile(`\s*"([^"]+)"`)

func parseFragment(s string) (*FragmentValue, error) {
	head := reFragmentHead.FindStringSubmatch(s)
	if head == nil {
		return nil, fmt.Errorf("expected FRAGMENT head like `$sme(\"id\").path` followed by one or more string literals, got %q", s)
	}
	tail := strings.TrimPrefix(s, head[0])
	frags := reStringLit.FindAllStringSubmatch(tail, -1)
	if len(frags) == 0 {
		return nil, fmt.Errorf("FRAGMENT requires at least one trailing string literal, got %q", s)
	}
	id := Identifier{}
	if head[2] == "*" {
		id.IsAll = true
	} else {
		id.ID = head[2]
	}
	out := &FragmentValue{
		Scope:       head[1],
		ID:          id,
		iDShortPath: head[3],
	}
	for _, f := range frags {
		out.Fragments = append(out.Fragments, f[1])
	}
	return out, nil
}

// DESCRIPTOR <ws> ("$aasdesc" | "$smdesc") <IdentifierInstanceOrAll>
var reDescriptor = regexp.MustCompile(`^\s*(\$(?:aasdesc|smdesc))\s*\(\s*"(\*|[^"]+)"\s*\)\s*$`)

func parseDescriptor(s string) (*DescriptorValue, error) {
	m := reDescriptor.FindStringSubmatch(s)
	if m == nil {
		return nil, fmt.Errorf("expected DESCRIPTOR like `$aasdesc(\"id\")` or `$smdesc(\"*\")`, got %q", s)
	}
	id := Identifier{}
	if m[2] == "*" {
		id.IsAll = true
	} else {
		id.ID = m[2]
	}
	return &DescriptorValue{Scope: m[1], ID: id}, nil
}

func formatIdentifiableValue(scope string, id Identifier) (string, error) {
	return fmt.Sprintf("%s(%s)", scope, quoteIdentifier(id)), nil
}

func formatReferableValue(scope string, id Identifier, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("ObjectItem: REFERABLE path is empty")
	}
	return fmt.Sprintf("%s(%s).%s", scope, quoteIdentifier(id), path), nil
}

func formatFragmentValue(scope string, id Identifier, path string, fragments []string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("ObjectItem: FRAGMENT path is empty")
	}
	if len(fragments) == 0 {
		return "", errors.New("ObjectItem: FRAGMENT requires at least one fragment")
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s(%s).%s", scope, quoteIdentifier(id), path))
	for _, frag := range fragments {
		b.WriteByte(' ')
		b.WriteString(strconv.Quote(frag))
	}
	return b.String(), nil
}

func quoteIdentifier(id Identifier) string {
	if id.IsAll {
		return strconv.Quote("*")
	}
	return strconv.Quote(id.ID)
}
