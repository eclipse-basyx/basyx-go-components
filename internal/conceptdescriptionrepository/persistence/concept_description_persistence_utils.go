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

package persistence

import (
	"database/sql"
	"strconv"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

type typedValue struct {
	text     sql.NullString
	numeric  sql.NullString
	boolean  sql.NullString
	time     sql.NullString
	date     sql.NullString
	dateTime sql.NullString
}

func mapValueByType(valueType types.DataTypeDefXSD, value *string) typedValue {
	result := typedValue{}
	valid := value != nil && *value != ""
	actualValue := ""
	if valid {
		actualValue = *value
	}
	switch {
	case isTextType(valueType):
		result.text = sql.NullString{String: actualValue, Valid: valid}
	case isNumericType(valueType):
		if valid && !isValidNumeric(actualValue) {
			result.text = sql.NullString{String: actualValue, Valid: valid}
		} else {
			result.numeric = sql.NullString{String: actualValue, Valid: valid}
		}
	case valueType == types.DataTypeDefXSDBoolean:
		result.boolean = sql.NullString{String: actualValue, Valid: valid}
	case valueType == types.DataTypeDefXSDTime:
		result.time = sql.NullString{String: actualValue, Valid: valid}
	case valueType == types.DataTypeDefXSDDate:
		result.date = sql.NullString{String: actualValue, Valid: valid}
	case isDateTimeType(valueType):
		result.dateTime = sql.NullString{String: actualValue, Valid: valid}
	default:
		result.text = sql.NullString{String: actualValue, Valid: valid}
	}

	return result
}

func isTextType(valueType types.DataTypeDefXSD) bool {
	switch valueType {
	case types.DataTypeDefXSDString, types.DataTypeDefXSDAnyURI, types.DataTypeDefXSDBase64Binary, types.DataTypeDefXSDHexBinary:
		return true
	default:
		return false
	}
}

func isNumericType(valueType types.DataTypeDefXSD) bool {
	switch valueType {
	case types.DataTypeDefXSDInt, types.DataTypeDefXSDInteger, types.DataTypeDefXSDLong, types.DataTypeDefXSDShort, types.DataTypeDefXSDByte,
		types.DataTypeDefXSDUnsignedInt, types.DataTypeDefXSDUnsignedLong, types.DataTypeDefXSDUnsignedShort, types.DataTypeDefXSDUnsignedByte,
		types.DataTypeDefXSDPositiveInteger, types.DataTypeDefXSDNegativeInteger, types.DataTypeDefXSDNonNegativeInteger, types.DataTypeDefXSDNonPositiveInteger,
		types.DataTypeDefXSDDecimal, types.DataTypeDefXSDDouble, types.DataTypeDefXSDFloat:
		return true
	default:
		return false
	}
}

func isDateTimeType(valueType types.DataTypeDefXSD) bool {
	switch valueType {
	case types.DataTypeDefXSDDateTime, types.DataTypeDefXSDDuration, types.DataTypeDefXSDGDay, types.DataTypeDefXSDGMonth,
		types.DataTypeDefXSDGMonthDay, types.DataTypeDefXSDGYear, types.DataTypeDefXSDGYearMonth:
		return true
	default:
		return false
	}
}

func isValidNumeric(value string) bool {
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

func conceptDescriptionExists(tx *sql.Tx, id string) (bool, error) {
	var exists bool
	err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM concept_description WHERE id = $1)`, id).Scan(&exists)
	if err != nil {
		return false, common.NewInternalServerError("CDREPO-CRCD-EXISTS " + err.Error())
	}
	return exists, nil
}

func createLangStringTextTypes(tx *sql.Tx, textTypes []types.ILangStringTextType) (sql.NullInt64, error) {
	if len(textTypes) == 0 {
		return sql.NullInt64{}, nil
	}
	var id int64
	if err := tx.QueryRow(`INSERT INTO lang_string_text_type_reference DEFAULT VALUES RETURNING id`).Scan(&id); err != nil {
		return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRLST-REF " + err.Error())
	}
	for _, textType := range textTypes {
		_, err := tx.Exec(`INSERT INTO lang_string_text_type (lang_string_text_type_reference_id, text, language) VALUES ($1, $2, $3)`, id, textType.Text(), textType.Language())
		if err != nil {
			return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRLST-INS " + err.Error())
		}
	}
	return sql.NullInt64{Int64: id, Valid: true}, nil
}

func createLangStringNameTypes(tx *sql.Tx, nameTypes []types.ILangStringNameType) (sql.NullInt64, error) {
	if len(nameTypes) == 0 {
		return sql.NullInt64{}, nil
	}
	var id int64
	if err := tx.QueryRow(`INSERT INTO lang_string_name_type_reference DEFAULT VALUES RETURNING id`).Scan(&id); err != nil {
		return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRLSN-REF " + err.Error())
	}
	for _, nameType := range nameTypes {
		_, err := tx.Exec(`INSERT INTO lang_string_name_type (lang_string_name_type_reference_id, text, language) VALUES ($1, $2, $3)`, id, nameType.Text(), nameType.Language())
		if err != nil {
			return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRLSN-INS " + err.Error())
		}
	}
	return sql.NullInt64{Int64: id, Valid: true}, nil
}

func isEmptyReference(ref types.IReference) bool {
	if ref == nil {
		return true
	}
	if len(ref.Keys()) == 0 {
		return true
	}
	return false
}

func createReference(tx *sql.Tx, ref types.IReference, parentID sql.NullInt64, rootID sql.NullInt64) (sql.NullInt64, error) {
	if ref == nil || isEmptyReference(ref) {
		return sql.NullInt64{}, nil
	}

	var refID int64
	if err := tx.QueryRow(`INSERT INTO reference (type, parentReference, rootReference) VALUES ($1, $2, $3) RETURNING id`, ref.Type(), parentID, rootID).Scan(&refID); err != nil {
		return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRREF-INS " + err.Error())
	}

	for i, key := range ref.Keys() {
		_, err := tx.Exec(`INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`, refID, i, key.Type(), key.Value())
		if err != nil {
			return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRREF-KEY " + err.Error())
		}
	}

	root := sql.NullInt64{Int64: refID, Valid: true}
	if rootID.Valid {
		root = rootID
	}
	if _, err := insertNestedReferredSemanticIDs(tx, ref, sql.NullInt64{Int64: refID, Valid: true}, root); err != nil {
		return sql.NullInt64{}, err
	}

	return sql.NullInt64{Int64: refID, Valid: true}, nil
}

func insertNestedReferredSemanticIDs(tx *sql.Tx, ref types.IReference, parentID sql.NullInt64, rootID sql.NullInt64) (sql.NullInt64, error) {
	stack := make([]types.IReference, 0)
	if ref.ReferredSemanticID() != nil && !isEmptyReference(ref.ReferredSemanticID()) {
		stack = append(stack, ref.ReferredSemanticID())
	}
	for len(stack) > 0 {
		idx := len(stack) - 1
		current := stack[idx]
		stack = stack[:idx]

		var childID int64
		var parentReference interface{}
		if parentID.Valid {
			parentReference = parentID.Int64
		} else {
			parentReference = nil
		}
		if err := tx.QueryRow(`INSERT INTO reference (type, parentReference, rootReference) VALUES ($1, $2, $3) RETURNING id`, current.Type(), parentReference, rootID).Scan(&childID); err != nil {
			return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRREF-REFSEM " + err.Error())
		}
		for i, key := range current.Keys() {
			_, err := tx.Exec(`INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`, childID, i, key.Type(), key.Value())
			if err != nil {
				return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRREF-REFSEMKEY " + err.Error())
			}
		}
		if current.ReferredSemanticID() != nil && !isEmptyReference(current.ReferredSemanticID()) {
			stack = append(stack, current.ReferredSemanticID())
		}
	}
	return sql.NullInt64{}, nil
}

func createEmbeddedDataSpecification(tx *sql.Tx, eds types.IEmbeddedDataSpecification, position int) (sql.NullInt64, error) {
	var edsContentID sql.NullInt64
	if err := tx.QueryRow(`INSERT INTO data_specification_content DEFAULT VALUES RETURNING id`).Scan(&edsContentID); err != nil {
		return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CREDS-CONTENT " + err.Error())
	}
	dataSpecID, err := createReference(tx, eds.DataSpecification(), sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		return sql.NullInt64{}, err
	}
	var edsID sql.NullInt64
	if err := tx.QueryRow(`INSERT INTO data_specification (data_specification, data_specification_content) VALUES ($1, $2) RETURNING id`, dataSpecID, edsContentID).Scan(&edsID); err != nil {
		return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CREDS-INS " + err.Error())
	}

	iec, ok := eds.DataSpecificationContent().(*types.DataSpecificationIEC61360)
	if !ok {
		return sql.NullInt64{}, common.NewErrBadRequest("Unsupported DataSpecificationContent type")
	}
	if err := insertDataSpecificationIec61360(tx, iec, edsContentID, position); err != nil {
		return sql.NullInt64{}, err
	}
	return edsID, nil
}

func insertDataSpecificationIec61360(tx *sql.Tx, ds *types.DataSpecificationIEC61360, contentID sql.NullInt64, position int) error {
	var preferredName []types.ILangStringTextType
	var shortName []types.ILangStringTextType
	var definition []types.ILangStringTextType

	for _, pn := range ds.PreferredName() {
		preferredName = append(preferredName, pn)
	}
	for _, sn := range ds.ShortName() {
		shortName = append(shortName, sn)
	}
	for _, def := range ds.Definition() {
		definition = append(definition, def)
	}

	preferredNameID, err := createLangStringTextTypes(tx, preferredName)
	if err != nil {
		return err
	}

	shortNameID, err := createLangStringTextTypes(tx, shortName)
	if err != nil {
		return err
	}

	unitID, err := createReference(tx, ds.UnitID(), sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		return err
	}

	definitionID, err := createLangStringTextTypes(tx, definition)
	if err != nil {
		return err
	}

	valueListID, err := insertValueList(tx, ds.ValueList())
	if err != nil {
		return err
	}
	levelTypeID, err := insertLevelType(tx, ds.LevelType())
	if err != nil {
		return err
	}

	unit := sql.NullString{String: stringValue(ds.Unit()), Valid: ds.Unit() != nil && *ds.Unit() != ""}
	sourceOfDefinition := sql.NullString{String: stringValue(ds.SourceOfDefinition()), Valid: ds.SourceOfDefinition() != nil && *ds.SourceOfDefinition() != ""}
	symbol := sql.NullString{String: stringValue(ds.Symbol()), Valid: ds.Symbol() != nil && *ds.Symbol() != ""}
	valueFormat := sql.NullString{String: stringValue(ds.ValueFormat()), Valid: ds.ValueFormat() != nil && *ds.ValueFormat() != ""}
	value := sql.NullString{String: stringValue(ds.Value()), Valid: ds.Value() != nil && *ds.Value() != ""}

	var dataType interface{}
	if ds.DataType() != nil {
		dataType = *ds.DataType()
	} else {
		dataType = sql.NullString{String: "", Valid: false}
	}

	_, err = tx.Exec(`
		INSERT INTO data_specification_iec61360
		(id, position, preferred_name_id, short_name_id, unit, unit_id, source_of_definition, symbol, data_type, definition_id, value_format, value_list_id, level_type_id, value)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, contentID, position, preferredNameID, shortNameID, unit, unitID, sourceOfDefinition, symbol, dataType, definitionID, valueFormat, valueListID, levelTypeID, value)
	if err != nil {
		return common.NewInternalServerError("CDREPO-CRDS-IEC " + err.Error())
	}

	return nil
}

