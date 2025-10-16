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
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type SubmodelRow struct {
	Id                         string
	IdShort                    string
	Category                   string
	Kind                       string
	DisplayNames               json.RawMessage
	Descriptions               json.RawMessage
	SemanticId                 json.RawMessage
	ReferredSemanticIds        json.RawMessage
	SupplementalSemanticIds    json.RawMessage
	SupplementalReferredSemIds json.RawMessage
	EmbeddedDataSpecifications json.RawMessage
	DataSpecReferenceReferred  json.RawMessage
	DataSpecIEC61360           json.RawMessage
	IECLevelTypes              json.RawMessage
}

type ReferenceRow struct {
	ReferenceId   int64  `json:"reference_id"`
	ReferenceType string `json:"reference_type"`
	KeyID         int64  `json:"key_id"`
	KeyType       string `json:"key_type"`
	KeyValue      string `json:"key_value"`
}

type ReferredReferenceRow struct {
	SupplementalRootReferenceId int64  `json:"supplemental_root_reference_id"`
	ReferenceId                 int64  `json:"reference_id"`
	ReferenceType               string `json:"reference_type"`
	ParentReference             int64  `json:"parentReference"`
	RootReference               int64  `json:"rootReference"`
	KeyID                       int64  `json:"key_id"`
	KeyType                     string `json:"key_type"`
	KeyValue                    string `json:"key_value"`
}

func GetSubmodelJSON(db *sql.DB) ([]*gen.Submodel, error) {
	submodels := make(map[string]*gen.Submodel)
	result := []*gen.Submodel{}
	referenceBuilderRefs := make(map[int64]*builders.ReferenceBuilder)
	// //semanticIdReferredSemanticIdBuilderRefs := make(map[int64]*builders.ReferenceBuilder)
	// supplementalSemanticIdBuilderRefs := make(map[string]*builders.MultiReferenceBuilder)
	// nameTypeBuilderRefs := make(map[string]*builders.LangStringNameTypesBuilder)
	// textTypeBuilderRefs := make(map[string]*builders.LangStringTextTypesBuilder)

	// // Store pointers to the slices that will be populated
	// displayNameRefs := make(map[string]*[]gen.LangStringNameType)
	// descriptionRefs := make(map[string]*[]gen.LangStringTextType)
	rows, err := getSubmodelDataFromDbWithJSONQuery(db)
	if err != nil {
		return nil, fmt.Errorf("error getting submodel data from DB: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		var row SubmodelRow
		if err := rows.Scan(
			&row.Id, &row.IdShort, &row.Category, &row.Kind,
			&row.DisplayNames, &row.Descriptions,
			&row.SemanticId, &row.ReferredSemanticIds,
			&row.SupplementalSemanticIds, &row.SupplementalReferredSemIds,
			&row.EmbeddedDataSpecifications, &row.DataSpecReferenceReferred,
			&row.DataSpecIEC61360, &row.IECLevelTypes,
		); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}

		submodel := &gen.Submodel{
			Id:       row.Id,
			IdShort:  row.IdShort,
			Category: row.Category,
			Kind:     gen.ModellingKind(row.Kind),
		}

		semanticId, err := parseReferences(row.SemanticId, referenceBuilderRefs)
		if err != nil {
			return nil, err
		}

		if len(semanticId) == 1 {
			submodel.SemanticId = semanticId[0]
			parseReferredReferences(row.ReferredSemanticIds, referenceBuilderRefs)
		}

		supplementalSemanticIds, err := parseReferences(row.SupplementalSemanticIds, referenceBuilderRefs)
		if err != nil {
			return nil, err
		}
		if len(supplementalSemanticIds) > 0 {
			submodel.SupplementalSemanticIds = supplementalSemanticIds
			parseReferredReferences(row.SupplementalReferredSemIds, referenceBuilderRefs)
		}

		// DisplayNames
		if len(row.DisplayNames) > 0 {
			displayNames, err := ParseLangStringNameType(row.DisplayNames)
			if err != nil {
				return nil, fmt.Errorf("error parsing display names: %w", err)
			}
			submodel.DisplayName = displayNames
		}

		// Descriptions
		if len(row.Descriptions) > 0 {
			descriptions, err := ParseLangStringTextType(row.Descriptions)
			if err != nil {
				return nil, fmt.Errorf("error parsing descriptions: %w", err)
			}
			submodel.Description = descriptions
		}

		submodels[submodel.Id] = submodel
		result = append(result, submodel)
	}

	for _, referenceBuilder := range referenceBuilderRefs {
		referenceBuilder.BuildNestedStructure()
	}
	// for _, submodel := range submodels {
	// 	str, _ := json.Marshal(submodel)
	// 	fmt.Println(string(str))
	// }
	return result, nil
}

