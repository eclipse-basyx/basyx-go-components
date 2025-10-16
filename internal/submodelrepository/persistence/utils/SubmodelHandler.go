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

	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	qb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/querybuilder"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

func GetSubmodels(db *sql.DB) {
	GetSubmodel(db, "")
}

func GetSubmodel(db *sql.DB, submodelId string) {
	rows, err := getSubmodelDataFromDb(db, submodelId)
	if err != nil {
		fmt.Printf("Error getting submodel data from DB: %v\n", err)
		return
	}
	defer rows.Close()

	submodels := make(map[string]*gen.Submodel)
	semanticIdBuilderRefs := make(map[string]*builders.ReferenceBuilder)
	//semanticIdReferredSemanticIdBuilderRefs := make(map[int64]*builders.ReferenceBuilder)
	supplementalSemanticIdBuilderRefs := make(map[string]*builders.MultiReferenceBuilder)
	nameTypeBuilderRefs := make(map[string]*builders.LangStringNameTypesBuilder)
	textTypeBuilderRefs := make(map[string]*builders.LangStringTextTypesBuilder)

	// Store pointers to the slices that will be populated
	displayNameRefs := make(map[string]*[]gen.LangStringNameType)
	descriptionRefs := make(map[string]*[]gen.LangStringTextType)

	for rows.Next() {
		// Submodel Header
		var id, idShort, category, kind sql.NullString
		// DisplayName
		var display_name_language, display_name_text sql.NullString
		// Description
		var description_language, description_text sql.NullString
		// SemanticId
		var semantic_id_type, key_type, key_value sql.NullString
		var semantic_id_db_id sql.NullInt64 // DB ID of the root semantic reference
		// SemanticId -> referredSemanticIds
		var referred_semantic_id_type, referred_key_type, referred_key_value sql.NullString
		var referred_parent_reference_id sql.NullInt64
		// SupplementalSemanticIds
		var supplementalSemanticIdType, supplemental_semantic_id_key_type, supplemental_semantic_id_key_value sql.NullString
		// SupplementalSemanticIds -> referredSemanticIds
		var supplemental_semantic_id_referred_semantic_id_type, supplemental_semantic_id_referred_key_type, supplemental_semantic_id_referred_key_value sql.NullString
		var supplemental_semantic_id_referred_parent_reference_id sql.NullInt64
		// Identifiers
		var key_id, display_name_id, description_id, supplemental_semantic_id_key_id, supplemental_semantic_id_dbid, referred_key_id, referred_semantic_id_db_id, supplemental_semantic_id_referred_semantic_id_db_id, supplemental_semantic_id_referred_key_id sql.NullInt64

		err := rows.Scan(
			// Submodel
			&id, &idShort, &category, &kind,
			// DisplayName
			&display_name_language, &display_name_text, &display_name_id,
			// Description
			&description_language, &description_text, &description_id,
			// SemanticId
			&semantic_id_db_id, &semantic_id_type, &key_type, &key_value, &key_id,
			// SemanticId -> referredSemanticIds
			&referred_semantic_id_db_id, &referred_semantic_id_type, &referred_parent_reference_id, &referred_key_type, &referred_key_value, &referred_key_id,
			// SupplementalSemanticIds
			&supplementalSemanticIdType, &supplemental_semantic_id_dbid,
			&supplemental_semantic_id_key_type, &supplemental_semantic_id_key_value, &supplemental_semantic_id_key_id,
			// SupplementalSemanticIds -> referredSemanticIds
			&supplemental_semantic_id_referred_semantic_id_db_id, &supplemental_semantic_id_referred_semantic_id_type, &supplemental_semantic_id_referred_parent_reference_id, &supplemental_semantic_id_referred_key_type, &supplemental_semantic_id_referred_key_value, &supplemental_semantic_id_referred_key_id,
		)

		if err != nil {
			fmt.Printf("Error scanning row: %v\n", err)
			continue
		}

		if !isSubmodelAlreadyCreated(submodels, id) {
			semanticId, semanticIdBuilder := builders.NewReferenceBuilder(semantic_id_type.String, semantic_id_db_id.Int64)
			_, supplementalSemanticIdBuilders := builders.NewMultiReferenceBuilder()
			displayName, nameTypeBuilder := builders.NewLangStringNameTypesBuilder()
			description, textTypeBuilder := builders.NewLangStringTextTypesBuilder()

			semanticIdBuilderRefs[id.String] = semanticIdBuilder
			supplementalSemanticIdBuilderRefs[id.String] = supplementalSemanticIdBuilders
			nameTypeBuilderRefs[id.String] = nameTypeBuilder
			textTypeBuilderRefs[id.String] = textTypeBuilder

			// Store references to the slices
			displayNameRefs[id.String] = displayName
			descriptionRefs[id.String] = description
			// Note: supplementalSemanticIds will be retrieved from builder after loop

			submodels[id.String] = &gen.Submodel{
				IdShort:    idShort.String,
				Id:         id.String,
				Category:   category.String,
				Kind:       gen.ModellingKind(kind.String),
				ModelType:  "Submodel",
				SemanticId: semanticId,
				// DisplayName, Description, and SupplementalSemanticIds will be set after the loop
			}
			getEmbeddedDataSpecificationsForSubmodel(db, "")
		}

		if key_id.Valid {
			semanticIdBuilderRefs[id.String].CreateKey(key_id.Int64, key_type.String, key_value.String)
		}
		if referred_semantic_id_db_id.Valid {
			semanticIdBuilderRefs[id.String].CreateReferredSemanticId(referred_semantic_id_db_id.Int64, referred_parent_reference_id.Int64, referred_semantic_id_type.String)
		}
		if referred_key_id.Valid {
			semanticIdBuilderRefs[id.String].CreateReferredSemanticIdKey(referred_semantic_id_db_id.Int64, referred_key_id.Int64, referred_key_type.String, referred_key_value.String)
		}
		if display_name_id.Valid {
			nameTypeBuilderRefs[id.String].CreateLangStringNameType(display_name_id.Int64, display_name_language.String, display_name_text.String)
		}
		if description_id.Valid {
			textTypeBuilderRefs[id.String].CreateLangStringTextType(description_id.Int64, description_language.String, description_text.String)
		}

		if supplemental_semantic_id_dbid.Valid {
			supplementalSemanticIdBuilderRefs[id.String].CreateReference(supplemental_semantic_id_dbid.Int64, supplementalSemanticIdType.String)
		}
		if supplemental_semantic_id_key_id.Valid {
			supplementalSemanticIdBuilderRefs[id.String].CreateKey(supplemental_semantic_id_dbid.Int64, supplemental_semantic_id_key_id.Int64, supplemental_semantic_id_key_type.String, supplemental_semantic_id_key_value.String)
		}
		if supplemental_semantic_id_referred_semantic_id_db_id.Valid {
			supplementalSemanticIdBuilderRefs[id.String].CreateReferredSemanticId(supplemental_semantic_id_dbid.Int64, supplemental_semantic_id_referred_semantic_id_db_id.Int64, supplemental_semantic_id_referred_parent_reference_id.Int64, supplemental_semantic_id_referred_semantic_id_type.String)
		}
		if supplemental_semantic_id_referred_key_id.Valid {
			supplementalSemanticIdBuilderRefs[id.String].CreateReferredSemanticIdKey(supplemental_semantic_id_dbid.Int64, supplemental_semantic_id_referred_semantic_id_db_id.Int64, supplemental_semantic_id_referred_key_id.Int64, supplemental_semantic_id_referred_key_type.String, supplemental_semantic_id_referred_key_value.String)
		}
	}

	for _, referenceBuilder := range semanticIdBuilderRefs {
		referenceBuilder.BuildNestedStructure()
	}

	for _, multiReferenceBuilder := range supplementalSemanticIdBuilderRefs {
		multiReferenceBuilder.BuildNestedStructures()
	}

	// After all rows are processed, assign the populated slices to the submodels
	for submodelId, submodel := range submodels {
		if displayName, ok := displayNameRefs[submodelId]; ok {
			submodel.DisplayName = *displayName
		}
		if description, ok := descriptionRefs[submodelId]; ok {
			submodel.Description = *description
		}
		if supplementalSemanticIdBuilder, ok := supplementalSemanticIdBuilderRefs[submodelId]; ok {
			submodel.SupplementalSemanticIds = supplementalSemanticIdBuilder.GetReferences()
		}
	}

	for submodelId, submodel := range submodels {
		// json stringify submodel
		jsonData, err := json.Marshal(submodel)
		if err != nil {
			fmt.Printf("Error marshaling submodel to JSON: %v\n", err)
			continue
		}
		fmt.Printf("Submodel %s: %+v\n", submodelId, string(jsonData))
	}
}

