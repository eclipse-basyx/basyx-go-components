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

package auth

import (
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/stretchr/testify/require"
)

func TestMapDescriptorValueToRoute_SpecificDescriptor_DoesNotGrantBulkHandleRoutes(t *testing.T) {
	testCases := []struct {
		name  string
		scope string
	}{
		{name: "AAS descriptor", scope: "$aasdesc"},
		{name: "Submodel descriptor", scope: "$smdesc"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			descriptorID := "urn:example:descriptor:1"
			routes := mapDescriptorValueToRoute(grammar.DescriptorValue{
				Scope: testCase.scope,
				ID:    grammar.Identifier{ID: descriptorID},
			}, "")

			actualRoutes := extractMappedRoutes(routes)
			if testCase.scope == "$aasdesc" {
				require.Contains(t, actualRoutes, "/bulk/shell-descriptors")
			} else {
				require.Contains(t, actualRoutes, "/bulk/submodel-descriptors")
			}
			require.NotContains(t, actualRoutes, "/bulk/status/*")
			require.NotContains(t, actualRoutes, "/bulk/result/*")
			require.NotContains(t, actualRoutes, "/bulk/status/"+common.EncodeString(descriptorID))
			require.NotContains(t, actualRoutes, "/bulk/result/"+common.EncodeString(descriptorID))
		})
	}
}

func TestMatchRouteObjectsObjItem_DescriptorSpecific_DoesNotMatchBulkHandleRoutes(t *testing.T) {
	testCases := []struct {
		name           string
		scope          string
		statusPath     string
		resultPath     string
		otherRoutePath string
	}{
		{
			name:           "AAS descriptor",
			scope:          "$aasdesc",
			statusPath:     "/bulk/status/handle-123",
			resultPath:     "/bulk/result/handle-123",
			otherRoutePath: "/shell-descriptors/" + common.EncodeString("urn:example:other"),
		},
		{
			name:           "Submodel descriptor",
			scope:          "$smdesc",
			statusPath:     "/bulk/status/handle-456",
			resultPath:     "/bulk/result/handle-456",
			otherRoutePath: "/submodel-descriptors/" + common.EncodeString("urn:example:other"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			objs := []grammar.ObjectItem{
				{
					Kind: grammar.Descriptor,
					Descriptor: &grammar.DescriptorValue{
						Scope: testCase.scope,
						ID:    grammar.Identifier{ID: "urn:example:allowed"},
					},
				},
			}

			statusAccess := matchRouteObjectsObjItem(objs, testCase.statusPath, "")
			require.False(t, statusAccess.access)

			resultAccess := matchRouteObjectsObjItem(objs, testCase.resultPath, "")
			require.False(t, resultAccess.access)

			otherAccess := matchRouteObjectsObjItem(objs, testCase.otherRoutePath, "")
			require.False(t, otherAccess.access)
		})
	}
}

func extractMappedRoutes(routes []RouteWithFilter) []string {
	out := make([]string, 0, len(routes))
	for _, route := range routes {
		if strings.Contains(route.route, "%!") {
			continue
		}
		out = append(out, route.route)
	}
	return out
}
