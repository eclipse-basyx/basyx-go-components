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

package descriptors

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type bulkCreateRows struct {
	descriptor                      []goqu.Record
	descriptorPayload               []goqu.Record
	aasDescriptor                   []goqu.Record
	endpoint                        []goqu.Record
	aasIdentifier                   []goqu.Record
	specificAssetID                 []goqu.Record
	specificAssetIDPayload          []goqu.Record
	externalSubjectReference        []goqu.Record
	externalSubjectReferencePayload []goqu.Record
	externalSubjectReferenceKey     []goqu.Record
	specificSupplementalReference   []goqu.Record
	specificSupplementalPayload     []goqu.Record
	specificSupplementalKey         []goqu.Record
	submodelDescriptor              []goqu.Record
	submodelSemanticReference       []goqu.Record
	submodelSemanticPayload         []goqu.Record
	submodelSemanticKey             []goqu.Record
	submodelSupplementalReference   []goqu.Record
	submodelSupplementalPayload     []goqu.Record
	submodelSupplementalKey         []goqu.Record
}

type bulkCreateIDs struct {
	descriptor                    []int64
	specificAssetID               []int64
	specificSupplementalReference []int64
	submodelSupplementalReference []int64
}

type bulkCreateIDCursor struct {
	ids                       bulkCreateIDs
	descriptorIndex           int
	specificAssetIDIndex      int
	specificSupplementalIndex int
	submodelSupplementalIndex int
}

// BuildAdministrationShellDescriptorsCreateBatch builds bulk insert statements for AAS descriptors.
//
// The function reserves database ids in the provided transaction and renders
// table-oriented insert statements in dependency order.
//
// Parameters:
//   - ctx: Request context carrying configuration and security data.
//   - tx: Transaction used to reserve database ids.
//   - descriptors: Asset Administration Shell descriptors to insert.
//
// Returns:
//   - *common.PostgreSQLBatch: Ordered bulk insert batch.
//   - error: Error when id reservation, row collection, or statement rendering fails.
func BuildAdministrationShellDescriptorsCreateBatch(
	ctx context.Context,
	tx *sql.Tx,
	descriptors []model.AssetAdministrationShellDescriptor,
) (*common.PostgreSQLBatch, error) {
	counts := countBulkCreateIDs(ctx, descriptors)
	ids, err := reserveBulkCreateIDs(ctx, tx, counts)
	if err != nil {
		return nil, err
	}

	rows := &bulkCreateRows{}
	cursor := &bulkCreateIDCursor{ids: ids}
	for _, descriptor := range descriptors {
		if err = collectAASDescriptorRows(ctx, rows, cursor, descriptor); err != nil {
			return nil, err
		}
	}
	if err = cursor.validateConsumed(); err != nil {
		return nil, err
	}

	batch := &common.PostgreSQLBatch{}
	if err = appendBulkCreateRows(ctx, batch, rows); err != nil {
		return nil, err
	}
	return batch, nil
}

// BuildSubmodelDescriptorsCreateBatch builds bulk insert statements for submodel descriptors.
//
// The function reserves database ids in the provided transaction and renders
// table-oriented insert statements in dependency order.
//
// Parameters:
//   - ctx: Request context carrying configuration and batch settings.
//   - tx: Transaction used to reserve database ids.
//   - descriptors: Global submodel descriptors to insert.
//
// Returns:
//   - *common.PostgreSQLBatch: Ordered bulk insert batch.
//   - error: Error when id reservation, row collection, or statement rendering fails.
func BuildSubmodelDescriptorsCreateBatch(
	ctx context.Context,
	tx *sql.Tx,
	descriptors []model.SubmodelDescriptor,
) (*common.PostgreSQLBatch, error) {
	counts := countBulkSubmodelCreateIDs(descriptors)
	ids, err := reserveBulkCreateIDs(ctx, tx, counts)
	if err != nil {
		return nil, err
	}

	rows := &bulkCreateRows{}
	cursor := &bulkCreateIDCursor{ids: ids}
	for position, descriptor := range descriptors {
		if err = collectSubmodelDescriptorRows(rows, cursor, nil, position, descriptor); err != nil {
			return nil, err
		}
	}
	if err = cursor.validateConsumed(); err != nil {
		return nil, err
	}

	batch := &common.PostgreSQLBatch{}
	if err = appendBulkCreateRows(ctx, batch, rows); err != nil {
		return nil, err
	}
	return batch, nil
}

