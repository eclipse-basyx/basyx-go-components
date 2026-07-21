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
	"bytes"
	"context"
	"database/sql"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type memoryStagedUpload struct {
	*bytes.Reader
	closed bool
}

func (upload *memoryStagedUpload) Size() int64 { return int64(upload.Len()) }

func (upload *memoryStagedUpload) Promote(ctx context.Context, persist func(context.Context, *sql.Tx, int64, int64) error) error {
	return persist(ctx, nil, 0, upload.Size())
}

func (upload *memoryStagedUpload) Close() error {
	upload.closed = true
	return nil
}

func newMemoryStager(_ context.Context, input io.Reader, _ int64) (StagedUpload, error) {
	content, err := io.ReadAll(input)
	if err != nil {
		return nil, err
	}
	return &memoryStagedUpload{Reader: bytes.NewReader(content)}, nil
}

func TestConnectionReservedUploadStagerReleasesAdmissionOnClose(t *testing.T) {
	stager := NewConnectionReservedUploadStager(newMemoryStager, 2, 1)
	first, err := stager(t.Context(), bytes.NewReader([]byte("first")), 10)
	require.NoError(t, err)

	waitingContext, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = stager(waitingContext, bytes.NewReader([]byte("second")), 10)
	require.Error(t, err)
	require.True(t, IsErrServiceUnavailable(err))

	require.NoError(t, first.Close())
	second, err := stager(t.Context(), bytes.NewReader([]byte("second")), 10)
	require.NoError(t, err)
	require.NoError(t, second.Close())
}

func TestConnectionReservedUploadStagerReleasesAdmissionOnPromotion(t *testing.T) {
	stager := NewConnectionReservedUploadStager(newMemoryStager, 2, 1)
	first, err := stager(t.Context(), bytes.NewReader([]byte("first")), 10)
	require.NoError(t, err)
	require.NoError(t, first.Promote(t.Context(), func(context.Context, *sql.Tx, int64, int64) error { return nil }))

	second, err := stager(t.Context(), bytes.NewReader([]byte("second")), 10)
	require.NoError(t, err)
	require.NoError(t, second.Close())
}

func TestConnectionReservedUploadStagerRejectsPoolWithoutSpareConnection(t *testing.T) {
	stager := NewConnectionReservedUploadStager(newMemoryStager, 1, 1)
	_, err := stager(t.Context(), bytes.NewReader([]byte("content")), 10)
	require.Error(t, err)
	require.True(t, IsErrServiceUnavailable(err))
}

func TestConnectionReservedUploadStagerRejectsMissingStager(t *testing.T) {
	stager := NewConnectionReservedUploadStager(nil, 10, 1)
	_, err := stager(t.Context(), bytes.NewReader([]byte("content")), 10)
	require.Error(t, err)
	require.True(t, IsInternalServerError(err))
}
