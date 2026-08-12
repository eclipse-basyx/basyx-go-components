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

package persistence

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
)

type aasReconciliationMutationResult struct {
	UpdatedSpecific    int
	InsertedSpecific   int
	DeletedSpecific    int
	UpdatedReferences  int
	InsertedReferences int
	DeletedReferences  int
}

type aasReconciliationCTE struct {
	name  string
	query exp.Expression
}

type aasReconciliationQueryBuilder struct {
	dialect goqu.DialectWrapper
	ctes    []aasReconciliationCTE
}

func newAASReconciliationQueryBuilder() *aasReconciliationQueryBuilder {
	return &aasReconciliationQueryBuilder{dialect: goqu.Dialect(common.Dialect)}
}

func (b *aasReconciliationQueryBuilder) add(name string, query exp.Expression) {
	b.ctes = append(b.ctes, aasReconciliationCTE{name: name, query: query})
}

func (b *aasReconciliationQueryBuilder) build(planJSON []byte, aasID string) (string, []any, error) {
	b.addInputs(planJSON, aasID)
	b.addMetadataCTEs()
	b.addSpecificAssetIDCTEs()
	b.addSubmodelReferenceCTEs()
	final := b.dialect.Select(
		goqu.L("(SELECT COUNT(*) FROM resolved_specific_updates)").As("updated_specific"),
		goqu.L("(SELECT COUNT(*) FROM inserted_specific_rows)").As("inserted_specific"),
		goqu.L("(SELECT COUNT(*) FROM deleted_specific_rows)").As("deleted_specific"),
		goqu.L("(SELECT COUNT(*) FROM resolved_reference_updates)").As("updated_references"),
		goqu.L("(SELECT COUNT(*) FROM inserted_reference_rows)").As("inserted_references"),
		goqu.L("(SELECT COUNT(*) FROM deleted_reference_rows)").As("deleted_references"),
	)
	for _, cte := range b.ctes {
		final = final.With(cte.name, cte.query)
	}
	return final.Prepared(true).ToSQL()
}

func (b *aasReconciliationQueryBuilder) addInputs(planJSON []byte, aasID string) {
	b.add("aas_reconciliation_plan", b.dialect.Select(goqu.L("?::jsonb", string(planJSON)).As("data")))
	b.add("target_aas", b.dialect.From("aas").Select("id").Where(goqu.C("aas_id").Eq(aasID)))
	b.add("raw_specific_updates", aasJSONRecordRows(b.dialect, "specificUpdates"))
	b.add("raw_specific_inserts", aasJSONRecordRows(b.dialect, "specificInserts"))
	b.add("raw_specific_deletes", aasJSONRecordRows(b.dialect, "specificDeletes"))
	b.add("raw_reference_updates", aasJSONRecordRows(b.dialect, "referenceUpdates"))
	b.add("raw_reference_inserts", aasJSONRecordRows(b.dialect, "referenceInserts"))
	b.add("raw_reference_deletes", aasJSONRecordRows(b.dialect, "referenceDeletes"))
}

func aasJSONRecordRows(dialect goqu.DialectWrapper, field string) *goqu.SelectDataset {
	return dialect.From(
		goqu.T("aas_reconciliation_plan").As("p"),
		goqu.L(
			"jsonb_to_recordset(COALESCE(? -> ?, '[]'::jsonb)) AS ?",
			goqu.I("p.data"), common.PostgreSQLTextLiteral(field), goqu.L("decoded(row jsonb)"),
		),
	).Select(goqu.I("decoded.row"))
}

