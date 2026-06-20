/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// PostgreSQLBatchStatement contains one SQL statement and its arguments.
type PostgreSQLBatchStatement struct {
	SQL  string
	Args []any
}

// PostgreSQLBatch stores statements that must be executed in order.
type PostgreSQLBatch struct {
	statements []PostgreSQLBatchStatement
}

type sqlDataset interface {
	ToSQL() (string, []interface{}, error)
}

// SupportsPostgreSQLBatch reports whether the database uses the pgx stdlib driver.
func SupportsPostgreSQLBatch(db *sql.DB) bool {
	if db == nil {
		return false
	}
	_, ok := db.Driver().(*stdlib.Driver)
	return ok
}

// AppendDataset builds and appends a Goqu dataset to the batch.
func (b *PostgreSQLBatch) AppendDataset(dataset sqlDataset) error {
	query, args, err := dataset.ToSQL()
	if err != nil {
		return err
	}
	b.statements = append(b.statements, PostgreSQLBatchStatement{SQL: query, Args: args})
	return nil
}

// Statements returns the statements collected in execution order.
func (b *PostgreSQLBatch) Statements() []PostgreSQLBatchStatement {
	return b.statements
}

// ExecutePostgreSQLBatchInTransaction executes rendered statements in an existing transaction.
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
		return NewInternalServerError("COMMON-PGBATCH-EXEC " + err.Error())
	}
	return nil
}

// PostgreSQLCurrentSequenceValue returns a Goqu expression for the current sequence value.
func PostgreSQLCurrentSequenceValue(table string, column string) exp.LiteralExpression {
	return goqu.L("currval(pg_get_serial_sequence(?, ?))", table, column)
}

// ExecutePostgreSQLBatchTransaction sends a complete pgx batch enclosed by BEGIN and COMMIT.
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

		batch := &pgx.Batch{}
		batch.Queue("BEGIN")
		for _, statement := range statements {
			batch.Queue(statement.SQL, statement.Args...)
		}
		batch.Queue("COMMIT")

		results := pgxConn.Conn().SendBatch(ctx, batch)
		if _, execErr := results.Exec(); execErr != nil {
			_ = results.Close()
			return NewInternalServerError("COMMON-PGBATCH-BEGIN " + execErr.Error())
		}
		if readResults != nil {
			if readErr := readResults(results); readErr != nil {
				_ = results.Close()
				return readErr
			}
		}
		if _, execErr := results.Exec(); execErr != nil {
			_ = results.Close()
			return NewInternalServerError("COMMON-PGBATCH-COMMIT " + execErr.Error())
		}
		if closeErr := results.Close(); closeErr != nil {
			return NewInternalServerError("COMMON-PGBATCH-CLOSE " + closeErr.Error())
		}
		return nil
	})
}
