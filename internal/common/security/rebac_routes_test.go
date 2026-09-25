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
	"net/http"
	"reflect"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func TestClassifyReBACRoute(t *testing.T) {
	aasID := common.EncodeString("urn:example:aas:one")
	submodelID := common.EncodeString("https://example.org/submodel/one")
	cdID := common.EncodeString("https://example.org/cd/one")
	tests := []struct {
		name   string
		method string
		path   string
		want   ReBACRoute
	}{
		{
			name:   "nested element value route keeps decoded parent",
			method: http.MethodPatch,
			path:   "/shells/" + aasID + "/submodels/" + submodelID + "/submodel-elements/plant%2Emotor/$value",
			want: ReBACRoute{
				Kind: ReBACRouteKindElement, ElementPath: "plant.motor", Action: ReBACRouteActionUpdate,
				Parent: &ReBACRouteTarget{Kind: ReBACRouteKindSubmodel, Identifier: "https://example.org/submodel/one"},
			},
		},
		{
			name:   "submodel element creation is child creation",
			method: http.MethodPost,
			path:   "/submodels/" + submodelID + "/submodel-elements",
			want: ReBACRoute{
				Kind: ReBACRouteKindElement, Action: ReBACRouteActionCreateChild, Collection: true,
				Parent: &ReBACRouteTarget{Kind: ReBACRouteKindSubmodel, Identifier: "https://example.org/submodel/one"},
			},
		},
		{
			name:   "element invoke is execute",
			method: http.MethodPost,
			path:   "/submodels/" + submodelID + "/submodel-elements/operation/invoke/$value",
			want: ReBACRoute{
				Kind: ReBACRouteKindElement, ElementPath: "operation", Action: ReBACRouteActionExecute,
				Parent: &ReBACRouteTarget{Kind: ReBACRouteKindSubmodel, Identifier: "https://example.org/submodel/one"},
			},
		},
		{
			name:   "nested descriptor has shell descriptor parent",
			method: http.MethodGet,
			path:   "/shell-descriptors/" + aasID + "/submodel-descriptors/" + submodelID,
			want: ReBACRoute{
				Kind: ReBACRouteKindSubmodelDescriptor, Identifier: "https://example.org/submodel/one", Action: ReBACRouteActionRead,
				Parent: &ReBACRouteTarget{Kind: ReBACRouteKindShellDescriptor, Identifier: "urn:example:aas:one"},
			},
		},
		{
			name:   "concept description access management",
			method: http.MethodGet,
			path:   "/concept-descriptions/" + cdID + "/$access",
			want:   ReBACRoute{Kind: ReBACRouteKindConceptDescription, Identifier: "https://example.org/cd/one", Action: ReBACRouteActionRead, Management: "$access"},
		},
		{
			name:   "discovery collection is explicit aggregate",
			method: http.MethodGet,
			path:   "/lookup/shells",
			want:   ReBACRoute{Kind: ReBACRouteKindDiscovery, Action: ReBACRouteActionRead, Collection: true, Aggregate: true},
		},
		{
			name:   "package identifier is base64 decoded",
			method: http.MethodDelete,
			path:   "/packages/" + common.EncodeString("package-123"),
			want:   ReBACRoute{Kind: ReBACRouteKindPackage, Identifier: "package-123", Action: ReBACRouteActionDelete},
		},
		{
			name:   "bulk route is never a descriptor item",
			method: http.MethodPut,
			path:   "/bulk/shell-descriptors",
			want:   ReBACRoute{Kind: ReBACRouteKindBulk, Action: ReBACRouteActionUpdate, Collection: true, Aggregate: true},
		},
		{
			name:   "import route is aggregate",
			method: http.MethodPost,
			path:   "/packages-async",
			want:   ReBACRoute{Kind: ReBACRouteKindImport, Action: ReBACRouteActionCreate, Collection: true, Aggregate: true},
		},
		{
			name:   "dpp element route is a ReBAC target",
			method: http.MethodPatch,
			path:   "/v1/dpps/https%3A%2F%2Fexample.org%2Fdpp%2Fone/elements/a.b",
			want:   ReBACRoute{Kind: ReBACRouteKindDPP, Identifier: "https://example.org/dpp/one", Action: ReBACRouteActionUpdate},
		},
		{
			name:   "query post is read aggregate",
			method: http.MethodPost,
			path:   "/query/submodels",
			want:   ReBACRoute{Kind: ReBACRouteKindSubmodel, Action: ReBACRouteActionRead, Collection: true, Aggregate: true},
		},
		{
			name:   "serialization is explicit aggregate",
			method: http.MethodGet,
			path:   "/serialization",
			want:   ReBACRoute{Kind: ReBACRouteKindSerialization, Action: ReBACRouteActionRead, Collection: true, Aggregate: true},
		},
		{
			name:   "rebac audit management is explicit",
			method: http.MethodGet,
			path:   "/security/rebac/audit",
			want:   ReBACRoute{Kind: ReBACRouteKindManagement, Action: ReBACRouteActionRead, Collection: true, Aggregate: true, Management: "audit"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := ClassifyReBACRoute(testCase.method, testCase.path)
			if err != nil {
				t.Fatalf("ClassifyReBACRoute() error = %v", err)
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("ClassifyReBACRoute() = %#v, want %#v", got, testCase.want)
			}
		})
	}
}

