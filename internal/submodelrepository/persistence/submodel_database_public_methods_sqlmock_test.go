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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

	tmp, err := os.CreateTemp(t.TempDir(), "upload-*.txt")
	require.NoError(t, err)
	_, err = tmp.WriteString("payload")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	uploadFile, err := os.Open(tmp.Name())
	require.NoError(t, err)
	defer uploadFile.Close()

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
	defer db.Close()

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
	defer db.Close()

	sut := &SubmodelDatabase{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FROM .*submodel`).WillReturnError(errors.New("lookup failed"))
	mock.ExpectRollback()

	err = sut.DeleteFileAttachment("sm", "file")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
