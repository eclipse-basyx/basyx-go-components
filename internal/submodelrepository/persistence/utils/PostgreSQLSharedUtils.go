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

// Package persistenceutils provides utility functions for persisting AAS entities
package persistenceutils

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// CreateExtension inserts an extension entity into the database.
//
// This function creates an extension record with its associated semantic IDs, supplemental semantic IDs,
// and refersTo references. The extension value is stored in different columns based on its data type
// (text, numeric, boolean, time, or datetime).
//
// Parameters:
//   - tx: Active database transaction
//   - extension: Extension object to be created
//
// Returns:
//   - sql.NullInt64: Database ID of the created extension
//   - error: An error if the insertion fails or if semantic ID creation fails
func CreateExtension(tx *sql.Tx, extension gen.Extension, position int) (sql.NullInt64, error) {
	var extensionDbID sql.NullInt64
	var semanticIDRefDbID sql.NullInt64

	if extension.SemanticID != nil {
		id, err := CreateReference(tx, extension.SemanticID, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			_, _ = fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error inserting SemanticID Reference for Extension.")
		}
		semanticIDRefDbID = id
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
	extension (name, position, value_type, value_text, value_num, value_bool, value_time, value_datetime, semantic_id)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	RETURNING id`, extension.Name, position, valueType, valueText, valueNum, valueBool, valueTime, valueDatetime, semanticIDRefDbID).Scan(&extensionDbID)

	if err != nil {
		_, _ = fmt.Println(err)
		return sql.NullInt64{}, common.NewInternalServerError("Error inserting Extension. See console for details.")
	}

	for _, supplemental := range extension.SupplementalSemanticIds {
		id, err := CreateReference(tx, &supplemental, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			_, _ = fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error inserting Supplemental Semantic IDs for Extension. See console for details.")
		}
		_, err = tx.Exec(`
		INSERT INTO
		extension_supplemental_semantic_id(extension_id, reference_id)
		VALUES($1, $2)
		`, extensionDbID, id)

		if err != nil {
			_, _ = fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error linking Supplemental Semantic IDs with Extension. See console for details.")
		}
	}

	for _, referred := range extension.RefersTo {
		id, err := CreateReference(tx, &referred, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			_, _ = fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error inserting RefersTo for Extension. See console for details.")
		}
		_, err = tx.Exec(`
		INSERT INTO
		extension_refers_to(extension_id, reference_id)
		VALUES($1, $2)
		`, extensionDbID, id)

		if err != nil {
			_, _ = fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error linking RefersTo Reference with Extension. See console for details.")
		}
	}

	return extensionDbID, nil
}

// CreateQualifier inserts a qualifier entity into the database.
//
// A qualifier is a characteristic that affects the value or interpretation of an element.
// This function creates a qualifier record with its associated semantic IDs, value IDs, and
// supplemental semantic IDs. The qualifier value is stored in different columns based on
// its data type (text, numeric, boolean, time, or datetime).
//
// Parameters:
//   - tx: Active database transaction
//   - qualifier: Qualifier object to be created
//
// Returns:
//   - sql.NullInt64: Database ID of the created qualifier
//   - error: An error if the insertion fails or if reference creation fails
func CreateQualifier(tx *sql.Tx, qualifier gen.Qualifier, position int) (sql.NullInt64, error) {
	var qualifierDbID sql.NullInt64
	var valueIDRefDbID sql.NullInt64
	var semanticIDRefDbID sql.NullInt64

	if qualifier.ValueID != nil {
		id, err := CreateReference(tx, qualifier.ValueID, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			_, _ = fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error inserting ValueID Reference for Qualifier.")
		}
		valueIDRefDbID = id
	}

	if qualifier.SemanticID != nil {
		id, err := CreateReference(tx, qualifier.SemanticID, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			_, _ = fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error inserting SemanticID Reference for Qualifier.")
		}
		semanticIDRefDbID = id
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
	qualifier (kind, position, type, value_type, value_text, value_num, value_bool, value_time, value_datetime, value_id, semantic_id)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	RETURNING id`, kind, position, qualifier.Type, qualifier.ValueType, valueText, valueNum, valueBool, valueTime, valueDatetime, valueIDRefDbID, semanticIDRefDbID).Scan(&qualifierDbID)

	if err != nil {
		_, _ = fmt.Println(err)
		return sql.NullInt64{}, common.NewInternalServerError("Error inserting Qualifier. See console for details.")
	}

	for _, supplemental := range qualifier.SupplementalSemanticIds {
		id, err := CreateReference(tx, &supplemental, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			_, _ = fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error inserting Supplemental Semantic IDs for Qualifier. See console for details.")
		}
		_, err = tx.Exec(`
		INSERT INTO
		qualifier_supplemental_semantic_id(qualifier_id, reference_id)
		VALUES($1, $2)
		`, qualifierDbID, id)

		if err != nil {
			_, _ = fmt.Println(err)
			return sql.NullInt64{}, common.NewInternalServerError("Error linking Supplemental Semantic IDs with Qualifier. See console for details.")
		}
	}

	return qualifierDbID, nil
}