func countBulkCreateIDs(
	ctx context.Context,
	descriptors []model.AssetAdministrationShellDescriptor,
) bulkCreateIDs {
	var counts bulkCreateIDs
	discoveryIntegration := false
	if cfg, ok := common.ConfigFromContext(ctx); ok {
		discoveryIntegration = cfg.General.DiscoveryIntegration
	}
	for _, descriptor := range descriptors {
		counts.descriptor = append(counts.descriptor, 0)
		specificAssetIDCount := len(descriptor.SpecificAssetIds)
		if discoveryIntegration && descriptor.GlobalAssetId != "" {
			specificAssetIDCount++
		}
		counts.specificAssetID = append(counts.specificAssetID, make([]int64, specificAssetIDCount)...)
		for _, assetID := range descriptor.SpecificAssetIds {
			counts.specificSupplementalReference = append(
				counts.specificSupplementalReference,
				make([]int64, countNonNilReferences(assetID.SupplementalSemanticIDs()))...,
			)
		}
		for _, submodel := range descriptor.SubmodelDescriptors {
			counts.descriptor = append(counts.descriptor, 0)
			counts.submodelSupplementalReference = append(
				counts.submodelSupplementalReference,
				make([]int64, countNonNilReferences(submodel.SupplementalSemanticId))...,
			)
		}
	}
	return counts
}

func countBulkSubmodelCreateIDs(descriptors []model.SubmodelDescriptor) bulkCreateIDs {
	var counts bulkCreateIDs
	for _, descriptor := range descriptors {
		counts.descriptor = append(counts.descriptor, 0)
		counts.submodelSupplementalReference = append(
			counts.submodelSupplementalReference,
			make([]int64, countNonNilReferences(descriptor.SupplementalSemanticId))...,
		)
	}
	return counts
}

func countNonNilReferences(references []types.IReference) int {
	count := 0
	for _, reference := range references {
		if reference != nil {
			count++
		}
	}
	return count
}

func reserveBulkCreateIDs(ctx context.Context, tx *sql.Tx, counts bulkCreateIDs) (bulkCreateIDs, error) {
	var ids bulkCreateIDs
	var err error
	if ids.descriptor, err = reserveSequenceIDs(ctx, tx, common.TblDescriptor, common.ColID, len(counts.descriptor)); err != nil {
		return bulkCreateIDs{}, err
	}
	if ids.specificAssetID, err = reserveSequenceIDs(ctx, tx, common.TblSpecificAssetID, common.ColID, len(counts.specificAssetID)); err != nil {
		return bulkCreateIDs{}, err
	}
	if ids.specificSupplementalReference, err = reserveSequenceIDs(ctx, tx, common.TblSpecificAssetIDSuppSemantic, common.ColID, len(counts.specificSupplementalReference)); err != nil {
		return bulkCreateIDs{}, err
	}
	if ids.submodelSupplementalReference, err = reserveSequenceIDs(ctx, tx, common.TblSubmodelDescriptorSuppSemantic, common.ColID, len(counts.submodelSupplementalReference)); err != nil {
		return bulkCreateIDs{}, err
	}
	return ids, nil
}

func reserveSequenceIDs(ctx context.Context, tx *sql.Tx, table string, column string, count int) ([]int64, error) {
	if count == 0 {
		return nil, nil
	}
	if tx == nil {
		return nil, common.NewInternalServerError("AASDESC-BULKRESERVE-NILTX transaction must not be nil")
	}
	query, args, err := goqu.
		From(goqu.Func("generate_series", 1, count)).
		Select(goqu.Func("nextval", goqu.Func("pg_get_serial_sequence", table, column))).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("AASDESC-BULKRESERVE-BUILDSQL " + err.Error())
	}
	resultRows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("AASDESC-BULKRESERVE-EXECQUERY " + err.Error())
	}
	defer func() {
		_ = resultRows.Close()
	}()

	ids := make([]int64, 0, count)
	for resultRows.Next() {
		var id int64
		if scanErr := resultRows.Scan(&id); scanErr != nil {
			return nil, common.NewInternalServerError("AASDESC-BULKRESERVE-SCANID " + scanErr.Error())
		}
		ids = append(ids, id)
	}
	if err = resultRows.Err(); err != nil {
		return nil, common.NewInternalServerError("AASDESC-BULKRESERVE-ITERATE " + err.Error())
	}
	if len(ids) != count {
		return nil, common.NewInternalServerError("AASDESC-BULKRESERVE-IDCOUNT unexpected reserved id count")
	}
	return ids, nil
}

