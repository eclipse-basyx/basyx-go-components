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
	"path"
	"regexp"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

type scopedFilteredRouteMapping struct {
	scope       string
	route       string
	filterField string
	hasWildcard bool
}

type descriptorRouteMapping = scopedFilteredRouteMapping
type identifiableRouteMapping = scopedFilteredRouteMapping

type referableRouteMapping struct {
	scope       string
	route       string
	hasWildcard bool
	useFilter   bool
}

var descriptorRouteMappings = []descriptorRouteMapping{
	{
		scope:       "$aasdesc",
		route:       "/shell-descriptors",
		filterField: "$aasdesc#id",
		hasWildcard: false,
	},
	{
		scope:       "$aasdesc",
		route:       "/shell-descriptors/%s",
		filterField: "$aasdesc#id",
		hasWildcard: true,
	},
	{
		scope:       "$aasdesc",
		route:       "/shell-descriptors/%s/submodel-descriptors",
		filterField: "$aasdesc#id",
		hasWildcard: true,
	},
	{
		scope:       "$aasdesc",
		route:       "/shell-descriptors/%s/submodel-descriptors/*",
		filterField: "$aasdesc#id",
		hasWildcard: true,
	},
	{
		scope:       "$smdesc",
		route:       "/submodel-descriptors",
		filterField: "$smdesc#id",
		hasWildcard: false,
	},
	{
		scope:       "$smdesc",
		route:       "/submodel-descriptors/%s",
		filterField: "$smdesc#id",
		hasWildcard: true,
	},
}

var identifiableRouteMappings = []identifiableRouteMapping{
	// Submodel Repository collection/query endpoints use an additional filter
	// on $sm#id when a concrete IDENTIFIABLE is provided.
	{
		scope:       "$sm",
		route:       "/query/submodels",
		filterField: "$sm#id",
		hasWildcard: false,
	},
	{
		scope:       "$sm",
		route:       "/submodels",
		filterField: "$sm#id",
		hasWildcard: false,
	},
	{
		scope:       "$sm",
		route:       "/submodels/$metadata",
		filterField: "$sm#id",
		hasWildcard: false,
	},
	{
		scope:       "$sm",
		route:       "/submodels/$value",
		filterField: "$sm#id",
		hasWildcard: false,
	},
	{
		scope:       "$sm",
		route:       "/submodels/$reference",
		filterField: "$sm#id",
		hasWildcard: false,
	},
	{
		scope:       "$sm",
		route:       "/submodels/$path",
		filterField: "$sm#id",
		hasWildcard: false,
	},
	// Covers all concrete submodel endpoints under /submodels/{submodelIdentifier}/...
	{
		scope:       "$sm",
		route:       "/submodels/%s",
		filterField: "$sm#id",
		hasWildcard: true,
	},
	{
		scope:       "$sm",
		route:       "/submodels/%s/**",
		filterField: "$sm#id",
		hasWildcard: true,
	},
}

var referableRouteMappings = []referableRouteMapping{
	// Submodel element collection endpoints can be narrowed via an ABAC filter.
	{
		scope:       "$sme",
		route:       "/submodels/%s/submodel-elements",
		hasWildcard: true,
		useFilter:   true,
	},
	{
		scope:       "$sme",
		route:       "/submodels/%s/submodel-elements/$metadata",
		hasWildcard: true,
		useFilter:   true,
	},
	{
		scope:       "$sme",
		route:       "/submodels/%s/submodel-elements/$value",
		hasWildcard: true,
		useFilter:   true,
	},
	{
		scope:       "$sme",
		route:       "/submodels/%s/submodel-elements/$reference",
		hasWildcard: true,
		useFilter:   true,
	},
	{
		scope:       "$sme",
		route:       "/submodels/%s/submodel-elements/$path",
		hasWildcard: true,
		useFilter:   true,
	},
	// Exact element route and all element sub-resources.
	{
		scope:       "$sme",
		route:       "/submodels/%s/submodel-elements/%s",
		hasWildcard: true,
		useFilter:   false,
	},
	{
		scope:       "$sme",
		route:       "/submodels/%s/submodel-elements/%s/**",
		hasWildcard: true,
		useFilter:   false,
	},
}

// RouteWithFilter couples a concrete HTTP route pattern with an optional
// logical expression. When the route matches a request path, the expression
// (if present) is AND-ed into the overall filter used by the backend.
type RouteWithFilter struct {
	route string
	le    *grammar.LogicalExpression
}

func mapDescriptorValueToRoute(descriptorValue grammar.DescriptorValue, basePath string) []RouteWithFilter {
	return mapScopedIdentifierValueToRoute(
		descriptorValue.Scope,
		descriptorValue.ID,
		basePath,
		descriptorRouteMappings,
	)
}

func mapIdentifiableValueToRoute(identifiableValue grammar.IdentifiableValue, basePath string) []RouteWithFilter {
	return mapScopedIdentifierValueToRoute(
		identifiableValue.Scope,
		identifiableValue.ID,
		basePath,
		identifiableRouteMappings,
	)
}

