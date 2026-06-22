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

package descriptors

import (
	"context"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func TestCanSkipCreateReadbackRequiresUnrestrictedCreateFormula(t *testing.T) {
	t.Parallel()

	allow := true
	deny := false

	tests := []struct {
		name string
		qf   *auth.QueryFilter
		want bool
	}{
		{
			name: "no query filter",
			want: true,
		},
		{
			name: "unrestricted create formula",
			qf: &auth.QueryFilter{
				FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
					grammar.RightsEnumCREATE: {Boolean: &allow},
				},
			},
			want: true,
		},
		{
			name: "restricted create formula",
			qf: &auth.QueryFilter{
				FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
					grammar.RightsEnumCREATE: {Boolean: &deny},
				},
			},
			want: false,
		},
		{
			name: "read-only unrestricted formula",
			qf: &auth.QueryFilter{
				FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
					grammar.RightsEnumREAD: {Boolean: &allow},
				},
			},
			want: false,
		},
		{
			name: "fragment filter",
			qf: &auth.QueryFilter{
				Filters: auth.FragmentFilters{
					grammar.FragmentStringPattern("$aas#idShort"): {Boolean: &allow},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			if tt.qf != nil {
				ctx = auth.WithQueryFilter(ctx, tt.qf)
			}

			if got := CanSkipCreateReadback(ctx); got != tt.want {
				t.Fatalf("CanSkipCreateReadback() = %v, want %v", got, tt.want)
			}
		})
	}
}
