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

package aasregistrydatabase

import (
	"context"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestDescriptorWithCreatedAt(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	incoming := createdAt.Add(-time.Hour)
	descriptor := model.AssetAdministrationShellDescriptor{Id: "urn:test", CreatedAt: &incoming}
	tests := []struct {
		name    string
		ctx     context.Context
		include bool
	}{
		{name: "base registry", ctx: context.Background()},
		{name: "DTR", ctx: descriptors.WithIncludeAASDescriptorCreatedAt(context.Background()), include: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := descriptorWithCreatedAt(tt.ctx, descriptor, createdAt)
			jsonable, err := result.ToJsonable()
			require.NoError(t, err)
			if tt.include {
				require.Equal(t, &createdAt, result.CreatedAt)
				require.Contains(t, jsonable, "createdAt")
			} else {
				require.Nil(t, result.CreatedAt)
				require.NotContains(t, jsonable, "createdAt")
			}
		})
	}
}