func parseReferredReferences(row json.RawMessage, referenceBuilderRefs map[int64]*builders.ReferenceBuilder) error {
	if len(row) > 0 {
		var semanticIdData []ReferredReferenceRow
		if err := json.Unmarshal(row, &semanticIdData); err != nil {
			return fmt.Errorf("error unmarshalling referred semantic ID data: %w", err)
		}
		for _, ref := range semanticIdData {
			builder, semanticIdCreated := referenceBuilderRefs[ref.RootReference]
			if !semanticIdCreated {
				return fmt.Errorf("parent reference with id %d not found for referred reference with id %d", ref.ParentReference, ref.ReferenceId)
			}
			builder.CreateReferredSemanticId(ref.ReferenceId, ref.ParentReference, ref.ReferenceType)
			builder.CreateReferredSemanticIdKey(ref.ReferenceId, ref.KeyID, ref.KeyType, ref.KeyValue)
		}
	}
	return nil
}

func parseReferences(row json.RawMessage, referenceBuilderRefs map[int64]*builders.ReferenceBuilder) ([]*gen.Reference, error) {
	var semanticId *gen.Reference
	var semanticIdBuilder *builders.ReferenceBuilder
	semanticId, semanticIdBuilder = nil, nil
	resultArray := make([]*gen.Reference, 0)
	if len(row) > 0 {
		var semanticIdData []ReferenceRow
		if err := json.Unmarshal(row, &semanticIdData); err != nil {
			return nil, fmt.Errorf("error unmarshalling semantic ID data: %w", err)
		}
		for _, ref := range semanticIdData {
			_, semanticIdCreated := referenceBuilderRefs[ref.ReferenceId]

			if !semanticIdCreated {
				semanticId, semanticIdBuilder = builders.NewReferenceBuilder(ref.ReferenceType, ref.ReferenceId)
				referenceBuilderRefs[ref.ReferenceId] = semanticIdBuilder
				resultArray = append(resultArray, semanticId)
			}
			semanticIdBuilder.CreateKey(ref.KeyID, ref.KeyType, ref.KeyValue)
		}
	}
	return resultArray, nil
}

func ParseLangStringNameType(displayNames json.RawMessage) ([]gen.LangStringNameType, error) {
	var names []gen.LangStringNameType
	// remove id field from json
	var temp []map[string]interface{}
	if err := json.Unmarshal(displayNames, &temp); err != nil {
		fmt.Printf("Error unmarshalling display names: %v\n", err)
		return nil, err
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Error parsing display names: %v\n", r)
		}
	}()

	for _, item := range temp {
		if _, ok := item["id"]; ok {
			delete(item, "id")
			names = append(names, gen.LangStringNameType{
				Text:     item["text"].(string),
				Language: item["language"].(string),
			})
		}
	}

	return names, nil
}

func ParseLangStringTextType(descriptions json.RawMessage) ([]gen.LangStringTextType, error) {
	var texts []gen.LangStringTextType
	// remove id field from json
	var temp []map[string]interface{}
	if err := json.Unmarshal(descriptions, &temp); err != nil {
		fmt.Printf("Error unmarshalling descriptions: %v\n", err)
		return nil, err
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Error parsing descriptions: %v\n", r)
		}
	}()

	for _, item := range temp {
		if _, ok := item["id"]; ok {
			delete(item, "id")
			texts = append(texts, gen.LangStringTextType{
				Text:     item["text"].(string),
				Language: item["language"].(string),
			})
		}
	}

	return texts, nil
}

func getSubmodelDataFromDbWithJSONQuery(db *sql.DB) (*sql.Rows, error) {
	rows, err := db.Query(getQuery())
	if err != nil {
		fmt.Printf("Error querying database: %v\n", err)
		return nil, err
	}
	return rows, nil
}