// CreateEmbeddedDataSpecification inserts an embedded data specification into the database.
//
// An embedded data specification provides structured metadata about an element according to
// a specific data specification standard. This function currently supports DataSpecificationIec61360.
//
// Parameters:
//   - tx: Active database transaction
//   - embeddedDataSpecification: EmbeddedDataSpecification object containing the data specification reference
//     and its content
//
// Returns:
//   - sql.NullInt64: Database ID of the created embedded data specification
//   - error: An error if the insertion fails, if the content type is unsupported, or if IEC 61360 insertion fails
func CreateEmbeddedDataSpecification(tx *sql.Tx, embeddedDataSpecification gen.EmbeddedDataSpecification, position int) (sql.NullInt64, error) {
	var embeddedDataSpecificationContentDbID sql.NullInt64
	var embeddedDataSpecificationDbID sql.NullInt64
	err := tx.QueryRow(`INSERT INTO data_specification_content DEFAULT VALUES RETURNING id`).Scan(&embeddedDataSpecificationContentDbID)
	if err != nil {
		_, _ = fmt.Println(err)
		return sql.NullInt64{}, common.NewInternalServerError("Error inserting DataSpecificationContent. See console for details.")
	}
	dataSpecificationDbID, err := CreateReference(tx, embeddedDataSpecification.DataSpecification, sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		return sql.NullInt64{}, err
	}
	err = tx.QueryRow(`INSERT INTO data_specification (data_specification, data_specification_content) VALUES ($1, $2) RETURNING id`, dataSpecificationDbID, embeddedDataSpecificationContentDbID).Scan(&embeddedDataSpecificationDbID)
	if err != nil {
		return sql.NullInt64{}, err
	}
	// Check if embeddedDataSpecificationContent is of type DataSpecificationIec61360
	ds, ok := embeddedDataSpecification.DataSpecificationContent.(*gen.DataSpecificationIec61360)
	if !ok {
		return sql.NullInt64{}, common.NewErrBadRequest("Unsupported DataSpecificationContent type")
	}
	err = insertDataSpecificationIec61360(tx, ds, embeddedDataSpecificationContentDbID, position)
	if err != nil {
		return sql.NullInt64{}, err
	}

	return embeddedDataSpecificationDbID, nil
}

func insertDataSpecificationIec61360(tx *sql.Tx, ds *gen.DataSpecificationIec61360, embeddedDataSpecificationContentDbID sql.NullInt64, position int) error {
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

	// Insert UnitID (optional)
	unitIDDbID, err := CreateReference(tx, ds.UnitID, sql.NullInt64{}, sql.NullInt64{})
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
	levelTypeID, err := insertLevelType(tx, ds.LevelType)
	if err != nil {
		return err
	}

	var iec61360contentDbID sql.NullInt64

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
	err = tx.QueryRow("INSERT INTO data_specification_iec61360(id, position, preferred_name_id, short_name_id, unit, unit_id, source_of_definition, symbol, data_type, definition_id, value_format, value_list_id, level_type_id, value) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING id",
		embeddedDataSpecificationContentDbID, position, preferredNameID, shortNameID, unit, unitIDDbID, sourceOfDefinition, symbol, dataType, definitionID, valueFormat, valueList, levelTypeID, value).Scan(&iec61360contentDbID)
	if err != nil {
		return err
	}

	return nil
}

