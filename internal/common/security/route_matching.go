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
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// Route patterns and authorization paths share one canonical notation: a "/"
// is always a separator of the router, and a slash or percent sign inside a
// path value is written as the encoded unit "%2F" or "%25". Wildcards consume
// whole units, so a match never starts or ends inside an encoded unit.
const (
	encodedSlashUnit   = "%2F"
	encodedPercentUnit = "%25"
	routeAnyUnits      = `(?:%2F|%25|[^%])*`
	routeSegmentUnits  = `(?:%2F|%25|[^%/])+`
)

var (
	authorizationSegmentEncoder = strings.NewReplacer("%", encodedPercentUnit, "/", encodedSlashUnit)
	encodedRouteUnitPattern     = regexp.MustCompile(`(?i)%2F|%25`)
)

// canonicalAuthorizationPath returns the path that ABAC matches ROUTE
// patterns against. It splits the request path at the separators the router
// sees, decodes each segment once and re-encodes only "%" and "/".
func canonicalAuthorizationPath(requestURL *url.URL) (string, error) {
	segments, err := routedPathSegments(requestURL)
	if err != nil {
		return "", err
	}
	lastIndex := len(segments) - 1
	for index, segment := range segments {
		if err := validateRequestPathSegment(segment, index > 0 && index < lastIndex); err != nil {
			return "", err
		}
		segments[index] = authorizationSegmentEncoder.Replace(segment)
	}
	return strings.Join(segments, "/"), nil
}

// routedPathSegments returns the decoded segments of the path the router
// matches. EscapedPath must not be used, because it falls back to re-escaping
// Path when RawPath holds bytes sent unescaped, which turns "%2F" into
// separators.
func routedPathSegments(requestURL *url.URL) ([]string, error) {
	segments := strings.Split(common.RoutedPath(requestURL), "/")
	for index, segment := range segments {
		decoded, err := common.DecodeRoutedPathParam(requestURL, segment)
		if err != nil {
			return nil, fmt.Errorf("SECURITY-AUTHPATH-DECODESEGMENT invalid percent-encoding in request path: %w", err)
		}
		segments[index] = decoded
	}
	return segments, nil
}

func validateRequestPathSegment(segment string, inner bool) error {
	switch {
	case segment == "." || segment == "..":
		return errors.New("SECURITY-AUTHPATH-DOTSEGMENT request path must not contain dot segments")
	case segment == "" && inner:
		return errors.New("SECURITY-AUTHPATH-EMPTYSEGMENT request path must not contain empty segments")
	case containsControlCharacter(segment):
		return errors.New("SECURITY-AUTHPATH-CONTROLCHAR request path must not contain control characters")
	default:
		return nil
	}
}

func containsControlCharacter(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < 0x20 || value[index] == 0x7F {
			return true
		}
	}
	return false
}

// normalizeRoutePath brings route patterns and authorization paths into an
// absolute form without a trailing slash. "*" stays the global wildcard.
func normalizeRoutePath(routePath string) string {
	switch routePath {
	case "":
		return "/"
	case "/", "*":
		return routePath
	}
	if !strings.HasPrefix(routePath, "/") {
		routePath = "/" + routePath
	}
	return strings.TrimSuffix(routePath, "/")
}

// canonicalRoutePattern validates a route pattern and returns its canonical
// spelling with upper-case encoded units.
func canonicalRoutePattern(pattern string) (string, error) {
	normalized := normalizeRoutePath(pattern)
	if normalized == "/" || normalized == "*" {
		return normalized, nil
	}
	segments := strings.Split(normalized, "/")
	for index := 1; index < len(segments); index++ {
		canonical, err := canonicalRoutePatternSegment(segments[index])
		if err != nil {
			return "", err
		}
		segments[index] = canonical
	}
	return strings.Join(segments, "/"), nil
}

func canonicalRoutePatternSegment(segment string) (string, error) {
	if segment == "" {
		return "", errors.New("SECURITY-ROUTEPATTERN-EMPTYSEGMENT route must not contain empty segments")
	}
	if segment == "." || segment == ".." {
		return "", errors.New("SECURITY-ROUTEPATTERN-DOTSEGMENT route must not contain dot segments")
	}
	if !strings.Contains(segment, "%") {
		return segment, nil
	}
	canonical := encodedRouteUnitPattern.ReplaceAllStringFunc(segment, strings.ToUpper)
	if strings.Count(canonical, "%") != len(encodedRouteUnitPattern.FindAllStringIndex(canonical, -1)) {
		return "", errors.New(`SECURITY-ROUTEPATTERN-PERCENT route may only use "%" in "%2F" (slash inside a value) or "%25" (percent sign inside a value)`)
	}
	return canonical, nil
}

