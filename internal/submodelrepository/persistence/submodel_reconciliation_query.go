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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/postgresstaging"
)

type reconciliationMutationResult struct {
	UpdatedElements  int
	InsertedElements int
	DeletedElements  int
}

type reconciliationCTE struct {
	name      string
	query     exp.Expression
	recursive bool
}

type reconciliationQueryBuilder struct {
	dialect goqu.DialectWrapper
	ctes    []reconciliationCTE
}

func newReconciliationQueryBuilder() *reconciliationQueryBuilder {
	return &reconciliationQueryBuilder{dialect: goqu.Dialect(common.Dialect)}
}

func (b *reconciliationQueryBuilder) add(name string, query exp.Expression) {
	b.ctes = append(b.ctes, reconciliationCTE{name: name, query: query})
}

func (b *reconciliationQueryBuilder) addRecursive(name string, query exp.Expression) {
	b.ctes = append(b.ctes, reconciliationCTE{name: name, query: query, recursive: true})
}

func (b *reconciliationQueryBuilder) build(stage *postgresstaging.Stage, submodelID string) (string, []any, error) {
	b.addInputs(stage, submodelID)
	b.addResolutionAndCleanupCTEs()
	b.addSubmodelMetadataCTEs()
	b.addElementBaseCTEs()
	b.addElementReferenceCTEs()
	b.addTypeSpecificCTEs()

	final := b.dialect.Select(
		goqu.L("(SELECT COUNT(*) FROM resolved_update_rows)").As("updated_count"),
		goqu.L("(SELECT COUNT(*) FROM inserted_element_rows)").As("inserted_count"),
		goqu.L("(SELECT COUNT(*) FROM deleted_element_rows)").As("deleted_count"),
	)
	for _, cte := range b.ctes {
		if cte.recursive {
			final = final.WithRecursive(cte.name, cte.query)
		} else {
			final = final.With(cte.name, cte.query)
		}
	}
	return final.Prepared(true).ToSQL()
}

func (b *reconciliationQueryBuilder) addInputs(stage *postgresstaging.Stage, submodelID string) {
	b.add("target_submodel", b.dialect.From("submodel").
		Select("id").
		Where(goqu.C("submodel_identifier").Eq(submodelID)))
	b.addStagedReconciliationInputs(stage)
	b.add("owned_managed_files", b.dialect.From(goqu.T("target_submodel").As("sm")).
		Join(goqu.T("submodel_element").As("sme"), goqu.On(goqu.I("sme.submodel_id").Eq(goqu.I("sm.id")))).
		Join(goqu.T("file_element").As("fe"), goqu.On(goqu.I("fe.id").Eq(goqu.I("sme.id")))).
		Join(goqu.T("file_binary_reference").As("fr"), goqu.On(goqu.I("fr.file_element_id").Eq(goqu.I("sme.id")))).
		Select(
			goqu.I("sme.idshort_path").As("path"),
			goqu.I("fe.value").As("value"),
			goqu.I("fr.binary_content_id"),
			goqu.I("fr.path_token"),
			goqu.I("fr.safe_file_name"),
		))
	b.add("update_rows", sanitizeManagedFileRows(b.dialect, "raw_update_rows"))
	b.add("insert_rows", sanitizeManagedFileRows(b.dialect, "raw_insert_rows"))
}

func sanitizeManagedFileRows(dialect goqu.DialectWrapper, source string) *goqu.SelectDataset {
	row := goqu.I("source.row")
	value := goqu.L("? -> 'typeData' ->> 'value'", row)
	path := goqu.L("? ->> 'path'", row)
	reassigned := dialect.From(goqu.T("owned_managed_files").As("owned")).
		Select(goqu.L("1")).
		Where(
			goqu.I("owned.value").Eq(value),
			goqu.I("owned.path").Neq(path),
		)
	return dialect.From(goqu.T(source).As("source")).Select(
		goqu.Case().
			When(
				goqu.And(
					goqu.L("? ->> 'typeTable' = 'file_element'", row),
					goqu.Func("EXISTS", reassigned),
				),
				goqu.Func("jsonb_set", row, goqu.L("'{typeData,value}'::text[]"), goqu.L("'null'::jsonb"), goqu.L("TRUE")),
			).
			Else(row).
			As("row"),
	)
}

func (b *reconciliationQueryBuilder) addResolutionAndCleanupCTEs() {
	b.add("resolved_update_rows", b.dialect.From(goqu.T("update_rows").As("u")).
		Join(goqu.T("target_submodel").As("sm"), goqu.On(goqu.L("TRUE"))).
		Join(goqu.T("submodel_element").As("sme"), goqu.On(
			goqu.I("sme.submodel_id").Eq(goqu.I("sm.id")),
			goqu.I("sme.idshort_path").Eq(jsonText("u.row", "path")),
		)).
		Select(goqu.I("sme.id"), goqu.I("u.row")))
	b.add("allocated_insert_rows", b.dialect.From(goqu.T("insert_rows").As("i")).Select(
		goqu.Func("nextval", goqu.Func("pg_get_serial_sequence", common.PostgreSQLTextLiteral("submodel_element"), common.PostgreSQLTextLiteral("id"))).As("id"),
		goqu.I("i.row"),
	))
	b.add("resolved_insert_rows", resolvedInsertRows(b.dialect))
	b.add("delete_element_targets", deleteElementTargets(b.dialect))
	b.add("changed_file_rows", changedFileRows(b.dialect))
	b.add("managed_file_transfers", managedFileTransfers(b.dialect))
	b.add("file_cleanup_targets", b.dialect.From(goqu.T("delete_element_targets").As("d")).Select("d.id").
		Union(b.dialect.From(goqu.T("changed_file_rows").As("f")).Select("f.id")))
	b.add("unlinked_large_objects", b.dialect.From(goqu.T("file_data").As("fd")).
		Join(goqu.T("file_cleanup_targets").As("cleanup"), goqu.On(goqu.I("cleanup.id").Eq(goqu.I("fd.id")))).
		Select(goqu.Func("lo_unlink", goqu.I("fd.file_oid")).As("unlinked")))
	b.add("deleted_changed_file_references", b.dialect.Delete("file_binary_reference").
		Where(goqu.Func("EXISTS", b.dialect.From(goqu.T("changed_file_rows").As("changed")).
			Select(goqu.L("1")).
			Where(goqu.I("file_binary_reference.file_element_id").Eq(goqu.I("changed.id"))))).
		Returning(goqu.I("file_binary_reference.file_element_id")))
	b.add("deleted_changed_file_data", b.dialect.Delete("file_data").
		Where(
			goqu.Func("EXISTS", b.dialect.From(goqu.T("changed_file_rows").As("changed")).
				Select(goqu.L("1")).
				Where(goqu.I("file_data.id").Eq(goqu.I("changed.id")))),
			goqu.L("(SELECT COUNT(*) FROM unlinked_large_objects) >= 0"),
		).
		Returning(goqu.I("file_data.id")))
	b.add("deleted_element_rows", b.dialect.Delete("submodel_element").
		Where(
			goqu.Func("EXISTS", b.dialect.From(goqu.T("delete_element_targets").As("target")).
				Select(goqu.L("1")).
				Where(goqu.I("submodel_element.id").Eq(goqu.I("target.id")))),
			goqu.L("(SELECT COUNT(*) FROM unlinked_large_objects) >= 0"),
		).
		Returning(goqu.I("submodel_element.id")))
}

