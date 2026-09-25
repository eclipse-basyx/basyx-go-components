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

package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
)

// SearchFilter constrains one stable audit page.
type SearchFilter struct {
	Stream, Actor, Resource, Outcome, CorrelationID string
	AfterSequence                                   int64
	Limit                                           uint
	From, Until                                     time.Time
}

// List returns one ordered administrative page after afterSequence.
func (repository Repository) List(ctx context.Context, stream string, afterSequence int64, limit uint) ([]Record, error) {
	return repository.Search(ctx, SearchFilter{Stream: stream, AfterSequence: afterSequence, Limit: limit})
}

// Search returns ordered audit records matching all supplied filters.
func (repository Repository) Search(ctx context.Context, filter SearchFilter) ([]Record, error) {
	stream, afterSequence, limit := filter.Stream, filter.AfterSequence, filter.Limit
	if repository.DB == nil {
		return nil, fmt.Errorf("AUDIT-LIST-DB database is required")
	}
	if strings.TrimSpace(stream) == "" || limit == 0 {
		return nil, fmt.Errorf("AUDIT-LIST-INPUT stream and positive limit are required")
	}
	query, args, err := dialect.From("audit_record").Select("event_id", "sequence", "occurred_at", "actor", "resource", "outcome", "correlation_id", "event", "previous_hash", "content_hash").
		Where(searchConditions(filter, stream, afterSequence)...).Order(goqu.C("sequence").Asc()).Limit(limit).Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("AUDIT-LIST-SQL: %w", err)
	}
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("AUDIT-LIST-QUERY: %w", err)
	}
	defer func() { _ = rows.Close() }()
	records := make([]Record, 0)
	for rows.Next() {
		var record Record
		var payload []byte
		if err = rows.Scan(&record.EventID, &record.Sequence, &record.OccurredAt, &record.Actor, &record.Resource, &record.Outcome, &record.CorrelationID, &payload, &record.PreviousHash, &record.ContentHash); err != nil {
			return nil, fmt.Errorf("AUDIT-LIST-SCAN: %w", err)
		}
		if err = json.Unmarshal(payload, &record); err != nil {
			return nil, fmt.Errorf("AUDIT-LIST-DECODE: %w", err)
		}
		record.Stream = stream
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("AUDIT-LIST-ROWS: %w", err)
	}
	return records, nil
}

// Verify recalculates a stream's hash chain and reports the first bad sequence.
func (repository Repository) Verify(ctx context.Context, stream string) (Verification, error) {
	if repository.DB == nil {
		return Verification{}, fmt.Errorf("AUDIT-VERIFY-DB database is required")
	}
	if strings.TrimSpace(stream) == "" {
		return Verification{}, fmt.Errorf("AUDIT-VERIFY-STREAM stream is required")
	}
	tx, err := repository.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return Verification{}, fmt.Errorf("AUDIT-VERIFY-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	verification, previousHash, err := verifyStreamRows(ctx, tx, stream)
	if err != nil {
		return Verification{}, err
	}
	if verification.Valid {
		if err = verifyStreamState(ctx, tx, stream, verification.Records, previousHash, &verification); err != nil {
			return Verification{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Verification{}, fmt.Errorf("AUDIT-VERIFY-COMMIT: %w", err)
	}
	return verification, nil
}

func verifyStreamRows(ctx context.Context, tx *sql.Tx, stream string) (Verification, string, error) {
	query, args, err := dialect.From("audit_record").Select("sequence", "event", "previous_hash", "content_hash").Where(goqu.C("stream").Eq(stream)).Order(goqu.C("sequence").Asc()).Prepared(true).ToSQL()
	if err != nil {
		return Verification{}, "", fmt.Errorf("AUDIT-VERIFY-SQL: %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return Verification{}, "", fmt.Errorf("AUDIT-VERIFY-QUERY: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return verifyRows(rows)
}

func verifyRows(rows *sql.Rows) (Verification, string, error) {
	verification := Verification{Valid: true}
	previousHash := ""
	expectedSequence := int64(1)
	for rows.Next() {
		var sequence int64
		var payload []byte
		var storedPrevious, storedHash string
		if err := rows.Scan(&sequence, &payload, &storedPrevious, &storedHash); err != nil {
			return Verification{}, "", fmt.Errorf("AUDIT-VERIFY-SCAN: %w", err)
		}
		var event Event
		if err := json.Unmarshal(payload, &event); err != nil {
			return Verification{}, "", fmt.Errorf("AUDIT-VERIFY-DECODE: %w", err)
		}
		hash, _, err := hashEvent(previousHash, sequence, event)
		if err != nil {
			return Verification{}, "", err
		}
		verification.Records++
		if sequence != expectedSequence || storedPrevious != previousHash || storedHash != hash {
			verification.Valid = false
			verification.FailedSequence = sequence
			return verification, previousHash, nil
		}
		previousHash = storedHash
		expectedSequence++
	}
	if err := rows.Err(); err != nil {
		return Verification{}, "", fmt.Errorf("AUDIT-VERIFY-ROWS: %w", err)
	}
	return verification, previousHash, nil
}

func verifyStreamState(ctx context.Context, db *sql.Tx, stream string, records int64, previousHash string, verification *Verification) error {
	streamQuery, streamArgs, err := dialect.From("audit_stream").Select("last_sequence", "last_hash").Where(goqu.C("stream").Eq(stream)).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("AUDIT-VERIFY-STREAMSQL: %w", err)
	}
	var lastSequence int64
	var lastHash string
	if err = db.QueryRowContext(ctx, streamQuery, streamArgs...).Scan(&lastSequence, &lastHash); err != nil {
		return fmt.Errorf("AUDIT-VERIFY-STREAM: %w", err)
	}
	if lastSequence != records || lastHash != previousHash {
		verification.Valid = false
		verification.FailedSequence = lastSequence
	}
	return nil
}

func searchConditions(filter SearchFilter, stream string, afterSequence int64) []goqu.Expression {
	conditions := []goqu.Expression{goqu.C("stream").Eq(stream), goqu.C("sequence").Gt(afterSequence)}
	for column, value := range map[string]string{"actor": filter.Actor, "resource": filter.Resource, "outcome": filter.Outcome, "correlation_id": filter.CorrelationID} {
		if value != "" {
			conditions = append(conditions, goqu.C(column).Eq(value))
		}
	}
	if !filter.From.IsZero() {
		conditions = append(conditions, goqu.C("occurred_at").Gte(filter.From))
	}
	if !filter.Until.IsZero() {
		conditions = append(conditions, goqu.C("occurred_at").Lte(filter.Until))
	}
	return conditions
}
