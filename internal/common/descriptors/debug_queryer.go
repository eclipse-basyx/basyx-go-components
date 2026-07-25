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
	"database/sql"
	"log/slog"
	"time"
)

type descriptorDebugQueryer struct {
	ctx context.Context
	db  DBQueryer
}

func withDescriptorDebugQueryer(ctx context.Context, db DBQueryer) DBQueryer {
	if !debugEnabled(ctx) || db == nil {
		return db
	}
	if _, ok := db.(*descriptorDebugQueryer); ok {
		return db
	}
	return &descriptorDebugQueryer{
		ctx: ctx,
		db:  db,
	}
}

func (d *descriptorDebugQueryer) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if debugEnabled(ctx) || debugEnabled(d.ctx) {
		slog.DebugContext(ctx, "descriptor database query", "query", query)
	}
	start := time.Now()
	rows, err := d.db.QueryContext(ctx, query, args...)
	if debugEnabled(ctx) || debugEnabled(d.ctx) {
		slog.DebugContext(ctx, "descriptor database query completed", "duration", time.Since(start))
		if err != nil {
			slog.ErrorContext(ctx, "descriptor database query failed", "error.code", "DESCRIPTORS-QUERYCONTEXT-EXECUTE", "error", err)
		}
	}
	return rows, err
}

func (d *descriptorDebugQueryer) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if debugEnabled(ctx) || debugEnabled(d.ctx) {
		slog.DebugContext(ctx, "descriptor database row query", "query", query)
	}
	return d.db.QueryRowContext(ctx, query, args...)
}

func (d *descriptorDebugQueryer) Query(query string, args ...any) (*sql.Rows, error) {
	if debugEnabled(d.ctx) {
		slog.DebugContext(d.ctx, "descriptor database query", "query", query)
	}
	start := time.Now()
	rows, err := d.db.Query(query, args...)
	if debugEnabled(d.ctx) {
		slog.DebugContext(d.ctx, "descriptor database query completed", "duration", time.Since(start))
		if err != nil {
			slog.ErrorContext(d.ctx, "descriptor database query failed", "error.code", "DESCRIPTORS-QUERY-EXECUTE", "error", err)
		}
	}
	return rows, err
}
