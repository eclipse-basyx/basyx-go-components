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

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/postgresstaging"
)

func (b *reconciliationQueryBuilder) addStagedReconciliationInputs(stage *postgresstaging.Stage) {
	b.add("target_metadata_rows", stage.Dataset(b.dialect, submodelMetadataStageDataset))
	b.add("target_metadata", b.dialect.From("target_metadata_rows").Select(goqu.I("row_data").As("data")))
	b.add("target_element_rows", stage.Dataset(b.dialect, submodelElementStageDataset))
	b.add("reconciliation_plan", stagedReconciliationMetadata(b.dialect))
	b.add("direct_insert_candidates", directInsertCandidates(b.dialect))
	b.add("insert_roots", stagedInsertRoots(b.dialect))
	b.addRecursive("insert_tree", stagedInsertTree(b.dialect))
	b.add("staged_insert_rows", stagedInsertRows(b.dialect))
	b.add("retained_target_rows", retainedStagedRows(b.dialect))
	b.add("classified_update_rows", classifiedStagedUpdateRows(b.dialect))
	b.add("raw_update_rows", changedStagedRows(b.dialect))
	b.add("raw_insert_rows", b.dialect.From("staged_insert_rows").Select("row"))
	b.add("delete_candidates", stagedDeleteCandidates(b.dialect))
}

func stagedReconciliationMetadata(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	metadata := goqu.I("metadata.data")
	changes := stagedMetadataChanges(dialect, metadata)
	return dialect.From(goqu.T("target_metadata").As("metadata")).
		CrossJoin(goqu.T("target_submodel").As("target")).
		LeftJoin(goqu.T("submodel").As("sm"), goqu.On(goqu.I("sm.id").Eq(goqu.I("target.id")))).
		LeftJoin(goqu.T("submodel_payload").As("payload"), goqu.On(goqu.I("payload.submodel_id").Eq(goqu.I("target.id")))).
		Select(goqu.Func(
			"jsonb_build_object",
			common.PostgreSQLTextLiteral("metadata"),
			goqu.L("? || ?", metadata, changes),
		).As("data"))
}

func stagedMetadataChanges(dialect goqu.DialectWrapper, metadata exp.Expression) exp.Expression {
	coreChanged := goqu.Or(
		goqu.L("sm.id_short IS DISTINCT FROM (? ->> 'idShort')", metadata),
		goqu.L("sm.category IS DISTINCT FROM (? ->> 'category')", metadata),
		goqu.L("sm.kind IS DISTINCT FROM NULLIF(? ->> 'kind', '')::integer", metadata),
	)
	payloadChanged := goqu.Or(
		goqu.L("payload.description_payload IS DISTINCT FROM ?", nullableJSONField(metadata, "description")),
		goqu.L("payload.displayname_payload IS DISTINCT FROM ?", nullableJSONField(metadata, "displayName")),
		goqu.L("payload.administrative_information_payload IS DISTINCT FROM ?", nullableJSONField(metadata, "administration")),
		goqu.L("payload.embedded_data_specification_payload IS DISTINCT FROM ?", nullableJSONField(metadata, "embeddedDataSpecifications")),
		goqu.L("payload.supplemental_semantic_ids_payload IS DISTINCT FROM ?", nullableJSONField(metadata, "supplementalSemanticIds")),
		goqu.L("payload.extensions_payload IS DISTINCT FROM ?", nullableJSONField(metadata, "extensions")),
		goqu.L("payload.qualifiers_payload IS DISTINCT FROM ?", nullableJSONField(metadata, "qualifiers")),
	)
	semanticChanged := goqu.L(
		"(? -> 'semanticId') IS DISTINCT FROM ?",
		metadata,
		currentSemanticReferenceJSON(dialect, submodelSemanticReferenceTables(), goqu.I("target.id")),
	)
	supplementalChanged := goqu.L(
		"(? -> 'supplementalReferences') IS DISTINCT FROM ?",
		metadata,
		currentSupplementalReferencesJSON(dialect, submodelSupplementalReferenceTables(), goqu.I("target.id")),
	)
	return goqu.Func(
		"jsonb_build_object",
		common.PostgreSQLTextLiteral("coreChanged"), coreChanged,
		common.PostgreSQLTextLiteral("payloadChanged"), payloadChanged,
		common.PostgreSQLTextLiteral("semanticIdChanged"), semanticChanged,
		common.PostgreSQLTextLiteral("supplementalChanged"), supplementalChanged,
	)
}

