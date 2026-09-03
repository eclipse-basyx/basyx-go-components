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
// Author: Aaron Zielstorff (Fraunhofer IESE)

package descriptors

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestListAssetAdministrationShellDescriptorsUsesBatchedPageQuery(t *testing.T) {
	t.Parallel()

	var query string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
		query = actual
		return nil
	})))
	require.NoError(t, err)
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{
		"descriptor_id",
		"asset_kind",
		"asset_type",
		"global_asset_id",
		"id_short",
		"id",
		"created_at",
		"administration",
		"display_name",
		"description",
		"extensions",
	}))
	mock.ExpectClose()

	descriptors, cursor, err := ListAssetAdministrationShellDescriptors(
		common.ContextWithConfig(t.Context(), &common.Config{}),
		db,
		100,
		"",
		model.AssetKind(""),
		"",
		"",
		time.Time{},
		time.Time{},
	)

	require.NoError(t, err)
	require.Empty(t, descriptors)
	require.Empty(t, cursor)
	require.NotContains(t, query, "jsonb_build_object")
	require.Contains(t, query, `FROM (SELECT`)
	require.Contains(t, query, `INNER JOIN "descriptor_payload" AS "aas_descriptor_payload"`)
	require.NotContains(t, query, `LEFT JOIN "descriptor_payload" AS "aas_descriptor_payload"`)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAssembleAASDescriptorListUsesPageExtensionPayload(t *testing.T) {
	t.Parallel()

	descriptor, err := assembleAASDescriptorListRow(
		model.AssetAdministrationShellDescriptorRow{
			DescID:                    7,
			IDStr:                     "aas-1",
			AdministrativeInfoPayload: []byte("null"),
			DisplayNamePayload:        []byte("[]"),
			DescriptionPayload:        []byte("[]"),
			ExtensionsPayload:         []byte(`[{"name":"tag","valueType":"xs:string","value":"v1"}]`),
		},
		aasDescriptorListChildren{},
	)

	require.NoError(t, err)
	require.Len(t, descriptor.Extensions, 1)
	require.Equal(t, "tag", descriptor.Extensions[0].Name())
}

func TestListAssetAdministrationShellDescriptorsUsesOneQueryWithRestrictiveFragmentMask(t *testing.T) {
	deny := false
	ctx := auth.WithQueryFilter(common.ContextWithConfig(t.Context(), &common.Config{}), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			grammar.FragmentStringPattern("$aasdesc#specificAssetIds[]"): auth.NewFragmentFilterPredicate(
				grammar.LogicalExpression{Boolean: &deny},
				false,
			),
		},
	})
	assertFilteredAASDescriptorListUsesOneQuery(ctx, t)
}

func assertFilteredAASDescriptorListUsesOneQuery(ctx context.Context, t *testing.T) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"descriptor"}).AddRow([]byte(`{"id":"aas-1"}`)))
	mock.ExpectClose()

	descriptors, cursor, err := ListAssetAdministrationShellDescriptors(
		ctx,
		db,
		100,
		"",
		model.AssetKind(""),
		"",
		"",
		time.Time{},
		time.Time{},
	)

	require.NoError(t, err)
	require.Empty(t, cursor)
	require.Len(t, descriptors, 1)
	require.Equal(t, "aas-1", descriptors[0].Id)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListSubmodelDescriptorsUsesOneQuery(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{"id", "descriptor"}).
			AddRow("sm-1", []byte(`{"id":"sm-1","endpoints":[]}`)).
			AddRow("sm-2", []byte(`{"id":"sm-2","endpoints":[]}`)),
	)
	mock.ExpectClose()

	descriptors, cursor, err := ListSubmodelDescriptors(
		common.ContextWithConfig(t.Context(), &common.Config{}),
		db,
		1,
		"",
		time.Time{},
		time.Time{},
	)
	require.NoError(t, err)
	require.Len(t, descriptors, 1)
	require.Equal(t, "sm-1", descriptors[0].Id)
	require.Equal(t, "sm-2", cursor)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListSubmodelDescriptorsCalculatesCursorBeforeAuthorization(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{"id", "descriptor"}).
			AddRow("sm-denied", nil).
			AddRow("sm-next", []byte(`{"id":"sm-next","endpoints":[]}`)),
	)
	mock.ExpectClose()

	descriptors, cursor, err := ListSubmodelDescriptors(
		common.ContextWithConfig(t.Context(), &common.Config{}),
		db,
		1,
		"",
		time.Time{},
		time.Time{},
	)
	require.NoError(t, err)
	require.Empty(t, descriptors)
	require.Equal(t, "sm-next", cursor)
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSubmodelDescriptorListSQLShapeIsStableAcrossPageSizes(t *testing.T) {
	t.Parallel()

	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	scope := submodelDescriptorListScope{
		collectorRoot:  grammar.CollectorRootSMDesc,
		fragmentPrefix: "$smdesc",
	}
	one, err := buildSubmodelDescriptorListQuery(ctx, 2, "", time.Time{}, time.Time{}, scope)
	require.NoError(t, err)
	many, err := buildSubmodelDescriptorListQuery(ctx, 101, "", time.Time{}, time.Time{}, scope)
	require.NoError(t, err)
	oneSQL, _, err := one.Prepared(true).ToSQL()
	require.NoError(t, err)
	manySQL, _, err := many.Prepared(true).ToSQL()
	require.NoError(t, err)

	require.Equal(t, oneSQL, manySQL)
	require.Contains(t, oneSQL, "raw_submodel_descriptor_page")
	require.Contains(t, oneSQL, "authorized_submodel_descriptors")
	require.Contains(t, oneSQL, `LEFT JOIN "authorized_submodel_descriptors"`)
	require.Contains(t, oneSQL, `IN ((SELECT "raw_submodel_descriptor_page"."descriptor_id"`)
	require.NotContains(t, oneSQL, `= (SELECT "raw_submodel_descriptor_page"."descriptor_id"`)
	require.Contains(t, oneSQL, `ORDER BY "raw_submodel_descriptor_page"."id" ASC, "raw_submodel_descriptor_page"."descriptor_id" ASC`)
}

