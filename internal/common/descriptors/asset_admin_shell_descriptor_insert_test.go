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

package descriptors

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func TestBuildAASDescriptorInsertRecord_DoesNotWriteCreatedAtByDefault(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2024, time.January, 10, 15, 30, 0, 0, time.UTC)
	record := buildAASDescriptorInsertRecord(
		context.Background(),
		42,
		model.AssetAdministrationShellDescriptor{
			Id:        "aas-id",
			CreatedAt: &createdAt,
		},
	)

	if _, ok := record[common.ColCreatedAt]; ok {
		t.Fatalf("expected %q to be absent without override context", common.ColCreatedAt)
	}
}

func TestBuildAASDescriptorInsertRecord_WritesCreatedAtWhenOverrideEnabled(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2024, time.January, 10, 15, 30, 0, 0, time.UTC)
	ctx := WithAllowAASDescriptorCreatedAtOverride(context.Background())
	record := buildAASDescriptorInsertRecord(
		ctx,
		42,
		model.AssetAdministrationShellDescriptor{
			Id:        "aas-id",
			CreatedAt: &createdAt,
		},
	)

	got, ok := record[common.ColCreatedAt]
	if !ok {
		t.Fatalf("expected %q to be present when override context is enabled", common.ColCreatedAt)
	}

	gotTime, ok := got.(time.Time)
	if !ok {
		t.Fatalf("expected %q value to be time.Time, got %T", common.ColCreatedAt, got)
	}
	if !gotTime.Equal(createdAt) {
		t.Fatalf("expected createdAt %v, got %v", createdAt, gotTime)
	}
}

func TestBuildAASDescriptorInsertRecord_DoesNotWriteCreatedAtWhenMissing(t *testing.T) {
	t.Parallel()

	ctx := WithAllowAASDescriptorCreatedAtOverride(context.Background())
	record := buildAASDescriptorInsertRecord(
		ctx,
		42,
		model.AssetAdministrationShellDescriptor{
			Id: "aas-id",
		},
	)

	if _, ok := record[common.ColCreatedAt]; ok {
		t.Fatalf("expected %q to be absent when payload createdAt is nil", common.ColCreatedAt)
	}
}

func TestBuildAASDescriptorUpdateRecord_DoesNotWriteCreatedAt(t *testing.T) {
	t.Parallel()

	incomingCreatedAt := time.Date(2030, time.January, 10, 15, 30, 0, 0, time.UTC)
	ctx := WithAllowAASDescriptorCreatedAtOverride(context.Background())
	record := buildAASDescriptorUpdateRecord(
		ctx,
		42,
		model.AssetAdministrationShellDescriptor{
			Id:        "aas-id",
			CreatedAt: &incomingCreatedAt,
		},
	)

	if _, ok := record[common.ColDescriptorID]; ok {
		t.Fatalf("expected %q to be absent from update record", common.ColDescriptorID)
	}
	if _, ok := record[common.ColCreatedAt]; ok {
		t.Fatalf("expected %q to be absent from update record", common.ColCreatedAt)
	}
	if got := record[common.ColAASID]; got != "aas-id" {
		t.Fatalf("expected %q to be updated, got %#v", common.ColAASID, got)
	}
}

func TestBuildAASDescriptorUpsertLockSQLUsesPostgresPlaceholders(t *testing.T) {
	t.Parallel()

	query, args, err := buildAASDescriptorUpsertLockSQL("aas-1")

	if err != nil {
		t.Fatalf("buildAASDescriptorUpsertLockSQL returned error: %v", err)
	}
	if query != "SELECT pg_advisory_xact_lock(hashtextextended($1, $2))" {
		t.Fatalf("unexpected query: %s", query)
	}
	if len(args) != 2 || args[0] != "aas_descriptor:aas-1" || args[1] != int64(0) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestGetAASDescriptorCreatedAtByIDTxLocksAndReturnsTimestamp(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	createdAt := time.Date(2024, time.March, 1, 2, 3, 4, 0, time.UTC)
	query := regexp.QuoteMeta(`SELECT "created_at" FROM "aas_descriptor" WHERE ("id" = 'aas-1') FOR UPDATE`)

	mock.ExpectBegin()
	mock.ExpectQuery(query).
		WillReturnRows(sqlmock.NewRows([]string{common.ColCreatedAt}).AddRow(createdAt))
	mock.ExpectRollback()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	got, err := GetAASDescriptorCreatedAtByIDTx(context.Background(), tx, "aas-1")
	if err != nil {
		t.Fatalf("GetAASDescriptorCreatedAtByIDTx returned error: %v", err)
	}
	if !got.Equal(createdAt) {
		t.Fatalf("expected createdAt %v, got %v", createdAt, got)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatalf("failed to roll back transaction: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
