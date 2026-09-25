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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebac

import (
	"net/http"
	"strings"
)

// Route parameters of the covered AAS API routes.
const (
	paramAAS       = "aasIdentifier"
	paramSubmodel  = "submodelIdentifier"
	paramPath      = "idShortPath"
	paramCD        = "cdIdentifier"
	superpathShell = "/shells/{" + paramAAS + "}"
)

type routeTarget uint8

const (
	targetList routeTarget = iota
	targetAAS
	targetSubmodel
	targetConceptDescription
	targetElement
	targetSubmodelElements
)

type routeAction uint8

const (
	// actionRelation checks spec.relation on the target.
	actionRelation routeAction = iota
	// actionUpsert checks can_update on existing targets and creation rights otherwise.
	actionUpsert
	// actionCreate checks the repository creator relation.
	actionCreate
	// actionParentUpdate checks can_update on the parent of an element.
	actionParentUpdate
)

// routeSpec describes how ReBAC authorizes one covered route.
type routeSpec struct {
	target      routeTarget
	kind        ResourceKind
	action      routeAction
	relation    string
	aasRelation string
}

// routeMatrix maps "METHOD pattern" of covered routes to their spec. Routes
// that are not listed stay ABAC-only: history, $recent-changes, event feeds,
// $signed representations, verification and management routes.
type routeMatrix map[string]routeSpec

func routeKey(method string, pattern string) string {
	return method + " " + pattern
}

func (m routeMatrix) lookup(method string, pattern string) (routeSpec, bool) {
	spec, ok := m[routeKey(method, pattern)]
	return spec, ok
}

func (m routeMatrix) add(method string, pattern string, spec routeSpec) {
	m[routeKey(method, pattern)] = spec
}

func newRouteMatrix() routeMatrix {
	matrix := routeMatrix{}
	addListRoutes(matrix)
	addAASRoutes(matrix)
	addSubmodelRoutes(matrix, "", "")
	addSubmodelRoutes(matrix, superpathShell, PermissionRead)
	addConceptDescriptionRoutes(matrix)
	return matrix
}

func addListRoutes(matrix routeMatrix) {
	aasList := routeSpec{target: targetList, kind: KindAAS}
	submodelList := routeSpec{target: targetList, kind: KindSubmodel}
	cdList := routeSpec{target: targetList, kind: KindConceptDescription}
	for _, route := range []struct {
		method  string
		pattern string
		spec    routeSpec
	}{
		{http.MethodGet, "/shells", aasList},
		{http.MethodPost, "/query/shells", aasList},
		{http.MethodGet, "/shells/$reference", aasList},
		{http.MethodGet, "/submodels", submodelList},
		{http.MethodPost, "/query/submodels", submodelList},
		{http.MethodGet, "/submodels/$metadata", submodelList},
		{http.MethodGet, "/submodels/$value", submodelList},
		{http.MethodGet, "/submodels/$reference", submodelList},
		{http.MethodGet, "/submodels/$path", submodelList},
		{http.MethodGet, "/concept-descriptions", cdList},
		{http.MethodPost, "/query/concept-descriptions", cdList},
	} {
		matrix.add(route.method, route.pattern, route.spec)
	}
	matrix.add(http.MethodPost, "/shells", routeSpec{target: targetAAS, kind: KindAAS, action: actionCreate})
	matrix.add(http.MethodPost, "/submodels", routeSpec{target: targetSubmodel, kind: KindSubmodel, action: actionCreate})
	matrix.add(http.MethodPost, "/concept-descriptions", routeSpec{target: targetConceptDescription, kind: KindConceptDescription, action: actionCreate})
}

func addAASRoutes(matrix routeMatrix) {
	read := routeSpec{target: targetAAS, kind: KindAAS, relation: PermissionRead}
	update := routeSpec{target: targetAAS, kind: KindAAS, relation: PermissionUpdate}
	shell := superpathShell
	matrix.add(http.MethodGet, shell, read)
	matrix.add(http.MethodPut, shell, routeSpec{target: targetAAS, kind: KindAAS, action: actionUpsert})
	matrix.add(http.MethodDelete, shell, routeSpec{target: targetAAS, kind: KindAAS, relation: PermissionDelete})
	for _, suffix := range []string{"/$reference", "/asset-information", "/asset-information/thumbnail", "/submodel-refs"} {
		matrix.add(http.MethodGet, shell+suffix, read)
	}
	matrix.add(http.MethodPut, shell+"/asset-information", update)
	matrix.add(http.MethodPut, shell+"/asset-information/thumbnail", update)
	matrix.add(http.MethodDelete, shell+"/asset-information/thumbnail", update)
	matrix.add(http.MethodPost, shell+"/submodel-refs", update)
	matrix.add(http.MethodDelete, shell+"/submodel-refs/{"+paramSubmodel+"}", update)
}

