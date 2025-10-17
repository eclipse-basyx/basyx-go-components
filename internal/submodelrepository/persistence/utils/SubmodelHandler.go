/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

package persistence_utils

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

func GetSubmodelJSON(db *sql.DB, submodelId string) ([]*gen.Submodel, error) {
	var result []*gen.Submodel
	referenceBuilderRefs := make(map[int64]*builders.ReferenceBuilder)
	start := time.Now().Local().UnixMilli()
	rows, err := getSubmodelDataFromDbWithJSONQuery(db, submodelId)
	end := time.Now().Local().UnixMilli()
	fmt.Printf("Total Qury Only time: %d milliseconds\n", end-start)
	if err != nil {
		return nil, fmt.Errorf("error getting submodel data from DB: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var row builders.SubmodelRow
		if err := rows.Scan(
			&row.Id, &row.IdShort, &row.Category, &row.Kind,
			&row.DisplayNames, &row.Descriptions,
			&row.SemanticId, &row.ReferredSemanticIds,
			&row.SupplementalSemanticIds, &row.SupplementalReferredSemIds,
			&row.EmbeddedDataSpecifications, &row.DataSpecReferenceReferred,
			&row.DataSpecIEC61360, &row.IECLevelTypes, &row.TotalSubmodels,
		); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}

		if result == nil {
			result = make([]*gen.Submodel, 0, row.TotalSubmodels)
		}

		submodel := &gen.Submodel{
			ModelType: "Submodel",
			Id:        row.Id,
			IdShort:   row.IdShort,
			Category:  row.Category,
			Kind:      gen.ModellingKind(row.Kind),
		}

		var semanticId []*gen.Reference
		if isArrayNotEmpty(row.EmbeddedDataSpecifications) {
			semanticId, err = builders.ParseReferences(row.SemanticId, referenceBuilderRefs)
			if err != nil {
				return nil, err
			}
			if hasSemanticId(semanticId) {
				submodel.SemanticId = semanticId[0]
				builders.ParseReferredReferences(row.ReferredSemanticIds, referenceBuilderRefs)
			}
		}

		if isArrayNotEmpty(row.SupplementalSemanticIds) {
			supplementalSemanticIds, err := builders.ParseReferences(row.SupplementalSemanticIds, referenceBuilderRefs)
			if err != nil {
				return nil, err
			}
			if hasSupplementalSemanticIds(supplementalSemanticIds) {
				submodel.SupplementalSemanticIds = supplementalSemanticIds
				builders.ParseReferredReferences(row.SupplementalReferredSemIds, referenceBuilderRefs)
			}
		}

		// DisplayNames
		err = addDisplayNames(row, submodel)
		if err != nil {
			return nil, err
		}

		// Descriptions
		err = addDescriptions(row, submodel)
		if err != nil {
			return nil, err
		}

		result = append(result, submodel)
	}

	for _, referenceBuilder := range referenceBuilderRefs {
		referenceBuilder.BuildNestedStructure()
	}
	return result, nil
}

func addDisplayNames(row builders.SubmodelRow, submodel *gen.Submodel) error {
	if isArrayNotEmpty(row.DisplayNames) {
		displayNames, err := builders.ParseLangStringNameType(row.DisplayNames)
		if err != nil {
			return fmt.Errorf("error parsing display names: %w", err)
		}
		submodel.DisplayName = displayNames
	}
	return nil
}

func addDescriptions(row builders.SubmodelRow, submodel *gen.Submodel) error {
	if isArrayNotEmpty(row.Descriptions) {
		descriptions, err := builders.ParseLangStringTextType(row.Descriptions)
		if err != nil {
			return fmt.Errorf("error parsing descriptions: %w", err)
		}
		submodel.Description = descriptions
	}
	return nil
}

func isArrayNotEmpty(data json.RawMessage) bool {
	return len(data) > 0 && string(data) != "null"
}

func hasSemanticId(semanticIdData []*gen.Reference) bool {
	return len(semanticIdData) == 1
}

func hasSupplementalSemanticIds(supplementalSemanticIdsData []*gen.Reference) bool {
	return len(supplementalSemanticIdsData) > 0
}

func getSubmodelDataFromDbWithJSONQuery(db *sql.DB, submodelId string) (*sql.Rows, error) {
	q, err := getQueryWithGoqu(submodelId)
	if err != nil {
		return nil, fmt.Errorf("error building query: %w", err)
	}
	//fmt.Print(q)
	rows, err := db.Query(q)
	if err != nil {
		fmt.Printf("Error querying database: %v\n", err)
		return nil, err
	}
	return rows, nil
}