func (b *aasReconciliationQueryBuilder) addMetadataCTEs() {
	metadata := goqu.L("p.data -> 'metadata'")
	b.add("updated_aas_metadata", b.dialect.Update(goqu.T("aas").As("a")).Set(goqu.Record{
		"id_short": aasJSONTextExpression(metadata, "idShort"),
		"category": aasJSONTextExpression(metadata, "category"),
	}).From(goqu.T("target_aas").As("target"), goqu.T("aas_reconciliation_plan").As("p")).Where(
		goqu.I("a.id").Eq(goqu.I("target.id")), aasMetadataChange(metadata, "coreChanged"),
	).Returning(goqu.I("a.id")))

	payloadSource := b.dialect.From(goqu.T("target_aas").As("target"), goqu.T("aas_reconciliation_plan").As("p")).Select(
		goqu.I("target.id"),
		aasNullableJSONField(metadata, "description"),
		aasNullableJSONField(metadata, "displayName"),
		aasNullableJSONField(metadata, "administration"),
		aasNullableJSONField(metadata, "embeddedDataSpecifications"),
		aasNullableJSONField(metadata, "extensions"),
		aasNullableJSONField(metadata, "derivedFrom"),
	).Where(aasMetadataChange(metadata, "payloadChanged"))
	payloadColumns := []string{"description_payload", "displayname_payload", "administrative_information_payload", "embedded_data_specification_payload", "extensions_payload", "derived_from_payload"}
	b.add("upserted_aas_payload", b.dialect.Insert("aas_payload").
		Cols(aasStringInterfaces(append([]string{"aas_id"}, payloadColumns...))...).
		FromQuery(payloadSource).
		OnConflict(goqu.DoUpdate("aas_id", aasExcludedRecord(payloadColumns...))).
		Returning("aas_id"))

	assetInformationSource := b.dialect.From(goqu.T("target_aas").As("target"), goqu.T("aas_reconciliation_plan").As("p")).Select(
		goqu.I("target.id"), aasJSONIntExpression(metadata, "assetKind"),
		aasJSONTextExpression(metadata, "globalAssetId"), aasJSONTextExpression(metadata, "assetType"),
	).Where(aasMetadataChange(metadata, "assetInformationChanged"))
	b.add("upserted_asset_information", b.dialect.Insert("asset_information").
		Cols("asset_information_id", "asset_kind", "global_asset_id", "asset_type").
		FromQuery(assetInformationSource).
		OnConflict(goqu.DoUpdate("asset_information_id", aasExcludedRecord("asset_kind", "global_asset_id", "asset_type"))).
		Returning("asset_information_id"))

	b.addThumbnailCTEs(metadata)
}

func (b *aasReconciliationQueryBuilder) addThumbnailCTEs(metadata exp.Expression) {
	thumbnailChanged := aasMetadataChange(metadata, "thumbnailChanged")
	targetThumbnail := goqu.L("? -> 'thumbnail'", metadata)
	targetPath := goqu.L("? ->> 'path'", targetThumbnail)
	changedOwner := b.dialect.From(goqu.T("target_aas").As("target"), goqu.T("aas_reconciliation_plan").As("p"), goqu.T("thumbnail_file_element").As("element")).
		Select(goqu.L("1")).Where(
		goqu.I("element.id").Eq(goqu.I("target.id")),
		goqu.I("thumbnail_binary_reference.thumbnail_element_id").Eq(goqu.I("target.id")),
		thumbnailChanged,
		goqu.Or(goqu.L("? IS NULL", targetThumbnail), goqu.L("? = 'null'::jsonb", targetThumbnail), goqu.L("? IS DISTINCT FROM ?", goqu.I("element.value"), targetPath)),
	)
	b.add("deleted_thumbnail_binary_reference", b.dialect.Delete(binarycontent.TableThumbnailReference).
		Where(goqu.Func("EXISTS", changedOwner)).Returning("thumbnail_element_id"))

	unlinkedSource := b.dialect.From(
		goqu.T("target_aas").As("target"), goqu.T("aas_reconciliation_plan").As("p"), goqu.T("thumbnail_file_data").As("data"),
	).Select(goqu.Func("lo_unlink", goqu.I("data.file_oid")).As("unlinked")).Where(
		goqu.I("data.id").Eq(goqu.I("target.id")), goqu.I("data.file_oid").IsNotNull(), thumbnailChanged,
		goqu.L("(SELECT COUNT(*) FROM deleted_thumbnail_binary_reference) >= 0"),
	)
	b.add("unlinked_thumbnail_large_objects", unlinkedSource)
	b.add("deleted_thumbnail_file_data", b.dialect.Delete("thumbnail_file_data").Where(
		goqu.Func("EXISTS", b.dialect.From(goqu.T("target_aas").As("target"), goqu.T("aas_reconciliation_plan").As("p")).
			Select(goqu.L("1")).Where(goqu.I("thumbnail_file_data.id").Eq(goqu.I("target.id")), thumbnailChanged)),
		goqu.L("(SELECT COUNT(*) FROM unlinked_thumbnail_large_objects) >= 0"),
	).Returning("id"))
	b.add("deleted_thumbnail_element", b.dialect.Delete("thumbnail_file_element").Where(
		goqu.Func("EXISTS", b.dialect.From(goqu.T("target_aas").As("target"), goqu.T("aas_reconciliation_plan").As("p")).
			Select(goqu.L("1")).Where(
			goqu.I("thumbnail_file_element.id").Eq(goqu.I("target.id")), thumbnailChanged,
			goqu.Or(goqu.L("? IS NULL", targetThumbnail), goqu.L("? = 'null'::jsonb", targetThumbnail)),
		)),
		goqu.L("(SELECT COUNT(*) FROM deleted_thumbnail_file_data) >= 0"),
	).Returning("id"))

	thumbnailSource := b.dialect.From(goqu.T("target_aas").As("target"), goqu.T("aas_reconciliation_plan").As("p")).Select(
		goqu.I("target.id"), aasJSONTextExpression(targetThumbnail, "contentType"), goqu.L("NULL"), targetPath,
	).Where(thumbnailChanged, goqu.L("? IS NOT NULL", targetThumbnail), goqu.L("? <> 'null'::jsonb", targetThumbnail),
		goqu.L("(SELECT COUNT(*) FROM deleted_thumbnail_element) >= 0"))
	b.add("upserted_thumbnail_element", b.dialect.Insert("thumbnail_file_element").
		Cols("id", "content_type", "file_name", "value").FromQuery(thumbnailSource).
		OnConflict(goqu.DoUpdate("id", goqu.Record{
			"content_type": goqu.I("excluded.content_type"),
			"file_name":    goqu.L("CASE WHEN thumbnail_file_element.value IS NOT DISTINCT FROM excluded.value AND EXISTS (SELECT 1 FROM thumbnail_binary_reference AS reference WHERE reference.thumbnail_element_id = thumbnail_file_element.id) THEN thumbnail_file_element.file_name ELSE NULL END"),
			"value":        goqu.I("excluded.value"),
		})).Returning("id"))
}

