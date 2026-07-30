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

package aasregistrydatabase

import (
	"context"
	"fmt"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapInsertAASDescriptorErrorMapsUniqueViolationToConflict(t *testing.T) {
	err := mapInsertAASDescriptorError(fmt.Errorf("insert failed: %w", &pgconn.PgError{Code: "23505"}))
	if !common.IsErrConflict(err) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestMapInsertAASDescriptorErrorPreservesNonUniqueViolation(t *testing.T) {
	originalErr := fmt.Errorf("insert failed: %w", &pgconn.PgError{Code: "23503"})
	err := mapInsertAASDescriptorError(originalErr)
	if err != originalErr {
		t.Fatalf("expected original error, got %v", err)
	}
}

func TestAASRegistryReadPoolSelection(t *testing.T) {
	writer, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("writer sqlmock.New() failed: %v", err)
	}
	defer func() { _ = writer.Close() }()
	reader, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("reader sqlmock.New() failed: %v", err)
	}
	defer func() { _ = reader.Close() }()

	backend, err := NewPostgreSQLAASRegistryDatabaseFromPools(writer, reader, false)
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	if got := backend.readDB(t.Context()); got != reader {
		t.Fatal("eligible registry read did not select the reader")
	}
	writerCtx := common.WithWriterPostgresReads(t.Context())
	if got := backend.readDB(writerCtx); got != writer {
		t.Fatal("consistency-sensitive registry read did not select the writer")
	}
}

func TestAASRegistryExistenceChecksSelectPoolFromContext(t *testing.T) {
	checks := []struct {
		name string
		run  func(*PostgreSQLAASRegistryDatabase, context.Context) (bool, error)
	}{
		{
			name: "aas",
			run: func(backend *PostgreSQLAASRegistryDatabase, ctx context.Context) (bool, error) {
				return backend.ExistsAASByID(ctx, "aas-1")
			},
		},
		{
			name: "submodel",
			run: func(backend *PostgreSQLAASRegistryDatabase, ctx context.Context) (bool, error) {
				return backend.ExistsSubmodelForAAS(ctx, "aas-1", "sm-1")
			},
		},
	}
	contexts := []struct {
		name        string
		writerReads bool
	}{
		{name: "reader"},
		{name: "writer", writerReads: true},
	}

	for _, check := range checks {
		for _, contextCase := range contexts {
			t.Run(check.name+"/"+contextCase.name, func(t *testing.T) {
				writer, writerMock, err := sqlmock.New()
				if err != nil {
					t.Fatalf("writer sqlmock.New() failed: %v", err)
				}
				defer func() { _ = writer.Close() }()
				reader, readerMock, err := sqlmock.New()
				if err != nil {
					t.Fatalf("reader sqlmock.New() failed: %v", err)
				}
				defer func() { _ = reader.Close() }()

				backend, err := NewPostgreSQLAASRegistryDatabaseFromPools(writer, reader, false)
				if err != nil {
					t.Fatalf("create backend: %v", err)
				}
				selectedMock := readerMock
				ctx := t.Context()
				if contextCase.writerReads {
					selectedMock = writerMock
					ctx = common.WithWriterPostgresReads(ctx)
				}
				selectedMock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"one"}).AddRow(1))

				exists, existsErr := check.run(backend, ctx)
				if existsErr != nil {
					t.Fatalf("existence check failed: %v", existsErr)
				}
				if !exists {
					t.Fatal("expected descriptor to exist")
				}
				if err = writerMock.ExpectationsWereMet(); err != nil {
					t.Fatalf("writer expectations: %v", err)
				}
				if err = readerMock.ExpectationsWereMet(); err != nil {
					t.Fatalf("reader expectations: %v", err)
				}
			})
		}
	}
}
