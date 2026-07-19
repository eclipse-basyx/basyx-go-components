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

package binarycontent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"unicode"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const (
	TableContent            = "binary_content"
	TableFileReference      = "file_binary_reference"
	TableThumbnailReference = "thumbnail_binary_reference"
	managedPathPrefix       = "/aasx/files"
	largeObjectReadMode     = 0x00040000
	largeObjectWriteMode    = 0x00020000
)

// Content identifies one canonical PostgreSQL large object.
type Content struct {
	ID        int64
	SHA256    string
	SizeBytes int64
	OID       int64
}

// Reference identifies a logical owner association with canonical content.
type Reference struct {
	OwnerID      int64
	Content      Content
	PathToken    string
	SafeFileName string
}

// ManagedPath returns the stable AAS/AASX value for a logical reference.
func (reference Reference) ManagedPath() string {
	return path.Join(managedPathPrefix, reference.PathToken, url.PathEscape(reference.SafeFileName))
}

// NewReference creates a fresh high-entropy path for one successful upload.
func NewReference(ownerID int64, content Content, fileName string) (Reference, error) {
	safeFileName, err := SafeFileName(fileName)
	if err != nil {
		return Reference{}, err
	}
	randomBytes := make([]byte, 24)
	if _, err = rand.Read(randomBytes); err != nil {
		return Reference{}, common.NewInternalServerError("BINARYCONTENT-PATH-RANDOM " + err.Error())
	}
	return Reference{
		OwnerID: ownerID, Content: content,
		PathToken: base64.RawURLEncoding.EncodeToString(randomBytes), SafeFileName: safeFileName,
	}, nil
}

// SafeFileName validates and normalizes a user-supplied upload filename.
func SafeFileName(fileName string) (string, error) {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "", common.NewErrBadRequest("BINARYCONTENT-FILENAME-EMPTY upload filename is required")
	}
	decoded, err := url.PathUnescape(fileName)
	if err != nil {
		return "", common.NewErrBadRequest("BINARYCONTENT-FILENAME-ESCAPE filename contains invalid escaping")
	}
	if decoded != fileName || strings.ContainsAny(fileName, `/\\`) || path.Base(fileName) != fileName || fileName == "." || fileName == ".." {
		return "", common.NewErrBadRequest("BINARYCONTENT-FILENAME-PATH filename must contain one safe path segment")
	}
	for _, character := range fileName {
		if unicode.IsControl(character) || character == 0 {
			return "", common.NewErrBadRequest("BINARYCONTENT-FILENAME-CONTROL filename contains control characters")
		}
	}
	return fileName, nil
}

// StoreTx streams an upload into PostgreSQL and reuses a canonical large object
// when its SHA-256 and byte length already exist.
func StoreTx(ctx context.Context, tx *sql.Tx, reader io.Reader) (Content, error) {
	if tx == nil {
		return Content{}, common.NewInternalServerError("BINARYCONTENT-STORE-NILTX transaction must not be nil")
	}
	if reader == nil {
		return Content{}, common.NewErrBadRequest("BINARYCONTENT-STORE-NILREADER file payload is required")
	}
	oid, digest, size, err := writeTransientLargeObjectTx(ctx, tx, reader)
	if err != nil {
		return Content{}, err
	}
	if err = lockDigestTx(ctx, tx, digest, size); err != nil {
		return Content{}, err
	}
	existing, err := findContentTx(ctx, tx, digest, size)
	if err == nil {
		if unlinkErr := unlinkLargeObjectTx(ctx, tx, oid); unlinkErr != nil {
			return Content{}, unlinkErr
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Content{}, err
	}
	query, args, err := goqu.Insert(TableContent).Rows(goqu.Record{
		"sha256": digest, "size_bytes": size, "file_oid": oid,
	}).Returning("id").ToSQL()
	if err != nil {
		return Content{}, common.NewInternalServerError("BINARYCONTENT-STORE-BUILDINSERT " + err.Error())
	}
	var contentID int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&contentID); err != nil {
		return Content{}, common.NewInternalServerError("BINARYCONTENT-STORE-INSERT " + err.Error())
	}
	return Content{ID: contentID, SHA256: digest, SizeBytes: size, OID: oid}, nil
}

