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

package asyncjob

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
)

const asyncJobTable = "async_job"

type postgresStore struct {
	db      *sql.DB
	dialect goqu.DialectWrapper
}

func newPostgresStore(db *sql.DB) *postgresStore {
	return &postgresStore{db: db, dialect: goqu.Dialect("postgres")}
}

func (s *postgresStore) Create(ctx context.Context, handleID string, record Record) error {
	metadata, err := json.Marshal(record.Metadata)
	if err != nil {
		return fmt.Errorf("ASYNCJOB-PGCREATE-MARSHALMETADATA %w", err)
	}
	query, args, err := s.dialect.Insert(asyncJobTable).Rows(goqu.Record{
		"handle_id":          handleID,
		"manager_key":        record.ManagerKey,
		"job_kind":           record.JobKind,
		"execution_state":    record.ExecutionState,
		"owner_key":          record.OwnerKey,
		"metadata":           string(metadata),
		"worker_id":          record.WorkerID,
		"lease_expires_at":   nullableTime(record.LeaseExpiresAt),
		"execution_deadline": nullableTime(record.ExecutionDeadline),
		"created_at":         record.CreatedAt,
		"updated_at":         record.CreatedAt,
	}).ToSQL()
	if err != nil {
		return fmt.Errorf("ASYNCJOB-PGCREATE-BUILDQUERY %w", err)
	}
	if _, err = s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("ASYNCJOB-PGCREATE-EXECQUERY %w", err)
	}
	return nil
}

func (s *postgresStore) Get(ctx context.Context, handleID string, managerKey string, ownerKey string) (Record, bool, error) {
	conditions := goqu.Ex{"handle_id": handleID, "manager_key": managerKey}
	if ownerKey != "" {
		conditions["owner_key"] = ownerKey
	}
	query, args, err := s.dialect.From(asyncJobTable).
		Select(
			"manager_key", "job_kind", "execution_state", "owner_key", "metadata",
			"bulk_result", "result_payload", "error_status", "error_payload", "worker_id",
			"created_at", "terminal_at", "expires_at", "lease_expires_at", "execution_deadline",
		).
		Where(conditions).
		ToSQL()
	if err != nil {
		return Record{}, false, fmt.Errorf("ASYNCJOB-PGGET-BUILDQUERY %w", err)
	}

	var record Record
	var metadata, bulkResult, payload, errorBody []byte
	var terminalAt, expiresAt, leaseExpiresAt, executionDeadline sql.NullTime
	err = s.db.QueryRowContext(ctx, query, args...).Scan(
		&record.ManagerKey, &record.JobKind, &record.ExecutionState, &record.OwnerKey, &metadata,
		&bulkResult, &payload, &record.ErrorStatus, &errorBody, &record.WorkerID,
		&record.CreatedAt, &terminalAt, &expiresAt, &leaseExpiresAt, &executionDeadline,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Record{}, false, nil
		}
		return Record{}, false, fmt.Errorf("ASYNCJOB-PGGET-EXECQUERY %w", err)
	}
	if err := decodeRecordJSON(&record, metadata, bulkResult, payload, errorBody); err != nil {
		return Record{}, false, err
	}
	record.TerminalAt = nullTimeValue(terminalAt)
	record.ExpiresAt = nullTimeValue(expiresAt)
	record.LeaseExpiresAt = nullTimeValue(leaseExpiresAt)
	record.ExecutionDeadline = nullTimeValue(executionDeadline)
	return record, true, nil
}

func (s *postgresStore) Transition(
	ctx context.Context,
	handleID string,
	managerKey string,
	workerID string,
	terminal Record,
	expiresAt time.Time,
) (bool, error) {
	bulkResult, err := marshalOptional(terminal.Result, terminal.HasResult)
	if err != nil {
		return false, fmt.Errorf("ASYNCJOB-PGTRANS-MARSHALRESULT %w", err)
	}
	payload, err := marshalOptional(terminal.Payload, terminal.Payload != nil)
	if err != nil {
		return false, fmt.Errorf("ASYNCJOB-PGTRANS-MARSHALPAYLOAD %w", err)
	}
	errorBody, err := marshalOptional(terminal.ErrorBody, terminal.ErrorBody != nil)
	if err != nil {
		return false, fmt.Errorf("ASYNCJOB-PGTRANS-MARSHALERROR %w", err)
	}
	now := time.Now().UTC()
	query, args, err := s.dialect.Update(asyncJobTable).
		Set(goqu.Record{
			"execution_state":  terminal.ExecutionState,
			"bulk_result":      bulkResult,
			"result_payload":   payload,
			"error_status":     terminal.ErrorStatus,
			"error_payload":    errorBody,
			"terminal_at":      now,
			"expires_at":       expiresAt,
			"lease_expires_at": nil,
			"updated_at":       now,
		}).
		Where(goqu.Ex{
			"handle_id":       handleID,
			"manager_key":     managerKey,
			"worker_id":       workerID,
			"execution_state": executionStateRunning,
		}).
		ToSQL()
	if err != nil {
		return false, fmt.Errorf("ASYNCJOB-PGTRANS-BUILDQUERY %w", err)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("ASYNCJOB-PGTRANS-EXECQUERY %w", err)
	}
	return affected(result)
}

