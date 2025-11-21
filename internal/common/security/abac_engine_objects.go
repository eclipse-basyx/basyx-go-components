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
	"path"
	"regexp"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

type descriptorRouteMapping struct {
	scope           string
	collectionRoute string
	itemRoute       string // printf-style format expecting the encoded ID or "*" for wildcard
	filterField     string // model string pattern used to generate the EQ filter
}

var descriptorRouteMappings = []descriptorRouteMapping{
	{
		scope:           "$aasdesc",
		collectionRoute: "/shell-descriptors",
		itemRoute:       "/shell-descriptors/%s",
		filterField:     "$aasdesc#id",
	},
	{
		scope:           "$smdesc",
		collectionRoute: "/shell-descriptors/*/submodel-descriptors",
		itemRoute:       "/shell-descriptors/*/submodel-descriptors/%s",
		filterField:     "$smdesc#id",
	},
}

// RouteWithFilter couples a concrete HTTP route pattern with an optional
// logical expression. When the route matches a request path, the expression
// (if present) is AND-ed into the overall filter used by the backend.
type RouteWithFilter struct {
	route string
	le    *grammar.LogicalExpression
}

func mapDescriptorValueToRoute(descriptorValue grammar.DescriptorValue) []RouteWithFilter {
	for _, mapping := range descriptorRouteMappings {
		if mapping.scope != descriptorValue.Scope {
			continue
		}

		if descriptorValue.ID.IsAll {
			return []RouteWithFilter{
				{route: mapping.collectionRoute},
				{route: fmt.Sprintf(mapping.itemRoute, "*")},
			}
		}

		rawID := descriptorValue.ID.ID
		encodedID := common.EncodeString(rawID)

		field := grammar.ModelStringPattern(mapping.filterField)
		standardString := grammar.StandardString(rawID)

		extraFilter := grammar.LogicalExpression{
			Eq: grammar.ComparisonItems{
				grammar.Value{Field: &field},
				grammar.Value{StrVal: &standardString},
			},
		}

		return []RouteWithFilter{
			{route: mapping.collectionRoute, le: &extraFilter},
			{route: fmt.Sprintf(mapping.itemRoute, encodedID)},
		}
	}

	return nil
}

// AccessWithLE represents the outcome of matching a request path against a
// set of object definitions. If access is true, the optional LogicalExpression
// can be used to further constrain the result set (e.g. as an additional
// backend filter).
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

			if matchRouteACL(oi.Route.Route, reqPath) {
				return AccessWithLE{access: true}
			}

		case grammar.Descriptor:
			desc := oi.Descriptor
			if desc != nil {
				for _, routeWithFilter := range mapDescriptorValueToRoute(*desc) {

					if matchRouteANT(routeWithFilter.route, reqPath) {
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

// --- HELPER ---

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

func matchRouteACL(pattern, path string) bool {
	pat := normalize(pattern)
	p := normalize(path)

	// Escape regex special chars, keep '*' for later
	regex := regexp.QuoteMeta(pat)

	// Simple ACL rule: * = .*
	regex = strings.ReplaceAll(regex, `\*`, `.*`)

	// Anchor
	regex = "^" + regex + "$"

	matched, _ := regexp.MatchString(regex, p)
	return matched
}

func matchRouteANT(route string, userPath string) bool {
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