func insertValueList(tx *sql.Tx, valueList types.IValueList) (sql.NullInt64, error) {
	if valueList == nil {
		return sql.NullInt64{}, nil
	}
	var valueListID sql.NullInt64
	if err := tx.QueryRow(`INSERT INTO value_list DEFAULT VALUES RETURNING id`).Scan(&valueListID); err != nil {
		return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRVL-INS " + err.Error())
	}
	for i, pair := range valueList.ValueReferencePairs() {
		valueID, err := createReference(tx, pair.ValueID(), sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			return sql.NullInt64{}, err
		}
		_, err = tx.Exec(`INSERT INTO value_list_value_reference_pair (value_list_id, position, value, value_id) VALUES ($1, $2, $3, $4)`, valueListID, i, pair.Value(), valueID)
		if err != nil {
			return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRVL-PAIR " + err.Error())
		}
	}
	return valueListID, nil
}

func insertLevelType(tx *sql.Tx, levelType types.ILevelType) (sql.NullInt64, error) {
	if levelType == nil {
		return sql.NullInt64{}, nil
	}
	var levelTypeID sql.NullInt64
	if err := tx.QueryRow(`INSERT INTO level_type (min, max, nom, typ) VALUES ($1, $2, $3, $4) RETURNING id`, levelType.Min(), levelType.Max(), levelType.Nom(), levelType.Typ()).Scan(&levelTypeID); err != nil {
		return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRLT-INS " + err.Error())
	}
	return levelTypeID, nil
}