func directInsertCandidates(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(goqu.T("target_element_rows").As("target")).
		CrossJoin(goqu.T("target_submodel").As("sm")).
		LeftJoin(goqu.T("submodel_element").As("live"), goqu.On(
			goqu.I("live.submodel_id").Eq(goqu.I("sm.id")),
			goqu.I("live.idshort_path").Eq(goqu.I("target.match_key")),
		)).
		Select(
			goqu.I("target.match_key"),
			goqu.I("target.parent_key"),
			goqu.I("target.row_type"),
			goqu.I("target.ordinal"),
			goqu.I("target.row_data"),
		).
		Where(goqu.Or(
			goqu.I("live.id").IsNull(),
			goqu.I("live.model_type").Neq(goqu.I("target.row_type")),
		))
}

func stagedInsertRoots(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	parentCandidate := dialect.From(goqu.T("direct_insert_candidates").As("parent")).
		Select(goqu.L("1")).
		Where(goqu.I("parent.match_key").Eq(goqu.I("candidate.parent_key")))
	return dialect.From(goqu.T("direct_insert_candidates").As("candidate")).
		Select("candidate.match_key", "candidate.parent_key", "candidate.row_type", "candidate.ordinal", "candidate.row_data").
		Where(goqu.Or(
			goqu.I("candidate.parent_key").IsNull(),
			goqu.Func("NOT EXISTS", parentCandidate),
		))
}

func stagedInsertTree(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	seed := dialect.From("insert_roots").Select("match_key", "parent_key", "row_type", "ordinal", "row_data")
	descendants := dialect.From(goqu.T("target_element_rows").As("child")).
		Join(goqu.T("insert_tree").As("parent"), goqu.On(goqu.I("child.parent_key").Eq(goqu.I("parent.match_key")))).
		Select("child.match_key", "child.parent_key", "child.row_type", "child.ordinal", "child.row_data")
	return seed.UnionAll(descendants)
}

func stagedInsertRows(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From("insert_tree").
		Select(goqu.Func(
			"jsonb_set",
			goqu.I("row_data"),
			goqu.L("'{changes}'::text[]"),
			allStagedElementChanges(),
			true,
		).As("row")).
		Order(goqu.I("ordinal").Asc())
}

func allStagedElementChanges() exp.Expression {
	return goqu.Func(
		"jsonb_build_object",
		common.PostgreSQLTextLiteral("core"), goqu.L("TRUE"),
		common.PostgreSQLTextLiteral("payload"), goqu.L("TRUE"),
		common.PostgreSQLTextLiteral("semanticId"), goqu.L("TRUE"),
		common.PostgreSQLTextLiteral("supplementalId"), goqu.L("TRUE"),
		common.PostgreSQLTextLiteral("typeData"), goqu.L("TRUE"),
		common.PostgreSQLTextLiteral("languageValues"), goqu.L("TRUE"),
		common.PostgreSQLTextLiteral("valueId"), goqu.L("TRUE"),
	)
}

func retainedStagedRows(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	inserted := dialect.From("insert_tree").Select(goqu.L("1")).
		Where(goqu.I("insert_tree.match_key").Eq(goqu.I("target.match_key")))
	return dialect.From(goqu.T("target_element_rows").As("target")).
		CrossJoin(goqu.T("target_submodel").As("sm")).
		Join(goqu.T("submodel_element").As("live"), goqu.On(
			goqu.I("live.submodel_id").Eq(goqu.I("sm.id")),
			goqu.I("live.idshort_path").Eq(goqu.I("target.match_key")),
			goqu.I("live.model_type").Eq(goqu.I("target.row_type")),
		)).
		Select(
			goqu.I("live.id"),
			goqu.I("target.match_key"),
			goqu.I("target.parent_key"),
			goqu.I("target.row_type"),
			goqu.I("target.ordinal"),
			goqu.I("target.row_data"),
		).
		Where(goqu.Func("NOT EXISTS", inserted))
}

