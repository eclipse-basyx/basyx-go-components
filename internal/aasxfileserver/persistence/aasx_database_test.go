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
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/pagination"
	"github.com/stretchr/testify/require"
)

const maximumPackageReadRequest = 512 * 1024

var errPackageReadRequestTooLarge = errors.New("package reader requested a full-file-sized buffer")

var errPromotionIntercepted = errors.New("staged upload promotion intercepted")

type guardedReadSeeker struct {
	io.ReadSeeker
	maximumReadRequest int
}

type promotionTrackingUpload struct {
	io.ReadSeeker
	size     int64
	promoted bool
}

func (upload *promotionTrackingUpload) Size() int64 {
	return upload.size
}

func (upload *promotionTrackingUpload) Promote(context.Context, func(context.Context, *sql.Tx, int64, int64) error) error {
	upload.promoted = true
	return errPromotionIntercepted
}

func (*promotionTrackingUpload) Close() error {
	return nil
}

func (reader *guardedReadSeeker) Read(destination []byte) (int, error) {
	if len(destination) > reader.maximumReadRequest {
		return 0, errPackageReadRequestTooLarge
	}
	return reader.ReadSeeker.Read(destination)
}

func TestAASXFileServerReadPoolSelection(t *testing.T) {
	writer, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = writer.Close() }()
	reader, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()

	backend, err := NewAASXFileServerDatabaseFromPools(writer, reader)
	require.NoError(t, err)
	require.Same(t, reader, backend.readDB(t.Context()))
	require.Same(t, writer, backend.readDB(common.WithWriterPostgresReads(t.Context())))
}

func TestParseCursorID(t *testing.T) {
	t.Parallel()

	cursor, err := ParseCursorID("")
	require.NoError(t, err)
	require.Equal(t, int64(0), cursor)

	cursor, err = ParseCursorID("42")
	require.NoError(t, err)
	require.Equal(t, int64(42), cursor)

	_, err = ParseCursorID("-1")
	require.Error(t, err)

	_, err = ParseCursorID("abc")
	require.Error(t, err)
}

func TestValidatePackageForUploadRejectsMalformedAASX(t *testing.T) {
	t.Parallel()

	err := validatePackageForUpload(bytes.NewReader([]byte("not an AASX package")), common.AASXLimitsFromConfig(nil))
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err), "expected bad request, got %v", err)
}

func TestValidatePackageForUploadDoesNotReadAASXIntoOneBuffer(t *testing.T) {
	t.Parallel()

	filePath := filepath.Clean("../../aasenvironment/integration_tests/testdata/IESEDriveMotorDM3000.aasx")
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()
	fileInfo, err := file.Stat()
	require.NoError(t, err)
	require.Greater(t, fileInfo.Size(), int64(maximumPackageReadRequest))

	guardedPackage := &guardedReadSeeker{ReadSeeker: file, maximumReadRequest: maximumPackageReadRequest}
	require.NoError(t, validatePackageForUpload(guardedPackage, common.AASXLimitsFromConfig(nil)))
}

func TestCreatePackageTransfersOriginalStagedUploadToPromotion(t *testing.T) {
	t.Parallel()

	filePath := filepath.Clean("../../aasenvironment/integration_tests/testdata/IESEDriveMotorDM3000.aasx")
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()
	fileInfo, err := file.Stat()
	require.NoError(t, err)

	upload := &promotionTrackingUpload{ReadSeeker: file, size: fileInfo.Size()}
	backend := &AASXFileServerDatabase{}
	_, err = backend.CreatePackage(t.Context(), "ownership-transfer", upload, nil, filepath.Base(filePath))
	require.ErrorIs(t, err, errPromotionIntercepted)
	require.True(t, upload.promoted, "persistence replaced the caller's staged upload instead of promoting it")
}

func TestNormalizeAASIDs(t *testing.T) {
	t.Parallel()

	result := normalizeAASIDs([]string{"urn:a,1", " id2 ", "", "id2", "urn:a,1"})
	require.Equal(t, []string{"urn:a,1", "id2"}, result)
}

func TestNormalizeFileName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "provided.aasx", normalizeFileName("  provided.aasx ", "upload-file.aasx"))
	require.Equal(t, "upload-file.aasx", normalizeFileName("", "upload-file.aasx"))
}

func TestValidatePackageForUploadAcceptsXMLAndJSONEnvironments(t *testing.T) {
	t.Parallel()

	for name, relativeAASX := range map[string]string{
		"xml environment aasx":  "../../aasenvironment/integration_tests/testdata/IESEDriveMotorDM3000.aasx",
		"json environment aasx": "../../aasenvironment/integration_tests/testdata/ProductionPlanSFKL.aasx",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			file, err := os.Open(filepath.Clean(relativeAASX))
			require.NoError(t, err)
			defer func() { _ = file.Close() }()

			require.NoError(t, validatePackageForUpload(file, common.AASXLimitsFromConfig(nil)))
		})
	}
}

type positiveInt64Argument struct{}

func (positiveInt64Argument) Match(value driver.Value) bool {
	number, ok := value.(int64)
	return ok && number > 0
}

func TestListPackagesLargeLimitsKeepLookaheadPositive(t *testing.T) {
	for name, limit := range map[string]int32{
		"largest supported limit": pagination.MaxSupportedLimit,
		"largest int32 limit":     math.MaxInt32,
	} {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			backend, err := NewAASXFileServerDatabaseFromPools(db, db)
			require.NoError(t, err)

			mock.ExpectBegin()
			mock.ExpectQuery("SELECT").
				WithArgs(positiveInt64Argument{}).
				WillReturnRows(sqlmock.NewRows([]string{"id", "package_id", "file_name"}))
			mock.ExpectCommit()

			records, nextCursor, err := backend.ListPackages(t.Context(), limit, 0, "")
			require.NoError(t, err)
			require.Empty(t, records)
			require.Zero(t, nextCursor)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