func resolvedInsertRows(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	i := goqu.T("allocated_insert_rows").As("i")
	return dialect.From(i).
		CrossJoin(goqu.T("target_submodel").As("sm")).
		LeftJoin(goqu.T("allocated_insert_rows").As("new_parent"), goqu.On(
			jsonText("new_parent.row", "path").Eq(jsonText("i.row", "parentPath")),
		)).
		LeftJoin(goqu.T("submodel_element").As("old_parent"), goqu.On(
			goqu.I("old_parent.submodel_id").Eq(goqu.I("sm.id")),
			goqu.I("old_parent.idshort_path").Eq(jsonText("i.row", "parentPath")),
		)).
		LeftJoin(goqu.T("allocated_insert_rows").As("new_root"), goqu.On(
			jsonText("new_root.row", "path").Eq(jsonText("i.row", "rootPath")),
		)).
		LeftJoin(goqu.T("submodel_element").As("old_root"), goqu.On(
			goqu.I("old_root.submodel_id").Eq(goqu.I("sm.id")),
			goqu.I("old_root.idshort_path").Eq(jsonText("i.row", "rootPath")),
		)).
		Select(
			goqu.I("i.id"),
			goqu.I("i.row"),
			goqu.Case().When(jsonText("i.row", "parentPath").Eq(common.PostgreSQLTextLiteral("")), goqu.L("NULL")).
				Else(goqu.Func("COALESCE", goqu.I("new_parent.id"), goqu.I("old_parent.id"))).As("parent_id"),
			goqu.Func("COALESCE", goqu.I("new_root.id"), goqu.I("old_root.id"), goqu.I("i.id")).As("root_id"),
		)
}

func deleteElementTargets(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From("delete_candidates").Select("id")
}

func changedFileRows(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(goqu.T("resolved_update_rows").As("u")).
		Join(goqu.T("file_element").As("fe"), goqu.On(goqu.I("fe.id").Eq(goqu.I("u.id")))).
		Select(goqu.I("u.id")).
		Where(
			jsonText("u.row", "typeTable").Eq(common.PostgreSQLTextLiteral("file_element")),
			goqu.L("fe.value IS DISTINCT FROM ?", jsonNestedText("u.row", "typeData", "value")),
		)
}

func managedFileTransfers(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(goqu.T("resolved_insert_rows").As("i")).
		Join(goqu.T("owned_managed_files").As("owned"), goqu.On(
			goqu.I("owned.path").Eq(jsonText("i.row", "path")),
			goqu.I("owned.value").Eq(jsonNestedText("i.row", "typeData", "value")),
		)).
		Select(
			goqu.I("i.id"),
			goqu.I("owned.binary_content_id"),
			goqu.I("owned.path_token"),
			goqu.I("owned.safe_file_name"),
		)
}

func (b *reconciliationQueryBuilder) addSubmodelMetadataCTEs() {
	metadata := goqu.L("p.data -> 'metadata'")
	b.add("updated_submodel_metadata", b.dialect.Update(goqu.T("submodel").As("sm")).
		Set(goqu.Record{
			"id_short": goqu.L("? ->> 'idShort'", metadata),
			"category": goqu.L("? ->> 'category'", metadata),
			"kind":     goqu.L("(? ->> 'kind')::integer", metadata),
		}).
		From(goqu.T("reconciliation_plan").As("p"), goqu.T("target_submodel").As("target")).
		Where(
			goqu.I("sm.id").Eq(goqu.I("target.id")),
			goqu.L("COALESCE((? ->> 'coreChanged')::boolean, false)", metadata),
		).
		Returning(goqu.I("sm.id")))
	b.add("upserted_submodel_payload", upsertSubmodelPayload(b.dialect))
	b.add("deleted_submodel_semantic_id", b.dialect.Delete("submodel_semantic_id_reference").
		Where(
			goqu.Func("EXISTS", b.dialect.From(goqu.T("target_submodel").As("target"), goqu.T("reconciliation_plan").As("p")).
				Select(goqu.L("1")).
				Where(
					goqu.I("submodel_semantic_id_reference.id").Eq(goqu.I("target.id")),
					goqu.L("COALESCE((p.data -> 'metadata' ->> 'semanticIdChanged')::boolean, false)"),
					goqu.Or(
						goqu.L("p.data -> 'metadata' -> 'semanticId' IS NULL"),
						goqu.L("p.data -> 'metadata' -> 'semanticId' = 'null'::jsonb"),
					),
				)),
		).
		Returning(goqu.I("submodel_semantic_id_reference.id")))
	b.add("inserted_submodel_semantic_id", insertSubmodelSemanticReference(b.dialect))
	b.add("updated_submodel_semantic_payload", updateSubmodelSemanticPayload(b.dialect))
	b.add("inserted_submodel_semantic_payload", insertSubmodelSemanticPayload(b.dialect))
	b.add("deleted_submodel_semantic_keys", deleteSubmodelSemanticKeys(b.dialect))
	b.add("inserted_submodel_semantic_keys", insertSubmodelSemanticKeys(b.dialect))
	b.add("deleted_submodel_supplemental", deleteRootSupplementalReferences(b.dialect))
	b.add("submodel_supplemental_rows", rootSupplementalRows(b.dialect))
	b.add("inserted_submodel_supplemental", insertRootSupplementalBase(b.dialect))
	b.add("updated_submodel_supplemental_payload", updateRootSupplementalPayload(b.dialect))
	b.add("inserted_submodel_supplemental_payload", insertRootSupplementalPayload(b.dialect))
	b.add("deleted_submodel_supplemental_keys", deleteRootSupplementalKeys(b.dialect))
	b.add("inserted_submodel_supplemental_keys", insertRootSupplementalKeys(b.dialect))
}

func upsertSubmodelPayload(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	metadata := goqu.L("p.data -> 'metadata'")
	source := dialect.From(goqu.T("target_submodel").As("target"), goqu.T("reconciliation_plan").As("p")).
		Select(
			goqu.I("target.id"),
			nullableJSONField(metadata, "description"),
			nullableJSONField(metadata, "displayName"),
			nullableJSONField(metadata, "administration"),
			nullableJSONField(metadata, "embeddedDataSpecifications"),
			nullableJSONField(metadata, "supplementalSemanticIds"),
			nullableJSONField(metadata, "extensions"),
			nullableJSONField(metadata, "qualifiers"),
		).
		Where(goqu.L("COALESCE((? ->> 'payloadChanged')::boolean, false)", metadata))
	update := excludedRecord("description_payload", "displayname_payload", "administrative_information_payload", "embedded_data_specification_payload", "supplemental_semantic_ids_payload", "extensions_payload", "qualifiers_payload")
	return dialect.Insert("submodel_payload").
		Cols("submodel_id", "description_payload", "displayname_payload", "administrative_information_payload", "embedded_data_specification_payload", "supplemental_semantic_ids_payload", "extensions_payload", "qualifiers_payload").
		FromQuery(source).
		OnConflict(goqu.DoUpdate("submodel_id", update)).
		Returning("submodel_id")
}

