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

// Package postgresstaging provides transaction-local, bounded PostgreSQL row
// staging for set-oriented persistence operations.
package postgresstaging

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const (
	tableName        = "basyx_mutation_stage"
	defaultBatchSize = 500
)

// Row is one endpoint-defined normalized record in a staged dataset.
type Row struct {
	MatchKey  string          `json:"match_key"`
	ParentKey *string         `json:"parent_key"`
	RowType   *int            `json:"row_type"`
	Ordinal   int64           `json:"ordinal"`
	Data      json.RawMessage `json:"row_data"`
}

// Stage scopes temporary rows to one logical operation inside a transaction.
type Stage struct {
	tx *sql.Tx
	id string
}

// Writer buffers a bounded number of rows before appending them to a dataset.
type Writer struct {
	stage     *Stage
	dataset   string
	batchSize int
	rows      []bufferedRow
}

type bufferedRow struct {
	Dataset string `json:"dataset"`
	Row
}

// Open prepares the session-local staging relation and allocates an operation ID.
func Open(ctx context.Context, tx *sql.Tx) (*Stage, error) {
	if tx == nil {
		return nil, common.NewErrBadRequest("COMMON-STAGE-OPEN-NILTX transaction must not be nil")
	}
	if err := validateContext(ctx, "COMMON-STAGE-OPEN-NILCTX"); err != nil {
		return nil, err
	}
	statement := stagingDDL()
	if _, err := tx.ExecContext(ctx, statement.Literal(), statement.Args()...); err != nil {
		return nil, common.NewInternalServerError("COMMON-STAGE-OPEN-DDL " + err.Error())
	}
	stageID, err := newStageID()
	if err != nil {
		return nil, common.NewInternalServerError("COMMON-STAGE-OPEN-ID " + err.Error())
	}
	return &Stage{tx: tx, id: stageID}, nil
}

func stagingDDL() exp.LiteralExpression {
	return goqu.L(`DO $basyx_stage$
		BEGIN
			CREATE TEMPORARY TABLE IF NOT EXISTS basyx_mutation_stage (
			stage_id TEXT NOT NULL,
			dataset TEXT NOT NULL,
			match_key TEXT NOT NULL,
			parent_key TEXT,
			row_type INTEGER,
			ordinal BIGINT NOT NULL,
			row_data JSONB NOT NULL,
			PRIMARY KEY (stage_id, dataset, match_key)
			) ON COMMIT DELETE ROWS;
			CREATE INDEX IF NOT EXISTS ix_basyx_mutation_stage_parent
				ON basyx_mutation_stage (stage_id, dataset, parent_key, ordinal);
			CREATE INDEX IF NOT EXISTS ix_basyx_mutation_stage_order
				ON basyx_mutation_stage (stage_id, dataset, ordinal);
		END
	$basyx_stage$`)
}

func newStageID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

// NewWriter creates a bounded row writer for one dataset.
func (s *Stage) NewWriter(dataset string) (*Writer, error) {
	if s == nil || s.tx == nil {
		return nil, common.NewErrBadRequest("COMMON-STAGE-WRITER-NILSTAGE stage must not be nil")
	}
	dataset = strings.TrimSpace(dataset)
	if dataset == "" {
		return nil, common.NewErrBadRequest("COMMON-STAGE-WRITER-DATASET dataset must not be empty")
	}
	return &Writer{
		stage:     s,
		dataset:   dataset,
		batchSize: defaultBatchSize,
		rows:      make([]bufferedRow, 0, defaultBatchSize),
	}, nil
}

// Add validates and buffers one row, flushing when the configured batch is full.
func (w *Writer) Add(ctx context.Context, row Row) error {
	if w == nil {
		return common.NewErrBadRequest("COMMON-STAGE-ADD-NILWRITER writer must not be nil")
	}
	return w.add(ctx, w.dataset, row)
}

// AddToDataset buffers a row for another dataset in the same database write batch.
func (w *Writer) AddToDataset(ctx context.Context, dataset string, row Row) error {
	return w.add(ctx, dataset, row)
}

func (w *Writer) add(ctx context.Context, dataset string, row Row) error {
	if w == nil || w.stage == nil {
		return common.NewErrBadRequest("COMMON-STAGE-ADD-NILWRITER writer must not be nil")
	}
	if err := validateContext(ctx, "COMMON-STAGE-ADD-NILCTX"); err != nil {
		return err
	}
	dataset = strings.TrimSpace(dataset)
	if dataset == "" {
		return common.NewErrBadRequest("COMMON-STAGE-ADD-DATASET dataset must not be empty")
	}
	if strings.TrimSpace(row.MatchKey) == "" {
		return common.NewErrBadRequest("COMMON-STAGE-ADD-KEY match key must not be empty")
	}
	if len(row.Data) == 0 || !json.Valid(row.Data) {
		return common.NewErrBadRequest("COMMON-STAGE-ADD-DATA row data must contain valid JSON")
	}
	w.rows = append(w.rows, bufferedRow{Dataset: dataset, Row: row})
	if len(w.rows) < w.batchSize {
		return nil
	}
	return w.Flush(ctx)
}

