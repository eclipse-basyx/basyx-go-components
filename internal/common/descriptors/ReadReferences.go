/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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
// Author: Martin Stemmer ( Fraunhofer IESE )

package descriptors

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/lib/pq"
)

// GetReferencesByIDsBatch loads full Reference trees for a set of root reference
// IDs in one round of batched queries.
//
// It performs three steps:
//  1. Load all requested root references and any keys attached to them.
//  2. Load all descendant references for those roots (children, grandchildren,
//     …) together with their keys.
//  3. Link the flat rows into nested structures via builder.BuildNestedStructure.
//
// The function returns a map keyed by root reference ID to the fully hydrated
// *model.Reference. Missing or unknown IDs are simply absent from the result
// map. If ids is empty, an empty map is returned.
//
// The query uses LEFT JOINs so roots without keys are still returned. Within a
// root, duplicates from the SQL result are de-duplicated when constructing the
// tree; multiple keys for the same node are accumulated.
//
// Errors are returned for SQL statement construction failures, query/scan
// errors, or if the builder returns an error while attaching keys.
//
// Note: the function prints the elapsed time to stdout for basic diagnostics.
func GetReferencesByIDsBatch(db *sql.DB, ids []int64) (map[int64]*model.Reference, error) {
	if len(ids) == 0 {
		return map[int64]*model.Reference{}, nil
	}

	d := goqu.Dialect(dialect)
	arr := pq.Array(ids)

	// --- 1) Load roots and their keys (LEFT JOIN to include roots without keys) ---
	r := goqu.T(tblReference).As("r")
	rk := goqu.T(tblReferenceKey).As("rk")

	qRoots := getQRoots(d, r, rk, arr)

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
	defer func() {
		_ = rows.Close()
	}()

	refs := make(map[int64]*model.Reference)
	builders := make(map[int64]*builder.ReferenceBuilder)

	for rows.Next() {
		var rr rootRow
		if err := rows.Scan(&rr.rootID, &rr.rootType, &rr.keyID, &rr.keyType, &rr.keyValue); err != nil {
			return nil, fmt.Errorf("scan root row: %w", err)
		}

		_, ok := refs[rr.rootID]
		var b *builder.ReferenceBuilder
		if !ok {
			rf, nb := builder.NewReferenceBuilder(rr.rootType, rr.rootID)
			refs[rr.rootID] = rf
			builders[rr.rootID] = nb
			b = nb
		} else {
			b = builders[rr.rootID]
		}

		if rr.keyID.Valid && rr.keyType.Valid && rr.keyValue.Valid {
			b.CreateKey(rr.keyID.Int64, rr.keyType.String, rr.keyValue.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate root rows: %w", err)
	}

	if len(refs) == 0 {
		return map[int64]*model.Reference{}, nil
	}

	ref := goqu.T(tblReference).As("ref")

	qDesc := getQDesc(d, ref, rk, arr)

	sqlDesc, argsDesc, err := qDesc.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("build descendant query: %w", err)
	}

	descRows, err := db.Query(sqlDesc, argsDesc...)
	if err != nil {
		return nil, fmt.Errorf("load descendants: %w", err)
	}
	defer func() {
		_ = descRows.Close()
	}()

	if err := processDescendantRows(descRows, builders); err != nil {
		return nil, err
	}

	for _, b := range builders {
		b.BuildNestedStructure()
	}

	return refs, nil
}

// processDescendantRows processes the descendant rows and builds the reference tree
func processDescendantRows(descRows *sql.Rows, builders map[int64]*builder.ReferenceBuilder) error {
	type descRow struct {
		id        int64
		typ       string
		parentRef sql.NullInt64
		rootRef   sql.NullInt64
		keyID     sql.NullInt64
		keyType   sql.NullString
		keyValue  sql.NullString
	}

	seenPerRoot := make(map[int64]map[int64]bool)

	for descRows.Next() {
		var dr descRow
		if err := descRows.Scan(&dr.id, &dr.typ, &dr.parentRef, &dr.rootRef, &dr.keyID, &dr.keyType, &dr.keyValue); err != nil {
			return fmt.Errorf("scan descendant row: %w", err)
		}
		if !dr.rootRef.Valid {
			continue
		}
		rootID := dr.rootRef.Int64

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

		if !seenPerRoot[rootID][dr.id] {
			b.CreateReferredSemanticID(dr.id, parentID, dr.typ)
			seenPerRoot[rootID][dr.id] = true
		}

		if dr.keyID.Valid && dr.keyType.Valid && dr.keyValue.Valid {
			if err := b.CreateReferredSemanticIDKey(dr.id, dr.keyID.Int64, dr.keyType.String, dr.keyValue.String); err != nil {
				return err
			}
		}
	}
	if err := descRows.Err(); err != nil {
		return fmt.Errorf("iterate descendant rows: %w", err)
	}

	return nil
}

func getQRoots(d goqu.DialectWrapper, r exp.AliasedExpression, rk exp.AliasedExpression, arr any) *goqu.SelectDataset {
	qRoots := d.
		From(r).
		Select(
			r.Col(colID).As("root_id"),
			r.Col(colType).As("root_type"),
			rk.Col(colID).As("key_id"),
			rk.Col(colType).As("key_type"),
			rk.Col(colValue).As("key_value"),
		).
		LeftJoin(
			rk,
			goqu.On(rk.Col(colReferenceID).Eq(r.Col(colID))),
		).
		Where(goqu.L(fmt.Sprintf("r.%s = ANY(?::bigint[])", colID), arr)).
		Order(r.Col(colID).Asc())
	return qRoots
}

func getQDesc(d goqu.DialectWrapper, ref exp.AliasedExpression, rk exp.AliasedExpression, arr any) *goqu.SelectDataset {
	qDesc := d.
		From(ref).
		Select(
			ref.Col(colID).As("id"),
			ref.Col(colType).As("type"),
			ref.Col(colParentReference).As(colParentReference),
			ref.Col(colRootReference).As(colRootReference),
			rk.Col(colID).As("key_id"),
			rk.Col(colType).As("key_type"),
			rk.Col(colValue).As("key_value"),
		).
		LeftJoin(
			rk,
			goqu.On(rk.Col(colReferenceID).Eq(ref.Col(colID))),
		).
		Where(
			goqu.And(
				goqu.L(fmt.Sprintf("ref.%s = ANY(?::bigint[])", colRootReference), arr),
				ref.Col(colID).Neq(ref.Col(colRootReference)),
			),
		).
		Order(
			ref.Col(colRootReference).Asc(),
			ref.Col(colParentReference).Asc(),
			ref.Col(colID).Asc(),
		)
	return qDesc
}

// readEntityReferences1ToMany loads references for a batch of entity IDs
// via a link table (entityFKCol -> referenceFKCol), hydrating full Reference trees.
func readEntityReferences1ToMany(
	ctx context.Context,
	db *sql.DB,
	entityIDs []int64,
	relationTable string,
	entityFKCol string,
	referenceFKCol string,
) (map[int64][]model.Reference, error) {
	out := make(map[int64][]model.Reference, len(entityIDs))
	if len(entityIDs) == 0 {
		return out, nil
	}
	ids := entityIDs

	d := goqu.Dialect(dialect)
	lt := goqu.T(relationTable)

	arr := pq.Array(ids)
	ds := d.From(lt).
		Select(
			lt.Col(entityFKCol),
			lt.Col(referenceFKCol),
		).
		Where(goqu.L(fmt.Sprintf("%s = ANY(?::bigint[])", entityFKCol), arr)).
		Order(lt.Col(entityFKCol).Asc(), lt.Col(referenceFKCol).Asc())

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("build link query: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query links: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

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

	uniqRefIDs := allRefIDs

	refByID, err := GetReferencesByIDsBatch(db, uniqRefIDs)
	if err != nil {
		return nil, fmt.Errorf("GetReferencesByIdsBatch: %w", err)
	}

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