func isSubmodelAlreadyCreated(submodels map[string]*gen.Submodel, id sql.NullString) bool {
	_, ok := submodels[id.String]
	return ok
}

func getSubmodelDataFromDb(db *sql.DB, id string) (*sql.Rows, error) {
	// Submodel Header
	submodelSelects := getSubmodelSelects()
	// DisplayName
	displayNameSelects := getDisplayNameSelects()
	// Description
	descriptionSelects := getDescriptionSelects()
	// SemanticId
	semanticIdSelects, semanticIdKeySelects, semanticIdReferredSemanticIdSelects, semanticIdReferredSemanticIdKeySelects := getSemanticIdSelects()
	// SupplementalSemanticIds
	supplementalSemanticIdSelects, supplementalSemanticIdKeySelects, supplementalSemanticIdReferredSemanticIdSelects, supplementalSemanticIdReferredSemanticIdKeySelects := getSupplementalSemanticIdSelects()

	combined := []string{}
	combined = append(combined, submodelSelects...)
	combined = append(combined, displayNameSelects...)
	combined = append(combined, descriptionSelects...)
	combined = append(combined, semanticIdSelects...)
	combined = append(combined, semanticIdKeySelects...)
	combined = append(combined, semanticIdReferredSemanticIdSelects...)
	combined = append(combined, semanticIdReferredSemanticIdKeySelects...)
	combined = append(combined, supplementalSemanticIdSelects...)
	combined = append(combined, supplementalSemanticIdKeySelects...)
	combined = append(combined, supplementalSemanticIdReferredSemanticIdSelects...)
	combined = append(combined, supplementalSemanticIdReferredSemanticIdKeySelects...)

	builder := qb.NewSelect(combined...).From("submodel s")
	//----- DisplayName -----//
	addDisplayNameJoins(builder)
	//----- Description -----//
	addDescriptionJoins(builder)
	//----- SemanticId -----//
	addSemanticIdJoins(builder)
	//----- SupplementalSemanticIds -----//
	addSupplementalSemanticIdJoins(builder)

	query, args := builder.Build()
	rows, err := db.Query(query, args...)
	if err != nil {
		fmt.Printf("Error querying database: %v\n", err)
		return nil, err
	}
	return rows, nil
}

