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
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const submodelDescriptorSemanticIDTable = "submodel_descriptor_semantic_id_reference"

type administrationShellDescriptorUpdatePlan struct {
	root                bool
	payload             descriptorPayloadUpdatePlan
	endpoints           bool
	specificAssetIDs    bool
	submodelDescriptors bool
}

func (p administrationShellDescriptorUpdatePlan) changed() bool {
	return p.root || p.payload.changed() || p.endpoints || p.specificAssetIDs || p.submodelDescriptors
}

type submodelDescriptorUpdatePlan struct {
	root                 bool
	payload              descriptorPayloadUpdatePlan
	endpoints            bool
	semanticID           bool
	supplementalSemantic bool
}

func (p submodelDescriptorUpdatePlan) changed() bool {
	return p.root || p.payload.changed() || p.endpoints || p.semanticID || p.supplementalSemantic
}

type descriptorPayloadUpdatePlan struct {
	description    bool
	displayName    bool
	administration bool
	extensions     bool
}

func (p descriptorPayloadUpdatePlan) changed() bool {
	return p.description || p.displayName || p.administration || p.extensions
}

// LockAdministrationShellDescriptorForUpdateTx locks an existing AAS
// descriptor and returns its stable internal descriptor id.
func LockAdministrationShellDescriptorForUpdateTx(ctx context.Context, tx *sql.Tx, aasID string) (int64, error) {
	if err := lockAASDescriptorUpsertTx(ctx, tx, aasID); err != nil {
		return 0, err
	}
	descriptorID, found, err := selectAASDescriptorIDForUpdateTx(ctx, tx, aasID)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, common.NewErrNotFound("AAS Descriptor not found")
	}
	return descriptorID, nil
}

// LockGlobalSubmodelDescriptorForUpdateTx locks an existing global Submodel
// descriptor and returns its stable internal descriptor id.
func LockGlobalSubmodelDescriptorForUpdateTx(ctx context.Context, tx *sql.Tx, submodelID string) (int64, error) {
	d := goqu.Dialect(common.Dialect)
	submodelDescriptor := goqu.T(common.TblSubmodelDescriptor)
	query, args, err := d.
		From(submodelDescriptor).
		Select(submodelDescriptor.Col(common.ColDescriptorID)).
		Where(
			submodelDescriptor.Col(common.ColAASID).Eq(submodelID),
			submodelDescriptor.Col(common.ColAASDescriptorID).IsNull(),
		).
		ForUpdate(goqu.Wait).
		Prepared(true).
		ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("SMDESC-UPDATE-LOCK-BUILDQ " + err.Error())
	}
	var descriptorID int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&descriptorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, common.NewErrNotFound("Submodel Descriptor not found")
		}
		return 0, common.NewInternalServerError("SMDESC-UPDATE-LOCK-EXECQ " + err.Error())
	}
	return descriptorID, nil
}

// LockSubmodelDescriptorForAASUpdateTx locks an embedded Submodel descriptor
// and returns its stable internal descriptor id and list position.
func LockSubmodelDescriptorForAASUpdateTx(
	ctx context.Context,
	tx *sql.Tx,
	aasID string,
	submodelID string,
) (int64, int, error) {
	d := goqu.Dialect(common.Dialect)
	aasDescriptor := goqu.T(common.TblAASDescriptor).As("aas")
	submodelDescriptor := goqu.T(common.TblSubmodelDescriptor).As("smd")
	query, args, err := d.
		From(submodelDescriptor).
		InnerJoin(
			aasDescriptor,
			goqu.On(submodelDescriptor.Col(common.ColAASDescriptorID).Eq(aasDescriptor.Col(common.ColDescriptorID))),
		).
		Select(
			submodelDescriptor.Col(common.ColDescriptorID),
			submodelDescriptor.Col(common.ColPosition),
		).
		Where(
			aasDescriptor.Col(common.ColAASID).Eq(aasID),
			submodelDescriptor.Col(common.ColAASID).Eq(submodelID),
		).
		Order(submodelDescriptor.Col(common.ColPosition).Asc()).
		Limit(1).
		ForUpdate(goqu.Wait).
		Prepared(true).
		ToSQL()
	if err != nil {
		return 0, 0, common.NewInternalServerError("SMDESC-UPDATE-EMBEDDEDLOCK-BUILDQ " + err.Error())
	}
	var descriptorID int64
	var position int
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&descriptorID, &position); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, common.NewErrNotFound("Submodel Descriptor not found")
		}
		return 0, 0, common.NewInternalServerError("SMDESC-UPDATE-EMBEDDEDLOCK-EXECQ " + err.Error())
	}
	return descriptorID, position, nil
}

