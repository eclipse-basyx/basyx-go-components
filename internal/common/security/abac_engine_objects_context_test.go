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
	"encoding/json"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/stretchr/testify/require"
)

func TestObjectContextSeparatesRouteAndIdentifiableMatching(t *testing.T) {
	for _, test := range []struct {
		name, object      string
		allowed, filtered bool
	}{
		{"actual route", `{"ROUTE":"/aggregate"}`, true, false},
		{"collection route", `{"ROUTE":"/submodels"}`, false, false},
		{"selected wildcard", `{"IDENTIFIABLE":"$sm(\"*\")"}`, true, false},
		{"selected identifier", `{"IDENTIFIABLE":"$sm(\"same-id\")"}`, true, true},
		{"other wildcard", `{"IDENTIFIABLE":"$aas(\"*\")"}`, false, false},
		{"other identifier", `{"IDENTIFIABLE":"$cd(\"same-id\")"}`, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var object grammar.ObjectItem
			require.NoError(t, json.Unmarshal([]byte(test.object), &object))
			match := matchObjects([]grammar.ObjectItem{object}, "/api/aggregate", "/api", &objectMatchContext{identifiableResource: SemanticResourceSM})
			require.Equal(t, test.allowed, match.access)
			require.Equal(t, test.filtered, match.le != nil)
		})
	}
}

func TestInvalidObjectContextDeniesEvenWildcardRoutes(t *testing.T) {
	var object grammar.ObjectItem
	require.NoError(t, json.Unmarshal([]byte(`{"ROUTE":"*"}`), &object))
	objects := []grammar.ObjectItem{object}
	require.True(t, matchObjects(objects, "/aggregate", "", nil).access)
	for _, resource := range []SemanticResourceKind{"", "invalid", SemanticResourceAASDesc, SemanticResourceSMDesc} {
		match := matchObjects(objects, "/aggregate", "", &objectMatchContext{identifiableResource: resource})
		require.False(t, match.access, resource)
		require.Nil(t, match.le)
	}
}

func TestObjectContextExcludesDescriptorReferableAndFragment(t *testing.T) {
	objects := []grammar.ObjectItem{
		{Kind: grammar.Descriptor, Descriptor: &grammar.DescriptorValue{Scope: "$smdesc", ID: grammar.Identifier{IsAll: true}}},
		{Kind: grammar.Referable, Referable: &grammar.ReferableValue{Scope: "$sme", ID: grammar.Identifier{IsAll: true}, IDShortPath: "Value"}},
		{Kind: grammar.Fragment, Fragment: &grammar.FragmentValue{Scope: "$sme", ID: grammar.Identifier{IsAll: true}, IDShortPath: "Value", Fragments: []string{"value"}}},
	}
	for _, requestPath := range []string{"/submodel-descriptors", "/submodels/a/submodel-elements", "/submodels/a/submodel-elements/Value"} {
		match := matchObjects(objects, requestPath, "", &objectMatchContext{identifiableResource: SemanticResourceSM})
		require.False(t, match.access, requestPath)
		require.Nil(t, match.le)
	}
	require.True(t, matchObjects(objects[:1], "/submodel-descriptors", "", nil).access)
	require.True(t, matchObjects(objects[1:2], "/submodels/a/submodel-elements/Value", "", nil).access)
}
