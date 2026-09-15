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
	"errors"
	"regexp"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestAASRepositoryReadPoolSelection(t *testing.T) {
	writer, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = writer.Close() }()
	reader, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()

	backend, err := NewAssetAdministrationShellDatabaseFromPools(writer, reader, "off")
	require.NoError(t, err)
	require.Same(t, reader, backend.readDB(t.Context()))
	require.Same(t, writer, backend.readDB(common.WithWriterPostgresReads(t.Context())))
}

func TestGetAssetAdministrationShellByDPPIDDatasetUsesMetadataPropertyAndDistinctOwner(t *testing.T) {
	dialect := goqu.Dialect("postgres")
	query, args, err := buildGetAssetAdministrationShellDBIDByDPPIDDataset(
		&dialect,
		"https://example.org/dpps/42",
		[]string{"urn:samm:io.admin-shell.idta.dpp_meta:1.0.0#DppMetadata"},
	).Prepared(true).ToSQL()

	require.NoError(t, err)
	require.Len(t, args, 4)
	require.Contains(t, query, `SELECT DISTINCT "aas"."id"`)
	require.Contains(t, query, `"metadata_element"."id_short" = $1`)
	require.Contains(t, query, `"metadata_element"."parent_sme_id" IS NULL`)
	require.Contains(t, query, `"metadata_property"."value_text" = $3`)
	require.True(t, strings.HasSuffix(query, "LIMIT $4"))
}

func TestGetDPPIDsByAssetAndMetadataSemanticIDsDatasetUsesDPPIDCursor(t *testing.T) {
	dialect := goqu.Dialect("postgres")
	query, err := buildGetDPPIDsByAssetAndMetadataSemanticIDsDataset(
		&dialect,
		[]string{"product-1"},
		[]string{"urn:samm:io.admin-shell.idta.dpp_meta:1.0.0#DppMetadata"},
		10,
		"https://example.org/dpps/41",
	)
	require.NoError(t, err)
	sqlQuery, sqlArgs, err := query.Prepared(true).ToSQL()

	require.NoError(t, err)
	require.Len(t, sqlArgs, 5)
	require.Contains(t, sqlQuery, `SELECT DISTINCT "metadata_property"."value_text"`)
	require.Contains(t, sqlQuery, `"metadata_property"."value_text" > $4`)
	require.Contains(t, sqlQuery, `ORDER BY "metadata_property"."value_text" ASC`)
	require.True(t, strings.HasSuffix(sqlQuery, "LIMIT $5"))
}

func TestSubmodelReferenceCheckSelectsPoolFromContext(t *testing.T) {
	tests := []struct {
		name        string
		writerReads bool
	}{
		{name: "reader"},
		{name: "writer", writerReads: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer, writerMock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = writer.Close() }()
			reader, readerMock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = reader.Close() }()

			backend, err := NewAssetAdministrationShellDatabaseFromPools(writer, reader, "off")
			require.NoError(t, err)

			selectedMock := readerMock
			ctx := t.Context()
			if test.writerReads {
				selectedMock = writerMock
				ctx = common.WithWriterPostgresReads(ctx)
			}
			selectedMock.ExpectBegin()
			selectedMock.ExpectQuery("SELECT").WillReturnError(errors.New("lookup failed"))
			selectedMock.ExpectRollback()

			require.Error(t, backend.CheckIfSubmodelReferenceExistsInAssetAdministrationShell(ctx, "aas-1", "sm-1"))
			require.NoError(t, writerMock.ExpectationsWereMet())
			require.NoError(t, readerMock.ExpectationsWereMet())
		})
	}
}

func TestGetAssetAdministrationShellIDsByAssetAndSubmodelSemanticIDsUsesJoinedQuery(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &AssetAdministrationShellDatabase{db: db}
	queryPattern := regexp.QuoteMeta(
		`SELECT DISTINCT "aas"."aas_id" FROM "aas" AS "aas" ` +
			`INNER JOIN "asset_information" AS "asset_information" ON ("asset_information"."asset_information_id" = "aas"."id") ` +
			`INNER JOIN "aas_submodel_reference" AS "submodel_reference" ON ("submodel_reference"."aas_id" = "aas"."id") ` +
			`INNER JOIN "aas_submodel_reference_key" AS "submodel_reference_key" ON ("submodel_reference_key"."reference_id" = "submodel_reference"."id") ` +
			`INNER JOIN "submodel" AS "submodel" ON ("submodel"."submodel_identifier" = "submodel_reference_key"."value") ` +
			`INNER JOIN "submodel_semantic_id_reference_key" AS "semantic_id_key" ON ("semantic_id_key"."reference_id" = "submodel"."id") ` +
			`WHERE ("asset_information"."global_asset_id" = ANY($1::text[]) AND "semantic_id_key"."value" = ANY($2::text[])) ` +
			`ORDER BY "aas"."aas_id" ASC LIMIT $3`,
	)
	mock.ExpectQuery(queryPattern).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 2).
		WillReturnRows(sqlmock.NewRows([]string{"aas_id"}).
			AddRow("dpp-1").
			AddRow("dpp-2"))

	identifiers, cursor, err := sut.GetAssetAdministrationShellIDsByAssetAndSubmodelSemanticIDs(
		contextWithConfig(),
		[]string{"product-1", "product-2"},
		[]string{"https://admin-shell.io/idta/cds/dppMetadata/1"},
		1,
		"",
	)
	require.NoError(t, err)
	require.Equal(t, []string{"dpp-1"}, identifiers)
	require.Equal(t, "dpp-1", cursor)
	require.NoError(t, mock.ExpectationsWereMet())
}
