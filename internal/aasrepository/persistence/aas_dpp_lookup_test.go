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
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

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
			`WHERE (("asset_information"."global_asset_id" IN ('product-1', 'product-2')) AND ("semantic_id_key"."value" IN ('https://admin-shell.io/idta/cds/dppMetadata/1'))) ` +
			`ORDER BY "aas"."aas_id" ASC LIMIT 2`,
	)
	mock.ExpectQuery(queryPattern).
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
