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
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
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

	mock.ExpectQuery(`INSERT INTO "aas".*RETURNING "id"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	mock.ExpectExec(`INSERT INTO "aas_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "asset_information"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectInternalThumbnailCleanup(mock)
	mock.ExpectExec(`INSERT INTO "thumbnail_file_element"`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repository := &AssetAdministrationShellDatabase{}
	require.NoError(t, repository.createAssetAdministrationShellInTransaction(tx, aas))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateAssetInformationRecordPersistsDefaultThumbnail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	tx := beginMockTransaction(t, db, mock)
	dialect := goquDialect()
	assetInformation := assetInformationWithDefaultThumbnail("https://example.com/thumb-update.png", "image/png")

	mock.ExpectExec(`UPDATE "asset_information"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectInternalThumbnailCleanup(mock)
	mock.ExpectExec(`INSERT INTO "thumbnail_file_element"`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, updateAssetInformationRecord(
		tx,
		&dialect,
		int64(42),
		"https://example.com/ids/aas/default-thumbnail-update",
		assetInformation,
		currentAssetInformationState{},
	))
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectInternalThumbnailCleanup(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`DELETE FROM "thumbnail_binary_reference"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT "file_oid" FROM "thumbnail_file_data"`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`DELETE FROM "thumbnail_file_data"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

func TestBuildUpsertDefaultThumbnailQueryPreservesExistingContentTypeWhenUnset(t *testing.T) {
	dialect := goquDialect()
	thumbnail := types.NewResource("https://example.com/thumb.png")

	upsertSQL, _, err := buildUpsertDefaultThumbnailQuery(
		&dialect,
		int64(42),
		thumbnail,
		thumbnail.Path(),
	)

	require.NoError(t, err)
	require.Contains(t, upsertSQL, `COALESCE("excluded"."content_type", "thumbnail_file_element"."content_type")`)
	require.Contains(t, upsertSQL, `"file_name"=NULL`)
	require.NotContains(t, strings.ToLower(upsertSQL), `"content_type"=null`)
}

func TestBuildUpsertDefaultThumbnailQueryClearsFileNameForExternalURL(t *testing.T) {
	dialect := goquDialect()
	assetInformation := assetInformationWithDefaultThumbnail("https://example.com/thumb-update.png", "image/png")
	thumbnail := assetInformation.DefaultThumbnail()

	upsertSQL, _, err := buildUpsertDefaultThumbnailQuery(
		&dialect,
		int64(42),
		thumbnail,
		thumbnail.Path(),
	)

	require.NoError(t, err)
	require.Contains(t, upsertSQL, `"file_name"=NULL`)
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

func goquDialect() goqu.DialectWrapper {
	return goqu.Dialect("postgres")
}
