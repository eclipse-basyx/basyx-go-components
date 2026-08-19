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
	"time"

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

func TestAASListMaterializationBatchUsesStableQueueOrder(t *testing.T) {
	t.Parallel()

	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	build := func(ids []int64) []common.PostgreSQLBatchStatement {
		statements, err := buildAASListMaterializationStatements(ctx, aasListPage{databaseIDs: ids})
		require.NoError(t, err)
		require.Len(t, statements, 3)
		return statements
	}
	one := build([]int64{1})
	many := build([]int64{1, 2, 3})
	for index := range one {
		require.Equal(t, one[index].SQL, many[index].SQL)
	}

	require.Contains(t, one[0].SQL, `FROM "aas" AS "aas"`)
	require.Contains(t, one[1].SQL, `FROM "aas_submodel_reference" AS "aas_submodel_reference"`)
	require.Contains(t, one[2].SQL, "UNION ALL")
	require.Contains(t, one[2].SQL, "specific_asset_id_external_subject_id_reference")
	require.Contains(t, one[2].SQL, "specific_asset_id_supplemental_semantic_id_reference")
}

func TestAASListSaturatedPageQueuesCursorLast(t *testing.T) {
	t.Parallel()

	statements, err := buildAASListMaterializationStatements(
		common.ContextWithConfig(t.Context(), &common.Config{}),
		aasListPage{databaseIDs: []int64{1}, nextDatabaseID: 2},
	)
	require.NoError(t, err)
	require.Len(t, statements, 4)
	require.Contains(t, statements[3].SQL, `SELECT "aas_id" FROM "aas"`)
}

func TestAASIdentifierPageEmbedsInclusiveCursorValidation(t *testing.T) {
	t.Parallel()

	dialect := goqu.Dialect("postgres")
	dataset, err := buildGetAssetAdministrationShellIdentifiersDataset(&dialect, 100, "urn:example:aas", "", nil)
	require.NoError(t, err)
	query, _, err := dataset.Prepared(true).ToSQL()
	require.NoError(t, err)
	require.Contains(t, query, `EXISTS((SELECT`)
	require.Contains(t, query, `"aas"."aas_id" >=`)
	require.Contains(t, query, `ORDER BY "aas"."aas_id" ASC`)
}

func TestAASReferenceListUsesOneIdentifierOnlyQuery(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(`SELECT "aas"\."aas_id"`).WillReturnRows(
		sqlmock.NewRows([]string{"aas_id"}).
			AddRow("aas-1").
			AddRow("aas-2"),
	)

	backend := &AssetAdministrationShellDatabase{db: db}
	references, cursor, err := backend.GetAssetAdministrationShellReferences(
		common.ContextWithConfig(t.Context(), &common.Config{}),
		1,
		"",
		"",
		nil,
	)
	require.NoError(t, err)
	require.Len(t, references, 1)
	require.Equal(t, "aas-2", cursor)
	require.Equal(t, "aas-1", references[0].Keys()[0].Value())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAASListSequentialFallbackUsesSharedThreeQueryMaterializer(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{
			"database_id",
			"aas_id",
			"id_short",
			"category",
			"display_name",
			"description",
			"administration",
			"embedded_data_specifications",
			"extensions",
			"derived_from",
			"asset_kind",
			"global_asset_id",
			"asset_type",
			"thumbnail_path",
			"thumbnail_content_type",
		}).AddRow(int64(7), "aas-1", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil),
	)
	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{"owner_id", "reference_id", "reference_type", "key_id", "key_type", "key_value", "parent_payload"}),
	)
	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{
			"row_kind",
			"owner_id",
			"specific_id",
			"specific_position",
			"name",
			"value",
			"semantic_payload",
			"reference_id",
			"reference_position",
			"reference_type",
			"key_id",
			"key_position",
			"key_type",
			"key_value",
			"parent_payload",
		}),
	)
	mock.ExpectCommit()

	backend := &AssetAdministrationShellDatabase{db: db}
	result, cursor, err := backend.GetAssetAdministrationShells(
		common.ContextWithConfig(t.Context(), &common.Config{}),
		100,
		"",
		"",
		nil,
		time.Time{},
		time.Time{},
	)
	require.NoError(t, err)
	require.Empty(t, cursor)
	require.Len(t, result, 1)
	require.Equal(t, "aas-1", result[0].ID())
	require.NoError(t, mock.ExpectationsWereMet())
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