func collectAASDescriptorRows(
	ctx context.Context,
	rows *bulkCreateRows,
	cursor *bulkCreateIDCursor,
	descriptor model.AssetAdministrationShellDescriptor,
) error {
	descriptorID, err := cursor.nextDescriptorID()
	if err != nil {
		return err
	}
	rows.descriptor = append(rows.descriptor, goqu.Record{common.ColID: descriptorID})
	payload, err := buildDescriptorPayloadRecord(descriptorID, descriptor.Description, descriptor.DisplayName, descriptor.Administration, descriptor.Extensions)
	if err != nil {
		return err
	}
	rows.descriptorPayload = append(rows.descriptorPayload, payload)
	aasDescriptorRecord := buildAASDescriptorInsertRecord(ctx, descriptorID, descriptor)
	if _, hasCreatedAt := aasDescriptorRecord[common.ColCreatedAt]; !hasCreatedAt {
		aasDescriptorRecord[common.ColCreatedAt] = goqu.Default()
	}
	rows.aasDescriptor = append(rows.aasDescriptor, aasDescriptorRecord)
	if err = collectEndpointRows(rows, descriptorID, descriptor.Endpoints); err != nil {
		return err
	}
	if err = collectSpecificAssetIDRows(ctx, rows, cursor, descriptorID, descriptor); err != nil {
		return err
	}
	for position, submodel := range descriptor.SubmodelDescriptors {
		if err = collectSubmodelDescriptorRows(rows, cursor, descriptorID, position, submodel); err != nil {
			return err
		}
	}
	return nil
}

func collectSpecificAssetIDRows(
	ctx context.Context,
	rows *bulkCreateRows,
	cursor *bulkCreateIDCursor,
	descriptorID int64,
	descriptor model.AssetAdministrationShellDescriptor,
) error {
	assetIDs := specificAssetIDsWithGlobalAssetID(ctx, descriptor)
	var aasRef any
	if cfg, ok := common.ConfigFromContext(ctx); ok && cfg.General.DiscoveryIntegration {
		rows.aasIdentifier = append(rows.aasIdentifier, goqu.Record{"aasid": descriptor.Id})
		aasRef = goqu.From(common.TblAASIdentifier).Select(common.ColID).Where(goqu.C("aasid").Eq(descriptor.Id))
	}
	for position, assetID := range assetIDs {
		specificAssetID, err := cursor.nextSpecificAssetID()
		if err != nil {
			return err
		}
		rows.specificAssetID = append(rows.specificAssetID, goqu.Record{
			common.ColID:                 specificAssetID,
			common.ColDescriptorID:       descriptorID,
			common.ColAssetInformationID: sql.NullInt64{},
			common.ColPosition:           position,
			common.ColName:               assetID.Name(),
			common.ColValue:              assetID.Value(),
			common.ColAASRef:             aasRef,
		})
		if err := collectSpecificAssetIDReferenceRows(rows, cursor, specificAssetID, assetID); err != nil {
			return err
		}
	}
	return nil
}

func collectSpecificAssetIDReferenceRows(
	rows *bulkCreateRows,
	cursor *bulkCreateIDCursor,
	specificAssetID int64,
	assetID types.ISpecificAssetID,
) error {
	if err := collectOwnedReferenceRows(
		specificAssetID,
		assetID.ExternalSubjectID(),
		&rows.externalSubjectReference,
		&rows.externalSubjectReferencePayload,
		&rows.externalSubjectReferenceKey,
	); err != nil {
		return err
	}
	payloadRecord := goqu.Record{
		common.ColSpecificAssetID: specificAssetID,
		"semantic_id_payload":     goqu.Default(),
	}
	if assetID.SemanticID() != nil {
		payload, err := common.BuildReferencePayload(assetID.SemanticID())
		if err != nil {
			return err
		}
		payloadRecord["semantic_id_payload"] = goqu.L("?::jsonb", string(payload))
	}
	rows.specificAssetIDPayload = append(rows.specificAssetIDPayload, payloadRecord)
	for position, reference := range assetID.SupplementalSemanticIDs() {
		if reference == nil {
			continue
		}
		referenceID, err := cursor.nextSpecificSupplementalID()
		if err != nil {
			return err
		}
		if err := collectGeneratedReferenceRows(
			referenceID,
			specificAssetID,
			common.ColSpecificAssetIDID,
			position,
			reference,
			&rows.specificSupplementalReference,
			&rows.specificSupplementalPayload,
			&rows.specificSupplementalKey,
		); err != nil {
			return err
		}
	}
	return nil
}