func insertValueList(tx *sql.Tx, valueList *gen.ValueList) (sql.NullInt64, error) {
	if valueList == nil {
		return sql.NullInt64{}, nil
	}
	var valueListDbID sql.NullInt64
	err := tx.QueryRow(`INSERT INTO value_list DEFAULT VALUES RETURNING id`).Scan(&valueListDbID)
	if err != nil {
		return sql.NullInt64{}, err
	}

	if len(valueList.ValueReferencePairs) > 0 {
		for i, vrp := range valueList.ValueReferencePairs {
			if vrp.ValueID != nil {
				valueIDDbID, err := CreateReference(tx, vrp.ValueID, sql.NullInt64{}, sql.NullInt64{})
				if err != nil {
					return sql.NullInt64{}, err
				}
				_, err = tx.Exec(`INSERT INTO value_list_value_reference_pair (value_list_id, position, value, value_id) VALUES ($1, $2, $3, $4)`, valueListDbID, i, vrp.Value, valueIDDbID)
				if err != nil {
					return sql.NullInt64{}, err
				}
			}
		}
	}
	return valueListDbID, nil
}

func insertLevelType(tx *sql.Tx, levelType *gen.LevelType) (sql.NullInt64, error) {
	if levelType == nil {
		return sql.NullInt64{}, nil
	}
	var levelTypeDbID sql.NullInt64
	err := tx.QueryRow(`INSERT INTO level_type (min, max, nom, typ) VALUES ($1, $2, $3, $4) RETURNING id`, levelType.Min, levelType.Max, levelType.Nom, levelType.Typ).Scan(&levelTypeDbID)
	if err != nil {
		return sql.NullInt64{}, err
	}
	return levelTypeDbID, nil
}

