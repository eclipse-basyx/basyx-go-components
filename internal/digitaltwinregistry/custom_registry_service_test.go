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

package digitaltwinregistry

import (
	"context"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func TestGlobalAssetIDLookupReadUnrestrictedWithoutQueryFilter(t *testing.T) {
	t.Parallel()

	ctx := common.ContextWithConfig(context.Background(), &common.Config{})

	readUnrestricted, err := globalAssetIDLookupReadUnrestricted(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !readUnrestricted {
		t.Fatalf("expected globalAssetId lookup to be unrestricted without query filter")
	}
}

func TestGlobalAssetIDLookupReadUnrestrictedUsesActiveReadFormula(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		read bool
		want bool
	}{
		{name: "restricted", read: false, want: false},
		{name: "unrestricted", read: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := common.ContextWithConfig(context.Background(), &common.Config{})
			ctx = auth.WithQueryFilter(ctx, &auth.QueryFilter{
				FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
					grammar.RightsEnumREAD: {Boolean: &tt.read},
				},
			})

			readUnrestricted, err := globalAssetIDLookupReadUnrestricted(ctx)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if readUnrestricted != tt.want {
				t.Fatalf("expected readUnrestricted=%v, got %v", tt.want, readUnrestricted)
			}
		})
	}
}