func (b *aasReconciliationQueryBuilder) addSpecificAssetIDCTEs() {
	b.add("resolved_specific_updates", b.dialect.From(goqu.T("raw_specific_updates").As("u")).
		Join(goqu.T("target_aas").As("target"), goqu.On(goqu.L("TRUE"))).
		Join(goqu.T("specific_asset_id").As("specific"), goqu.On(
			goqu.I("specific.asset_information_id").Eq(goqu.I("target.id")),
			goqu.I("specific.position").Eq(aasJSONInt("u.row", "matchPosition")),
		)).Select(goqu.I("specific.id"), goqu.I("u.row")))
	b.add("resolved_specific_deletes", b.dialect.From(goqu.T("raw_specific_deletes").As("d")).
		Join(goqu.T("target_aas").As("target"), goqu.On(goqu.L("TRUE"))).
		Join(goqu.T("specific_asset_id").As("specific"), goqu.On(
			goqu.I("specific.asset_information_id").Eq(goqu.I("target.id")),
			goqu.I("specific.position").Eq(aasJSONInt("d.row", "matchPosition")),
		)).Select(goqu.I("specific.id")))
	b.add("deleted_specific_rows", b.dialect.Delete("specific_asset_id").Where(
		goqu.Func("EXISTS", b.dialect.From(goqu.T("resolved_specific_deletes").As("d")).Select(goqu.L("1")).
			Where(goqu.I("specific_asset_id.id").Eq(goqu.I("d.id")))),
	).Returning("id"))
	b.add("updated_specific_rows", b.dialect.Update(goqu.T("specific_asset_id").As("specific")).Set(goqu.Record{
		"position": aasJSONInt("u.row", "position"), "name": aasJSONText("u.row", "name"), "value": aasJSONText("u.row", "value"),
	}).From(goqu.T("resolved_specific_updates").As("u")).Where(
		goqu.I("specific.id").Eq(goqu.I("u.id")), aasJSONChange("u.row", "core"),
		goqu.L("(SELECT COUNT(*) FROM deleted_specific_rows) >= 0"),
	).Returning(goqu.I("specific.id")))
	specificInsertSource := b.dialect.From(goqu.T("raw_specific_inserts").As("i"), goqu.T("target_aas").As("target")).Select(
		goqu.I("target.id"), aasJSONInt("i.row", "position"), aasJSONText("i.row", "name"), aasJSONText("i.row", "value"),
	).Where(goqu.L("(SELECT COUNT(*) FROM deleted_specific_rows) >= 0"))
	b.add("inserted_specific_rows", b.dialect.Insert("specific_asset_id").
		Cols("asset_information_id", "position", "name", "value").FromQuery(specificInsertSource).Returning("id", "position"))
	updatedSpecific := b.dialect.From(goqu.T("resolved_specific_updates").As("source")).Select("source.id", "source.row").
		Where(goqu.L("(SELECT COUNT(*) FROM updated_specific_rows) >= 0"))
	insertedSpecific := b.dialect.From(goqu.T("raw_specific_inserts").As("source")).
		Join(goqu.T("inserted_specific_rows").As("inserted"), goqu.On(goqu.I("inserted.position").Eq(aasJSONInt("source.row", "position")))).
		Select(goqu.I("inserted.id"), goqu.I("source.row"))
	b.add("affected_specific_rows", updatedSpecific.UnionAll(insertedSpecific))

	semanticSource := b.dialect.From(goqu.T("affected_specific_rows").As("a")).Select(
		goqu.I("a.id"), goqu.L("a.row -> 'semanticId'"),
	).Where(aasJSONChange("a.row", "payload"))
	b.add("upserted_specific_payloads", b.dialect.Insert("specific_asset_id_payload").
		Cols("specific_asset_id", "semantic_id_payload").FromQuery(semanticSource).
		OnConflict(goqu.DoUpdate("specific_asset_id", aasExcludedRecord("semantic_id_payload"))).Returning("specific_asset_id"))

	b.addSpecificExternalReferenceCTEs()
	b.addSpecificSupplementalReferenceCTEs()
}

