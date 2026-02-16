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
)

var bdExpMapper = []auth.ExpressionIdentifiableMapper{
	{
		Exp: tSpecificAssetID.Col(colID),
	},
	{
		Exp:      tSpecificAssetID.Col(colName),
		Fragment: fragPtr("$aasdesc#specificAssetIds[].name"),
	},
	{
		Exp:      tSpecificAssetID.Col(colValue),
		Fragment: fragPtr("$aasdesc#specificAssetIds[].value"),
	},
	{
		Exp: goqu.I("specific_asset_id_payload.semantic_id_payload"),
	},
	{
		Exp:      goqu.I(aliasExternalSubjectReference + "." + colID),
		Fragment: fragPtr("$aasdesc#specificAssetIds[].externalSubjectId"),
	},
}

// ReadSpecificAssetIDsByAASIdentifier returns SpecificAssetIDs linked via the
// discovery aas_identifier table.
func ReadSpecificAssetIDsByAASIdentifier(
	ctx context.Context,
	db *sql.DB,
	aasID string,
) ([]types.ISpecificAssetID, error) {
	var aasRef int64
	d := goqu.Dialect(dialect)
	tAASIdentifier := goqu.T(tblAASIdentifier)
	sqlStr, args, err := d.
		From(tAASIdentifier).
		Select(tAASIdentifier.Col(colID)).
		Where(tAASIdentifier.Col("aasid").Eq(aasID)).
		ToSQL()
	if err != nil {
		return nil, err
	}
	if err := db.QueryRowContext(ctx, sqlStr, args...).Scan(&aasRef); err != nil {
		if err == sql.ErrNoRows {
			return nil, common.NewErrNotFound("AAS identifier '" + aasID + "'")
		}
		return nil, err
	}
	return ReadSpecificAssetIDsByAASRef(ctx, db, aasRef)
}

// ReadSpecificAssetIDsByAASRef returns SpecificAssetIDs for a discovery AAS ref.
func ReadSpecificAssetIDsByAASRef(
	ctx context.Context,
	db DBQueryer,
	aasRef int64,
) ([]types.ISpecificAssetID, error) {
	if debugEnabled(ctx) {
		defer func(start time.Time) {
			_, _ = fmt.Printf("ReadSpecificAssetIDsByAASRef took %s\n", time.Since(start))
		}(time.Now())
	}

	d := goqu.Dialect(dialect)
	tAASIdentifier := goqu.T(tblAASIdentifier)
	externalSubjectReferenceAlias := goqu.T("specific_asset_id_external_subject_id_reference").As(aliasExternalSubjectReference)
	specificAssetIDPayloadAlias := goqu.T(tblSpecificAssetIDPayload).As("specific_asset_id_payload")
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootBD)
	if err != nil {
		return nil, err
	}
	expressions, err := auth.GetColumnSelectStatement(ctx, bdExpMapper, collector)
	if err != nil {
		return nil, err
	}

	ds := d.From(tSpecificAssetID).
		InnerJoin(
			tAASIdentifier,
			goqu.On(tSpecificAssetID.Col(colAASRef).Eq(tAASIdentifier.Col(colID))),
		).
		LeftJoin(
			externalSubjectReferenceAlias,
			goqu.On(externalSubjectReferenceAlias.Col(colID).Eq(tSpecificAssetID.Col(colID))),
		).
		LeftJoin(
			specificAssetIDPayloadAlias,
			goqu.On(specificAssetIDPayloadAlias.Col(colSpecificAssetID).Eq(tSpecificAssetID.Col(colID))),
		).
		Select(
			expressions[0],
			expressions[1],
			expressions[2],
			expressions[3],
			expressions[4],
		).
		Where(tSpecificAssetID.Col(colAASRef).Eq(aasRef)).
		Order(
			tSpecificAssetID.Col(colPosition).Asc(),
			tSpecificAssetID.Col(colID).Asc(),
		)

	ds, err = auth.AddFormulaQueryFromContext(ctx, ds, collector)
	if err != nil {
		return nil, err
	}

	sqlStr, args, err := ds.ToSQL()
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

	type rowData struct {
		specificID           int64
		name, value          sql.NullString
		semanticPayload      []byte
		externalSubjectRefID sql.NullInt64
	}

	perRef := make([]rowData, 0, 32)
	allSpecificIDs := make([]int64, 0, 32)

	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.specificID,
			&r.name,
			&r.value,
			&r.semanticPayload,
			&r.externalSubjectRefID,
		); err != nil {
			return nil, err
		}
		perRef = append(perRef, r)
		allSpecificIDs = append(allSpecificIDs, r.specificID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(allSpecificIDs) == 0 {
		return []types.ISpecificAssetID{}, nil
	}

	suppBySpecific, err := readSpecificAssetIDSupplementalSemanticBySpecificIDs(ctx, db, allSpecificIDs)
	if err != nil {
		return nil, err
	}

	extRefBySpecific, err := ReadSpecificAssetExternalSubjectReferencesBySpecificAssetIDs(ctx, db, allSpecificIDs)
	if err != nil {
		return nil, err
	}

	out := make([]types.ISpecificAssetID, 0, len(perRef))
	for _, r := range perRef {
		var extRef types.IReference
		if r.externalSubjectRefID.Valid {
			extRef = extRefBySpecific[r.specificID]
		}
		semRef, err := parseReferencePayload(r.semanticPayload)
		if err != nil {
			return nil, err
		}

		said := types.NewSpecificAssetID(nvl(r.name), nvl(r.value))
		said.SetSemanticID(semRef)
		said.SetExternalSubjectID(extRef)
		said.SetSupplementalSemanticIDs(suppBySpecific[r.specificID])
		out = append(out, said)
	}

	return out, nil
}

// ReplaceSpecificAssetIDsByAASIdentifier upserts the AAS identifier and replaces
// all linked SpecificAssetIDs.
func ReplaceSpecificAssetIDsByAASIdentifier(
	ctx context.Context,
	db *sql.DB,
	aasID string,
	specificAssetIDs []types.ISpecificAssetID,
) error {
	return WithTx(ctx, db, func(tx *sql.Tx) error {
		aasRef, err := ensureAASIdentifierTx(ctx, tx, aasID)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM specific_asset_id WHERE aasRef = $1`, aasRef); err != nil {
			return err
		}
		return insertSpecificAssetIDs(
			tx,
			sql.NullInt64{},
			sql.NullInt64{Int64: aasRef, Valid: true},
			specificAssetIDs,
		)
	})
}

func ensureAASIdentifierTx(ctx context.Context, tx *sql.Tx, aasID string) (int64, error) {
	var aasRef int64
	d := goqu.Dialect(dialect)
	tAASIdentifier := goqu.T(tblAASIdentifier)
	sqlStr, args, err := d.
		Insert(tblAASIdentifier).
		Rows(goqu.Record{"aasid": aasID}).
		OnConflict(
			goqu.DoUpdate(
				"aasid",
				goqu.Record{"aasid": goqu.I("excluded.aasid")},
			),
		).
		Returning(tAASIdentifier.Col(colID)).
		ToSQL()
	if err != nil {
		return 0, err
	}
	if err := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&aasRef); err != nil {
		return 0, err
	}
	return aasRef, nil
}
