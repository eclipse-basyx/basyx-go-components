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

package common

import (
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
)

// AppendContextReference appends statements for one reference whose primary key
// is the owning row ID. It writes the reference row, its payload row, and all
// reference key rows to the provided reference tables.
func (b *PostgreSQLBatch) AppendContextReference(
	ownerID any,
	reference types.IReference,
	referenceTable string,
	referenceKeyTable string,
) error {
	if reference == nil {
		return nil
	}

	dialect := goqu.Dialect(Dialect)
	if err := b.AppendDataset(dialect.Insert(referenceTable).Rows(goqu.Record{
		ColID:   ownerID,
		ColType: reference.Type(),
	})); err != nil {
		return err
	}

	parentPayload, err := buildReferencePayload(reference.ReferredSemanticID())
	if err != nil {
		return err
	}
	if err = b.AppendDataset(dialect.Insert(referenceTable + "_payload").Rows(goqu.Record{
		ColReferenceID:             ownerID,
		"parent_reference_payload": goqu.L("?::jsonb", string(parentPayload)),
	})); err != nil {
		return err
	}

	keys := reference.Keys()
	if len(keys) == 0 {
		return nil
	}
	rows := make([]goqu.Record, 0, len(keys))
	for position, key := range keys {
		rows = append(rows, goqu.Record{
			ColReferenceID: ownerID,
			ColPosition:    position,
			ColType:        key.Type(),
			ColValue:       key.Value(),
		})
	}
	return b.AppendDataset(dialect.Insert(referenceKeyTable).Rows(rows))
}

// AppendContextReferences appends statements for references that get generated
// IDs from referenceTable. Each reference is linked to ownerID through
// ownerColumn and includes payload and key rows.
func (b *PostgreSQLBatch) AppendContextReferences(
	ownerID any,
	references []types.IReference,
	referenceTable string,
	ownerColumn string,
) error {
	for _, reference := range references {
		if reference == nil {
			continue
		}
		dialect := goqu.Dialect(Dialect)
		if err := b.AppendDataset(dialect.Insert(referenceTable).Rows(goqu.Record{
			ownerColumn: ownerID,
			ColType:     reference.Type(),
		})); err != nil {
			return err
		}

		referenceID := PostgreSQLCurrentSequenceValue(referenceTable, ColID)
		parentPayload, err := buildReferencePayload(reference.ReferredSemanticID())
		if err != nil {
			return err
		}
		if err = b.AppendDataset(dialect.Insert(referenceTable + "_payload").Rows(goqu.Record{
			ColReferenceID:             referenceID,
			"parent_reference_payload": goqu.L("?::jsonb", string(parentPayload)),
		})); err != nil {
			return err
		}

		keys := reference.Keys()
		if len(keys) == 0 {
			continue
		}
		rows := make([]goqu.Record, 0, len(keys))
		for position, key := range keys {
			rows = append(rows, goqu.Record{
				ColReferenceID: referenceID,
				ColPosition:    position,
				ColType:        key.Type(),
				ColValue:       key.Value(),
			})
		}
		if err = b.AppendDataset(dialect.Insert(referenceTable + "_key").Rows(rows)); err != nil {
			return err
		}
	}
	return nil
}

// AppendSpecificAssetIDs appends statements for descriptor specific asset IDs
// and their dependent semantic, supplemental semantic, and external subject
// references.
func (b *PostgreSQLBatch) AppendSpecificAssetIDs(
	descriptorID any,
	aasRef any,
	specificAssetIDs []types.ISpecificAssetID,
) error {
	dialect := goqu.Dialect(Dialect)
	for position, assetID := range specificAssetIDs {
		if err := b.AppendDataset(dialect.Insert(TblSpecificAssetID).Rows(goqu.Record{
			ColDescriptorID:       descriptorID,
			ColAssetInformationID: sql.NullInt64{},
			ColPosition:           position,
			ColName:               assetID.Name(),
			ColValue:              assetID.Value(),
			ColAASRef:             aasRef,
		})); err != nil {
			return err
		}

		specificAssetID := PostgreSQLCurrentSequenceValue(TblSpecificAssetID, ColID)
		if err := b.AppendContextReference(
			specificAssetID,
			assetID.ExternalSubjectID(),
			"specific_asset_id_external_subject_id_reference",
			"specific_asset_id_external_subject_id_reference_key",
		); err != nil {
			return err
		}

		payloadRecord := goqu.Record{ColSpecificAssetID: specificAssetID}
		if assetID.SemanticID() != nil {
			payload, err := buildReferencePayload(assetID.SemanticID())
			if err != nil {
				return err
			}
			payloadRecord["semantic_id_payload"] = goqu.L("?::jsonb", string(payload))
		}
		if err := b.AppendDataset(dialect.Insert(TblSpecificAssetIDPayload).Rows(payloadRecord)); err != nil {
			return err
		}
		if err := b.AppendContextReferences(
			specificAssetID,
			assetID.SupplementalSemanticIDs(),
			TblSpecificAssetIDSuppSemantic,
			ColSpecificAssetIDID,
		); err != nil {
			return err
		}
	}
	return nil
}