func createAdministrativeInformation(tx *sql.Tx, adminInfo types.IAdministrativeInformation) (sql.NullInt64, error) {
	if adminInfo == nil {
		return sql.NullInt64{}, nil
	}
	var creatorID sql.NullInt64
	if !isEmptyReference(adminInfo.Creator()) {
		var err error
		creatorID, err = createReference(tx, adminInfo.Creator(), sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			return sql.NullInt64{}, err
		}
	}

	var adminID sql.NullInt64
	if err := tx.QueryRow(`INSERT INTO administrative_information (version, revision, creator, templateID) VALUES ($1, $2, $3, $4) RETURNING id`, adminInfo.Version(), adminInfo.Revision(), creatorID, adminInfo.TemplateID()).Scan(&adminID); err != nil {
		return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRAI-INS " + err.Error())
	}

	for i, eds := range adminInfo.EmbeddedDataSpecifications() {
		edSId, err := createEmbeddedDataSpecification(tx, eds, i)
		if err != nil {
			return sql.NullInt64{}, err
		}
		_, err = tx.Exec(`INSERT INTO administrative_information_embedded_data_specification (administrative_information_id, embedded_data_specification_id) VALUES ($1, $2)`, adminID, edSId)
		if err != nil {
			return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CRAI-EDS " + err.Error())
		}
	}

	return adminID, nil
}