func writeTransientLargeObjectTx(ctx context.Context, tx *sql.Tx, reader io.Reader) (int64, string, int64, error) {
	query, args, err := goqu.Select(goqu.Func("lo_create", 0)).ToSQL()
	if err != nil {
		return 0, "", 0, common.NewInternalServerError("BINARYCONTENT-STORE-BUILDCREATELO " + err.Error())
	}
	var oid int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&oid); err != nil {
		return 0, "", 0, common.NewInternalServerError("BINARYCONTENT-STORE-CREATELO " + err.Error())
	}
	openQuery, openArgs, err := goqu.Select(goqu.Func("lo_open", oid, largeObjectWriteMode)).ToSQL()
	if err != nil {
		return 0, "", 0, common.NewInternalServerError("BINARYCONTENT-STORE-BUILDOPENLO " + err.Error())
	}
	var descriptor int
	if err = tx.QueryRowContext(ctx, openQuery, openArgs...).Scan(&descriptor); err != nil {
		return 0, "", 0, common.NewInternalServerError("BINARYCONTENT-STORE-OPENLO " + err.Error())
	}
	hash := sha256.New()
	buffer := make([]byte, 32*1024)
	var size int64
	for {
		count, readErr := reader.Read(buffer)
		if count > 0 {
			chunk := buffer[:count]
			if _, err = hash.Write(chunk); err != nil {
				return 0, "", 0, common.NewInternalServerError("BINARYCONTENT-STORE-HASH " + err.Error())
			}
			writeQuery, writeArgs, buildErr := goqu.Select(goqu.Func("lowrite", descriptor, chunk)).ToSQL()
			if buildErr != nil {
				return 0, "", 0, common.NewInternalServerError("BINARYCONTENT-STORE-BUILDWRITELO " + buildErr.Error())
			}
			if _, err = tx.ExecContext(ctx, writeQuery, writeArgs...); err != nil {
				return 0, "", 0, common.NewInternalServerError("BINARYCONTENT-STORE-WRITELO " + err.Error())
			}
			size += int64(count)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return 0, "", 0, common.NewInternalServerError("BINARYCONTENT-STORE-READ " + readErr.Error())
		}
		if err = ctx.Err(); err != nil {
			return 0, "", 0, common.NewInternalServerError("BINARYCONTENT-STORE-CONTEXT " + err.Error())
		}
	}
	if err = closeLargeObjectTx(ctx, tx, descriptor); err != nil {
		return 0, "", 0, err
	}
	return oid, hex.EncodeToString(hash.Sum(nil)), size, nil
}

func lockDigestTx(ctx context.Context, tx *sql.Tx, digest string, size int64) error {
	lockKey := fmt.Sprintf("binary-content:%s:%d", digest, size)
	query, args, err := goqu.Select(goqu.Func("pg_advisory_xact_lock", goqu.Func("hashtextextended", lockKey, int64(0)))).Prepared(true).ToSQL()
	if err != nil {
		return common.NewInternalServerError("BINARYCONTENT-LOCK-BUILD " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("BINARYCONTENT-LOCK-EXEC " + err.Error())
	}
	return nil
}

func findContentTx(ctx context.Context, tx *sql.Tx, digest string, size int64) (Content, error) {
	query, args, err := goqu.From(TableContent).
		Select("id", "sha256", "size_bytes", "file_oid").
		Where(goqu.Ex{"sha256": digest, "size_bytes": size}).ToSQL()
	if err != nil {
		return Content{}, common.NewInternalServerError("BINARYCONTENT-FIND-BUILD " + err.Error())
	}
	var content Content
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&content.ID, &content.SHA256, &content.SizeBytes, &content.OID); err != nil {
		return Content{}, err
	}
	return content, nil
}