// UpdateAdministrationShellDescriptorTx applies full PUT semantics while
// preserving unchanged rows of the normalized descriptor graph.
func UpdateAdministrationShellDescriptorTx(
	ctx context.Context,
	tx *sql.Tx,
	descriptorID int64,
	previous model.AssetAdministrationShellDescriptor,
	next model.AssetAdministrationShellDescriptor,
) (bool, error) {
	if previous.Id != next.Id {
		return false, common.NewErrBadRequest("AASDESC-UPDATE-IDMISMATCH descriptor ids do not match")
	}
	plan, err := buildAdministrationShellDescriptorUpdatePlan(ctx, previous, next)
	if err != nil {
		return false, err
	}
	if !plan.changed() {
		return false, nil
	}
	if err = updateAASDescriptorRowTx(ctx, tx, descriptorID, next); err != nil {
		return false, common.NewInternalServerError("AASDESC-UPDATE-ROOT " + err.Error())
	}
	if plan.payload.changed() {
		if err = updateAdministrationShellDescriptorPayloadTx(ctx, tx, descriptorID, next, plan.payload); err != nil {
			return false, err
		}
	}
	if plan.endpoints {
		if err = replaceDescriptorEndpointsTx(ctx, tx, descriptorID, next.Endpoints); err != nil {
			return false, err
		}
	}
	if plan.specificAssetIDs {
		if err = replaceAdministrationShellDescriptorAssetIDsTx(ctx, tx, descriptorID, next); err != nil {
			return false, err
		}
	}
	if plan.submodelDescriptors {
		if err = reconcileEmbeddedSubmodelDescriptorsTx(ctx, tx, descriptorID, previous.SubmodelDescriptors, next.SubmodelDescriptors); err != nil {
			return false, err
		}
	}
	return true, nil
}

// UpdateSubmodelDescriptorTx applies full PUT semantics while preserving
// unchanged rows of a Submodel descriptor graph.
func UpdateSubmodelDescriptorTx(
	ctx context.Context,
	tx *sql.Tx,
	descriptorID int64,
	previous model.SubmodelDescriptor,
	next model.SubmodelDescriptor,
	position int,
	positionChanged bool,
) (bool, error) {
	if previous.Id != next.Id {
		return false, common.NewErrBadRequest("SMDESC-UPDATE-IDMISMATCH descriptor ids do not match")
	}
	plan, err := buildSubmodelDescriptorUpdatePlan(previous, next)
	if err != nil {
		return false, err
	}
	if !plan.changed() && !positionChanged {
		return false, nil
	}
	if err = updateSubmodelDescriptorRowTx(ctx, tx, descriptorID, next, position); err != nil {
		return false, err
	}
	if plan.payload.changed() {
		if err = updateSubmodelDescriptorPayloadTx(ctx, tx, descriptorID, next, plan.payload); err != nil {
			return false, err
		}
	}
	if plan.endpoints {
		if err = replaceDescriptorEndpointsTx(ctx, tx, descriptorID, next.Endpoints); err != nil {
			return false, err
		}
	}
	if plan.semanticID {
		if err = replaceSubmodelDescriptorSemanticIDTx(ctx, tx, descriptorID, next); err != nil {
			return false, err
		}
	}
	if plan.supplementalSemantic {
		if err = common.ReplaceContextReferences1ToMany(
			tx,
			descriptorID,
			next.SupplementalSemanticId,
			common.TblSubmodelDescriptorSuppSemantic,
			common.ColDescriptorID,
		); err != nil {
			return false, common.NewInternalServerError("SMDESC-UPDATE-SUPPLEMENTALSEMANTIC " + err.Error())
		}
	}
	return true, nil
}

func buildAdministrationShellDescriptorUpdatePlan(
	ctx context.Context,
	previous model.AssetAdministrationShellDescriptor,
	next model.AssetAdministrationShellDescriptor,
) (administrationShellDescriptorUpdatePlan, error) {
	previousJSON, err := previous.ToJsonable()
	if err != nil {
		return administrationShellDescriptorUpdatePlan{}, common.NewInternalServerError("AASDESC-UPDATE-PREVIOUSJSON " + err.Error())
	}
	nextJSON, err := next.ToJsonable()
	if err != nil {
		return administrationShellDescriptorUpdatePlan{}, common.NewInternalServerError("AASDESC-UPDATE-NEXTJSON " + err.Error())
	}
	assetIDsChanged := !jsonFieldsEqual(previousJSON, nextJSON, "specificAssetIds")
	if discoveryIntegrationEnabled(ctx) && previous.GlobalAssetId != next.GlobalAssetId {
		assetIDsChanged = true
	}
	rootChanged := !jsonFieldsEqual(previousJSON, nextJSON, "assetKind", "assetType", "globalAssetId", "idShort", "id")
	return administrationShellDescriptorUpdatePlan{
		root:                rootChanged,
		payload:             buildDescriptorPayloadUpdatePlan(previousJSON, nextJSON),
		endpoints:           !jsonFieldsEqual(previousJSON, nextJSON, "endpoints"),
		specificAssetIDs:    assetIDsChanged,
		submodelDescriptors: !jsonFieldsEqual(previousJSON, nextJSON, "submodelDescriptors"),
	}, nil
}

