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

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

// ReadSpecificAssetIDsByAASIdentifier returns SpecificAssetIDs linked via the
// discovery aas_identifier table.
func ReadSpecificAssetIDsByAASIdentifier(
	ctx context.Context,
	db *sql.DB,
	aasID string,
) ([]model.SpecificAssetID, error) {
	var aasRef int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM aas_identifier WHERE aasId = $1`, aasID).Scan(&aasRef); err != nil {
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
) ([]model.SpecificAssetID, error) {
	if debugEnabled(ctx) {
		defer func(start time.Time) {
			_, _ = fmt.Printf("ReadSpecificAssetIDsByAASRef took %s\n", time.Since(start))
		}(time.Now())
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id, name, value, semantic_id, external_subject_ref
		FROM specific_asset_id
		WHERE aasRef = $1
		ORDER BY position ASC, id ASC
	`, aasRef)
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
		return []model.SpecificAssetID{}, nil
	}

	suppBySpecific, err := readSpecificAssetIDSupplementalSemanticBySpecificIDs(ctx, db, allSpecificIDs)
	if err != nil {
		return nil, err
	}

	allRefIDs := append(append([]int64{}, semRefIDs...), extRefIDs...)
	refByID := make(map[int64]*model.Reference)
	if len(allRefIDs) > 0 {
		refByID, err = GetReferencesByIDsBatch(db, allRefIDs)
		if err != nil {
			return nil, err
		}
	}

	out := make([]model.SpecificAssetID, 0, len(perRef))
	for _, r := range perRef {
		var semRef *model.Reference
		if r.semanticRefID.Valid {
			semRef = refByID[r.semanticRefID.Int64]
		}
		var extRef *model.Reference
		if r.externalSubjectRefID.Valid {
			extRef = refByID[r.externalSubjectRefID.Int64]
		}

		out = append(out, model.SpecificAssetID{
			Name:                    nvl(r.name),
			Value:                   nvl(r.value),
			SemanticID:              semRef,
			ExternalSubjectID:       extRef,
			SupplementalSemanticIds: suppBySpecific[r.specificID],
		})
	}

	return out, nil
}

// ReplaceSpecificAssetIDsByAASIdentifier upserts the AAS identifier and replaces
// all linked SpecificAssetIDs.
func ReplaceSpecificAssetIDsByAASIdentifier(
	ctx context.Context,
	db *sql.DB,
	aasID string,
	specificAssetIDs []model.SpecificAssetID,
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

			externalSubjectReferenceID, err := persistence_utils.CreateReference(tx, val.ExternalSubjectID, a, a)
			if err != nil {
				return err
			}
			semanticID, err := persistence_utils.CreateReference(tx, val.SemanticID, a, a)
			if err != nil {
				return err
			}

			sqlStr, args, err := d.
				Insert(tblSpecificAssetID).
				Rows(goqu.Record{
					colDescriptorID:       nil,
					colPosition:           i,
					colSemanticID:         semanticID,
					colName:               val.Name,
					colValue:              val.Value,
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

			if err = createSpecificAssetIDSupplementalSemantic(tx, id, val.SupplementalSemanticIds); err != nil {
				return err
			}
		}
		return nil
	})
}

func ensureAASIdentifierTx(ctx context.Context, tx *sql.Tx, aasID string) (int64, error) {
	var aasRef int64
	if err := tx.QueryRowContext(
		ctx,
		`INSERT INTO aas_identifier (aasId) VALUES ($1)
		 ON CONFLICT (aasId) DO UPDATE SET aasId = EXCLUDED.aasId
		 RETURNING id`,
		aasID,
	).Scan(&aasRef); err != nil {
		return 0, err
	}
	return aasRef, nil
}
