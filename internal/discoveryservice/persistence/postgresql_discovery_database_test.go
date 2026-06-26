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
	"fmt"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func TestSearchAASIDsByAssetLinks_GlobalAssetIDUsesOnlyDescriptorColumn(t *testing.T) {
	t.Parallel()

	matcher := sqlmock.QueryMatcherFunc(func(_ string, actualSQL string) error {
		if !strings.Contains(actualSQL, `"aas_descriptor"."global_asset_id" =`) {
			return fmt.Errorf("expected direct global_asset_id lookup, got SQL: %s", actualSQL)
		}
		if strings.Contains(actualSQL, "OR EXISTS") {
			return fmt.Errorf("did not expect specific_asset_id fallback OR, got SQL: %s", actualSQL)
		}
		if strings.Contains(actualSQL, `"sai"."name" =`) {
			return fmt.Errorf("did not expect generated asset-link fallback, got SQL: %s", actualSQL)
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
