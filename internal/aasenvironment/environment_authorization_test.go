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
	"net/http"
	"testing"

	aastypes "github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestUploadRepositoryStatusIsPreserved(t *testing.T) {
	for _, status := range []int{200, 201, 204, 400, 403, 404, 405, 409, 413, 500} {
		err := checkUploadRepositoryResponse("PUTSM", "id", model.Response(status, []model.Message{{Text: "detail"}}))
		if status < 300 {
			require.NoError(t, err)
			continue
		}
		require.Error(t, err)
		require.Equal(t, status, uploadProcessingStatus(err))
		require.Contains(t, err.Error(), "detail")
	}
}

type deniedEnvironmentWriter struct{}

func (deniedEnvironmentWriter) PutSubmodelByID(context.Context, string, aastypes.ISubmodel) (model.ImplResponse, error) {
	return model.Response(http.StatusForbidden, nil), nil
}

func (deniedEnvironmentWriter) PutAssetAdministrationShellById(context.Context, string, aastypes.IAssetAdministrationShell) (model.ImplResponse, error) {
	return model.Response(http.StatusForbidden, nil), nil
}

func TestUploadWrapperDenialRemainsForbidden(t *testing.T) {
	service := &uploadAPIService{submodelRepositoryService: deniedEnvironmentWriter{}, aasRepositoryService: deniedEnvironmentWriter{}}
	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	err := service.storeEnvironmentSubmodel(ctx, aastypes.NewSubmodel("id"))
	require.True(t, common.IsErrDenied(err))
	err = service.storeEnvironmentShell(ctx, aastypes.NewAssetAdministrationShell("id", aastypes.NewAssetInformation(aastypes.AssetKindInstance)))
	require.True(t, common.IsErrDenied(err))
}

func TestSerializationThumbnailsRequireVisibleShellAndReference(t *testing.T) {
	environment := aastypes.NewEnvironment()
	visible := aastypes.NewAssetAdministrationShell("visible", aastypes.NewAssetInformation(aastypes.AssetKindInstance))
	visible.AssetInformation().SetDefaultThumbnail(aastypes.NewResource("/thumbnail.png"))
	hiddenReference := aastypes.NewAssetAdministrationShell("hidden-reference", aastypes.NewAssetInformation(aastypes.AssetKindInstance))
	emptyReference := aastypes.NewAssetAdministrationShell("empty-reference", aastypes.NewAssetInformation(aastypes.AssetKindInstance))
	emptyReference.AssetInformation().SetDefaultThumbnail(aastypes.NewResource(""))
	environment.SetAssetAdministrationShells([]aastypes.IAssetAdministrationShell{visible, hiddenReference, emptyReference})
	for _, requested := range [][]string{nil, {common.EncodeString("visible"), common.EncodeString("absent"), common.EncodeString("hidden-reference")}} {
		ids, err := resolveSerializationThumbnailAASIDs(requested, environment)
		require.NoError(t, err)
		require.Equal(t, []string{"visible"}, ids)
	}
	ids, err := resolveSerializationThumbnailAASIDs([]string{common.EncodeString("absent")}, environment)
	require.NoError(t, err)
	require.Empty(t, ids)
	rewriteSerializationThumbnailReference(environment, "hidden-reference", "/leaked.png", "image/png")
	require.Nil(t, hiddenReference.AssetInformation().DefaultThumbnail())
	rewriteSerializationThumbnailReference(environment, "empty-reference", "/leaked.png", "image/png")
	require.Empty(t, emptyReference.AssetInformation().DefaultThumbnail().Path())
	rewriteSerializationThumbnailReference(environment, "visible", "/export.png", "image/png")
	require.Equal(t, "image/png", *visible.AssetInformation().DefaultThumbnail().ContentType())
	require.Equal(t, "/export.png", visible.AssetInformation().DefaultThumbnail().Path())
}
