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

package companylookuppostgresql

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestCompanyLookupReadPoolSelection(t *testing.T) {
	writer, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = writer.Close() }()
	reader, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()

	backend, err := NewPostgreSQLCompanyLookupBackendFromPools(writer, reader, false)
	require.NoError(t, err)
	require.Same(t, reader, backend.readDB(t.Context()))
	require.Same(t, writer, backend.readDB(common.WithWriterPostgresReads(t.Context())))
}