func (b *aasReconciliationQueryBuilder) addSpecificExternalReferenceCTEs() {
	external := goqu.L("a.row -> 'externalSubjectId'")
	b.add("deleted_specific_external", b.dialect.Delete("specific_asset_id_external_subject_id_reference").Where(
		goqu.Func("EXISTS", b.dialect.From(goqu.T("affected_specific_rows").As("a")).Select(goqu.L("1")).Where(
			goqu.I("specific_asset_id_external_subject_id_reference.id").Eq(goqu.I("a.id")), aasJSONChange("a.row", "external"),
			goqu.Or(goqu.L("? IS NULL", external), goqu.L("? = 'null'::jsonb", external)),
		)),
	).Returning("id"))
	externalSource := b.dialect.From(goqu.T("affected_specific_rows").As("a")).Select(
		goqu.I("a.id"), aasJSONIntExpression(external, "type"),
	).Where(aasJSONChange("a.row", "external"), goqu.L("? IS NOT NULL", external), goqu.L("? <> 'null'::jsonb", external),
		goqu.L("(SELECT COUNT(*) FROM deleted_specific_external) >= 0"))
	b.add("upserted_specific_external", b.dialect.Insert("specific_asset_id_external_subject_id_reference").Cols("id", "type").
		FromQuery(externalSource).OnConflict(goqu.DoUpdate("id", aasExcludedRecord("type"))).Returning("id"))
	b.add("updated_specific_external_payload", b.dialect.Update(
		goqu.T("specific_asset_id_external_subject_id_reference_payload").As("payload"),
	).Set(goqu.Record{
		"parent_reference_payload": goqu.L("a.row -> 'externalSubjectId' -> 'payload'"),
	}).From(
		goqu.T("affected_specific_rows").As("a"), goqu.T("upserted_specific_external").As("external"),
	).Where(
		goqu.I("external.id").Eq(goqu.I("a.id")),
		goqu.I("payload.reference_id").Eq(goqu.I("external.id")),
	).Returning(goqu.I("payload.reference_id")))
	externalPayloadSource := b.dialect.From(
		goqu.T("affected_specific_rows").As("a"), goqu.T("upserted_specific_external").As("external"),
	).Select(
		goqu.I("a.id"), goqu.L("a.row -> 'externalSubjectId' -> 'payload'"),
	).Where(
		goqu.I("external.id").Eq(goqu.I("a.id")),
		goqu.L("(SELECT COUNT(*) FROM updated_specific_external_payload) >= 0"),
		goqu.Func("NOT EXISTS", b.dialect.From("specific_asset_id_external_subject_id_reference_payload").
			Select(goqu.L("1")).Where(goqu.I("reference_id").Eq(goqu.I("external.id")))),
	)
	b.add("inserted_specific_external_payload", b.dialect.Insert("specific_asset_id_external_subject_id_reference_payload").
		Cols("reference_id", "parent_reference_payload").FromQuery(externalPayloadSource).Returning("reference_id"))
	b.add("deleted_specific_external_keys", b.dialect.Delete("specific_asset_id_external_subject_id_reference_key").Where(
		goqu.Func("EXISTS", b.dialect.From(goqu.T("affected_specific_rows").As("a"), goqu.T("upserted_specific_external").As("external")).
			Select(goqu.L("1")).Where(
			goqu.I("external.id").Eq(goqu.I("a.id")),
			goqu.I("specific_asset_id_external_subject_id_reference_key.reference_id").Eq(goqu.I("a.id")),
			goqu.Func("NOT EXISTS", aasNestedArrayPositionMatch(b.dialect, "a.row", "externalSubjectId", "keys", "specific_asset_id_external_subject_id_reference_key.position")),
		)),
	).Returning("reference_id"))
	externalKeySource := b.dialect.From(
		goqu.T("affected_specific_rows").As("a"), goqu.T("upserted_specific_external").As("external"),
		goqu.L("jsonb_array_elements(COALESCE(a.row -> 'externalSubjectId' -> 'keys', '[]'::jsonb))").As("key"),
	).Select(goqu.I("a.id"), aasJSONInt("key", "position"), aasJSONInt("key", "type"), aasJSONText("key", "value")).Where(
		goqu.I("external.id").Eq(goqu.I("a.id")), goqu.L("(SELECT COUNT(*) FROM deleted_specific_external_keys) >= 0"),
	)
	b.add("upserted_specific_external_keys", b.dialect.Insert("specific_asset_id_external_subject_id_reference_key").
		Cols("reference_id", "position", "type", "value").FromQuery(externalKeySource).
		OnConflict(goqu.DoUpdate("reference_id,position", aasExcludedRecord("type", "value"))).Returning("reference_id"))
}