func classifiedStagedUpdateRows(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	changes := stagedElementChanges(dialect)
	return dialect.From(goqu.T("retained_target_rows").As("target")).
		Join(goqu.T("submodel_element").As("sme"), goqu.On(goqu.I("sme.id").Eq(goqu.I("target.id")))).
		LeftJoin(goqu.T("submodel_element_payload").As("payload"), goqu.On(goqu.I("payload.submodel_element_id").Eq(goqu.I("sme.id")))).
		Select(
			goqu.I("target.id"),
			goqu.Func("jsonb_set", goqu.I("target.row_data"), goqu.L("'{changes}'::text[]"), changes, true).As("row"),
		)
}

func stagedElementChanges(dialect goqu.DialectWrapper) exp.Expression {
	row := goqu.I("target.row_data")
	coreChanged := goqu.Or(
		goqu.L("sme.position IS DISTINCT FROM (? ->> 'position')::integer", row),
		goqu.L("sme.id_short IS DISTINCT FROM (? ->> 'idShort')", row),
		goqu.L("sme.category IS DISTINCT FROM (? ->> 'category')", row),
	)
	payloadChanged := goqu.L("? IS DISTINCT FROM (? -> 'payload')", currentElementPayloadJSON(), row)
	semanticChanged := goqu.L(
		"? IS DISTINCT FROM (? -> 'semanticId')",
		currentSemanticReferenceJSON(dialect, elementSemanticReferenceTables(), goqu.I("sme.id")),
		row,
	)
	supplementalChanged := goqu.L(
		"? IS DISTINCT FROM (? -> 'supplementalSemanticIds')",
		currentSupplementalReferencesJSON(dialect, elementSupplementalReferenceTables(), goqu.I("sme.id")),
		row,
	)
	languageChanged := goqu.L("? IS DISTINCT FROM (? -> 'languageValues')", currentLanguageValuesJSON(), row)
	valueIDChanged := goqu.L("? IS DISTINCT FROM (? -> 'valueId')", currentValueIDJSON(), row)
	return goqu.Func(
		"jsonb_build_object",
		common.PostgreSQLTextLiteral("core"), coreChanged,
		common.PostgreSQLTextLiteral("payload"), payloadChanged,
		common.PostgreSQLTextLiteral("semanticId"), semanticChanged,
		common.PostgreSQLTextLiteral("supplementalId"), supplementalChanged,
		common.PostgreSQLTextLiteral("typeData"), stagedTypeDataChanged(dialect, row),
		common.PostgreSQLTextLiteral("languageValues"), languageChanged,
		common.PostgreSQLTextLiteral("valueId"), valueIDChanged,
	)
}

func changedStagedRows(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From("classified_update_rows").
		Select("row").
		Where(goqu.Or(
			jsonChange("row", "core"),
			jsonChange("row", "payload"),
			jsonChange("row", "semanticId"),
			jsonChange("row", "supplementalId"),
			jsonChange("row", "typeData"),
			jsonChange("row", "languageValues"),
			jsonChange("row", "valueId"),
		))
}

func stagedDeleteCandidates(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	retained := dialect.From("retained_target_rows").Select(goqu.L("1")).
		Where(goqu.I("retained_target_rows.id").Eq(goqu.I("live.id")))
	return dialect.From(goqu.T("submodel_element").As("live"), goqu.T("target_submodel").As("sm")).
		Select("live.id", "live.parent_sme_id", goqu.I("live.idshort_path").As("path")).
		Where(
			goqu.I("live.submodel_id").Eq(goqu.I("sm.id")),
			goqu.Func("NOT EXISTS", retained),
		)
}

func currentElementPayloadJSON() exp.Expression {
	return goqu.Func(
		"jsonb_build_object",
		common.PostgreSQLTextLiteral("description"), goqu.L("COALESCE(payload.description_payload, '[]'::jsonb)"),
		common.PostgreSQLTextLiteral("displayName"), goqu.L("COALESCE(payload.displayname_payload, '[]'::jsonb)"),
		common.PostgreSQLTextLiteral("embeddedDataSpecifications"), goqu.L("COALESCE(payload.embedded_data_specification_payload, '[]'::jsonb)"),
		common.PostgreSQLTextLiteral("supplementalSemanticIds"), goqu.L("COALESCE(payload.supplemental_semantic_ids_payload, '[]'::jsonb)"),
		common.PostgreSQLTextLiteral("extensions"), goqu.L("COALESCE(payload.extensions_payload, '[]'::jsonb)"),
		common.PostgreSQLTextLiteral("qualifiers"), goqu.L("COALESCE(payload.qualifiers_payload, '[]'::jsonb)"),
	)
}