func insertSubmodelSemanticReference(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(goqu.T("target_submodel").As("target"), goqu.T("reconciliation_plan").As("p")).
		Select(goqu.I("target.id"), goqu.L("(p.data -> 'metadata' -> 'semanticId' ->> 'type')::integer")).
		Where(
			goqu.L("p.data -> 'metadata' -> 'semanticId' IS NOT NULL"),
			goqu.L("p.data -> 'metadata' -> 'semanticId' <> 'null'::jsonb"),
			goqu.L("COALESCE((p.data -> 'metadata' ->> 'semanticIdChanged')::boolean, false)"),
			goqu.L("(SELECT COUNT(*) FROM deleted_submodel_semantic_id) >= 0"),
		)
	return dialect.Insert("submodel_semantic_id_reference").
		Cols("id", "type").
		FromQuery(source).
		OnConflict(goqu.DoUpdate("id", excludedRecord("type"))).
		Returning("id")
}

func updateSubmodelSemanticPayload(dialect goqu.DialectWrapper) *goqu.UpdateDataset {
	return dialect.Update(goqu.T("submodel_semantic_id_reference_payload").As("payload")).
		Set(goqu.Record{"parent_reference_payload": goqu.L("p.data -> 'metadata' -> 'semanticId' -> 'payload'")}).
		From(goqu.T("inserted_submodel_semantic_id").As("inserted"), goqu.T("reconciliation_plan").As("p")).
		Where(goqu.I("payload.reference_id").Eq(goqu.I("inserted.id"))).
		Returning(goqu.I("payload.reference_id"))
}

func insertSubmodelSemanticPayload(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(goqu.T("inserted_submodel_semantic_id").As("inserted"), goqu.T("reconciliation_plan").As("p")).
		Select(goqu.I("inserted.id"), goqu.L("p.data -> 'metadata' -> 'semanticId' -> 'payload'")).
		Where(
			goqu.L("(SELECT COUNT(*) FROM updated_submodel_semantic_payload) >= 0"),
			goqu.Func("NOT EXISTS", dialect.From("submodel_semantic_id_reference_payload").
				Select(goqu.L("1")).
				Where(goqu.I("reference_id").Eq(goqu.I("inserted.id")))),
		)
	return dialect.Insert("submodel_semantic_id_reference_payload").Cols("reference_id", "parent_reference_payload").FromQuery(source).Returning("reference_id")
}

func deleteSubmodelSemanticKeys(dialect goqu.DialectWrapper) *goqu.DeleteDataset {
	matchingPosition := dialect.From(
		goqu.T("reconciliation_plan").As("p"),
		goqu.L("jsonb_array_elements(COALESCE(p.data -> 'metadata' -> 'semanticId' -> 'keys', '[]'::jsonb))").As("key"),
	).Select(goqu.L("1")).Where(
		jsonInt("key", "position").Eq(goqu.I("submodel_semantic_id_reference_key.position")),
	)
	matchingReference := dialect.From(goqu.T("inserted_submodel_semantic_id").As("inserted")).
		Select(goqu.L("1")).
		Where(
			goqu.I("submodel_semantic_id_reference_key.reference_id").Eq(goqu.I("inserted.id")),
			goqu.Func("NOT EXISTS", matchingPosition),
		)
	return dialect.Delete("submodel_semantic_id_reference_key").
		Where(goqu.Func("EXISTS", matchingReference)).
		Returning(goqu.I("submodel_semantic_id_reference_key.reference_id"))
}

func insertSubmodelSemanticKeys(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(
		goqu.T("inserted_submodel_semantic_id").As("inserted"),
		goqu.T("reconciliation_plan").As("p"),
		goqu.L("jsonb_array_elements(COALESCE(p.data -> 'metadata' -> 'semanticId' -> 'keys', '[]'::jsonb))").As("key"),
	).Select(
		goqu.I("inserted.id"),
		goqu.L("(key ->> 'position')::integer"),
		goqu.L("(key ->> 'type')::integer"),
		goqu.L("key ->> 'value'"),
	)
	return dialect.Insert("submodel_semantic_id_reference_key").
		Cols("reference_id", "position", "type", "value").
		FromQuery(source).
		OnConflict(goqu.DoUpdate("reference_id,position", excludedRecord("type", "value"))).
		Returning("reference_id")
}

func rootSupplementalRows(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(
		goqu.T("target_submodel").As("target"),
		goqu.T("reconciliation_plan").As("p"),
		goqu.L("jsonb_array_elements(COALESCE(p.data -> 'metadata' -> 'supplementalReferences', '[]'::jsonb))").As("ref"),
	).Select(
		goqu.I("target.id").As("owner_id"),
		goqu.I("ref").As("row"),
	).Where(
		goqu.L("COALESCE((p.data -> 'metadata' ->> 'supplementalChanged')::boolean, false)"),
		goqu.L("(SELECT COUNT(*) FROM deleted_submodel_supplemental) >= 0"),
	)
}

func deleteRootSupplementalReferences(dialect goqu.DialectWrapper) *goqu.DeleteDataset {
	matchingPosition := dialect.From(
		goqu.T("reconciliation_plan").As("p"),
		goqu.L("jsonb_array_elements(COALESCE(p.data -> 'metadata' -> 'supplementalReferences', '[]'::jsonb))").As("ref"),
	).Select(goqu.L("1")).Where(
		jsonInt("ref", "position").Eq(goqu.I("submodel_supplemental_semantic_id_reference.position")),
	)
	matchingOwner := dialect.From(goqu.T("target_submodel").As("target"), goqu.T("reconciliation_plan").As("p")).
		Select(goqu.L("1")).
		Where(
			goqu.I("submodel_supplemental_semantic_id_reference.submodel_id").Eq(goqu.I("target.id")),
			goqu.L("COALESCE((p.data -> 'metadata' ->> 'supplementalChanged')::boolean, false)"),
			goqu.Func("NOT EXISTS", matchingPosition),
		)
	return dialect.Delete("submodel_supplemental_semantic_id_reference").
		Where(goqu.Func("EXISTS", matchingOwner)).
		Returning(goqu.I("submodel_supplemental_semantic_id_reference.id"))
}

func insertRootSupplementalBase(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From("submodel_supplemental_rows").Select(
		"owner_id", jsonInt("row", "position"), jsonInt("row", "type"),
	)
	return dialect.Insert("submodel_supplemental_semantic_id_reference").
		Cols("submodel_id", "position", "type").
		FromQuery(source).
		OnConflict(goqu.DoUpdate("submodel_id,position", excludedRecord("type"))).
		Returning("id", "submodel_id", "position")
}