func getEmbeddedDataSpecificationsForSubmodel(db *sql.DB, id string) (*sql.Rows, error) {
	builder := qb.NewSelect(getEmbeddedDataSpecificationSelects()...).From("submodel s")
	addEmbeddedDataSpecificationJoins(builder)
	query, args := builder.Build()
	rows, err := db.Query(query, args...)
	if err != nil {
		fmt.Printf("Error querying database: %v\n", err)
		return nil, err
	}
	return rows, nil
}

func getSupplementalSemanticIdSelects() ([]string, []string, []string, []string) {
	supplementalSemanticIdSelects := []string{"supl_ref.type as submodel_supplemental_semantic_id_reference_type", "supl_ref.id as submodel_supplemental_semantic_id_dbid"}
	supplementalSemanticIdKeySelects := []string{"supl_rk.type as submodel_supplemental_semantic_id_key_type", "supl_rk.value as submodel_supplemental_semantic_id_key_value", "supl_rk.id as submodel_supplemental_semantic_id_key_id"}
	// SupplementalSemanticIds -> referredSemanticIds
	supplementalSemanticIdReferredSemanticIdSelects := []string{"supl_referredReference.id as submodel_semantic_id_referred_semantic_id_reference_id", "supl_referredReference.type as submodel_semantic_id_referred_semantic_id_reference_type", "supl_referredReference.parentReference as submodel_semantic_id_referred_parent_reference_id"}
	supplementalSemanticIdReferredSemanticIdKeySelects := []string{"supl_referredRK.type as submodel_semantic_id_referred_semantic_id_reference_key_type", "supl_referredRK.value as submodel_semantic_id_referred_semantic_id_reference_key_value", "supl_referredRK.id as submodel_semantic_id_referred_semantic_id_key_id"}
	return supplementalSemanticIdSelects, supplementalSemanticIdKeySelects, supplementalSemanticIdReferredSemanticIdSelects, supplementalSemanticIdReferredSemanticIdKeySelects
}

