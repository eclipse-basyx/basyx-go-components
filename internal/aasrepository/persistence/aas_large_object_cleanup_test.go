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
	"context"
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestDeleteAssetAdministrationShellCleansThumbnailLargeObjectBeforeDelete(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	tx := beginMockTransaction(t, db, mock)
	repository := &AssetAdministrationShellDatabase{}
	aasID := "https://example.com/ids/aas/delete-cleanup"

	mock.ExpectQuery(`SELECT .*FROM "aas".*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	mock.ExpectQuery(`SELECT COUNT\(\*\).*lo_unlink.*thumbnail_file_data`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectExec(`DELETE FROM "aas"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	err = repository.DeleteAssetAdministrationShellByIDInTransaction(contextWithConfig(), tx, aasID)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutAssetAdministrationShellReplacementCleansThumbnailLargeObjectBeforeDelete(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	tx := beginMockTransaction(t, db, mock)
	repository := &AssetAdministrationShellDatabase{}
	aasID := "https://example.com/ids/aas/put-cleanup"
	assetInformation := types.NewAssetInformation(types.AssetKindInstance)
	globalAssetID := "https://example.com/global-assets/put-cleanup"
	assetInformation.SetGlobalAssetID(&globalAssetID)
	aas := types.NewAssetAdministrationShell(aasID, assetInformation)

	mock.ExpectQuery(`SELECT .*FROM "aas".*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	mock.ExpectQuery(`SELECT .*thumbnail.*value.*FROM "thumbnail_file_element" AS "thumbnail"`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT COUNT\(\*\).*lo_unlink.*thumbnail_file_data`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectExec(`DELETE FROM "aas"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO "aas".*RETURNING "id"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(43)))
	mock.ExpectExec(`INSERT INTO "aas_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "asset_information"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	isUpdate, err := repository.PutAssetAdministrationShellByIDInTransaction(contextWithConfig(), tx, aasID, aas)
	require.NoError(t, err)
	require.True(t, isUpdate)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func contextWithConfig() context.Context {
	return common.ContextWithConfig(context.Background(), &common.Config{})
}
