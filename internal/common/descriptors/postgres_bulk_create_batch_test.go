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
	if err := appendBulkCreateRows(context.Background(), batch, rows); err != nil {
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

func TestBulkCreateCollectorSupportsGlobalSubmodelDescriptors(t *testing.T) {
	t.Parallel()

	endpoint := model.Endpoint{
		Interface: "SUBMODEL-3.0",
		ProtocolInformation: model.ProtocolInformation{
			Href: "https://example.com/submodel",
		},
	}
	rows := &bulkCreateRows{}
	cursor := &bulkCreateIDCursor{ids: bulkCreateIDs{descriptor: []int64{201}}}
	err := collectSubmodelDescriptorRows(
		rows,
		cursor,
		nil,
		0,
		model.SubmodelDescriptor{Id: "urn:example:submodel:1", Endpoints: []model.Endpoint{endpoint}},
	)
	if err != nil {
		t.Fatalf("collectSubmodelDescriptorRows returned error: %v", err)
	}

	batch := &common.PostgreSQLBatch{}
	if err = appendBulkCreateRows(context.Background(), batch, rows); err != nil {
		t.Fatalf("appendBulkCreateRows returned error: %v", err)
	}

	var submodelInsert string
	for _, statement := range batch.Statements() {
		if strings.HasPrefix(statement.SQL, `INSERT INTO "`+common.TblSubmodelDescriptor+`"`) {
			submodelInsert = statement.SQL
		}
	}
	if submodelInsert == "" {
		t.Fatal("expected submodel descriptor insert statement")
	}
	if !strings.Contains(submodelInsert, `"aas_descriptor_id"`) || !strings.Contains(submodelInsert, "NULL") {
		t.Fatalf("expected global submodel insert to use NULL aas_descriptor_id, got %s", submodelInsert)
	}
}

func TestAppendChunkedRowsSplitsAfterBulkBatchLimit(t *testing.T) {
	t.Parallel()

	const limit = 2

	rows := make([]goqu.Record, limit+1)
	for index := range rows {
		rows[index] = goqu.Record{common.ColID: index + 1}
	}

	batch := &common.PostgreSQLBatch{}
	if err := appendChunkedRows(batch, common.TblDescriptor, rows, nil, limit); err != nil {
		t.Fatalf("appendChunkedRows returned error: %v", err)
	}
	if len(batch.Statements()) != 2 {
		t.Fatalf("expected two chunked inserts, got %d", len(batch.Statements()))
	}
}