func TestClassifyReBACRouteExcludesHistoryAndRejectsAmbiguousRoutes(t *testing.T) {
	identifier := common.EncodeString("urn:example:submodel")
	tests := []struct {
		path string
		want ReBACRoute
		err  error
	}{
		{path: "/submodels/" + identifier + "/$history", want: ReBACRoute{Kind: ReBACRouteKindHistory, Action: ReBACRouteActionRead, ABACOnly: true}},
		{path: "/events", want: ReBACRoute{Kind: ReBACRouteKindEvent, Action: ReBACRouteActionRead, Collection: true, ABACOnly: true, Aggregate: true}},
		{path: "/v1/dppsByIdAndDate/anything", want: ReBACRoute{Kind: ReBACRouteKindDPP, Action: ReBACRouteActionRead, Collection: true, ABACOnly: true, Aggregate: true}},
		{path: "/submodels/not-base64", err: ErrReBACRouteUnknown},
		{path: "/shells/" + identifier + "/unknown-child", err: ErrReBACRouteUnknown},
		{path: "/submodels/" + identifier + "/submodel-elements/$unknown", err: ErrReBACRouteUnknown},
		{path: "/concept-descriptions/" + identifier + "/$access/unknown", err: ErrReBACRouteUnknown},
	}

	for _, testCase := range tests {
		got, err := ClassifyReBACRoute(http.MethodGet, testCase.path)
		if testCase.err != nil {
			if !errors.Is(err, testCase.err) {
				t.Fatalf("ClassifyReBACRoute(%q) error = %v, want %v", testCase.path, err, testCase.err)
			}
			return
		}
		if err != nil || !reflect.DeepEqual(got, testCase.want) {
			t.Fatalf("ClassifyReBACRoute(%q) = %#v, %v; want %#v", testCase.path, got, err, testCase.want)
		}
	}
}

func TestDiscoveryReplacementResolvesCreationByExistence(t *testing.T) {
	route, err := ClassifyReBACRoute(http.MethodPost, "/lookup/shells/"+"dXJuOmRpc2NvdmVyeTpuZXc")
	if err != nil {
		t.Fatal(err)
	}
	if route.Action != ReBACRouteActionUpdate || route.Collection {
		t.Fatalf("unexpected replacement route: %+v", route)
	}
}

func TestPackageCanonicalPathRetainsOpaqueIdentifier(t *testing.T) {
	id := "opaque-package-with-slashes/and spaces"
	path, err := rebacCanonicalPath("package", id)
	if err != nil {
		t.Fatal(err)
	}
	route, err := ClassifyReBACRoute(http.MethodGet, path)
	if err != nil || route.Identifier != id {
		t.Fatalf("package identity round trip: %+v %v", route, err)
	}
}
