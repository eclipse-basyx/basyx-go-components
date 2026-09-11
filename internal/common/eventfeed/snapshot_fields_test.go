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

package eventfeed

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSubmodelAssetOwnersPreservesEveryAASForSharedAssets(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT DISTINCT .*"aas"."aas_id".*INNER JOIN "aas"`).
		WillReturnRows(sqlmock.NewRows([]string{"global_asset_id", "aas_id"}).
			AddRow("asset-1", "aas-a").AddRow("asset-1", "aas-b").AddRow("asset-2", "aas-b"))
	mock.ExpectRollback()
	tx, err := db.Begin()
	require.NoError(t, err)
	assets, owners, err := submodelAssetOwnersTx(t.Context(), tx, "sm-1")
	require.NoError(t, err)
	require.Equal(t, []string{"asset-1", "asset-2"}, assets)
	require.Equal(t, []string{"aas-a", "aas-b"}, owners)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuilderDistinguishesKnownAndUnknownAASProvenance(t *testing.T) {
	builder := NewBuilder(DefaultConfig())
	aas, err := builder.AASCreated("aas-1", "asset-1", nil)
	require.NoError(t, err)
	require.Equal(t, []string{"aas-1"}, aas.AuthorizationAASIDs)
	asset, err := builder.AssetCreated("asset-1", "aas-1", nil)
	require.NoError(t, err)
	require.Equal(t, []string{"aas-1"}, asset.AuthorizationAASIDs)
	for _, ids := range [][]string{nil, {"asset-1"}} {
		sm, err := builder.SubmodelCreated("sm-1", "", ids)
		require.NoError(t, err)
		pcn, err := builder.PCN("sm-1", ids, map[string]any{})
		require.NoError(t, err)
		for _, event := range []FeedEvent{sm, pcn} {
			if len(ids) == 0 {
				require.NotNil(t, event.AuthorizationAASIDs)
				require.Empty(t, event.AuthorizationAASIDs)
			} else {
				require.Nil(t, event.AuthorizationAASIDs)
			}
		}
	}
}