type reconciliationReferenceTables struct {
	reference string
	payload   string
	key       string
	owner     string
}

func submodelSemanticReferenceTables() reconciliationReferenceTables {
	return reconciliationReferenceTables{
		reference: "submodel_semantic_id_reference",
		payload:   "submodel_semantic_id_reference_payload",
		key:       "submodel_semantic_id_reference_key",
		owner:     "id",
	}
}

func submodelSupplementalReferenceTables() reconciliationReferenceTables {
	return reconciliationReferenceTables{
		reference: "submodel_supplemental_semantic_id_reference",
		payload:   "submodel_supplemental_semantic_id_reference_payload",
		key:       "submodel_supplemental_semantic_id_reference_key",
		owner:     "submodel_id",
	}
}

func elementSemanticReferenceTables() reconciliationReferenceTables {
	return reconciliationReferenceTables{
		reference: "submodel_element_semantic_id_reference",
		payload:   "submodel_element_semantic_id_reference_payload",
		key:       "submodel_element_semantic_id_reference_key",
		owner:     "id",
	}
}

func elementSupplementalReferenceTables() reconciliationReferenceTables {
	return reconciliationReferenceTables{
		reference: "submodel_element_supplemental_semantic_id_reference",
		payload:   "submodel_element_supplemental_semantic_id_reference_payload",
		key:       "submodel_element_supplemental_semantic_id_reference_key",
		owner:     "submodel_element_id",
	}
}

func currentReferenceKeysJSON(dialect goqu.DialectWrapper, tables reconciliationReferenceTables) exp.Expression {
	key := goqu.Func(
		"jsonb_build_object",
		common.PostgreSQLTextLiteral("position"), goqu.I("reference_key.position"),
		common.PostgreSQLTextLiteral("type"), goqu.I("reference_key.type"),
		common.PostgreSQLTextLiteral("value"), goqu.I("reference_key.value"),
	)
	keys := dialect.From(goqu.T(tables.key).As("reference_key")).
		Select(goqu.L("jsonb_agg(? ORDER BY ?)", key, goqu.I("reference_key.position"))).
		Where(goqu.I("reference_key.reference_id").Eq(goqu.I("reference.id")))
	return goqu.Func("COALESCE", keys, goqu.L("'[]'::jsonb"))
}

func currentSemanticReferenceJSON(dialect goqu.DialectWrapper, tables reconciliationReferenceTables, owner exp.Expression) exp.Expression {
	reference := goqu.Func(
		"jsonb_build_object",
		common.PostgreSQLTextLiteral("position"), integerLiteral(0),
		common.PostgreSQLTextLiteral("type"), goqu.I("reference.type"),
		common.PostgreSQLTextLiteral("payload"), goqu.I("payload.parent_reference_payload"),
		common.PostgreSQLTextLiteral("keys"), currentReferenceKeysJSON(dialect, tables),
	)
	query := dialect.From(goqu.T(tables.reference).As("reference")).
		LeftJoin(goqu.T(tables.payload).As("payload"), goqu.On(goqu.I("payload.reference_id").Eq(goqu.I("reference.id")))).
		Select(reference).
		Where(goqu.I("reference." + tables.owner).Eq(owner))
	return goqu.L("(?)", query)
}

