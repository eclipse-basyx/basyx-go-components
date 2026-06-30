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

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/stretchr/testify/require"
)

func TestMatchRouteObjectsObjItem_RecentChangesSpecificObjectAddsIdentifierFilter(t *testing.T) {
	testCases := []struct {
		name        string
		object      grammar.ObjectItem
		path        string
		filterField string
	}{
		{
			name: "submodel",
			object: grammar.ObjectItem{
				Kind:         grammar.Identifiable,
				Identifiable: &grammar.IdentifiableValue{Scope: "$sm", ID: grammar.Identifier{ID: "submodel-1"}},
			},
			path:        "/submodels/$recent-changes",
			filterField: "$sm#id",
		},
		{
			name: "aas",
			object: grammar.ObjectItem{
				Kind:         grammar.Identifiable,
				Identifiable: &grammar.IdentifiableValue{Scope: "$aas", ID: grammar.Identifier{ID: "aas-1"}},
			},
			path:        "/shells/$recent-changes",
			filterField: "$aas#id",
		},
		{
			name: "concept description",
			object: grammar.ObjectItem{
				Kind:         grammar.Identifiable,
				Identifiable: &grammar.IdentifiableValue{Scope: "$cd", ID: grammar.Identifier{ID: "cd-1"}},
			},
			path:        "/concept-descriptions/$recent-changes",
			filterField: "$cd#id",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			access := matchRouteObjectsObjItem([]grammar.ObjectItem{testCase.object}, testCase.path, "")

			require.True(t, access.access)
			require.NotNil(t, access.le)
			require.Len(t, access.le.Or, 1)
			requireIdentifierFilter(t, access.le.Or[0], testCase.filterField)
		})
	}
}

func requireIdentifierFilter(t *testing.T, expr grammar.LogicalExpression, filterField string) {
	t.Helper()

	require.Len(t, expr.Eq, 2)
	require.NotNil(t, expr.Eq[0].Field)
	require.Equal(t, grammar.ModelStringPattern(filterField), *expr.Eq[0].Field)
	require.NotNil(t, expr.Eq[1].StrVal)
}
