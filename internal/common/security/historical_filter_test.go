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

func TestHistoricalReadRequiresBackendFilter(t *testing.T) {
	trueValue := true
	falseValue := false

	tests := []struct {
		name       string
		filter     *QueryFilter
		restricted bool
	}{
		{name: "no filter"},
		{name: "unrestricted formula", filter: &QueryFilter{Formula: &grammar.LogicalExpression{Boolean: &trueValue}}},
		{name: "restrictive formula", filter: &QueryFilter{Formula: &grammar.LogicalExpression{Boolean: &falseValue}}, restricted: true},
		{name: "backend formula", filter: &QueryFilter{Formula: &grammar.LogicalExpression{}}, restricted: true},
		{name: "fragment filter", filter: &QueryFilter{Filters: FragmentFilters{"$aas#idShort": {}}}, restricted: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := WithQueryFilter(t.Context(), test.filter)
			require.Equal(t, test.restricted, HistoricalReadRequiresBackendFilter(ctx))
		})
	}
}