func currentSupplementalReferencesJSON(dialect goqu.DialectWrapper, tables reconciliationReferenceTables, owner exp.Expression) exp.Expression {
	reference := goqu.Func(
		"jsonb_build_object",
		common.PostgreSQLTextLiteral("position"), goqu.I("reference.position"),
		common.PostgreSQLTextLiteral("type"), goqu.I("reference.type"),
		common.PostgreSQLTextLiteral("payload"), goqu.I("payload.parent_reference_payload"),
		common.PostgreSQLTextLiteral("keys"), currentReferenceKeysJSON(dialect, tables),
	)
	query := dialect.From(goqu.T(tables.reference).As("reference")).
		LeftJoin(goqu.T(tables.payload).As("payload"), goqu.On(goqu.I("payload.reference_id").Eq(goqu.I("reference.id")))).
		Select(goqu.L("COALESCE(jsonb_agg(? ORDER BY ?), '[]'::jsonb)", reference, goqu.I("reference.position"))).
		Where(goqu.I("reference." + tables.owner).Eq(owner))
	return goqu.L("(?)", query)
}

func currentLanguageValuesJSON() exp.Expression {
	return goqu.L(`(SELECT COALESCE(jsonb_agg(
		jsonb_build_object('language', value.language, 'text', value.text)
		ORDER BY value.language, value.text
	), '[]'::jsonb)
	FROM multilanguage_property_value AS value
	WHERE value.submodel_element_id = sme.id)`)
}

func currentValueIDJSON() exp.Expression {
	return goqu.Case().
		When(
			goqu.And(
				jsonText("target.row_data", "typeTable").Eq(common.PostgreSQLTextLiteral("")),
				jsonInt("target.row_data", "modelType").Eq(integerLiteral(int64(types.ModelTypeMultiLanguageProperty))),
			),
			goqu.L("COALESCE((SELECT value_id_payload FROM multilanguage_property_payload WHERE submodel_element_id = sme.id), 'null'::jsonb)"),
		).
		When(
			jsonText("target.row_data", "typeTable").Eq(common.PostgreSQLTextLiteral("property_element")),
			goqu.L("COALESCE((SELECT value_id_payload FROM property_element_payload WHERE property_element_id = sme.id), 'null'::jsonb)"),
		).
		Else(goqu.L("'null'::jsonb"))
}

func stagedTypeDataChanged(dialect goqu.DialectWrapper, row exp.Expression) exp.Expression {
	result := goqu.Case()
	for _, spec := range reconciliationTypeSpecs() {
		alias := "current_" + spec.table
		conditions := []exp.Expression{goqu.I(alias + ".id").Eq(goqu.I("sme.id"))}
		for _, column := range spec.columns {
			if spec.table == "file_element" && column.name == "file_name" {
				continue
			}
			conditions = append(conditions, goqu.L("? IS NOT DISTINCT FROM ?", goqu.I(alias+"."+column.name), column.expr("target.row_data")))
		}
		matches := dialect.From(goqu.T(spec.table).As(alias)).Select(goqu.L("1")).Where(conditions...)
		result = result.When(
			goqu.L("? ->> 'typeTable' = ?", row, common.PostgreSQLTextLiteral(spec.table)),
			goqu.Func("NOT EXISTS", matches),
		)
	}
	return result.Else(goqu.L("(? ->> 'typeTable') <> '' OR (? -> 'typeData') IS DISTINCT FROM '{}'::jsonb", row, row))
}

func verifyStagedSubmodelStateTx(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
	staged *stagedSubmodelTarget,
) error {
	if staged == nil || staged.stage == nil {
		return common.NewInternalServerError("SMREPO-VERIFY-NILSTAGE staged target must not be nil")
	}
	querySQL, args, err := buildStagedSubmodelVerificationQuery(submodelID, staged)
	if err != nil {
		return common.NewInternalServerError("SMREPO-VERIFY-BUILD " + err.Error())
	}
	var missingSubmodel bool
	var metadataDiff bool
	var missingTarget bool
	var extraCurrent bool
	var changedCurrent bool
	if err = tx.QueryRowContext(ctx, querySQL, args...).Scan(&missingSubmodel, &metadataDiff, &missingTarget, &extraCurrent, &changedCurrent); err != nil {
		return common.NewInternalServerError("SMREPO-VERIFY-EXEC " + err.Error())
	}
	if missingSubmodel || metadataDiff || missingTarget || extraCurrent || changedCurrent {
		return common.NewInternalServerError("SMREPO-PUTSM-READBACKMISMATCH persisted Submodel does not match staged replacement")
	}
	return nil
}

