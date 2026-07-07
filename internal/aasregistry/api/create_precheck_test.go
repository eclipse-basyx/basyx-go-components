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

package aasregistryapi

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/registryprecheck"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestRegistryCreateExistingUnauthorizedDescriptorDoesNotReturnConflict(t *testing.T) {
	t.Parallel()

	ctx := auth.WithQueryFilter(context.Background(), limitedCreateQueryFilter())

	err := registryprecheck.EnsureVisibleCreate(
		ctx,
		func(context.Context) (bool, error) { return true, nil },
		func(context.Context) error { return common.NewErrNotFound("hidden descriptor") },
		"AAS with given id already exists",
		"AAS Descriptor access not allowed",
	)

	require.Error(t, err)
	require.True(t, common.IsErrDenied(err))
	require.False(t, common.IsErrConflict(err))
	statusCode, _ := registryprecheck.ResponseStatus(err)
	require.Equal(t, http.StatusForbidden, statusCode)
}

func TestRegistryCreatePrecheckReturnsConflictForVisibleDescriptor(t *testing.T) {
	t.Parallel()

	ctx := auth.WithQueryFilter(context.Background(), limitedCreateQueryFilter())

	err := registryprecheck.EnsureVisibleCreate(
		ctx,
		func(context.Context) (bool, error) { return true, nil },
		func(context.Context) error { return nil },
		"AAS with given id already exists",
		"AAS Descriptor access not allowed",
	)

	require.Error(t, err)
	require.True(t, common.IsErrConflict(err))
	statusCode, _ := registryprecheck.ResponseStatus(err)
	require.Equal(t, http.StatusConflict, statusCode)
}

func TestRegistryCreatePrecheckAllowsMissingDescriptor(t *testing.T) {
	t.Parallel()

	err := registryprecheck.EnsureVisibleCreate(
		context.Background(),
		func(context.Context) (bool, error) { return false, nil },
		func(context.Context) error { return errors.New("read must not be called") },
		"AAS with given id already exists",
		"AAS Descriptor access not allowed",
	)

	require.NoError(t, err)
}

func TestRegistryCreatePrecheckPropagatesErrors(t *testing.T) {
	t.Parallel()

	rawErr := errors.New("raw existence failed")
	err := registryprecheck.EnsureVisibleCreate(
		context.Background(),
		func(context.Context) (bool, error) { return false, rawErr },
		func(context.Context) error { return nil },
		"AAS with given id already exists",
		"AAS Descriptor access not allowed",
	)
	require.ErrorIs(t, err, rawErr)

	filteredErr := errors.New("filtered read failed")
	ctx := auth.WithQueryFilter(context.Background(), limitedCreateQueryFilter())
	err = registryprecheck.EnsureVisibleCreate(
		ctx,
		func(context.Context) (bool, error) { return true, nil },
		func(context.Context) error { return filteredErr },
		"AAS with given id already exists",
		"AAS Descriptor access not allowed",
	)
	require.ErrorIs(t, err, filteredErr)
}

func limitedCreateQueryFilter() *auth.QueryFilter {
	falseExpression := grammar.LogicalExpression{Boolean: boolPtr(false)}
	return &auth.QueryFilter{
		Formula: &falseExpression,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumCREATE: falseExpression,
		},
	}
}

func boolPtr(value bool) *bool {
	return &value
}