func buildSubmodelDescriptorUpdatePlan(previous, next model.SubmodelDescriptor) (submodelDescriptorUpdatePlan, error) {
	previousJSON, err := previous.ToJsonable()
	if err != nil {
		return submodelDescriptorUpdatePlan{}, common.NewInternalServerError("SMDESC-UPDATE-PREVIOUSJSON " + err.Error())
	}
	nextJSON, err := next.ToJsonable()
	if err != nil {
		return submodelDescriptorUpdatePlan{}, common.NewInternalServerError("SMDESC-UPDATE-NEXTJSON " + err.Error())
	}
	return submodelDescriptorUpdatePlan{
		root:                 !jsonFieldsEqual(previousJSON, nextJSON, "idShort", "id"),
		payload:              buildDescriptorPayloadUpdatePlan(previousJSON, nextJSON),
		endpoints:            !jsonFieldsEqual(previousJSON, nextJSON, "endpoints"),
		semanticID:           !jsonFieldsEqual(previousJSON, nextJSON, "semanticId"),
		supplementalSemantic: !jsonFieldsEqual(previousJSON, nextJSON, "supplementalSemanticId", "supplementalSemanticIds"),
	}, nil
}

func buildDescriptorPayloadUpdatePlan(previous, next map[string]any) descriptorPayloadUpdatePlan {
	return descriptorPayloadUpdatePlan{
		description:    !jsonFieldsEqual(previous, next, "description"),
		displayName:    !jsonFieldsEqual(previous, next, "displayName"),
		administration: !jsonFieldsEqual(previous, next, "administration"),
		extensions:     !jsonFieldsEqual(previous, next, "extensions"),
	}
}

func jsonFieldsEqual(previous, next map[string]any, fields ...string) bool {
	for _, field := range fields {
		previousJSON, previousErr := json.Marshal(previous[field])
		nextJSON, nextErr := json.Marshal(next[field])
		if previousErr != nil || nextErr != nil || !bytes.Equal(previousJSON, nextJSON) {
			return false
		}
	}
	return true
}

func updateAdministrationShellDescriptorPayloadTx(
	ctx context.Context,
	tx *sql.Tx,
	descriptorID int64,
	descriptor model.AssetAdministrationShellDescriptor,
	plan descriptorPayloadUpdatePlan,
) error {
	return updateDescriptorPayloadTx(
		ctx,
		tx,
		descriptorID,
		descriptor.Description,
		descriptor.DisplayName,
		descriptor.Administration,
		descriptor.Extensions,
		plan,
	)
}

func updateSubmodelDescriptorPayloadTx(
	ctx context.Context,
	tx *sql.Tx,
	descriptorID int64,
	descriptor model.SubmodelDescriptor,
	plan descriptorPayloadUpdatePlan,
) error {
	return updateDescriptorPayloadTx(
		ctx,
		tx,
		descriptorID,
		descriptor.Description,
		descriptor.DisplayName,
		descriptor.Administration,
		descriptor.Extensions,
		plan,
	)
}