func updateRootSupplementalPayload(dialect goqu.DialectWrapper) *goqu.UpdateDataset {
	return dialect.Update(goqu.T("submodel_supplemental_semantic_id_reference_payload").As("payload")).
		Set(goqu.Record{"parent_reference_payload": jsonObject("source.row", "payload")}).
		From(goqu.T("submodel_supplemental_rows").As("source"), goqu.T("inserted_submodel_supplemental").As("inserted")).
		Where(
			goqu.I("inserted.submodel_id").Eq(goqu.I("source.owner_id")),
			goqu.I("inserted.position").Eq(jsonInt("source.row", "position")),
			goqu.I("payload.reference_id").Eq(goqu.I("inserted.id")),
		).
		Returning(goqu.I("payload.reference_id"))
}

func insertRootSupplementalPayload(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(goqu.T("submodel_supplemental_rows").As("source")).
		Join(goqu.T("inserted_submodel_supplemental").As("inserted"), goqu.On(
			goqu.I("inserted.submodel_id").Eq(goqu.I("source.owner_id")),
			goqu.I("inserted.position").Eq(jsonInt("source.row", "position")),
		)).
		Select(goqu.I("inserted.id"), jsonObject("source.row", "payload")).
		Where(
			goqu.L("(SELECT COUNT(*) FROM updated_submodel_supplemental_payload) >= 0"),
			goqu.Func("NOT EXISTS", dialect.From("submodel_supplemental_semantic_id_reference_payload").
				Select(goqu.L("1")).
				Where(goqu.I("reference_id").Eq(goqu.I("inserted.id")))),
		)
	return dialect.Insert("submodel_supplemental_semantic_id_reference_payload").Cols("reference_id", "parent_reference_payload").FromQuery(source).Returning("reference_id")
}

func deleteRootSupplementalKeys(dialect goqu.DialectWrapper) *goqu.DeleteDataset {
	matchingPosition := dialect.From(
		goqu.T("submodel_supplemental_rows").As("source"),
		goqu.T("inserted_submodel_supplemental").As("inserted"),
		goqu.L("jsonb_array_elements(COALESCE(source.row -> 'keys', '[]'::jsonb))").As("key"),
	).Select(goqu.L("1")).Where(
		goqu.I("inserted.submodel_id").Eq(goqu.I("source.owner_id")),
		goqu.I("inserted.position").Eq(jsonInt("source.row", "position")),
		goqu.I("submodel_supplemental_semantic_id_reference_key.reference_id").Eq(goqu.I("inserted.id")),
		jsonInt("key", "position").Eq(goqu.I("submodel_supplemental_semantic_id_reference_key.position")),
	)
	matchingReference := dialect.From(goqu.T("inserted_submodel_supplemental").As("inserted")).
		Select(goqu.L("1")).
		Where(
			goqu.I("submodel_supplemental_semantic_id_reference_key.reference_id").Eq(goqu.I("inserted.id")),
			goqu.Func("NOT EXISTS", matchingPosition),
		)
	return dialect.Delete("submodel_supplemental_semantic_id_reference_key").
		Where(goqu.Func("EXISTS", matchingReference)).
		Returning(goqu.I("submodel_supplemental_semantic_id_reference_key.reference_id"))
}

func insertRootSupplementalKeys(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(goqu.T("submodel_supplemental_rows").As("source")).
		Join(goqu.T("inserted_submodel_supplemental").As("inserted"), goqu.On(
			goqu.I("inserted.submodel_id").Eq(goqu.I("source.owner_id")),
			goqu.I("inserted.position").Eq(jsonInt("source.row", "position")),
		)).CrossJoin(goqu.L("jsonb_array_elements(COALESCE(source.row -> 'keys', '[]'::jsonb))").As("key")).Select(
		goqu.I("inserted.id"), jsonInt("key", "position"), jsonInt("key", "type"), jsonText("key", "value"),
	)
	return dialect.Insert("submodel_supplemental_semantic_id_reference_key").
		Cols("reference_id", "position", "type", "value").
		FromQuery(source).
		OnConflict(goqu.DoUpdate("reference_id,position", excludedRecord("type", "value"))).
		Returning("reference_id")
}

func (b *reconciliationQueryBuilder) addElementBaseCTEs() {
	b.add("updated_element_rows", updateElementBaseRows(b.dialect))
	b.add("inserted_element_rows", insertElementBaseRows(b.dialect))
	b.add("affected_element_rows", affectedElementRows(b.dialect))
	b.add("upserted_element_payloads", upsertElementPayloads(b.dialect))
}

func updateElementBaseRows(dialect goqu.DialectWrapper) *goqu.UpdateDataset {
	return dialect.Update(goqu.T("submodel_element").As("sme")).Set(goqu.Record{
		"position": jsonInt("u.row", "position"),
		"id_short": jsonText("u.row", "idShort"),
		"category": jsonText("u.row", "category"),
	}).From(goqu.T("resolved_update_rows").As("u")).
		Where(
			goqu.I("sme.id").Eq(goqu.I("u.id")),
			jsonChange("u.row", "core"),
		).
		Returning(goqu.I("sme.id"), goqu.I("sme.idshort_path").As("path"))
}

func insertElementBaseRows(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(goqu.T("resolved_insert_rows").As("i"), goqu.T("target_submodel").As("sm")).Select(
		goqu.I("i.id"),
		goqu.I("sm.id"),
		goqu.I("i.parent_id"),
		jsonInt("i.row", "position"),
		jsonText("i.row", "idShort"),
		jsonText("i.row", "category"),
		jsonInt("i.row", "modelType"),
		jsonText("i.row", "path"),
		goqu.I("i.root_id"),
		jsonInt("i.row", "depth"),
	).Where(goqu.L("(SELECT COUNT(*) FROM deleted_element_rows) >= 0"))
	return dialect.Insert("submodel_element").
		Cols("id", "submodel_id", "parent_sme_id", "position", "id_short", "category", "model_type", "idshort_path", "root_sme_id", "depth").
		FromQuery(source).
		Returning("id", "idshort_path")
}

func affectedElementRows(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	updated := dialect.From(goqu.T("resolved_update_rows").As("source")).
		Select(goqu.I("source.id"), goqu.I("source.row")).
		Where(goqu.L("(SELECT COUNT(*) FROM updated_element_rows) >= 0"))
	inserted := dialect.From(goqu.T("resolved_insert_rows").As("source")).
		Join(goqu.T("inserted_element_rows").As("inserted"), goqu.On(goqu.I("inserted.id").Eq(goqu.I("source.id")))).
		Select(goqu.I("source.id"), goqu.I("source.row"))
	return updated.UnionAll(inserted)
}

func upsertElementPayloads(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(goqu.T("affected_element_rows").As("a")).Select(
		goqu.I("a.id"),
		jsonNestedObject("a.row", "payload", "description"),
		jsonNestedObject("a.row", "payload", "displayName"),
		jsonNestedObject("a.row", "payload", "embeddedDataSpecifications"),
		jsonNestedObject("a.row", "payload", "supplementalSemanticIds"),
		jsonNestedObject("a.row", "payload", "extensions"),
		jsonNestedObject("a.row", "payload", "qualifiers"),
	).Where(jsonChange("a.row", "payload"))
	columns := []string{"description_payload", "displayname_payload", "embedded_data_specification_payload", "supplemental_semantic_ids_payload", "extensions_payload", "qualifiers_payload"}
	return dialect.Insert("submodel_element_payload").
		Cols(stringInterfaces(append([]string{"submodel_element_id"}, columns...))...).
		FromQuery(source).
		OnConflict(goqu.DoUpdate("submodel_element_id", excludedRecord(columns...))).
		Returning("submodel_element_id")
}