func getSemanticIdSelects() ([]string, []string, []string, []string) {
	semanticIdSelects := []string{"r.id as submodel_semantic_id_db_id", "r.type as submodel_semantic_id_reference_type"}
	semanticIdKeySelects := []string{"rk.type as submodel_semantic_id_reference_key_type", "rk.value as submodel_semantic_id_reference_key_value", "rk.id as submodel_semantic_id_key_id"}
	// SemanticId -> referredSemanticIds
	semanticIdReferredSemanticIdSelects := []string{"referredReference.id as submodel_semantic_id_referred_semantic_id_reference_id", "referredReference.type as submodel_semantic_id_referred_semantic_id_reference_type", "referredReference.parentReference as submodel_semantic_id_referred_parent_reference_id"}
	semanticIdReferredSemanticIdKeySelects := []string{"referredRK.type as submodel_semantic_id_referred_semantic_id_reference_key_type", "referredRK.value as submodel_semantic_id_referred_semantic_id_reference_key_value", "referredRK.id as submodel_semantic_id_referred_semantic_id_key_id"}
	return semanticIdSelects, semanticIdKeySelects, semanticIdReferredSemanticIdSelects, semanticIdReferredSemanticIdKeySelects
}

func getDescriptionSelects() []string {
	descriptionSelects := []string{"description.language as submodel_description_language", "description.text as submodel_description_text", "description.id as submodel_description_id"}
	return descriptionSelects
}

func getDisplayNameSelects() []string {
	displayNameSelects := []string{"dn.language as submodel_display_name_language", "dn.text as submodel_display_name_text", "dn.id as submodel_display_name_id"}
	return displayNameSelects
}

func getSubmodelSelects() []string {
	submodelSelects := []string{"s.id as submodel_id", "s.id_short as submodel_id_short", "s.category as submodel_category", "s.kind as submodel_kind"}
	return submodelSelects
}

func getEmbeddedDataSpecificationSelects() []string {
	return []string{
		// Embedded Data Specification Header
		"data_spec_reference.id as data_spec_reference_id",
		"data_spec_reference.type as data_spec_reference_type",
		"data_spec_reference_key.type as data_spec_reference_key_type",
		"data_spec_reference_key.value as data_spec_reference_key_value",
		"data_spec_reference_key.id as data_spec_reference_key_id",
		// iec61360 Content
		"iec61360.unit as dsc_iec_unit",
		"iec61360.source_of_definition as dsc_iec_sod",
		"iec61360.symbol as dsc_iec_symbol",
		"iec61360.data_type as dsc_iec_datatype",
		"iec61360.value_format as dsc_iec_valueformat",
		"iec61360.value as dsc_iec_value",
		// iec61360 preferredName
		"ds_pn.language as dsc_iec_preferred_name_language",
		"ds_pn.text as dsc_iec_preferred_name_text",
		"ds_pn.id as dsc_iec_preferred_name_id",
		// iec61360 shortName
		"ds_sn.language as dsc_iec_short_name_language",
		"ds_sn.text as dsc_iec_short_name_text",
		"ds_sn.id as dsc_iec_short_name_id",
		// iec61360 definition
		"ds_def.language as dsc_iec_definition_language",
		"ds_def.text as dsc_iec_definition_text",
		"ds_def.id as dsc_iec_definition_id",
	}
}

func addDisplayNameJoins(queryBuilder *qb.SelectBuilder) {
	queryBuilder.
		Join("LEFT JOIN lang_string_name_type_reference dn_ref ON s.displayname_id = dn_ref.id").
		Join("LEFT JOIN lang_string_name_type dn ON dn.lang_string_name_type_reference_id = dn_ref.id")
}
func addDescriptionJoins(queryBuilder *qb.SelectBuilder) {
	queryBuilder.
		Join("LEFT JOIN lang_string_text_type_reference desc_ref ON s.description_id = desc_ref.id").
		Join("LEFT JOIN lang_string_text_type description ON description.lang_string_text_type_reference_id = desc_ref.id")
}

