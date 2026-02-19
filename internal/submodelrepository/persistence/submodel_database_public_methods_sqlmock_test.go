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
	"os"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/stretchr/testify/require"
)

func TestNewSubmodelDatabaseInvalidDSNReturnsError(t *testing.T) {
	t.Parallel()

	sut, err := NewSubmodelDatabase("bad dsn", 0, 0, 0, "", nil, false)
	require.Error(t, err)
	require.Nil(t, sut)
}

func TestGetSubmodelsDatabaseQueryError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}

	mock.ExpectQuery(`SELECT .*FROM .*submodel`).
		WillReturnError(errors.New("query failed"))

	items, cursor, err := sut.GetSubmodels(10, "", "")
	require.Error(t, err)
	require.Nil(t, items)
	require.Empty(t, cursor)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSubmodelByIDReturnsErrorWhenParallelReadsFail(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.MatchExpectationsInOrder(false)

	sut := &SubmodelDatabase{db: db}

	mock.ExpectQuery(`SELECT .*`).WillReturnError(errors.New("read failed"))

	item, err := sut.GetSubmodelByID("")
	require.Error(t, err)
	require.Nil(t, item)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateSubmodelInsertFailureRollsBack(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	submodel := types.NewSubmodel("sm-create")
	idShort := "create"
	submodel.SetIDShort(&idShort)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO .*submodel.*RETURNING`).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	err = sut.CreateSubmodel(submodel)
	require.Error(t, err)
	require.True(t, common.IsInternalServerError(err))
	require.Contains(t, err.Error(), "SMREPO-NEWSM-CREATE-EXECSQL")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSubmodelElementEmptyPathReturnsBadRequest(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}

	elem, err := sut.GetSubmodelElement("sm", "", false)
	require.Error(t, err)
	require.Nil(t, elem)
	require.True(t, common.IsErrBadRequest(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSubmodelElementsEmptySubmodelIDReturnsBadRequest(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}

	elems, cursor, err := sut.GetSubmodelElements("", nil, "", false)
	require.Error(t, err)
	require.Nil(t, elems)
	require.Empty(t, cursor)
	require.True(t, common.IsErrBadRequest(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAddSubmodelElementSubmodelNotFoundRollsBack(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	var elem types.ISubmodelElement

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	err = sut.AddSubmodelElement("missing", elem)
	require.Error(t, err)
	require.True(t, common.IsErrNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAddSubmodelElementWithPathSubmodelNotFoundRollsBack(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}
	var elem types.ISubmodelElement

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	err = sut.AddSubmodelElementWithPath("missing", "container", elem)
	require.Error(t, err)
	require.True(t, common.IsErrNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteSubmodelElementByPathFailureRollsBack(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*`).WillReturnError(errors.New("delete failed"))
	mock.ExpectRollback()

	err = sut.DeleteSubmodelElementByPath("sm", "a.b")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateSubmodelElementModelTypeLookupFails(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}

	mock.ExpectQuery(`SELECT .*`).WillReturnError(errors.New("lookup failed"))

	err = sut.UpdateSubmodelElement("sm", "path", nil, true)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateSubmodelElementValueOnlyModelTypeLookupFails(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}

	mock.ExpectQuery(`SELECT .*`).WillReturnError(errors.New("lookup failed"))

	err = sut.UpdateSubmodelElementValueOnly("sm", "path", nil)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateSubmodelValueOnlyPropagatesElementError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}

	valueOnly := gen.SubmodelValue{"x": nil}
	mock.ExpectQuery(`SELECT .*`).WillReturnError(errors.New("lookup failed"))

	err = sut.UpdateSubmodelValueOnly("sm", valueOnly)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUploadFileAttachmentSubmodelLookupFails(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	tmp, err := os.CreateTemp(t.TempDir(), "upload-*.txt")
	require.NoError(t, err)
	_, err = tmp.WriteString("payload")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	uploadFile, err := os.Open(tmp.Name())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, uploadFile.Close())
	}()

	sut := &SubmodelDatabase{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).WillReturnError(errors.New("lookup failed"))
	mock.ExpectRollback()

	err = sut.UploadFileAttachment("sm", "file", uploadFile, "file.txt")
	require.Error(t, err)
	_, statErr := os.Stat(tmp.Name())
	require.NoError(t, statErr)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDownloadFileAttachmentSubmodelLookupFails(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).WillReturnError(errors.New("lookup failed"))
	mock.ExpectRollback()

	content, contentType, fileName, err := sut.DownloadFileAttachment("sm", "file")
	require.Error(t, err)
	require.Nil(t, content)
	require.Empty(t, contentType)
	require.Empty(t, fileName)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteFileAttachmentSubmodelLookupFails(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &SubmodelDatabase{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).WillReturnError(errors.New("lookup failed"))
	mock.ExpectRollback()

	err = sut.DeleteFileAttachment("sm", "file")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQuerySubmodelsNilQueryWrapperReturnsBadRequest(t *testing.T) {
	t.Parallel()

	sut := &SubmodelDatabase{}

	items, cursor, err := sut.QuerySubmodels(10, "", nil, false)
	require.Error(t, err)
	require.Nil(t, items)
	require.Empty(t, cursor)
	require.True(t, common.IsErrBadRequest(err))
	require.Contains(t, err.Error(), "SMREPO-QUERYSMS-INVALIDQUERY")
}

func TestQuerySubmodelsMissingConditionReturnsBadRequest(t *testing.T) {
	t.Parallel()

	sut := &SubmodelDatabase{}
	queryWrapper := &grammar.QueryWrapper{}

	items, cursor, err := sut.QuerySubmodels(10, "", queryWrapper, false)
	require.Error(t, err)
	require.Nil(t, items)
	require.Empty(t, cursor)
	require.True(t, common.IsErrBadRequest(err))
	require.Contains(t, err.Error(), "SMREPO-QUERYSMS-INVALIDQUERY")
}

func TestIsSiblingIDShortCollisionEmptyIDShortReturnsFalse(t *testing.T) {
	t.Parallel()

	element := types.NewProperty(types.DataTypeDefXSDString)

	collision := isSiblingIDShortCollision(nil, 1, nil, element)
	require.False(t, collision)
}

func TestIsSiblingIDShortCollisionTopLevelReturnsTrue(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	idShort := "duplicate"
	element := types.NewProperty(types.DataTypeDefXSDString)
	element.SetIDShort(&idShort)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM "submodel_element"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	collision := isSiblingIDShortCollision(tx, 42, nil, element)
	require.True(t, collision)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestIsSiblingIDShortCollisionNestedReturnsFalse(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	idShort := "nested"
	element := types.NewProperty(types.DataTypeDefXSDString)
	element.SetIDShort(&idShort)

	parentID := 99
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM "submodel_element"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	collision := isSiblingIDShortCollision(tx, 42, &parentID, element)
	require.False(t, collision)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
