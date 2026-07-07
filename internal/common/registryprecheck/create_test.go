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

package registryprecheck

import (
	"context"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestEnsureVisibleCreateUsesReadScopedFormula(t *testing.T) {
	t.Parallel()

	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumCREATE: booleanExpression(true),
			grammar.RightsEnumREAD:   booleanExpression(false),
		},
	})

	err := EnsureVisibleCreate(
		ctx,
		existingDescriptor,
		readUsingActiveFormula,
		"descriptor already exists",
		"descriptor access not allowed",
	)

	require.Error(t, err)
	require.True(t, common.IsErrDenied(err))
	require.False(t, common.IsErrConflict(err))
}

func TestEnsureVisibleCreateReturnsConflictWhenReadScopedFormulaAllowsDescriptor(t *testing.T) {
	t.Parallel()

	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumCREATE: booleanExpression(true),
			grammar.RightsEnumREAD:   booleanExpression(true),
		},
	})

	err := EnsureVisibleCreate(
		ctx,
		existingDescriptor,
		readUsingActiveFormula,
		"descriptor already exists",
		"descriptor access not allowed",
	)

	require.Error(t, err)
	require.True(t, common.IsErrConflict(err))
}

func TestEnsureVisibleCreateFailsClosedWhenReadScopedFormulaIsMissing(t *testing.T) {
	t.Parallel()

	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumCREATE: booleanExpression(true),
		},
	})

	err := EnsureVisibleCreate(
		ctx,
		existingDescriptor,
		readUsingActiveFormula,
		"descriptor already exists",
		"descriptor access not allowed",
	)

	require.Error(t, err)
	require.True(t, common.IsErrDenied(err))
	require.False(t, common.IsErrConflict(err))
}

func existingDescriptor(context.Context) (bool, error) {
	return true, nil
}

func readUsingActiveFormula(ctx context.Context) error {
	queryFilter := auth.GetQueryFilter(ctx)
	if queryFilter == nil || queryFilter.Formula == nil || queryFilter.Formula.Boolean == nil {
		return nil
	}
	if *queryFilter.Formula.Boolean {
		return nil
	}
	return common.NewErrNotFound("descriptor hidden by read formula")
}

func booleanExpression(value bool) grammar.LogicalExpression {
	return grammar.LogicalExpression{Boolean: &value}
}