func (b *aasReconciliationQueryBuilder) addSpecificSupplementalReferenceCTEs() {
	b.add("deleted_specific_supplemental", b.dialect.Delete("specific_asset_id_supplemental_semantic_id_reference").Where(
		goqu.Func("EXISTS", b.dialect.From(goqu.T("affected_specific_rows").As("a")).Select(goqu.L("1")).Where(
			goqu.I("specific_asset_id_supplemental_semantic_id_reference.specific_asset_id_id").Eq(goqu.I("a.id")),
			aasJSONChange("a.row", "supplementalId"),
			goqu.Func("NOT EXISTS", aasArrayPositionMatch(b.dialect, "a.row", "supplementalSemanticIds", "specific_asset_id_supplemental_semantic_id_reference.position")),
		)),
	).Returning("id"))
	supplementalSource := b.dialect.From(
		goqu.T("affected_specific_rows").As("a"),
		goqu.L("jsonb_array_elements(COALESCE(a.row -> 'supplementalSemanticIds', '[]'::jsonb))").As("reference"),
	).Select(goqu.I("a.id"), aasJSONInt("reference", "position"), aasJSONInt("reference", "type")).Where(
		aasJSONChange("a.row", "supplementalId"), goqu.L("(SELECT COUNT(*) FROM deleted_specific_supplemental) >= 0"),
	)
	b.add("upserted_specific_supplemental", b.dialect.Insert("specific_asset_id_supplemental_semantic_id_reference").
		Cols("specific_asset_id_id", "position", "type").FromQuery(supplementalSource).
		OnConflict(goqu.DoUpdate("specific_asset_id_id,position", aasExcludedRecord("type"))).Returning("id", "specific_asset_id_id", "position"))
	b.add("updated_specific_supplemental_payload", b.dialect.Update(
		goqu.T("specific_asset_id_supplemental_semantic_id_reference_payload").As("payload"),
	).Set(goqu.Record{
		"parent_reference_payload": goqu.L("? -> 'payload'", goqu.I("reference")),
	}).From(
		goqu.T("affected_specific_rows").As("a"), goqu.T("upserted_specific_supplemental").As("persisted"),
		goqu.L("jsonb_array_elements(COALESCE(a.row -> 'supplementalSemanticIds', '[]'::jsonb))").As("reference"),
	).Where(
		goqu.I("persisted.specific_asset_id_id").Eq(goqu.I("a.id")),
		goqu.I("persisted.position").Eq(aasJSONInt("reference", "position")),
		goqu.I("payload.reference_id").Eq(goqu.I("persisted.id")),
	).Returning(goqu.I("payload.reference_id")))
	supplementalPayloadSource := b.dialect.From(
		goqu.T("affected_specific_rows").As("a"), goqu.T("upserted_specific_supplemental").As("persisted"),
		goqu.L("jsonb_array_elements(COALESCE(a.row -> 'supplementalSemanticIds', '[]'::jsonb))").As("reference"),
	).Select(goqu.I("persisted.id"), goqu.L("? -> 'payload'", goqu.I("reference"))).Where(
		goqu.I("persisted.specific_asset_id_id").Eq(goqu.I("a.id")),
		goqu.I("persisted.position").Eq(aasJSONInt("reference", "position")),
		goqu.L("(SELECT COUNT(*) FROM updated_specific_supplemental_payload) >= 0"),
		goqu.Func("NOT EXISTS", b.dialect.From("specific_asset_id_supplemental_semantic_id_reference_payload").
			Select(goqu.L("1")).Where(goqu.I("reference_id").Eq(goqu.I("persisted.id")))),
	)
	b.add("inserted_specific_supplemental_payload", b.dialect.Insert("specific_asset_id_supplemental_semantic_id_reference_payload").
		Cols("reference_id", "parent_reference_payload").FromQuery(supplementalPayloadSource).Returning("reference_id"))
	b.add("deleted_specific_supplemental_keys", b.dialect.Delete("specific_asset_id_supplemental_semantic_id_reference_key").Where(
		goqu.Func("EXISTS", b.dialect.From(
			goqu.T("affected_specific_rows").As("a"), goqu.T("upserted_specific_supplemental").As("persisted"),
			goqu.L("jsonb_array_elements(COALESCE(a.row -> 'supplementalSemanticIds', '[]'::jsonb))").As("reference"),
		).Select(goqu.L("1")).Where(
			goqu.I("persisted.specific_asset_id_id").Eq(goqu.I("a.id")), goqu.I("persisted.position").Eq(aasJSONInt("reference", "position")),
			goqu.I("specific_asset_id_supplemental_semantic_id_reference_key.reference_id").Eq(goqu.I("persisted.id")),
			goqu.Func("NOT EXISTS", aasObjectArrayPositionMatch(b.dialect, "reference", "keys", "specific_asset_id_supplemental_semantic_id_reference_key.position")),
		)),
	).Returning("reference_id"))
	supplementalKeySource := b.dialect.From(
		goqu.T("affected_specific_rows").As("a"), goqu.T("upserted_specific_supplemental").As("persisted"),
		goqu.L("jsonb_array_elements(COALESCE(a.row -> 'supplementalSemanticIds', '[]'::jsonb))").As("reference"),
		goqu.L("jsonb_array_elements(COALESCE(reference -> 'keys', '[]'::jsonb))").As("key"),
	).Select(goqu.I("persisted.id"), aasJSONInt("key", "position"), aasJSONInt("key", "type"), aasJSONText("key", "value")).Where(
		goqu.I("persisted.specific_asset_id_id").Eq(goqu.I("a.id")), goqu.I("persisted.position").Eq(aasJSONInt("reference", "position")),
		goqu.L("(SELECT COUNT(*) FROM deleted_specific_supplemental_keys) >= 0"),
	)
	b.add("upserted_specific_supplemental_keys", b.dialect.Insert("specific_asset_id_supplemental_semantic_id_reference_key").
		Cols("reference_id", "position", "type", "value").FromQuery(supplementalKeySource).
		OnConflict(goqu.DoUpdate("reference_id,position", aasExcludedRecord("type", "value"))).Returning("reference_id"))
}

