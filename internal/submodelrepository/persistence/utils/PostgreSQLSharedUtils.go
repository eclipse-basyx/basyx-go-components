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
	"fmt"
	"reflect"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	qb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/querybuilder"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

func CreateExtension(tx *sql.Tx, extension gen.Extension) (sql.NullInt64, error) {
	var extensionDbId sql.NullInt64
	var semanticIdRefDbId sql.NullInt64

	if extension.SemanticId != nil {
		id, err := CreateReference(tx, extension.SemanticId, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error inserting SemanticId Reference for Extension.")
		}
		semanticIdRefDbId = id
	}

	var valueText, valueNum, valueBool, valueTime, valueDatetime sql.NullString

	switch extension.ValueType {
	case "xs:string", "xs:anyURI", "xs:base64Binary", "xs:hexBinary":
		valueText = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	case "xs:int", "xs:integer", "xs:long", "xs:short", "xs:byte",
		"xs:unsignedInt", "xs:unsignedLong", "xs:unsignedShort", "xs:unsignedByte",
		"xs:positiveInteger", "xs:negativeInteger", "xs:nonNegativeInteger", "xs:nonPositiveInteger",
		"xs:decimal", "xs:double", "xs:float":
		valueNum = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	case "xs:boolean":
		valueBool = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	case "xs:time":
		valueTime = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	case "xs:date", "xs:dateTime", "xs:duration", "xs:gDay", "xs:gMonth",
		"xs:gMonthDay", "xs:gYear", "xs:gYearMonth":
		valueDatetime = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	default:
		// Fallback to text for unknown types
		valueText = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	}

	var valueType any
	if extension.ValueType != "" && !reflect.ValueOf(extension.ValueType).IsZero() {
		valueType = extension.ValueType
	} else {
		valueType = sql.NullString{String: "", Valid: false}
	}

	err := tx.QueryRow(`
	INSERT INTO
	extension (name, value_type, value_text, value_num, value_bool, value_time, value_datetime, semantic_id)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	RETURNING id`, extension.Name, valueType, valueText, valueNum, valueBool, valueTime, valueDatetime, semanticIdRefDbId).Scan(&extensionDbId)

	if err != nil {
		fmt.Println(err)
		return sql.NullInt64{}, common.NewInternalServerError("Error inserting Extension. See console for details.")
	}

	for _, supplemental := range extension.SupplementalSemanticIds {
		id, err := CreateReference(tx, &supplemental, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error inserting Supplemental Semantic IDs for Extension. See console for details.")
		}
		_, err = tx.Exec(`
		INSERT INTO
		extension_supplemental_semantic_id(extension_id, reference_id)
		VALUES($1, $2)
		`, extensionDbId, id)

		if err != nil {
			fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error linking Supplemental Semantic IDs with Extension. See console for details.")
		}
	}

	for _, referred := range extension.RefersTo {
		id, err := CreateReference(tx, &referred, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error inserting RefersTo for Extension. See console for details.")
		}
		_, err = tx.Exec(`
		INSERT INTO
		extension_refers_to(extension_id, reference_id)
		VALUES($1, $2)
		`, extensionDbId, id)

		if err != nil {
			fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error linking RefersTo Reference with Extension. See console for details.")
		}
	}

	return extensionDbId, nil
}