func collectSubmodelDescriptorRows(
	rows *bulkCreateRows,
	cursor *bulkCreateIDCursor,
	aasDescriptorID any,
	position int,
	descriptor model.SubmodelDescriptor,
) error {
	if len(descriptor.Endpoints) == 0 {
		return common.NewErrBadRequest("AASDESC-BULK-SMD-ENDPOINTS Submodel Descriptor needs at least 1 Endpoint.")
	}
	descriptorID, err := cursor.nextDescriptorID()
	if err != nil {
		return err
	}
	rows.descriptor = append(rows.descriptor, goqu.Record{common.ColID: descriptorID})
	rows.submodelDescriptor = append(rows.submodelDescriptor, goqu.Record{
		common.ColDescriptorID:    descriptorID,
		common.ColPosition:        position,
		common.ColAASDescriptorID: aasDescriptorID,
		common.ColIDShort:         descriptor.IdShort,
		common.ColAASID:           descriptor.Id,
	})
	payload, err := buildDescriptorPayloadRecord(descriptorID, descriptor.Description, descriptor.DisplayName, descriptor.Administration, descriptor.Extensions)
	if err != nil {
		return err
	}
	rows.descriptorPayload = append(rows.descriptorPayload, payload)
	if err = collectOwnedReferenceRows(
		descriptorID,
		descriptor.SemanticId,
		&rows.submodelSemanticReference,
		&rows.submodelSemanticPayload,
		&rows.submodelSemanticKey,
	); err != nil {
		return err
	}
	for position, reference := range descriptor.SupplementalSemanticId {
		if reference == nil {
			continue
		}
		referenceID, err := cursor.nextSubmodelSupplementalID()
		if err != nil {
			return err
		}
		if err = collectGeneratedReferenceRows(
			referenceID,
			descriptorID,
			common.ColDescriptorID,
			position,
			reference,
			&rows.submodelSupplementalReference,
			&rows.submodelSupplementalPayload,
			&rows.submodelSupplementalKey,
		); err != nil {
			return err
		}
	}
	return collectEndpointRows(rows, descriptorID, descriptor.Endpoints)
}

func buildDescriptorPayloadRecord(
	descriptorID int64,
	description []types.ILangStringTextType,
	displayName []types.ILangStringNameType,
	administration types.IAdministrativeInformation,
	extensions []types.Extension,
) (goqu.Record, error) {
	descriptionPayload, err := buildLangStringTextPayload(description)
	if err != nil {
		return nil, common.NewInternalServerError("AASDESC-BULK-DESCRIPTIONPAYLOAD " + err.Error())
	}
	displayNamePayload, err := buildLangStringNamePayload(displayName)
	if err != nil {
		return nil, common.NewInternalServerError("AASDESC-BULK-DISPLAYNAMEPAYLOAD " + err.Error())
	}
	administrationPayload, err := buildAdministrativeInfoPayload(administration)
	if err != nil {
		return nil, common.NewInternalServerError("AASDESC-BULK-ADMINPAYLOAD " + err.Error())
	}
	extensionsPayload, err := buildExtensionsPayload(extensions)
	if err != nil {
		return nil, common.NewInternalServerError("AASDESC-BULK-EXTENSIONPAYLOAD " + err.Error())
	}
	return goqu.Record{
		common.ColDescriptorID:              descriptorID,
		common.ColDescriptionPayload:        goqu.L("?::jsonb", string(descriptionPayload)),
		common.ColDisplayNamePayload:        goqu.L("?::jsonb", string(displayNamePayload)),
		common.ColAdministrativeInfoPayload: goqu.L("?::jsonb", string(administrationPayload)),
		common.ColExtensionsPayload:         goqu.L("?::jsonb", string(extensionsPayload)),
	}, nil
}

