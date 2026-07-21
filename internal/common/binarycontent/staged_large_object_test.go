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
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
* IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
* CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
* TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
* SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package binarycontent

import (
	"context"
	"database/sql"
	"io"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func expectStagedLargeObjectWrite(mock sqlmock.Sqlmock, content string) {
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT lo_create`).WillReturnRows(sqlmock.NewRows([]string{"lo_create"}).AddRow(17))
	mock.ExpectQuery(`SELECT lo_open`).WillReturnRows(sqlmock.NewRows([]string{"lo_open"}).AddRow(23))
	mock.ExpectQuery(`SELECT lowrite`).WillReturnRows(sqlmock.NewRows([]string{"lowrite"}).AddRow(len(content)))
	mock.ExpectExec(`SELECT lo_close`).WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestStagedLargeObjectCloseRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectStagedLargeObjectWrite(mock, "package")
	mock.ExpectRollback()

	staged, err := Stage(t.Context(), db, strings.NewReader("package"), 1024)
	require.NoError(t, err)
	require.Equal(t, int64(len("package")), staged.Size())
	require.NoError(t, staged.Close())
	require.NoError(t, staged.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStagedLargeObjectPromoteCommits(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectStagedLargeObjectWrite(mock, "package")
	mock.ExpectCommit()

	staged, err := Stage(t.Context(), db, strings.NewReader("package"), 1024)
	require.NoError(t, err)
	err = staged.Promote(t.Context(), func(_ context.Context, tx *sql.Tx, oid int64, size int64) error {
		require.NotNil(t, tx)
		require.Equal(t, int64(17), oid)
		require.Equal(t, int64(len("package")), size)
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, staged.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpenOIDClosesDescriptorAndTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT lo_open`).WillReturnRows(sqlmock.NewRows([]string{"lo_open"}).AddRow(23))
	mock.ExpectQuery(`SELECT loread`).WillReturnRows(sqlmock.NewRows([]string{"loread"}).AddRow([]byte("package")))
	mock.ExpectQuery(`SELECT loread`).WillReturnRows(sqlmock.NewRows([]string{"loread"}).AddRow([]byte{}))
	mock.ExpectExec(`SELECT lo_close`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	reader, err := OpenOID(t.Context(), db, 17)
	require.NoError(t, err)
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "package", string(content))
	require.NoError(t, reader.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}
