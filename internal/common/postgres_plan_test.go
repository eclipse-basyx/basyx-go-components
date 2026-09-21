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

package common

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func expectPlanMode(mock sqlmock.Sqlmock, mode string) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT current_setting('plan_cache_mode'::text)")).
		WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow(mode))
}

func expectSetPlanMode(mock sqlmock.Sqlmock, mode string) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta("SELECT set_config('plan_cache_mode'::text, '" + mode + "'::text, TRUE)"))
}

func TestWithPostgreSQLGenericPlanTxRestoresMode(t *testing.T) {
	for _, mode := range []string{"auto", "force_custom_plan", "force_generic_plan"} {
		t.Run(mode, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			mock.ExpectBegin()
			tx, err := db.Begin()
			require.NoError(t, err)
			expectPlanMode(mock, mode)
			if mode != "force_generic_plan" {
				expectSetPlanMode(mock, "force_generic_plan").WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow("force_generic_plan"))
				expectSetPlanMode(mock, mode).WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow(mode))
			}
			result, err := WithPostgreSQLGenericPlanTx(ContextWithConfig(t.Context(), &Config{}), tx, func() (string, error) {
				return "descriptor", nil
			})
			require.NoError(t, err)
			require.Equal(t, "descriptor", result)
			mock.ExpectRollback()
			require.NoError(t, tx.Rollback())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestWithPostgreSQLGenericPlanTxHandlesErrors(t *testing.T) {
	readErr := errors.New("TEST-PGPLAN-READ")
	setupErr := errors.New("TEST-PGPLAN-SETUP")
	restoreErr := errors.New("TEST-PGPLAN-RESTORE")
	tests := []struct {
		name          string
		setup         func(sqlmock.Sqlmock)
		readError     error
		readCalled    bool
		expectedError string
	}{
		{"get mode", func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery("SELECT current_setting").WillReturnError(setupErr)
		}, nil, false, "COMMON-PGPLAN-GETPLANMODE-EXECQ"},
		{"set mode", func(mock sqlmock.Sqlmock) {
			expectPlanMode(mock, "auto")
			expectSetPlanMode(mock, "force_generic_plan").WillReturnError(setupErr)
		}, nil, false, "COMMON-PGPLAN-SETPLANMODE-EXECQ"},
		{"mode mismatch", func(mock sqlmock.Sqlmock) {
			expectPlanMode(mock, "auto")
			expectSetPlanMode(mock, "force_generic_plan").WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow("auto"))
		}, nil, false, "COMMON-PGPLAN-SETPLANMODE-MISMATCH"},
		{"read", func(mock sqlmock.Sqlmock) {
			expectPlanMode(mock, "auto")
			expectSetPlanMode(mock, "force_generic_plan").WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow("force_generic_plan"))
			expectSetPlanMode(mock, "auto").WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow("auto"))
		}, readErr, true, readErr.Error()},
		{"restore", func(mock sqlmock.Sqlmock) {
			expectPlanMode(mock, "auto")
			expectSetPlanMode(mock, "force_generic_plan").WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow("force_generic_plan"))
			expectSetPlanMode(mock, "auto").WillReturnError(restoreErr)
		}, nil, true, restoreErr.Error()},
		{"read and restore", func(mock sqlmock.Sqlmock) {
			expectPlanMode(mock, "auto")
			expectSetPlanMode(mock, "force_generic_plan").WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow("force_generic_plan"))
			expectSetPlanMode(mock, "auto").WillReturnError(restoreErr)
		}, readErr, true, restoreErr.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			mock.ExpectBegin()
			tx, err := db.Begin()
			require.NoError(t, err)
			tt.setup(mock)
			called := false
			result, err := WithPostgreSQLGenericPlanTx(ContextWithConfig(t.Context(), &Config{}), tx, func() (string, error) {
				called = true
				if tt.readError != nil {
					return "", tt.readError
				}
				return "descriptor", nil
			})
			require.ErrorContains(t, err, tt.expectedError)
			if tt.readError != nil {
				require.ErrorIs(t, err, tt.readError)
			}
			require.Empty(t, result)
			require.Equal(t, tt.readCalled, called)
			mock.ExpectRollback()
			require.NoError(t, tx.Rollback())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