func updateDescriptorPayloadTx(
	ctx context.Context,
	tx *sql.Tx,
	descriptorID int64,
	description []types.ILangStringTextType,
	displayName []types.ILangStringNameType,
	administration types.IAdministrativeInformation,
	extensions []types.Extension,
	plan descriptorPayloadUpdatePlan,
) error {
	record := goqu.Record{}
	if plan.description {
		descriptionPayload, err := buildLangStringTextPayload(description)
		if err != nil {
			return common.NewInternalServerError("DESC-UPDATE-DESCRIPTIONPAYLOAD " + err.Error())
		}
		record[common.ColDescriptionPayload] = goqu.L("?::jsonb", string(descriptionPayload))
	}
	if plan.displayName {
		displayNamePayload, err := buildLangStringNamePayload(displayName)
		if err != nil {
			return common.NewInternalServerError("DESC-UPDATE-DISPLAYNAMEPAYLOAD " + err.Error())
		}
		record[common.ColDisplayNamePayload] = goqu.L("?::jsonb", string(displayNamePayload))
	}
	if plan.administration {
		administrationPayload, err := buildAdministrativeInfoPayload(administration)
		if err != nil {
			return common.NewInternalServerError("DESC-UPDATE-ADMINPAYLOAD " + err.Error())
		}
		record[common.ColAdministrativeInfoPayload] = goqu.L("?::jsonb", string(administrationPayload))
	}
	if plan.extensions {
		extensionsPayload, err := buildExtensionsPayload(extensions)
		if err != nil {
			return common.NewInternalServerError("DESC-UPDATE-EXTENSIONPAYLOAD " + err.Error())
		}
		record[common.ColExtensionsPayload] = goqu.L("?::jsonb", string(extensionsPayload))
	}
	d := goqu.Dialect(common.Dialect)
	query, args, err := d.
		Update(common.TblDescriptorPayload).
		Set(record).
		Where(goqu.C(common.ColDescriptorID).Eq(descriptorID)).
		Prepared(true).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("DESC-UPDATE-PAYLOAD-BUILDQ " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("DESC-UPDATE-PAYLOAD-EXECQ " + err.Error())
	}
	return nil
}

func replaceDescriptorEndpointsTx(ctx context.Context, tx *sql.Tx, descriptorID int64, endpoints []model.Endpoint) error {
	d := goqu.Dialect(common.Dialect)
	query, args, err := d.
		Delete(common.TblAASDescriptorEndpoint).
		Where(goqu.C(common.ColDescriptorID).Eq(descriptorID)).
		Prepared(true).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("DESC-UPDATE-ENDPOINTS-BUILDDELETE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("DESC-UPDATE-ENDPOINTS-EXECDELETE " + err.Error())
	}
	if err = CreateEndpoints(tx, descriptorID, endpoints); err != nil {
		return common.NewInternalServerError("DESC-UPDATE-ENDPOINTS-INSERT " + err.Error())
	}
	return nil
}

func replaceAdministrationShellDescriptorAssetIDsTx(
	ctx context.Context,
	tx *sql.Tx,
	descriptorID int64,
	descriptor model.AssetAdministrationShellDescriptor,
) error {
	d := goqu.Dialect(common.Dialect)
	query, args, err := d.
		Delete(common.TblSpecificAssetID).
		Where(goqu.C(common.ColDescriptorID).Eq(descriptorID)).
		Prepared(true).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASDESC-UPDATE-ASSETIDS-BUILDDELETE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("AASDESC-UPDATE-ASSETIDS-EXECDELETE " + err.Error())
	}

	var aasReference sql.NullInt64
	if discoveryIntegrationEnabled(ctx) {
		reference, referenceErr := ensureAASIdentifierTx(ctx, tx, descriptor.Id)
		if referenceErr != nil {
			return referenceErr
		}
		aasReference = sql.NullInt64{Int64: reference, Valid: true}
	}
	if err = common.CreateSpecificAssetIDDescriptor(
		tx,
		descriptorID,
		aasReference,
		specificAssetIDsWithGlobalAssetID(ctx, descriptor),
	); err != nil {
		return common.NewInternalServerError("AASDESC-UPDATE-ASSETIDS-INSERT " + err.Error())
	}
	return nil
}

func updateSubmodelDescriptorRowTx(
	ctx context.Context,
	tx *sql.Tx,
	descriptorID int64,
	descriptor model.SubmodelDescriptor,
	position int,
) error {
	d := goqu.Dialect(common.Dialect)
	query, args, err := d.
		Update(common.TblSubmodelDescriptor).
		Set(goqu.Record{
			common.ColIDShort:  descriptor.IdShort,
			common.ColAASID:    descriptor.Id,
			common.ColPosition: position,
		}).
		Where(goqu.C(common.ColDescriptorID).Eq(descriptorID)).
		Prepared(true).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMDESC-UPDATE-ROOT-BUILDQ " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("SMDESC-UPDATE-ROOT-EXECQ " + err.Error())
	}
	return nil
}

func replaceSubmodelDescriptorSemanticIDTx(
	ctx context.Context,
	tx *sql.Tx,
	descriptorID int64,
	descriptor model.SubmodelDescriptor,
) error {
	d := goqu.Dialect(common.Dialect)
	query, args, err := d.
		Delete(submodelDescriptorSemanticIDTable).
		Where(goqu.C(common.ColID).Eq(descriptorID)).
		Prepared(true).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMDESC-UPDATE-SEMANTIC-BUILDDELETE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("SMDESC-UPDATE-SEMANTIC-EXECDELETE " + err.Error())
	}
	if err = common.CreateContextReference(
		tx,
		descriptorID,
		descriptor.SemanticId,
		submodelDescriptorSemanticIDTable,
		submodelDescriptorSemanticIDTable+"_key",
	); err != nil {
		return common.NewInternalServerError("SMDESC-UPDATE-SEMANTIC-INSERT " + err.Error())
	}
	return nil
}

