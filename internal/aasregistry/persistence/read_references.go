package persistence_postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// Assumes you have a package/global variable `dialect` like "postgres"
//
// var dialect = "postgres"

func GetReferencesByIdsBatch(db *sql.DB, ids []int64) (map[int64]*model.Reference, error) {
	if len(ids) == 0 {
		return map[int64]*model.Reference{}, nil
	}
	start := time.Now()

	d := goqu.Dialect(dialect)

	// --- 1) Load roots and their keys (LEFT JOIN to include roots without keys) ---
	r := goqu.T("reference").As("r")
	rk := goqu.T("reference_key").As("rk")

	qRoots := d.
		From(r).
		Select(
			r.Col("id").As("root_id"),
			r.Col("type").As("root_type"),
			rk.Col("id").As("key_id"),
			rk.Col("type").As("key_type"),
			rk.Col("value").As("key_value"),
		).
		LeftJoin(
			rk,
			goqu.On(rk.Col("reference_id").Eq(r.Col("id"))),
		).
		Where(r.Col("id").In(ids)).
		Order(r.Col("id").Asc())

	sqlRoots, argsRoots, err := qRoots.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("build root query: %w", err)
	}

	type rootRow struct {
		rootID   int64
		rootType string
		keyID    sql.NullInt64
		keyType  sql.NullString
		keyValue sql.NullString
	}

	rows, err := db.Query(sqlRoots, argsRoots...)
	if err != nil {
		return nil, fmt.Errorf("load roots: %w", err)
	}
	defer rows.Close()

	refs := make(map[int64]*model.Reference)
	builders := make(map[int64]*builder.ReferenceBuilder)

	for rows.Next() {
		var rr rootRow
		if err := rows.Scan(&rr.rootID, &rr.rootType, &rr.keyID, &rr.keyType, &rr.keyValue); err != nil {
			return nil, fmt.Errorf("scan root row: %w", err)
		}

		// Ensure builder exists per root
		rf, ok := refs[rr.rootID]
		var b *builder.ReferenceBuilder
		if !ok {
			rf, b = builder.NewReferenceBuilder(rr.rootType, rr.rootID)
			refs[rr.rootID] = rf
			builders[rr.rootID] = b
		} else {
			b = builders[rr.rootID]
		}

		// Add key if present
		if rr.keyID.Valid && rr.keyType.Valid && rr.keyValue.Valid {
			b.CreateKey(rr.keyID.Int64, rr.keyType.String, rr.keyValue.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate root rows: %w", err)
	}

	// If no roots found at all, return empty
	if len(refs) == 0 {
		return map[int64]*model.Reference{}, nil
	}

	// --- 2) Load all descendants for all roots in one go (including their keys) ---
	ref := goqu.T("reference").As("ref")

	qDesc := d.
		From(ref).
		Select(
			ref.Col("id").As("id"),
			ref.Col("type").As("type"),
			ref.Col("parentreference").As("parentreference"),
			ref.Col("rootreference").As("rootreference"),
			rk.Col("id").As("key_id"),
			rk.Col("type").As("key_type"),
			rk.Col("value").As("key_value"),
		).
		LeftJoin(
			rk,
			goqu.On(rk.Col("reference_id").Eq(ref.Col("id"))),
		).
		Where(
			goqu.And(
				ref.Col("rootreference").In(ids),
				ref.Col("id").Neq(ref.Col("rootreference")),
			),
		).
		Order(
			ref.Col("rootreference").Asc(),
			ref.Col("parentreference").Asc(),
			ref.Col("id").Asc(),
		)

	sqlDesc, argsDesc, err := qDesc.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("build descendant query: %w", err)
	}

	type descRow struct {
		id        int64
		typ       string
		parentRef sql.NullInt64
		rootRef   sql.NullInt64
		keyID     sql.NullInt64
		keyType   sql.NullString
		keyValue  sql.NullString
	}

	descRows, err := db.Query(sqlDesc, argsDesc...)
	if err != nil {
		return nil, fmt.Errorf("load descendants: %w", err)
	}
	defer descRows.Close()

	// Track "seen" per root to avoid duplicate CreateReferredSemanticId calls
	seenPerRoot := make(map[int64]map[int64]bool)

	for descRows.Next() {
		var dr descRow
		if err := descRows.Scan(&dr.id, &dr.typ, &dr.parentRef, &dr.rootRef, &dr.keyID, &dr.keyType, &dr.keyValue); err != nil {
			return nil, fmt.Errorf("scan descendant row: %w", err)
		}
		if !dr.rootRef.Valid {
			// Shouldn't happen if data is consistent; skip safely
			continue
		}
		rootID := dr.rootRef.Int64

		// Only build trees for roots we actually loaded
		b, ok := builders[rootID]
		if !ok {
			continue
		}

		if _, ok := seenPerRoot[rootID]; !ok {
			seenPerRoot[rootID] = make(map[int64]bool)
		}

		parentID := rootID
		if dr.parentRef.Valid {
			parentID = dr.parentRef.Int64
		}

		// Create node once
		if !seenPerRoot[rootID][dr.id] {
			b.CreateReferredSemanticID(dr.id, parentID, dr.typ)
			seenPerRoot[rootID][dr.id] = true
		}

		// Add key if present
		if dr.keyID.Valid && dr.keyType.Valid && dr.keyValue.Valid {
			if err := b.CreateReferredSemanticIDKey(dr.id, dr.keyID.Int64, dr.keyType.String, dr.keyValue.String); err != nil {
				return nil, err
			}
		}
	}
	if err := descRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate descendant rows: %w", err)
	}

	// --- 3) Link nested structures per root ---
	for _, b := range builders {
		b.BuildNestedStructure()
	}

	duration := time.Since(start)
	fmt.Printf("batch references took %v to complete\n", duration)
	return refs, nil
}

