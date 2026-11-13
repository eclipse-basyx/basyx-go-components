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
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

const (
	comparisonAASA = "urn:aas:a"
	comparisonAASB = "urn:aas:b"
	comparisonSMA  = "urn:sm:a"
	comparisonSMB  = "urn:sm:b"
)

var comparisonElementPaths = []string{"Coll", "Coll.x", "Coll[0]", "Coll[0].y", "Other"}

var comparisonSubmodelFields = []grammar.ModelStringPattern{"$sm#id", "$sm#idShort"}

// comparisonElementFields maps an element field to the representation that
// returns it besides the full element.
var comparisonElementFields = map[string]string{"value": "/$value", "idShort": "/$metadata"}

// TestSemanticRouteSubsetNeverExceedsDirectAccess compares, for every
// supported ROUTE shape, the data a query may observe with the data the same
// rule allows through direct requests, including data returned with a parent
// element, a whole Submodel or the query endpoint.
func TestSemanticRouteSubsetNeverExceedsDirectAccess(t *testing.T) {
	t.Parallel()

	sm := common.EncodeString(comparisonSMA)
	aas := common.EncodeString(comparisonAASA)
	for _, route := range []string{
		"*",
		"/*",
		"/query/submodels",
		"/submodels",
		"/submodels/$metadata",
		"/submodels/$reference",
		"/submodels/*",
		"/submodels/**",
		"/submodels/" + sm,
		"/submodels/" + sm + "/**",
		"/submodels/" + sm + "/$metadata",
		"/submodels/" + sm + "/$reference",
		"/submodels/" + sm + "/submodel-elements",
		"/submodels/" + sm + "/submodel-elements/*",
		"/submodels/" + sm + "/submodel-elements/Coll",
		"/submodels/" + sm + "/submodel-elements/Coll/**",
		"/submodels/" + sm + "/submodel-elements/Coll[0]",
		"/submodels/*/submodel-elements/Coll",
		"/shells/" + aas + "/submodels/" + sm,
		"/shells/" + aas + "/submodels/" + sm + "/submodel-elements/Coll",
		"/shells/*/submodels/" + sm + "/$metadata",
	} {
		t.Run(route, func(t *testing.T) {
			t.Parallel()
			assertSemanticCoverageWithinDirectAccess(t, route)
		})
	}
}

func TestSemanticRouteSubsetKeepsSupportedShapes(t *testing.T) {
	t.Parallel()

	sm := common.EncodeString(comparisonSMA)
	aas := common.EncodeString(comparisonAASA)
	for _, test := range []struct {
		route    string
		resource SemanticResourceKind
	}{
		{"*", SemanticResourceSME},
		{"/*", SemanticResourceSME},
		{"/query/submodels", SemanticResourceSME},
		{"/submodels/" + sm + "/submodel-elements/Coll[0].y", SemanticResourceSME},
		{"/shells/" + aas + "/submodels/" + sm + "/submodel-elements/Coll", SemanticResourceSME},
		{"/shells/*/submodels/" + sm, SemanticResourceSM},
		{"/submodels/" + sm + "=/submodel-elements/Coll", SemanticResourceSME},
	} {
		if _, covered := semanticSubmodelRouteCoverage(test.route, test.resource, ""); !covered {
			t.Errorf("supported route %q lost its %s coverage", test.route, test.resource)
		}
	}
}

func TestSemanticRouteSegmentsAreCheckedExactlyAsWritten(t *testing.T) {
	t.Parallel()

	sm := common.EncodeString(comparisonSMA)
	for _, route := range []string{
		"/submodels/" + sm + "/submodel-elements/ Coll",
		"/submodels/" + sm + "/submodel-elements/Coll ",
		"/submodels/ " + sm + "/submodel-elements/Coll",
		"/submodels/" + sm + " /submodel-elements/Coll",
		"/submodels/" + sm + "/submodel-elements/%41",
		"/submodels/" + sm + "/submodel-elements/a%25b",
		"/submodels/" + sm + "/submodel-elements/Coll%2Fx",
		"/submodels/" + sm + "/submodel-elements/Coll*",
		"/submodels/" + sm + "/submodel-elements/Coll[]",
		"/submodels/" + sm + "/submodel-elements/Coll[01]",
		"/submodels/" + sm + "/submodel-elements/Coll..x",
		"/submodels/" + sm + "==/submodel-elements/Coll",
		"/submodels/" + sm + "%3D/submodel-elements/Coll",
		"/submodels/YR/submodel-elements/Coll",
		"/submodels/YQ=/submodel-elements/Coll",
		"/submodels/YW+i/submodel-elements/Coll",
		"/submodels/" + common.EncodeString(" ") + "/submodel-elements/Coll",
		"/submodels/_w/submodel-elements/Coll",
		"/submodels/" + common.EncodeString("urn:\x01") + "/submodel-elements/Coll",
		"/shells/ " + common.EncodeString(comparisonAASA) + "/submodels/" + sm,
	} {
		for _, resource := range []SemanticResourceKind{SemanticResourceSM, SemanticResourceSME} {
			if _, covered := semanticSubmodelRouteCoverage(route, resource, ""); covered {
				t.Errorf("route %q produced %s coverage", route, resource)
			}
		}
	}
}

