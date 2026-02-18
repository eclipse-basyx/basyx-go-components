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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"database/sql"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestDeleteSubmodelSuccessCleansLargeObjectsAndDeletesSubmodel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	sut := &SubmodelDatabase{db: db}
	submodelID := "sm-1"
	submodelDatabaseID := 101

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(submodelDatabaseID))
	mock.ExpectQuery(`SELECT .*file_oid.*FROM .*submodel_element.*file_data`).
		WillReturnRows(sqlmock.NewRows([]string{"file_oid"}).AddRow(int64(9001)))
	mock.ExpectExec(`SELECT .*lo_unlink`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`DELETE FROM .*submodel`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = sut.DeleteSubmodel(submodelID)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteSubmodelNotFoundReturnsErrNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	sut := &SubmodelDatabase{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	err = sut.DeleteSubmodel("missing-submodel")
	require.Error(t, err)
	require.True(t, common.IsErrNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteSubmodelDeleteFailsRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	sut := &SubmodelDatabase{db: db}
	submodelID := "sm-delete-fail"
	submodelDatabaseID := 202

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(submodelDatabaseID))
	mock.ExpectQuery(`SELECT .*file_oid.*FROM .*submodel_element.*file_data`).
		WillReturnRows(sqlmock.NewRows([]string{"file_oid"}))
	mock.ExpectExec(`DELETE FROM .*submodel`).
		WillReturnError(errors.New("delete failed"))
	mock.ExpectRollback()

	err = sut.DeleteSubmodel(submodelID)
	require.Error(t, err)
	require.True(t, common.IsInternalServerError(err))
	require.Contains(t, err.Error(), "SMREPO-DELSM-DELETESM")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteSubmodelCommitFailsReturnsInternalError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	sut := &SubmodelDatabase{db: db}
	submodelID := "sm-commit-fail"
	submodelDatabaseID := 303

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(submodelDatabaseID))
	mock.ExpectQuery(`SELECT .*file_oid.*FROM .*submodel_element.*file_data`).
		WillReturnRows(sqlmock.NewRows([]string{"file_oid"}))
	mock.ExpectExec(`DELETE FROM .*submodel`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

	err = sut.DeleteSubmodel(submodelID)
	require.Error(t, err)
	require.True(t, common.IsInternalServerError(err))
	require.Contains(t, err.Error(), "SMREPO-DELSM-COMMIT")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteSubmodelOrphanCleanupFailsRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	sut := &SubmodelDatabase{db: db}
	submodelID := "sm-unlink-fail"
	submodelDatabaseID := 404

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(submodelDatabaseID))
	mock.ExpectQuery(`SELECT .*file_oid.*FROM .*submodel_element.*file_data`).
		WillReturnRows(sqlmock.NewRows([]string{"file_oid"}).AddRow(int64(9999)))
	mock.ExpectExec(`SELECT .*lo_unlink`).
		WillReturnError(errors.New("unlink failed"))
	mock.ExpectRollback()

	err = sut.DeleteSubmodel(submodelID)
	require.Error(t, err)
	require.True(t, common.IsInternalServerError(err))
	require.Contains(t, err.Error(), "SMREPO-DELSM-UNLINKLO")
	require.NoError(t, mock.ExpectationsWereMet())
}
