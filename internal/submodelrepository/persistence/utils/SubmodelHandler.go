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
				DisplayName: *displayName,
				Description: *description,
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

func GetSubmodelWithJson(db *sql.DB, submodelId string) {
	rows, err := getSubmodelDataFromDbJson(db, submodelId)
	if err != nil {
		fmt.Printf("Error getting submodel data from DB: %v\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id, idShort, category, kind                      string
			semanticIdJSON, displayNameJSON, descriptionJSON []byte
		)

		err := rows.Scan(
			&id, &idShort, &category, &kind,
			&semanticIdJSON, &displayNameJSON, &descriptionJSON,
		)
		if err != nil {
			fmt.Printf("Error scanning row: %v\n", err)
			continue
		}

		var submodel gen.Submodel
		submodel.Id = id
		submodel.IdShort = idShort
		submodel.Category = category
		submodel.Kind = gen.ModellingKind(kind)

		// Unmarshal JSON fields into proper structs
		json.Unmarshal(semanticIdJSON, &submodel.SemanticId)
		json.Unmarshal(displayNameJSON, &submodel.DisplayName)
		json.Unmarshal(descriptionJSON, &submodel.Description)
		submodel.ModelType = "Submodel"

		jsonData, _ := json.MarshalIndent(submodel, "", "  ")
		fmt.Println(string(jsonData))
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

func getSubmodelDataFromDbJson(db *sql.DB, id string) (*sql.Rows, error) {
	query := `
SELECT
	s.id,
	s.id_short,
	s.category,
	s.kind,
	-- SemanticId as JSON (nullable)
	CASE WHEN r.id IS NOT NULL THEN
		jsonb_build_object(
			'type', r.type,
			'keys', (
				SELECT jsonb_agg(
					jsonb_build_object(
						'type', rk.type,
						'value', rk.value
					)
					ORDER BY rk.position
				)
				FROM reference_key rk
				WHERE rk.reference_id = r.id
			)
		)
	END AS semantic_id,

	-- DisplayName as JSON (nullable)
	(
		SELECT jsonb_agg(
			jsonb_build_object(
				'language', ln.language,
				'text', ln.text
			)
			ORDER BY ln.id
		)
		FROM lang_string_name_type ln
		WHERE ln.lang_string_name_type_reference_id = s.displayname_id
	) AS display_name,

	-- Description as JSON (nullable)
	(
		SELECT jsonb_agg(
			jsonb_build_object(
				'language', lt.language,
				'text', lt.text
			)
			ORDER BY lt.id
		)
		FROM lang_string_text_type lt
		WHERE lt.lang_string_text_type_reference_id = s.description_id
	) AS description

FROM submodel s
LEFT JOIN reference r ON s.semantic_id = r.id
WHERE ($1 = '' OR s.id = $1)
ORDER BY s.id;
`

	rows, err := db.Query(query, id)
	if err != nil {
		return nil, fmt.Errorf("error querying database: %v", err)
	}
	return rows, nil
}

func GetSubmodelJson(db *sql.DB, submodelId string) (*gen.Submodel, error) {
	row, err := getSubmodelJsonFromDb(db, submodelId)
	if err != nil {
		return nil, err
	}

	var jsonData []byte
	if err := row.Scan(&jsonData); err != nil {
		return nil, fmt.Errorf("error scanning json: %v", err)
	}

	var submodel gen.Submodel
	if err := json.Unmarshal(jsonData, &submodel); err != nil {
		return nil, fmt.Errorf("error unmarshalling submodel json: %v", err)
	}

	return &submodel, nil
}

func getSubmodelJsonFromDb(db *sql.DB, submodelId string) (*sql.Row, error) {
	query := `
	SELECT jsonb_build_object(
		'id', s.id,
		'idShort', s.id_short,
		'category', s.category,
		'kind', s.kind,
		'semanticId', CASE WHEN r.id IS NOT NULL THEN
			jsonb_build_object(
				'type', r.type,
				'keys', (
					SELECT jsonb_agg(
						jsonb_build_object(
							'type', rk.type,
							'value', rk.value
						)
						ORDER BY rk.position
					)
					FROM reference_key rk
					WHERE rk.reference_id = r.id
				)
			)
		END,
		'displayName', (
			SELECT jsonb_agg(
				jsonb_build_object(
					'language', ln.language,
					'text', ln.text
				)
				ORDER BY ln.id
			)
			FROM lang_string_name_type ln
			WHERE ln.lang_string_name_type_reference_id = s.displayname_id
		),
		'description', (
			SELECT jsonb_agg(
				jsonb_build_object(
					'language', lt.language,
					'text', lt.text
				)
				ORDER BY lt.id
			)
			FROM lang_string_text_type lt
			WHERE lt.lang_string_text_type_reference_id = s.description_id
		),
		'elements', (
			SELECT jsonb_agg(
				jsonb_build_object(
					'id', sme.id,
					'parentId', sme.parent_sme_id,
					'idShort', sme.id_short,
					'category', sme.category,
					'modelType', sme.model_type,
					'idShortPath', sme.idshort_path,
					'semanticId', CASE WHEN rsme.id IS NOT NULL THEN
						jsonb_build_object(
							'type', rsme.type,
							'keys', (
								SELECT jsonb_agg(
									jsonb_build_object(
										'type', rksme.type,
										'value', rksme.value
									)
									ORDER BY rksme.position
								)
								FROM reference_key rksme
								WHERE rksme.reference_id = rsme.id
							)
						)
					END,
					'displayName', (
						SELECT jsonb_agg(
							jsonb_build_object(
								'language', ln.language,
								'text', ln.text
							)
							ORDER BY ln.id
						)
						FROM lang_string_name_type ln
						WHERE ln.lang_string_name_type_reference_id = sme.displayname_id
					),
					'description', (
						SELECT jsonb_agg(
							jsonb_build_object(
								'language', lt.language,
								'text', lt.text
							)
							ORDER BY lt.id
						)
						FROM lang_string_text_type lt
						WHERE lt.lang_string_text_type_reference_id = sme.description_id
					)
				)
				ORDER BY sme.id
			)
			FROM submodel_element sme
			LEFT JOIN reference rsme ON sme.semantic_id = rsme.id
			WHERE sme.submodel_id = s.id
		)
	) AS submodel_json
	FROM submodel s
	LEFT JOIN reference r ON s.semantic_id = r.id
	WHERE s.id = $1;
	`

	row := db.QueryRow(query, submodelId)
	return row, nil
}