func (b *reconciliationQueryBuilder) addElementReferenceCTEs() {
	b.add("deleted_element_semantic_ids", b.dialect.Delete("submodel_element_semantic_id_reference").
		Where(goqu.Func("EXISTS", b.dialect.From(goqu.T("resolved_update_rows").As("u")).
			Select(goqu.L("1")).
			Where(
				goqu.I("submodel_element_semantic_id_reference.id").Eq(goqu.I("u.id")),
				jsonChange("u.row", "semanticId"),
				goqu.Or(
					goqu.L("u.row -> 'semanticId' IS NULL"),
					goqu.L("u.row -> 'semanticId' = 'null'::jsonb"),
				),
			))).
		Returning(goqu.I("submodel_element_semantic_id_reference.id")))
	b.add("inserted_element_semantic_ids", insertElementSemanticBase(b.dialect))
	b.add("updated_element_semantic_payloads", updateElementSemanticPayloads(b.dialect))
	b.add("inserted_element_semantic_payloads", insertElementSemanticPayloads(b.dialect))
	b.add("deleted_element_semantic_keys", deleteElementSemanticKeys(b.dialect))
	b.add("inserted_element_semantic_keys", insertElementSemanticKeys(b.dialect))
	b.add("deleted_element_supplemental", deleteElementSupplementalReferences(b.dialect))
	b.add("element_supplemental_rows", elementSupplementalRows(b.dialect))
	b.add("inserted_element_supplemental", insertElementSupplementalBase(b.dialect))
	b.add("updated_element_supplemental_payloads", updateElementSupplementalPayloads(b.dialect))
	b.add("inserted_element_supplemental_payloads", insertElementSupplementalPayloads(b.dialect))
	b.add("deleted_element_supplemental_keys", deleteElementSupplementalKeys(b.dialect))
	b.add("inserted_element_supplemental_keys", insertElementSupplementalKeys(b.dialect))
}

func insertElementSemanticBase(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(goqu.T("affected_element_rows").As("a")).Select(
		goqu.I("a.id"), jsonNestedInt("a.row", "semanticId", "type"),
	).Where(
		jsonChange("a.row", "semanticId"),
		goqu.L("a.row -> 'semanticId' IS NOT NULL"),
		goqu.L("a.row -> 'semanticId' <> 'null'::jsonb"),
		goqu.L("(SELECT COUNT(*) FROM deleted_element_semantic_ids) >= 0"),
	)
	return dialect.Insert("submodel_element_semantic_id_reference").
		Cols("id", "type").
		FromQuery(source).
		OnConflict(goqu.DoUpdate("id", excludedRecord("type"))).
		Returning("id")
}

func updateElementSemanticPayloads(dialect goqu.DialectWrapper) *goqu.UpdateDataset {
	return dialect.Update(goqu.T("submodel_element_semantic_id_reference_payload").As("payload")).
		Set(goqu.Record{"parent_reference_payload": jsonNestedObject("a.row", "semanticId", "payload")}).
		From(goqu.T("affected_element_rows").As("a"), goqu.T("inserted_element_semantic_ids").As("inserted")).
		Where(
			goqu.I("payload.reference_id").Eq(goqu.I("a.id")),
			goqu.I("inserted.id").Eq(goqu.I("a.id")),
		).
		Returning(goqu.I("payload.reference_id"))
}

func insertElementSemanticPayloads(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(goqu.T("affected_element_rows").As("a")).
		Join(goqu.T("inserted_element_semantic_ids").As("inserted"), goqu.On(goqu.I("inserted.id").Eq(goqu.I("a.id")))).
		Select(goqu.I("a.id"), jsonNestedObject("a.row", "semanticId", "payload")).
		Where(
			goqu.L("(SELECT COUNT(*) FROM updated_element_semantic_payloads) >= 0"),
			goqu.Func("NOT EXISTS", dialect.From("submodel_element_semantic_id_reference_payload").
				Select(goqu.L("1")).
				Where(goqu.I("reference_id").Eq(goqu.I("a.id")))),
		)
	return dialect.Insert("submodel_element_semantic_id_reference_payload").Cols("reference_id", "parent_reference_payload").FromQuery(source).Returning("reference_id")
}

func deleteElementSemanticKeys(dialect goqu.DialectWrapper) *goqu.DeleteDataset {
	matchingPosition := dialect.From(
		goqu.T("affected_element_rows").As("a"),
		goqu.L("jsonb_array_elements(COALESCE(a.row -> 'semanticId' -> 'keys', '[]'::jsonb))").As("key"),
	).Select(goqu.L("1")).Where(
		goqu.I("a.id").Eq(goqu.I("submodel_element_semantic_id_reference_key.reference_id")),
		jsonInt("key", "position").Eq(goqu.I("submodel_element_semantic_id_reference_key.position")),
	)
	matchingReference := dialect.From(goqu.T("inserted_element_semantic_ids").As("inserted")).
		Select(goqu.L("1")).
		Where(
			goqu.I("submodel_element_semantic_id_reference_key.reference_id").Eq(goqu.I("inserted.id")),
			goqu.Func("NOT EXISTS", matchingPosition),
		)
	return dialect.Delete("submodel_element_semantic_id_reference_key").
		Where(goqu.Func("EXISTS", matchingReference)).
		Returning(goqu.I("submodel_element_semantic_id_reference_key.reference_id"))
}

func insertElementSemanticKeys(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(goqu.T("affected_element_rows").As("a")).
		Join(goqu.T("inserted_element_semantic_ids").As("inserted"), goqu.On(goqu.I("inserted.id").Eq(goqu.I("a.id")))).
		CrossJoin(goqu.L("jsonb_array_elements(COALESCE(a.row -> 'semanticId' -> 'keys', '[]'::jsonb))").As("key")).
		Select(goqu.I("a.id"), jsonInt("key", "position"), jsonInt("key", "type"), jsonText("key", "value"))
	return dialect.Insert("submodel_element_semantic_id_reference_key").
		Cols("reference_id", "position", "type", "value").
		FromQuery(source).
		OnConflict(goqu.DoUpdate("reference_id,position", excludedRecord("type", "value"))).
		Returning("reference_id")
}

func elementSupplementalRows(dialect goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.From(
		goqu.T("affected_element_rows").As("a"),
		goqu.L("jsonb_array_elements(COALESCE(a.row -> 'supplementalSemanticIds', '[]'::jsonb))").As("ref"),
	).Select(
		goqu.I("a.id").As("owner_id"),
		goqu.I("ref").As("row"),
	).Where(
		jsonChange("a.row", "supplementalId"),
		goqu.L("(SELECT COUNT(*) FROM deleted_element_supplemental) >= 0"),
	)
}

