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
// Author: BaSyx Authors

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

var (
	// ErrReBACRouteExcluded marks routes that must not be authorized as a ReBAC resource.
	ErrReBACRouteExcluded = errors.New("ReBAC route excluded")
	// ErrReBACRouteUnknown marks a path outside the supported resource route matrix.
	ErrReBACRouteUnknown = errors.New("ReBAC route unknown")
)

// ReBACRouteKind identifies the resource family targeted by an API.
type ReBACRouteKind string

// Supported resource families.
const (
	ReBACRouteKindShell              ReBACRouteKind = "aas"
	ReBACRouteKindSubmodel           ReBACRouteKind = "submodel"
	ReBACRouteKindElement            ReBACRouteKind = "element"
	ReBACRouteKindShellDescriptor    ReBACRouteKind = "aas_descriptor"
	ReBACRouteKindSubmodelDescriptor ReBACRouteKind = "submodel_descriptor"
	ReBACRouteKindConceptDescription ReBACRouteKind = "concept_description"
	ReBACRouteKindDiscovery          ReBACRouteKind = "discovery"
	ReBACRouteKindPackage            ReBACRouteKind = "package"
	ReBACRouteKindDPP                ReBACRouteKind = "dpp"
	ReBACRouteKindImport             ReBACRouteKind = "import"
	ReBACRouteKindSerialization      ReBACRouteKind = "serialization"
	ReBACRouteKindBulk               ReBACRouteKind = "bulk"
	ReBACRouteKindManagement         ReBACRouteKind = "management"
	ReBACRouteKindHistory            ReBACRouteKind = "history"
	ReBACRouteKindEvent              ReBACRouteKind = "event"
)

// ReBACRouteAction identifies the permission evaluated for a route.
type ReBACRouteAction string

// Supported data permissions.
const (
	ReBACRouteActionRead        ReBACRouteAction = "read"
	ReBACRouteActionUpdate      ReBACRouteAction = "update"
	ReBACRouteActionCreateChild ReBACRouteAction = "create_child"
	ReBACRouteActionExecute     ReBACRouteAction = "execute"
	ReBACRouteActionDelete      ReBACRouteAction = "delete"
	ReBACRouteActionCreate      ReBACRouteAction = "create"
)

// ReBACRouteTarget identifies the parent resource of a qualified child route.
type ReBACRouteTarget struct {
	Kind       ReBACRouteKind
	Identifier string
}

// ReBACRoute is the authorization-relevant interpretation of one API route.
// Identifier is decoded from base64url for AAS API identifiers. DPP and package
// identifiers are ordinary escaped path parameters and are returned as-is.
type ReBACRoute struct {
	Kind        ReBACRouteKind
	Identifier  string
	ElementPath string
	Action      ReBACRouteAction
	Collection  bool
	Management  string
	ABACOnly    bool
	Aggregate   bool
	Parent      *ReBACRouteTarget
}

