package auth

import (
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/stretchr/testify/require"
)

func TestMapDescriptorValueToRoute_SpecificDescriptor_UsesHandleWildcardRoutes(t *testing.T) {
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
			require.Contains(t, actualRoutes, "/bulk/status/*")
			require.Contains(t, actualRoutes, "/bulk/result/*")
			require.NotContains(t, actualRoutes, "/bulk/status/"+common.EncodeString(descriptorID))
			require.NotContains(t, actualRoutes, "/bulk/result/"+common.EncodeString(descriptorID))
		})
	}
}

func TestMatchRouteObjectsObjItem_DescriptorSpecific_MatchesBulkHandleRoutes(t *testing.T) {
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
			require.True(t, statusAccess.access)
			require.Nil(t, statusAccess.le)

			resultAccess := matchRouteObjectsObjItem(objs, testCase.resultPath, "")
			require.True(t, resultAccess.access)
			require.Nil(t, resultAccess.le)

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
