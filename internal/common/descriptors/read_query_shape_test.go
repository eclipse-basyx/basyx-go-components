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

package descriptors

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestDescriptorBatchReadReusesSQLShape(t *testing.T) {
	t.Parallel()

	build := func(ids []int64) string {
		var query string
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
			query = actual
			return nil
		})))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("descriptor batch read").
			WillReturnRows(sqlmock.NewRows([]string{"descriptor_id", "extensions_payload"}))
		_, err = ReadExtensionsByDescriptorIDs(context.Background(), db, ids)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
		return query
	}

	oneIDQuery := build([]int64{1})
	manyIDsQuery := build([]int64{1, 2, 3})

	require.Equal(t, oneIDQuery, manyIDsQuery)
	require.Contains(t, oneIDQuery, `= ANY($1::bigint[])`)
}

func TestListSubmodelDescriptorsForAASReusesLookupSQLShape(t *testing.T) {
	t.Parallel()

	build := func(aasID string) string {
		queries := make([]string, 0, 2)
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
			queries = append(queries, actual)
			return nil
		})))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("AAS descriptor lookup").
			WithArgs(aasID, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"descriptor_id"}).AddRow(17))
		mock.ExpectQuery("Submodel descriptor page").
			WillReturnRows(sqlmock.NewRows([]string{"payload"}))
		descriptors, cursor, err := ListSubmodelDescriptorsForAAS(contextWithABACDisabled(t), db, aasID, 100, "")
		require.NoError(t, err)
		require.Empty(t, descriptors)
		require.Empty(t, cursor)
		require.NoError(t, mock.ExpectationsWereMet())
		require.Len(t, queries, 2)
		return queries[0]
	}

	firstQuery := build("urn:example:aas:first")
	secondQuery := build("urn:example:aas:second")

	require.Equal(t, firstQuery, secondQuery)
	require.Equal(t, `SELECT "aas"."descriptor_id" FROM "aas_descriptor" AS "aas" WHERE ("aas"."id" = $1) LIMIT $2`, firstQuery)
}

func TestReadSubmodelDescriptorsByAASDescriptorIDsFilterCorrelation(t *testing.T) {
	tests := []struct {
		name                string
		field               grammar.ModelStringPattern
		match               bool
		expectedCorrelation string
		unexpected          string
	}{
		{
			name:                "non-MATCH correlates to parent descriptor",
			field:               "$aasdesc#submodelDescriptors[].idShort",
			expectedCorrelation: `"submodel_descriptor"."aas_descriptor_id" = "descriptor"."id"`,
			unexpected:          `"submodel_descriptor__exists"."descriptor_id" = "submodel_descriptor"."descriptor_id"`,
		},
		{
			name:                "MATCH correlates to current submodel descriptor",
			field:               "$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value",
			match:               true,
			expectedCorrelation: `"submodel_descriptor__exists"."descriptor_id" = "submodel_descriptor"."descriptor_id"`,
			unexpected:          `"submodel_descriptor"."aas_descriptor_id" = "descriptor"."id"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := grammar.StandardString("matching-sibling")
			fragment := grammar.FragmentStringPattern("$aasdesc#submodelDescriptors[]")
			condition := grammar.LogicalExpression{
				Eq: grammar.ComparisonItems{
					{Field: &test.field},
					{StrVal: &value},
				},
			}
			ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
				Filters: auth.FragmentFilters{fragment: auth.NewFragmentFilterPredicate(condition, test.match)},
			})

			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
				if !strings.Contains(actual, `EXISTS`) {
					return fmt.Errorf("expected filter to use EXISTS, got: %s", actual)
				}
				if !strings.Contains(actual, test.expectedCorrelation) {
					return fmt.Errorf("expected correlation %q, got: %s", test.expectedCorrelation, actual)
				}
				if strings.Contains(actual, test.unexpected) {
					return fmt.Errorf("unexpected correlation %q, got: %s", test.unexpected, actual)
				}
				return nil
			})))
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			mock.ExpectQuery("nested submodel descriptor filter correlation").
				WillReturnRows(sqlmock.NewRows([]string{
					"c0",
					"c1",
					"c2",
					"c3",
					"c4",
					"c5",
					"c6",
					"c7",
				}))

			descriptors, err := ReadSubmodelDescriptorsByAASDescriptorIDs(ctx, db, []int64{12}, false)
			require.NoError(t, err)
			require.Nil(t, descriptors[12])
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestReadSubmodelDescriptorEndpointsFilterCorrelation(t *testing.T) {
	tests := []struct {
		name                string
		fragment            grammar.FragmentStringPattern
		field               grammar.ModelStringPattern
		match               bool
		expectedCorrelation string
		unexpected          string
	}{
		{
			name:                "nested non-MATCH correlates to owning AAS descriptor",
			fragment:            "$aasdesc#submodelDescriptors[].endpoints[]",
			field:               "$aasdesc#submodelDescriptors[].idShort",
			expectedCorrelation: `"submodel_descriptor__exists"."aas_descriptor_id" = "submodel_descriptor"."aas_descriptor_id"`,
			unexpected:          `"submodel_descriptor__exists"."descriptor_id" = "submodel_descriptor"."descriptor_id"`,
		},
		{
			name:                "nested MATCH correlates to current submodel descriptor",
			fragment:            "$aasdesc#submodelDescriptors[].endpoints[]",
			field:               "$aasdesc#specificAssetIds[].name",
			match:               true,
			expectedCorrelation: `"submodel_descriptor__exists"."descriptor_id" = "submodel_descriptor"."descriptor_id"`,
			unexpected:          `"submodel_descriptor__exists"."aas_descriptor_id" = "submodel_descriptor"."aas_descriptor_id"`,
		},
		{
			name:                "standalone non-MATCH stays on current submodel descriptor",
			fragment:            "$smdesc#endpoints[]",
			field:               "$smdesc#idShort",
			expectedCorrelation: `"submodel_descriptor__exists"."descriptor_id" = "submodel_descriptor"."descriptor_id"`,
			unexpected:          `"submodel_descriptor__exists"."aas_descriptor_id" = "submodel_descriptor"."aas_descriptor_id"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := grammar.StandardString("matching-sibling")
			condition := grammar.LogicalExpression{
				Eq: grammar.ComparisonItems{
					{Field: &test.field},
					{StrVal: &value},
				},
			}
			ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
				Filters: auth.FragmentFilters{
					test.fragment: auth.NewFragmentFilterPredicate(condition, test.match),
				},
			})

			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
				if !strings.Contains(actual, test.expectedCorrelation) {
					return fmt.Errorf("expected correlation %q, got: %s", test.expectedCorrelation, actual)
				}
				if strings.Contains(actual, test.unexpected) {
					return fmt.Errorf("unexpected correlation %q, got: %s", test.unexpected, actual)
				}
				return nil
			})))
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			mock.ExpectQuery("submodel descriptor endpoint filter correlation").
				WillReturnRows(sqlmock.NewRows([]string{
					"descriptor_id",
					"endpoint_id",
					"href",
					"endpoint_protocol",
					"subprotocol",
					"subprotocol_body",
					"subprotocol_body_encoding",
					"interface",
					"versions",
					"security_attributes",
				}))

			endpoints, err := ReadEndpointsByDescriptorIDs(ctx, db, []int64{12}, "submodel")
			require.NoError(t, err)
			require.Empty(t, endpoints[12])
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
