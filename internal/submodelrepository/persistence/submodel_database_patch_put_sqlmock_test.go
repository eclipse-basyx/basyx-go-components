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
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestPatchSubmodelIDMismatchReturnsBadRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-body")

	err = sut.PatchSubmodel("sm-path", submodel)
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPatchSubmodelNotFoundRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-missing")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	err = sut.PatchSubmodel("sm-missing", submodel)
	require.Error(t, err)
	require.True(t, common.IsErrNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPatchSubmodelSuccessReplacesSubmodel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-1")
	idShort := "sm1"
	submodel.SetIDShort(&idShort)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(100))
	mock.ExpectQuery(`SELECT .*file_oid.*FROM .*submodel_element.*file_data`).
		WillReturnRows(sqlmock.NewRows([]string{"file_oid"}))
	mock.ExpectExec(`DELETE FROM .*submodel`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO .*submodel.*RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(200))
	mock.ExpectExec(`INSERT INTO .*submodel_payload`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = sut.PatchSubmodel("sm-1", submodel)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutSubmodelIDMismatchReturnsBadRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-body")

	isUpdate, err := sut.PutSubmodel("sm-path", submodel)
	require.Error(t, err)
	require.False(t, isUpdate)
	require.True(t, common.IsErrBadRequest(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutSubmodelCreatePathReturnsFalse(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-new")
	idShort := "smnew"
	submodel.SetIDShort(&idShort)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO .*submodel.*RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(300))
	mock.ExpectExec(`INSERT INTO .*submodel_payload`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	isUpdate, err := sut.PutSubmodel("sm-new", submodel)
	require.NoError(t, err)
	require.False(t, isUpdate)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutSubmodelUpdatePathReturnsTrue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-existing")
	idShort := "smexisting"
	submodel.SetIDShort(&idShort)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(400))
	mock.ExpectQuery(`SELECT .*file_oid.*FROM .*submodel_element.*file_data`).
		WillReturnRows(sqlmock.NewRows([]string{"file_oid"}))
	mock.ExpectExec(`DELETE FROM .*submodel`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO .*submodel.*RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(401))
	mock.ExpectExec(`INSERT INTO .*submodel_payload`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	isUpdate, err := sut.PutSubmodel("sm-existing", submodel)
	require.NoError(t, err)
	require.True(t, isUpdate)
	require.NoError(t, mock.ExpectationsWereMet())
}
