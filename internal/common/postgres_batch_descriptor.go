/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package common

import (
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
)

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
