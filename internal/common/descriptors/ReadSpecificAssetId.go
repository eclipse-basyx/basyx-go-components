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
	"encoding/json"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/lib/pq"
)

type rowData struct {
	descID               int64
	specificID           int64
	name, value          sql.NullString
	semanticJSON         json.RawMessage
	supplementalJSON     json.RawMessage
	externalSubjectRefID sql.NullInt64
}

var sai = goqu.T(tblSpecificAssetID).As("specific_asset_id")
var expMapper = []ExpressionIdentifiableMapper{
	{
		iexp:          sai.Col(colDescriptorID),
		canBeFiltered: false,
	},
	{
		iexp:          sai.Col(colID),
		canBeFiltered: false,
	},
	{
		iexp:          sai.Col(colName),
		canBeFiltered: true,
		identifable:   strPtr("$aasdesc#specificAssetIds[].name"),
	},
	{
		iexp:          sai.Col(colValue),
		canBeFiltered: true,
		identifable:   strPtr("$aasdesc#specificAssetIds[].value"),
	},
	{
		iexp:          sai.Col(colSemanticID),
		canBeFiltered: true,
	},
	{
		iexp:          sai.Col("supplemental_semantic_ids"),
		canBeFiltered: true,
	},
	{
		iexp:          sai.Col(colExternalSubjectRef),
		canBeFiltered: true,
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

// GetSpecificAssetIDsSubquery subquery that is not used at the momement
func GetSpecificAssetIDsSubquery(
	ctx context.Context,
	joinOn exp.IdentifierExpression, // e.g. goqu.I("aas.descriptor_id")
) (*goqu.SelectDataset, error) {
	d := goqu.Dialect(dialect)

	expressions, err := getColumnSelectStatement(ctx, expMapper)
	if err != nil {
		return nil, err
	}

	// Build a JSON object for each specific_asset_id row
	// expressions[] map to: descriptor_id, id, name, value, semantic_id, supplemental_semantic_ids, external_subject_ref
	rowObj := goqu.Func(
		"jsonb_build_object",
		goqu.L("'descriptor_id'"), expressions[0],
		goqu.L("'id'"), expressions[1],
		goqu.L("'name'"), expressions[2],
		goqu.L("'value'"), expressions[3],
		goqu.L("'semantic_id'"), expressions[4],
		goqu.L("'supplemental_semantic_ids'"), expressions[5],
		goqu.L("'external_subject_ref'"), expressions[6],
	)

	// Aggregate to a single jsonb array, deterministic ordering by position
	// NOTE: ORDER BY inside jsonb_agg requires a literal expression in goqu.
	agg := goqu.COALESCE(
		goqu.L("jsonb_agg(? ORDER BY ?)", rowObj, sai.Col("position")),
		goqu.L("'[]'::jsonb"),
	)

	base := getJoinTables(d).
		Select(agg).
		Where(joinOn.Eq(sai.Col(colDescriptorID))) // correlate to outer row (aas.descriptor_id = specific_asset_id.descriptor_id)

	base, err = addSpecificAssetFilter(ctx, base, "$aasdesc#specificAssetIds[]")
	if err != nil {
		return nil, err
	}

	return base, nil
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
		fmt.Printf("ReadSpecificAssetIDsByDescriptorIDs took %s for %d descriptor IDs\n", time.Since(start), len(descriptorIDs))
	}()

	out := make(map[int64][]model.SpecificAssetID, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}

	d := goqu.Dialect(dialect)

	arr := pq.Array(descriptorIDs)

	expressions, err := getColumnSelectStatement(ctx, expMapper)
	if err != nil {
		return nil, err
	}
	base := getJoinTables(d).Select(
		expressions[0],
		expressions[1],
		expressions[2],
		expressions[3],
		expressions[4],
		expressions[5],
		expressions[6],
	).
		Where(goqu.L("specific_asset_id.descriptor_id = ANY(?::bigint[])", arr)).
		GroupBy(
			expressions[0], // descriptor_id
			expressions[1], // id
		).
		Order(
			sai.Col("position").Asc(),
		)

	base, err = addSpecificAssetFilter(ctx, base, "$aasdesc#specificAssetIds[]")
	if err != nil {
		return nil, err
	}

	sqlStr, args, err := base.ToSQL()
	fmt.Println(sqlStr)
	if err != nil {
		return nil, err
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
	extRefIDs := make([]int64, 0, 128)

	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.descID,
			&r.specificID,
			&r.name,
			&r.value,
			&r.semanticJSON,
			&r.supplementalJSON,
			&r.externalSubjectRefID,
		); err != nil {
			return nil, err
		}
		perDesc[r.descID] = append(perDesc[r.descID], r)
		allSpecificIDs = append(allSpecificIDs, r.specificID)

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

	refByID := make(map[int64]*model.Reference)
	if len(extRefIDs) > 0 {
		refByID, err = GetReferencesByIDsBatch(db, extRefIDs)
		if err != nil {
			return nil, err
		}
	}

	for descID, rowsForDesc := range perDesc {
		for _, r := range rowsForDesc {

			semRef, err := unmarshalRefPtr(r.semanticJSON)
			if err != nil {
				return nil, fmt.Errorf("unmarshal semantic_id (specific_asset_id=%d): %w", r.specificID, err)
			}

			suppRefs, err := unmarshalRefs(r.supplementalJSON)
			if err != nil {
				return nil, fmt.Errorf("unmarshal supplemental_semantic_ids (specific_asset_id=%d): %w", r.specificID, err)
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
				SupplementalSemanticIds: suppRefs,
			})
		}
	}

	return out, nil
}

func nvl(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func strPtr(s string) *string {
	return &s
}

func isNullJSON(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	// Handles NULL scan (nil/empty) and JSON null
	return string(b) == "null"
}

func unmarshalRefPtr(raw json.RawMessage) (*model.Reference, error) {
	if isNullJSON(raw) {
		return nil, nil
	}
	var r model.Reference
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func unmarshalRefs(raw json.RawMessage) ([]model.Reference, error) {
	if isNullJSON(raw) {
		return nil, nil
	}
	// If your DB stores [] by default, this will produce empty slice.
	var refs []model.Reference
	if err := json.Unmarshal(raw, &refs); err != nil {
		return nil, err
	}
	return refs, nil
}
