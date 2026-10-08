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

package conditional

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // registers the postgres dialect
	"github.com/doug-martin/goqu/v9/exp"
)

const (
	revisionTable    = "resource_revision"
	revisionSequence = "basyx_resource_revision_seq"
	columnKind       = "kind"
	columnIdentifier = "identifier"
	columnRevision   = "revision"
)

var dialect = goqu.Dialect("postgres")

// Queryer is satisfied by *sql.DB and *sql.Tx.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func nextRevision() exp.LiteralExpression {
	return goqu.L("nextval('" + revisionSequence + "')")
}

func refCondition(ref ResourceRef) exp.Ex {
	return goqu.Ex{columnKind: string(ref.Kind), columnIdentifier: ref.Identifier}
}

// readRevision returns the committed revision of a resource. Resources
// without a revision row have revision 0.
func readRevision(ctx context.Context, q Queryer, ref ResourceRef) (int64, error) {
	ds := dialect.From(revisionTable).Select(goqu.C(columnRevision)).Where(refCondition(ref)).Prepared(true)
	return scanRevision(ctx, q, "COMMON-CONDREQ-READREVISION", ds)
}

// lockRevision locks the revision row of a resource for the transaction and
// returns its revision. A missing row is created as a placeholder with
// revision 0, which serializes concurrent first writes.
func lockRevision(ctx context.Context, tx *sql.Tx, ref ResourceRef) (int64, error) {
	placeholder := dialect.Insert(revisionTable).
		Rows(goqu.Record{columnKind: string(ref.Kind), columnIdentifier: ref.Identifier, columnRevision: 0}).
		OnConflict(goqu.DoNothing()).Prepared(true)
	if err := execDataset(ctx, tx, "COMMON-CONDREQ-LOCKREVISION-ENSURE", placeholder); err != nil {
		return 0, err
	}
	ds := dialect.From(revisionTable).Select(goqu.C(columnRevision)).Where(refCondition(ref)).
		ForUpdate(exp.Wait).Prepared(true)
	return scanRevision(ctx, tx, "COMMON-CONDREQ-LOCKREVISION-LOCK", ds)
}

// bumpRevision assigns a new revision to a locked resource.
func bumpRevision(ctx context.Context, tx *sql.Tx, ref ResourceRef) (int64, error) {
	ds := dialect.Update(revisionTable).Set(goqu.Record{columnRevision: nextRevision()}).
		Where(refCondition(ref)).Returning(goqu.C(columnRevision)).Prepared(true)
	return scanRevision(ctx, tx, "COMMON-CONDREQ-BUMPREVISION", ds)
}

// upsertRevisions assigns new revisions to resources in the given order with
// one statement and returns them.
func upsertRevisions(ctx context.Context, tx *sql.Tx, refs []ResourceRef, into map[ResourceRef]int64) error {
	if len(refs) == 0 {
		return nil
	}
	query, args, err := RevisionUpsertDataset(refs...).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("COMMON-CONDREQ-UPSERTREVISIONS-BUILDQ: %w", err)
	}
	result, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("COMMON-CONDREQ-UPSERTREVISIONS-EXECQ: %w", err)
	}
	defer func() {
		_ = result.Close()
	}()
	for result.Next() {
		var kind, identifier string
		var revision int64
		if err = result.Scan(&kind, &identifier, &revision); err != nil {
			return fmt.Errorf("COMMON-CONDREQ-UPSERTREVISIONS-SCAN: %w", err)
		}
		into[Ref(Kind(kind), identifier)] = revision
	}
	if err = result.Err(); err != nil {
		return fmt.Errorf("COMMON-CONDREQ-UPSERTREVISIONS-ROWS: %w", err)
	}
	return nil
}

func readRevisions(ctx context.Context, q Queryer, refs []ResourceRef) (map[ResourceRef]int64, error) {
	revisions := make(map[ResourceRef]int64, len(refs))
	for _, ref := range sortedRefs(refs) {
		revision, err := readRevision(ctx, q, ref)
		if err != nil {
			return nil, err
		}
		revisions[ref] = revision
	}
	return revisions, nil
}

func scanRevision(ctx context.Context, q Queryer, code string, ds interface {
	ToSQL() (string, []any, error)
}) (int64, error) {
	query, args, err := ds.ToSQL()
	if err != nil {
		return 0, fmt.Errorf("%s-BUILDQ: %w", code, err)
	}
	var revision int64
	err = q.QueryRowContext(ctx, query, args...).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("%s-EXECQ: %w", code, err)
	}
	return revision, nil
}

func execDataset(ctx context.Context, q Queryer, code string, ds interface {
	ToSQL() (string, []any, error)
}) error {
	query, args, err := ds.ToSQL()
	if err != nil {
		return fmt.Errorf("%s-BUILDQ: %w", code, err)
	}
	if _, err = q.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("%s-EXECQ: %w", code, err)
	}
	return nil
}

func sortedRefs(refs []ResourceRef) []ResourceRef {
	sorted := append([]ResourceRef(nil), refs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].less(sorted[j]) })
	return sorted
}
