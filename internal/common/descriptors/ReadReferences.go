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
// Author: Martin Stemmer ( Fraunhofer IESE )

package descriptors

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/lib/pq"
)

// ReadSubmodelDescriptorSemanticReferencesByDescriptorIDs loads semantic
// references for submodel descriptors keyed by descriptor ID.
func ReadSubmodelDescriptorSemanticReferencesByDescriptorIDs(
	ctx context.Context,
	db DBQueryer,
	descriptorIDs []int64,
) (map[int64]types.IReference, error) {
	out := make(map[int64]types.IReference, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}

	rows, err := queryReferenceRowsByOwnerIDs(
		ctx,
		db,
		descriptorIDs,
		"submodel_descriptor",
		"descriptor_id",
		"submodel_descriptor_semantic_id_reference",
		"submodel_descriptor_semantic_id_reference_key",
	)
	if err != nil {
		return nil, err
	}

	for _, id := range descriptorIDs {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	for ownerID, ref := range rows {
		out[ownerID] = ref
	}

	return out, nil
}

// ReadSpecificAssetExternalSubjectReferencesBySpecificAssetIDs loads external
// subject references for specific asset IDs keyed by specific asset ID.
func ReadSpecificAssetExternalSubjectReferencesBySpecificAssetIDs(
	ctx context.Context,
	db DBQueryer,
	specificAssetIDs []int64,
) (map[int64]types.IReference, error) {
	out := make(map[int64]types.IReference, len(specificAssetIDs))
	if len(specificAssetIDs) == 0 {
		return out, nil
	}

	rows, err := queryReferenceRowsByOwnerIDs(
		ctx,
		db,
		specificAssetIDs,
		"specific_asset_id",
		"id",
		"specific_asset_id_external_subject_id_reference",
		"specific_asset_id_external_subject_id_reference_key",
	)
	if err != nil {
		return nil, err
	}

	for _, id := range specificAssetIDs {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	for ownerID, ref := range rows {
		out[ownerID] = ref
	}

	return out, nil
}

// ReadSpecificAssetSupplementalSemanticReferencesBySpecificAssetIDs loads
// supplemental semantic references for specific asset IDs keyed by specific
// asset ID.
func ReadSpecificAssetSupplementalSemanticReferencesBySpecificAssetIDs(
	ctx context.Context,
	db DBQueryer,
	specificAssetIDs []int64,
) (map[int64][]types.IReference, error) {
	out := make(map[int64][]types.IReference, len(specificAssetIDs))
	if len(specificAssetIDs) == 0 {
		return out, nil
	}

	for _, id := range specificAssetIDs {
		out[id] = nil
	}

	d := goqu.Dialect(dialect)
	arr := pq.Array(specificAssetIDs)

	rt := goqu.T(tblSpecificAssetIDSuppSemantic).As("rt")
	rkt := goqu.T(tblSpecificAssetIDSuppSemantic + "_key").As("rkt")
	rpt := goqu.T(tblSpecificAssetIDSuppSemantic + "_payload").As("rpt")

	ds := d.From(rt).
		LeftJoin(rpt, goqu.On(rpt.Col(colReferenceID).Eq(rt.Col(colID)))).
		LeftJoin(rkt, goqu.On(rkt.Col(colReferenceID).Eq(rt.Col(colID)))).
		Select(
			rt.Col(colSpecificAssetIDID).As("owner_id"),
			rt.Col(colID).As("ref_id"),
			rt.Col(colType).As("ref_type"),
			rkt.Col(colID).As("key_id"),
			rkt.Col(colType).As("key_type"),
			rkt.Col(colValue).As("key_value"),
			rpt.Col("parent_reference_payload").As("parent_reference_payload"),
		).
		Where(goqu.L("? = ANY(?::bigint[])", rt.Col(colSpecificAssetIDID), arr)).
		Order(
			rt.Col(colSpecificAssetIDID).Asc(),
			rt.Col(colID).Asc(),
			rkt.Col(colPosition).Asc(),
			rkt.Col(colID).Asc(),
		)

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REFREAD-SUPPSPEC-BUILDQUERY: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("REFREAD-SUPPSPEC-QUERYDB: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	type suppContextReferenceRow struct {
		ownerID                sql.NullInt64
		referenceID            sql.NullInt64
		refType                sql.NullInt64
		keyID                  sql.NullInt64
		keyType                sql.NullInt64
		keyVal                 sql.NullString
		parentReferencePayload []byte
	}

	refBuilders := map[int64]*builder.ReferenceBuilder{}
	refByID := map[int64]types.IReference{}
	refIDsByOwner := map[int64][]int64{}
	seenRefByOwner := map[int64]map[int64]struct{}{}

	for rows.Next() {
		var row suppContextReferenceRow
		if err := rows.Scan(
			&row.ownerID,
			&row.referenceID,
			&row.refType,
			&row.keyID,
			&row.keyType,
			&row.keyVal,
			&row.parentReferencePayload,
		); err != nil {
			return nil, fmt.Errorf("REFREAD-SUPPSPEC-SCANROW: %w", err)
		}

		if !row.ownerID.Valid || !row.referenceID.Valid || !row.refType.Valid {
			continue
		}
		ownerID := row.ownerID.Int64
		referenceID := row.referenceID.Int64

		if _, ok := refBuilders[referenceID]; !ok {
			ref, rb := builder.NewReferenceBuilder(types.ReferenceTypes(row.refType.Int64), referenceID)
			parentReference, err := parseReferencePayload(row.parentReferencePayload)
			if err != nil {
				return nil, fmt.Errorf("REFREAD-SUPPSPEC-PARSEPARENTPAYLOAD: %w", err)
			}
			ref.SetReferredSemanticID(parentReference)
			refBuilders[referenceID] = rb
			refByID[referenceID] = ref
		}

		if _, ok := seenRefByOwner[ownerID]; !ok {
			seenRefByOwner[ownerID] = map[int64]struct{}{}
		}
		if _, ok := seenRefByOwner[ownerID][referenceID]; !ok {
			seenRefByOwner[ownerID][referenceID] = struct{}{}
			refIDsByOwner[ownerID] = append(refIDsByOwner[ownerID], referenceID)
		}

		if row.keyID.Valid && row.keyType.Valid && row.keyVal.Valid {
			refBuilders[referenceID].CreateKey(
				row.keyID.Int64,
				types.KeyTypes(row.keyType.Int64),
				row.keyVal.String,
			)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("REFREAD-SUPPSPEC-ITERATEROWS: %w", err)
	}

	for _, b := range refBuilders {
		b.BuildNestedStructure()
	}

	for ownerID, referenceIDs := range refIDsByOwner {
		refs := make([]types.IReference, 0, len(referenceIDs))
		for _, referenceID := range referenceIDs {
			if ref, ok := refByID[referenceID]; ok {
				refs = append(refs, ref)
			}
		}
		out[ownerID] = refs
	}

	return out, nil
}

type contextReferenceRow struct {
	ownerID                int64
	refType                sql.NullInt64
	keyID                  sql.NullInt64
	keyType                sql.NullInt64
	keyVal                 sql.NullString
	parentReferencePayload []byte
}

func queryReferenceRowsByOwnerIDs(
	ctx context.Context,
	db DBQueryer,
	ownerIDs []int64,
	ownerTable string,
	ownerIDColumn string,
	referenceTable string,
	referenceKeyTable string,
) (map[int64]types.IReference, error) {
	if len(ownerIDs) == 0 {
		return map[int64]types.IReference{}, nil
	}

	d := goqu.Dialect(dialect)
	arr := pq.Array(ownerIDs)

	ot := goqu.T(ownerTable).As("ot")
	rt := goqu.T(referenceTable).As("rt")
	rkt := goqu.T(referenceKeyTable).As("rkt")
	rpt := goqu.T(referenceTable + "_payload").As("rpt")

	ds := d.From(ot).
		LeftJoin(rt, goqu.On(rt.Col(colID).Eq(ot.Col(ownerIDColumn)))).
		LeftJoin(rpt, goqu.On(rpt.Col(colReferenceID).Eq(rt.Col(colID)))).
		LeftJoin(rkt, goqu.On(rkt.Col(colReferenceID).Eq(rt.Col(colID)))).
		Select(
			ot.Col(ownerIDColumn).As("owner_id"),
			rt.Col(colType).As("ref_type"),
			rkt.Col(colID).As("key_id"),
			rkt.Col(colType).As("key_type"),
			rkt.Col(colValue).As("key_value"),
			rpt.Col("parent_reference_payload").As("parent_reference_payload"),
		).
		Where(goqu.L(fmt.Sprintf("ot.%s = ANY(?::bigint[])", ownerIDColumn), arr)).
		Order(
			ot.Col(ownerIDColumn).Asc(),
			rkt.Col(colPosition).Asc(),
			rkt.Col(colID).Asc(),
		)

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REFREAD-BUILDQUERY: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("REFREAD-QUERYDB: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	builders := make(map[int64]*builder.ReferenceBuilder, len(ownerIDs))
	refs := make(map[int64]types.IReference, len(ownerIDs))

	for rows.Next() {
		var row contextReferenceRow
		if err := rows.Scan(
			&row.ownerID,
			&row.refType,
			&row.keyID,
			&row.keyType,
			&row.keyVal,
			&row.parentReferencePayload,
		); err != nil {
			return nil, fmt.Errorf("REFREAD-SCANROW: %w", err)
		}

		if !row.refType.Valid {
			continue
		}

		b, ok := builders[row.ownerID]
		if !ok {
			ref, rb := builder.NewReferenceBuilder(types.ReferenceTypes(row.refType.Int64), row.ownerID)
			parentReference, err := parseReferencePayload(row.parentReferencePayload)
			if err != nil {
				return nil, fmt.Errorf("REFREAD-PARSEPARENTPAYLOAD: %w", err)
			}
			ref.SetReferredSemanticID(parentReference)
			refs[row.ownerID] = ref
			builders[row.ownerID] = rb
			b = rb
		}

		if row.keyID.Valid && row.keyType.Valid && row.keyVal.Valid {
			b.CreateKey(row.keyID.Int64, types.KeyTypes(row.keyType.Int64), row.keyVal.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("REFREAD-ITERATEROWS: %w", err)
	}

	for _, b := range builders {
		b.BuildNestedStructure()
	}

	return refs, nil
}

// readEntityReferences1ToMany loads references for a batch of entity IDs
// via a link table (entityFKCol -> referenceFKCol), hydrating full Reference trees.
func readEntityReferences1ToMany(
	ctx context.Context,
	db DBQueryer,
	entityIDs []int64,
	relationTable string,
	entityFKCol string,
	referenceFKCol string,
) (map[int64][]types.IReference, error) {
	out := make(map[int64][]types.IReference, len(entityIDs))
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
		list := make([]types.IReference, 0, len(refIDs))
		for _, rid := range refIDs {
			if _, ok := seen[rid]; ok {
				continue
			}
			seen[rid] = struct{}{}
			if r := refByID[rid]; r != nil {
				list = append(list, r)
			}
		}
		out[eID] = list
	}

	return out, nil
}

// GetReferencesByIDsBatch loads full references (including keys and nested
// referred semantic references) keyed by reference ID.
func GetReferencesByIDsBatch(db DBQueryer, ids []int64) (map[int64]types.IReference, error) {
	if len(ids) == 0 {
		return map[int64]types.IReference{}, nil
	}

	d := goqu.Dialect(dialect)
	arr := pq.Array(ids)

	r := goqu.T(tblReference).As("r")
	rk := goqu.T(tblReferenceKey).As("rk")

	qRoots := getQRoots(d, r, rk, arr)

	sqlRoots, argsRoots, err := qRoots.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("build root query: %w", err)
	}

	type rootRow struct {
		rootID   int64
		rootType int64
		keyID    sql.NullInt64
		keyType  sql.NullInt64
		keyValue sql.NullString
	}

	rows, err := db.Query(sqlRoots, argsRoots...)
	if err != nil {
		return nil, fmt.Errorf("load roots: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	refs := make(map[int64]types.IReference)
	builders := make(map[int64]*builder.ReferenceBuilder)

	for rows.Next() {
		var rr rootRow
		if err := rows.Scan(&rr.rootID, &rr.rootType, &rr.keyID, &rr.keyType, &rr.keyValue); err != nil {
			return nil, fmt.Errorf("scan root row: %w", err)
		}

		_, ok := refs[rr.rootID]
		var b *builder.ReferenceBuilder
		if !ok {
			refType := types.ReferenceTypes(rr.rootType)
			rf, nb := builder.NewReferenceBuilder(refType, rr.rootID)
			refs[rr.rootID] = rf
			builders[rr.rootID] = nb
			b = nb
		} else {
			b = builders[rr.rootID]
		}

		if rr.keyID.Valid && rr.keyType.Valid && rr.keyValue.Valid {
			b.CreateKey(rr.keyID.Int64, types.KeyTypes(rr.keyType.Int64), rr.keyValue.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate root rows: %w", err)
	}

	if len(refs) == 0 {
		return map[int64]types.IReference{}, nil
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

func processDescendantRows(descRows *sql.Rows, builders map[int64]*builder.ReferenceBuilder) error {
	type descRow struct {
		id        int64
		typ       int64
		parentRef sql.NullInt64
		rootRef   sql.NullInt64
		keyID     sql.NullInt64
		keyType   sql.NullInt64
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
			b.CreateReferredSemanticID(dr.id, parentID, types.ReferenceTypes(dr.typ))
			seenPerRoot[rootID][dr.id] = true
		}

		if dr.keyID.Valid && dr.keyType.Valid && dr.keyValue.Valid {
			if err := b.CreateReferredSemanticIDKey(dr.id, dr.keyID.Int64, types.KeyTypes(dr.keyType.Int64), dr.keyValue.String); err != nil {
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
