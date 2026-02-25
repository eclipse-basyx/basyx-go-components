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
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/lib/pq"
)

type rowData struct {
	descID               int64
	specificID           int64
	name, value          sql.NullString
	semanticPayload      []byte
	externalSubjectRefID sql.NullInt64
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
	db DBQueryer,
	descriptorID int64,
) ([]types.ISpecificAssetID, error) {
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
	db DBQueryer,
	descriptorIDs []int64,
) (map[int64][]types.ISpecificAssetID, error) {
	if debugEnabled(ctx) {
		defer func(start time.Time) {
			_, _ = fmt.Printf("ReadSpecificAssetIDsByDescriptorIDs took %s\n", time.Since(start))
		}(time.Now())
	}
	out := make(map[int64][]types.ISpecificAssetID, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}

	d := goqu.Dialect(common.Dialect)
	externalSubjectReferenceAlias := goqu.T("specific_asset_id_external_subject_id_reference").As(common.AliasExternalSubjectReference)
	specificAssetIDPayloadAlias := goqu.T(common.TblSpecificAssetIDPayload).As("specific_asset_id_payload")

	arr := pq.Array(descriptorIDs)

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAASDesc)
	if err != nil {
		return nil, err
	}
	const dataAlias = "specific_asset_id_data"
	maskedColumns := []auth.MaskedInnerColumnSpec{
		{Fragment: "$aasdesc#specificAssetIds[].name", FlagAlias: "flag_said_name", RawAlias: "c2"},
		{Fragment: "$aasdesc#specificAssetIds[].value", FlagAlias: "flag_said_value", RawAlias: "c3"},
		{Fragment: "$aasdesc#specificAssetIds[].externalSubjectId", FlagAlias: "flag_said_external_subject", RawAlias: "c5"},
	}
	maskRuntime, err := auth.BuildSharedFragmentMaskRuntime(ctx, collector, maskedColumns)
	if err != nil {
		return nil, err
	}
	maskedExpressions, err := maskRuntime.MaskedInnerAliasExprs(dataAlias, maskedColumns)
	if err != nil {
		return nil, err
	}

	inner := d.From(common.TDescriptor).
		InnerJoin(
			common.TAASDescriptor,
			goqu.On(common.TAASDescriptor.Col(common.ColDescriptorID).Eq(common.TDescriptor.Col(common.ColID))),
		).
		LeftJoin(
			specificAssetIDAlias,
			goqu.On(specificAssetIDAlias.Col(common.ColDescriptorID).Eq(common.TDescriptor.Col(common.ColID))),
		).
		LeftJoin(
			externalSubjectReferenceAlias,
			goqu.On(externalSubjectReferenceAlias.Col(common.ColID).Eq(specificAssetIDAlias.Col(common.ColID))),
		).
		LeftJoin(
			specificAssetIDPayloadAlias,
			goqu.On(specificAssetIDPayloadAlias.Col(common.ColSpecificAssetID).Eq(specificAssetIDAlias.Col(common.ColID))),
		).Select(append([]interface{}{
		common.TSpecificAssetID.Col(common.ColDescriptorID).As("c0"),
		common.TSpecificAssetID.Col(common.ColID).As("c1"),
		common.TSpecificAssetID.Col(common.ColName).As("c2"),
		common.TSpecificAssetID.Col(common.ColValue).As("c3"),
		goqu.I("specific_asset_id_payload.semantic_id_payload").As("c4"),
		goqu.I(common.AliasExternalSubjectReference + "." + common.ColID).As("c5"),
		common.TSpecificAssetID.Col(common.ColPosition).As("sort_specific_asset_position"),
	}, maskRuntime.Projections()...)...).
		Where(goqu.L(fmt.Sprintf("%s.%s = ANY(?::bigint[])", common.AliasSpecificAssetID, common.ColDescriptorID), arr))

	inner = inner.
		Order(
			common.TSpecificAssetID.Col(common.ColPosition).Asc(),
		)

	inner, err = auth.AddFilterQueryFromContext(ctx, inner, "$aasdesc#specificAssetIds[]", collector)
	if err != nil {
		return nil, err
	}

	base := d.From(inner.As(dataAlias)).
		Select(
			goqu.I(dataAlias+".c0"),
			goqu.I(dataAlias+".c1"),
			maskedExpressions[0],
			maskedExpressions[1],
			goqu.I(dataAlias+".c4"),
			maskedExpressions[2],
		).
		Order(goqu.I(dataAlias + ".sort_specific_asset_position").Asc())

	sqlStr, args, err := base.ToSQL()
	if err != nil {
		return nil, err
	}
	if debugEnabled(ctx) {
		_, _ = fmt.Println(sqlStr)
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	perDesc := make(map[int64][]rowData, len(descriptorIDs))
	allSpecificIDs := make([]int64, 0, 256)

	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.descID,
			&r.specificID,
			&r.name,
			&r.value,
			&r.semanticPayload,
			&r.externalSubjectRefID,
		); err != nil {
			return nil, err
		}
		perDesc[r.descID] = append(perDesc[r.descID], r)
		allSpecificIDs = append(allSpecificIDs, r.specificID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(allSpecificIDs) == 0 {
		return out, nil
	}

	suppBySpecific, err := readSpecificAssetIDSupplementalSemanticBySpecificIDs(ctx, db, allSpecificIDs)
	if err != nil {
		return nil, err
	}

	extRefBySpecific, err := ReadSpecificAssetExternalSubjectReferencesBySpecificAssetIDs(ctx, db, allSpecificIDs)
	if err != nil {
		return nil, err
	}

	for descID, rowsForDesc := range perDesc {
		for _, r := range rowsForDesc {
			var extRef types.IReference
			if r.externalSubjectRefID.Valid {
				extRef = extRefBySpecific[r.specificID]
			}
			semRef, err := parseReferencePayload(r.semanticPayload)
			if err != nil {
				return nil, err
			}

			// out[descID] = append(out[descID], model.SpecificAssetID{
			// 	Name:                    nvl(r.name),
			// 	Value:                   nvl(r.value),
			// 	SemanticID:              semRef,
			// 	ExternalSubjectID:       extRef,
			// 	SupplementalSemanticIds: suppBySpecific[r.specificID],
			// })
			said := types.NewSpecificAssetID(nvl(r.name), nvl(r.value))
			if semRef != nil {
				said.SetSemanticID(semRef)
			}
			if extRef != nil {
				said.SetExternalSubjectID(extRef)
			}
			if suppRefs, ok := suppBySpecific[r.specificID]; ok {
				said.SetSupplementalSemanticIDs(suppRefs)
			}
			out[descID] = append(out[descID], said)
		}
	}

	return out, nil
}

func readSpecificAssetIDSupplementalSemanticBySpecificIDs(
	ctx context.Context,
	db DBQueryer,
	specificAssetIDs []int64,
) (map[int64][]types.IReference, error) {
	out := make(map[int64][]types.IReference, len(specificAssetIDs))
	if len(specificAssetIDs) == 0 {
		return out, nil
	}
	m, err := ReadSpecificAssetSupplementalSemanticReferencesBySpecificAssetIDs(ctx, db, specificAssetIDs)
	if err != nil {
		return nil, err
	}

	for _, id := range specificAssetIDs {
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