func getQueryWithGoqu(submodelId string) (string, error) {
	dialect := goqu.Dialect("postgres")

	// Build display names subquery
	displayNamesSubquery := dialect.From(goqu.T("lang_string_name_type_reference").As("dn_ref")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('language', dn.language, 'text', dn.text, 'id', dn.id))")).
		Join(
			goqu.T("lang_string_name_type").As("dn"),
			goqu.On(goqu.I("dn.lang_string_name_type_reference_id").Eq(goqu.I("dn_ref.id"))),
		).
		Where(goqu.I("dn_ref.id").Eq(goqu.I("s.displayname_id")))

	// Build descriptions subquery
	descriptionsSubquery := dialect.From(goqu.T("lang_string_text_type_reference").As("dr")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('language', d.language, 'text', d.text, 'id', d.id))")).
		Join(
			goqu.T("lang_string_text_type").As("d"),
			goqu.On(goqu.I("d.lang_string_text_type_reference_id").Eq(goqu.I("dr.id"))),
		).
		Where(goqu.I("dr.id").Eq(goqu.I("s.description_id")))

	// Build semantic_id subquery
	semanticIdSubquery := dialect.From(goqu.T("reference").As("r")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('reference_id', r.id, 'reference_type', r.type, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value))")).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("r.id"))),
		).
		Where(goqu.I("r.id").Eq(goqu.I("s.semantic_id")))

	// Build semantic_id referred references subquery
	semanticIdReferredSubquery := dialect.From(goqu.T("reference").As("ref")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('reference_id', ref.id, 'reference_type', ref.type, 'parentReference', ref.parentreference, 'rootReference', ref.rootreference, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value))")).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("ref.id"))),
		).
		Where(
			goqu.I("ref.rootreference").Eq(goqu.I("s.semantic_id")),
			goqu.I("ref.id").Neq(goqu.I("s.semantic_id")),
		)

	// Build supplemental semantic ids subquery
	supplementalSemanticIdsSubquery := dialect.From(goqu.T("submodel_supplemental_semantic_id").As("sssi")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('reference_id', ref.id, 'reference_type', ref.type, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value))")).
		LeftJoin(
			goqu.T("reference").As("ref"),
			goqu.On(goqu.I("ref.id").Eq(goqu.I("sssi.reference_id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("ref.id"))),
		).
		Where(goqu.I("sssi.submodel_id").Eq(goqu.I("s.id")))

	// Build supplemental semantic ids referred subquery
	supplementalSemanticIdsReferredSubquery := dialect.From(goqu.T("submodel_supplemental_semantic_id").As("sssi")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('supplemental_root_reference_id', sssi.reference_id, 'reference_id', ref.id, 'reference_type', ref.type, 'parentReference', ref.parentreference, 'rootReference', ref.rootreference, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value))")).
		LeftJoin(
			goqu.T("reference").As("ref"),
			goqu.On(goqu.I("ref.rootreference").Eq(goqu.I("sssi.reference_id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("ref.id"))),
		).
		Where(
			goqu.I("sssi.submodel_id").Eq(goqu.I("s.id")),
			goqu.I("ref.id").IsNotNull(),
		)

	// Build embedded data specifications subquery
	embeddedDataSpecsSubquery := dialect.From(goqu.T("submodel_embedded_data_specification").As("seds")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('submodel_id', seds.submodel_id, 'data_specification_id', data_spec.id, 'data_spec_reference_id', data_spec_reference.id, 'data_spec_reference_type', data_spec_reference.type, 'data_spec_reference_key_id', data_spec_reference_key.id, 'data_spec_reference_key_type', data_spec_reference_key.type, 'data_spec_reference_key_value', data_spec_reference_key.value))")).
		LeftJoin(
			goqu.T("data_specification").As("data_spec"),
			goqu.On(goqu.I("data_spec.id").Eq(goqu.I("seds.embedded_data_specification_id"))),
		).
		LeftJoin(
			goqu.T("reference").As("data_spec_reference"),
			goqu.On(goqu.I("data_spec.data_specification").Eq(goqu.I("data_spec_reference.id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("data_spec_reference_key"),
			goqu.On(goqu.I("data_spec.id").Eq(goqu.I("data_spec_reference_key.reference_id"))),
		).
		Where(goqu.I("seds.submodel_id").Eq(goqu.I("s.id")))

	// Build data spec reference referred subquery
	dataSpecReferenceReferredSubquery := dialect.From(goqu.T("submodel_embedded_data_specification").As("seds")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('data_spec_reference_id', dsr.rootreference, 'reference_id', ref.id, 'reference_type', ref.type, 'parentReference', ref.parentreference, 'rootReference', ref.rootreference, 'key_id', rk.id, 'key_type', rk.type, 'key_value', rk.value))")).
		LeftJoin(
			goqu.T("data_specification").As("data_spec"),
			goqu.On(goqu.I("data_spec.id").Eq(goqu.I("seds.embedded_data_specification_id"))),
		).
		LeftJoin(
			goqu.T("reference").As("data_spec_reference"),
			goqu.On(goqu.I("data_spec.data_specification").Eq(goqu.I("data_spec_reference.id"))),
		).
		LeftJoin(
			goqu.T("reference").As("ref"),
			goqu.On(goqu.I("ref.rootreference").Eq(goqu.I("data_spec_reference.id"))),
		).
		LeftJoin(
			goqu.T("reference_key").As("rk"),
			goqu.On(goqu.I("rk.reference_id").Eq(goqu.I("ref.id"))),
		).
		LeftJoin(
			goqu.T("reference").As("dsr"),
			goqu.On(goqu.I("dsr.id").Eq(goqu.I("data_spec_reference.id"))),
		).
		Where(
			goqu.I("seds.submodel_id").Eq(goqu.I("s.id")),
			goqu.I("ref.id").IsNotNull(),
		)

	// Build IEC61360 subquery with nested subqueries for lang strings
	preferredNameSubquery := goqu.L(`(SELECT jsonb_agg(DISTINCT jsonb_build_object('language', pn.language, 'text', pn.text, 'id', pn.id))
		FROM lang_string_text_type_reference pn_ref
		JOIN lang_string_text_type pn ON pn.lang_string_text_type_reference_id = pn_ref.id
		WHERE pn_ref.id = iec.preferred_name_id)`)

	shortNameSubquery := goqu.L(`(SELECT jsonb_agg(DISTINCT jsonb_build_object('language', sn.language, 'text', sn.text, 'id', sn.id))
		FROM lang_string_text_type_reference sn_ref
		JOIN lang_string_text_type sn ON sn.lang_string_text_type_reference_id = sn_ref.id
		WHERE sn_ref.id = iec.short_name_id)`)

	definitionSubquery := goqu.L(`(SELECT jsonb_agg(DISTINCT jsonb_build_object('language', df.language, 'text', df.text, 'id', df.id))
		FROM lang_string_text_type_reference df_ref
		JOIN lang_string_text_type df ON df.lang_string_text_type_reference_id = df_ref.id
		WHERE df_ref.id = iec.definition_id)`)

	unitReferenceKeysSubquery := goqu.L(`(SELECT jsonb_agg(DISTINCT jsonb_build_object('reference_id', dsu.id, 'key_id', dsk.id, 'key_type', dsk.type, 'key_value', dsk.value))
		FROM reference dsu
		LEFT JOIN reference_key dsk ON dsk.reference_id = dsu.id
		WHERE dsu.id = iec.unit_id)`)

	unitReferenceReferredSubquery := goqu.L(`(SELECT jsonb_agg(DISTINCT jsonb_build_object('rootReference', dsu2.rootreference, 'reference_id', dsu2.id, 'key_id', dsk2.id, 'key_type', dsk2.type, 'key_value', dsk2.value))
		FROM reference dsu2
		LEFT JOIN reference_key dsk2 ON dsk2.reference_id = dsu2.id
		WHERE dsu2.rootreference = iec.unit_id AND dsu2.id != iec.unit_id)`)

	valueListEntriesSubquery := goqu.L(`(SELECT jsonb_agg(DISTINCT jsonb_build_object('value_reference_pair_id', vlvrp.id, 'reference_id', vlref.id))
		FROM value_list vl
		JOIN value_list_value_reference_pair vlvrp ON vl.id = vlvrp.value_list_id
		LEFT JOIN reference vlref ON vlvrp.value_id = vlref.id
		WHERE vl.id = iec.value_list_id)`)

	iec61360Subquery := dialect.From(goqu.T("submodel_embedded_data_specification").As("seds")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('iec_id', iec.id, 'unit', iec.unit, 'source_of_definition', iec.source_of_definition, 'symbol', iec.symbol, 'data_type', iec.data_type, 'value_format', iec.value_format, 'value', iec.value, 'level_type_id', iec.level_type_id, 'preferred_name', ?, 'short_name', ?, 'definition', ?, 'unit_reference_keys', ?, 'unit_reference_referred', ?, 'value_list_entries', ?))",
			preferredNameSubquery, shortNameSubquery, definitionSubquery, unitReferenceKeysSubquery, unitReferenceReferredSubquery, valueListEntriesSubquery)).
		Join(
			goqu.T("data_specification").As("ds"),
			goqu.On(goqu.I("ds.id").Eq(goqu.I("seds.embedded_data_specification_id"))),
		).
		Join(
			goqu.T("data_specification_content").As("dsc"),
			goqu.On(goqu.I("dsc.id").Eq(goqu.I("ds.data_specification_content"))),
		).
		Join(
			goqu.T("data_specification_iec61360").As("iec"),
			goqu.On(goqu.I("iec.id").Eq(goqu.I("dsc.id"))),
		).
		Where(goqu.I("seds.submodel_id").Eq(goqu.I("s.id")))

	// Build level types subquery
	levelTypesSubquery := dialect.From(goqu.T("submodel_embedded_data_specification").As("seds")).
		Select(goqu.L("jsonb_agg(DISTINCT jsonb_build_object('iec_id', iec.id, 'level_type_id', lvl.id))")).
		Join(
			goqu.T("data_specification").As("ds"),
			goqu.On(goqu.I("ds.id").Eq(goqu.I("seds.embedded_data_specification_id"))),
		).
		Join(
			goqu.T("data_specification_content").As("dsc"),
			goqu.On(goqu.I("dsc.id").Eq(goqu.I("ds.data_specification_content"))),
		).
		Join(
			goqu.T("data_specification_iec61360").As("iec"),
			goqu.On(goqu.I("iec.id").Eq(goqu.I("dsc.id"))),
		).
		LeftJoin(
			goqu.T("level_type").As("lvl"),
			goqu.On(goqu.I("lvl.id").Eq(goqu.I("iec.level_type_id"))),
		).
		Where(goqu.I("seds.submodel_id").Eq(goqu.I("s.id")))

	// Main query
	query := dialect.From(goqu.T("submodel").As("s")).
		Select(
			goqu.I("s.id").As("submodel_id"),
			goqu.I("s.id_short").As("submodel_id_short"),
			goqu.I("s.category").As("submodel_category"),
			goqu.I("s.kind").As("submodel_kind"),
			goqu.L("COALESCE((?), '[]'::jsonb)", displayNamesSubquery).As("submodel_display_names"),
			goqu.L("COALESCE((?), '[]'::jsonb)", descriptionsSubquery).As("submodel_descriptions"),
			goqu.L("COALESCE((?), '[]'::jsonb)", semanticIdSubquery).As("submodel_semantic_id"),
			goqu.L("COALESCE((?), '[]'::jsonb)", semanticIdReferredSubquery).As("submodel_semantic_id_referred"),
			goqu.L("COALESCE((?), '[]'::jsonb)", supplementalSemanticIdsSubquery).As("submodel_supplemental_semantic_ids"),
			goqu.L("COALESCE((?), '[]'::jsonb)", supplementalSemanticIdsReferredSubquery).As("submodel_supplemental_semantic_id_referred"),
			goqu.L("COALESCE((?), '[]'::jsonb)", embeddedDataSpecsSubquery).As("submodel_embedded_data_specifications"),
			goqu.L("COALESCE((?), '[]'::jsonb)", dataSpecReferenceReferredSubquery).As("submodel_data_spec_reference_referred"),
			goqu.L("COALESCE((?), '[]'::jsonb)", iec61360Subquery).As("submodel_data_spec_iec61360"),
			goqu.L("COALESCE((?), '[]'::jsonb)", levelTypesSubquery).As("submodel_iec_level_types"),
		)

	// Add optional WHERE clause for submodel ID filtering
	if submodelId != "" {
		query = query.Where(goqu.I("s.id").Eq(submodelId))
	}

	// add a field that counts number of submodels to presize slices in calling function
	query = query.SelectAppend(goqu.L("COUNT(s.id) OVER() AS total_submodels"))

	sql, _, err := query.ToSQL()
	if err != nil {
		return "", fmt.Errorf("error generating SQL: %w", err)
	}

	return sql, nil
}
