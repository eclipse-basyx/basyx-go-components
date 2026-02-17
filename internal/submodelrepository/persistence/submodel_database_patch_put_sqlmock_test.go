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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
	defer db.Close()

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
