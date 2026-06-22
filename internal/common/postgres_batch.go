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
	"fmt"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// PostgreSQLBatchStatement is one rendered PostgreSQL statement and the values
// that must be passed when executing it with pgx batch support.
type PostgreSQLBatchStatement struct {
	SQL  string
	Args []any
}

// PostgreSQLBatch stores rendered statements in the order required by their
// table dependencies.
type PostgreSQLBatch struct {
	statements []PostgreSQLBatchStatement
}

type sqlDataset interface {
	ToSQL() (string, []interface{}, error)
}

// SupportsPostgreSQLBatch reports whether pgx batch execution is available.
//
// The function inspects the database handle's driver and returns true only for
// the pgx stdlib driver.
//
// Parameters:
//   - db: Database handle to inspect.
//
// Returns:
//   - bool: True when pgx batch execution is supported.
func SupportsPostgreSQLBatch(db *sql.DB) bool {
	if db == nil {
		return false
	}
	_, ok := db.Driver().(*stdlib.Driver)
	return ok
}

// AppendDataset appends a rendered SQL dataset to the batch.
//
// The dataset is rendered immediately and stored in append order.
//
// Parameters:
//   - dataset: SQL dataset to render and append.
//
// Returns:
//   - error: Error when dataset rendering fails.
func (b *PostgreSQLBatch) AppendDataset(dataset sqlDataset) error {
	query, args, err := dataset.ToSQL()
	if err != nil {
		return err
	}
	b.statements = append(b.statements, PostgreSQLBatchStatement{SQL: query, Args: args})
	return nil
}

// Statements returns the statements collected in append order.
//
// Returns:
//   - []PostgreSQLBatchStatement: Rendered statements in dependency order.
func (b *PostgreSQLBatch) Statements() []PostgreSQLBatchStatement {
	return b.statements
}

// ExecutePostgreSQLBatchInTransaction executes rendered statements in a transaction.
//
// Statements must already be rendered to SQL and must not contain unresolved
// Goqu arguments.
//
// Parameters:
//   - ctx: Request context used for statement execution.
//   - tx: Transaction used to execute the statements.
//   - statements: Rendered PostgreSQL statements to execute in order.
//
// Returns:
//   - error: Error when validation fails or PostgreSQL execution fails.
func ExecutePostgreSQLBatchInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	statements []PostgreSQLBatchStatement,
) error {
	if tx == nil {
		return NewErrBadRequest("COMMON-PGBATCH-NILTX transaction must not be nil")
	}
	if len(statements) == 0 {
		return NewErrBadRequest("COMMON-PGBATCH-EMPTY no statements supplied")
	}

	queries := make([]string, 0, len(statements))
	for _, statement := range statements {
		if len(statement.Args) != 0 {
			return NewInternalServerError("COMMON-PGBATCH-PARAMS collected statement contains unresolved parameters")
		}
		queries = append(queries, statement.SQL)
	}
	if _, err := tx.ExecContext(ctx, strings.Join(queries, ";\n")); err != nil {
		return internalServerErrorWithCause("COMMON-PGBATCH-EXEC", err)
	}
	return nil
}

// PostgreSQLCurrentSequenceValue returns a Goqu expression for a sequence value.
//
// The expression resolves the current value of the table column sequence in the
// current PostgreSQL session.
//
// Parameters:
//   - table: Table that owns the sequence.
//   - column: Column that owns the sequence.
//
// Returns:
//   - exp.LiteralExpression: Goqu expression for currval(pg_get_serial_sequence(...)).
func PostgreSQLCurrentSequenceValue(table string, column string) exp.LiteralExpression {
	return goqu.L("currval(pg_get_serial_sequence(?, ?))", table, column)
}

// ExecutePostgreSQLBatchTransaction executes statements as one pgx batch.
//
// The function opens a dedicated connection, starts a pgx transaction, sends all
// statements as one batch, consumes results, and commits on success.
//
// Parameters:
//   - ctx: Request context used for connection and batch execution.
//   - db: Database handle used to obtain the pgx connection.
//   - statements: Rendered PostgreSQL statements to queue in order.
//   - readResults: Optional callback that consumes queued batch results.
//
// Returns:
//   - error: Error when connection setup, execution, result reading, or commit fails.
func ExecutePostgreSQLBatchTransaction(
	ctx context.Context,
	db *sql.DB,
	statements []PostgreSQLBatchStatement,
	readResults func(pgx.BatchResults) error,
) error {
	if db == nil {
		return NewErrBadRequest("COMMON-PGBATCH-NILDB database handle must not be nil")
	}
	if len(statements) == 0 {
		return NewErrBadRequest("COMMON-PGBATCH-EMPTY no statements supplied")
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return NewInternalServerError("COMMON-PGBATCH-GETCONN " + err.Error())
	}
	defer func() {
		_ = conn.Close()
	}()

	return conn.Raw(func(driverConn any) error {
		pgxConn, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("COMMON-PGBATCH-UNSUPPORTEDDRIVER expected pgx stdlib connection")
		}

		pgxTx, beginErr := pgxConn.Conn().Begin(ctx)
		if beginErr != nil {
			return internalServerErrorWithCause("COMMON-PGBATCH-BEGIN", beginErr)
		}
		committed := false
		defer func() {
			if !committed {
				_ = pgxTx.Rollback(ctx)
			}
		}()

		batch := &pgx.Batch{}
		for _, statement := range statements {
			batch.Queue(statement.SQL, statement.Args...)
		}

		results := pgxTx.SendBatch(ctx, batch)
		if readResults != nil {
			if readErr := readResults(results); readErr != nil {
				_ = results.Close()
				return readErr
			}
		} else if execErr := executePostgreSQLBatchResults(results, len(statements)); execErr != nil {
			return execErr
		}
		if closeErr := results.Close(); closeErr != nil {
			return internalServerErrorWithCause("COMMON-PGBATCH-CLOSE", closeErr)
		}
		if commitErr := pgxTx.Commit(ctx); commitErr != nil {
			return internalServerErrorWithCause("COMMON-PGBATCH-COMMIT", commitErr)
		}
		committed = true
		return nil
	})
}

func executePostgreSQLBatchResults(results pgx.BatchResults, count int) error {
	for index := 0; index < count; index++ {
		if _, execErr := results.Exec(); execErr != nil {
			_ = results.Close()
			return internalServerErrorWithCause("COMMON-PGBATCH-EXEC", execErr)
		}
	}
	return nil
}

func internalServerErrorWithCause(code string, err error) error {
	return fmt.Errorf("%s: %w", NewInternalServerError(code), err)
}
