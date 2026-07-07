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
	"errors"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestEnsureVisibleCreateSkipsReadWithoutQueryFilter(t *testing.T) {
	t.Parallel()

	err := EnsureVisibleCreate(
		context.Background(),
		existingDescriptor,
		readMustNotBeCalled,
		"descriptor already exists",
		"descriptor access not allowed",
	)

	require.Error(t, err)
	require.True(t, common.IsErrConflict(err))
}

func TestEnsureVisibleCreateSkipsReadWithUnrestrictedCreateFormula(t *testing.T) {
	t.Parallel()

	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		Formula: boolExpressionPtr(true),
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumCREATE: booleanExpression(true),
		},
	})

	err := EnsureVisibleCreate(
		ctx,
		existingDescriptor,
		readMustNotBeCalled,
		"descriptor already exists",
		"descriptor access not allowed",
	)

	require.Error(t, err)
	require.True(t, common.IsErrConflict(err))
}

func TestEnsureVisibleCreateReturnsConflictWhenFilteredReadAllowsDescriptor(t *testing.T) {
	t.Parallel()

	ctx := auth.WithQueryFilter(context.Background(), limitedCreateQueryFilter())

	err := EnsureVisibleCreate(
		ctx,
		existingDescriptor,
		func(context.Context) error { return nil },
		"descriptor already exists",
		"descriptor access not allowed",
	)

	require.Error(t, err)
	require.True(t, common.IsErrConflict(err))
}

func TestEnsureVisibleCreateReturnsDeniedWhenFilteredReadHidesDescriptor(t *testing.T) {
	t.Parallel()

	ctx := auth.WithQueryFilter(context.Background(), limitedCreateQueryFilter())

	err := EnsureVisibleCreate(
		ctx,
		existingDescriptor,
		func(context.Context) error { return common.NewErrNotFound("hidden descriptor") },
		"descriptor already exists",
		"descriptor access not allowed",
	)

	require.Error(t, err)
	require.True(t, common.IsErrDenied(err))
	require.False(t, common.IsErrConflict(err))
}

func TestEnsureVisibleCreateAllowsMissingDescriptor(t *testing.T) {
	t.Parallel()

	err := EnsureVisibleCreate(
		context.Background(),
		func(context.Context) (bool, error) { return false, nil },
		readMustNotBeCalled,
		"descriptor already exists",
		"descriptor access not allowed",
	)

	require.NoError(t, err)
}

func TestEnsureVisibleDuplicateAllowsMissingDescriptor(t *testing.T) {
	t.Parallel()

	err := EnsureVisibleDuplicate(
		context.Background(),
		false,
		readMustNotBeCalled,
		"descriptor already exists",
		"descriptor access not allowed",
	)

	require.NoError(t, err)
}

func TestEnsureVisibleDuplicateReturnsDeniedWhenFilteredReadHidesDescriptor(t *testing.T) {
	t.Parallel()

	ctx := auth.WithQueryFilter(context.Background(), limitedCreateQueryFilter())

	err := EnsureVisibleDuplicate(
		ctx,
		true,
		func(context.Context) error { return common.NewErrDenied("hidden descriptor") },
		"descriptor already exists",
		"descriptor access not allowed",
	)

	require.Error(t, err)
	require.True(t, common.IsErrDenied(err))
	require.False(t, common.IsErrConflict(err))
}

func TestEnsureVisibleCreatePropagatesCallbackErrors(t *testing.T) {
	t.Parallel()

	rawErr := errors.New("raw existence failed")
	err := EnsureVisibleCreate(
		context.Background(),
		func(context.Context) (bool, error) { return false, rawErr },
		func(context.Context) error { return nil },
		"descriptor already exists",
		"descriptor access not allowed",
	)
	require.ErrorIs(t, err, rawErr)

	filteredErr := errors.New("filtered read failed")
	ctx := auth.WithQueryFilter(context.Background(), limitedCreateQueryFilter())
	err = EnsureVisibleCreate(
		ctx,
		existingDescriptor,
		func(context.Context) error { return filteredErr },
		"descriptor already exists",
		"descriptor access not allowed",
	)
	require.ErrorIs(t, err, filteredErr)
}

func existingDescriptor(context.Context) (bool, error) {
	return true, nil
}

func readMustNotBeCalled(context.Context) error {
	return errors.New("read must not be called")
}

func limitedCreateQueryFilter() *auth.QueryFilter {
	return &auth.QueryFilter{
		Formula: boolExpressionPtr(false),
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumCREATE: booleanExpression(false),
		},
	}
}

func boolExpressionPtr(value bool) *grammar.LogicalExpression {
	expr := booleanExpression(value)
	return &expr
}

func booleanExpression(value bool) grammar.LogicalExpression {
	return grammar.LogicalExpression{Boolean: &value}
}