// ClassifyReBACRoute classifies a supported API route without inspecting request
// state. It deliberately excludes history and event-feed routes and rejects
// unrecognised descendants so callers cannot authorize them as a parent object.
func ClassifyReBACRoute(method, requestPath string) (ReBACRoute, error) {
	if !isReBACMethod(method) {
		return ReBACRoute{}, fmt.Errorf("REBAC-CLASSIFY-METHOD %w: %s", ErrReBACRouteUnknown, method)
	}
	segments, err := rebacPathSegments(requestPath)
	if err != nil {
		return ReBACRoute{}, err
	}
	if route, excluded := classifyExcludedReBACRoute(method, segments); excluded {
		return route, nil
	}
	segments, management := splitReBACManagementSuffix(segments)
	route, err := classifyReBACSegments(method, segments)
	if err != nil {
		return ReBACRoute{}, err
	}
	if management != "" && route.Collection && (route.Parent != nil || !rebacManagedRepositoryKind(route.Kind)) {
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
	if management != "" {
		route.Management = management
	}
	return route, nil
}

func rebacPathSegments(requestPath string) ([]string, error) {
	path, _, _ := strings.Cut(requestPath, "?")
	if path == "" || !strings.HasPrefix(path, "/") || (path != "/" && strings.HasSuffix(path, "/")) {
		return nil, fmt.Errorf("REBAC-CLASSIFY-PATH %w: %q", ErrReBACRouteUnknown, requestPath)
	}
	if path == "/" {
		return nil, fmt.Errorf("REBAC-CLASSIFY-ROOT %w", ErrReBACRouteUnknown)
	}
	rawSegments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	segments := make([]string, len(rawSegments))
	for index, rawSegment := range rawSegments {
		segment, err := url.PathUnescape(rawSegment)
		if err != nil || segment == "" {
			return nil, fmt.Errorf("REBAC-CLASSIFY-UNESCAPE %w: %q", ErrReBACRouteUnknown, rawSegment)
		}
		segments[index] = segment
	}
	return segments, nil
}

func classifyExcludedReBACRoute(method string, segments []string) (ReBACRoute, bool) {
	if len(segments) == 0 {
		return ReBACRoute{}, false
	}
	if segments[0] == "events" || segments[0] == ".well-known" {
		return ReBACRoute{Kind: ReBACRouteKindEvent, Action: rebacAction(method), Collection: true, ABACOnly: true, Aggregate: true}, true
	}
	if len(segments) >= 2 && segments[0] == "v1" && segments[1] == "dppsByIdAndDate" {
		return ReBACRoute{Kind: ReBACRouteKindDPP, Action: rebacAction(method), Collection: true, ABACOnly: true, Aggregate: true}, true
	}
	for _, segment := range segments {
		if segment == "$history" || segment == "$recent-changes" {
			return ReBACRoute{Kind: ReBACRouteKindHistory, Action: rebacAction(method), ABACOnly: true}, true
		}
	}
	return ReBACRoute{}, false
}

func splitReBACManagementSuffix(segments []string) ([]string, string) {
	for index, segment := range segments {
		if segment != "$access" {
			continue
		}
		if index == len(segments)-1 {
			return segments[:index], "$access"
		}
		if index == len(segments)-2 && isAccessManagementChild(segments[index+1]) {
			return segments[:index], "$access/" + segments[index+1]
		}
		return segments, ""
	}
	return segments, ""
}

func isAccessManagementChild(segment string) bool {
	return segment == "grants" || segment == "effective" || segment == "inheritance"
}

func classifyReBACSegments(method string, segments []string) (ReBACRoute, error) {
	if len(segments) == 0 {
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
	switch segments[0] {
	case "shells":
		return classifyShellRoute(method, segments[1:])
	case "submodels":
		return classifySubmodelRoute(method, segments[1:], nil)
	case "concept-descriptions":
		return classifyIdentifierRoute(method, segments[1:], ReBACRouteKindConceptDescription, nil)
	case "shell-descriptors":
		return classifyShellDescriptorRoute(method, segments[1:])
	case "submodel-descriptors":
		return classifyIdentifierRoute(method, segments[1:], ReBACRouteKindSubmodelDescriptor, nil)
	case "lookup":
		return classifyDiscoveryRoute(method, segments[1:])
	case "packages":
		return classifyPackageRoute(method, segments[1:])
	case "packages-async", "upload":
		return classifyImportRoute(method, segments)
	case "bulk":
		return classifyBulkRoute(method, segments[1:])
	case "query":
		return classifyQueryRoute(method, segments[1:])
	case "v1":
		return classifyDPPRoute(method, segments[1:])
	case "serialization":
		return rebacAggregateRoute(method, ReBACRouteKindSerialization), nil
	case "security":
		return classifyReBACManagementRoute(method, segments[1:])
	default:
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
}

func classifyShellRoute(method string, segments []string) (ReBACRoute, error) {
	if len(segments) == 0 {
		return rebacCollectionRoute(method, ReBACRouteKindShell), nil
	}
	shell, err := decodeReBACIdentifier(segments[0])
	if err != nil {
		return ReBACRoute{}, err
	}
	route := rebacItemRoute(method, ReBACRouteKindShell, shell, nil)
	if len(segments) == 1 {
		return route, nil
	}
	if segments[1] == "submodels" && len(segments) >= 3 {
		parent := &ReBACRouteTarget{Kind: ReBACRouteKindShell, Identifier: shell}
		return classifySubmodelRoute(method, segments[2:], parent)
	}
	if isShellDescendant(segments[1:]) {
		return route, nil
	}
	return ReBACRoute{}, rebacUnknownRoute(segments)
}

func isShellDescendant(segments []string) bool {
	if len(segments) == 1 {
		return segments[0] == "$reference" || segments[0] == "$signed" || segments[0] == "asset-information" || segments[0] == "submodel-refs"
	}
	if len(segments) == 2 {
		return (segments[0] == "asset-information" && segments[1] == "thumbnail") || segments[0] == "submodel-refs"
	}
	return false
}

func classifySubmodelRoute(method string, segments []string, parent *ReBACRouteTarget) (ReBACRoute, error) {
	if len(segments) == 0 {
		route := rebacCollectionRoute(method, ReBACRouteKindSubmodel)
		route.Parent = parent
		return route, nil
	}
	submodel, err := decodeReBACIdentifier(segments[0])
	if err != nil {
		return ReBACRoute{}, err
	}
	route := rebacItemRoute(method, ReBACRouteKindSubmodel, submodel, parent)
	if len(segments) == 1 {
		return route, nil
	}
	if len(segments) == 2 && isSubmodelDescendant(segments[1:]) {
		return route, nil
	}
	if segments[1] == "submodel-elements" {
		return classifyElementRoute(method, segments[2:], routeTarget(route))
	}
	return ReBACRoute{}, rebacUnknownRoute(segments)
}

func isSubmodelDescendant(segments []string) bool {
	return len(segments) == 1 && (segments[0] == "$metadata" || segments[0] == "$value" || segments[0] == "$reference" || segments[0] == "$path" || segments[0] == "$signed")
}

func classifyElementRoute(method string, segments []string, parent *ReBACRouteTarget) (ReBACRoute, error) {
	if len(segments) == 0 {
		return rebacElementCollectionRoute(method, parent), nil
	}
	if isElementSubresource(segments[0]) {
		return classifyElementCollectionSubresource(method, segments, parent)
	}
	if strings.HasPrefix(segments[0], "$") {
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
	route := rebacItemRoute(method, ReBACRouteKindElement, "", parent)
	route.ElementPath = segments[0]
	return classifyElementItemRoute(method, segments[1:], route)
}

func rebacElementCollectionRoute(method string, parent *ReBACRouteTarget) ReBACRoute {
	route := rebacCollectionRoute(method, ReBACRouteKindElement)
	route.Parent = parent
	if method == http.MethodPost {
		route.Action = ReBACRouteActionCreateChild
	}
	return route
}

func classifyElementCollectionSubresource(method string, segments []string, parent *ReBACRouteTarget) (ReBACRoute, error) {
	if len(segments) != 1 {
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
	return rebacElementCollectionRoute(method, parent), nil
}

func classifyElementItemRoute(method string, segments []string, route ReBACRoute) (ReBACRoute, error) {
	if len(segments) == 0 {
		if method == http.MethodPost {
			route.Action = ReBACRouteActionCreateChild
		}
		return route, nil
	}
	if len(segments) == 1 && isElementSubresource(segments[0]) {
		return route, nil
	}
	if isElementOperationRoute(segments) {
		route.Action = ReBACRouteActionExecute
		return route, nil
	}
	if len(segments) == 1 && segments[0] == "attachment" {
		return route, nil
	}
	return ReBACRoute{}, rebacUnknownRoute(segments)
}

func isElementSubresource(segment string) bool {
	return segment == "$metadata" || segment == "$value" || segment == "$reference" || segment == "$path"
}

func isElementOperationRoute(segments []string) bool {
	if len(segments) == 1 {
		return segments[0] == "invoke" || segments[0] == "invoke-async"
	}
	if len(segments) == 2 {
		return (segments[0] == "invoke" || segments[0] == "invoke-async") && segments[1] == "$value" || (segments[0] == "operation-status" || segments[0] == "operation-results")
	}
	return len(segments) == 3 && segments[0] == "operation-results" && segments[2] == "$value"
}

func classifyShellDescriptorRoute(method string, segments []string) (ReBACRoute, error) {
	if len(segments) == 0 {
		return rebacCollectionRoute(method, ReBACRouteKindShellDescriptor), nil
	}
	shell, err := decodeReBACIdentifier(segments[0])
	if err != nil {
		return ReBACRoute{}, err
	}
	route := rebacItemRoute(method, ReBACRouteKindShellDescriptor, shell, nil)
	if len(segments) == 1 {
		return route, nil
	}
	if segments[1] != "submodel-descriptors" {
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
	return classifyIdentifierRoute(method, segments[2:], ReBACRouteKindSubmodelDescriptor, routeTarget(route))
}

func classifyIdentifierRoute(method string, segments []string, kind ReBACRouteKind, parent *ReBACRouteTarget) (ReBACRoute, error) {
	if len(segments) == 0 {
		route := rebacCollectionRoute(method, kind)
		route.Parent = parent
		return route, nil
	}
	if len(segments) != 1 {
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
	identifier, err := decodeReBACIdentifier(segments[0])
	if err != nil {
		return ReBACRoute{}, err
	}
	return rebacItemRoute(method, kind, identifier, parent), nil
}

func classifyDiscoveryRoute(method string, segments []string) (ReBACRoute, error) {
	if len(segments) == 1 && segments[0] == "shells" {
		route := rebacCollectionRoute(method, ReBACRouteKindDiscovery)
		route.Aggregate = true
		return route, nil
	}
	if len(segments) == 1 && segments[0] == "shellsByAssetLink" {
		return rebacAggregateRoute(method, ReBACRouteKindDiscovery), nil
	}
	if len(segments) == 2 && segments[0] == "shells" {
		identifier, err := decodeReBACIdentifier(segments[1])
		if err != nil {
			return ReBACRoute{}, err
		}
		route := rebacItemRoute(method, ReBACRouteKindDiscovery, identifier, nil)
		if method == http.MethodPost {
			route.Action = ReBACRouteActionUpdate
		}
		return route, nil
	}
	return ReBACRoute{}, rebacUnknownRoute(segments)
}

func classifyPackageRoute(method string, segments []string) (ReBACRoute, error) {
	if len(segments) == 0 {
		return rebacCollectionRoute(method, ReBACRouteKindPackage), nil
	}
	if len(segments) != 1 {
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
	identifier, err := common.DecodeAPIIdentifier(segments[0])
	if err != nil || identifier == "" {
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
	return rebacItemRoute(method, ReBACRouteKindPackage, identifier, nil), nil
}

func classifyImportRoute(method string, segments []string) (ReBACRoute, error) {
	if (len(segments) == 1 && (segments[0] == "upload" || segments[0] == "packages-async")) || (len(segments) == 3 && segments[0] == "packages-async" && (segments[1] == "status" || segments[1] == "result")) {
		return rebacAggregateRoute(method, ReBACRouteKindImport), nil
	}
	return ReBACRoute{}, rebacUnknownRoute(segments)
}

func classifyBulkRoute(method string, segments []string) (ReBACRoute, error) {
	if len(segments) == 1 && (segments[0] == "shell-descriptors" || segments[0] == "submodel-descriptors") {
		return rebacAggregateRoute(method, ReBACRouteKindBulk), nil
	}
	if len(segments) == 2 && (segments[0] == "status" || segments[0] == "result") {
		return rebacAggregateRoute(method, ReBACRouteKindBulk), nil
	}
	return ReBACRoute{}, rebacUnknownRoute(segments)
}

func classifyQueryRoute(method string, segments []string) (ReBACRoute, error) {
	if len(segments) != 1 {
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
	var route ReBACRoute
	switch segments[0] {
	case "shells":
		route = rebacAggregateRoute(method, ReBACRouteKindShell)
	case "submodels":
		route = rebacAggregateRoute(method, ReBACRouteKindSubmodel)
	case "concept-descriptions":
		route = rebacAggregateRoute(method, ReBACRouteKindConceptDescription)
	case "shell-descriptors":
		route = rebacAggregateRoute(method, ReBACRouteKindShellDescriptor)
	case "submodel-descriptors":
		route = rebacAggregateRoute(method, ReBACRouteKindSubmodelDescriptor)
	default:
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
	route.Action = ReBACRouteActionRead
	return route, nil
}

func classifyDPPRoute(method string, segments []string) (ReBACRoute, error) {
	if len(segments) == 0 || segments[0] != "dpps" && segments[0] != "dppsByProductId" && segments[0] != "dppsByIdAndDate" && segments[0] != "dppsByProductIds" {
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
	if segments[0] != "dpps" {
		route := rebacAggregateRoute(method, ReBACRouteKindDPP)
		if segments[0] == "dppsByProductIds" && method == http.MethodPost {
			route.Action = ReBACRouteActionRead
		}
		return route, nil
	}
	if len(segments) == 1 {
		return rebacAggregateRoute(method, ReBACRouteKindDPP), nil
	}
	route := rebacItemRoute(method, ReBACRouteKindDPP, segments[1], nil)
	if len(segments) == 2 || (len(segments) >= 3 && segments[2] == "elements") {
		return route, nil
	}
	return ReBACRoute{}, rebacUnknownRoute(segments)
}

func rebacCollectionRoute(method string, kind ReBACRouteKind) ReBACRoute {
	route := ReBACRoute{Kind: kind, Action: rebacAction(method), Collection: true}
	return route
}

func rebacAggregateRoute(method string, kind ReBACRouteKind) ReBACRoute {
	route := rebacCollectionRoute(method, kind)
	route.Aggregate = true
	return route
}

func rebacItemRoute(method string, kind ReBACRouteKind, identifier string, parent *ReBACRouteTarget) ReBACRoute {
	return ReBACRoute{Kind: kind, Identifier: identifier, Action: rebacAction(method), Parent: parent}
}

func rebacAction(method string) ReBACRouteAction {
	switch method {
	case http.MethodGet, http.MethodHead:
		return ReBACRouteActionRead
	case http.MethodPut, http.MethodPatch:
		return ReBACRouteActionUpdate
	case http.MethodDelete:
		return ReBACRouteActionDelete
	default:
		return ReBACRouteActionCreate
	}
}

func isReBACMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func routeTarget(route ReBACRoute) *ReBACRouteTarget {
	return &ReBACRouteTarget{Kind: route.Kind, Identifier: route.Identifier}
}

func decodeReBACIdentifier(segment string) (string, error) {
	identifier, err := common.DecodeAPIIdentifier(segment)
	if err != nil || identifier == "" {
		return "", fmt.Errorf("REBAC-CLASSIFY-DECODEIDENTIFIER %w: %q", ErrReBACRouteUnknown, segment)
	}
	return identifier, nil
}

func classifyReBACManagementRoute(method string, segments []string) (ReBACRoute, error) {
	if len(segments) == 3 && segments[0] == "rebac" && segments[1] == "audit" && (segments[2] == "verify" || segments[2] == "export") {
		segments = segments[:2]
	}
	if len(segments) != 2 || segments[0] != "rebac" || (segments[1] != "operations" && segments[1] != "audit") {
		return ReBACRoute{}, rebacUnknownRoute(segments)
	}
	route := rebacAggregateRoute(method, ReBACRouteKindManagement)
	route.Management = segments[1]
	if segments[1] == "operations" && method == http.MethodPost {
		route.Action = ReBACRouteActionExecute
	}
	return route, nil
}

func rebacUnknownRoute(segments []string) error {
	return fmt.Errorf("REBAC-CLASSIFY-UNKNOWN %w: /%s", ErrReBACRouteUnknown, strings.Join(segments, "/"))
}

func rebacManagedRepositoryKind(kind ReBACRouteKind) bool {
	switch kind {
	case ReBACRouteKindShell, ReBACRouteKindSubmodel, ReBACRouteKindShellDescriptor, ReBACRouteKindSubmodelDescriptor, ReBACRouteKindConceptDescription, ReBACRouteKindDiscovery, ReBACRouteKindPackage:
		return true
	default:
		return false
	}
}