// CreateAdministrativeInformation inserts administrative information into the database.
//
// Administrative information includes version control, revision tracking, creator references,
// and embedded data specifications. This function handles the complete insertion including
// all nested structures.
//
// Parameters:
//   - tx: Active database transaction
//   - adminInfo: Pointer to AdministrativeInformation object to be created. Returns immediately if nil.
//
// Returns:
//   - sql.NullInt64: Database ID of the created administrative information record, or an invalid NullInt64 if adminInfo is nil
//   - error: An error if the insertion fails or if nested structure creation fails
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

		edsJSONString := "[]"
		if len(adminInfo.EmbeddedDataSpecifications) > 0 {
			edsBytes, err := json.Marshal(adminInfo.EmbeddedDataSpecifications)
			if err != nil {
				_, _ = fmt.Println(err)
				return sql.NullInt64{}, common.NewInternalServerError("Failed to marshal EmbeddedDataSpecifications - no changes applied - see console for details")
			}
			if edsBytes != nil {
				edsJSONString = string(edsBytes)
			}
		}

		err = tx.QueryRow(`INSERT INTO administrative_information (version, revision, creator, templateID, embedded_data_specification) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			adminInfo.Version, adminInfo.Revision, creatorID, adminInfo.TemplateID, edsJSONString).Scan(&id)
		if err != nil {
			return sql.NullInt64{}, err
		}
		adminInfoID = sql.NullInt64{Int64: int64(id), Valid: true}
	}
	return adminInfoID, nil
}

// CreateReference inserts a reference entity and its keys into the database.
//
// A reference consists of a type and a sequence of keys that form a path to an element.
// This function handles nested referred semantic IDs recursively. If the reference is empty
// or nil, it returns an invalid NullInt64 without inserting anything.
//
// Parameters:
//   - tx: Active database transaction
//   - semanticID: Pointer to the Reference object to be created
//   - parentID: Database ID of the parent reference (for nested structures)
//   - rootID: Database ID of the root reference (for nested structures)
//
// Returns:
//   - sql.NullInt64: Database ID of the created reference, or an invalid NullInt64 if the reference is empty
//   - error: An error if the insertion fails or if key insertion fails
func CreateReference(tx *sql.Tx, semanticID *gen.Reference, parentID sql.NullInt64, rootID sql.NullInt64) (sql.NullInt64, error) {
	var id int
	var referenceID sql.NullInt64

	insertKeyQuery := `INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`

	if semanticID != nil && !isEmptyReference(*semanticID) {
		err := tx.QueryRow(`INSERT INTO reference (type, parentReference, rootReference) VALUES ($1, $2, $3) RETURNING id`, semanticID.Type, parentID, rootID).Scan(&id)
		if err != nil {
			return sql.NullInt64{}, err
		}
		referenceID = sql.NullInt64{Int64: int64(id), Valid: true}

		references := semanticID.Keys
		for i := range references {
			_, err = tx.Exec(insertKeyQuery,
				id, i, references[i].Type, references[i].Value)
			if err != nil {
				return sql.NullInt64{}, err
			}
		}

		_, err = insertNestedRefferedSemanticIDs(semanticID, tx, referenceID, insertKeyQuery)
		if err != nil {
			return sql.NullInt64{}, err
		}
	}
	return referenceID, nil
}

func insertNestedRefferedSemanticIDs(semanticID *gen.Reference, tx *sql.Tx, referenceID sql.NullInt64, insertKeyQuery string) (sql.NullInt64, error) {
	stack := make([]*gen.Reference, 0)
	rootID := referenceID
	if semanticID.ReferredSemanticID != nil && !isEmptyReference(*semanticID.ReferredSemanticID) {
		stack = append(stack, semanticID.ReferredSemanticID)
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
		err := tx.QueryRow(`INSERT INTO reference (type, parentReference, rootReference) VALUES ($1, $2, $3) RETURNING id`, current.Type, parentReference, rootID).Scan(&childRefID)
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

		if current.ReferredSemanticID != nil && !isEmptyReference(*current.ReferredSemanticID) {
			stack = append(stack, current.ReferredSemanticID)
		}

		referenceID = childReferenceID
	}
	return referenceID, nil
}

// GetReferenceByReferenceDBID retrieves a reference and its keys from the database by its ID.
//
// This function reconstructs a Reference object from the database, including its type and all
// associated keys in the correct order. It does not retrieve nested referred semantic IDs.
//
// Parameters:
//   - db: Database connection
//   - referenceID: Database ID of the reference to retrieve
//
// Returns:
//   - *gen.Reference: The reconstructed Reference object, or nil if the reference ID is invalid or not found
//   - error: An error if the query fails or if type conversion fails
func GetReferenceByReferenceDBID(db *sql.DB, referenceID sql.NullInt64) (*gen.Reference, error) {
	if !referenceID.Valid {
		return nil, nil
	}
	var refType string
	// avoid driver-specific type casts in the query string which can confuse the pq parser
	ds := goqu.Dialect("postgres").
		From("reference").
		Select("type").
		Where(goqu.Ex{"id": referenceID.Int64})

	qRef, argsRef, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	err = db.QueryRow(qRef, argsRef...).Scan(&refType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	// similarly, select the type column directly and let the driver handle conversion
	ds = goqu.Dialect("postgres").
		From("reference_key").
		Select("type", "value").
		Where(goqu.Ex{"reference_id": referenceID.Int64}).
		Order(goqu.I("position").Asc())

	sqlQuery, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_, _ = fmt.Printf("Error closing rows: %v\n", closeErr)
		}
	}()

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

// CreateLangStringNameTypes inserts language-specific name strings into the database.
//
// This function creates a reference record and then inserts all language-specific name strings
// associated with it. Each name string contains text and language information.
//
// Parameters:
//   - tx: Active database transaction
//   - nameTypes: Slice of LangStringNameType objects to be created
//
// Returns:
//   - sql.NullInt64: Database ID of the created lang_string_name_type_reference, or an invalid NullInt64 if the slice is empty
//   - error: An error if the insertion fails
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

// GetLangStringNameTypes retrieves language-specific name strings from the database.
//
// This function fetches all language-specific name strings associated with a given reference ID.
//
// Parameters:
//   - db: Database connection
//   - nameTypeID: Database ID of the lang_string_name_type_reference
//
// Returns:
//   - []gen.LangStringNameType: Slice of retrieved LangStringNameType objects, or nil if the ID is invalid
//   - error: An error if the query fails
func GetLangStringNameTypes(db *sql.DB, nameTypeID sql.NullInt64) ([]gen.LangStringNameType, error) {
	if !nameTypeID.Valid {
		return nil, nil
	}
	var nameTypes []gen.LangStringNameType
	ds := goqu.Dialect("postgres").
		From("lang_string_name_type").
		Select("text", "language").
		Where(goqu.Ex{"lang_string_name_type_reference_id": nameTypeID.Int64})

	q, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_, _ = fmt.Printf("Error closing rows: %v\n", closeErr)
		}
	}()

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

// CreateLangStringTextTypes inserts language-specific text strings into the database.
//
// This function creates a reference record and then inserts all language-specific text strings
// associated with it. Each text string contains text and language information. The text types
// can be any implementation of the LangStringText interface.
//
// Parameters:
//   - tx: Active database transaction
//   - textTypes: Slice of LangStringText objects to be created
//
// Returns:
//   - sql.NullInt64: Database ID of the created lang_string_text_type_reference, or an invalid NullInt64 if the slice is empty
//   - error: An error if the insertion fails
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

// GetLangStringTextTypes retrieves language-specific text strings from the database.
//
// This function fetches all language-specific text strings associated with a given reference ID.
//
// Parameters:
//   - db: Database connection
//   - textTypeID: Database ID of the lang_string_text_type_reference
//
// Returns:
//   - []gen.LangStringTextType: Slice of retrieved LangStringTextType objects, or nil if the ID is invalid
//   - error: An error if the query fails
func GetLangStringTextTypes(db *sql.DB, textTypeID sql.NullInt64) ([]gen.LangStringTextType, error) {
	if !textTypeID.Valid {
		return nil, nil
	}
	var textTypes []gen.LangStringTextType
	ds := goqu.Dialect("postgres").
		From("lang_string_text_type").
		Select("text", "language").
		Where(goqu.Ex{"lang_string_text_type_reference_id": textTypeID.Int64})

	q, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_, _ = fmt.Printf("Error closing rows: %v\n", closeErr)
		}
	}()

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

// CreateExtensionForSubmodel creates an extension and links it to a submodel.
//
// This function creates a complete extension with all its associated references (semantic IDs,
// supplemental semantic IDs, and refersTo references) and then establishes the relationship
// between the extension and the specified submodel.
//
// Parameters:
//   - tx: Active database transaction
//   - submodel_id: String identifier of the submodel to link the extension to
//   - extension: Extension object to be created
//
// Returns:
//   - sql.NullInt64: Database ID of the created extension
//   - error: An error if the insertion fails or if any reference creation fails
func CreateExtensionForSubmodel(tx *sql.Tx, submodelID string, extension gen.Extension) (sql.NullInt64, error) {
	semanticIDDbID, err := CreateReference(tx, extension.SemanticID, sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		return sql.NullInt64{}, err
	}

	extensionDbID, err := insertExtension(extension, semanticIDDbID, tx)
	if err != nil {
		return sql.NullInt64{}, err
	}

	_, err = tx.Query("INSERT INTO submodel_extension (submodel_id, extension_id) VALUES ($1, $2)", submodelID, extensionDbID)
	if err != nil {
		return sql.NullInt64{}, err
	}

	err = insertSupplementalSemanticIDsForExtensions(extension, semanticIDDbID, tx, extensionDbID)
	if err != nil {
		return sql.NullInt64{}, err
	}

	err = insertRefersToReferences(extension, tx, extensionDbID)
	if err != nil {
		return sql.NullInt64{}, err
	}
	return extensionDbID, nil
}

func insertExtension(extension gen.Extension, semanticIDDbID sql.NullInt64, tx *sql.Tx) (sql.NullInt64, error) {
	var extensionDbID sql.NullInt64
	var valueText, valueNum, valueBool, valueTime, valueDatetime sql.NullString
	fillValueBasedOnType(extension, &valueText, &valueNum, &valueBool, &valueTime, &valueDatetime)
	ds := goqu.Dialect("postgres").
		Insert("extension").
		Cols(
			"semantic_id",
			"name",
			"value_type",
			"value_text",
			"value_num",
			"value_bool",
			"value_time",
			"value_datetime",
		).
		Vals(goqu.Vals{
			semanticIDDbID,
			extension.Name,
			extension.ValueType,
			valueText,
			valueNum,
			valueBool,
			valueTime,
			valueDatetime,
		}).
		Returning("id")

	q, args, err := ds.ToSQL()
	if err != nil {
		return sql.NullInt64{}, err
	}

	err = tx.QueryRow(q, args...).Scan(&extensionDbID)
	if err != nil {
		return sql.NullInt64{}, err
	}
	return extensionDbID, nil
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

func insertRefersToReferences(extension gen.Extension, tx *sql.Tx, extensionDbID sql.NullInt64) error {
	if len(extension.RefersTo) > 0 {
		for _, ref := range extension.RefersTo {
			refDbID, refErr := CreateReference(tx, &ref, sql.NullInt64{}, sql.NullInt64{})
			if refErr != nil {
				return refErr
			}
			ds := goqu.Dialect("postgres").
				Insert("extension_refers_to").
				Cols("extension_id", "reference_id").
				Vals(goqu.Vals{extensionDbID, refDbID})

			q, args, err := ds.ToSQL()
			if err != nil {
				return err
			}

			_, execErr := tx.Exec(q, args...)
			if execErr != nil {
				return execErr
			}
		}
	}
	return nil
}

func insertSupplementalSemanticIDsForExtensions(extension gen.Extension, semanticIDDbID sql.NullInt64, tx *sql.Tx, extensionDbID sql.NullInt64) error {
	if len(extension.SupplementalSemanticIds) > 0 {
		if !semanticIDDbID.Valid {
			return common.NewErrBadRequest("Supplemental Semantic IDs require a main Semantic ID to be present. (See AAS Constraint: AASd-118)")
		}
		for _, supplementalSemanticID := range extension.SupplementalSemanticIds {
			supplementalSemanticIDDbID, err := CreateReference(tx, &supplementalSemanticID, sql.NullInt64{}, sql.NullInt64{})
			if err != nil {
				return err
			}
			ds := goqu.Dialect("postgres").
				Insert("extension_supplemental_semantic_id").
				Cols("extension_id", "reference_id").
				Vals(goqu.Vals{extensionDbID, supplementalSemanticIDDbID})

			q, args, err := ds.ToSQL()
			if err != nil {
				return err
			}

			_, err = tx.Exec(q, args...)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// InsertSupplementalSemanticIDsSubmodel inserts supplemental semantic IDs for a submodel.
//
// Supplemental semantic IDs provide additional semantic information beyond the main semantic ID.
// This function creates all supplemental semantic ID references and links them to the specified submodel.
// According to AAS constraint AASd-118, supplemental semantic IDs require a main semantic ID to be present.
//
// Parameters:
//   - tx: Active database transaction
//   - submodel_id: String identifier of the submodel
//   - supplementalSemanticIds: Slice of pointers to Reference objects representing supplemental semantic IDs
//
// Returns:
//   - error: An error if reference creation or linking fails
func InsertSupplementalSemanticIDsSubmodel(tx *sql.Tx, submodelID string, supplementalSemanticIDs []*gen.Reference) error {
	if len(supplementalSemanticIDs) > 0 {
		for _, supplementalSemanticID := range supplementalSemanticIDs {
			supplementalSemanticIDDbID, err := CreateReference(tx, supplementalSemanticID, sql.NullInt64{}, sql.NullInt64{})
			if err != nil {
				return err
			}
			_, err = tx.Exec(`INSERT INTO submodel_supplemental_semantic_id (submodel_id, reference_id) VALUES ($1, $2)`, submodelID, supplementalSemanticIDDbID)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// InsertSupplementalSemanticIDsSME inserts supplemental semantic IDs for a submodel element.
//
// Supplemental semantic IDs provide additional semantic information beyond the main semantic ID.
// This function creates all supplemental semantic ID references and links them to the specified
// submodel element (SME). According to AAS constraint AASd-118, supplemental semantic IDs require
// a main semantic ID to be present.
//
// Parameters:
//   - tx: Active database transaction
//   - sme_id: Database ID of the submodel element
//   - supplementalSemanticIds: Slice of Reference objects representing supplemental semantic IDs
//
// Returns:
//   - error: An error if reference creation or linking fails
func InsertSupplementalSemanticIDsSME(tx *sql.Tx, smeID int64, supplementalSemanticIDs []gen.Reference) error {
	if len(supplementalSemanticIDs) > 0 {
		for _, supplementalSemanticID := range supplementalSemanticIDs {
			supplementalSemanticIDDbID, err := CreateReference(tx, &supplementalSemanticID, sql.NullInt64{}, sql.NullInt64{})
			if err != nil {
				return err
			}
			_, err = tx.Exec(`INSERT INTO submodel_element_supplemental_semantic_id (submodel_element_id, reference_id) VALUES ($1, $2)`, smeID, supplementalSemanticIDDbID)
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

type ValueOnlyElementsToProcess struct {
	Element     gen.SubmodelElementValue
	IdShortPath string
}

func BuildElementsToProcessStackValueOnly(db *sql.DB, submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) ([]ValueOnlyElementsToProcess, error) {
	stack := []ValueOnlyElementsToProcess{}
	elementsToProcess := []ValueOnlyElementsToProcess{}
	stack = append(stack, ValueOnlyElementsToProcess{
		Element:     valueOnly,
		IdShortPath: idShortOrPath,
	})
	// Build Iteratively
	for len(stack) > 0 {
		// Pop
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch elem := current.Element.(type) {
		case gen.AmbiguousSubmodelElementValue:
			// Check if it is a MLP or SME List in the database
			//1. GoQu query
			sqlQuery, args := buildCheckMultiLanguagePropertyOrSubmodelElementListQuery(current.IdShortPath, submodelID)
			row := db.QueryRow(sqlQuery, args...)
			var modelType string
			if err := row.Scan(&modelType); err != nil {
				return nil, err
			}
			if modelType == "MultiLanguageProperty" {
				mlpValue, err := elem.ConvertToMultiLanguagePropertyValue()
				if err != nil {
					return nil, err
				}
				el := ValueOnlyElementsToProcess{
					Element:     mlpValue,
					IdShortPath: current.IdShortPath,
				}
				elementsToProcess = append(elementsToProcess, el)
			} else {
				value, err := elem.ConvertToSubmodelElementListValue()
				if err != nil {
					return nil, err
				}
				for i, v := range value {
					stack = append(stack, ValueOnlyElementsToProcess{
						Element:     v,
						IdShortPath: current.IdShortPath + "[" + strconv.Itoa(i) + "]",
					})
				}
			}
		case gen.SubmodelElementCollectionValue:
			for idShort, v := range elem {
				stack = append(stack, ValueOnlyElementsToProcess{
					Element:     v,
					IdShortPath: current.IdShortPath + "." + idShort,
				})
			}
		case gen.SubmodelElementListValue:
			for i, v := range elem {
				el := ValueOnlyElementsToProcess{
					Element:     v,
					IdShortPath: current.IdShortPath + "[" + strconv.Itoa(i) + "]",
				}
				stack = append(stack, el)
			}
		case gen.AnnotatedRelationshipElementValue:
			el := ValueOnlyElementsToProcess{
				Element:     elem,
				IdShortPath: current.IdShortPath,
			}
			elementsToProcess = append(elementsToProcess, el)
			for idShort, annotation := range elem.Annotations {
				stack = append(stack, ValueOnlyElementsToProcess{
					Element:     annotation,
					IdShortPath: current.IdShortPath + "." + idShort,
				})
			}
		case gen.EntityValue:
			el := ValueOnlyElementsToProcess{
				Element:     elem,
				IdShortPath: current.IdShortPath,
			}
			elementsToProcess = append(elementsToProcess, el)
			for idShort, child := range elem.Statements {
				stack = append(stack, ValueOnlyElementsToProcess{
					Element:     child,
					IdShortPath: current.IdShortPath + "." + idShort,
				})
			}
		default:
			// Process basic element
			el := ValueOnlyElementsToProcess{
				Element:     elem,
				IdShortPath: current.IdShortPath,
			}
			elementsToProcess = append(elementsToProcess, el)
		}
	}
	return elementsToProcess, nil
}

func buildCheckMultiLanguagePropertyOrSubmodelElementListQuery(idShortOrPath string, submodelID string) (string, []interface{}) {
	sqlQuery := `
	SELECT sme.model_type
	FROM submodel_element sme
	WHERE sme.idshort_path = $1 AND sme.submodel_id = $2
	LIMIT 1;
	`
	args := []interface{}{idShortOrPath, submodelID}
	return sqlQuery, args
}
