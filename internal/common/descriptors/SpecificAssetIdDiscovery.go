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
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

var bdExpMapper = []auth.ExpressionIdentifiableMapper{
	{
		Exp: tSpecificAssetID.Col(colID),
	},
	{
		Exp:      tSpecificAssetID.Col(colName),
		Fragment: fragPtr("$bd#specificAssetIds[].name"),
	},
	{
		Exp:      tSpecificAssetID.Col(colValue),
		Fragment: fragPtr("$bd#specificAssetIds[].value"),
	},
	{
		Exp: tSpecificAssetID.Col(colSemanticID),
	},
	{
		Exp:      tSpecificAssetID.Col(colExternalSubjectRef),
		Fragment: fragPtr("$bd#specificAssetIds[].externalSubjectId"),
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
		semanticRefID        sql.NullInt64
		externalSubjectRefID sql.NullInt64
	}

	perRef := make([]rowData, 0, 32)
	allSpecificIDs := make([]int64, 0, 32)
	semRefIDs := make([]int64, 0, 16)
	extRefIDs := make([]int64, 0, 16)

	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.specificID,
			&r.name,
			&r.value,
			&r.semanticRefID,
			&r.externalSubjectRefID,
		); err != nil {
			return nil, err
		}
		perRef = append(perRef, r)
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
		return []types.ISpecificAssetID{}, nil
	}

	suppBySpecific, err := readSpecificAssetIDSupplementalSemanticBySpecificIDs(ctx, db, allSpecificIDs)
	if err != nil {
		return nil, err
	}

	allRefIDs := append(append([]int64{}, semRefIDs...), extRefIDs...)
	refByID := make(map[int64]types.IReference)
	if len(allRefIDs) > 0 {
		refByID, err = GetReferencesByIDsBatch(db, allRefIDs)
		if err != nil {
			return nil, err
		}
	}

	out := make([]types.ISpecificAssetID, 0, len(perRef))
	for _, r := range perRef {
		var semRef types.IReference
		if r.semanticRefID.Valid {
			semRef = refByID[r.semanticRefID.Int64]
		}
		var extRef types.IReference
		if r.externalSubjectRefID.Valid {
			extRef = refByID[r.externalSubjectRefID.Int64]
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
		if len(specificAssetIDs) == 0 {
			return nil
		}

		d := goqu.Dialect(dialect)
		for i, val := range specificAssetIDs {
			var a sql.NullInt64

			externalSubjectReferenceID, err := persistence_utils.CreateReference(tx, val.ExternalSubjectID(), a, a)
			if err != nil {
				return err
			}
			semanticID, err := persistence_utils.CreateReference(tx, val.SemanticID(), a, a)
			if err != nil {
				return err
			}

			sqlStr, args, err := d.
				Insert(tblSpecificAssetID).
				Rows(goqu.Record{
					colDescriptorID:       nil,
					colPosition:           i,
					colSemanticID:         semanticID,
					colName:               val.Name(),
					colValue:              val.Value(),
					colExternalSubjectRef: externalSubjectReferenceID,
					colAASRef:             aasRef,
				}).
				Returning(tSpecificAssetID.Col(colID)).
				ToSQL()
			if err != nil {
				return err
			}
			var id int64
			if err = tx.QueryRowContext(ctx, sqlStr, args...).Scan(&id); err != nil {
				return err
			}

			if err = createSpecificAssetIDSupplementalSemantic(tx, id, val.SupplementalSemanticIDs()); err != nil {
				return err
			}
		}
		return nil
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