// UpsertReferenceTx replaces one logical reference and cleans its prior
// canonical payload when it no longer has any owners.
func UpsertReferenceTx(ctx context.Context, tx *sql.Tx, table string, ownerColumn string, reference Reference) error {
	if !validReferenceTable(table, ownerColumn) {
		return common.NewInternalServerError("BINARYCONTENT-REFERENCE-TABLE unsupported binary reference table")
	}
	previousContentID, err := referenceContentIDTx(ctx, tx, table, ownerColumn, reference.OwnerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	record := goqu.Record{
		ownerColumn: reference.OwnerID, "binary_content_id": reference.Content.ID,
		"path_token": reference.PathToken, "safe_file_name": reference.SafeFileName,
	}
	query, args, err := goqu.Insert(table).Rows(record).
		OnConflict(goqu.DoUpdate(ownerColumn, goqu.Record{
			"binary_content_id": reference.Content.ID, "path_token": reference.PathToken,
			"safe_file_name": reference.SafeFileName, "db_updated_at": goqu.L("NOW()"),
		})).ToSQL()
	if err != nil {
		return common.NewInternalServerError("BINARYCONTENT-REFERENCE-BUILDUPSERT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("BINARYCONTENT-REFERENCE-UPSERT " + err.Error())
	}
	if previousContentID > 0 && previousContentID != reference.Content.ID {
		return CleanupUnreferencedTx(ctx, tx, previousContentID)
	}
	return nil
}

// LoadReferenceTx resolves canonical content from its owning model element.
func LoadReferenceTx(ctx context.Context, tx *sql.Tx, table string, ownerColumn string, ownerID int64) (Reference, error) {
	if !validReferenceTable(table, ownerColumn) {
		return Reference{}, common.NewInternalServerError("BINARYCONTENT-REFERENCE-TABLE unsupported binary reference table")
	}
	referenceTable := goqu.T(table).As("reference")
	contentTable := goqu.T(TableContent).As("content")
	query, args, err := goqu.From(referenceTable).
		Join(contentTable, goqu.On(referenceTable.Col("binary_content_id").Eq(contentTable.Col("id")))).
		Select(referenceTable.Col(ownerColumn), referenceTable.Col("path_token"), referenceTable.Col("safe_file_name"),
			contentTable.Col("id"), contentTable.Col("sha256"), contentTable.Col("size_bytes"), contentTable.Col("file_oid")).
		Where(referenceTable.Col(ownerColumn).Eq(ownerID)).ToSQL()
	if err != nil {
		return Reference{}, common.NewInternalServerError("BINARYCONTENT-REFERENCE-BUILDLOAD " + err.Error())
	}
	var reference Reference
	if err = tx.QueryRowContext(ctx, query, args...).Scan(
		&reference.OwnerID, &reference.PathToken, &reference.SafeFileName,
		&reference.Content.ID, &reference.Content.SHA256, &reference.Content.SizeBytes, &reference.Content.OID,
	); err != nil {
		return Reference{}, err
	}
	return reference, nil
}

// DeleteReferenceTx removes a logical owner and deletes the canonical large
// object only when neither attachments nor thumbnails still reference it.
func DeleteReferenceTx(ctx context.Context, tx *sql.Tx, table string, ownerColumn string, ownerID int64) error {
	if !validReferenceTable(table, ownerColumn) {
		return common.NewInternalServerError("BINARYCONTENT-REFERENCE-TABLE unsupported binary reference table")
	}
	contentID, err := referenceContentIDTx(ctx, tx, table, ownerColumn, ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	query, args, err := goqu.Delete(table).Where(goqu.C(ownerColumn).Eq(ownerID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("BINARYCONTENT-REFERENCE-BUILDDELETE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("BINARYCONTENT-REFERENCE-DELETE " + err.Error())
	}
	return CleanupUnreferencedTx(ctx, tx, contentID)
}

func referenceContentIDTx(ctx context.Context, tx *sql.Tx, table string, ownerColumn string, ownerID int64) (int64, error) {
	query, args, err := goqu.From(table).Select("binary_content_id").Where(goqu.C(ownerColumn).Eq(ownerID)).ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("BINARYCONTENT-REFERENCE-BUILDCONTENTID " + err.Error())
	}
	var contentID int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&contentID); err != nil {
		return 0, err
	}
	return contentID, nil
}

// CleanupUnreferencedTx removes an unreferenced canonical row and large object.
func CleanupUnreferencedTx(ctx context.Context, tx *sql.Tx, contentID int64) error {
	fileCount, err := referenceCountTx(ctx, tx, TableFileReference, contentID)
	if err != nil {
		return err
	}
	thumbnailCount, err := referenceCountTx(ctx, tx, TableThumbnailReference, contentID)
	if err != nil {
		return err
	}
	if fileCount+thumbnailCount > 0 {
		return nil
	}
	query, args, err := goqu.Delete(TableContent).Where(goqu.C("id").Eq(contentID)).Returning("file_oid").ToSQL()
	if err != nil {
		return common.NewInternalServerError("BINARYCONTENT-CLEANUP-BUILDDELETE " + err.Error())
	}
	var oid int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&oid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return common.NewInternalServerError("BINARYCONTENT-CLEANUP-DELETE " + err.Error())
	}
	return unlinkLargeObjectTx(ctx, tx, oid)
}

func referenceCountTx(ctx context.Context, tx *sql.Tx, table string, contentID int64) (int64, error) {
	query, args, err := goqu.From(table).Select(goqu.COUNT("*")).Where(goqu.C("binary_content_id").Eq(contentID)).ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("BINARYCONTENT-CLEANUP-BUILDCOUNT " + err.Error())
	}
	var count int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, common.NewInternalServerError("BINARYCONTENT-CLEANUP-COUNT " + err.Error())
	}
	return count, nil
}