func buildStagedSubmodelVerificationQuery(submodelID string, staged *stagedSubmodelTarget) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	metadataRows := staged.stage.Dataset(dialect, submodelMetadataStageDataset)
	elementRows := staged.stage.Dataset(dialect, submodelElementStageDataset)
	targetSubmodel := dialect.From("submodel").Select("id").Where(goqu.C("submodel_identifier").Eq(submodelID))

	metadataChanges := stagedMetadataChanges(dialect, goqu.I("metadata.data"))
	metadataMismatch := dialect.From(goqu.T("target_metadata").As("metadata")).
		CrossJoin(goqu.T("target_submodel").As("target")).
		Join(goqu.T("submodel").As("sm"), goqu.On(goqu.I("sm.id").Eq(goqu.I("target.id")))).
		LeftJoin(goqu.T("submodel_payload").As("payload"), goqu.On(goqu.I("payload.submodel_id").Eq(goqu.I("target.id")))).
		Select(goqu.L("1")).
		Where(stagedChangesAny(metadataChanges, "coreChanged", "payloadChanged", "semanticIdChanged", "supplementalChanged"))

	targetMismatch := dialect.From(goqu.T("target_element_rows").As("target")).
		CrossJoin(goqu.T("target_submodel").As("sm")).
		LeftJoin(goqu.T("submodel_element").As("live"), goqu.On(
			goqu.I("live.submodel_id").Eq(goqu.I("sm.id")),
			goqu.I("live.idshort_path").Eq(goqu.I("target.match_key")),
			goqu.I("live.model_type").Eq(goqu.I("target.row_type")),
		)).
		Select(goqu.L("1")).
		Where(goqu.I("live.id").IsNull())

	extraLive := dialect.From(goqu.T("submodel_element").As("live")).
		CrossJoin(goqu.T("target_submodel").As("sm")).
		LeftJoin(goqu.T("target_element_rows").As("target"), goqu.On(
			goqu.I("target.match_key").Eq(goqu.I("live.idshort_path")),
			goqu.I("target.row_type").Eq(goqu.I("live.model_type")),
		)).
		Select(goqu.L("1")).
		Where(
			goqu.I("live.submodel_id").Eq(goqu.I("sm.id")),
			goqu.I("target.match_key").IsNull(),
		)

	elementChanges := stagedElementChanges(dialect)
	changedTarget := dialect.From(goqu.T("target_element_rows").As("target")).
		CrossJoin(goqu.T("target_submodel").As("sm")).
		Join(goqu.T("submodel_element").As("sme"), goqu.On(
			goqu.I("sme.submodel_id").Eq(goqu.I("sm.id")),
			goqu.I("sme.idshort_path").Eq(goqu.I("target.match_key")),
			goqu.I("sme.model_type").Eq(goqu.I("target.row_type")),
		)).
		LeftJoin(goqu.T("submodel_element_payload").As("payload"), goqu.On(goqu.I("payload.submodel_element_id").Eq(goqu.I("sme.id")))).
		Select(goqu.L("1")).
		Where(stagedChangesAny(elementChanges, "core", "payload", "semanticId", "supplementalId", "typeData", "languageValues", "valueId"))

	query := dialect.Select(
		goqu.L("NOT EXISTS (SELECT 1 FROM target_submodel)"),
		goqu.Func("EXISTS", metadataMismatch),
		goqu.Func("EXISTS", targetMismatch),
		goqu.Func("EXISTS", extraLive),
		goqu.Func("EXISTS", changedTarget),
	).
		With("target_submodel", targetSubmodel).
		With("target_metadata_rows", metadataRows).
		With("target_metadata", dialect.From("target_metadata_rows").Select(goqu.I("row_data").As("data"))).
		With("target_element_rows", elementRows).
		Prepared(true)
	return query.ToSQL()
}

func stagedChangesAny(changes exp.Expression, fields ...string) exp.Expression {
	expressions := make([]exp.Expression, 0, len(fields))
	for _, field := range fields {
		expressions = append(expressions, goqu.L(
			"COALESCE((? ->> ?)::boolean, false)",
			changes,
			common.PostgreSQLTextLiteral(field),
		))
	}
	return goqu.Or(expressions...)
}
