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
	"context"
	"database/sql"
	"errors"

	"github.com/doug-martin/goqu/v9"
)

// WithPostgreSQLGenericPlanTx reuses generic plans while read runs in tx, then
// restores the previous transaction-local plan mode before returning.
func WithPostgreSQLGenericPlanTx[T any](ctx context.Context, tx *sql.Tx, read func() (T, error)) (result T, err error) {
	restore, err := forcePostgreSQLGenericPlanTx(ctx, tx)
	if err != nil {
		return result, err
	}
	defer func() {
		if restoreErr := restore(); restoreErr != nil {
			var zero T
			result = zero
			err = errors.Join(err, restoreErr)
		}
	}()
	return read()
}

func forcePostgreSQLGenericPlanTx(ctx context.Context, tx *sql.Tx) (func() error, error) {
	previousMode, err := currentPostgreSQLPlanCacheModeTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	const genericPlanMode = "force_generic_plan"
	if previousMode == genericPlanMode {
		return func() error { return nil }, nil
	}
	if err = setPostgreSQLPlanCacheModeTx(ctx, tx, genericPlanMode); err != nil {
		return nil, err
	}
	return func() error {
		return setPostgreSQLPlanCacheModeTx(ctx, tx, previousMode)
	}, nil
}

func currentPostgreSQLPlanCacheModeTx(ctx context.Context, tx *sql.Tx) (string, error) {
	query := goqu.Dialect("postgres").Select(
		goqu.Func("current_setting", PostgreSQLTextLiteral("plan_cache_mode")),
	)
	sqlQuery, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return "", NewInternalServerError("COMMON-PGPLAN-GETPLANMODE-BUILDQ " + err.Error())
	}
	var mode string
	if err = tx.QueryRowContext(ctx, sqlQuery, args...).Scan(&mode); err != nil {
		return "", NewInternalServerError("COMMON-PGPLAN-GETPLANMODE-EXECQ " + err.Error())
	}
	return mode, nil
}

func setPostgreSQLPlanCacheModeTx(ctx context.Context, tx *sql.Tx, mode string) error {
	query := goqu.Dialect("postgres").Select(
		goqu.Func(
			"set_config",
			PostgreSQLTextLiteral("plan_cache_mode"),
			PostgreSQLTextLiteral(mode),
			goqu.L("TRUE"),
		),
	)
	sqlQuery, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return NewInternalServerError("COMMON-PGPLAN-SETPLANMODE-BUILDQ " + err.Error())
	}
	var configuredMode string
	if err = tx.QueryRowContext(ctx, sqlQuery, args...).Scan(&configuredMode); err != nil {
		return NewInternalServerError("COMMON-PGPLAN-SETPLANMODE-EXECQ " + err.Error())
	}
	if configuredMode != mode {
		return NewInternalServerError("COMMON-PGPLAN-SETPLANMODE-MISMATCH expected " + mode + " but PostgreSQL returned " + configuredMode)
	}
	return nil
}
