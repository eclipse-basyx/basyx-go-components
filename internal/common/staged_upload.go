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

package common

import (
	"context"
	"database/sql"
	"fmt"
	"io"
)

// StagedUpload is seekable request content held outside process memory.
// Callers must call Close unless Promote succeeds. Promote transfers the staged
// object into persistent ownership by executing its callback in the staging transaction.
type StagedUpload interface {
	io.ReadSeeker
	// Size returns the number of staged bytes.
	Size() int64
	// Promote runs persist in the staging transaction and commits on success.
	Promote(context.Context, func(context.Context, *sql.Tx, int64, int64) error) error
	// Close discards an unpromoted upload and is safe to call after promotion.
	Close() error
}

// UploadStager copies one request stream into bounded seekable storage.
//
// Parameters:
//   - context.Context: Request context used for cancellation and authorization propagation.
//   - io.Reader: Source payload to stage.
//   - int64: Maximum accepted source size in bytes.
//
// Returns:
//   - StagedUpload: Seekable staged content owned by the caller.
//   - error: A coded staging or payload-limit error.
type UploadStager func(context.Context, io.Reader, int64) (StagedUpload, error)

type payloadLimitReader struct {
	reader    io.Reader
	remaining uint64
	maximum   uint64
	label     string
}

// NewPayloadLimitReader wraps a stream with a typed payload-size limit.
//
// Parameters:
//   - reader: Source stream.
//   - maximum: Maximum bytes that may be returned.
//   - label: Human-readable payload label included in limit errors.
//
// Returns:
//   - io.Reader: Reader that returns a 413-classified error when source data exceeds maximum.
func NewPayloadLimitReader(reader io.Reader, maximum uint64, label string) io.Reader {
	return &payloadLimitReader{reader: reader, remaining: maximum, maximum: maximum, label: label}
}

func (reader *payloadLimitReader) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	if reader.remaining > 0 {
		if uint64(len(destination)) > reader.remaining {
			destination = destination[:reader.remaining]
		}
		read, err := reader.reader.Read(destination)
		if read < 0 {
			return 0, NewInternalServerError("COMMON-LIMITREADER-NEGATIVEREAD source returned a negative byte count")
		}
		reader.remaining -= uint64(read)
		return read, err
	}
	var probe [1]byte
	read, err := reader.reader.Read(probe[:])
	if read > 0 {
		return 0, NewErrPayloadTooLarge(fmt.Sprintf("COMMON-LIMITREADER-TOOLARGE %s exceeds configured maximum of %d bytes", reader.label, reader.maximum))
	}
	return 0, err
}