func TestSubmodelDescriptorListCursorExistenceUsesSQLLiteral(t *testing.T) {
	t.Parallel()

	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	scope := submodelDescriptorListScope{
		collectorRoot:  grammar.CollectorRootSMDesc,
		fragmentPrefix: "$smdesc",
	}
	dataset, err := buildSubmodelDescriptorListQuery(ctx, 101, "urn:example:cursor", time.Time{}, time.Time{}, scope)
	require.NoError(t, err)
	query, _, err := dataset.Prepared(true).ToSQL()
	require.NoError(t, err)
	require.Contains(t, query, `FROM "submodel_descriptor" AS "submodel_descriptor_cursor"`)
	require.NotContains(t, query, `SELECT 1 FROM "authorized_submodel_descriptors"`)
}

func TestNestedSubmodelDescriptorListKeepsPayloadJoinOptional(t *testing.T) {
	t.Parallel()

	aasDescriptorID := int64(42)
	scope := submodelDescriptorListScope{
		collectorRoot:   grammar.CollectorRootAASDesc,
		fragmentPrefix:  "$aasdesc#submodelDescriptors[]",
		aasDescriptorID: &aasDescriptorID,
	}
	dataset, err := buildSubmodelDescriptorListQuery(
		common.ContextWithConfig(t.Context(), &common.Config{}),
		101,
		"",
		time.Time{},
		time.Time{},
		scope,
	)
	require.NoError(t, err)
	query, _, err := dataset.Prepared(true).ToSQL()
	require.NoError(t, err)
	require.Contains(t, query, `LEFT JOIN "descriptor_payload" AS "submodel_descriptor_payload"`)
	require.NotContains(t, query, `INNER JOIN "descriptor_payload" AS "submodel_descriptor_payload"`)
	require.Contains(t, query, `FROM "authorized_submodel_descriptors"`)
	require.NotContains(t, query, "raw_submodel_descriptor_page")
}

func TestNestedSubmodelDescriptorListDoesNotPromoteFieldMaskToRowFilter(t *testing.T) {
	t.Parallel()

	deny := false
	supplementalField := grammar.ModelStringPattern("$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value")
	supplementalValue := grammar.StandardString("WRITTEN_BY_X")
	supplementalMatch := grammar.LogicalExpression{
		Eq: grammar.ComparisonItems{
			{Field: &supplementalField},
			{StrVal: &supplementalValue},
		},
	}
	formulaField := grammar.ModelStringPattern("$smdesc#supplementalSemanticIds")
	formula := grammar.LogicalExpression{
		Eq: grammar.ComparisonItems{
			{Field: &formulaField},
			{StrVal: &supplementalValue},
		},
	}
	ctx := auth.WithQueryFilter(common.ContextWithConfig(t.Context(), &common.Config{}), &auth.QueryFilter{
		Formula: &formula,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumREAD: formula,
		},
		Filters: auth.FragmentFilters{
			"$aasdesc#submodelDescriptors[].semanticId": auth.NewFragmentFilterPredicate(
				grammar.LogicalExpression{Boolean: &deny},
				false,
			),
			"$aasdesc#submodelDescriptors[].supplementalSemanticIds[]": auth.NewFragmentFilterPredicate(
				supplementalMatch,
				true,
			),
		},
	})
	aasDescriptorID := int64(42)
	dataset, err := buildSubmodelDescriptorListQuery(
		ctx,
		101,
		"",
		time.Time{},
		time.Time{},
		submodelDescriptorListScope{
			collectorRoot:   grammar.CollectorRootAASDesc,
			fragmentPrefix:  "$aasdesc#submodelDescriptors[]",
			aasDescriptorID: &aasDescriptorID,
		},
	)
	require.NoError(t, err)
	_, args, err := dataset.Prepared(true).ToSQL()
	require.NoError(t, err)

	falseArguments := 0
	for _, argument := range args {
		if value, ok := argument.(bool); ok && !value {
			falseArguments++
		}
	}
	require.Equal(t, 1, falseArguments)
	require.Contains(t, args, "WRITTEN_BY_X")
}

func TestStandaloneSubmodelDescriptorListFiltersExactIdentifier(t *testing.T) {
	t.Parallel()

	dataset, err := buildSubmodelDescriptorListQuery(
		common.ContextWithConfig(t.Context(), &common.Config{}),
		1,
		"",
		time.Time{},
		time.Time{},
		submodelDescriptorListScope{
			collectorRoot:  grammar.CollectorRootSMDesc,
			fragmentPrefix: "$smdesc",
			identifiable:   "urn:example:submodel:target",
		},
	)
	require.NoError(t, err)
	query, args, err := dataset.Prepared(true).ToSQL()
	require.NoError(t, err)
	require.Contains(t, query, `"submodel_descriptor_raw_page"."id" = $1`)
	require.Contains(t, args, "urn:example:submodel:target")
}