func CreateQualifier(tx *sql.Tx, qualifier gen.Qualifier) (sql.NullInt64, error) {
	var qualifierDbId sql.NullInt64
	var valueIdRefDbId sql.NullInt64
	var semanticIdRefDbId sql.NullInt64

	if qualifier.ValueId != nil {
		id, err := CreateReference(tx, qualifier.ValueId, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error inserting ValueID Reference for Qualifier.")
		}
		valueIdRefDbId = id
	}

	if qualifier.SemanticId != nil {
		id, err := CreateReference(tx, qualifier.SemanticId, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error inserting SemanticId Reference for Qualifier.")
		}
		semanticIdRefDbId = id
	}

	var valueText, valueNum, valueBool, valueTime, valueDatetime sql.NullString

	switch qualifier.ValueType {
	case "xs:string", "xs:anyURI", "xs:base64Binary", "xs:hexBinary":
		valueText = sql.NullString{String: qualifier.Value, Valid: qualifier.Value != ""}
	case "xs:int", "xs:integer", "xs:long", "xs:short", "xs:byte",
		"xs:unsignedInt", "xs:unsignedLong", "xs:unsignedShort", "xs:unsignedByte",
		"xs:positiveInteger", "xs:negativeInteger", "xs:nonNegativeInteger", "xs:nonPositiveInteger",
		"xs:decimal", "xs:double", "xs:float":
		valueNum = sql.NullString{String: qualifier.Value, Valid: qualifier.Value != ""}
	case "xs:boolean":
		valueBool = sql.NullString{String: qualifier.Value, Valid: qualifier.Value != ""}
	case "xs:time":
		valueTime = sql.NullString{String: qualifier.Value, Valid: qualifier.Value != ""}
	case "xs:date", "xs:dateTime", "xs:duration", "xs:gDay", "xs:gMonth",
		"xs:gMonthDay", "xs:gYear", "xs:gYearMonth":
		valueDatetime = sql.NullString{String: qualifier.Value, Valid: qualifier.Value != ""}
	default:
		// Fallback to text for unknown types
		valueText = sql.NullString{String: qualifier.Value, Valid: qualifier.Value != ""}
	}

	var kind any
	if qualifier.Kind != "" && !reflect.ValueOf(qualifier.Kind).IsZero() {
		kind = qualifier.Kind
	} else {
		kind = sql.NullString{String: "", Valid: false}
	}

	err := tx.QueryRow(`
	INSERT INTO
	qualifier (kind, type, value_type, value_text, value_num, value_bool, value_time, value_datetime, value_id, semantic_id)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	RETURNING id`, kind, qualifier.Type, qualifier.ValueType, valueText, valueNum, valueBool, valueTime, valueDatetime, valueIdRefDbId, semanticIdRefDbId).Scan(&qualifierDbId)

	if err != nil {
		fmt.Println(err)
		return sql.NullInt64{}, common.NewInternalServerError("Error inserting Qualifier. See console for details.")
	}

	for _, supplemental := range qualifier.SupplementalSemanticIds {
		id, err := CreateReference(tx, &supplemental, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error inserting Supplemental Semantic IDs for Qualifier. See console for details.")
		}
		_, err = tx.Exec(`
		INSERT INTO
		qualifier_supplemental_semantic_id(qualifier_id, reference_id)
		VALUES($1, $2)
		`, qualifierDbId, id)

		if err != nil {
			fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error linking Supplemental Semantic IDs with Qualifier. See console for details.")
		}
	}

	return qualifierDbId, nil
}
func CreateEmbeddedDataSpecification(tx *sql.Tx, embeddedDataSpecification gen.EmbeddedDataSpecification) (sql.NullInt64, error) {
	var embeddedDataSpecificationContentDbId sql.NullInt64
	var embeddedDataSpecificationDbId sql.NullInt64
	err := tx.QueryRow(`INSERT INTO data_specification_content DEFAULT VALUES RETURNING id`).Scan(&embeddedDataSpecificationContentDbId)
	if err != nil {
		fmt.Println(err)
		return sql.NullInt64{}, common.NewInternalServerError("Error inserting DataSpecificationContent. See console for details.")
	}
	dataSpecificationDbId, err := CreateReference(tx, embeddedDataSpecification.DataSpecification, sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		return sql.NullInt64{}, err
	}
	err = tx.QueryRow(`INSERT INTO data_specification (data_specification, data_specification_content) VALUES ($1, $2) RETURNING id`, dataSpecificationDbId, embeddedDataSpecificationContentDbId).Scan(&embeddedDataSpecificationDbId)
	if err != nil {
		return sql.NullInt64{}, err
	}
	// Check if embeddedDataSpecificationContent is of type DataSpecificationIec61360
	if ds, ok := embeddedDataSpecification.DataSpecificationContent.(*gen.DataSpecificationIec61360); ok {
		err = insertDataSpecificationIec61360(tx, ds, embeddedDataSpecificationContentDbId)
		if err != nil {
			return sql.NullInt64{}, err
		}
	} else {
		return sql.NullInt64{}, common.NewErrBadRequest("Unsupported DataSpecificationContent type")
	}

	return embeddedDataSpecificationDbId, nil
}