func (b *aasReconciliationQueryBuilder) addSubmodelReferenceCTEs() {
	b.add("resolved_reference_updates", b.resolveReferenceRows("raw_reference_updates", "u"))
	b.add("resolved_reference_deletes", b.resolveReferenceRows("raw_reference_deletes", "d").Select(goqu.I("reference.id")))
	b.add("deleted_reference_rows", b.dialect.Delete("aas_submodel_reference").Where(
		goqu.Func("EXISTS", b.dialect.From(goqu.T("resolved_reference_deletes").As("d")).Select(goqu.L("1")).
			Where(goqu.I("aas_submodel_reference.id").Eq(goqu.I("d.id")))),
	).Returning("id"))
	b.add("updated_reference_rows", b.dialect.Update(goqu.T("aas_submodel_reference").As("reference")).Set(goqu.Record{
		"position": aasJSONInt("u.row", "position"), "type": aasJSONInt("u.row", "type"),
	}).From(goqu.T("resolved_reference_updates").As("u")).Where(
		goqu.I("reference.id").Eq(goqu.I("u.id")), aasJSONChange("u.row", "core"),
		goqu.L("(SELECT COUNT(*) FROM deleted_reference_rows) >= 0"),
	).Returning(goqu.I("reference.id")))
	referenceInsertSource := b.dialect.From(goqu.T("raw_reference_inserts").As("i"), goqu.T("target_aas").As("target")).Select(
		goqu.I("target.id"), aasJSONInt("i.row", "position"), aasJSONInt("i.row", "type"),
	).Where(goqu.L("(SELECT COUNT(*) FROM deleted_reference_rows) >= 0"))
	b.add("inserted_reference_rows", b.dialect.Insert("aas_submodel_reference").Cols("aas_id", "position", "type").
		FromQuery(referenceInsertSource).Returning("id", "position"))
	updatedReferences := b.dialect.From(goqu.T("resolved_reference_updates").As("source")).Select("source.id", "source.row").
		Where(goqu.L("(SELECT COUNT(*) FROM updated_reference_rows) >= 0"))
	insertedReferences := b.dialect.From(goqu.T("raw_reference_inserts").As("source")).
		Join(goqu.T("inserted_reference_rows").As("inserted"), goqu.On(goqu.I("inserted.position").Eq(aasJSONInt("source.row", "position")))).
		Select(goqu.I("inserted.id"), goqu.I("source.row"))
	b.add("affected_reference_rows", updatedReferences.UnionAll(insertedReferences))

	payloadSource := b.dialect.From(goqu.T("affected_reference_rows").As("a")).Select(goqu.I("a.id"), goqu.L("a.row -> 'payload'")).
		Where(aasJSONChange("a.row", "payload"))
	b.add("upserted_reference_payloads", b.dialect.Insert("aas_submodel_reference_payload").
		Cols("reference_id", "parent_reference_payload").FromQuery(payloadSource).
		OnConflict(goqu.DoUpdate("reference_id", aasExcludedRecord("parent_reference_payload"))).Returning("reference_id"))
	b.add("deleted_reference_keys", b.dialect.Delete("aas_submodel_reference_key").Where(
		goqu.Func("EXISTS", b.dialect.From(goqu.T("affected_reference_rows").As("a")).Select(goqu.L("1")).Where(
			goqu.I("aas_submodel_reference_key.reference_id").Eq(goqu.I("a.id")), aasJSONChange("a.row", "keys"),
			goqu.Func("NOT EXISTS", aasArrayPositionMatch(b.dialect, "a.row", "keys", "aas_submodel_reference_key.position")),
		)),
	).Returning("reference_id"))
	keySource := b.dialect.From(
		goqu.T("affected_reference_rows").As("a"), goqu.L("jsonb_array_elements(COALESCE(a.row -> 'keys', '[]'::jsonb))").As("key"),
	).Select(goqu.I("a.id"), aasJSONInt("key", "position"), aasJSONInt("key", "type"), aasJSONText("key", "value")).Where(
		aasJSONChange("a.row", "keys"), goqu.L("(SELECT COUNT(*) FROM deleted_reference_keys) >= 0"),
	)
	b.add("upserted_reference_keys", b.dialect.Insert("aas_submodel_reference_key").Cols("reference_id", "position", "type", "value").
		FromQuery(keySource).OnConflict(goqu.DoUpdate("reference_id,position", aasExcludedRecord("type", "value"))).Returning("reference_id"))
}