func collectEndpointRows(rows *bulkCreateRows, descriptorID int64, endpoints []model.Endpoint) error {
	for position, endpoint := range endpoints {
		versionsJSON, err := marshalProtocolVersions(endpoint.ProtocolInformation.EndpointProtocolVersion)
		if err != nil {
			return fmt.Errorf("AASDESC-BULK-ENDPOINTVERSIONS %w", err)
		}
		securityJSON, err := marshalSecurityAttributes(endpoint.ProtocolInformation.SecurityAttributes)
		if err != nil {
			return fmt.Errorf("AASDESC-BULK-ENDPOINTSECURITY %w", err)
		}
		rows.endpoint = append(rows.endpoint, goqu.Record{
			common.ColDescriptorID:            descriptorID,
			common.ColPosition:                position,
			common.ColHref:                    endpoint.ProtocolInformation.Href,
			common.ColEndpointProtocol:        endpoint.ProtocolInformation.EndpointProtocol,
			common.ColEndpointProtocolVersion: goqu.L("?::jsonb", versionsJSON),
			common.ColSubProtocol:             endpoint.ProtocolInformation.Subprotocol,
			common.ColSubProtocolBody:         endpoint.ProtocolInformation.SubprotocolBody,
			common.ColSubProtocolBodyEncoding: endpoint.ProtocolInformation.SubprotocolBodyEncoding,
			common.ColSecurityAttributes:      goqu.L("?::jsonb", securityJSON),
			common.ColInterface:               endpoint.Interface,
		})
	}
	return nil
}

func collectOwnedReferenceRows(
	ownerID int64,
	reference types.IReference,
	referenceRows *[]goqu.Record,
	payloadRows *[]goqu.Record,
	keyRows *[]goqu.Record,
) error {
	if reference == nil {
		return nil
	}
	*referenceRows = append(*referenceRows, goqu.Record{common.ColID: ownerID, common.ColType: reference.Type()})
	return collectReferencePayloadAndKeys(ownerID, reference, payloadRows, keyRows)
}

func collectGeneratedReferenceRows(
	referenceID int64,
	ownerID int64,
	ownerColumn string,
	position int,
	reference types.IReference,
	referenceRows *[]goqu.Record,
	payloadRows *[]goqu.Record,
	keyRows *[]goqu.Record,
) error {
	*referenceRows = append(*referenceRows, goqu.Record{
		common.ColID:       referenceID,
		ownerColumn:        ownerID,
		common.ColPosition: position,
		common.ColType:     reference.Type(),
	})
	return collectReferencePayloadAndKeys(referenceID, reference, payloadRows, keyRows)
}

func collectReferencePayloadAndKeys(
	referenceID int64,
	reference types.IReference,
	payloadRows *[]goqu.Record,
	keyRows *[]goqu.Record,
) error {
	parentPayload, err := common.BuildReferencePayload(reference.ReferredSemanticID())
	if err != nil {
		return err
	}
	*payloadRows = append(*payloadRows, goqu.Record{
		common.ColReferenceID:      referenceID,
		"parent_reference_payload": goqu.L("?::jsonb", string(parentPayload)),
	})
	for position, key := range reference.Keys() {
		*keyRows = append(*keyRows, goqu.Record{
			common.ColReferenceID: referenceID,
			common.ColPosition:    position,
			common.ColType:        key.Type(),
			common.ColValue:       key.Value(),
		})
	}
	return nil
}

func appendBulkCreateRows(ctx context.Context, batch *common.PostgreSQLBatch, rows *bulkCreateRows) error {
	tableRows := []struct {
		table      string
		rows       []goqu.Record
		onConflict exp.ConflictExpression
	}{
		{common.TblDescriptor, rows.descriptor, nil},
		{common.TblDescriptorPayload, rows.descriptorPayload, nil},
		{common.TblAASDescriptor, rows.aasDescriptor, nil},
		{common.TblAASIdentifier, rows.aasIdentifier, aasIdentifierConflict()},
		{common.TblSpecificAssetID, rows.specificAssetID, nil},
		{common.TblSpecificAssetIDPayload, rows.specificAssetIDPayload, nil},
		{"specific_asset_id_external_subject_id_reference", rows.externalSubjectReference, nil},
		{"specific_asset_id_external_subject_id_reference_payload", rows.externalSubjectReferencePayload, nil},
		{"specific_asset_id_external_subject_id_reference_key", rows.externalSubjectReferenceKey, nil},
		{common.TblSpecificAssetIDSuppSemantic, rows.specificSupplementalReference, nil},
		{common.TblSpecificAssetIDSuppSemantic + "_payload", rows.specificSupplementalPayload, nil},
		{common.TblSpecificAssetIDSuppSemantic + "_key", rows.specificSupplementalKey, nil},
		{common.TblSubmodelDescriptor, rows.submodelDescriptor, nil},
		{"submodel_descriptor_semantic_id_reference", rows.submodelSemanticReference, nil},
		{"submodel_descriptor_semantic_id_reference_payload", rows.submodelSemanticPayload, nil},
		{"submodel_descriptor_semantic_id_reference_key", rows.submodelSemanticKey, nil},
		{common.TblSubmodelDescriptorSuppSemantic, rows.submodelSupplementalReference, nil},
		{common.TblSubmodelDescriptorSuppSemantic + "_payload", rows.submodelSupplementalPayload, nil},
		{common.TblSubmodelDescriptorSuppSemantic + "_key", rows.submodelSupplementalKey, nil},
		{common.TblAASDescriptorEndpoint, rows.endpoint, nil},
	}
	limit := common.BulkBatchLimitFromContext(ctx)
	for _, entry := range tableRows {
		if err := appendChunkedRows(batch, entry.table, entry.rows, entry.onConflict, limit); err != nil {
			return err
		}
	}
	return nil
}

