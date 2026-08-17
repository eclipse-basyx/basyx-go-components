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
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestInsertSubmodelElementsExecutesGraphAsSingleBatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	property := types.NewProperty(types.DataTypeDefXSDString)
	idShort := "temperature"
	property.SetIDShort(&idShort)
	mock.ExpectQuery(`SELECT .*nextval.*generate_series`).
		WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(101))
	mock.ExpectExec(`(?s)INSERT INTO "submodel_element".*INSERT INTO "property_element"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	ids, err := InsertSubmodelElementsForSubmodelDatabaseIDContext(
		t.Context(),
		db,
		42,
		[]types.ISubmodelElement{property},
		tx,
		nil,
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, []int{101}, ids)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertSubmodelElementsRollsBackOwnedTransactionAfterConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	property := types.NewProperty(types.DataTypeDefXSDString)
	idShort := "temperature"
	property.SetIDShort(&idShort)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*nextval.*generate_series`).
		WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(101))
	mock.ExpectExec(`(?s)INSERT INTO "submodel_element".*INSERT INTO "property_element"`).
		WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "uq_sibling_idshort"})
	mock.ExpectRollback()

	_, err = InsertSubmodelElementsForSubmodelDatabaseID(
		db,
		42,
		[]types.ISubmodelElement{property},
		nil,
		nil,
	)
	require.Error(t, err)
	require.True(t, common.IsErrConflict(err), "expected conflict error, got %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertSubmodelElementsMapsPathConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	property := types.NewProperty(types.DataTypeDefXSDString)
	idShort := "temperature"
	property.SetIDShort(&idShort)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*nextval.*generate_series`).
		WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(101))
	mock.ExpectExec(`(?s)INSERT INTO "submodel_element".*INSERT INTO "property_element"`).
		WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "uq_sme_path"})
	mock.ExpectRollback()

	_, err = InsertSubmodelElementsForSubmodelDatabaseID(
		db,
		42,
		[]types.ISubmodelElement{property},
		nil,
		nil,
	)
	require.Error(t, err)
	require.True(t, common.IsErrConflict(err), "expected conflict error, got %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertSubmodelElementsDependentConflictRemainsInternal(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	property := types.NewProperty(types.DataTypeDefXSDString)
	idShort := "temperature"
	property.SetIDShort(&idShort)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*nextval.*generate_series`).
		WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(101))
	postgresErr := &pgconn.PgError{Code: "23505", ConstraintName: "property_element_pkey"}
	mock.ExpectExec(`(?s)INSERT INTO "submodel_element".*INSERT INTO "property_element"`).
		WillReturnError(postgresErr)
	mock.ExpectRollback()

	_, err = InsertSubmodelElementsForSubmodelDatabaseID(
		db,
		42,
		[]types.ISubmodelElement{property},
		nil,
		nil,
	)
	require.Error(t, err)
	require.True(t, common.IsInternalServerError(err), "expected internal-server error, got %v", err)
	require.False(t, common.IsErrConflict(err), "dependent conflict must not be mapped as an SME conflict")
	var unwrapped *pgconn.PgError
	require.True(t, errors.As(err, &unwrapped), "expected PostgreSQL cause, got %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}
