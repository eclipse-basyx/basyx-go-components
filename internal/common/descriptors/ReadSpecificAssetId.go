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
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/lib/pq"
)

type rowData struct {
	descID               int64
	specificID           int64
	name, value          sql.NullString
	semanticRefID        sql.NullInt64
	externalSubjectRefID sql.NullInt64
}

var expMapper = []auth.ExpressionIdentifiableMapper{
	{
		Exp:           tSpecificAssetID.Col(colDescriptorID),
		CanBeFiltered: false,
	},
	{
		Exp:           tSpecificAssetID.Col(colID),
		CanBeFiltered: false,
	},
	{
		Exp:           tSpecificAssetID.Col(colName),
		CanBeFiltered: true,
		Fragment:      fragPtr("$aasdesc#specificAssetIds[].name"),
	},
	{
		Exp:           tSpecificAssetID.Col(colValue),
		CanBeFiltered: true,
		Fragment:      fragPtr("$aasdesc#specificAssetIds[].value"),
	},
	{
		Exp:           tSpecificAssetID.Col(colSemanticID),
		CanBeFiltered: true,
	},
	{
		Exp:           tSpecificAssetID.Col(colExternalSubjectRef),
		CanBeFiltered: true,
	},
}

// ReadSpecificAssetIDsByDescriptorID returns all SpecificAssetIDs that belong to
// a single AAS descriptor identified by its numeric descriptor ID.
//
// Parameters:
// - ctx: request-scoped context used for cancelation and deadlines.
// - db: open PostgreSQL handle.
// - descriptorID: the primary key (bigint) of the AAS descriptor row.
//
// It internally delegates to ReadSpecificAssetIDsByDescriptorIDs for efficient
// query construction and returns the slice mapped to the provided descriptorID.
// When the descriptor has no SpecificAssetIDs, it returns an empty slice (nil
// allowed) and a nil error.
func ReadSpecificAssetIDsByDescriptorID(
	ctx context.Context,
	db *sql.DB,
	descriptorID int64,
) ([]model.SpecificAssetID, error) {
	v, err := ReadSpecificAssetIDsByDescriptorIDs(ctx, db, []int64{descriptorID})
	return v[descriptorID], err
}

