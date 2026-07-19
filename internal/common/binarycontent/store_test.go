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

package binarycontent

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestSafeFileName(t *testing.T) {
	tests := []struct {
		name      string
		fileName  string
		expected  string
		wantError bool
	}{
		{name: "ordinary", fileName: "manual.pdf", expected: "manual.pdf"},
		{name: "unicode", fileName: "Prüfbericht 1.pdf", expected: "Prüfbericht 1.pdf"},
		{name: "trims whitespace", fileName: " report.txt ", expected: "report.txt"},
		{name: "empty", fileName: "  ", wantError: true},
		{name: "parent", fileName: "..", wantError: true},
		{name: "slash", fileName: "folder/report.txt", wantError: true},
		{name: "backslash", fileName: `folder\report.txt`, wantError: true},
		{name: "encoded slash", fileName: "folder%2freport.txt", wantError: true},
		{name: "control", fileName: "report\n.txt", wantError: true},
		{name: "maximum bytes", fileName: strings.Repeat("a", maxSafeFileNameBytes), expected: strings.Repeat("a", maxSafeFileNameBytes)},
		{name: "too many bytes", fileName: strings.Repeat("a", maxSafeFileNameBytes+1), wantError: true},
		{name: "unicode byte limit", fileName: strings.Repeat("ü", 128), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := SafeFileName(test.fileName)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestNewReferenceUsesFreshOpaqueManagedPath(t *testing.T) {
	content := Content{ID: 1, SHA256: strings.Repeat("a", 64), SizeBytes: 4, OID: 7}
	first, err := NewReference(5, content, "manual.pdf")
	require.NoError(t, err)
	second, err := NewReference(5, content, "manual.pdf")
	require.NoError(t, err)

	require.NotEqual(t, first.PathToken, second.PathToken)
	require.Len(t, first.PathToken, 32)
	require.Equal(t, "/aasx/files/"+first.PathToken+"/manual.pdf", first.ManagedPath())
}

func TestStoreTxRejectsContentAboveConfiguredLimitBeforeWriting(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT lo_create`).WillReturnRows(sqlmock.NewRows([]string{"lo_create"}).AddRow(17))
	mock.ExpectQuery(`SELECT lo_open`).WillReturnRows(sqlmock.NewRows([]string{"lo_open"}).AddRow(23))
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)
	cfg := &common.Config{}
	cfg.General.UploadMaxSizeBytes = 3
	ctx := common.ContextWithConfig(context.Background(), cfg)

	_, _, _, err = writeTransientLargeObjectTx(ctx, tx, strings.NewReader("four"), common.UploadMaxSizeBytesFromContext(ctx))
	require.ErrorContains(t, err, "BINARYCONTENT-STORE-TOOLARGE")
	require.True(t, common.IsErrBadRequest(err))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLockContentRowsUsesDeterministicOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "id" FROM "binary_content" WHERE \("id" IN \(1, 2\)\) ORDER BY "id" ASC FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2))
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, lockContentRowsTx(t.Context(), tx, 2, 1, 2))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
