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

package aasenvironment

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	aastypes "github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

type deniedEnvironmentWriter struct{}

func (deniedEnvironmentWriter) UpsertSubmodelInTransaction(context.Context, *sql.Tx, aastypes.ISubmodel) error {
	return common.NewErrDenied("denied")
}

func (deniedEnvironmentWriter) UpsertAssetAdministrationShellInTransaction(context.Context, *sql.Tx, aastypes.IAssetAdministrationShell) error {
	return common.NewErrDenied("denied")
}

func TestUploadWrapperDenialRemainsForbidden(t *testing.T) {
	service := &uploadAPIService{submodelRepositoryService: deniedEnvironmentWriter{}, aasRepositoryService: deniedEnvironmentWriter{}}
	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	err := service.storeEnvironmentSubmodels(ctx, nil, []aastypes.ISubmodel{aastypes.NewSubmodel("submodel-id")})
	require.True(t, common.IsErrDenied(err))
	require.Equal(t, http.StatusForbidden, uploadProcessingStatus(err))
	require.Contains(t, err.Error(), `"submodel-id"`)
	shell := aastypes.NewAssetAdministrationShell("shell-id", aastypes.NewAssetInformation(aastypes.AssetKindInstance))
	err = service.storeEnvironmentShells(ctx, nil, []aastypes.IAssetAdministrationShell{shell})
	require.True(t, common.IsErrDenied(err))
	require.Equal(t, http.StatusForbidden, uploadProcessingStatus(err))
	require.Contains(t, err.Error(), `"shell-id"`)
}

func TestSerializationThumbnailsRequireVisibleShellAndReference(t *testing.T) {
	visible := aastypes.NewAssetAdministrationShell("visible", aastypes.NewAssetInformation(aastypes.AssetKindInstance))
	visible.AssetInformation().SetDefaultThumbnail(aastypes.NewResource("/thumbnail.png"))
	hiddenReference := aastypes.NewAssetAdministrationShell("hidden-reference", aastypes.NewAssetInformation(aastypes.AssetKindInstance))
	emptyReference := aastypes.NewAssetAdministrationShell("empty-reference", aastypes.NewAssetInformation(aastypes.AssetKindInstance))
	emptyReference.AssetInformation().SetDefaultThumbnail(aastypes.NewResource(""))
	parts := &serializationPackageParts{}
	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	require.NoError(t, parts.addThumbnail(ctx, nil))
	require.NoError(t, parts.addThumbnail(ctx, hiddenReference))
	require.NoError(t, parts.addThumbnail(ctx, emptyReference))
	require.Empty(t, parts.thumbnails)
	rewriteSerializationThumbnailReference(hiddenReference, "/leaked.png", "image/png")
	require.Nil(t, hiddenReference.AssetInformation().DefaultThumbnail())
	rewriteSerializationThumbnailReference(emptyReference, "/leaked.png", "image/png")
	require.Empty(t, emptyReference.AssetInformation().DefaultThumbnail().Path())
	rewriteSerializationThumbnailReference(visible, "/export.png", "image/png")
	require.Equal(t, "image/png", *visible.AssetInformation().DefaultThumbnail().ContentType())
	require.Equal(t, "/export.png", visible.AssetInformation().DefaultThumbnail().Path())
}
