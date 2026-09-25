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

// Package audit persists a shared tamper-evident audit stream.
package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/google/uuid"
)

const deliveryLockID int64 = 894221037

var dialect = goqu.Dialect("postgres")

// Repository stores audit records and optionally enqueues them for WORM delivery.
type Repository struct {
	DB           *sql.DB
	WORM         bool
	BacklogLimit int
}

// Event contains normalized audit facts. Payload never carries credentials.
type Event struct {
	ID            string
	OccurredAt    time.Time
	Actor         string
	Resource      string
	Outcome       string
	CorrelationID string
	Payload       Payload
}

// Payload contains structured, non-secret audit metadata.
type Payload struct {
	Action  string            `json:"action"`
	Details map[string]string `json:"details,omitempty"`
}

// Receipt identifies the immutable audit row appended to a stream.
type Receipt struct {
	EventID     string
	Sequence    int64
	ContentHash string
}

// Record is an audit row returned to an administrative consumer.
type Record struct {
	Receipt
	Stream        string
	OccurredAt    time.Time
	Actor         string
	Resource      string
	Outcome       string
	CorrelationID string
	Payload       Payload
	PreviousHash  string
}

// Verification reports the state of a stream hash chain.
type Verification struct {
	Valid          bool
	Records        int64
	FailedSequence int64
}

// Append atomically appends event to stream using the caller's transaction.
func (repository Repository) Append(ctx context.Context, tx *sql.Tx, stream string, event Event) (Receipt, error) {
	if tx == nil {
		return Receipt{}, fmt.Errorf("AUDIT-APPEND-TX transaction is required")
	}
	if err := validateEvent(stream, event); err != nil {
		return Receipt{}, err
	}
	if repository.WORM {
		if err := repository.ensureDeliveryCapacity(ctx, tx); err != nil {
			return Receipt{}, err
		}
	}
	lastSequence, lastHash, err := lockStream(ctx, tx, stream)
	if err != nil {
		return Receipt{}, err
	}
	normalized := normalizeEvent(event)
	contentHash, payload, err := hashEvent(lastHash, lastSequence+1, normalized)
	if err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{EventID: normalized.ID, Sequence: lastSequence + 1, ContentHash: contentHash}
	if err := insertRecord(ctx, tx, stream, normalized, payload, lastHash, receipt); err != nil {
		return Receipt{}, err
	}
	if err := updateStream(ctx, tx, stream, receipt); err != nil {
		return Receipt{}, err
	}
	if repository.WORM {
		if err := enqueueDelivery(ctx, tx, normalized.ID); err != nil {
			return Receipt{}, err
		}
	}
	return receipt, nil
}

// Record appends an audit event in an independent short transaction.
func (repository Repository) Record(ctx context.Context, stream string, event Event) (Receipt, error) {
	if repository.DB == nil {
		return Receipt{}, fmt.Errorf("AUDIT-RECORD-DB database is required")
	}
	tx, err := repository.DB.BeginTx(ctx, nil)
	if err != nil {
		return Receipt{}, fmt.Errorf("AUDIT-RECORD-BEGIN: %w", err)
	}
	receipt, err := repository.Append(ctx, tx, stream, event)
	if err != nil {
		_ = tx.Rollback()
		return Receipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return Receipt{}, fmt.Errorf("AUDIT-RECORD-COMMIT: %w", err)
	}
	return receipt, nil
}

func (repository Repository) ensureDeliveryCapacity(ctx context.Context, tx *sql.Tx) error {
	if repository.BacklogLimit < 1 {
		return fmt.Errorf("AUDIT-DELIVERY-LIMIT WORM delivery backlog limit must be positive")
	}
	lockQuery, lockArgs, err := dialect.Select(goqu.L("pg_advisory_xact_lock(?)", deliveryLockID)).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("AUDIT-DELIVERY-LOCKSQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, lockQuery, lockArgs...); err != nil {
		return fmt.Errorf("AUDIT-DELIVERY-LOCK: %w", err)
	}
	query, args, err := dialect.From("audit_delivery").Select(goqu.COUNT("*")).Where(goqu.C("archived_at").IsNull()).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("AUDIT-DELIVERY-COUNSQL: %w", err)
	}
	var pending int
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&pending); err != nil {
		return fmt.Errorf("AUDIT-DELIVERY-COUNT: %w", err)
	}
	if pending >= repository.BacklogLimit {
		return fmt.Errorf("AUDIT-DELIVERY-LIMIT WORM delivery backlog limit reached")
	}
	return nil
}

