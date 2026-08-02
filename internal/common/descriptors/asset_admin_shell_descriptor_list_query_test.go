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

func TestListAssetAdministrationShellDescriptorsUsesOneQuery(t *testing.T) {
	assertAASDescriptorListUsesOneQuery(common.ContextWithConfig(t.Context(), &common.Config{}), t)
}

func TestListAssetAdministrationShellDescriptorsUsesOneQueryWithRestrictiveFragmentMask(t *testing.T) {
	deny := false
	ctx := auth.WithQueryFilter(common.ContextWithConfig(t.Context(), &common.Config{}), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			grammar.FragmentStringPattern("$aasdesc#specificAssetIds[]"): {Boolean: &deny},
		},
	})
	assertAASDescriptorListUsesOneQuery(ctx, t)
}

func assertAASDescriptorListUsesOneQuery(ctx context.Context, t *testing.T) {
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
