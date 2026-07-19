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

package persistence

import (
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestMergeAssetInformationSnapshotClearsOmittedThumbnailOnReplacement(t *testing.T) {
	current := map[string]any{
		"assetKind":        "Type",
		"globalAssetId":    "old-global",
		"assetType":        "old-type",
		"specificAssetIds": []any{map[string]any{"name": "old"}},
		"defaultThumbnail": map[string]any{"path": "thumbnail"},
	}
	assetInformation := types.NewAssetInformation(types.AssetKindInstance)
	newGlobalID := "new-global"
	assetInformation.SetGlobalAssetID(&newGlobalID)

	mergeAssetInformationSnapshot(current, map[string]any{
		"assetKind":     "Instance",
		"globalAssetId": newGlobalID,
	}, assetInformation)

	require.Equal(t, "Instance", current["assetKind"])
	require.Equal(t, newGlobalID, current["globalAssetId"])
	require.Equal(t, "old-type", current["assetType"])
	require.Equal(t, []any{map[string]any{"name": "old"}}, current["specificAssetIds"])
	require.NotContains(t, current, "defaultThumbnail")
}

func TestSnapshotReferenceContainsKeyValue(t *testing.T) {
	reference := map[string]any{
		"keys": []any{
			map[string]any{"type": "Submodel", "value": "sm-1"},
		},
	}

	require.True(t, snapshotReferenceContainsKeyValue(reference, "sm-1"))
	require.False(t, snapshotReferenceContainsKeyValue(reference, "sm-2"))
}
