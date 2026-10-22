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

package conditional

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestRevisionObserverRunsInsideTransactionAndCanPreventCommit(t *testing.T) {
	tx, mock := newMockTx(t)
	failure := errors.New("receipt failed")
	ctx := WithRevisionObserver(t.Context(), func(_ context.Context, actual *sql.Tx, revisions map[ResourceRef]int64) error {
		require.Same(t, tx, actual)
		require.Equal(t, int64(11), revisions[shellA])
		return failure
	})
	require.NoError(t, Touch(ctx, tx, shellA, OpCreate))
	mock.ExpectQuery(upsertPattern).WithArgs(shellA.Identifier, string(KindAAS)).WillReturnRows(sqlmock.NewRows([]string{"kind", "identifier", "revision"}).AddRow(string(KindAAS), shellA.Identifier, 11))
	_, err := FlushTx(tx)
	require.ErrorIs(t, err, failure)
	require.NoError(t, mock.ExpectationsWereMet())
}