func mapReferableValueToRoute(referableValue grammar.ReferableValue, basePath string) []RouteWithFilter {
	var routes = []RouteWithFilter{}

	if referableValue.Scope != "$sme" {
		return routes
	}

	idShortPath := strings.TrimSpace(referableValue.IDShortPath)
	if idShortPath == "" {
		return routes
	}

	lastPathSegment := idShortPath
	if idx := strings.LastIndex(idShortPath, "."); idx >= 0 && idx+1 < len(idShortPath) {
		lastPathSegment = idShortPath[idx+1:]
	}

	routeFilter := buildStringEqFilter("$sme."+idShortPath+"#idShort", lastPathSegment)

	for _, mapping := range referableRouteMappings {
		if mapping.scope != referableValue.Scope {
			continue
		}

		smIDPart := "*"
		if !referableValue.ID.IsAll {
			smIDPart = common.EncodeString(referableValue.ID.ID)
		}

		route := mapping.route
		if strings.Count(route, "%s") == 2 {
			route = fmt.Sprintf(route, smIDPart, idShortPath)
		} else {
			route = fmt.Sprintf(route, smIDPart)
		}

		if mapping.useFilter {
			routes = append(routes, RouteWithFilter{
				route: joinBasePath(basePath, route),
				le:    routeFilter,
			})
			continue
		}

		routes = append(routes, RouteWithFilter{route: joinBasePath(basePath, route)})
	}

	return routes
}

func mapScopedIdentifierValueToRoute(scope string, identifier grammar.Identifier, basePath string, mappings []scopedFilteredRouteMapping) []RouteWithFilter {
	routes := make([]RouteWithFilter, 0)
	for _, mapping := range mappings {
		if mapping.scope != scope {
			continue
		}

		if identifier.IsAll {
			routes = append(routes, buildWildcardRoute(basePath, mapping))
			continue
		}

		rawID := identifier.ID
		encodedID := common.EncodeString(rawID)
		extraFilter := buildStringEqFilter(mapping.filterField, rawID)

		if !mapping.hasWildcard {
			routes = append(routes, RouteWithFilter{
				route: joinBasePath(basePath, mapping.route),
				le:    extraFilter,
			})
		}

		routes = append(routes, RouteWithFilter{
			route: joinBasePath(basePath, fmt.Sprintf(mapping.route, encodedID)),
		})
	}

	return routes
}

func buildWildcardRoute(basePath string, mapping scopedFilteredRouteMapping) RouteWithFilter {
	if !mapping.hasWildcard {
		return RouteWithFilter{route: joinBasePath(basePath, mapping.route)}
	}

	return RouteWithFilter{
		route: joinBasePath(basePath, fmt.Sprintf(mapping.route, "*")),
	}
}

func buildStringEqFilter(fieldPath string, value string) *grammar.LogicalExpression {
	field := grammar.ModelStringPattern(fieldPath)
	standardString := grammar.StandardString(value)
	expr := grammar.LogicalExpression{
		Eq: grammar.ComparisonItems{
			grammar.Value{Field: &field},
			grammar.Value{StrVal: &standardString},
		},
	}
	return &expr
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
func matchRouteObjectsObjItem(objs []grammar.ObjectItem, reqPath string, basePath string) AccessWithLE {
	var logicalExpressions []grammar.LogicalExpression
	access := false
	for _, oi := range objs {
		switch oi.Kind {
		case grammar.Route:
			if matchRouteACL(joinBasePath(basePath, oi.Route.Route), reqPath) {
				return AccessWithLE{access: true}
			}
		case grammar.Descriptor:
			desc := oi.Descriptor
			if desc != nil {
				if appendMatchedMappedRoutes(mapDescriptorValueToRoute(*desc, basePath), reqPath, &access, &logicalExpressions) {
					return AccessWithLE{access: true}
				}
			}
		case grammar.Identifiable:
			identifiable := oi.Identifiable
			if identifiable != nil {
				if appendMatchedMappedRoutes(mapIdentifiableValueToRoute(*identifiable, basePath), reqPath, &access, &logicalExpressions) {
					return AccessWithLE{access: true}
				}
			}
		case grammar.Referable:
			referable := oi.Referable
			if referable != nil {
				if appendMatchedMappedRoutes(mapReferableValueToRoute(*referable, basePath), reqPath, &access, &logicalExpressions) {
					return AccessWithLE{access: true}
				}
			}
		}
	}

	var objectLogicalExpression *grammar.LogicalExpression
	if len(logicalExpressions) > 0 {
		objectLogicalExpression = &grammar.LogicalExpression{
			Or: logicalExpressions,
		}
	}
	return AccessWithLE{access: access, le: objectLogicalExpression}
}

func appendMatchedMappedRoutes(routes []RouteWithFilter, reqPath string, access *bool, logicalExpressions *[]grammar.LogicalExpression) bool {
	for _, routeWithFilter := range routes {
		if !matchRouteANT(routeWithFilter.route, reqPath) {
			continue
		}

		if routeWithFilter.le == nil {
			return true
		}

		*access = true
		*logicalExpressions = append(*logicalExpressions, *routeWithFilter.le)
	}

	return false
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

func matchRouteACL(pattern, userPath string) bool {
	pat := normalize(pattern)
	userPathNorm := normalize(userPath)

	// Escape regex special chars, keep '*' for later
	regex := regexp.QuoteMeta(pat)

	// Simple ACL rule: * = .*
	regex = strings.ReplaceAll(regex, `\*`, `.*`)

	// Anchor
	regex = "^" + regex + "$"

	matched, _ := regexp.MatchString(regex, userPathNorm)
	return matched
}

func matchRouteANT(route string, userPath string) bool {
	userPathNorm := normalize(userPath)
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

	matched, _ := regexp.MatchString(regexPattern, userPathNorm)
	return matched
}
