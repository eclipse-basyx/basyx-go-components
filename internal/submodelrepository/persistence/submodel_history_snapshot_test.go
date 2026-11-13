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
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestSubmodelElementRootPathReturnsTopLevelPath(t *testing.T) {
	root, err := submodelElementRootPath("Collection.Items[2].Temperature")

	require.NoError(t, err)
	require.Equal(t, "Collection", root)
}

func TestReplaceSubmodelElementRootSnapshotReplacesOnlyAffectedRoot(t *testing.T) {
	snapshot := map[string]any{
		"id": "sm-1",
		submodelElementsSnapshotField: []any{
			map[string]any{"idShort": "First", "value": "old"},
			map[string]any{"idShort": "Second", "value": "untouched"},
		},
	}

	err := replaceSubmodelElementRootSnapshot(snapshot, "First", map[string]any{"idShort": "Renamed", "value": "new"})

	require.NoError(t, err)
	require.Equal(t, []any{
		map[string]any{"idShort": "Renamed", "value": "new"},
		map[string]any{"idShort": "Second", "value": "untouched"},
	}, snapshot[submodelElementsSnapshotField])
}

func TestReplaceSubmodelElementRootSnapshotDeletesLastRoot(t *testing.T) {
	snapshot := map[string]any{
		"id": "sm-1",
		submodelElementsSnapshotField: []any{
			map[string]any{"idShort": "Only"},
		},
	}

	err := replaceSubmodelElementRootSnapshot(snapshot, "Only", nil)

	require.NoError(t, err)
	_, exists := snapshot[submodelElementsSnapshotField]
	require.False(t, exists)
}

func TestGetSubmodelByIDAndDateHidesVersionFromRestrictedReaders(t *testing.T) {
	denied := false
	for name, filter := range map[string]*auth.QueryFilter{
		"backend formula": {Formula: &grammar.LogicalExpression{}},
		"fragment filter": {Filters: auth.FragmentFilters{
			"$sm#supplementalSemanticIds[]": auth.NewFragmentFilterPredicate(grammar.LogicalExpression{Boolean: &denied}, true),
		}},
	} {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			ctx := auth.WithQueryFilter(contextWithABACDisabled(t), filter)
			_, err = (&SubmodelDatabase{db: db}).GetSubmodelByIDAndDate(ctx, "urn:sm", time.Now())

			require.True(t, common.IsErrNotFound(err), "error=%v", err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGetSubmodelByIDAndDateReadsHistoryForUnrestrictedReaders(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	allowed := true
	ctx := auth.WithQueryFilter(contextWithABACDisabled(t), &auth.QueryFilter{Formula: &grammar.LogicalExpression{Boolean: &allowed}})
	mock.ExpectQuery(`SELECT "history"."history_id" FROM "submodel_history" AS "history"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}))

	_, err = (&SubmodelDatabase{db: db}).GetSubmodelByIDAndDate(ctx, "urn:sm", time.Now())

	require.True(t, common.IsErrNotFound(err), "error=%v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}
