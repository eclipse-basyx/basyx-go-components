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
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/stretchr/testify/require"
)

func TestAssetAdministrationShellBatchReadQueriesReuseSQLShape(t *testing.T) {
	t.Parallel()

	dialect := goqu.Dialect("postgres")
	build := func(ids []int64) []string {
		core, coreArgs, coreErr := buildGetAssetAdministrationShellMapsByDBIDsQueryWithSelect(
			&dialect,
			ids,
			unmaskedCoreAssetAdministrationShellSelectExpressions(true),
		)
		require.NoError(t, coreErr)
		require.Len(t, coreArgs, 1)

		references, referenceArgs, referenceErr := buildGetSubmodelReferencePayloadsByAASIDsDataset(&dialect, ids).
			Prepared(true).
			ToSQL()
		require.NoError(t, referenceErr)
		require.Len(t, referenceArgs, 1)

		specificAssetIDs, specificAssetIDArgs, specificAssetIDErr := buildReadSpecificAssetIDsByAssetInformationIDsDataset(&dialect, ids).
			Prepared(true).
			ToSQL()
		require.NoError(t, specificAssetIDErr)
		require.Len(t, specificAssetIDArgs, 1)

		return []string{core, references, specificAssetIDs}
	}

	require.Equal(t, build([]int64{1}), build([]int64{1, 2, 3}))
}

func TestAASSubmodelReferenceExistenceQueryReusesSQLShape(t *testing.T) {
	t.Parallel()

	dialect := goqu.Dialect("postgres")
	build := func(aasDBID int64, submodelIdentifier string) string {
		query, args, err := buildCheckAssetAdministrationShellSubmodelReferenceExistsQuery(
			&dialect, aasDBID, submodelIdentifier,
		)
		require.NoError(t, err)
		require.Len(t, args, 3)
		return query
	}

	firstQuery := build(1, "urn:example:submodel:first")
	secondQuery := build(2, "urn:example:submodel:second")
	require.Equal(t, firstQuery, secondQuery)
	require.Contains(t, firstQuery, `"ref"."aas_id" = $1`)
	require.Contains(t, firstQuery, `"key"."value" = $2`)
	require.Contains(t, firstQuery, `LIMIT $3`)
}

func TestCreateAASSubmodelReferenceDuplicateSkipsAASSnapshotLoad(t *testing.T) {
	previousHistoryConfig := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeOff})
	t.Cleanup(func() { history.Configure(previousHistoryConfig) })

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	aasIdentifier := "urn:example:aas:existing"
	submodelIdentifier := "urn:example:submodel:existing"
	reference := types.NewReference(
		types.ReferenceTypesModelReference,
		[]types.IKey{types.NewKey(types.KeyTypesSubmodel, submodelIdentifier)},
	)
	repository := &AssetAdministrationShellDatabase{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "id" FROM "aas".*FOR UPDATE`).
		WithArgs(aasIdentifier).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	mock.ExpectQuery(`SELECT 1 FROM "aas_submodel_reference"`).
		WithArgs(int64(42), submodelIdentifier, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(1))
	mock.ExpectRollback()

	err = repository.CreateSubmodelReferenceInAssetAdministrationShell(
		common.ContextWithConfig(t.Context(), &common.Config{}), aasIdentifier, reference,
	)
	require.Error(t, err)
	require.True(t, common.IsErrConflict(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateAASSubmodelReferenceUsesTargetedMutationWithoutHistory(t *testing.T) {
	previousHistoryConfig := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeOff})
	t.Cleanup(func() { history.Configure(previousHistoryConfig) })

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	aasIdentifier := "urn:example:aas:existing"
	submodelIdentifier := "urn:example:submodel:new"
	reference := types.NewReference(
		types.ReferenceTypesModelReference,
		[]types.IKey{types.NewKey(types.KeyTypesSubmodel, submodelIdentifier)},
	)
	repository := &AssetAdministrationShellDatabase{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "id" FROM "aas".*FOR UPDATE`).
		WithArgs(aasIdentifier).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	mock.ExpectQuery(`SELECT 1 FROM "aas_submodel_reference"`).
		WithArgs(int64(42), submodelIdentifier, sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(position\), -1\) \+ 1 FROM "aas_submodel_reference"`).
		WillReturnRows(sqlmock.NewRows([]string{"position"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "aas_submodel_reference".*RETURNING "id"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(101)))
	mock.ExpectExec(`INSERT INTO "aas_submodel_reference_key"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "aas_submodel_reference_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repository.CreateSubmodelReferenceInAssetAdministrationShell(
		common.ContextWithConfig(t.Context(), &common.Config{}), aasIdentifier, reference,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteAASSubmodelReferenceUsesTargetedMutationWithoutHistory(t *testing.T) {
	previousHistoryConfig := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeOff})
	t.Cleanup(func() { history.Configure(previousHistoryConfig) })

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	aasIdentifier := "urn:example:aas:existing"
	submodelIdentifier := "urn:example:submodel:existing"
	repository := &AssetAdministrationShellDatabase{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "id" FROM "aas".*FOR UPDATE`).
		WithArgs(aasIdentifier).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	mock.ExpectQuery(`SELECT "r"\."id" FROM "aas_submodel_reference" AS "r"`).
		WithArgs(int64(42), submodelIdentifier, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(101)))
	mock.ExpectExec(`DELETE FROM "aas_submodel_reference"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repository.DeleteSubmodelReferenceInAssetAdministrationShell(
		common.ContextWithConfig(t.Context(), &common.Config{}), aasIdentifier, submodelIdentifier,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