func assertSemanticCoverageWithinDirectAccess(t *testing.T, route string) {
	t.Helper()
	for _, aasID := range []string{comparisonAASA, comparisonAASB} {
		for _, submodelID := range []string{comparisonSMA, comparisonSMB} {
			for _, field := range comparisonSubmodelFields {
				assertObservationAllowed(t, route, SemanticResourceSM, field, aasID, submodelID, submodelRequestPaths(aasID, submodelID, field))
			}
			for _, elementPath := range comparisonElementPaths {
				for fragment, representation := range comparisonElementFields {
					field := grammar.ModelStringPattern("$sme." + elementPath + "#" + fragment)
					requestPaths := elementRequestPaths(aasID, submodelID, elementPath, representation)
					assertObservationAllowed(t, route, SemanticResourceSME, field, aasID, submodelID, requestPaths)
				}
			}
		}
	}
}

func assertObservationAllowed(
	t *testing.T,
	route string,
	resource SemanticResourceKind,
	field grammar.ModelStringPattern,
	aasID string,
	submodelID string,
	requestPaths []string,
) {
	t.Helper()
	coverage, covered := semanticSubmodelRouteCoverage(route, resource, "")
	if !covered || !coverageObserves(coverage, resource, field, aasID, submodelID) {
		return
	}
	for _, requestPath := range requestPaths {
		if matchRouteACL(route, requestPath) {
			return
		}
	}
	t.Errorf("route %q lets queries observe %s of Submodel %q in AAS %q, but no direct request is allowed", route, field, submodelID, aasID)
}

func coverageObserves(
	coverage semanticObjectCoverage,
	resource SemanticResourceKind,
	field grammar.ModelStringPattern,
	aasID string,
	submodelID string,
) bool {
	if coverage.enclosingAAS && !coverage.allAASIDs && coverage.aasID != aasID {
		return false
	}
	if !coverage.allSubmodelIDs && coverage.submodelID != submodelID {
		return false
	}
	if resource == SemanticResourceSM {
		return coverageCoversSubmodelField(coverage, field)
	}
	return coverageCoversSMEField(coverage, field, submodelElementPath(field))
}

// submodelRequestPaths lists the direct requests that return the field of a
// Submodel: the Submodel and the Submodel collection, their metadata, their
// references for the id, and the query endpoint, also below the enclosing AAS.
func submodelRequestPaths(aasID string, submodelID string, field grammar.ModelStringPattern) []string {
	suffixes := []string{"", "/$metadata"}
	if field == "$sm#id" {
		suffixes = append(suffixes, "/$reference")
	}
	paths := []string{"/query/submodels"}
	for _, base := range append(submodelBasePaths(aasID, submodelID), "/submodels") {
		for _, suffix := range suffixes {
			paths = append(paths, base+suffix)
		}
	}
	return paths
}

// elementRequestPaths lists the direct requests that return a field of an
// element: the element or one of its ancestors, in full or in the given
// representation, all elements, the whole Submodel, the Submodel collection
// and the query endpoint, also below the enclosing AAS.
func elementRequestPaths(aasID string, submodelID string, elementPath string, representation string) []string {
	paths := []string{"/query/submodels", "/submodels", "/submodels" + representation}
	for _, base := range submodelBasePaths(aasID, submodelID) {
		paths = append(paths, base, base+representation, base+"/submodel-elements", base+"/submodel-elements"+representation)
		for _, ancestor := range elementPathAncestors(elementPath) {
			elementRoute := base + "/submodel-elements/" + ancestor
			paths = append(paths, elementRoute, elementRoute+representation)
		}
	}
	return paths
}

func submodelBasePaths(aasID string, submodelID string) []string {
	encodedSubmodelID := common.EncodeString(submodelID)
	return []string{
		"/submodels/" + encodedSubmodelID,
		"/shells/" + common.EncodeString(aasID) + "/submodels/" + encodedSubmodelID,
	}
}

func elementPathAncestors(elementPath string) []string {
	ancestors := []string{}
	for index := 1; index < len(elementPath); index++ {
		if elementPath[index] == '.' || elementPath[index] == '[' {
			ancestors = append(ancestors, elementPath[:index])
		}
	}
	return append(ancestors, elementPath)
}