// ReadSpecificAssetIDsByDescriptorIDs performs a batched read of SpecificAssetIDs
// for multiple AAS descriptors in a single query.
//
// Parameters:
// - ctx: request-scoped context used for cancelation and deadlines.
// - db: open PostgreSQL handle.
// - descriptorIDs: list of AAS descriptor primary keys (bigint) to fetch for.
//
// Returns a map keyed by descriptor ID with the corresponding ordered slice of
// SpecificAssetID domain models. Descriptors with no SpecificAssetIDs are
// present in the map with a nil slice to distinguish from absent keys.
//
// Implementation notes:
// - Uses goqu to build SQL and pq.Array for efficient ANY(bigint[]) filtering.
// - Preloads semantic and external subject references in one pass to avoid N+1.
// - Preserves a stable order by descriptor_id, id to ensure deterministic output.
func ReadSpecificAssetIDsByDescriptorIDs(
	ctx context.Context,
	db *sql.DB,
	descriptorIDs []int64,
) (map[int64][]model.SpecificAssetID, error) {
	start := time.Now()
	defer func() {
		_, _ = fmt.Printf("ReadSpecificAssetIDsByDescriptorIDs took %s for %d descriptor IDs\n", time.Since(start), len(descriptorIDs))
	}()
	out := make(map[int64][]model.SpecificAssetID, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}

	d := goqu.Dialect(dialect)

	arr := pq.Array(descriptorIDs)

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot("$aasdesc", "descriptor_flags")
	if err != nil {
		return nil, err
	}
	expressions, err := auth.GetColumnSelectStatement(ctx, expMapper, collector)
	if err != nil {
		return nil, err
	}
	base := d.From(tDescriptor).
		InnerJoin(
			tAASDescriptor,
			goqu.On(tAASDescriptor.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
		).
		LeftJoin(
			specificAssetIDAlias,
			goqu.On(specificAssetIDAlias.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
		).Select(
		expressions[0],
		expressions[1],
		expressions[2],
		expressions[3],
		expressions[4],
		expressions[5],
	).
		Where(goqu.L(fmt.Sprintf("%s.%s = ANY(?::bigint[])", aliasSpecificAssetID, colDescriptorID), arr)).
		GroupBy(
			expressions[0], // descriptor_id
			expressions[1], // id
		).
		Order(
			tSpecificAssetID.Col(colPosition).Asc(),
		)

	base, err = auth.AddFilterQueryFromContext(ctx, base, "$aasdesc#specificAssetIds[]", collector)
	if err != nil {
		return nil, err
	}
	cteWhere := goqu.L(fmt.Sprintf("%s.%s = ANY(?::bigint[])", aliasSpecificAssetID, colDescriptorID), arr)
	base, err = auth.ApplyResolvedFieldPathCTEs(base, collector, cteWhere)
	if err != nil {
		return nil, err
	}

	sqlStr, args, err := base.ToSQL()
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Println(sqlStr)
	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	perDesc := make(map[int64][]rowData, len(descriptorIDs))
	allSpecificIDs := make([]int64, 0, 256)
	semRefIDs := make([]int64, 0, 128)
	extRefIDs := make([]int64, 0, 128)

	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.descID,
			&r.specificID,
			&r.name,
			&r.value,
			&r.semanticRefID,
			&r.externalSubjectRefID,
		); err != nil {
			return nil, err
		}
		perDesc[r.descID] = append(perDesc[r.descID], r)
		allSpecificIDs = append(allSpecificIDs, r.specificID)
		if r.semanticRefID.Valid {
			semRefIDs = append(semRefIDs, r.semanticRefID.Int64)
		}
		if r.externalSubjectRefID.Valid {
			extRefIDs = append(extRefIDs, r.externalSubjectRefID.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(allSpecificIDs) == 0 {
		return out, nil
	}

	uniqSem := semRefIDs
	uniqExt := extRefIDs

	suppBySpecific, err := readSpecificAssetIDSupplementalSemanticBySpecificIDs(ctx, db, allSpecificIDs)
	if err != nil {
		return nil, err
	}

	allRefIDs := append(append([]int64{}, uniqSem...), uniqExt...)
	refByID := make(map[int64]*model.Reference)
	if len(allRefIDs) > 0 {
		refByID, err = GetReferencesByIDsBatch(db, allRefIDs)
		if err != nil {
			return nil, err
		}
	}

	for descID, rowsForDesc := range perDesc {
		for _, r := range rowsForDesc {
			var semRef *model.Reference
			if r.semanticRefID.Valid {
				semRef = refByID[r.semanticRefID.Int64]
			}
			var extRef *model.Reference
			if r.externalSubjectRefID.Valid {
				extRef = refByID[r.externalSubjectRefID.Int64]
			}

			out[descID] = append(out[descID], model.SpecificAssetID{
				Name:                    nvl(r.name),
				Value:                   nvl(r.value),
				SemanticID:              semRef,
				ExternalSubjectID:       extRef,
				SupplementalSemanticIds: suppBySpecific[r.specificID],
			})
		}
	}

	return out, nil
}

func readSpecificAssetIDSupplementalSemanticBySpecificIDs(
	ctx context.Context,
	db *sql.DB,
	specificAssetIDs []int64,
) (map[int64][]model.Reference, error) {
	out := make(map[int64][]model.Reference, len(specificAssetIDs))
	if len(specificAssetIDs) == 0 {
		return out, nil
	}
	uniqSpecific := specificAssetIDs

	m, err := readEntityReferences1ToMany(
		ctx,
		db,
		specificAssetIDs,
		tblSpecificAssetIDSuppSemantic,
		colSpecificAssetIDID,
		colReferenceID,
	)
	if err != nil {
		return nil, err
	}

	for _, id := range uniqSpecific {
		out[id] = m[id]
	}
	return out, nil
}

func nvl(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func fragPtr(s string) *grammar.FragmentStringPattern {
	frag := grammar.FragmentStringPattern(s)
	return &frag
}