func insertDataSpecificationIec61360(tx *sql.Tx, ds *gen.DataSpecificationIec61360, embeddedDataSpecificationContentDbId sql.NullInt64) error {
	var preferredNameConverted []gen.LangStringText
	var shortNameConverted []gen.LangStringText
	var definitionConverted []gen.LangStringText

	// Convert PreferredName to []LangStringText (required field)
	for _, pn := range ds.PreferredName {
		preferredNameConverted = append(preferredNameConverted, pn)
	}
	// Convert ShortName to []LangStringText (optional)
	for _, sn := range ds.ShortName {
		shortNameConverted = append(shortNameConverted, sn)
	}

	// Convert Definition to []LangStringText (optional)
	for _, def := range ds.Definition {
		definitionConverted = append(definitionConverted, def)
	}

	// Insert PreferredName (required)
	preferredNameID, err := CreateLangStringTextTypes(tx, preferredNameConverted)
	if err != nil {
		return err
	}

	// Insert ShortName (optional)
	var shortNameID sql.NullInt64
	if len(shortNameConverted) > 0 {
		shortNameID, err = CreateLangStringTextTypes(tx, shortNameConverted)
		if err != nil {
			return err
		}
	}

	// Insert UnitId (optional)
	unitIdID, err := CreateReference(tx, ds.UnitId, sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		return err
	}

	// Insert Definition (optional)
	var definitionID sql.NullInt64
	if len(definitionConverted) > 0 {
		definitionID, err = CreateLangStringTextTypes(tx, definitionConverted)
		if err != nil {
			return err
		}
	}

	// Insert ValueList (optional)
	valueList, err := insertValueList(tx, ds.ValueList)
	if err != nil {
		return err
	}

	// Insert LevelType (optional)
	levelTypeId, err := insertLevelType(tx, ds.LevelType)
	if err != nil {
		return err
	}

	var iec61360contentDbId sql.NullInt64

	// Prepare optional string fields
	unit := sql.NullString{String: ds.Unit, Valid: ds.Unit != ""}
	sourceOfDefinition := sql.NullString{String: ds.SourceOfDefinition, Valid: ds.SourceOfDefinition != ""}
	symbol := sql.NullString{String: ds.Symbol, Valid: ds.Symbol != ""}
	valueFormat := sql.NullString{String: ds.ValueFormat, Valid: ds.ValueFormat != ""}
	value := sql.NullString{String: ds.Value, Valid: ds.Value != ""}

	// Handle DataType (optional)
	var dataType interface{}
	if ds.DataType != "" && !reflect.ValueOf(ds.DataType).IsZero() {
		dataType = ds.DataType
	} else {
		dataType = sql.NullString{String: "", Valid: false}
	}

	// INSERT
	err = tx.QueryRow("INSERT INTO data_specification_iec61360(id, preferred_name_id, short_name_id, unit, unit_id, source_of_definition, symbol, data_type, definition_id, value_format, value_list_id, level_type_id, value) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING id",
		embeddedDataSpecificationContentDbId, preferredNameID, shortNameID, unit, unitIdID, sourceOfDefinition, symbol, dataType, definitionID, valueFormat, valueList, levelTypeId, value).Scan(&iec61360contentDbId)
	if err != nil {
		return err
	}

	return nil
}

