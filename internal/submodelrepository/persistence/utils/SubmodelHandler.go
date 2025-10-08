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
	qb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/querybuilder"
	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
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
	nameTypeBuilderRefs := make(map[string]*builders.LangStringNameTypesBuilder)
	textTypeBuilderRefs := make(map[string]*builders.LangStringTextTypesBuilder)

	for rows.Next() {
		var id, idShort, category, kind, semantic_id_type, key_type, key_value, display_name_language, display_name_text, description_language, description_text sql.NullString
		var key_id, display_name_id, description_id sql.NullInt64

		err := rows.Scan(
			&id, &idShort, &category, &kind, // Submodel Header
			&semantic_id_type, &key_type, &key_value, &key_id, // SemanticId
			&display_name_language, &display_name_text, &display_name_id, // DisplayName
			&description_language, &description_text, &description_id) // Description

		if err != nil {
			fmt.Printf("Error scanning row: %v\n", err)
			continue
		}

		if isSubmodelAlreadyCreated(submodels, id) {
			ref, refBuilder := builders.NewReferenceBuilder(semantic_id_type.String)
			displayName, nameTypeBuilder := builders.NewLangStringNameTypesBuilder()
			description, textTypeBuilder := builders.NewLangStringTextTypesBuilder()

			semanticIdBuilderRefs[id.String] = refBuilder
			nameTypeBuilderRefs[id.String] = nameTypeBuilder
			textTypeBuilderRefs[id.String] = textTypeBuilder

			submodels[id.String] = &gen.Submodel{
				IdShort:     idShort.String,
				Id:          id.String,
				Category:    category.String,
				Kind:        gen.ModellingKind(kind.String),
				SemanticId:  ref,
				DisplayName: displayName,
				Description: description,
				ModelType:   "Submodel",
			}
		}

		if key_id.Valid {
			semanticIdBuilderRefs[id.String].CreateKey(int(key_id.Int64), key_type.String, key_value.String)
		}
		if display_name_id.Valid {
			nameTypeBuilderRefs[id.String].CreateLangStringNameType(int(display_name_id.Int64), display_name_language.String, display_name_text.String)
		}
		if description_id.Valid {
			textTypeBuilderRefs[id.String].CreateLangStringTextType(int(description_id.Int64), description_language.String, description_text.String)
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
	submodelSelects := []string{"s.id as submodel_id", "s.id_short as submodel_id_short", "s.category as submodel_category", "s.kind as submodel_kind"}
	referenceSelects := []string{"r.type as submodel_semantic_id_reference_type"}
	referenceKeySelects := []string{"rk.type as submodel_semantic_id_reference_key_type", "rk.value as submodel_semantic_id_reference_key_value", "rk.id as submodel_semantic_id_key_id"}
	displayNameSelects := []string{"dn.language as submodel_display_name_language", "dn.text as submodel_display_name_text", "dn.id as submodel_display_name_id"}
	descriptionSelects := []string{"description.language as submodel_description_language", "description.text as submodel_description_text", "description.id as submodel_description_id"}

	combined := []string{}
	combined = append(combined, submodelSelects...)
	combined = append(combined, referenceSelects...)
	combined = append(combined, referenceKeySelects...)
	combined = append(combined, displayNameSelects...)
	combined = append(combined, descriptionSelects...)

	query, args := qb.NewSelect(combined...).
		From("submodel s").
		// SemanticId
		Join("LEFT JOIN reference r ON s.semantic_id = r.id").
		Join("LEFT JOIN reference_key rk ON r.id = rk.reference_id").
		// DisplayName
		Join("LEFT JOIN lang_string_name_type_reference dn_ref ON s.displayname_id = dn_ref.id").
		Join("LEFT JOIN lang_string_name_type dn ON dn.lang_string_name_type_reference_id = dn_ref.id").
		// Description
		Join("LEFT JOIN lang_string_text_type_reference desc_ref ON s.description_id = desc_ref.id").
		Join("LEFT JOIN lang_string_text_type description ON description.lang_string_text_type_reference_id = desc_ref.id").
		Build()

	fmt.Print("Executing query:", query, " with args ", args, "\n\n")

	rows, err := db.Query(query, args...)
	if err != nil {
		fmt.Printf("Error querying database: %v\n", err)
		return nil, err
	}
	return rows, nil
}