func deleteElementSupplementalReferences(dialect goqu.DialectWrapper) *goqu.DeleteDataset {
	matchingPosition := dialect.From(
		goqu.T("affected_element_rows").As("a"),
		goqu.L("jsonb_array_elements(COALESCE(a.row -> 'supplementalSemanticIds', '[]'::jsonb))").As("ref"),
	).Select(goqu.L("1")).Where(
		goqu.I("a.id").Eq(goqu.I("submodel_element_supplemental_semantic_id_reference.submodel_element_id")),
		jsonChange("a.row", "supplementalId"),
		jsonInt("ref", "position").Eq(goqu.I("submodel_element_supplemental_semantic_id_reference.position")),
	)
	matchingOwner := dialect.From(goqu.T("affected_element_rows").As("a")).
		Select(goqu.L("1")).
		Where(
			goqu.I("submodel_element_supplemental_semantic_id_reference.submodel_element_id").Eq(goqu.I("a.id")),
			jsonChange("a.row", "supplementalId"),
			goqu.Func("NOT EXISTS", matchingPosition),
		)
	return dialect.Delete("submodel_element_supplemental_semantic_id_reference").
		Where(goqu.Func("EXISTS", matchingOwner)).
		Returning(goqu.I("submodel_element_supplemental_semantic_id_reference.id"))
}

func insertElementSupplementalBase(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From("element_supplemental_rows").Select("owner_id", jsonInt("row", "position"), jsonInt("row", "type"))
	return dialect.Insert("submodel_element_supplemental_semantic_id_reference").
		Cols("submodel_element_id", "position", "type").
		FromQuery(source).
		OnConflict(goqu.DoUpdate("submodel_element_id,position", excludedRecord("type"))).
		Returning("id", "submodel_element_id", "position")
}

func updateElementSupplementalPayloads(dialect goqu.DialectWrapper) *goqu.UpdateDataset {
	return dialect.Update(goqu.T("submodel_element_supplemental_semantic_id_reference_payload").As("payload")).
		Set(goqu.Record{"parent_reference_payload": jsonObject("source.row", "payload")}).
		From(goqu.T("element_supplemental_rows").As("source"), goqu.T("inserted_element_supplemental").As("inserted")).
		Where(
			goqu.I("inserted.submodel_element_id").Eq(goqu.I("source.owner_id")),
			goqu.I("inserted.position").Eq(jsonInt("source.row", "position")),
			goqu.I("payload.reference_id").Eq(goqu.I("inserted.id")),
		).
		Returning(goqu.I("payload.reference_id"))
}

func insertElementSupplementalPayloads(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(goqu.T("element_supplemental_rows").As("source")).
		Join(goqu.T("inserted_element_supplemental").As("inserted"), goqu.On(
			goqu.I("inserted.submodel_element_id").Eq(goqu.I("source.owner_id")),
			goqu.I("inserted.position").Eq(jsonInt("source.row", "position")),
		)).Select(goqu.I("inserted.id"), jsonObject("source.row", "payload")).
		Where(
			goqu.L("(SELECT COUNT(*) FROM updated_element_supplemental_payloads) >= 0"),
			goqu.Func("NOT EXISTS", dialect.From("submodel_element_supplemental_semantic_id_reference_payload").
				Select(goqu.L("1")).
				Where(goqu.I("reference_id").Eq(goqu.I("inserted.id")))),
		)
	return dialect.Insert("submodel_element_supplemental_semantic_id_reference_payload").Cols("reference_id", "parent_reference_payload").FromQuery(source).Returning("reference_id")
}

func deleteElementSupplementalKeys(dialect goqu.DialectWrapper) *goqu.DeleteDataset {
	matchingPosition := dialect.From(
		goqu.T("element_supplemental_rows").As("source"),
		goqu.T("inserted_element_supplemental").As("inserted"),
		goqu.L("jsonb_array_elements(COALESCE(source.row -> 'keys', '[]'::jsonb))").As("key"),
	).Select(goqu.L("1")).Where(
		goqu.I("inserted.submodel_element_id").Eq(goqu.I("source.owner_id")),
		goqu.I("inserted.position").Eq(jsonInt("source.row", "position")),
		goqu.I("submodel_element_supplemental_semantic_id_reference_key.reference_id").Eq(goqu.I("inserted.id")),
		jsonInt("key", "position").Eq(goqu.I("submodel_element_supplemental_semantic_id_reference_key.position")),
	)
	matchingReference := dialect.From(goqu.T("inserted_element_supplemental").As("inserted")).
		Select(goqu.L("1")).
		Where(
			goqu.I("submodel_element_supplemental_semantic_id_reference_key.reference_id").Eq(goqu.I("inserted.id")),
			goqu.Func("NOT EXISTS", matchingPosition),
		)
	return dialect.Delete("submodel_element_supplemental_semantic_id_reference_key").
		Where(goqu.Func("EXISTS", matchingReference)).
		Returning(goqu.I("submodel_element_supplemental_semantic_id_reference_key.reference_id"))
}

func insertElementSupplementalKeys(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(goqu.T("element_supplemental_rows").As("source")).
		Join(goqu.T("inserted_element_supplemental").As("inserted"), goqu.On(
			goqu.I("inserted.submodel_element_id").Eq(goqu.I("source.owner_id")),
			goqu.I("inserted.position").Eq(jsonInt("source.row", "position")),
		)).CrossJoin(goqu.L("jsonb_array_elements(COALESCE(source.row -> 'keys', '[]'::jsonb))").As("key")).
		Select(goqu.I("inserted.id"), jsonInt("key", "position"), jsonInt("key", "type"), jsonText("key", "value"))
	return dialect.Insert("submodel_element_supplemental_semantic_id_reference_key").
		Cols("reference_id", "position", "type", "value").
		FromQuery(source).
		OnConflict(goqu.DoUpdate("reference_id,position", excludedRecord("type", "value"))).
		Returning("reference_id")
}

func (b *reconciliationQueryBuilder) addTypeSpecificCTEs() {
	typeSpecs := reconciliationTypeSpecs()
	for _, spec := range typeSpecs {
		b.add("upserted_"+spec.table, buildTypeUpsert(b.dialect, spec))
	}
	b.add("deleted_multilanguage_values", deleteAffectedRows(b.dialect, "multilanguage_property_value", "submodel_element_id", ""))
	b.add("inserted_multilanguage_values", insertMultilanguageValues(b.dialect))
	b.add("deleted_multilanguage_payloads", deleteEmptyValueIDRows(b.dialect, "multilanguage_property_payload", "submodel_element_id", types.ModelTypeMultiLanguageProperty))
	b.add("upserted_multilanguage_payloads", upsertValueIDPayload(b.dialect, "multilanguage_property_payload", "submodel_element_id"))
	b.add("deleted_property_payloads", deleteEmptyValueIDRows(b.dialect, "property_element_payload", "property_element_id", types.ModelTypeProperty))
	b.add("upserted_property_payloads", upsertValueIDPayload(b.dialect, "property_element_payload", "property_element_id"))
	b.add("inserted_managed_file_transfers", insertManagedFileTransfers(b.dialect))
}

type reconciliationTypeSpec struct {
	table   string
	columns []reconciliationTypeColumn
}

