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
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestCreateAssetAdministrationShellPersistsDefaultThumbnail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	tx := beginMockTransaction(t, db, mock)
	aas := types.NewAssetAdministrationShell(
		"https://example.com/ids/aas/default-thumbnail-create",
		assetInformationWithDefaultThumbnail("https://example.com/thumb-create.png", "image/png"),
	)
	aas.AssetInformation().SetSpecificAssetIDs([]types.ISpecificAssetID{
		types.NewSpecificAssetID("serialNumber", "123"),
	})
	aas.SetSubmodels([]types.IReference{types.NewReference(
		types.ReferenceTypesModelReference,
		[]types.IKey{types.NewKey(types.KeyTypesSubmodel, "https://example.com/ids/sm/1")},
	)})

	mock.ExpectExec(`(?s)INSERT INTO "aas".*INSERT INTO "aas_payload".*INSERT INTO "asset_information".*INSERT INTO "thumbnail_file_element".*INSERT INTO "specific_asset_id".*INSERT INTO "specific_asset_id_payload".*INSERT INTO "aas_submodel_reference".*INSERT INTO "aas_submodel_reference_key".*INSERT INTO "aas_submodel_reference_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repository := &AssetAdministrationShellDatabase{}
	require.NoError(t, repository.createAssetAdministrationShellInTransaction(t.Context(), tx, aas))
	require.NoError(t, mock.ExpectationsWereMet())
}

func assetInformationWithDefaultThumbnail(path string, contentType string) types.IAssetInformation {
	assetInformation := types.NewAssetInformation(types.AssetKindInstance)
	thumbnail := types.NewResource(path)
	thumbnail.SetContentType(&contentType)
	assetInformation.SetDefaultThumbnail(thumbnail)
	return assetInformation
}

func beginMockTransaction(t *testing.T, db *sql.DB, mock sqlmock.Sqlmock) *sql.Tx {
	t.Helper()

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	return tx
}