func lockStream(ctx context.Context, tx *sql.Tx, stream string) (int64, string, error) {
	insert, args, err := dialect.Insert("audit_stream").Rows(goqu.Record{"stream": stream}).OnConflict(goqu.DoNothing()).Prepared(true).ToSQL()
	if err != nil {
		return 0, "", fmt.Errorf("AUDIT-APPEND-STREAMSQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, insert, args...); err != nil {
		return 0, "", fmt.Errorf("AUDIT-APPEND-STREAM: %w", err)
	}
	query, args, err := dialect.From("audit_stream").Select("last_sequence", "last_hash").Where(goqu.C("stream").Eq(stream)).ForUpdate(goqu.Wait).Prepared(true).ToSQL()
	if err != nil {
		return 0, "", fmt.Errorf("AUDIT-APPEND-LOCKSQL: %w", err)
	}
	var sequence int64
	var hash string
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&sequence, &hash); err != nil {
		return 0, "", fmt.Errorf("AUDIT-APPEND-LOCK: %w", err)
	}
	return sequence, hash, nil
}

func insertRecord(ctx context.Context, tx *sql.Tx, stream string, event Event, payload []byte, previousHash string, receipt Receipt) error {
	query, args, err := dialect.Insert("audit_record").Rows(goqu.Record{
		"event_id": receipt.EventID, "stream": stream, "sequence": receipt.Sequence, "occurred_at": event.OccurredAt,
		"actor": event.Actor, "resource": event.Resource, "outcome": event.Outcome, "correlation_id": event.CorrelationID,
		"event": goqu.L("?::jsonb", string(payload)), "previous_hash": previousHash, "content_hash": receipt.ContentHash,
	}).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("AUDIT-APPEND-RECORDSQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("AUDIT-APPEND-RECORD: %w", err)
	}
	return nil
}

func updateStream(ctx context.Context, tx *sql.Tx, stream string, receipt Receipt) error {
	query, args, err := dialect.Update("audit_stream").Set(goqu.Record{"last_sequence": receipt.Sequence, "last_hash": receipt.ContentHash}).Where(goqu.C("stream").Eq(stream)).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("AUDIT-APPEND-UPDATESQL: %w", err)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("AUDIT-APPEND-UPDATE: %w", err)
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return fmt.Errorf("AUDIT-APPEND-UPDATECOUNT expected one stream row")
	}
	return nil
}

func enqueueDelivery(ctx context.Context, tx *sql.Tx, eventID string) error {
	query, args, err := dialect.Insert("audit_delivery").Rows(goqu.Record{"event_id": eventID}).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("AUDIT-DELIVERY-ENQUEUESQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("AUDIT-DELIVERY-ENQUEUE: %w", err)
	}
	return nil
}

func validateEvent(stream string, event Event) error {
	if strings.TrimSpace(stream) == "" || strings.TrimSpace(event.Actor) == "" || strings.TrimSpace(event.Resource) == "" || strings.TrimSpace(event.Outcome) == "" || strings.TrimSpace(event.CorrelationID) == "" || strings.TrimSpace(event.Payload.Action) == "" {
		return fmt.Errorf("AUDIT-APPEND-VALIDATE required audit fields must not be blank")
	}
	if event.ID != "" {
		if _, err := uuid.Parse(event.ID); err != nil {
			return fmt.Errorf("AUDIT-APPEND-EVENTID event ID must be a UUID")
		}
	}
	return nil
}

func normalizeEvent(event Event) Event {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	} else {
		event.OccurredAt = event.OccurredAt.UTC()
	}
	event.Payload.Details = safeDetails(event.Payload.Details)
	return event
}

func safeDetails(details map[string]string) map[string]string {
	if len(details) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(details))
	for key, value := range details {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") {
			continue
		}
		normalized[key] = value
	}
	return normalized
}

func hashEvent(previousHash string, sequence int64, event Event) (string, []byte, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return "", nil, fmt.Errorf("AUDIT-APPEND-JSON: %w", err)
	}
	input, err := json.Marshal(struct {
		PreviousHash string          `json:"previousHash"`
		Sequence     int64           `json:"sequence"`
		Event        json.RawMessage `json:"event"`
	}{previousHash, sequence, payload})
	if err != nil {
		return "", nil, fmt.Errorf("AUDIT-APPEND-HASHJSON: %w", err)
	}
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:]), payload, nil
}
