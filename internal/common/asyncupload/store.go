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

// Package asyncupload transfers request-scoped staged uploads into the durable
// ownership of an asynchronous job.
package asyncupload

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"sync"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
)

const asyncUploadTable = "aasx_async_upload"

// Store transfers request-scoped staged uploads into durable async ownership.
type Store struct {
	db *sql.DB
}

// NewStore creates durable staging backed by PostgreSQL Large Objects.
func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("ASYNCUPLOAD-STAGE-NILDB database handle must not be nil")
	}
	return &Store{db: db}, nil
}

// Accept atomically creates the persistent handle and its staging ownership.
func (s *Store) Accept(
	ctx context.Context,
	manager *asyncjob.Manager,
	upload common.StagedUpload,
	ownerKey string,
	options asyncjob.StartOptions,
	successPayload any,
) (string, *DurableUpload, error) {
	if manager == nil || upload == nil {
		return "", nil, common.NewInternalServerError("ASYNCUPLOAD-STAGE-INVALID manager and upload are required")
	}

	var handleID string
	var oid int64
	var size int64
	err := upload.Promote(ctx, func(ctx context.Context, tx *sql.Tx, stagedOID int64, stagedSize int64) error {
		var startErr error
		handleID, startErr = manager.StartTx(ctx, tx, ownerKey, options)
		if startErr != nil {
			return startErr
		}
		oid = stagedOID
		size = stagedSize
		query, args, buildErr := goqu.Dialect("postgres").Insert(asyncUploadTable).Rows(goqu.Record{
			"handle_id":  handleID,
			"file_oid":   stagedOID,
			"size_bytes": stagedSize,
		}).ToSQL()
		if buildErr != nil {
			return common.NewInternalServerError("ASYNCUPLOAD-STAGE-BUILDINSERT " + buildErr.Error())
		}
		if _, execErr := tx.ExecContext(ctx, query, args...); execErr != nil {
			return common.NewInternalServerError("ASYNCUPLOAD-STAGE-INSERT " + execErr.Error())
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}

	return handleID, &DurableUpload{
		ctx: ctx, db: s.db, manager: manager,
		handleID: handleID, oid: oid, size: size, successPayload: successPayload,
	}, nil
}

// DurableUpload is a staged upload owned by a running asynchronous job. Its
// staging row and Large Object are removed when the job reaches a terminal state.
type DurableUpload struct {
	ctx            context.Context
	db             *sql.DB
	manager        *asyncjob.Manager
	handleID       string
	oid            int64
	size           int64
	successPayload any
	reader         binarycontent.ReadSeekCloser
	closed         bool
	promoted       bool
	mu             sync.Mutex
}

// Size returns the number of staged bytes.
func (upload *DurableUpload) Size() int64 {
	if upload == nil {
		return 0
	}
	return upload.size
}

// Read reads staged bytes through a reader that owns its own transaction.
func (upload *DurableUpload) Read(destination []byte) (int, error) {
	upload.mu.Lock()
	defer upload.mu.Unlock()
	if err := upload.ensureReaderLocked(); err != nil {
		return 0, err
	}
	return upload.reader.Read(destination)
}

// Seek moves the read position of the staged bytes.
func (upload *DurableUpload) Seek(offset int64, whence int) (int64, error) {
	upload.mu.Lock()
	defer upload.mu.Unlock()
	if err := upload.ensureReaderLocked(); err != nil {
		return 0, err
	}
	return upload.reader.Seek(offset, whence)
}

// Promote transfers the staged Large Object to persist and completes the job
// with the payload given to Accept in the same transaction.
func (upload *DurableUpload) Promote(ctx context.Context, persist func(context.Context, *sql.Tx, int64, int64) error) error {
	if upload == nil || persist == nil {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-PROMOTEINVALID upload and callback are required")
	}
	upload.mu.Lock()
	defer upload.mu.Unlock()
	if upload.closed {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-PROMOTECLOSED staged upload is closed")
	}
	if err := upload.closeReaderLocked(); err != nil {
		return err
	}

	tx, err := upload.db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-PROMOTESTARTTX " + err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err = upload.lockOwnership(ctx, tx); err != nil {
		return err
	}
	if err = persist(ctx, tx, upload.oid, upload.size); err != nil {
		return err
	}
	if err = upload.releaseOwnership(ctx, tx); err != nil {
		return err
	}
	if err = upload.manager.CompletePayloadTx(ctx, tx, upload.handleID, upload.successPayload); err != nil {
		return err
	}
	if err = common.CommitTransaction(tx); err != nil {
		return common.CommitError("ASYNCUPLOAD-STAGE-PROMOTECOMMIT", err)
	}
	committed = true
	upload.promoted = true
	upload.closed = true
	return nil
}

// Close discards the staging ownership unless it was promoted or consumed.
func (upload *DurableUpload) Close() error {
	if upload == nil {
		return nil
	}
	upload.mu.Lock()
	defer upload.mu.Unlock()
	if upload.closed {
		return nil
	}
	if err := upload.closeReaderLocked(); err != nil {
		return err
	}
	if upload.promoted {
		upload.closed = true
		return nil
	}

	ctx, cancel := asyncjob.NewPersistenceContext(upload.ctx)
	defer cancel()
	tx, err := upload.db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-CLOSESTARTTX " + err.Error())
	}
	query, args, err := goqu.Dialect("postgres").Delete(asyncUploadTable).
		Where(goqu.C("handle_id").Eq(upload.handleID)).ToSQL()
	if err != nil {
		_ = tx.Rollback()
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-CLOSEBUILDDELETE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		_ = tx.Rollback()
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-CLOSEDELETE " + err.Error())
	}
	if err = common.CommitTransaction(tx); err != nil {
		return common.CommitError("ASYNCUPLOAD-STAGE-CLOSECOMMIT", err)
	}
	upload.closed = true
	return nil
}