func addSemanticIdJoins(queryBulder *qb.SelectBuilder) {
	queryBulder.
		Join("LEFT JOIN reference r ON s.semantic_id = r.id").
		Join("LEFT JOIN reference_key rk ON r.id = rk.reference_id").
		// SemanticId -> referredSemanticIds (get all references in the tree, excluding root)
		Join("LEFT JOIN reference referredReference ON referredReference.rootReference = r.id AND referredReference.id != r.id").
		Join("LEFT JOIN reference_key referredRK ON referredReference.id = referredRK.reference_id")
}

func addSupplementalSemanticIdJoins(queryBuilder *qb.SelectBuilder) {
	queryBuilder.
		Join("LEFT JOIN submodel_supplemental_semantic_id sssi ON s.id = sssi.submodel_id").
		Join("LEFT JOIN reference supl_ref ON sssi.reference_id = supl_ref.id").
		Join("LEFT JOIN reference_key supl_rk ON supl_ref.id = supl_rk.reference_id").
		// SupplementalSemanticIds -> ReferredSemanticIds (get all references in the tree, excluding root)
		Join("LEFT JOIN reference supl_referredReference ON supl_referredReference.rootReference = supl_ref.id").
		Join("LEFT JOIN reference_key supl_referredRK ON supl_referredReference.id = supl_referredRK.reference_id")
}

func addEmbeddedDataSpecificationJoins(queryBuilder *qb.SelectBuilder) {
	queryBuilder.
		Join("LEFT JOIN submodel_embedded_data_specification seds ON s.id = seds.submodel_id").
		Join("LEFT JOIN data_specification data_spec ON seds.embedded_data_specification_id = data_spec.id").
		// Data Specification Reference in Data Specification
		Join("LEFT JOIN reference data_spec_reference ON data_spec.data_specification = data_spec_reference.id").
		Join("LEFT JOIN reference_key data_spec_reference_key ON data_spec.id = data_spec_reference_key.reference_id").
		// Data Spec Ref ReferredSemanticId
		Join("LEFT JOIN reference data_spec_reference_referred ON data_spec_reference_referred.rootReference = data_spec_reference.id").
		Join("LEFT JOIN reference_key data_spec_reference_key_referred ON data_spec_reference_referred.id = data_spec_reference_key_referred.reference_id").
		// Data Specification Content in Data Specification
		Join("LEFT JOIN data_specification_content data_spec_content ON data_spec_content.id = data_spec.data_specification_content").
		// Data Specification IEC61360 Content in Data Specification Content
		Join("LEFT JOIN data_specification_iec61360 iec61360 ON data_spec_content.id = iec61360.id").
		// Preferred Name
		Join("LEFT JOIN lang_string_text_type_reference ds_pn_ref ON iec61360.preferred_name_id = ds_pn_ref.id").
		Join("LEFT JOIN lang_string_text_type ds_pn ON ds_pn.lang_string_text_type_reference_id = ds_pn_ref.id").
		// Short Name
		Join("LEFT JOIN lang_string_text_type_reference ds_sn_ref ON iec61360.short_name_id = ds_sn_ref.id").
		Join("LEFT JOIN lang_string_text_type ds_sn ON ds_sn.lang_string_text_type_reference_id = ds_sn_ref.id").
		// Definition
		Join("LEFT JOIN lang_string_text_type_reference ds_def_ref ON iec61360.short_name_id = ds_def_ref.id").
		Join("LEFT JOIN lang_string_text_type ds_def ON ds_def.lang_string_text_type_reference_id = ds_def_ref.id").
		// UnitId
		Join("LEFT JOIN reference ds_unit_id_ref ON iec61360.unit_id = ds_unit_id_ref.id").
		Join("LEFT JOIN reference_key ds_unit_key ON ds_unit_id_ref.id = ds_unit_key.reference_id").
		// UnitId -> ReferredSemanticId
		Join("LEFT JOIN reference ds_unit_ref_referred ON ds_unit_ref_referred.rootReference = ds_unit_id_ref.id").
		Join("LEFT JOIN reference_key ds_unit_key_referred ON ds_unit_ref_referred.id = ds_unit_key_referred.reference_id").
		// Value List
		Join("LEFT JOIN value_list ds_value_list ON iec61360.value_list_id = ds_value_list.id").
		Join("LEFT JOIN value_list_value_reference_pair ds_vlvrp ON ds_value_list.id = ds_vlvrp.value_list_id").
		// Level Type
		Join("LEFT JOIN level_type levelType ON levelType.id = iec61360.level_type_id")
}
