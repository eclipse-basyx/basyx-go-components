package persistencepostgresql

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func setupMockDB(t *testing.T) (*PostgreSQLAASDatabase, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	pg := &PostgreSQLAASDatabase{
		DB: db,
	}

	cleanup := func() {
		db.Close()
	}

	return pg, mock, cleanup
}

func TestDeleteAASByID_Success(t *testing.T) {
	pg, mock, cleanup := setupMockDB(t)
	defer cleanup()

	mock.ExpectExec(`DELETE FROM "aas"`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := pg.DeleteAASByID("aas-id-123")
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}
func TestInsertBaseAAS_Success(t *testing.T) {
	pg, mock, cleanup := setupMockDB(t)
	defer cleanup()

	aas := model.AssetAdministrationShell{
		ID:        "aas-id-1",
		IdShort:   "MyAAS",
		Category:  "INSTANCE",
		ModelType: "AssetAdministrationShell",
	}

	mock.ExpectExec(`INSERT INTO "aas"`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := pg.insertBaseAAS(aas)
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}
func TestInsertAAS_FailsOnBaseInsert(t *testing.T) {
	pg, mock, cleanup := setupMockDB(t)
	defer cleanup()

	aas := model.AssetAdministrationShell{
		ID:        "aas-id-fail",
		IdShort:   "BrokenAAS",
		ModelType: "AssetAdministrationShell",
	}

	mock.ExpectExec(`INSERT INTO "aas"`).
		WillReturnError(errors.New("db insert failed"))

	err := pg.InsertAAS(aas)
	require.Error(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}