func appendChunkedRows(
	batch *common.PostgreSQLBatch,
	table string,
	rows []goqu.Record,
	conflict exp.ConflictExpression,
	limit int,
) error {
	if limit <= 0 {
		limit = common.DefaultConfig.GeneralBulkBatchLimit
	}
	for start := 0; start < len(rows); start += limit {
		end := min(start+limit, len(rows))
		insert := goqu.Insert(table).Rows(rows[start:end])
		if conflict != nil {
			insert = insert.OnConflict(conflict)
		}
		if err := batch.AppendDataset(insert); err != nil {
			return common.NewInternalServerError("AASDESC-BULK-BUILDINSERT " + err.Error())
		}
	}
	return nil
}

func aasIdentifierConflict() exp.ConflictExpression {
	return goqu.DoUpdate("aasid", goqu.Record{"aasid": goqu.I("excluded.aasid")})
}

func (c *bulkCreateIDCursor) nextDescriptorID() (int64, error) {
	if c.descriptorIndex >= len(c.ids.descriptor) {
		return 0, common.NewInternalServerError("AASDESC-BULK-CURSOR-DESCRIPTOR exhausted reserved descriptor ids")
	}
	id := c.ids.descriptor[c.descriptorIndex]
	c.descriptorIndex++
	return id, nil
}

func (c *bulkCreateIDCursor) nextSpecificAssetID() (int64, error) {
	if c.specificAssetIDIndex >= len(c.ids.specificAssetID) {
		return 0, common.NewInternalServerError("AASDESC-BULK-CURSOR-SPECIFICASSETID exhausted reserved specific asset id ids")
	}
	id := c.ids.specificAssetID[c.specificAssetIDIndex]
	c.specificAssetIDIndex++
	return id, nil
}

func (c *bulkCreateIDCursor) nextSpecificSupplementalID() (int64, error) {
	if c.specificSupplementalIndex >= len(c.ids.specificSupplementalReference) {
		return 0, common.NewInternalServerError("AASDESC-BULK-CURSOR-SPECIFICSUPP exhausted reserved specific asset supplemental reference ids")
	}
	id := c.ids.specificSupplementalReference[c.specificSupplementalIndex]
	c.specificSupplementalIndex++
	return id, nil
}

func (c *bulkCreateIDCursor) nextSubmodelSupplementalID() (int64, error) {
	if c.submodelSupplementalIndex >= len(c.ids.submodelSupplementalReference) {
		return 0, common.NewInternalServerError("AASDESC-BULK-CURSOR-SMSUPP exhausted reserved submodel supplemental reference ids")
	}
	id := c.ids.submodelSupplementalReference[c.submodelSupplementalIndex]
	c.submodelSupplementalIndex++
	return id, nil
}

func (c *bulkCreateIDCursor) validateConsumed() error {
	if c.descriptorIndex != len(c.ids.descriptor) {
		return common.NewInternalServerError("AASDESC-BULK-CURSOR-DESCRIPTOR unused reserved descriptor ids")
	}
	if c.specificAssetIDIndex != len(c.ids.specificAssetID) {
		return common.NewInternalServerError("AASDESC-BULK-CURSOR-SPECIFICASSETID unused reserved specific asset id ids")
	}
	if c.specificSupplementalIndex != len(c.ids.specificSupplementalReference) {
		return common.NewInternalServerError("AASDESC-BULK-CURSOR-SPECIFICSUPP unused reserved specific asset supplemental reference ids")
	}
	if c.submodelSupplementalIndex != len(c.ids.submodelSupplementalReference) {
		return common.NewInternalServerError("AASDESC-BULK-CURSOR-SMSUPP unused reserved submodel supplemental reference ids")
	}
	return nil
}