// Consume reads the staged bytes and completes the job in one transaction.
//
// consume receives that transaction and a seekable reader of the staged bytes,
// writes its results in the transaction, and returns the job's success payload.
// Completing the job removes the staging row and its Large Object in the same
// transaction, so either the results, the Completed job state and the staging
// removal are committed together, or none of them is.
//
// Parameters:
//   - ctx: Execution context bounding the transaction.
//   - consume: Callback writing the upload's results in the given transaction.
//
// Returns:
//   - error: The callback's error, or a staging, job transition, or commit error.
func (upload *DurableUpload) Consume(
	ctx context.Context,
	consume func(context.Context, *sql.Tx, io.ReadSeeker) (any, error),
) error {
	if upload == nil || consume == nil {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-CONSUMEINVALID upload and callback are required")
	}
	upload.mu.Lock()
	defer upload.mu.Unlock()
	if upload.closed {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-CONSUMECLOSED staged upload is closed")
	}
	if err := upload.closeReaderLocked(); err != nil {
		return err
	}

	tx, err := upload.db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-CONSUMESTARTTX " + err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	payload, err := upload.consumeInTx(ctx, tx, consume)
	if err != nil {
		return err
	}
	if err = upload.manager.CompletePayloadTx(ctx, tx, upload.handleID, payload); err != nil {
		return err
	}
	if err = common.CommitTransaction(tx); err != nil {
		return common.CommitError("ASYNCUPLOAD-STAGE-CONSUMECOMMIT", err)
	}
	committed = true
	upload.closed = true
	return nil
}

func (upload *DurableUpload) consumeInTx(
	ctx context.Context,
	tx *sql.Tx,
	consume func(context.Context, *sql.Tx, io.ReadSeeker) (any, error),
) (any, error) {
	if err := upload.lockOwnership(ctx, tx); err != nil {
		return nil, err
	}
	reader, err := binarycontent.OpenSeekableOIDInTransaction(ctx, tx, upload.oid)
	if err != nil {
		return nil, err
	}
	payload, consumeErr := consume(ctx, tx, reader)
	closeErr := reader.Close()
	if consumeErr != nil {
		return nil, consumeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return payload, nil
}

func (upload *DurableUpload) ensureReaderLocked() error {
	if upload.closed {
		return io.ErrClosedPipe
	}
	if upload.reader != nil {
		return nil
	}
	reader, err := binarycontent.OpenSeekableOID(upload.ctx, upload.db, upload.oid)
	if err != nil {
		return err
	}
	upload.reader = reader
	return nil
}

func (upload *DurableUpload) closeReaderLocked() error {
	if upload.reader == nil {
		return nil
	}
	err := upload.reader.Close()
	upload.reader = nil
	return err
}

func (upload *DurableUpload) lockOwnership(ctx context.Context, tx *sql.Tx) error {
	query, args, err := goqu.Dialect("postgres").From(asyncUploadTable).
		Select("file_oid", "size_bytes").
		Where(goqu.C("handle_id").Eq(upload.handleID)).
		ForUpdate(exp.Wait).ToSQL()
	if err != nil {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-BUILDLOCK " + err.Error())
	}
	var oid int64
	var size int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&oid, &size); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return common.NewErrNotFound("ASYNCUPLOAD-STAGE-NOTFOUND durable staging ownership no longer exists")
		}
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-LOCK " + err.Error())
	}
	if oid != upload.oid || size != upload.size {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-MISMATCH durable staging metadata changed unexpectedly")
	}
	return nil
}

func (upload *DurableUpload) releaseOwnership(ctx context.Context, tx *sql.Tx) error {
	dialect := goqu.Dialect("postgres")
	updateQuery, updateArgs, err := dialect.Update(asyncUploadTable).
		Set(goqu.Record{"promoted": true}).
		Where(goqu.C("handle_id").Eq(upload.handleID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-BUILDTRANSFER " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, updateQuery, updateArgs...); err != nil {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-TRANSFER " + err.Error())
	}
	deleteQuery, deleteArgs, err := dialect.Delete(asyncUploadTable).
		Where(goqu.C("handle_id").Eq(upload.handleID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-BUILDRELEASE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, deleteQuery, deleteArgs...); err != nil {
		return common.NewInternalServerError("ASYNCUPLOAD-STAGE-RELEASE " + err.Error())
	}
	return nil
}
