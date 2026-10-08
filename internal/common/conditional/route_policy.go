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

package conditional

import "strings"

type identifierEncoding int

const (
	encodingBase64URL identifierEncoding = iota
	encodingPath
)

// RoutePolicy describes how conditional requests apply to one route.
type RoutePolicy struct {
	Mode     Mode
	Kind     Kind
	Param    string
	encoding identifierEncoding
}

type resourceRule struct {
	segments []string
	param    int
	kind     Kind
	encoding identifierEncoding
}

const anySegment = "{}"

// resourceRules map route prefixes to the top-level resource whose revision
// the route uses. Longer prefixes come first.
var resourceRules = []resourceRule{
	{segments: []string{"shells", anySegment, "submodels", anySegment}, param: 3, kind: KindSubmodel},
	{segments: []string{"lookup", "shells", anySegment}, param: 2, kind: KindDiscoveryEntry},
	{segments: []string{"shells", anySegment}, param: 1, kind: KindAAS},
	{segments: []string{"submodels", anySegment}, param: 1, kind: KindSubmodel},
	{segments: []string{"concept-descriptions", anySegment}, param: 1, kind: KindConceptDescription},
	{segments: []string{"shell-descriptors", anySegment}, param: 1, kind: KindAASDescriptor},
	{segments: []string{"submodel-descriptors", anySegment}, param: 1, kind: KindSubmodelDescriptor},
	{segments: []string{"packages", anySegment}, param: 1, kind: KindAASXPackage},
	{segments: []string{"companies", anySegment}, param: 1, kind: KindCompanyDescriptor},
	{segments: []string{"dpps", anySegment}, param: 1, kind: KindDPP, encoding: encodingPath},
}

var collectionRoots = map[string]Kind{
	"shells":               KindAAS,
	"submodels":            KindSubmodel,
	"concept-descriptions": KindConceptDescription,
	"shell-descriptors":    KindAASDescriptor,
	"submodel-descriptors": KindSubmodelDescriptor,
	"lookup":               KindDiscoveryEntry,
	"packages":             KindAASXPackage,
	"companies":            KindCompanyDescriptor,
	"dpps":                 KindDPP,
}

// excludedSegments mark representations without a stable entity tag and
// endpoints that do not address the resource itself.
var excludedSegments = map[string]bool{
	"$signed":           true,
	"$history":          true,
	"$recent-changes":   true,
	"operation-status":  true,
	"operation-results": true,
	"shellsByAssetLink": true,
	"$access":           true,
	"query":             true,
	"bulk":              true,
}

var verifyOnlySegments = map[string]bool{
	"invoke":       true,
	"invoke-async": true,
}

// Classify returns the conditional request policy of a route pattern. Leading
// segments before the first known resource root, such as a context path or an
// API version, are ignored.
func Classify(pattern string) RoutePolicy {
	all := strings.Split(strings.Trim(pattern, "/"), "/")
	for _, segment := range all {
		if excludedSegments[segment] {
			return RoutePolicy{Mode: ModeExcluded}
		}
	}
	segments := fromFirstRoot(all)
	if len(segments) == 0 {
		return RoutePolicy{Mode: ModeExcluded}
	}
	for _, rule := range resourceRules {
		if policy, ok := rule.match(segments); ok {
			return policy
		}
	}
	if len(segments) <= 2 {
		if kind, ok := collectionRoots[segments[0]]; ok {
			return RoutePolicy{Mode: ModeCollection, Kind: kind}
		}
	}
	return RoutePolicy{Mode: ModeExcluded}
}

func (rule resourceRule) match(segments []string) (RoutePolicy, bool) {
	if len(segments) < len(rule.segments) {
		return RoutePolicy{}, false
	}
	for index, expected := range rule.segments {
		isParam := isParamSegment(segments[index])
		if (expected == anySegment) != isParam || (!isParam && segments[index] != expected) {
			return RoutePolicy{}, false
		}
	}
	mode := ModeResource
	for _, segment := range segments[len(rule.segments):] {
		if verifyOnlySegments[segment] {
			mode = ModeVerifyOnly
		}
	}
	return RoutePolicy{Mode: mode, Kind: rule.kind, Param: paramName(segments[rule.param]), encoding: rule.encoding}, true
}

// fromFirstRoot drops the segments before the first known resource root.
func fromFirstRoot(segments []string) []string {
	for index, segment := range segments {
		if _, ok := collectionRoots[segment]; ok {
			return segments[index:]
		}
	}
	return nil
}

func isParamSegment(segment string) bool {
	return segment == "*" || (strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}"))
}

func paramName(segment string) string {
	name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
	if colon := strings.IndexByte(name, ':'); colon >= 0 {
		name = name[:colon]
	}
	return name
}
