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
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// BuildAdministrationShellDescriptorCreateBatch builds insert statements for one AAS descriptor.
//
// The returned batch contains the descriptor, payload, endpoint, specific asset
// id, discovery integration, and submodel descriptor insert statements in
// dependency order.
//
// Parameters:
//   - ctx: Request context carrying configuration and security data.
//   - descriptor: Asset Administration Shell descriptor to insert.
//
// Returns:
//   - *common.PostgreSQLBatch: Ordered insert batch.
//   - error: Error when statement rendering or reference payload rendering fails.
func BuildAdministrationShellDescriptorCreateBatch(
	ctx context.Context,
	descriptor model.AssetAdministrationShellDescriptor,
) (*common.PostgreSQLBatch, error) {
	batch := &common.PostgreSQLBatch{}
	descriptorID := common.PostgreSQLCurrentSequenceValue(common.TblDescriptor, common.ColID)

	if err := batch.AppendDataset(goqu.Insert(common.TblDescriptor).Rows(goqu.Record{})); err != nil {
		return nil, err
	}
	if err := appendDescriptorPayload(batch, descriptorID, descriptor.Description, descriptor.DisplayName, descriptor.Administration, descriptor.Extensions); err != nil {
		return nil, err
	}
	if err := batch.AppendDataset(goqu.Insert(common.TblAASDescriptor).Rows(buildAASDescriptorInsertRecord(ctx, descriptorID, descriptor))); err != nil {
		return nil, err
	}
	if err := appendEndpoints(batch, descriptorID, descriptor.Endpoints); err != nil {
		return nil, err
	}

	specificAssetIDs := descriptor.SpecificAssetIds
	var aasRef any
	if cfg, ok := common.ConfigFromContext(ctx); ok && cfg.General.DiscoveryIntegration {
		if err := batch.AppendDataset(
			goqu.Insert(common.TblAASIdentifier).
				Rows(goqu.Record{"aasid": descriptor.Id}).
				OnConflict(goqu.DoUpdate("aasid", goqu.Record{"aasid": goqu.I("excluded.aasid")})),
		); err != nil {
			return nil, err
		}
		aasRef = goqu.From(common.TblAASIdentifier).
			Select(common.ColID).
			Where(goqu.C("aasid").Eq(descriptor.Id))
		if descriptor.GlobalAssetId != "" {
			specificAssetIDs = append(
				append([]types.ISpecificAssetID(nil), specificAssetIDs...),
				types.NewSpecificAssetID(globalAssetIDSpecificAssetIDName, descriptor.GlobalAssetId),
			)
		}
	}
	if err := batch.AppendSpecificAssetIDs(descriptorID, aasRef, specificAssetIDs); err != nil {
		return nil, err
	}
	storedAASDescriptorID := goqu.From(common.TblAASDescriptor).
		Select(common.ColDescriptorID).
		Where(goqu.C(common.ColAASID).Eq(descriptor.Id))
	if err := appendSubmodelDescriptors(batch, storedAASDescriptorID, descriptor.SubmodelDescriptors); err != nil {
		return nil, err
	}
	return batch, nil
}

func appendSubmodelDescriptors(
	batch *common.PostgreSQLBatch,
	aasDescriptorID any,
	descriptors []model.SubmodelDescriptor,
) error {
	for position, descriptor := range descriptors {
		if len(descriptor.Endpoints) == 0 {
			return common.NewErrBadRequest("AASDESC-PGBATCH-SMD-ENDPOINTS Submodel Descriptor needs at least 1 Endpoint.")
		}
		if err := batch.AppendDataset(goqu.Insert(common.TblDescriptor).Rows(goqu.Record{})); err != nil {
			return err
		}
		descriptorID := common.PostgreSQLCurrentSequenceValue(common.TblDescriptor, common.ColID)
		if err := batch.AppendDataset(goqu.Insert(common.TblSubmodelDescriptor).Rows(goqu.Record{
			common.ColDescriptorID:    descriptorID,
			common.ColPosition:        position,
			common.ColAASDescriptorID: aasDescriptorID,
			common.ColIDShort:         descriptor.IdShort,
			common.ColAASID:           descriptor.Id,
		})); err != nil {
			return err
		}
		if err := batch.AppendContextReference(
			descriptorID,
			descriptor.SemanticId,
			"submodel_descriptor_semantic_id_reference",
			"submodel_descriptor_semantic_id_reference_key",
		); err != nil {
			return err
		}
		if err := appendDescriptorPayload(batch, descriptorID, descriptor.Description, descriptor.DisplayName, descriptor.Administration, descriptor.Extensions); err != nil {
			return err
		}
		if err := batch.AppendContextReferences(
			descriptorID,
			descriptor.SupplementalSemanticId,
			common.TblSubmodelDescriptorSuppSemantic,
			common.ColDescriptorID,
		); err != nil {
			return err
		}
		if err := appendEndpoints(batch, descriptorID, descriptor.Endpoints); err != nil {
			return err
		}
	}
	return nil
}

func appendDescriptorPayload(
	batch *common.PostgreSQLBatch,
	descriptorID any,
	description []types.ILangStringTextType,
	displayName []types.ILangStringNameType,
	administration types.IAdministrativeInformation,
	extensions []types.Extension,
) error {
	descriptionPayload, err := buildLangStringTextPayload(description)
	if err != nil {
		return common.NewInternalServerError("AASDESC-PGBATCH-DESCRIPTIONPAYLOAD " + err.Error())
	}
	displayNamePayload, err := buildLangStringNamePayload(displayName)
	if err != nil {
		return common.NewInternalServerError("AASDESC-PGBATCH-DISPLAYNAMEPAYLOAD " + err.Error())
	}
	administrationPayload, err := buildAdministrativeInfoPayload(administration)
	if err != nil {
		return common.NewInternalServerError("AASDESC-PGBATCH-ADMINPAYLOAD " + err.Error())
	}
	extensionsPayload, err := buildExtensionsPayload(extensions)
	if err != nil {
		return common.NewInternalServerError("AASDESC-PGBATCH-EXTENSIONPAYLOAD " + err.Error())
	}
	return batch.AppendDataset(goqu.Insert(common.TblDescriptorPayload).Rows(goqu.Record{
		common.ColDescriptorID:              descriptorID,
		common.ColDescriptionPayload:        goqu.L("?::jsonb", string(descriptionPayload)),
		common.ColDisplayNamePayload:        goqu.L("?::jsonb", string(displayNamePayload)),
		common.ColAdministrativeInfoPayload: goqu.L("?::jsonb", string(administrationPayload)),
		common.ColExtensionsPayload:         goqu.L("?::jsonb", string(extensionsPayload)),
	}))
}

func appendEndpoints(batch *common.PostgreSQLBatch, descriptorID any, endpoints []model.Endpoint) error {
	if len(endpoints) == 0 {
		return nil
	}
	rows := make([]goqu.Record, 0, len(endpoints))
	for position, endpoint := range endpoints {
		versionsJSON, err := marshalProtocolVersions(endpoint.ProtocolInformation.EndpointProtocolVersion)
		if err != nil {
			return fmt.Errorf("AASDESC-PGBATCH-ENDPOINTVERSIONS %w", err)
		}
		securityJSON, err := marshalSecurityAttributes(endpoint.ProtocolInformation.SecurityAttributes)
		if err != nil {
			return fmt.Errorf("AASDESC-PGBATCH-ENDPOINTSECURITY %w", err)
		}
		rows = append(rows, goqu.Record{
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
	return batch.AppendDataset(goqu.Insert(common.TblAASDescriptorEndpoint).Rows(rows))
}
