package persistence_utils

import (
	"database/sql"
	"reflect"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	qb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/querybuilder"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

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
			creatorID, err = CreateReference(tx, adminInfo.Creator)
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
	}
	return adminInfoID, nil
}

func CreateReference(tx *sql.Tx, semanticId *gen.Reference) (sql.NullInt64, error) {
	var id int
	var referenceID sql.NullInt64

	insertKeyQuery := `INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`

	if semanticId != nil && !isEmptyReference(*semanticId) {
		err := tx.QueryRow(`INSERT INTO reference (type) VALUES ($1) RETURNING id`, semanticId.Type).Scan(&id)
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

		stack := make([]*gen.Reference, 0)
		referenceID, err = insertNestedRefferedSemanticIds(semanticId, stack, tx, referenceID, insertKeyQuery)
		if err != nil {
			return sql.NullInt64{}, err
		}
	}
	return referenceID, nil
}

func insertNestedRefferedSemanticIds(semanticId *gen.Reference, stack []*gen.Reference, tx *sql.Tx, referenceID sql.NullInt64, insertKeyQuery string) (sql.NullInt64, error) {
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
		err := tx.QueryRow(`INSERT INTO reference (type, parentReference) VALUES ($1, $2) RETURNING id`, current.Type, parentReference).Scan(&childRefID)
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

func CreateLangStringTextTypes(tx *sql.Tx, textTypes []gen.LangStringTextType) (sql.NullInt64, error) {
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
			_, err := tx.Exec(`INSERT INTO lang_string_text_type (lang_string_text_type_reference_id, text, language) VALUES ($1, $2, $3)`, textTypeID.Int64, textTypes[i].Text, textTypes[i].Language)
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

func CreateExtension(tx *sql.Tx, submodel_id string, extension gen.Extension) (sql.NullInt64, error) {
	var extensionDbId sql.NullInt64
	// Create SemanticId
	semanticIdDbId, err := CreateReference(tx, extension.SemanticId)
	if err != nil {
		return sql.NullInt64{}, err
	}

	extensionDbId, err = insertExtension(extension, submodel_id, semanticIdDbId, err, tx, extensionDbId)
	if err != nil {
		return sql.NullInt64{}, err
	}

	err = insertSupplementalSemanticIds(extension, semanticIdDbId, err, tx, extensionDbId)
	if err != nil {
		return sql.NullInt64{}, err
	}

	err = insertRefersToReferences(extension, semanticIdDbId, err, tx, extensionDbId)
	if err != nil {
		return sql.NullInt64{}, err
	}
	return extensionDbId, nil
}

func insertExtension(extension gen.Extension, submodel_id string, semanticIdDbId sql.NullInt64, err error, tx *sql.Tx, extensionDbId sql.NullInt64) (sql.NullInt64, error) {
	var valueText, valueNum, valueBool, valueTime, valueDatetime sql.NullString
	fillValueBasedOnType(extension, &valueText, &valueNum, &valueBool, &valueTime, &valueDatetime)

	q, args := qb.NewInsert("extension").
		Columns("submodel_id", "semantic_id", "name", "value_type", "value_text", "value_num", "value_bool", "value_time", "value_datetime").
		Values(submodel_id, semanticIdDbId, extension.Name, extension.ValueType, valueText, valueNum, valueBool, valueTime, valueDatetime).
		Returning("id").
		Build()

	err = tx.QueryRow(q, args...).Scan(&extensionDbId)
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

func insertRefersToReferences(extension gen.Extension, semanticIdDbId sql.NullInt64, err error, tx *sql.Tx, extensionDbId sql.NullInt64) error {
	if len(extension.RefersTo) > 0 {
		semanticIdDbId, err = CreateReference(tx, extension.SemanticId)
		if err != nil {
			return err
		}
		q, args := qb.NewInsert("extension_refers_to").
			Columns("extension_id", "reference_id").
			Values(extensionDbId, semanticIdDbId).
			Build()
		_, err = tx.Exec(q, args...)
		if err != nil {
			return err
		}
	}
	return nil
}

func insertSupplementalSemanticIds(extension gen.Extension, semanticIdDbId sql.NullInt64, err error, tx *sql.Tx, extensionDbId sql.NullInt64) error {
	if len(extension.SupplementalSemanticIds) > 0 {
		if !semanticIdDbId.Valid {
			return common.NewErrBadRequest("Supplemental Semantic IDs require a main Semantic ID to be present. (See AAS Constraint: AASd-118)")
		}
		semanticIdDbId, err = CreateReference(tx, extension.SemanticId)
		if err != nil {
			return err
		}
		q, args := qb.NewInsert("extension_supplemental_semantic_id").
			Columns("extension_id", "reference_id").
			Values(extensionDbId, semanticIdDbId).
			Build()
		_, err = tx.Exec(q, args...)
		if err != nil {
			return err
		}
	}
	return nil
}

// isEmptyReference checks if a Reference is empty (zero value)

func isEmptyReference(ref gen.Reference) bool {
	return reflect.DeepEqual(ref, gen.Reference{})
}