// addSubmodelRoutes registers the Submodel routes below prefix. Superpaths
// below /shells/{aasIdentifier} additionally require aasRelation on the AAS,
// mirroring the existing reference check.
func addSubmodelRoutes(matrix routeMatrix, prefix string, aasRelation string) {
	submodel := prefix + "/submodels/{" + paramSubmodel + "}"
	elements := submodel + "/submodel-elements"
	element := elements + "/{" + paramPath + "}"
	withAAS := func(spec routeSpec, relation string) routeSpec {
		if prefix != "" {
			spec.aasRelation = relation
		}
		return spec
	}
	sm := func(relation string) routeSpec {
		return withAAS(routeSpec{target: targetSubmodel, kind: KindSubmodel, relation: relation}, aasRelation)
	}
	el := func(action routeAction, relation string) routeSpec {
		return withAAS(routeSpec{target: targetElement, kind: KindSubmodel, action: action, relation: relation}, aasRelation)
	}
	elementList := withAAS(routeSpec{target: targetSubmodelElements, kind: KindSubmodel, relation: PermissionRead}, aasRelation)

	matrix.add(http.MethodGet, submodel, sm(PermissionRead))
	matrix.add(http.MethodPut, submodel, withAAS(routeSpec{target: targetSubmodel, kind: KindSubmodel, action: actionUpsert}, PermissionUpdate))
	matrix.add(http.MethodDelete, submodel, withAAS(routeSpec{target: targetSubmodel, kind: KindSubmodel, relation: PermissionDelete}, PermissionUpdate))
	matrix.add(http.MethodPatch, submodel, sm(PermissionUpdate))
	for _, representation := range []string{"/$metadata", "/$value", "/$reference", "/$path"} {
		matrix.add(http.MethodGet, submodel+representation, sm(PermissionRead))
	}
	matrix.add(http.MethodPatch, submodel+"/$metadata", sm(PermissionUpdate))
	matrix.add(http.MethodPatch, submodel+"/$value", sm(PermissionUpdate))

	matrix.add(http.MethodGet, elements, elementList)
	for _, representation := range []string{"/$metadata", "/$value", "/$reference", "/$path"} {
		matrix.add(http.MethodGet, elements+representation, elementList)
		matrix.add(http.MethodGet, element+representation, el(actionRelation, PermissionRead))
	}
	matrix.add(http.MethodPost, elements, sm(PermissionUpdate))

	matrix.add(http.MethodGet, element, el(actionRelation, PermissionRead))
	matrix.add(http.MethodPut, element, el(actionUpsert, ""))
	matrix.add(http.MethodPost, element, el(actionRelation, PermissionUpdate))
	matrix.add(http.MethodDelete, element, el(actionParentUpdate, ""))
	matrix.add(http.MethodPatch, element, el(actionRelation, PermissionUpdate))
	matrix.add(http.MethodPatch, element+"/$metadata", el(actionRelation, PermissionUpdate))
	matrix.add(http.MethodPatch, element+"/$value", el(actionRelation, PermissionUpdate))
	matrix.add(http.MethodGet, element+"/attachment", el(actionRelation, PermissionRead))
	matrix.add(http.MethodPut, element+"/attachment", el(actionRelation, PermissionUpdate))
	matrix.add(http.MethodDelete, element+"/attachment", el(actionRelation, PermissionUpdate))
	for _, operation := range []string{"/invoke", "/invoke/$value", "/invoke-async", "/invoke-async/$value"} {
		matrix.add(http.MethodPost, element+operation, el(actionRelation, PermissionExecute))
	}
	for _, operation := range []string{"/operation-status/{handleId}", "/operation-results/{handleId}", "/operation-results/{handleId}/$value"} {
		matrix.add(http.MethodGet, element+operation, el(actionRelation, PermissionExecute))
	}
}

func addConceptDescriptionRoutes(matrix routeMatrix) {
	cd := "/concept-descriptions/{" + paramCD + "}"
	matrix.add(http.MethodGet, cd, routeSpec{target: targetConceptDescription, kind: KindConceptDescription, relation: PermissionRead})
	matrix.add(http.MethodPut, cd, routeSpec{target: targetConceptDescription, kind: KindConceptDescription, action: actionUpsert})
	matrix.add(http.MethodDelete, cd, routeSpec{target: targetConceptDescription, kind: KindConceptDescription, relation: PermissionDelete})
}

// isExcludedRoute reports routes that intentionally stay ABAC-only.
func isExcludedRoute(pattern string) bool {
	for _, marker := range []string{"/$history", "/$recent-changes", "/$signed", "/events", "/event-feed", "/security/", "/verify", "/description"} {
		if strings.Contains(pattern, marker) {
			return true
		}
	}
	return false
}