type embeddedSubmodelDescriptorRow struct {
	descriptorID int64
	position     int
	descriptor   model.SubmodelDescriptor
}

func reconcileEmbeddedSubmodelDescriptorsTx(
	ctx context.Context,
	tx *sql.Tx,
	aasDescriptorID int64,
	previous []model.SubmodelDescriptor,
	next []model.SubmodelDescriptor,
) error {
	rows, err := lockEmbeddedSubmodelDescriptorRowsTx(ctx, tx, aasDescriptorID, previous)
	if err != nil {
		return err
	}
	available := make(map[string][]embeddedSubmodelDescriptorRow, len(rows))
	for _, row := range rows {
		available[row.descriptor.Id] = append(available[row.descriptor.Id], row)
	}
	for position, descriptor := range next {
		candidates := available[descriptor.Id]
		if len(candidates) == 0 {
			if _, err = insertSubmodelDescriptorAtPositionTx(
				tx,
				sql.NullInt64{Int64: aasDescriptorID, Valid: true},
				descriptor,
				position,
			); err != nil {
				return common.NewInternalServerError("AASDESC-UPDATE-SUBMODELS-INSERT " + err.Error())
			}
			continue
		}
		current := candidates[0]
		available[descriptor.Id] = candidates[1:]
		if _, err = UpdateSubmodelDescriptorTx(
			ctx,
			tx,
			current.descriptorID,
			current.descriptor,
			descriptor,
			position,
			current.position != position,
		); err != nil {
			return err
		}
	}
	for _, candidates := range available {
		for _, candidate := range candidates {
			if err = deleteDescriptorByIDTx(ctx, tx, candidate.descriptorID); err != nil {
				return err
			}
		}
	}
	return nil
}

func lockEmbeddedSubmodelDescriptorRowsTx(
	ctx context.Context,
	tx *sql.Tx,
	aasDescriptorID int64,
	previous []model.SubmodelDescriptor,
) ([]embeddedSubmodelDescriptorRow, error) {
	d := goqu.Dialect(common.Dialect)
	submodelDescriptor := goqu.T(common.TblSubmodelDescriptor)
	query, args, err := d.
		From(submodelDescriptor).
		Select(
			submodelDescriptor.Col(common.ColDescriptorID),
			submodelDescriptor.Col(common.ColPosition),
		).
		Where(submodelDescriptor.Col(common.ColAASDescriptorID).Eq(aasDescriptorID)).
		Order(submodelDescriptor.Col(common.ColPosition).Asc()).
		ForUpdate(goqu.Wait).
		Prepared(true).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("AASDESC-UPDATE-SUBMODELS-BUILDLOCK " + err.Error())
	}
	databaseRows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("AASDESC-UPDATE-SUBMODELS-EXECLOCK " + err.Error())
	}
	defer func() {
		_ = databaseRows.Close()
	}()

	rows := make([]embeddedSubmodelDescriptorRow, 0, len(previous))
	for databaseRows.Next() {
		var row embeddedSubmodelDescriptorRow
		if err = databaseRows.Scan(&row.descriptorID, &row.position); err != nil {
			return nil, common.NewInternalServerError("AASDESC-UPDATE-SUBMODELS-SCANLOCK " + err.Error())
		}
		rows = append(rows, row)
	}
	if err = databaseRows.Err(); err != nil {
		return nil, common.NewInternalServerError("AASDESC-UPDATE-SUBMODELS-ITERATELOCK " + err.Error())
	}
	if len(rows) != len(previous) {
		return nil, common.NewInternalServerError("AASDESC-UPDATE-SUBMODELS-COUNT persisted descriptor count changed during update")
	}
	for index := range rows {
		rows[index].descriptor = previous[index]
	}
	return rows, nil
}

func deleteDescriptorByIDTx(ctx context.Context, tx *sql.Tx, descriptorID int64) error {
	d := goqu.Dialect(common.Dialect)
	query, args, err := d.
		Delete(common.TblDescriptor).
		Where(goqu.C(common.ColID).Eq(descriptorID)).
		Prepared(true).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("DESC-UPDATE-DELETE-BUILDQ " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("DESC-UPDATE-DELETE-EXECQ " + err.Error())
	}
	return nil
}
