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
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestAASIdentifiableDoesNotGrantNestedSubmodelRoutes(t *testing.T) {
	t.Parallel()

	aasID := "urn:example:aas:1"
	encodedAASID := common.EncodeString(aasID)
	object := grammar.ObjectItem{
		Kind: grammar.Identifiable,
		Identifiable: &grammar.IdentifiableValue{
			Scope: "$aas",
			ID:    grammar.Identifier{ID: aasID},
		},
	}

	allowedPaths := []string{
		"/shells/" + encodedAASID,
		"/shells/" + encodedAASID + "/$reference",
		"/shells/" + encodedAASID + "/$history",
		"/shells/" + encodedAASID + "/$signed",
		"/shells/" + encodedAASID + "/asset-information",
		"/shells/" + encodedAASID + "/asset-information/thumbnail",
		"/shells/" + encodedAASID + "/submodel-refs",
		"/shells/" + encodedAASID + "/submodel-refs/submodel-id",
	}
	for _, requestPath := range allowedPaths {
		if !matchRouteObjectsObjItem([]grammar.ObjectItem{object}, requestPath, "").access {
			t.Fatalf("AAS object did not cover AAS route %q", requestPath)
		}
	}

	submodelPath := "/shells/" + encodedAASID + "/submodels/submodel-id"
	if matchRouteObjectsObjItem([]grammar.ObjectItem{object}, submodelPath, "").access {
		t.Fatalf("AAS object granted nested Submodel route %q", submodelPath)
	}
}

func TestSubmodelIdentifiableGrantsNestedEnvironmentRoute(t *testing.T) {
	t.Parallel()

	submodelID := "urn:example:sm:1"
	object := grammar.ObjectItem{
		Kind: grammar.Identifiable,
		Identifiable: &grammar.IdentifiableValue{
			Scope: "$sm",
			ID:    grammar.Identifier{ID: submodelID},
		},
	}
	requestPath := "/shells/aas-id/submodels/" + common.EncodeString(submodelID) + "/$value"
	if !matchRouteObjectsObjItem([]grammar.ObjectItem{object}, requestPath, "").access {
		t.Fatalf("Submodel object did not grant nested environment route %q", requestPath)
	}
}

func TestReferableGrantsSegmentBoundedDescendantRoutes(t *testing.T) {
	t.Parallel()

	submodelID := "urn:example:sm:1"
	encodedSubmodelID := common.EncodeString(submodelID)
	object := grammar.ObjectItem{
		Kind: grammar.Referable,
		Referable: &grammar.ReferableValue{
			Scope:       "$sme",
			ID:          grammar.Identifier{ID: submodelID},
			IDShortPath: "Metrics",
		},
	}

	allowedPaths := []string{
		"/submodels/" + encodedSubmodelID + "/submodel-elements/Metrics",
		"/submodels/" + encodedSubmodelID + "/submodel-elements/Metrics.Temperature",
		"/submodels/" + encodedSubmodelID + "/submodel-elements/Metrics[0].Value/$value",
		"/shells/aas-id/submodels/" + encodedSubmodelID + "/submodel-elements/Metrics.Temperature/attachment",
	}
	for _, requestPath := range allowedPaths {
		if !matchRouteObjectsObjItem([]grammar.ObjectItem{object}, requestPath, "").access {
			t.Fatalf("Referable object did not cover descendant route %q", requestPath)
		}
	}

	collisionPath := "/submodels/" + encodedSubmodelID + "/submodel-elements/MetricsPrivate.Temperature"
	if matchRouteObjectsObjItem([]grammar.ObjectItem{object}, collisionPath, "").access {
		t.Fatalf("Referable object widened into prefix-collision route %q", collisionPath)
	}
}

func TestFragmentObjectCoverageIsExact(t *testing.T) {
	t.Parallel()

	coverages := fragmentObjectCoverages(SemanticResourceSME, grammar.FragmentValue{
		Scope:       "$sme",
		ID:          grammar.Identifier{ID: "urn:example:sm:1"},
		IDShortPath: "Metrics.Temperature",
		Fragments:   []string{"value"},
	})
	if len(coverages) != 1 {
		t.Fatalf("fragmentObjectCoverages() returned %d entries, want 1", len(coverages))
	}
	coverage := coverages[0]
	for _, field := range []grammar.ModelStringPattern{
		"$sme.Metrics.Temperature#idShort",
		"$sme.Metrics.Temperature.Child#value",
	} {
		if coverageCoversSMEField(coverage, field, submodelElementPath(field)) {
			t.Fatalf("Fragment object unexpectedly covered %q", field)
		}
	}
	field := grammar.ModelStringPattern("$sme.Metrics.Temperature#value")
	if !coverageCoversSMEField(coverage, field, submodelElementPath(field)) {
		t.Fatalf("Fragment object did not cover %q", field)
	}

	genericTarget := SemanticAccessTarget{
		Resource: SemanticResourceSME,
		Field:    grammar.ModelStringPattern("$sme#value"),
	}
	expression, _, applicable, err := compileSMEObjectCoverage(coverage, genericTarget)
	if err != nil {
		t.Fatalf("compile Fragment object coverage: %v", err)
	}
	if !applicable {
		t.Fatal("Fragment object did not apply to its generic field target")
	}
	sql, _, err := goqu.Dialect("postgres").From("submodel_element").Where(expression).Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("render Fragment object coverage: %v", err)
	}
	if !strings.Contains(sql, "idshort_path") || strings.Contains(sql, "LIKE") {
		t.Fatalf("Fragment object did not compile to an exact path guard:\n%s", sql)
	}
}
