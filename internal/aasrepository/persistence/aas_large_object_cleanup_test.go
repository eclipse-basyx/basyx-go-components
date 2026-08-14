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
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
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
	mock.ExpectExec(`(?s)SELECT COUNT\(\*\).*lo_unlink.*thumbnail_file_data.*DELETE FROM "aas"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	err = repository.DeleteAssetAdministrationShellByIDInTransaction(contextWithConfig(), tx, aasID)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutAssetAdministrationShellReconcilesExistingRootWithoutReplacement(t *testing.T) {
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
	expectExistingAASRead(mock, aasID, globalAssetID, "previous")
	mock.ExpectQuery(`WITH aas_reconciliation_plan`).
		WillReturnRows(sqlmock.NewRows([]string{
			"updated_specific", "inserted_specific", "deleted_specific",
			"updated_references", "inserted_references", "deleted_references",
		}).AddRow(0, 0, 0, 0, 0, 0))
	mock.ExpectRollback()

	isUpdate, err := repository.PutAssetAdministrationShellByIDInTransaction(contextWithConfig(), tx, aasID, aas)
	require.NoError(t, err)
	require.True(t, isUpdate)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutAssetAdministrationShellUnchangedAppendsHistoryWithoutLiveMutation(t *testing.T) {
	previousHistoryConfig := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeAPI})
	t.Cleanup(func() { history.Configure(previousHistoryConfig) })

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	tx := beginMockTransaction(t, db, mock)
	repository := &AssetAdministrationShellDatabase{}
	aasID := "https://example.com/ids/aas/put-unchanged"
	globalAssetID := "https://example.com/global-assets/put-unchanged"
	assetInformation := types.NewAssetInformation(types.AssetKindInstance)
	assetInformation.SetGlobalAssetID(&globalAssetID)
	aas := types.NewAssetAdministrationShell(aasID, assetInformation)

	mock.ExpectQuery(`SELECT .*FROM "aas".*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	expectExistingAASRead(mock, aasID, globalAssetID, "")
	mock.ExpectExec(`SELECT .*pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "row_hash" FROM "aas_history"`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO "aas_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(1)))
	mock.ExpectExec(`INSERT INTO "aas_history_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	result, err := repository.PutAssetAdministrationShellByIDInTransactionWithResult(contextWithConfig(), tx, aasID, aas)
	require.NoError(t, err)
	require.True(t, result.IsUpdate)
	require.False(t, result.Changed)
	require.NotNil(t, result.Previous)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectExistingAASRead(mock sqlmock.Sqlmock, aasID string, globalAssetID string, category string) {
	var storedCategory any
	if category != "" {
		storedCategory = category
	}
	mock.ExpectQuery(`SELECT .*FROM "aas" AS "aas"`).
		WillReturnRows(sqlmock.NewRows([]string{
			"aas_id", "id_short", "category", "displayname_payload", "description_payload",
			"administrative_information_payload", "embedded_data_specification_payload", "extensions_payload",
			"derived_from_payload", "asset_kind", "global_asset_id", "asset_type", "thumbnail_value", "thumbnail_content_type",
		}).AddRow(aasID, nil, storedCategory, nil, nil, nil, nil, nil, nil, int(types.AssetKindInstance), globalAssetID, nil, nil, nil))
	mock.ExpectQuery(`SELECT .*FROM "specific_asset_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "value", "semantic_id_payload"}))
	mock.ExpectQuery(`SELECT .*FROM "aas_submodel_reference"`).
		WillReturnRows(sqlmock.NewRows([]string{"owner_id"}))
}

func contextWithConfig() context.Context {
	return common.ContextWithConfig(context.Background(), &common.Config{})
}
