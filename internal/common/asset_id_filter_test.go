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

package common

import (
	"encoding/json"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestDecodeAssetIDFilterMatchesGlobalAssetID(t *testing.T) {
	filter, err := DecodeAssetIDFilter([]string{encodeSpecificAssetID(t, types.NewSpecificAssetID("globalAssetId", "asset-global"))})
	require.NoError(t, err)

	matches, err := filter.Matches("asset-global", nil)
	require.NoError(t, err)
	require.True(t, matches)
}

func TestDecodeAssetIDFilterMatchesExactSpecificAssetID(t *testing.T) {
	filter, err := DecodeAssetIDFilter([]string{encodeSpecificAssetID(t, types.NewSpecificAssetID("manufacturerId", "4711"))})
	require.NoError(t, err)

	matches, err := filter.Matches("", []types.ISpecificAssetID{types.NewSpecificAssetID("manufacturerId", "4711")})
	require.NoError(t, err)
	require.True(t, matches)

	matches, err = filter.Matches("", []types.ISpecificAssetID{types.NewSpecificAssetID("serialNumber", "4711")})
	require.NoError(t, err)
	require.False(t, matches)
}

func TestDecodeAssetIDFilterRejectsMalformedInput(t *testing.T) {
	_, err := DecodeAssetIDFilter([]string{"not-base64url!"})
	require.Error(t, err)
	require.True(t, IsErrBadRequest(err))
	require.Contains(t, err.Error(), "COMMON-ASSETIDFILTER-DECODE")
}

func encodeSpecificAssetID(t *testing.T, assetID types.ISpecificAssetID) string {
	t.Helper()
	jsonable, err := jsonization.ToJsonable(assetID)
	require.NoError(t, err)
	data, err := json.Marshal(jsonable)
	require.NoError(t, err)
	return Encode(data)
}