func insertConceptDescriptionEmbeddedDataSpecifications(tx *sql.Tx, cdID string, eds []types.IEmbeddedDataSpecification) error {
	for i, spec := range eds {
		edsID, err := createEmbeddedDataSpecification(tx, spec, i)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO concept_description_embedded_data_specification (concept_description_id, position, embedded_data_specification_id) VALUES ($1, $2, $3)`, cdID, i, edsID)
		if err != nil {
			return common.NewInternalServerError("CDREPO-CRCD-EDS " + err.Error())
		}
	}
	return nil
}

func insertConceptDescriptionIsCaseOf(tx *sql.Tx, cdID string, refs []types.IReference) error {
	for i, ref := range refs {
		refID, err := createReference(tx, ref, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO concept_description_is_case_of (concept_description_id, position, reference_id) VALUES ($1, $2, $3)`, cdID, i, refID)
		if err != nil {
			return common.NewInternalServerError("CDREPO-CRCD-ISCOF " + err.Error())
		}
	}
	return nil
}

func createExtension(tx *sql.Tx, extension types.IExtension, position int) (sql.NullInt64, error) {
	var semanticID sql.NullInt64
	if extension.SemanticID() != nil {
		id, err := createReference(tx, extension.SemanticID(), sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			return sql.NullInt64{}, err
		}
		semanticID = id
	}

	var valueType any
	if extension.ValueType() != nil {
		valueType = extension.ValueType()
	} else {
		valueType = sql.NullString{String: "", Valid: false}
	}

	var typed typedValue
	if extension.ValueType() != nil {
		typed = mapValueByType(*extension.ValueType(), extension.Value())
	} else {
		value := ""
		if extension.Value() != nil {
			value = *extension.Value()
		}
		typed = typedValue{text: sql.NullString{String: value, Valid: value != ""}}
	}

	var extensionID sql.NullInt64
	if err := tx.QueryRow(`
		INSERT INTO extension (name, position, value_type, value_text, value_num, value_bool, value_time, value_date, value_datetime, semantic_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`, extension.Name(), position, valueType, typed.text, typed.numeric, typed.boolean, typed.time, typed.date, typed.dateTime, semanticID).Scan(&extensionID); err != nil {
		return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CREXT-INS " + err.Error())
	}

	for _, supplemental := range extension.SupplementalSemanticIDs() {
		refID, err := createReference(tx, supplemental, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			return sql.NullInt64{}, err
		}
		_, err = tx.Exec(`INSERT INTO extension_supplemental_semantic_id (extension_id, reference_id) VALUES ($1, $2)`, extensionID, refID)
		if err != nil {
			return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CREXT-SUPSEM " + err.Error())
		}
	}

	for _, referred := range extension.RefersTo() {
		refID, err := createReference(tx, referred, sql.NullInt64{}, sql.NullInt64{})
		if err != nil {
			return sql.NullInt64{}, err
		}
		_, err = tx.Exec(`INSERT INTO extension_refers_to (extension_id, reference_id) VALUES ($1, $2)`, extensionID, refID)
		if err != nil {
			return sql.NullInt64{}, common.NewInternalServerError("CDREPO-CREXT-REF " + err.Error())
		}
	}

	return extensionID, nil
}

func insertConceptDescriptionExtensions(tx *sql.Tx, cdID string, extensions []types.IExtension) error {
	for i, ext := range extensions {
		id, err := createExtension(tx, ext, i)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO concept_description_extension (concept_description_id, position, extension_id) VALUES ($1, $2, $3)`, cdID, i, id)
		if err != nil {
			return common.NewInternalServerError("CDREPO-CRCD-EXT " + err.Error())
		}
	}
	return nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