func insertValueList(tx *sql.Tx, valueList *gen.ValueList) (sql.NullInt64, error) {
	if valueList == nil {
		return sql.NullInt64{}, nil
	}
	var valueListDbId sql.NullInt64
	err := tx.QueryRow(`INSERT INTO value_list DEFAULT VALUES RETURNING id`).Scan(&valueListDbId)
	if err != nil {
		return sql.NullInt64{}, err
	}

	if len(valueList.ValueReferencePairs) > 0 {
		for i, vrp := range valueList.ValueReferencePairs {
			if vrp.ValueId != nil {
				valueIdDbId, err := CreateReference(tx, vrp.ValueId, sql.NullInt64{}, sql.NullInt64{})
				if err != nil {
					return sql.NullInt64{}, err
				}
				_, err = tx.Exec(`INSERT INTO value_list_value_reference_pair (value_list_id, position, value, value_id) VALUES ($1, $2, $3, $4)`, valueListDbId, i, vrp.Value, valueIdDbId)
				if err != nil {
					return sql.NullInt64{}, err
				}
			}
		}
	}
	return valueListDbId, nil
}

func insertLevelType(tx *sql.Tx, levelType *gen.LevelType) (sql.NullInt64, error) {
	if levelType == nil {
		return sql.NullInt64{}, nil
	}
	var levelTypeDbId sql.NullInt64
	err := tx.QueryRow(`INSERT INTO level_type (min, max, nom, typ) VALUES ($1, $2, $3, $4) RETURNING id`, levelType.Min, levelType.Max, levelType.Nom, levelType.Typ).Scan(&levelTypeDbId)
	if err != nil {
		return sql.NullInt64{}, err
	}
	return levelTypeDbId, nil
}

func CreateAdministrativeInformation(tx *sql.Tx, adminInfo *gen.AdministrativeInformation) (sql.NullInt64, error) {
	if adminInfo == nil {
		return sql.NullInt64{}, nil
	}
	var id int
	var adminInfoID sql.NullInt64
	if !reflect.DeepEqual(*adminInfo, gen.AdministrativeInformation{}) {
		var creatorID sql.NullInt64
		var err error
		if adminInfo.Creator != nil {
			creatorID, err = CreateReference(tx, adminInfo.Creator, sql.NullInt64{}, sql.NullInt64{})
			if err != nil {
				return sql.NullInt64{}, err
			}
		}

		err = tx.QueryRow(`INSERT INTO administrative_information (version, revision, creator, templateId) VALUES ($1, $2, $3, $4) RETURNING id`,
			adminInfo.Version, adminInfo.Revision, creatorID, adminInfo.TemplateId).Scan(&id)
		if err != nil {
			return sql.NullInt64{}, err
		}
		adminInfoID = sql.NullInt64{Int64: int64(id), Valid: true}

		if len(adminInfo.EmbeddedDataSpecifications) > 0 {
			for _, eds := range adminInfo.EmbeddedDataSpecifications {
				edsId, err := CreateEmbeddedDataSpecification(tx, eds)
				tx.Exec(`INSERT INTO administrative_information_embedded_data_specification (administrative_information_id, embedded_data_specification_id) VALUES ($1, $2)`, adminInfoID, edsId)
				if err != nil {
					return sql.NullInt64{}, err
				}
			}
		}
	}
	return adminInfoID, nil
}

func CreateReference(tx *sql.Tx, semanticId *gen.Reference, parentId sql.NullInt64, rootId sql.NullInt64) (sql.NullInt64, error) {
	var id int
	var referenceID sql.NullInt64

	insertKeyQuery := `INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`

	if semanticId != nil && !isEmptyReference(*semanticId) {
		err := tx.QueryRow(`INSERT INTO reference (type, parentReference, rootReference) VALUES ($1, $2, $3) RETURNING id`, semanticId.Type, parentId, rootId).Scan(&id)
		if err != nil {
			return sql.NullInt64{}, err
		}
		referenceID = sql.NullInt64{Int64: int64(id), Valid: true}

		references := semanticId.Keys
		for i := range references {
			_, err = tx.Exec(insertKeyQuery,
				id, i, references[i].Type, references[i].Value)
			if err != nil {
				return sql.NullInt64{}, err
			}
		}

		_, err = insertNestedRefferedSemanticIds(semanticId, tx, referenceID, insertKeyQuery)
		if err != nil {
			return sql.NullInt64{}, err
		}
	}
	return referenceID, nil
}