func getQuery() string {
	return `SELECT
  s.id                                                      AS submodel_id,
  s.id_short                                                AS submodel_id_short,
  s.category                                                AS submodel_category,
  s.kind                                                    AS submodel_kind,

  -- display names (lang_string_name_type)
  COALESCE((
    SELECT jsonb_agg(DISTINCT jsonb_build_object(
      'language', dn.language,
      'text', dn.text,
      'id', dn.id
    ))
    FROM lang_string_name_type_reference dn_ref
    JOIN lang_string_name_type dn
      ON dn.lang_string_name_type_reference_id = dn_ref.id
    WHERE dn_ref.id = s.displayname_id
  ), '[]'::jsonb)                                               AS submodel_display_names,

  -- descriptions (lang_string_text_type)
  COALESCE((
    SELECT jsonb_agg(DISTINCT jsonb_build_object(
      'language', d.language,
      'text', d.text,
      'id', d.id
    ))
    FROM lang_string_text_type_reference dr
    JOIN lang_string_text_type d
      ON d.lang_string_text_type_reference_id = dr.id
    WHERE dr.id = s.description_id
  ), '[]'::jsonb)                                               AS submodel_descriptions,

  -- semantic_id root reference with its keys
  COALESCE((
    SELECT jsonb_agg(DISTINCT jsonb_build_object(
      'reference_id', r.id,
      'reference_type', r.type,
      'key_id', rk.id,
      'key_type', rk.type,
      'key_value', rk.value
    ))
    FROM reference r
    LEFT JOIN reference_key rk ON rk.reference_id = r.id
    WHERE r.id = s.semantic_id
  ), '[]'::jsonb)                                               AS submodel_semantic_id,

  -- semantic_id -> referred references (children of rootReference)
  COALESCE((
    SELECT jsonb_agg(DISTINCT jsonb_build_object(
      'reference_id', ref.id,
      'reference_type', ref.type,
      'parentReference', ref.parentReference,
      'rootReference', ref.rootReference,
      'key_id', rk.id,
      'key_type', rk.type,
      'key_value', rk.value
    ))
    FROM reference ref
    LEFT JOIN reference_key rk ON rk.reference_id = ref.id
    WHERE ref.rootReference = s.semantic_id
      AND ref.id != s.semantic_id
  ), '[]'::jsonb)                                               AS submodel_semantic_id_referred,

  -- supplemental semantic ids (submodel_supplemental_semantic_id -> reference & keys)
  COALESCE((
    SELECT jsonb_agg(DISTINCT jsonb_build_object(
      'reference_id', ref.id,
      'reference_type', ref.type,
      'key_id', rk.id,
      'key_type', rk.type,
      'key_value', rk.value
    ))
    FROM submodel_supplemental_semantic_id sssi
    LEFT JOIN reference ref ON ref.id = sssi.reference_id
    LEFT JOIN reference_key rk ON rk.reference_id = ref.id
    WHERE sssi.submodel_id = s.id
  ), '[]'::jsonb)                                               AS submodel_supplemental_semantic_ids,

  -- supplemental semantic ids -> referred references (children of supplemental rootReference)
  COALESCE((
    SELECT jsonb_agg(DISTINCT jsonb_build_object(
      'supplemental_root_reference_id', sssi.reference_id,
      'reference_id', ref.id,
      'reference_type', ref.type,
      'parentReference', ref.parentReference,
      'rootReference', ref.rootReference,
      'key_id', rk.id,
      'key_type', rk.type,
      'key_value', rk.value
    ))
    FROM submodel_supplemental_semantic_id sssi
    LEFT JOIN reference ref ON ref.rootReference = sssi.reference_id
    LEFT JOIN reference_key rk ON rk.reference_id = ref.id
    WHERE sssi.submodel_id = s.id
      AND ref.id IS NOT NULL
  ), '[]'::jsonb)                                               AS submodel_supplemental_semantic_id_referred,

  -- data_specification references (submodel_embedded_data_specification -> data_specification -> reference + reference_key)
  COALESCE((
    SELECT jsonb_agg(DISTINCT jsonb_build_object(
      'submodel_id', seds.submodel_id,
      'data_specification_id', data_spec.id,
      'data_spec_reference_id', data_spec_reference.id,
      'data_spec_reference_type', data_spec_reference.type,
      'data_spec_reference_key_id', data_spec_reference_key.id,
      'data_spec_reference_key_type', data_spec_reference_key.type,
      'data_spec_reference_key_value', data_spec_reference_key.value
    ))
    FROM submodel_embedded_data_specification seds
    LEFT JOIN data_specification data_spec ON data_spec.id = seds.embedded_data_specification_id
    LEFT JOIN reference data_spec_reference ON data_spec.data_specification = data_spec_reference.id
    LEFT JOIN reference_key data_spec_reference_key ON data_spec.id = data_spec_reference_key.reference_id
    WHERE seds.submodel_id = s.id
  ), '[]'::jsonb)                                               AS submodel_embedded_data_specifications,

  -- data_specification -> referred references for the data_spec_reference (children)
  COALESCE((
    SELECT jsonb_agg(DISTINCT jsonb_build_object(
      'data_spec_reference_id', dsr.rootReference,       -- the root reference id
      'reference_id', ref.id,
      'reference_type', ref.type,
      'parentReference', ref.parentReference,
      'rootReference', ref.rootReference,
      'key_id', rk.id,
      'key_type', rk.type,
      'key_value', rk.value
    ))
    FROM submodel_embedded_data_specification seds
    LEFT JOIN data_specification data_spec ON data_spec.id = seds.embedded_data_specification_id
    LEFT JOIN reference data_spec_reference ON data_spec.data_specification = data_spec_reference.id
    LEFT JOIN reference ref ON ref.rootReference = data_spec_reference.id
    LEFT JOIN reference_key rk ON rk.reference_id = ref.id
    LEFT JOIN reference dsr ON dsr.id = data_spec_reference.id
    WHERE seds.submodel_id = s.id
      AND ref.id IS NOT NULL
  ), '[]'::jsonb)                                               AS submodel_data_spec_reference_referred,

  -- IEC61360 entries (aggregated per embedded data specification)
  COALESCE((
    SELECT jsonb_agg(DISTINCT jsonb_build_object(
      'iec_id', iec.id,
      'unit', iec.unit,
      'source_of_definition', iec.source_of_definition,
      'symbol', iec.symbol,
      'data_type', iec.data_type,
      'value_format', iec.value_format,
      'value', iec.value,
      'level_type_id', iec.level_type_id,
      -- preferred / short / definition lang strings (flat objects)
      'preferred_name', (SELECT jsonb_agg(DISTINCT jsonb_build_object('language', pn.language,'text', pn.text,'id', pn.id))
                         FROM lang_string_text_type_reference pn_ref
                         JOIN lang_string_text_type pn ON pn.lang_string_text_type_reference_id = pn_ref.id
                         WHERE pn_ref.id = iec.preferred_name_id),
      'short_name',   (SELECT jsonb_agg(DISTINCT jsonb_build_object('language', sn.language,'text', sn.text,'id', sn.id))
                         FROM lang_string_text_type_reference sn_ref
                         JOIN lang_string_text_type sn ON sn.lang_string_text_type_reference_id = sn_ref.id
                         WHERE sn_ref.id = iec.short_name_id),
      'definition',   (SELECT jsonb_agg(DISTINCT jsonb_build_object('language', df.language,'text', df.text,'id', df.id))
                         FROM lang_string_text_type_reference df_ref
                         JOIN lang_string_text_type df ON df.lang_string_text_type_reference_id = df_ref.id
                         WHERE df_ref.id = iec.definition_id),
      -- unit reference keys (if unit_id is a reference)
      'unit_reference_keys', (SELECT jsonb_agg(DISTINCT jsonb_build_object(
                                 'reference_id', dsu.id,
                                 'key_id', dsk.id,
                                 'key_type', dsk.type,
                                 'key_value', dsk.value))
                               FROM reference dsu
                               LEFT JOIN reference_key dsk ON dsk.reference_id = dsu.id
                               WHERE dsu.id = iec.unit_id),
      -- unit reference -> referred children keys
      'unit_reference_referred', (SELECT jsonb_agg(DISTINCT jsonb_build_object(
                                 'rootReference', dsu2.rootReference,
                                 'reference_id', dsu2.id,
                                 'key_id', dsk2.id,
                                 'key_type', dsk2.type,
                                 'key_value', dsk2.value))
                               FROM reference dsu2
                               LEFT JOIN reference_key dsk2 ON dsk2.reference_id = dsu2.id
                               WHERE dsu2.rootReference = iec.unit_id AND dsu2.id != iec.unit_id),
      -- value list entries if present
      'value_list_entries', (SELECT jsonb_agg(DISTINCT jsonb_build_object('value_reference_pair_id', vlvrp.id, 'reference_id', vlref.id))
                              FROM value_list vl
                              JOIN value_list_value_reference_pair vlvrp ON vl.id = vlvrp.value_list_id
                              LEFT JOIN reference vlref ON vlvrp.value_id = vlref.id
                              WHERE vl.id = iec.value_list_id)
    ))
    FROM submodel_embedded_data_specification seds
    JOIN data_specification ds ON ds.id = seds.embedded_data_specification_id
    JOIN data_specification_content dsc ON dsc.id = ds.data_specification_content
    JOIN data_specification_iec61360 iec ON iec.id = dsc.id
    WHERE seds.submodel_id = s.id
  ), '[]'::jsonb)                                               AS submodel_data_spec_iec61360,

  -- level_type entries referenced by IEC61360 (flat)
  COALESCE((
    SELECT jsonb_agg(DISTINCT jsonb_build_object(
      'iec_id', iec.id,
      'level_type_id', lvl.id
    ))
    FROM submodel_embedded_data_specification seds
    JOIN data_specification ds ON ds.id = seds.embedded_data_specification_id
    JOIN data_specification_content dsc ON dsc.id = ds.data_specification_content
    JOIN data_specification_iec61360 iec ON iec.id = dsc.id
    LEFT JOIN level_type lvl ON lvl.id = iec.level_type_id
    WHERE seds.submodel_id = s.id
  ), '[]'::jsonb)                                               AS submodel_iec_level_types

FROM submodel s;
`
}
