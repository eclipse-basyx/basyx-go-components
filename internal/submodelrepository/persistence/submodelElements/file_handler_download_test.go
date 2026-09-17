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

package submodelelements

import (
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestReadDownloadFileMetadataUsesReferenceFileNameWhenElementFileNameIsNull(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT .*`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content_type", "coalesce", "value"}).
			AddRow(int64(17), "application/pdf", "supplement.pdf", "/aasx/files/manual.pdf"))
	mock.ExpectRollback()

	metadata, err := readDownloadFileMetadata(t.Context(), tx, "urn:sm", "Documents.Manual")
	require.NoError(t, err)
	require.Equal(t, int64(17), metadata.elementID)
	require.Equal(t, "application/pdf", metadata.contentType)
	require.Equal(t, "supplement.pdf", metadata.fileName)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReadDownloadFileMetadataUsesPathFallbackWhenFileNameIsNull(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT .*`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content_type", "coalesce", "value"}).
			AddRow(int64(17), "application/pdf", nil, "/aasx/files/manual.pdf"))
	mock.ExpectRollback()

	metadata, err := readDownloadFileMetadata(t.Context(), tx, "urn:sm", "Documents.Manual")
	require.NoError(t, err)
	require.Equal(t, "manual.pdf", metadata.fileName)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReadDownloadFileMetadataReportsMissingFileAsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT .*`).WillReturnRows(sqlmock.NewRows([]string{"id", "content_type", "coalesce", "value"}))
	mock.ExpectRollback()

	_, err = readDownloadFileMetadata(t.Context(), tx, "urn:sm", "Documents.Missing")
	require.Error(t, err)
	require.True(t, common.IsErrNotFound(err))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveDownloadFileNameUsesAttachmentForEmptyPath(t *testing.T) {
	fileName := resolveDownloadFileName(sql.NullString{}, sql.NullString{}, "")
	require.Equal(t, "attachment", fileName)
}

func TestResolveDownloadFileNameUsesFileValueWhenMetadataIsEmpty(t *testing.T) {
	fileName := resolveDownloadFileName(
		sql.NullString{String: "", Valid: true},
		sql.NullString{String: "/aasx/files/manual.pdf", Valid: true},
		"Documents.Manual",
	)
	require.Equal(t, "manual.pdf", fileName)
}

func TestReadLegacyFileOIDReportsNullOIDAsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT .*`).WillReturnRows(sqlmock.NewRows([]string{"file_oid"}).AddRow(nil))
	mock.ExpectRollback()

	_, err = readLegacyFileOID(t.Context(), tx, 17)
	require.Error(t, err)
	require.True(t, common.IsErrNotFound(err))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