type reconciliationTypeColumn struct {
	name string
	expr func(string) exp.Expression
}

func reconciliationTypeSpecs() []reconciliationTypeSpec {
	text := func(key string) func(string) exp.Expression {
		return func(alias string) exp.Expression { return jsonNestedText(alias, "typeData", key) }
	}
	integer := func(key string) func(string) exp.Expression {
		return func(alias string) exp.Expression { return jsonNestedInt(alias, "typeData", key) }
	}
	boolean := func(key string) func(string) exp.Expression {
		return func(alias string) exp.Expression { return jsonNestedBool(alias, "typeData", key) }
	}
	jsonValue := func(key string) func(string) exp.Expression {
		return func(alias string) exp.Expression { return jsonNestedTextAsJSON(alias, "typeData", key) }
	}
	typed := func(key string, cast string) func(string) exp.Expression {
		return func(alias string) exp.Expression { return jsonNestedCast(alias, "typeData", key, cast) }
	}
	return []reconciliationTypeSpec{
		{table: "property_element", columns: []reconciliationTypeColumn{{"value_type", integer("value_type")}, {"value_text", text("value_text")}, {"value_num", typed("value_num", "numeric")}, {"value_bool", typed("value_bool", "boolean")}, {"value_time", typed("value_time", "time")}, {"value_date", typed("value_date", "date")}, {"value_datetime", typed("value_datetime", "timestamptz")}}},
		{table: "blob_element", columns: []reconciliationTypeColumn{{"content_type", text("content_type")}, {"value", func(alias string) exp.Expression {
			return goqu.L("convert_to(COALESCE(?, ''), 'UTF8')", jsonNestedText(alias, "typeData", "value"))
		}}}},
		{table: "file_element", columns: []reconciliationTypeColumn{{"content_type", text("content_type")}, {"file_name", fileNameExpression}, {"value", text("value")}}},
		{table: "range_element", columns: []reconciliationTypeColumn{{"value_type", integer("value_type")}, {"min_text", text("min_text")}, {"max_text", text("max_text")}, {"min_num", typed("min_num", "numeric")}, {"max_num", typed("max_num", "numeric")}, {"min_time", typed("min_time", "time")}, {"max_time", typed("max_time", "time")}, {"min_date", typed("min_date", "date")}, {"max_date", typed("max_date", "date")}, {"min_datetime", typed("min_datetime", "timestamptz")}, {"max_datetime", typed("max_datetime", "timestamptz")}}},
		{table: "reference_element", columns: []reconciliationTypeColumn{{"value", jsonValue("value")}}},
		{table: "relationship_element", columns: []reconciliationTypeColumn{{"first", jsonValue("first")}, {"second", jsonValue("second")}}},
		{table: "annotated_relationship_element", columns: []reconciliationTypeColumn{{"first", jsonValue("first")}, {"second", jsonValue("second")}}},
		{table: "submodel_element_collection", columns: nil},
		{table: "submodel_element_list", columns: []reconciliationTypeColumn{{"order_relevant", boolean("order_relevant")}, {"semantic_id_list_element", jsonValue("semantic_id_list_element")}, {"type_value_list_element", integer("type_value_list_element")}, {"value_type_list_element", integer("value_type_list_element")}}},
		{table: "entity_element", columns: []reconciliationTypeColumn{{"entity_type", integer("entity_type")}, {"global_asset_id", text("global_asset_id")}, {"specific_asset_ids", jsonValue("specific_asset_ids")}}},
		{table: "operation_element", columns: []reconciliationTypeColumn{{"input_variables", jsonValue("input_variables")}, {"output_variables", jsonValue("output_variables")}, {"inoutput_variables", jsonValue("inoutput_variables")}}},
		{table: "basic_event_element", columns: []reconciliationTypeColumn{{"observed", jsonValue("observed")}, {"direction", integer("direction")}, {"state", integer("state")}, {"message_topic", text("message_topic")}, {"message_broker", jsonValue("message_broker")}, {"last_update", typed("last_update", "timestamptz")}, {"min_interval", typed("min_interval", "interval")}, {"max_interval", typed("max_interval", "interval")}}},
	}
}

func buildTypeUpsert(dialect goqu.DialectWrapper, spec reconciliationTypeSpec) *goqu.InsertDataset {
	alias := "a.row"
	projections := []interface{}{goqu.I("a.id")}
	columns := []string{"id"}
	for _, column := range spec.columns {
		columns = append(columns, column.name)
		projections = append(projections, column.expr(alias))
	}
	source := dialect.From(goqu.T("affected_element_rows").As("a")).Select(projections...).
		Where(
			jsonChange("a.row", "typeData"),
			jsonText("a.row", "typeTable").Eq(common.PostgreSQLTextLiteral(spec.table)),
		)
	insert := dialect.Insert(spec.table).Cols(stringInterfaces(columns)...).FromQuery(source)
	if len(spec.columns) > 0 {
		updateColumns := make([]string, 0, len(spec.columns))
		for _, column := range spec.columns {
			updateColumns = append(updateColumns, column.name)
		}
		insert = insert.OnConflict(goqu.DoUpdate("id", excludedRecord(updateColumns...)))
	} else {
		insert = insert.OnConflict(goqu.DoNothing())
	}
	return insert.Returning("id")
}

func deleteAffectedRows(dialect goqu.DialectWrapper, table string, ownerColumn string, typeTable string) *goqu.DeleteDataset {
	conditions := []exp.Expression{goqu.I(table + "." + ownerColumn).Eq(goqu.I("a.id"))}
	if typeTable != "" {
		conditions = append(conditions, jsonText("a.row", "typeTable").Eq(common.PostgreSQLTextLiteral(typeTable)))
	} else {
		conditions = append(conditions,
			jsonChange("a.row", "languageValues"),
			jsonInt("a.row", "modelType").Eq(integerLiteral(int64(types.ModelTypeMultiLanguageProperty))),
		)
	}
	matching := dialect.From(goqu.T("affected_element_rows").As("a")).
		Select(goqu.L("1")).
		Where(conditions...)
	return dialect.Delete(table).
		Where(goqu.Func("EXISTS", matching)).
		Returning(goqu.I(table + "." + ownerColumn))
}

func deleteEmptyValueIDRows(
	dialect goqu.DialectWrapper,
	table string,
	ownerColumn string,
	modelType types.ModelType,
) *goqu.DeleteDataset {
	matching := dialect.From(goqu.T("affected_element_rows").As("a")).
		Select(goqu.L("1")).
		Where(
			jsonChange("a.row", "valueId"),
			goqu.I(table+"."+ownerColumn).Eq(goqu.I("a.id")),
			jsonInt("a.row", "modelType").Eq(integerLiteral(int64(modelType))),
			goqu.Or(
				goqu.L("a.row -> 'valueId' IS NULL"),
				goqu.L("a.row -> 'valueId' = 'null'::jsonb"),
			),
		)
	return dialect.Delete(table).
		Where(goqu.Func("EXISTS", matching)).
		Returning(goqu.I(table + "." + ownerColumn))
}