// compileRoutePattern validates and compiles a route pattern. ROUTE patterns
// treat "*" as any units; mapped routes (segmentWildcards) treat "**" as any
// units and "*" as the units of one segment.
func compileRoutePattern(pattern string, segmentWildcards bool) (*regexp.Regexp, error) {
	canonical, err := canonicalRoutePattern(pattern)
	if err != nil {
		return nil, err
	}
	compiled, err := regexp.Compile(routePatternExpression(canonical, segmentWildcards))
	if err != nil {
		return nil, fmt.Errorf("SECURITY-ROUTEPATTERN-COMPILE route pattern cannot be compiled: %w", err)
	}
	return compiled, nil
}

func routePatternExpression(canonical string, segmentWildcards bool) string {
	expression := regexp.QuoteMeta(canonical)
	if segmentWildcards {
		expression = strings.ReplaceAll(expression, `\*\*`, routeAnyUnits)
		expression = strings.ReplaceAll(expression, `\*`, routeSegmentUnits)
	} else {
		expression = strings.ReplaceAll(expression, `\*`, routeAnyUnits)
	}
	return "^" + expression + "$"
}

func matchRoutePattern(pattern string, authorizationPath string, segmentWildcards bool) bool {
	compiled, err := compileRoutePattern(pattern, segmentWildcards)
	if err != nil {
		return false
	}
	return compiled.MatchString(normalizeRoutePath(authorizationPath))
}

// matchRouteACL matches a ROUTE object. "*" matches any units, including
// separators.
func matchRouteACL(pattern, authorizationPath string) bool {
	return matchRoutePattern(pattern, authorizationPath, false)
}

// matchRouteANT matches a route mapped from IDENTIFIABLE, REFERABLE or
// DESCRIPTOR objects. "**" matches any units and "*" the units of one segment.
func matchRouteANT(pattern, authorizationPath string) bool {
	return matchRoutePattern(pattern, authorizationPath, true)
}

// prepareRuleRoutes validates every route of the rules, so an invalid policy
// fails to load.
func prepareRuleRoutes(rules []materializedRule, basePath string) error {
	for ruleIndex, rule := range rules {
		for _, object := range rule.objs {
			if err := prepareObjectRoutes(object, basePath); err != nil {
				return fmt.Errorf("rule %d: %w", ruleIndex+1, err)
			}
		}
	}
	return nil
}

func prepareObjectRoutes(object grammar.ObjectItem, basePath string) error {
	if object.Kind == grammar.Route && object.Route != nil {
		if err := validateRouteObject(object.Route.Route, basePath); err != nil {
			return fmt.Errorf("ROUTE %q: %w", object.Route.Route, err)
		}
		return nil
	}
	for _, mapped := range mappedObjectRoutes(object, basePath) {
		if _, err := compileRoutePattern(mapped.route, true); err != nil {
			return fmt.Errorf("%s route %q: %w", object.Kind, mapped.route, err)
		}
	}
	return nil
}

func validateRouteObject(route string, basePath string) error {
	if route != strings.TrimSpace(route) {
		return errors.New("SECURITY-ROUTEPATTERN-WHITESPACE route must not start or end with whitespace")
	}
	_, err := compileRoutePattern(joinBasePath(basePath, route), false)
	return err
}

func mappedObjectRoutes(object grammar.ObjectItem, basePath string) []RouteWithFilter {
	switch {
	case object.Kind == grammar.Descriptor && object.Descriptor != nil:
		return mapDescriptorValueToRoute(*object.Descriptor, basePath)
	case object.Kind == grammar.Identifiable && object.Identifiable != nil:
		return mapIdentifiableValueToRoute(*object.Identifiable, basePath)
	case object.Kind == grammar.Referable && object.Referable != nil:
		return mapReferableValueToRoute(*object.Referable, basePath)
	default:
		return nil
	}
}
