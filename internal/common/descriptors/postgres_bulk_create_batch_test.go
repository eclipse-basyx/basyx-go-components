/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package descriptors

import (
	"context"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func TestBulkCreateCollectorUsesOneMultiRowInsertPerTable(t *testing.T) {
	t.Parallel()

	descriptors := []model.AssetAdministrationShellDescriptor{
		{Id: "urn:example:aas:1"},
		{Id: "urn:example:aas:2"},
	}
	rows := &bulkCreateRows{}
	cursor := &bulkCreateIDCursor{ids: bulkCreateIDs{descriptor: []int64{101, 102}}}
	for _, descriptor := range descriptors {
		if err := collectAASDescriptorRows(context.Background(), rows, cursor, descriptor); err != nil {
			t.Fatalf("collectAASDescriptorRows returned error: %v", err)
		}
	}

	batch := &common.PostgreSQLBatch{}
	if err := appendBulkCreateRows(batch, rows); err != nil {
		t.Fatalf("appendBulkCreateRows returned error: %v", err)
	}

	tableStatementCounts := map[string]int{}
	for _, statement := range batch.Statements() {
		for _, table := range []string{
			common.TblDescriptor,
			common.TblDescriptorPayload,
			common.TblAASDescriptor,
		} {
			if strings.HasPrefix(statement.SQL, `INSERT INTO "`+table+`"`) {
				tableStatementCounts[table]++
				if !strings.Contains(statement.SQL, "), (") {
					t.Fatalf("expected multi-row insert for %s, got %s", table, statement.SQL)
				}
			}
		}
	}

	for _, table := range []string{
		common.TblDescriptor,
		common.TblDescriptorPayload,
		common.TblAASDescriptor,
	} {
		if tableStatementCounts[table] != 1 {
			t.Fatalf("expected one insert for %s, got %d", table, tableStatementCounts[table])
		}
	}
}

func TestAppendChunkedRowsSplitsAfterBulkRowLimit(t *testing.T) {
	t.Parallel()

	rows := make([]goqu.Record, bulkInsertRowLimit+1)
	for index := range rows {
		rows[index] = goqu.Record{common.ColID: index + 1}
	}

	batch := &common.PostgreSQLBatch{}
	if err := appendChunkedRows(batch, common.TblDescriptor, rows, nil); err != nil {
		t.Fatalf("appendChunkedRows returned error: %v", err)
	}
	if len(batch.Statements()) != 2 {
		t.Fatalf("expected two chunked inserts, got %d", len(batch.Statements()))
	}
}
