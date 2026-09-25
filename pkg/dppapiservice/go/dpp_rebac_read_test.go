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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestReBACDPPPaginationUsesOnlyVisibleIDs(t *testing.T) {
	page := pagedVisibleDPPIDs(map[string]struct{}{"urn:dpp:z": {}, "urn:dpp:a": {}, "urn:dpp:b": {}}, 2)
	require.Equal(t, []string{"urn:dpp:a", "urn:dpp:b"}, page.Items)
	require.Equal(t, "urn:dpp:b", page.Cursor)
	final := pagedVisibleDPPIDs(map[string]struct{}{"urn:dpp:z": {}}, 2)
	require.Equal(t, []string{"urn:dpp:z"}, final.Items)
	require.Empty(t, final.Cursor)
}

func TestReBACDPPReadsRequireValidatedIdentity(t *testing.T) {
	cfg := &common.Config{ReBAC: common.ReBACConfig{Enabled: true}}
	ctx := common.ContextWithConfig(t.Context(), cfg)
	service := &DPPRepositoryService{}
	aas := types.NewAssetAdministrationShell("urn:dpp:example", types.NewAssetInformation(types.AssetKindInstance))
	err := service.authorizeResolvedDPP(ctx, resolvedDPP{aas: aas})
	require.True(t, common.IsErrServiceUnavailable(err))
	_, err = auth.AuthorizeReBACReadElement(ctx, "urn:sm:example", "Temperature")
	require.True(t, common.IsErrServiceUnavailable(err))
}

func TestReBACDPPDisabledPreservesReadContext(t *testing.T) {
	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	service := &DPPRepositoryService{}
	require.NoError(t, service.authorizeResolvedDPP(ctx, resolvedDPP{}))
	result, err := auth.AuthorizeReBACReadElement(ctx, "urn:sm:example", "Temperature")
	require.NoError(t, err)
	require.Equal(t, ctx, result)
}

func TestDPPAuthorizationErrorsRetainFailClosedStatus(t *testing.T) {
	require.Equal(t, http.StatusForbidden, mapPersistenceError(common.NewErrDenied("REBAC-TEST denied"), http.StatusNotFound).Code)
	require.Equal(t, http.StatusServiceUnavailable, mapPersistenceError(common.NewErrServiceUnavailable("REBAC-TEST unavailable"), http.StatusNotFound).Code)
}

func TestDPPMutationRequiresValidatedIdentityBeforeWriterAccess(t *testing.T) {
	ctx := common.ContextWithConfig(t.Context(), &common.Config{ReBAC: common.ReBACConfig{Enabled: true}})
	_, err := authorizeDPPMutation(ctx, nil, "aas", "urn:dpp:x", http.MethodPut)
	require.True(t, common.IsErrServiceUnavailable(err))
	_, err = auth.AuthorizeReBACMutationElement(ctx, nil, "urn:sm:x", "Temperature", http.MethodPut)
	require.True(t, common.IsErrServiceUnavailable(err))
}
