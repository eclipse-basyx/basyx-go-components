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
	"fmt"
	"log/slog"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

const (
	tombstoneBatchSize       = 1000
	tombstoneCleanupInterval = time.Hour
	tombstoneCleanupDelay    = 5 * time.Minute
	tombstoneCleanupLock     = "basyx_resource_revision_tombstones"
)

// liveResource names the table and column that hold the live resources of a
// kind, so revisions of deleted resources can be found.
type liveResource struct {
	table  string
	column string
	filter exp.Expression
}

var liveResources = map[Kind]liveResource{
	KindAAS:                {table: "aas", column: "aas_id"},
	KindSubmodel:           {table: "submodel", column: "submodel_identifier"},
	KindConceptDescription: {table: "concept_description", column: "id"},
	KindAASDescriptor:      {table: "aas_descriptor", column: "id"},
	KindSubmodelDescriptor: {table: "submodel_descriptor", column: "id", filter: goqu.I("live.aas_descriptor_id").IsNull()},
	KindDiscoveryEntry:     {table: "aas_identifier", column: "aasid"},
	KindAASXPackage:        {table: "aasx_package", column: "package_id"},
	KindCompanyDescriptor:  {table: "company_descriptor", column: "company_domain"},
}

// StartTombstoneCleanup removes the revisions of deleted resources once an
// hour until ctx ends. Revisions of deleted resources stay as tombstones, so
// deletes need no revision write; the cleanup keeps them from accumulating.
// Services sharing a database never clean up at the same time.
func StartTombstoneCleanup(ctx context.Context, db *sql.DB) {
	go func() {
		timer := time.NewTimer(tombstoneCleanupDelay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			removed, err := RemoveTombstones(ctx, db)
			if err != nil {
				slog.WarnContext(ctx, "removing revisions of deleted resources failed", "error.code", "COMMON-CONDREQ-TOMBSTONES", "error", err)
			} else if removed > 0 {
				slog.InfoContext(ctx, "removed revisions of deleted resources", "count", removed)
			}
			timer.Reset(tombstoneCleanupInterval)
		}
	}()
}

// RemoveTombstones removes the revisions of all deleted resources in batches.
func RemoveTombstones(ctx context.Context, db *sql.DB) (int64, error) {
	var total int64
	for _, kind := range []Kind{KindAAS, KindSubmodel, KindConceptDescription, KindAASDescriptor,
		KindSubmodelDescriptor, KindDiscoveryEntry, KindAASXPackage, KindCompanyDescriptor} {
		for {
			removed, err := removeTombstoneBatch(ctx, db, kind)
			total += removed
			if err != nil {
				return total, err
			}
			if removed < tombstoneBatchSize {
				break
			}
		}
	}
	return total, nil
}

// removeTombstoneBatch first locks revisions whose resource does not exist,
// skipping revisions a writer holds, and then deletes those whose resource
// still does not exist in a new snapshot. A writer that recreates a resource
// meanwhile waits for the lock and then writes a new revision.
func removeTombstoneBatch(ctx context.Context, db *sql.DB, kind Kind) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("COMMON-CONDREQ-TOMBSTONES-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	acquired, err := tryCleanupLock(ctx, tx)
	if err != nil || !acquired {
		return 0, err
	}
	candidates, err := lockTombstones(ctx, tx, kind)
	if err != nil || len(candidates) == 0 {
		return 0, err
	}
	removed, err := deleteTombstones(ctx, tx, kind, candidates)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("COMMON-CONDREQ-TOMBSTONES-COMMIT: %w", err)
	}
	return removed, nil
}

func tryCleanupLock(ctx context.Context, tx *sql.Tx) (bool, error) {
	query, args, err := dialect.Select(goqu.Func("pg_try_advisory_xact_lock", goqu.Func("hashtextextended", tombstoneCleanupLock, 0))).
		Prepared(true).ToSQL()
	if err != nil {
		return false, fmt.Errorf("COMMON-CONDREQ-TOMBSTONES-BUILDLOCK: %w", err)
	}
	var acquired bool
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&acquired); err != nil {
		return false, fmt.Errorf("COMMON-CONDREQ-TOMBSTONES-LOCK: %w", err)
	}
	return acquired, nil
}

func deletedResourceCondition(kind Kind) exp.Expression {
	live := liveResources[kind]
	conditions := []exp.Expression{goqu.I("live." + live.column).Eq(goqu.I(revisionTable + "." + columnIdentifier))}
	if live.filter != nil {
		conditions = append(conditions, live.filter)
	}
	exists := dialect.From(goqu.T(live.table).As("live")).Select(goqu.L("1")).Where(conditions...)
	return goqu.And(goqu.C(columnKind).Eq(string(kind)), goqu.L("NOT EXISTS ?", exists))
}

func lockTombstones(ctx context.Context, tx *sql.Tx, kind Kind) ([]string, error) {
	query, args, err := dialect.From(revisionTable).Select(goqu.C(columnIdentifier)).
		Where(deletedResourceCondition(kind)).Order(goqu.C(columnIdentifier).Asc()).Limit(tombstoneBatchSize).
		ForUpdate(exp.SkipLocked, goqu.T(revisionTable)).Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("COMMON-CONDREQ-TOMBSTONES-BUILDSELECT: %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("COMMON-CONDREQ-TOMBSTONES-SELECT: %w", err)
	}
	defer func() { _ = rows.Close() }()
	identifiers := []string{}
	for rows.Next() {
		var identifier string
		if err = rows.Scan(&identifier); err != nil {
			return nil, fmt.Errorf("COMMON-CONDREQ-TOMBSTONES-SCAN: %w", err)
		}
		identifiers = append(identifiers, identifier)
	}
	return identifiers, rows.Err()
}

func deleteTombstones(ctx context.Context, tx *sql.Tx, kind Kind, identifiers []string) (int64, error) {
	query, args, err := dialect.Delete(revisionTable).
		Where(deletedResourceCondition(kind), goqu.C(columnIdentifier).In(identifiers)).Prepared(true).ToSQL()
	if err != nil {
		return 0, fmt.Errorf("COMMON-CONDREQ-TOMBSTONES-BUILDDELETE: %w", err)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("COMMON-CONDREQ-TOMBSTONES-DELETE: %w", err)
	}
	return result.RowsAffected()
}

// readLiveRevision reads in one statement whether a resource exists and its
// revision, so a kept revision of a deleted resource is never taken for a
// live one.
func readLiveRevision(ctx context.Context, q Queryer, ref ResourceRef) (bool, int64, error) {
	live, ok := liveResources[ref.Kind]
	if !ok {
		return false, 0, fmt.Errorf("COMMON-CONDREQ-READLIVEREVISION unknown resource kind %s", ref.Kind)
	}
	conditions := []exp.Expression{goqu.I("live." + live.column).Eq(ref.Identifier)}
	if live.filter != nil {
		conditions = append(conditions, live.filter)
	}
	exists := dialect.From(goqu.T(live.table).As("live")).Select(goqu.L("1")).Where(conditions...)
	ds := dialect.Select(goqu.L("EXISTS ?", exists), RevisionExpression(ref)).Prepared(true)
	query, args, err := ds.ToSQL()
	if err != nil {
		return false, 0, fmt.Errorf("COMMON-CONDREQ-READLIVEREVISION-BUILDQ: %w", err)
	}
	var found bool
	var revision int64
	if err = q.QueryRowContext(ctx, query, args...).Scan(&found, &revision); err != nil {
		return false, 0, fmt.Errorf("COMMON-CONDREQ-READLIVEREVISION-EXECQ: %w", err)
	}
	return found, revision, nil
}
