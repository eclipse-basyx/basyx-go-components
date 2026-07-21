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
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
* IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
* CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
* TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
* SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package binarycontent

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"sync"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// StagedLargeObject owns a transaction-scoped PostgreSQL large object.
// Its transaction remains open until Promote commits it or Close rolls it back.
type StagedLargeObject struct {
	ctx        context.Context
	tx         *sql.Tx
	oid        int64
	size       int64
	descriptor int
	reader     *largeObjectReader
	closed     bool
	mu         sync.Mutex
}

// NewStager binds a database pool for request upload staging.
//
// Parameters:
//   - db: PostgreSQL connection pool used to create transaction-scoped large objects.
//
// Returns:
//   - common.UploadStager: Reusable staging function bound to db.
//
// Example:
//
//	stager := binarycontent.NewStager(db)
//	upload, err := stager(ctx, requestBody, 128<<20)
func NewStager(db *sql.DB) common.UploadStager {
	return func(ctx context.Context, input io.Reader, maxSizeBytes int64) (common.StagedUpload, error) {
		return Stage(ctx, db, input, maxSizeBytes)
	}
}

// Stage streams input into a transaction-scoped large object.
//
// Parameters:
//   - ctx: Request context used for database calls and cancellation.
//   - db: PostgreSQL connection pool.
//   - input: Source payload.
//   - maxSizeBytes: Maximum accepted payload size.
//
// Returns:
//   - common.StagedUpload: Caller-owned seekable object; Close it unless promoted.
//   - error: Coded validation, payload-limit, or database error.
func Stage(ctx context.Context, db *sql.DB, input io.Reader, maxSizeBytes int64) (common.StagedUpload, error) {
	if db == nil {
		return nil, common.NewInternalServerError("BINARYCONTENT-STAGE-NILDB database handle is required")
	}
	if input == nil {
		return nil, common.NewErrBadRequest("BINARYCONTENT-STAGE-NILREADER upload payload is required")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, common.NewInternalServerError("BINARYCONTENT-STAGE-STARTTX " + err.Error())
	}
	oid, _, size, err := writeTransientLargeObjectTx(ctx, tx, input, maxSizeBytes)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return &StagedLargeObject{ctx: ctx, tx: tx, oid: oid, size: size, descriptor: -1}, nil
}

// Size returns the number of staged bytes.
//
// Returns:
//   - int64: Staged payload size, or zero for a nil receiver.
func (stage *StagedLargeObject) Size() int64 {
	if stage == nil {
		return 0
	}
	return stage.size
}

// Read implements io.Reader for the staged large object.
//
// Parameters:
//   - destination: Buffer receiving staged bytes.
//
// Returns:
//   - int: Number of bytes read.
//   - error: Read, context, transaction, or EOF result.
func (stage *StagedLargeObject) Read(destination []byte) (int, error) {
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if err := stage.ensureReaderLocked(); err != nil {
		return 0, err
	}
	return stage.reader.Read(destination)
}

// Seek implements io.Seeker for the staged large object.
//
// Parameters:
//   - offset: Offset interpreted relative to whence.
//   - whence: One of io.SeekStart, io.SeekCurrent, or io.SeekEnd.
//
// Returns:
//   - int64: New absolute read position.
//   - error: Seek, context, or transaction error.
func (stage *StagedLargeObject) Seek(offset int64, whence int) (int64, error) {
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if err := stage.ensureReaderLocked(); err != nil {
		return 0, err
	}
	return stage.reader.Seek(offset, whence)
}

