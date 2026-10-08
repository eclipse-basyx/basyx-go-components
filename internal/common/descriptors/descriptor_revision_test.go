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
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/conditional/conditionaltest"
	"github.com/stretchr/testify/require"
)

const deleteEntryPattern = `DELETE FROM "aas_identifier" WHERE \("aas_identifier"."aasid" = \$1\) RETURNING EXISTS \(SELECT 1 FROM "specific_asset_id" WHERE \(\("specific_asset_id"."aasref" = "aas_identifier"."id"\) AND \("specific_asset_id"."descriptor_id" IS NOT NULL\)\)\)`

func TestDeleteDiscoveryEntryTouchesTheDescriptorOnlyWhenItOwnedLinks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rows  *sqlmock.Rows
		found bool
		touch bool
	}{
		{name: "missing", rows: sqlmock.NewRows([]string{"exists"}), found: false},
		{name: "discovery only", rows: sqlmock.NewRows([]string{"exists"}).AddRow(false), found: true},
		{name: "descriptor links", rows: sqlmock.NewRows([]string{"exists"}).AddRow(true), found: true, touch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			mock.ExpectBegin()
			tx, err := db.Begin()
			require.NoError(t, err)
			mock.ExpectQuery(deleteEntryPattern).WithArgs("urn:aas").WillReturnRows(tc.rows)

			found, err := DeleteDiscoveryEntryTx(context.Background(), tx, "urn:aas")
			require.NoError(t, err)
			require.Equal(t, tc.found, found)
			if tc.touch {
				conditionaltest.ExpectRevisionUpsert(mock, "aasDescriptor", "urn:aas")
			}
			mock.ExpectCommit()
			require.NoError(t, common.CommitTransaction(tx))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
