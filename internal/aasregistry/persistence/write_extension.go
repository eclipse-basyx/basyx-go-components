package persistence_postgresql

import (
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func createExtensions(tx *sql.Tx, descriptorId int64, extensions []model.Extension) error {
	if extensions == nil {
		return nil
	}
	if len(extensions) > 0 {
		d := goqu.Dialect(dialect)
		for _, val := range extensions {
			var a sql.NullInt64
			semanticId, err := persistence_utils.CreateReference(tx, val.SemanticID, a, a)
			if err != nil {
				return err
			}

			var valueText, valueNum, valueBool, valueTime, valueDatetime, valueType sql.NullString
			valueType = sql.NullString{String: string(val.ValueType), Valid: val.ValueType != ""}
			fillValueBasedOnType(val, &valueText, &valueNum, &valueBool, &valueTime, &valueDatetime)

			sqlStr, args, err := d.
				Insert(tblExtension).
				Rows(goqu.Record{
					colSemanticID:    semanticId,
					colName:          val.Name,
					colValueType:     valueType,
					colValueText:     valueText,
					colValueNum:      valueNum,
					colValueBool:     valueBool,
					colValueTime:     valueTime,
					colValueDatetime: valueDatetime,
				}).
				Returning(tExtension.Col(colID)).
				ToSQL()
			if err != nil {
				return err
			}
			var id int64
			if err = tx.QueryRow(sqlStr, args...).Scan(&id); err != nil {
				return err
			}

			if err = createDescriptorExtensionLink(tx, descriptorId, id); err != nil {
				return err
			}

			if err = createExtensionReferences(tx, id, val.SupplementalSemanticIds, "extension_supplemental_semantic_id"); err != nil {
				return err
			}

			if err = createExtensionReferences(tx, id, val.RefersTo, "extension_refers_to"); err != nil {
				return err
			}
		}
	}
	return nil
}

func createExtensionReferences(tx *sql.Tx, extensionId int64, references []model.Reference, tablename string) error {
	if len(references) == 0 {
		return nil
	}
	d := goqu.Dialect(dialect)
	rows := make([]goqu.Record, 0, len(references))
	for i := range references {
		var a sql.NullInt64
		referenceId, err := persistence_utils.CreateReference(tx, &references[i], a, a)
		if err != nil {
			return err
		}
		rows = append(rows, goqu.Record{
			colExtensionID: extensionId,
			colReferenceID: referenceId,
		})
	}
	sqlStr, args, err := d.Insert(tablename).Rows(rows).ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlStr, args...)
	return err
}

func createDescriptorExtensionLink(tx *sql.Tx, descriptorId int64, extensionId int64) error {
	d := goqu.Dialect(dialect)
	sqlStr, args, err := d.
		Insert(tblDescriptorExtension).
		Rows(goqu.Record{
			colDescriptorID: descriptorId,
			colExtensionID:  extensionId,
		}).
		ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlStr, args...)
	return err
}

func fillValueBasedOnType(extension model.Extension, valueText *sql.NullString, valueNum *sql.NullString, valueBool *sql.NullString, valueTime *sql.NullString, valueDatetime *sql.NullString) {
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
		*valueText = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	}
}