// Promote atomically commits the staged object after persist associates its OID.
//
// Parameters:
//   - ctx: Request context used for final persistence and commit.
//   - persist: Callback receiving the staging transaction, large-object OID, and byte size.
//
// Returns:
//   - error: Callback or commit error. On error, the caller must still call Close.
func (stage *StagedLargeObject) Promote(ctx context.Context, persist func(context.Context, *sql.Tx, int64, int64) error) error {
	if stage == nil || persist == nil {
		return common.NewInternalServerError("BINARYCONTENT-PROMOTE-INVALID stage and persistence callback are required")
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.closed {
		return common.NewInternalServerError("BINARYCONTENT-PROMOTE-CLOSED staged upload is closed")
	}
	if err := stage.closeReaderLocked(ctx); err != nil {
		return err
	}
	if err := persist(ctx, stage.tx, stage.oid, stage.size); err != nil {
		return err
	}
	if err := stage.tx.Commit(); err != nil {
		return common.NewInternalServerError("BINARYCONTENT-PROMOTE-COMMIT " + err.Error())
	}
	stage.closed = true
	return nil
}

// Close discards an unpromoted staged object by rolling back its transaction.
// It is idempotent and leaves successfully promoted objects intact.
//
// Returns:
//   - error: Large-object descriptor or transaction cleanup error.
func (stage *StagedLargeObject) Close() error {
	if stage == nil {
		return nil
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.closed {
		return nil
	}
	closeErr := stage.closeReaderLocked(stage.ctx)
	rollbackErr := stage.tx.Rollback()
	stage.closed = true
	if closeErr != nil {
		return closeErr
	}
	if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
		return common.NewInternalServerError("BINARYCONTENT-STAGE-ROLLBACK " + rollbackErr.Error())
	}
	return nil
}

func (stage *StagedLargeObject) ensureReaderLocked() error {
	if stage == nil || stage.closed {
		return common.NewInternalServerError("BINARYCONTENT-STAGE-CLOSED staged upload is closed")
	}
	if stage.reader != nil {
		return nil
	}
	query, args, err := goqu.Select(goqu.Func("lo_open", stage.oid, largeObjectReadMode)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("BINARYCONTENT-STAGE-BUILDOPEN " + err.Error())
	}
	if err = stage.tx.QueryRowContext(stage.ctx, query, args...).Scan(&stage.descriptor); err != nil {
		return common.NewInternalServerError("BINARYCONTENT-STAGE-OPEN " + err.Error())
	}
	stage.reader = &largeObjectReader{ctx: stage.ctx, tx: stage.tx, descriptor: stage.descriptor}
	return nil
}

func (stage *StagedLargeObject) closeReaderLocked(ctx context.Context) error {
	if stage.reader == nil {
		return nil
	}
	err := closeLargeObjectTx(ctx, stage.tx, stage.descriptor)
	stage.reader = nil
	stage.descriptor = -1
	return err
}

type transactionLargeObjectReader struct {
	ctx        context.Context
	tx         *sql.Tx
	descriptor int
	reader     *largeObjectReader
	closed     bool
	mu         sync.Mutex
}

// OpenOID opens a PostgreSQL large object for bounded-memory response streaming.
//
// Parameters:
//   - ctx: Request context used for reads and cancellation.
//   - db: PostgreSQL connection pool.
//   - oid: PostgreSQL large-object identifier.
//
// Returns:
//   - io.ReadCloser: Reader owning its database transaction; callers must close it.
//   - error: Coded transaction or large-object open error.
func OpenOID(ctx context.Context, db *sql.DB, oid int64) (io.ReadCloser, error) {
	if db == nil {
		return nil, common.NewInternalServerError("BINARYCONTENT-OPENOID-NILDB database handle is required")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, common.NewInternalServerError("BINARYCONTENT-OPENOID-STARTTX " + err.Error())
	}
	reader, err := OpenOIDTx(ctx, tx, oid)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return reader, nil
}

// OpenOIDTx opens a large object and transfers transaction ownership to the reader.
//
// Parameters:
//   - ctx: Request context used for reads and cancellation.
//   - tx: Existing transaction whose ownership transfers to the returned reader.
//   - oid: PostgreSQL large-object identifier.
//
// Returns:
//   - io.ReadCloser: Reader that closes the descriptor and rolls back tx on Close.
//   - error: Coded validation or large-object open error; tx remains caller-owned on failure.
func OpenOIDTx(ctx context.Context, tx *sql.Tx, oid int64) (io.ReadCloser, error) {
	if tx == nil {
		return nil, common.NewInternalServerError("BINARYCONTENT-OPENOID-NILTX transaction is required")
	}
	query, args, err := goqu.Select(goqu.Func("lo_open", oid, largeObjectReadMode)).ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("BINARYCONTENT-OPENOID-BUILDOPEN " + err.Error())
	}
	var descriptor int
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&descriptor); err != nil {
		return nil, common.NewInternalServerError("BINARYCONTENT-OPENOID-OPEN " + err.Error())
	}
	return &transactionLargeObjectReader{
		ctx: ctx, tx: tx, descriptor: descriptor,
		reader: &largeObjectReader{ctx: ctx, tx: tx, descriptor: descriptor},
	}, nil
}

func (reader *transactionLargeObjectReader) Read(destination []byte) (int, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if reader.closed {
		return 0, io.ErrClosedPipe
	}
	return reader.reader.Read(destination)
}

func (reader *transactionLargeObjectReader) Close() error {
	if reader == nil {
		return nil
	}
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if reader.closed {
		return nil
	}
	closeErr := closeLargeObjectTx(reader.ctx, reader.tx, reader.descriptor)
	rollbackErr := reader.tx.Rollback()
	reader.closed = true
	if closeErr != nil {
		return closeErr
	}
	if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
		return common.NewInternalServerError("BINARYCONTENT-OPENOID-ROLLBACK " + rollbackErr.Error())
	}
	return nil
}

// UnlinkOIDTx deletes a PostgreSQL large object in the caller's transaction.
//
// Parameters:
//   - ctx: Request context used for the delete operation.
//   - tx: Transaction that owns the delete operation.
//   - oid: PostgreSQL large-object identifier to unlink.
//
// Returns:
//   - error: Coded query-build or execution error.
func UnlinkOIDTx(ctx context.Context, tx *sql.Tx, oid int64) error {
	return unlinkLargeObjectTx(ctx, tx, oid)
}