func (b *aasReconciliationQueryBuilder) resolveReferenceRows(source string, alias string) *goqu.SelectDataset {
	identity := aasJSONText(alias+".row", "matchIdentity")
	semanticMatch := b.dialect.From(goqu.T("aas_submodel_reference_key").As("identity_key")).Select(goqu.L("1")).Where(
		goqu.I("identity_key.reference_id").Eq(goqu.I("reference.id")),
		goqu.I("identity_key.type").Eq(goqu.L(strconv.Itoa(int(types.KeyTypesSubmodel)))),
		goqu.I("identity_key.value").Eq(identity),
	)
	return b.dialect.From(goqu.T(source).As(alias)).
		Join(goqu.T("target_aas").As("target"), goqu.On(goqu.L("TRUE"))).
		Join(goqu.T("aas_submodel_reference").As("reference"), goqu.On(
			goqu.I("reference.aas_id").Eq(goqu.I("target.id")),
			goqu.Or(
				goqu.And(identity.IsNotNull(), goqu.Func("EXISTS", semanticMatch)),
				goqu.And(identity.IsNull(), goqu.I("reference.position").Eq(aasJSONInt(alias+".row", "matchPosition"))),
			),
		)).Select(goqu.I("reference.id"), goqu.I(alias+".row"))
}

func aasArrayPositionMatch(dialect goqu.DialectWrapper, row string, field string, positionColumn string) *goqu.SelectDataset {
	return dialect.From(goqu.L("jsonb_array_elements(COALESCE(? -> ?, '[]'::jsonb))", goqu.I(row), common.PostgreSQLTextLiteral(field)).As("candidate")).
		Select(goqu.L("1")).Where(aasJSONInt("candidate", "position").Eq(goqu.I(positionColumn)))
}