// ReadAllTx reads canonical content for the existing byte-oriented repository contracts.
func ReadAllTx(ctx context.Context, tx *sql.Tx, content Content) ([]byte, error) {
	return ReadOIDTx(ctx, tx, content.OID)
}

// ReadOIDTx reads a legacy or canonical PostgreSQL large object.
func ReadOIDTx(ctx context.Context, tx *sql.Tx, oid int64) ([]byte, error) {
	var result []byte
	err := streamOIDTx(ctx, tx, oid, func(reader io.Reader) error {
		var readErr error
		result, readErr = io.ReadAll(reader)
		return readErr
	})
	return result, err
}

// StreamTx opens canonical content and supplies a bounded-memory reader to the
// callback while the caller's transaction remains open.
func StreamTx(ctx context.Context, tx *sql.Tx, content Content, consume func(io.Reader) error) error {
	return streamOIDTx(ctx, tx, content.OID, consume)
}

func streamOIDTx(ctx context.Context, tx *sql.Tx, oid int64, consume func(io.Reader) error) error {
	if consume == nil {
		return common.NewInternalServerError("BINARYCONTENT-STREAM-NILCONSUMER stream consumer is required")
	}
	query, args, err := goqu.Select(goqu.Func("lo_open", oid, largeObjectReadMode)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("BINARYCONTENT-READ-BUILDOPEN " + err.Error())
	}
	var descriptor int
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&descriptor); err != nil {
		return common.NewInternalServerError("BINARYCONTENT-READ-OPEN " + err.Error())
	}
	reader := &largeObjectReader{ctx: ctx, tx: tx, descriptor: descriptor}
	consumeErr := consume(reader)
	closeErr := closeLargeObjectTx(ctx, tx, descriptor)
	if consumeErr != nil {
		return consumeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

type largeObjectReader struct {
	ctx        context.Context
	tx         *sql.Tx
	descriptor int
	pending    []byte
	done       bool
}

func (reader *largeObjectReader) Read(destination []byte) (int, error) {
	if len(reader.pending) == 0 && !reader.done {
		query, args, err := goqu.Select(goqu.Func("loread", reader.descriptor, 32*1024)).ToSQL()
		if err != nil {
			return 0, common.NewInternalServerError("BINARYCONTENT-READ-BUILDCHUNK " + err.Error())
		}
		if err = reader.tx.QueryRowContext(reader.ctx, query, args...).Scan(&reader.pending); err != nil {
			return 0, common.NewInternalServerError("BINARYCONTENT-READ-CHUNK " + err.Error())
		}
		reader.done = len(reader.pending) == 0
	}
	if len(reader.pending) == 0 {
		return 0, io.EOF
	}
	count := copy(destination, reader.pending)
	reader.pending = reader.pending[count:]
	return count, nil
}

func closeLargeObjectTx(ctx context.Context, tx *sql.Tx, descriptor int) error {
	query, args, err := goqu.Select(goqu.Func("lo_close", descriptor)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("BINARYCONTENT-LO-BUILDCLOSE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("BINARYCONTENT-LO-CLOSE " + err.Error())
	}
	return nil
}

func unlinkLargeObjectTx(ctx context.Context, tx *sql.Tx, oid int64) error {
	query, args, err := goqu.Select(goqu.Func("lo_unlink", oid)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("BINARYCONTENT-LO-BUILDUNLINK " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("BINARYCONTENT-LO-UNLINK " + err.Error())
	}
	return nil
}

func validReferenceTable(table string, ownerColumn string) bool {
	return (table == TableFileReference && ownerColumn == "file_element_id") ||
		(table == TableThumbnailReference && ownerColumn == "thumbnail_element_id")
}