func (s *postgresStore) RenewLease(
	ctx context.Context,
	handleID string,
	managerKey string,
	workerID string,
	expiresAt time.Time,
) (bool, error) {
	query, args, err := s.dialect.Update(asyncJobTable).
		Set(goqu.Record{"lease_expires_at": expiresAt, "updated_at": time.Now().UTC()}).
		Where(goqu.Ex{
			"handle_id":       handleID,
			"manager_key":     managerKey,
			"worker_id":       workerID,
			"execution_state": executionStateRunning,
		}).
		ToSQL()
	if err != nil {
		return false, fmt.Errorf("ASYNCJOB-PGRENEW-BUILDQUERY %w", err)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("ASYNCJOB-PGRENEW-EXECQUERY %w", err)
	}
	return affected(result)
}

func (s *postgresStore) Delete(ctx context.Context, handleID string, managerKey string, ownerKey string) error {
	conditions := goqu.Ex{"handle_id": handleID, "manager_key": managerKey}
	if ownerKey != "" {
		conditions["owner_key"] = ownerKey
	}
	query, args, err := s.dialect.Delete(asyncJobTable).
		Where(conditions).
		ToSQL()
	if err != nil {
		return fmt.Errorf("ASYNCJOB-PGDELETE-BUILDQUERY %w", err)
	}
	if _, err = s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("ASYNCJOB-PGDELETE-EXECQUERY %w", err)
	}
	return nil
}

func (s *postgresStore) RecoverAbandoned(
	ctx context.Context,
	managerKey string,
	handleID string,
	ownerKey string,
	now time.Time,
	expiresAt time.Time,
) (int64, error) {
	errorBody, err := json.Marshal(abandonedErrorBody())
	if err != nil {
		return 0, fmt.Errorf("ASYNCJOB-PGRECOVER-MARSHALERROR %w", err)
	}
	recoveryConditions := []goqu.Expression{
		goqu.C("manager_key").Eq(managerKey),
		goqu.C("execution_state").Eq(executionStateRunning),
		goqu.C("lease_expires_at").Lte(now),
	}
	if handleID != "" {
		recoveryConditions = append(recoveryConditions, goqu.C("handle_id").Eq(handleID))
	}
	if ownerKey != "" {
		recoveryConditions = append(recoveryConditions, goqu.C("owner_key").Eq(ownerKey))
	}
	query, args, err := s.dialect.Update(asyncJobTable).
		Set(goqu.Record{
			"execution_state":  executionStateFailed,
			"error_status":     500,
			"error_payload":    string(errorBody),
			"terminal_at":      now,
			"expires_at":       expiresAt,
			"lease_expires_at": nil,
			"updated_at":       now,
		}).
		Where(recoveryConditions...).
		ToSQL()
	if err != nil {
		return 0, fmt.Errorf("ASYNCJOB-PGRECOVER-BUILDQUERY %w", err)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("ASYNCJOB-PGRECOVER-EXECQUERY %w", err)
	}
	return rowsAffected(result)
}

func (s *postgresStore) DeleteExpired(
	ctx context.Context,
	managerKey string,
	now time.Time,
) (int64, error) {
	query, args, err := s.dialect.Delete(asyncJobTable).
		Where(
			goqu.C("manager_key").Eq(managerKey),
			goqu.C("execution_state").Neq(executionStateRunning),
			goqu.C("expires_at").Lte(now),
		).
		ToSQL()
	if err != nil {
		return 0, fmt.Errorf("ASYNCJOB-PGCLEANUP-BUILDQUERY %w", err)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("ASYNCJOB-PGCLEANUP-EXECQUERY %w", err)
	}
	return rowsAffected(result)
}

func decodeRecordJSON(record *Record, metadata, bulkResult, payload, errorBody []byte) error {
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &record.Metadata); err != nil {
			return fmt.Errorf("ASYNCJOB-PGGET-DECODEMETADATA %w", err)
		}
	}
	if len(bulkResult) > 0 {
		if err := json.Unmarshal(bulkResult, &record.Result); err != nil {
			return fmt.Errorf("ASYNCJOB-PGGET-DECODERESULT %w", err)
		}
		record.HasResult = true
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &record.Payload); err != nil {
			return fmt.Errorf("ASYNCJOB-PGGET-DECODEPAYLOAD %w", err)
		}
	}
	if len(errorBody) > 0 {
		if err := json.Unmarshal(errorBody, &record.ErrorBody); err != nil {
			return fmt.Errorf("ASYNCJOB-PGGET-DECODEERROR %w", err)
		}
	}
	return nil
}

func marshalOptional(value any, present bool) (any, error) {
	if !present {
		return nil, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(payload), nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullTimeValue(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func affected(result sql.Result) (bool, error) {
	count, err := rowsAffected(result)
	return count > 0, err
}

func rowsAffected(result sql.Result) (int64, error) {
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("ASYNCJOB-PG-ROWSAFFECTED %w", err)
	}
	return count, nil
}