func insertNestedRefferedSemanticIds(semanticId *gen.Reference, tx *sql.Tx, referenceID sql.NullInt64, insertKeyQuery string) (sql.NullInt64, error) {
	stack := make([]*gen.Reference, 0)
	rootId := referenceID
	if semanticId.ReferredSemanticId != nil && !isEmptyReference(*semanticId.ReferredSemanticId) {
		stack = append(stack, semanticId.ReferredSemanticId)
	}
	for len(stack) > 0 {
		// Pop
		n := len(stack) - 1
		current := stack[n]
		stack = stack[:n]

		var childRefID int
		var parentReference interface{}
		if referenceID.Valid {
			parentReference = referenceID.Int64
		} else {
			parentReference = nil
		}
		err := tx.QueryRow(`INSERT INTO reference (type, parentReference, rootReference) VALUES ($1, $2, $3) RETURNING id`, current.Type, parentReference, rootId).Scan(&childRefID)
		if err != nil {
			return sql.NullInt64{}, err
		}
		childReferenceID := sql.NullInt64{Int64: int64(childRefID), Valid: true}

		for i := range current.Keys {
			_, err = tx.Exec(insertKeyQuery,
				childRefID, i, current.Keys[i].Type, current.Keys[i].Value)
			if err != nil {
				return sql.NullInt64{}, err
			}
		}

		if current.ReferredSemanticId != nil && !isEmptyReference(*current.ReferredSemanticId) {
			stack = append(stack, current.ReferredSemanticId)
		}

		referenceID = childReferenceID
	}
	return referenceID, nil
}