func insertMultilanguageValues(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(
		goqu.T("affected_element_rows").As("a"),
		goqu.L("jsonb_array_elements(COALESCE(a.row -> 'languageValues', '[]'::jsonb))").As("value"),
	).Select(goqu.I("a.id"), jsonText("value", "language"), jsonText("value", "text")).
		Where(
			jsonChange("a.row", "languageValues"),
			jsonInt("a.row", "modelType").Eq(integerLiteral(int64(types.ModelTypeMultiLanguageProperty))),
			goqu.L("(SELECT COUNT(*) FROM deleted_multilanguage_values) >= 0"),
		)
	return dialect.Insert("multilanguage_property_value").Cols("submodel_element_id", "language", "text").FromQuery(source).Returning("submodel_element_id")
}

func upsertValueIDPayload(dialect goqu.DialectWrapper, table string, ownerColumn string) *goqu.InsertDataset {
	modelType := int64(types.ModelTypeMultiLanguageProperty)
	if table == "property_element_payload" {
		modelType = int64(types.ModelTypeProperty)
	}
	source := dialect.From(goqu.T("affected_element_rows").As("a")).Select(goqu.I("a.id"), jsonObject("a.row", "valueId")).
		Where(
			jsonChange("a.row", "valueId"),
			jsonInt("a.row", "modelType").Eq(integerLiteral(modelType)),
			goqu.L("a.row -> 'valueId' IS NOT NULL"),
			goqu.L("a.row -> 'valueId' <> 'null'::jsonb"),
		)
	return dialect.Insert(table).Cols(ownerColumn, "value_id_payload").FromQuery(source).
		OnConflict(goqu.DoUpdate(ownerColumn, goqu.Record{"value_id_payload": goqu.I("excluded.value_id_payload")})).
		Returning(ownerColumn)
}

func insertManagedFileTransfers(dialect goqu.DialectWrapper) *goqu.InsertDataset {
	source := dialect.From(goqu.T("managed_file_transfers").As("transfer")).
		Join(goqu.T("upserted_file_element").As("file"), goqu.On(goqu.I("file.id").Eq(goqu.I("transfer.id")))).
		Select("transfer.id", "transfer.binary_content_id", "transfer.path_token", "transfer.safe_file_name")
	return dialect.Insert("file_binary_reference").Cols("file_element_id", "binary_content_id", "path_token", "safe_file_name").FromQuery(source).Returning("file_element_id")
}

func fileNameExpression(alias string) exp.Expression {
	return goqu.L("COALESCE((SELECT safe_file_name FROM managed_file_transfers WHERE id = ?), (SELECT CASE WHEN existing.value IS NOT DISTINCT FROM ? THEN existing.file_name END FROM file_element AS existing WHERE existing.id = ?))", goqu.I("a.id"), jsonNestedText(alias, "typeData", "value"), goqu.I("a.id"))
}

func jsonText(alias string, key string) exp.LiteralExpression {
	return goqu.L("? ->> ?", goqu.I(alias), common.PostgreSQLTextLiteral(key))
}

func jsonObject(alias string, key string) exp.LiteralExpression {
	return goqu.L("? -> ?", goqu.I(alias), common.PostgreSQLTextLiteral(key))
}

func jsonInt(alias string, key string) exp.LiteralExpression {
	return goqu.L("(? ->> ?)::integer", goqu.I(alias), common.PostgreSQLTextLiteral(key))
}

func jsonNestedText(alias string, parent string, key string) exp.LiteralExpression {
	return goqu.L("? -> ? ->> ?", goqu.I(alias), common.PostgreSQLTextLiteral(parent), common.PostgreSQLTextLiteral(key))
}

func jsonNestedObject(alias string, parent string, key string) exp.LiteralExpression {
	return goqu.L("? -> ? -> ?", goqu.I(alias), common.PostgreSQLTextLiteral(parent), common.PostgreSQLTextLiteral(key))
}

func jsonNestedInt(alias string, parent string, key string) exp.LiteralExpression {
	return goqu.L("(? -> ? ->> ?)::integer", goqu.I(alias), common.PostgreSQLTextLiteral(parent), common.PostgreSQLTextLiteral(key))
}

func jsonNestedBool(alias string, parent string, key string) exp.LiteralExpression {
	return goqu.L("(? -> ? ->> ?)::boolean", goqu.I(alias), common.PostgreSQLTextLiteral(parent), common.PostgreSQLTextLiteral(key))
}

func jsonNestedTextAsJSON(alias string, parent string, key string) exp.LiteralExpression {
	return goqu.L("(? -> ? ->> ?)::jsonb", goqu.I(alias), common.PostgreSQLTextLiteral(parent), common.PostgreSQLTextLiteral(key))
}

func jsonNestedCast(alias string, parent string, key string, cast string) exp.LiteralExpression {
	return goqu.L(fmt.Sprintf("NULLIF(? -> ? ->> ?, '')::%s", cast), goqu.I(alias), common.PostgreSQLTextLiteral(parent), common.PostgreSQLTextLiteral(key))
}

func jsonChange(alias string, key string) exp.LiteralExpression {
	return goqu.L("COALESCE((? -> 'changes' ->> ?)::boolean, false)", goqu.I(alias), common.PostgreSQLTextLiteral(key))
}

func nullableJSONField(parent exp.Expression, key string) exp.LiteralExpression {
	return goqu.L("NULLIF(? -> ?, 'null'::jsonb)", parent, common.PostgreSQLTextLiteral(key))
}

func integerLiteral(value int64) exp.LiteralExpression {
	return goqu.L(strconv.FormatInt(value, 10))
}

func excludedRecord(columns ...string) goqu.Record {
	record := make(goqu.Record, len(columns))
	for _, column := range columns {
		record[column] = goqu.I("excluded." + column)
	}
	return record
}

func stringInterfaces(values []string) []interface{} {
	result := make([]interface{}, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func deferSubmodelElementReconciliationConstraints(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return common.NewInternalServerError("SMREPO-RECON-DEFER-NILTX transaction must not be nil")
	}
	statement := goqu.L("SET CONSTRAINTS uq_sibling_idshort, uq_sibling_pos DEFERRED")
	if _, err := tx.ExecContext(ctx, statement.Literal(), statement.Args()...); err != nil {
		return common.NewInternalServerError("SMREPO-RECON-DEFER-EXEC " + err.Error())
	}
	return nil
}

func executeSubmodelReconciliationStatement(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
	staged *stagedSubmodelTarget,
) (reconciliationMutationResult, error) {
	if staged == nil || staged.stage == nil {
		return reconciliationMutationResult{}, common.NewInternalServerError("SMREPO-RECON-NILSTAGE staged target must not be nil")
	}
	query, args, err := newReconciliationQueryBuilder().build(staged.stage, submodelID)
	if err != nil {
		return reconciliationMutationResult{}, common.NewInternalServerError("SMREPO-RECON-BUILD " + err.Error())
	}
	var result reconciliationMutationResult
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&result.UpdatedElements, &result.InsertedElements, &result.DeletedElements); err != nil {
		return reconciliationMutationResult{}, common.NewInternalServerError("SMREPO-RECON-EXEC " + err.Error())
	}
	return result, nil
}
