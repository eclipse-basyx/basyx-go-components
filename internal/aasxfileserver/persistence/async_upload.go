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

package persistence

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
)

const asyncUploadTable = "aasx_async_upload"

// AsyncUploadStore transfers request-scoped staged uploads into durable async ownership.
type AsyncUploadStore struct {
	db *sql.DB
}

// NewAsyncUploadStore creates durable staging backed by PostgreSQL Large Objects.
func NewAsyncUploadStore(db *sql.DB) (*AsyncUploadStore, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("AASXFS-ASYNCSTAGE-NILDB database handle must not be nil")
	}
	return &AsyncUploadStore{db: db}, nil
}

// Accept atomically creates the persistent handle and its staging ownership.
func (s *AsyncUploadStore) Accept(
	ctx context.Context,
	manager *asyncjob.Manager,
	upload common.StagedUpload,
	ownerKey string,
	options asyncjob.StartOptions,
	successPayload any,
) (string, common.StagedUpload, error) {
	if manager == nil || upload == nil {
		return "", nil, common.NewInternalServerError("AASXFS-ASYNCSTAGE-INVALID manager and upload are required")
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
			return common.NewInternalServerError("AASXFS-ASYNCSTAGE-BUILDINSERT " + buildErr.Error())
		}
		if _, execErr := tx.ExecContext(ctx, query, args...); execErr != nil {
			return common.NewInternalServerError("AASXFS-ASYNCSTAGE-INSERT " + execErr.Error())
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}

	return handleID, &durableAsyncUpload{
		ctx: ctx, db: s.db, manager: manager,
		handleID: handleID, oid: oid, size: size, successPayload: successPayload,
	}, nil
}

type durableAsyncUpload struct {
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

func (upload *durableAsyncUpload) Size() int64 {
	if upload == nil {
		return 0
	}
	return upload.size
}

func (upload *durableAsyncUpload) Read(destination []byte) (int, error) {
	upload.mu.Lock()
	defer upload.mu.Unlock()
	if err := upload.ensureReaderLocked(); err != nil {
		return 0, err
	}
	return upload.reader.Read(destination)
}

func (upload *durableAsyncUpload) Seek(offset int64, whence int) (int64, error) {
	upload.mu.Lock()
	defer upload.mu.Unlock()
	if err := upload.ensureReaderLocked(); err != nil {
		return 0, err
	}
	return upload.reader.Seek(offset, whence)
}

func (upload *durableAsyncUpload) Promote(ctx context.Context, persist func(context.Context, *sql.Tx, int64, int64) error) error {
	if upload == nil || persist == nil {
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-PROMOTEINVALID upload and callback are required")
	}
	upload.mu.Lock()
	defer upload.mu.Unlock()
	if upload.closed {
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-PROMOTECLOSED staged upload is closed")
	}
	if err := upload.closeReaderLocked(); err != nil {
		return err
	}

	tx, err := upload.db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-PROMOTESTARTTX " + err.Error())
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
	if err = tx.Commit(); err != nil {
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-PROMOTECOMMIT " + err.Error())
	}
	committed = true
	upload.promoted = true
	upload.closed = true
	return nil
}

func (upload *durableAsyncUpload) Close() error {
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

	ctx, cancel := context.WithTimeout(context.WithoutCancel(upload.ctx), 5*time.Second)
	defer cancel()
	tx, err := upload.db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-CLOSESTARTTX " + err.Error())
	}
	query, args, err := goqu.Dialect("postgres").Delete(asyncUploadTable).
		Where(goqu.C("handle_id").Eq(upload.handleID)).ToSQL()
	if err != nil {
		_ = tx.Rollback()
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-CLOSEBUILDDELETE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		_ = tx.Rollback()
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-CLOSEDELETE " + err.Error())
	}
	if err = tx.Commit(); err != nil {
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-CLOSECOMMIT " + err.Error())
	}
	upload.closed = true
	return nil
}

func (upload *durableAsyncUpload) ensureReaderLocked() error {
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

func (upload *durableAsyncUpload) closeReaderLocked() error {
	if upload.reader == nil {
		return nil
	}
	err := upload.reader.Close()
	upload.reader = nil
	return err
}

func (upload *durableAsyncUpload) lockOwnership(ctx context.Context, tx *sql.Tx) error {
	query, args, err := goqu.Dialect("postgres").From(asyncUploadTable).
		Select("file_oid", "size_bytes").
		Where(goqu.C("handle_id").Eq(upload.handleID)).
		ForUpdate(exp.Wait).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-BUILDLOCK " + err.Error())
	}
	var oid int64
	var size int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&oid, &size); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return common.NewErrNotFound("AASXFS-ASYNCSTAGE-NOTFOUND durable staging ownership no longer exists")
		}
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-LOCK " + err.Error())
	}
	if oid != upload.oid || size != upload.size {
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-MISMATCH durable staging metadata changed unexpectedly")
	}
	return nil
}

func (upload *durableAsyncUpload) releaseOwnership(ctx context.Context, tx *sql.Tx) error {
	dialect := goqu.Dialect("postgres")
	updateQuery, updateArgs, err := dialect.Update(asyncUploadTable).
		Set(goqu.Record{"promoted": true}).
		Where(goqu.C("handle_id").Eq(upload.handleID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-BUILDTRANSFER " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, updateQuery, updateArgs...); err != nil {
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-TRANSFER " + err.Error())
	}
	deleteQuery, deleteArgs, err := dialect.Delete(asyncUploadTable).
		Where(goqu.C("handle_id").Eq(upload.handleID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-BUILDRELEASE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, deleteQuery, deleteArgs...); err != nil {
		return common.NewInternalServerError("AASXFS-ASYNCSTAGE-RELEASE " + err.Error())
	}
	return nil
}
