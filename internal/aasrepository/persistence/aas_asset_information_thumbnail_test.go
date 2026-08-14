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
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/jackc/pgx/v5/pgconn"
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

	mock.ExpectQuery(`INSERT INTO "aas".*RETURNING "id"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(17))
	mock.ExpectExec(`(?s)INSERT INTO "aas_payload".*INSERT INTO "asset_information".*INSERT INTO "thumbnail_file_element".*INSERT INTO "specific_asset_id".*INSERT INTO "specific_asset_id_payload".*INSERT INTO "aas_submodel_reference".*INSERT INTO "aas_submodel_reference_key".*INSERT INTO "aas_submodel_reference_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repository := &AssetAdministrationShellDatabase{}
	require.NoError(t, repository.createAssetAdministrationShellInTransaction(t.Context(), tx, aas))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAssetAdministrationShellLargeDuplicateStopsAfterRootInsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	tx := beginMockTransaction(t, db, mock)
	aas := types.NewAssetAdministrationShell(
		"https://example.com/ids/aas/duplicate",
		types.NewAssetInformation(types.AssetKindInstance),
	)
	references := make([]types.IReference, 1000)
	for index := range references {
		references[index] = types.NewReference(
			types.ReferenceTypesModelReference,
			[]types.IKey{types.NewKey(types.KeyTypesSubmodel, "https://example.com/ids/sm/duplicate")},
		)
	}
	aas.SetSubmodels(references)

	duplicateErr := &pgconn.PgError{Code: "23505", ConstraintName: "aas_aas_id_key"}
	mock.ExpectQuery(`INSERT INTO "aas".*RETURNING "id"`).WillReturnError(duplicateErr)
	mock.ExpectRollback()

	repository := &AssetAdministrationShellDatabase{}
	err = repository.createAssetAdministrationShellInTransaction(t.Context(), tx, aas)
	require.Error(t, err)
	require.True(t, common.IsErrConflict(err), "expected conflict error, got %v", err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAssetAdministrationShellRejectsKeylessReferenceBeforeInsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	tx := beginMockTransaction(t, db, mock)
	aas := types.NewAssetAdministrationShell(
		"https://example.com/ids/aas/keyless-reference",
		types.NewAssetInformation(types.AssetKindInstance),
	)
	aas.SetSubmodels([]types.IReference{
		types.NewReference(types.ReferenceTypesModelReference, []types.IKey{}),
	})
	mock.ExpectRollback()

	repository := &AssetAdministrationShellDatabase{}
	err = repository.createAssetAdministrationShellInTransaction(t.Context(), tx, aas)
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err), "expected bad-request error, got %v", err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAssetAdministrationShellDependentConflictRemainsInternal(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	tx := beginMockTransaction(t, db, mock)
	aas := types.NewAssetAdministrationShell(
		"https://example.com/ids/aas/dependent-conflict",
		types.NewAssetInformation(types.AssetKindInstance),
	)

	mock.ExpectQuery(`INSERT INTO "aas".*RETURNING "id"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(17))
	dependentErr := &pgconn.PgError{Code: "23505"}
	mock.ExpectExec(`(?s)INSERT INTO "aas_payload".*INSERT INTO "asset_information"`).
		WillReturnError(dependentErr)
	mock.ExpectRollback()

	repository := &AssetAdministrationShellDatabase{}
	err = repository.createAssetAdministrationShellInTransaction(t.Context(), tx, aas)
	require.Error(t, err)
	require.True(t, common.IsInternalServerError(err), "expected internal-server error, got %v", err)
	require.False(t, common.IsErrConflict(err), "dependent conflict must not be mapped as an AAS identifier conflict")
	var postgresErr *pgconn.PgError
	require.True(t, errors.As(err, &postgresErr), "expected PostgreSQL cause, got %v", err)
	require.NoError(t, tx.Rollback())
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