// readEntityReferences1ToMany loads references for a batch of entity IDs
// via a link table (entityFKCol -> referenceFKCol), hydrating full Reference trees.
func readEntityReferences1ToMany(
	ctx context.Context,
	db *sql.DB,
	entityIDs []int64,
	relationTable string, // e.g. "extension_reference_supplemental"
	entityFKCol string, // e.g. "extension_id"
	referenceFKCol string, // e.g. "reference_id"
) (map[int64][]model.Reference, error) {
	out := make(map[int64][]model.Reference, len(entityIDs))
	if len(entityIDs) == 0 {
		return out, nil
	}
	ids := entityIDs

	d := goqu.Dialect(dialect)
	lt := goqu.T(relationTable)

	ds := d.From(lt).
		Select(
			lt.Col(entityFKCol),
			lt.Col(referenceFKCol),
		).
		Where(lt.Col(entityFKCol).In(ids)).
		Order(lt.Col(entityFKCol).Asc(), lt.Col(referenceFKCol).Asc())

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("build link query: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query links: %w", err)
	}
	defer rows.Close()

	perEntityRefIDs := make(map[int64][]int64, len(ids))
	allRefIDs := make([]int64, 0, 256)

	for rows.Next() {
		var eID int64
		var rID sql.NullInt64
		if err := rows.Scan(&eID, &rID); err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		if rID.Valid {
			perEntityRefIDs[eID] = append(perEntityRefIDs[eID], rID.Int64)
			allRefIDs = append(allRefIDs, rID.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate links: %w", err)
	}

	// ensure presence for all requested IDs
	for _, id := range ids {
		if _, ok := perEntityRefIDs[id]; !ok {
			perEntityRefIDs[id] = nil
		}
	}

	if len(allRefIDs) == 0 {
		for k := range perEntityRefIDs {
			out[k] = nil
		}
		return out, nil
	}

	// (Optional) de-duplicate if desired; keeping behavior identical to original:
	uniqRefIDs := allRefIDs

	refByID, err := GetReferencesByIdsBatch(db, uniqRefIDs)
	if err != nil {
		return nil, fmt.Errorf("GetReferencesByIdsBatch: %w", err)
	}

	// assemble per entity
	for eID, refIDs := range perEntityRefIDs {
		if len(refIDs) == 0 {
			out[eID] = nil
			continue
		}
		seen := make(map[int64]struct{}, len(refIDs))
		list := make([]model.Reference, 0, len(refIDs))
		for _, rid := range refIDs {
			if _, ok := seen[rid]; ok {
				continue
			}
			seen[rid] = struct{}{}
			if r := refByID[rid]; r != nil {
				list = append(list, *r)
			}
		}
		out[eID] = list
	}

	return out, nil
}
