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

package common

import (
	"database/sql"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
)

func CreateSpecificAssetIDDescriptor(tx *sql.Tx, descriptorID int64, aasRef sql.NullInt64, specificAssetIDs []types.ISpecificAssetID) error {
	return InsertSpecificAssetIDs(
		tx,
		sql.NullInt64{Int64: descriptorID, Valid: true},
		sql.NullInt64{},
		aasRef,
		specificAssetIDs,
	)
}

func CreateSpecificAssetIDForAssetInformation(
	tx *sql.Tx,
	assetInformationID int64,
	specificAssetIDs []types.ISpecificAssetID,
) error {
	return InsertSpecificAssetIDs(
		tx,
		sql.NullInt64{},
		sql.NullInt64{Int64: assetInformationID, Valid: true},
		sql.NullInt64{},
		specificAssetIDs,
	)
}

func InsertSpecificAssetIDs(
	tx *sql.Tx,
	descriptorID sql.NullInt64,
	assetInformationID sql.NullInt64,
	aasRef sql.NullInt64,
	specificAssetIDs []types.ISpecificAssetID,
) error {
	if descriptorID.Valid && assetInformationID.Valid {
		return fmt.Errorf("Insert into specific_asset_id: descriptor_id and asset_information_id must not both be set")
	}
	if specificAssetIDs == nil {
		return nil
	}
	if len(specificAssetIDs) > 0 {
		d := goqu.Dialect(Dialect)
		for i, val := range specificAssetIDs {
			var err error

			sqlStr, args, err := d.
				Insert(TblSpecificAssetID).
				Rows(goqu.Record{
					ColDescriptorID:       descriptorID,
					ColAssetInformationID: assetInformationID,
					ColPosition:           i,
					ColName:               val.Name(),
					ColValue:              val.Value(),
					ColAASRef:             aasRef,
				}).
				Returning(TSpecificAssetID.Col(ColID)).
				ToSQL()
			if err != nil {
				return err
			}
			var id int64
			if err = tx.QueryRow(sqlStr, args...).Scan(&id); err != nil {
				return err
			}

			if err = CreateContextReference(
				tx,
				id,
				val.ExternalSubjectID(),
				"specific_asset_id_external_subject_id_reference",
				"specific_asset_id_external_subject_id_reference_key",
			); err != nil {
				return err
			}

			if err = createSpecificAssetIDPayload(tx, id, val.SemanticID()); err != nil {
				return err
			}

			if err = createSpecificAssetIDSupplementalSemantic(tx, id, val.SupplementalSemanticIDs()); err != nil {
				return err
			}
		}
	}
	return nil
}

func createSpecificAssetIDPayload(tx *sql.Tx, specificAssetID int64, semanticID types.IReference) error {
	d := goqu.Dialect(Dialect)
	semanticPayload, err := buildReferencePayload(semanticID)
	if err != nil {
		return err
	}

	sqlStr, args, err := d.Insert(TblSpecificAssetIDPayload).Rows(goqu.Record{
		ColSpecificAssetID:    specificAssetID,
		"semantic_id_payload": goqu.L("?::jsonb", string(semanticPayload)),
	}).ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlStr, args...)
	return err
}

func createSpecificAssetIDSupplementalSemantic(tx *sql.Tx, specificAssetID int64, references []types.IReference) error {
	return CreateContextReferences1ToMany(
		tx,
		specificAssetID,
		references,
		TblSpecificAssetIDSuppSemantic,
		ColSpecificAssetIDID,
	)
}