func aasNestedArrayPositionMatch(dialect goqu.DialectWrapper, row string, parent string, field string, positionColumn string) *goqu.SelectDataset {
	return dialect.From(goqu.L("jsonb_array_elements(COALESCE(? -> ? -> ?, '[]'::jsonb))", goqu.I(row), common.PostgreSQLTextLiteral(parent), common.PostgreSQLTextLiteral(field)).As("candidate")).
		Select(goqu.L("1")).Where(aasJSONInt("candidate", "position").Eq(goqu.I(positionColumn)))
}

func aasObjectArrayPositionMatch(dialect goqu.DialectWrapper, object string, field string, positionColumn string) *goqu.SelectDataset {
	return dialect.From(goqu.L("jsonb_array_elements(COALESCE(? -> ?, '[]'::jsonb))", goqu.I(object), common.PostgreSQLTextLiteral(field)).As("candidate")).
		Select(goqu.L("1")).Where(aasJSONInt("candidate", "position").Eq(goqu.I(positionColumn)))
}

func aasJSONText(alias string, key string) exp.LiteralExpression {
	return goqu.L("? ->> ?", goqu.I(alias), common.PostgreSQLTextLiteral(key))
}

func aasJSONInt(alias string, key string) exp.LiteralExpression {
	return goqu.L("(? ->> ?)::integer", goqu.I(alias), common.PostgreSQLTextLiteral(key))
}

func aasJSONTextExpression(parent exp.Expression, key string) exp.LiteralExpression {
	return goqu.L("? ->> ?", parent, common.PostgreSQLTextLiteral(key))
}

func aasJSONIntExpression(parent exp.Expression, key string) exp.LiteralExpression {
	return goqu.L("(? ->> ?)::integer", parent, common.PostgreSQLTextLiteral(key))
}

func aasJSONChange(alias string, key string) exp.LiteralExpression {
	return goqu.L("COALESCE((? -> 'changes' ->> ?)::boolean, false)", goqu.I(alias), common.PostgreSQLTextLiteral(key))
}

func aasMetadataChange(metadata exp.Expression, key string) exp.LiteralExpression {
	return goqu.L("COALESCE((? ->> ?)::boolean, false)", metadata, common.PostgreSQLTextLiteral(key))
}

func aasNullableJSONField(parent exp.Expression, key string) exp.LiteralExpression {
	return goqu.L("NULLIF(? -> ?, 'null'::jsonb)", parent, common.PostgreSQLTextLiteral(key))
}

func aasExcludedRecord(columns ...string) goqu.Record {
	record := make(goqu.Record, len(columns))
	for _, column := range columns {
		record[column] = goqu.I("excluded." + column)
	}
	return record
}

func aasStringInterfaces(values []string) []interface{} {
	result := make([]interface{}, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func executeAASReconciliationStatement(
	ctx context.Context,
	tx *sql.Tx,
	aasID string,
	plan aasReconciliationPlan,
) (aasReconciliationMutationResult, error) {
	planJSON, err := plan.marshal()
	if err != nil {
		return aasReconciliationMutationResult{}, err
	}
	query, args, err := newAASReconciliationQueryBuilder().build(planJSON, aasID)
	if err != nil {
		return aasReconciliationMutationResult{}, common.NewInternalServerError("AASREPO-RECON-BUILD " + err.Error())
	}
	var result aasReconciliationMutationResult
	if err = tx.QueryRowContext(ctx, query, args...).Scan(
		&result.UpdatedSpecific, &result.InsertedSpecific, &result.DeletedSpecific,
		&result.UpdatedReferences, &result.InsertedReferences, &result.DeletedReferences,
	); err != nil {
		return aasReconciliationMutationResult{}, common.NewInternalServerError("AASREPO-RECON-EXEC " + err.Error())
	}
	if result.UpdatedSpecific != len(plan.SpecificUpdates) || result.InsertedSpecific != len(plan.SpecificInserts) ||
		result.DeletedSpecific != len(plan.SpecificDeletes) || result.UpdatedReferences != len(plan.ReferenceUpdates) ||
		result.InsertedReferences != len(plan.ReferenceInserts) || result.DeletedReferences != len(plan.ReferenceDeletes) {
		return aasReconciliationMutationResult{}, common.NewInternalServerError("AASREPO-RECON-COUNT affected row count does not match reconciliation plan")
	}
	return result, nil
}