// Flush appends buffered rows to the temporary relation as one JSONB recordset.
func (w *Writer) Flush(ctx context.Context) error {
	if w == nil || w.stage == nil {
		return common.NewErrBadRequest("COMMON-STAGE-FLUSH-NILWRITER writer must not be nil")
	}
	if len(w.rows) == 0 {
		return nil
	}
	if err := validateContext(ctx, "COMMON-STAGE-FLUSH-NILCTX"); err != nil {
		return err
	}
	encoded, err := json.Marshal(w.rows)
	if err != nil {
		return common.NewInternalServerError("COMMON-STAGE-FLUSH-MARSHAL " + err.Error())
	}
	dialect := goqu.Dialect(common.Dialect)
	decoded := goqu.L(
		"jsonb_to_recordset(?::jsonb) AS decoded(dataset text, match_key text, parent_key text, row_type integer, ordinal bigint, row_data jsonb)",
		string(encoded),
	)
	source := dialect.From(decoded).Select(
		goqu.L("?", w.stage.id),
		goqu.I("decoded.dataset"),
		goqu.I("decoded.match_key"),
		goqu.I("decoded.parent_key"),
		goqu.I("decoded.row_type"),
		goqu.I("decoded.ordinal"),
		goqu.I("decoded.row_data"),
	)
	query, args, err := dialect.Insert(tableName).
		Cols("stage_id", "dataset", "match_key", "parent_key", "row_type", "ordinal", "row_data").
		FromQuery(source).
		Prepared(true).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("COMMON-STAGE-FLUSH-BUILD " + err.Error())
	}
	if _, err = w.stage.tx.ExecContext(ctx, query, args...); err != nil {
		if common.IsPostgresUniqueViolation(err) {
			return common.NewErrBadRequest("COMMON-STAGE-FLUSH-DUPLICATE duplicate match key in staged dataset")
		}
		return common.NewInternalServerError("COMMON-STAGE-FLUSH-EXEC " + err.Error())
	}
	w.rows = w.rows[:0]
	return nil
}

// Dataset returns a query selecting this operation's rows for dataset.
func (s *Stage) Dataset(dialect goqu.DialectWrapper, dataset string) *goqu.SelectDataset {
	return dialect.From(goqu.T(tableName)).
		Select("match_key", "parent_key", "row_type", "ordinal", "row_data").
		Where(goqu.Ex{"stage_id": s.id, "dataset": dataset})
}

// Materialize stores a query result as another dataset in this operation.
// The source must project match_key, parent_key, row_type, ordinal, and row_data.
func (s *Stage) Materialize(ctx context.Context, dataset string, source *goqu.SelectDataset) error {
	if s == nil || s.tx == nil {
		return common.NewErrBadRequest("COMMON-STAGE-MATERIALIZE-NILSTAGE stage must not be nil")
	}
	if err := validateContext(ctx, "COMMON-STAGE-MATERIALIZE-NILCTX"); err != nil {
		return err
	}
	dataset = strings.TrimSpace(dataset)
	if dataset == "" {
		return common.NewErrBadRequest("COMMON-STAGE-MATERIALIZE-DATASET dataset must not be empty")
	}
	if source == nil {
		return common.NewErrBadRequest("COMMON-STAGE-MATERIALIZE-NILSOURCE source query must not be nil")
	}

	dialect := goqu.Dialect(common.Dialect)
	rows := dialect.From(source.As("source")).Select(
		goqu.V(s.id),
		goqu.V(dataset),
		goqu.I("source.match_key"),
		goqu.I("source.parent_key"),
		goqu.I("source.row_type"),
		goqu.I("source.ordinal"),
		goqu.I("source.row_data"),
	)
	query, args, err := dialect.Insert(tableName).
		Cols("stage_id", "dataset", "match_key", "parent_key", "row_type", "ordinal", "row_data").
		FromQuery(rows).
		Prepared(true).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("COMMON-STAGE-MATERIALIZE-BUILD " + err.Error())
	}
	if _, err = s.tx.ExecContext(ctx, query, args...); err != nil {
		if common.IsPostgresUniqueViolation(err) {
			return common.NewErrBadRequest("COMMON-STAGE-MATERIALIZE-DUPLICATE duplicate match key in materialized dataset")
		}
		return common.NewInternalServerError("COMMON-STAGE-MATERIALIZE-EXEC " + err.Error())
	}
	return nil
}

// Cleanup removes only this operation's rows. ON COMMIT DELETE ROWS remains the
// final safety net for errors and caller-owned transaction lifetimes.
func (s *Stage) Cleanup(ctx context.Context) error {
	if s == nil || s.tx == nil {
		return common.NewErrBadRequest("COMMON-STAGE-CLEAN-NILSTAGE stage must not be nil")
	}
	if err := validateContext(ctx, "COMMON-STAGE-CLEAN-NILCTX"); err != nil {
		return err
	}
	query, args, err := goqu.Dialect(common.Dialect).Delete(tableName).
		Where(goqu.C("stage_id").Eq(s.id)).
		Prepared(true).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("COMMON-STAGE-CLEAN-BUILD " + err.Error())
	}
	if _, err = s.tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("COMMON-STAGE-CLEAN-EXEC " + err.Error())
	}
	return nil
}

func validateContext(ctx context.Context, errorCode string) error {
	if ctx == nil {
		return common.NewErrBadRequest(errorCode + " context must not be nil")
	}
	return ctx.Err()
}