func GetReferenceByReferenceDBID(db *sql.DB, referenceID sql.NullInt64) (*gen.Reference, error) {
	if !referenceID.Valid {
		return nil, nil
	}
	var refType string
	// avoid driver-specific type casts in the query string which can confuse the pq parser
	qRef, argsRef := qb.NewSelect("type").
		From("reference").
		Where("id=$1", referenceID.Int64).
		Build()
	err := db.QueryRow(qRef, argsRef...).Scan(&refType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	// similarly, select the type column directly and let the driver handle conversion
	qKeys, argsKeys := qb.NewSelect("type", "value").
		From("reference_key").
		Where("reference_id=$1", referenceID.Int64).
		OrderBy("position").
		Build()
	rows, err := db.Query(qKeys, argsKeys...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []gen.Key
	for rows.Next() {
		var keyType, value string
		if err := rows.Scan(&keyType, &value); err != nil {
			return nil, err
		}
		cKeyType, err := gen.NewKeyTypesFromValue(keyType)
		if err != nil {
			return nil, err
		}
		keys = append(keys, gen.Key{Type: cKeyType, Value: value})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	cRefType, err := gen.NewReferenceTypesFromValue(refType)
	if err != nil {
		return nil, err
	}
	return &gen.Reference{
		Type: gen.ReferenceTypes(cRefType),
		Keys: keys,
	}, nil
}

func CreateLangStringNameTypes(tx *sql.Tx, nameTypes []gen.LangStringNameType) (sql.NullInt64, error) {
	if nameTypes == nil {
		return sql.NullInt64{}, nil
	}
	var id int
	var nameTypeID sql.NullInt64
	if len(nameTypes) > 0 {
		err := tx.QueryRow(`INSERT INTO lang_string_name_type_reference DEFAULT VALUES RETURNING id`).Scan(&id)
		if err != nil {
			return sql.NullInt64{}, err
		}
		nameTypeID = sql.NullInt64{Int64: int64(id), Valid: true}
		for i := 0; i < len(nameTypes); i++ {
			_, err := tx.Exec(`INSERT INTO lang_string_name_type (lang_string_name_type_reference_id, text, language) VALUES ($1, $2, $3)`, nameTypeID.Int64, nameTypes[i].Text, nameTypes[i].Language)
			if err != nil {
				return sql.NullInt64{}, err
			}
		}
	}
	return nameTypeID, nil
}

func GetLangStringNameTypes(db *sql.DB, nameTypeID sql.NullInt64) ([]gen.LangStringNameType, error) {
	if !nameTypeID.Valid {
		return nil, nil
	}
	var nameTypes []gen.LangStringNameType
	q, args := qb.NewSelect("text", "language").
		From("lang_string_name_type").
		Where("lang_string_name_type_reference_id=$1", nameTypeID.Int64).
		Build()
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var text, language string
		if err := rows.Scan(&text, &language); err != nil {
			return nil, err
		}
		nameTypes = append(nameTypes, gen.LangStringNameType{Text: text, Language: language})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nameTypes, nil
}

func CreateLangStringTextTypes(tx *sql.Tx, textTypes []gen.LangStringText) (sql.NullInt64, error) {
	if textTypes == nil {
		return sql.NullInt64{}, nil
	}
	var id int
	var textTypeID sql.NullInt64
	if len(textTypes) > 0 {
		err := tx.QueryRow(`INSERT INTO lang_string_text_type_reference DEFAULT VALUES RETURNING id`).Scan(&id)
		if err != nil {
			return sql.NullInt64{}, err
		}
		textTypeID = sql.NullInt64{Int64: int64(id), Valid: true}
		for i := 0; i < len(textTypes); i++ {
			_, err := tx.Exec(`INSERT INTO lang_string_text_type (lang_string_text_type_reference_id, text, language) VALUES ($1, $2, $3)`, textTypeID.Int64, textTypes[i].GetText(), textTypes[i].GetLanguage())
			if err != nil {
				return sql.NullInt64{}, err
			}
		}
	}
	return textTypeID, nil
}

func GetLangStringTextTypes(db *sql.DB, textTypeID sql.NullInt64) ([]gen.LangStringTextType, error) {
	if !textTypeID.Valid {
		return nil, nil
	}
	var textTypes []gen.LangStringTextType
	q, args := qb.NewSelect("text", "language").
		From("lang_string_text_type").
		Where("lang_string_text_type_reference_id=$1", textTypeID.Int64).
		Build()
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var text, language string
		if err := rows.Scan(&text, &language); err != nil {
			return nil, err
		}
		textTypes = append(textTypes, gen.LangStringTextType{Text: text, Language: language})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return textTypes, nil
}

func CreateExtensionForSubmodel(tx *sql.Tx, submodel_id string, extension gen.Extension) (sql.NullInt64, error) {
	// Create SemanticId
	semanticIdDbId, err := CreateReference(tx, extension.SemanticId, sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		return sql.NullInt64{}, err
	}

	extensionDbId, err := insertExtension(extension, semanticIdDbId, tx)
	if err != nil {
		return sql.NullInt64{}, err
	}

	_, err = tx.Query("INSERT INTO submodel_extension (submodel_id, extension_id) VALUES ($1, $2)", submodel_id, extensionDbId)
	if err != nil {
		return sql.NullInt64{}, err
	}

	err = insertSupplementalSemanticIdsForExtensions(extension, semanticIdDbId, tx, extensionDbId)
	if err != nil {
		return sql.NullInt64{}, err
	}

	err = insertRefersToReferences(extension, tx, extensionDbId)
	if err != nil {
		return sql.NullInt64{}, err
	}
	return extensionDbId, nil
}

func insertExtension(extension gen.Extension, semanticIdDbId sql.NullInt64, tx *sql.Tx) (sql.NullInt64, error) {
	var extensionDbId sql.NullInt64
	var valueText, valueNum, valueBool, valueTime, valueDatetime sql.NullString
	fillValueBasedOnType(extension, &valueText, &valueNum, &valueBool, &valueTime, &valueDatetime)
	// INSERT INTO extension(..,..,..,..) VALUES($1,$2,$3) RETURNING id
	q, args := qb.NewInsert("extension").
		Columns("semantic_id", "name", "value_type", "value_text", "value_num", "value_bool", "value_time", "value_datetime").
		Values(semanticIdDbId, extension.Name, extension.ValueType, valueText, valueNum, valueBool, valueTime, valueDatetime).
		Returning("id").
		Build()

	err := tx.QueryRow(q, args...).Scan(&extensionDbId)
	if err != nil {
		return sql.NullInt64{}, err
	}
	return extensionDbId, nil
}

func fillValueBasedOnType(extension gen.Extension, valueText *sql.NullString, valueNum *sql.NullString, valueBool *sql.NullString, valueTime *sql.NullString, valueDatetime *sql.NullString) {
	switch extension.ValueType {
	case "xs:string", "xs:anyURI", "xs:base64Binary", "xs:hexBinary":
		*valueText = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	case "xs:int", "xs:integer", "xs:long", "xs:short", "xs:byte",
		"xs:unsignedInt", "xs:unsignedLong", "xs:unsignedShort", "xs:unsignedByte",
		"xs:positiveInteger", "xs:negativeInteger", "xs:nonNegativeInteger", "xs:nonPositiveInteger",
		"xs:decimal", "xs:double", "xs:float":
		*valueNum = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	case "xs:boolean":
		*valueBool = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	case "xs:time":
		*valueTime = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	case "xs:date", "xs:dateTime", "xs:duration", "xs:gDay", "xs:gMonth",
		"xs:gMonthDay", "xs:gYear", "xs:gYearMonth":
		*valueDatetime = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	default:
		// Fallback to text for unknown types
		*valueText = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	}
}

func insertRefersToReferences(extension gen.Extension, tx *sql.Tx, extensionDbId sql.NullInt64) error {
	if len(extension.RefersTo) > 0 {
		for _, ref := range extension.RefersTo {
			refDbId, refErr := CreateReference(tx, &ref, sql.NullInt64{}, sql.NullInt64{})
			if refErr != nil {
				return refErr
			}
			q, args := qb.NewInsert("extension_refers_to").
				Columns("extension_id", "reference_id").
				Values(extensionDbId, refDbId).
				Build()
			_, execErr := tx.Exec(q, args...)
			if execErr != nil {
				return execErr
			}
		}
	}
	return nil
}

func insertSupplementalSemanticIdsForExtensions(extension gen.Extension, semanticIdDbId sql.NullInt64, tx *sql.Tx, extensionDbId sql.NullInt64) error {
	if len(extension.SupplementalSemanticIds) > 0 {
		if !semanticIdDbId.Valid {
			return common.NewErrBadRequest("Supplemental Semantic IDs require a main Semantic ID to be present. (See AAS Constraint: AASd-118)")
		}
		for _, supplementalSemanticId := range extension.SupplementalSemanticIds {
			supplementalSemanticIdDbId, err := CreateReference(tx, &supplementalSemanticId, sql.NullInt64{}, sql.NullInt64{})
			if err != nil {
				return err
			}
			q, args := qb.NewInsert("extension_supplemental_semantic_id").
				Columns("extension_id", "reference_id").
				Values(extensionDbId, supplementalSemanticIdDbId).
				Build()
			_, err = tx.Exec(q, args...)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func InsertSupplementalSemanticIds(tx *sql.Tx, submodel_id string, supplementalSemanticIds []*gen.Reference) error {
	if len(supplementalSemanticIds) > 0 {
		for _, supplementalSemanticId := range supplementalSemanticIds {
			supplementalSemanticIdDbId, err := CreateReference(tx, supplementalSemanticId, sql.NullInt64{}, sql.NullInt64{})
			if err != nil {
				return err
			}
			_, err = tx.Exec(`INSERT INTO submodel_supplemental_semantic_id (submodel_id, reference_id) VALUES ($1, $2)`, submodel_id, supplementalSemanticIdDbId)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// isEmptyReference checks if a Reference is empty (zero value)

func isEmptyReference(ref gen.Reference) bool {
	return reflect.DeepEqual(ref, gen.Reference{})
}
