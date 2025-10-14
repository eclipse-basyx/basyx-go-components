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
	semanticIdReferredSemanticIdBuilderRefs := make(map[int64]*builders.ReferenceBuilder)
	supplementalSemanticIdBuilderRefs := make(map[string]*builders.MultiReferenceBuilder)
	nameTypeBuilderRefs := make(map[string]*builders.LangStringNameTypesBuilder)
	textTypeBuilderRefs := make(map[string]*builders.LangStringTextTypesBuilder)

	// Store pointers to the slices that will be populated
	displayNameRefs := make(map[string]*[]gen.LangStringNameType)
	descriptionRefs := make(map[string]*[]gen.LangStringTextType)

	// Children Store
	//semanticIdReferredSemanticIds := []*gen.Reference{}
	var lastReference *gen.Reference

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
		// Identifiers
		var key_id, display_name_id, description_id, supplemental_semantic_id_key_id, supplemental_semantic_id_dbid, referred_key_id, referred_semantic_id_db_id sql.NullInt64

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
		)

		//print new semantic id info
		fmt.Printf("DEBUG ROW: submodel=%s, semantic_db_id=%v, semantic_type=%s, key_id=%v, referred_id=%v, referred_type=%s, referred_parent=%v, referred_key_id=%v\n",
			id.String, semantic_id_db_id, semantic_id_type.String, key_id, referred_semantic_id_db_id, referred_semantic_id_type.String, referred_parent_reference_id, referred_key_id)

		if err != nil {
			fmt.Printf("Error scanning row: %v\n", err)
			continue
		}

		if !isSubmodelAlreadyCreated(submodels, id) {
			semanticId, semanticIdBuilder := builders.NewReferenceBuilder(semantic_id_type.String)
			lastReference = semanticId
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
		}

		if key_id.Valid {
			semanticIdBuilderRefs[id.String].CreateKey(key_id.Int64, key_type.String, key_value.String)
		}
		if referred_semantic_id_db_id.Valid {
			// Check if we already have a builder for this referredSemanticId
			var referredSemanticIdBuilder *builders.ReferenceBuilder
			if _, exists := semanticIdReferredSemanticIdBuilderRefs[referred_semantic_id_db_id.Int64]; !exists {
				referredSemanticId, newBuilder := builders.NewReferenceBuilder(referred_semantic_id_type.String)
				referredSemanticIdBuilder = newBuilder
				semanticIdReferredSemanticIdBuilderRefs[referred_semantic_id_db_id.Int64] = referredSemanticIdBuilder

				lastReference.ReferredSemanticId = referredSemanticId
				lastReference = referredSemanticId
			}
		}
		if referred_key_id.Valid {
			semanticIdReferredSemanticIdBuilderRefs[referred_semantic_id_db_id.Int64].CreateKey(referred_key_id.Int64, referred_key_type.String, referred_key_value.String)
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
	submodelSelects := []string{"s.id as submodel_id", "s.id_short as submodel_id_short", "s.category as submodel_category", "s.kind as submodel_kind"}
	// DisplayName
	displayNameSelects := []string{"dn.language as submodel_display_name_language", "dn.text as submodel_display_name_text", "dn.id as submodel_display_name_id"}
	// Description
	descriptionSelects := []string{"description.language as submodel_description_language", "description.text as submodel_description_text", "description.id as submodel_description_id"}
	// SemanticId
	semanticIdSelects := []string{"r.id as submodel_semantic_id_db_id", "r.type as submodel_semantic_id_reference_type"}
	semanticIdKeySelects := []string{"rk.type as submodel_semantic_id_reference_key_type", "rk.value as submodel_semantic_id_reference_key_value", "rk.id as submodel_semantic_id_key_id"}
	// SemanticId -> referredSemanticIds
	semanticIdReferredSemanticIdSelects := []string{"referredReference.id as submodel_semantic_id_referred_semantic_id_reference_id", "referredReference.type as submodel_semantic_id_referred_semantic_id_reference_type", "referredReference.parentReference as submodel_semantic_id_referred_parent_reference_id"}
	semanticIdReferredSemanticIdKeySelects := []string{"referredRK.type as submodel_semantic_id_referred_semantic_id_reference_key_type", "referredRK.value as submodel_semantic_id_referred_semantic_id_reference_key_value", "referredRK.id as submodel_semantic_id_referred_semantic_id_key_id"}
	// SupplementalSemanticIds
	supplementalSemanticIdSelects := []string{"supl_ref.type as submodel_supplemental_semantic_id_reference_type", "supl_ref.id as submodel_supplemental_semantic_id_dbid"}
	supplementalSemanticIdKeySelects := []string{"supl_rk.type as submodel_supplemental_semantic_id_key_type", "supl_rk.value as submodel_supplemental_semantic_id_key_value", "supl_rk.id as submodel_supplemental_semantic_id_key_id"}

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

	query, args := qb.NewSelect(combined...).
		From("submodel s").
		// DisplayName
		Join("LEFT JOIN lang_string_name_type_reference dn_ref ON s.displayname_id = dn_ref.id").
		Join("LEFT JOIN lang_string_name_type dn ON dn.lang_string_name_type_reference_id = dn_ref.id").
		// Description
		Join("LEFT JOIN lang_string_text_type_reference desc_ref ON s.description_id = desc_ref.id").
		Join("LEFT JOIN lang_string_text_type description ON description.lang_string_text_type_reference_id = desc_ref.id").
		//----- SemanticId -----//
		Join("LEFT JOIN reference r ON s.semantic_id = r.id").
		Join("LEFT JOIN reference_key rk ON r.id = rk.reference_id").
		// SemanticId -> referredSemanticIds (get all references in the tree, excluding root)
		Join("LEFT JOIN reference referredReference ON referredReference.rootReference = r.id AND referredReference.id != r.id").
		Join("LEFT JOIN reference_key referredRK ON referredReference.id = referredRK.reference_id").
		// SupplementalSemanticIds
		Join("LEFT JOIN submodel_supplemental_semantic_id sssi ON s.id = sssi.submodel_id").
		Join("LEFT JOIN reference supl_ref ON sssi.reference_id = supl_ref.id").
		Join("LEFT JOIN reference_key supl_rk ON supl_ref.id = supl_rk.reference_id").
		OrderBy("submodel_semantic_id_referred_semantic_id_reference_id, submodel_semantic_id_referred_semantic_id_key_id").
		Build()

	fmt.Print("Executing query:", query, " with args ", args, "\n\n")

	rows, err := db.Query(query, args...)
	if err != nil {
		fmt.Printf("Error querying database: %v\n", err)
		return nil, err
	}
	return rows, nil
}
