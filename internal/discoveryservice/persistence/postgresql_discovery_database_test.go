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

package persistencepostgresql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func TestSearchAASIDsByAssetLinks_GlobalAssetIDUsesIndexedUnionCandidates(t *testing.T) {
	t.Parallel()

	matcher := sqlmock.QueryMatcherFunc(func(_ string, actualSQL string) error {
		if !strings.Contains(actualSQL, `"ad_global"."global_asset_id" =`) {
			return fmt.Errorf("expected direct global_asset_id lookup, got SQL: %s", actualSQL)
		}
		if !strings.Contains(actualSQL, "UNION") {
			return fmt.Errorf("expected globalAssetId candidate UNION, got SQL: %s", actualSQL)
		}
		if !strings.Contains(actualSQL, `"sai_global"."name" =`) {
			return fmt.Errorf("expected indexed specific_asset_id globalAssetId branch, got SQL: %s", actualSQL)
		}
		if strings.Contains(actualSQL, "OR EXISTS") {
			return fmt.Errorf("did not expect specific_asset_id fallback OR, got SQL: %s", actualSQL)
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	backend, err := NewPostgreSQLDiscoveryBackendFromDB(db)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}

	rows := sqlmock.NewRows([]string{"aasid"}).AddRow("urn:aas:test:global")
	mock.ExpectQuery("global asset id lookup").WillReturnRows(rows)

	ids, nextCursor, err := backend.SearchAASIDsByAssetLinks(
		context.Background(),
		[]model.AssetLink{{Name: common.GlobalAssetIDAssetLinkName, Value: "global-asset"}},
		100,
		"",
	)
	if err != nil {
		t.Fatalf("expected search to succeed: %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("expected no next cursor, got %q", nextCursor)
	}
	if len(ids) != 1 || ids[0] != "urn:aas:test:global" {
		t.Fatalf("expected global AAS id result, got %#v", ids)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expected query to be executed: %v", err)
	}
}

func TestSearchAASIDsByAssetLinks_ValidatesCursorInPageQuery(t *testing.T) {
	t.Parallel()

	matcher := sqlmock.QueryMatcherFunc(func(_ string, actualSQL string) error {
		for _, expected := range []string{
			`SELECT 1 FROM "aas_identifier" AS "cursor_ai"`,
			`"cursor_ai"."aasid" = $1`,
			`"aas_identifier"."aasid" >= $2`,
		} {
			if !strings.Contains(actualSQL, expected) {
				return fmt.Errorf("expected SQL to contain %q, got: %s", expected, actualSQL)
			}
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	if err != nil {
		t.Fatalf("failed to create sqlmock database: %v", err)
	}
	defer func() { _ = db.Close() }()

	backend, err := NewPostgreSQLDiscoveryBackendFromDB(db)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}

	mock.ExpectQuery("cursor query").
		WithArgs("urn:aas:cursor", "urn:aas:cursor", 11).
		WillReturnRows(sqlmock.NewRows([]string{"aasid"}))

	ids, nextCursor, err := backend.SearchAASIDsByAssetLinks(
		t.Context(),
		nil,
		10,
		"urn:aas:cursor",
	)
	if err != nil {
		t.Fatalf("expected cursor search to succeed: %v", err)
	}
	if len(ids) != 0 || nextCursor != "" {
		t.Fatalf("expected an empty page without a next cursor, got ids=%#v cursor=%q", ids, nextCursor)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("cursor validation was not part of the page query: %v", err)
	}
}

func TestGetAllAssetLinksReturnsCodedErrorWhenReadTransactionFails(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock database: %v", err)
	}
	defer func() { _ = db.Close() }()

	backend, err := NewPostgreSQLDiscoveryBackendFromDB(db)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}

	mock.ExpectBegin().WillReturnError(errors.New("begin failed"))

	links, err := backend.GetAllAssetLinks(t.Context(), "urn:aas:test")
	if err == nil {
		t.Fatal("expected transaction start failure")
	}
	if links != nil {
		t.Fatalf("expected no links, got %#v", links)
	}
	if !strings.Contains(err.Error(), "DISC-GETASSETLINKS-QUERY") {
		t.Fatalf("expected coded discovery error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expected transaction start attempt: %v", err)
	}
}

func TestDiscoveryReadPoolSelection(t *testing.T) {
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

	backend, err := NewPostgreSQLDiscoveryBackendFromPools(writer, reader)
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	if got := backend.readDB(t.Context()); got != reader {
		t.Fatal("eligible discovery read did not select the reader")
	}
	writerCtx := common.WithWriterPostgresReads(t.Context())
	if got := backend.readDB(writerCtx); got != writer {
		t.Fatal("consistency-sensitive discovery read did not select the writer")
	}
}
