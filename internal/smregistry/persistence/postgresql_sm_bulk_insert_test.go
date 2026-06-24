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

package smregistrypostgresql

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestBulkSubmodelInsertSkipsReadbackForUnrestrictedCreate(t *testing.T) {
	previousHistoryConfig := history.ActiveConfig()
	t.Cleanup(func() {
		history.Configure(previousHistoryConfig)
	})
	history.Configure(history.Config{
		Mode:              history.ModeOff,
		Immutability:      history.ImmutabilityNone,
		AuditIdentityMode: history.AuditIdentityNone,
	})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	submodel := model.SubmodelDescriptor{
		Id: "sm-1",
		Endpoints: []model.Endpoint{
			{
				Interface: "SUBMODEL-3.0",
				ProtocolInformation: model.ProtocolInformation{
					Href: "https://example.org/submodels/sm-1",
				},
			},
		},
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT nextval`).
		WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(21)))
	mock.ExpectExec(`INSERT INTO "descriptor"`).
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	failedIndex, err := (&PostgreSQLSMDatabase{}).InsertSubmodelDescriptorsInTransaction(
		context.Background(),
		tx,
		[]model.SubmodelDescriptor{submodel},
	)
	require.NoError(t, err)
	require.Equal(t, -1, failedIndex)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}
