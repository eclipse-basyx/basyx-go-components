package persistence_postgresql

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func readExtensionsByDescriptorID(
	ctx context.Context,
	db *sql.DB,
	descriptorID int64,
) ([]model.Extension, error) {
	d := goqu.Dialect(dialect)

	sqlStr, args, err := d.
		From(goqu.T(tblDescriptorExtension).As("de")).
		InnerJoin(
			goqu.T(tblExtension).As("e"),
			goqu.On(goqu.I("de."+colExtensionID).Eq(goqu.I("e."+colID))),
		).
		Select(
			goqu.I("e."+colID),
			goqu.I("e."+colSemanticID),
			goqu.I("e."+colName),
			goqu.I("e."+colValueType),
			goqu.I("e."+colValueText),
			goqu.I("e."+colValueNum),
			goqu.I("e."+colValueBool),
			goqu.I("e."+colValueTime),
			goqu.I("e."+colValueDatetime),
		).
		Where(goqu.I("de." + colDescriptorID).Eq(descriptorID)).
		Order(goqu.I("e." + colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	type row struct {
		id            int64
		semanticRefID sql.NullInt64
		name          sql.NullString
		valueType     sql.NullString
		valueText     sql.NullString
		valueNum      sql.NullString
		valueBool     sql.NullString
		valueTime     sql.NullString
		valueDatetime sql.NullString
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Extension

	for rows.Next() {
		var r row
		if err := rows.Scan(
			&r.id,
			&r.semanticRefID,
			&r.name,
			&r.valueType,
			&r.valueText,
			&r.valueNum,
			&r.valueBool,
			&r.valueTime,
			&r.valueDatetime,
		); err != nil {
			return nil, err
		}

		var semanticRef *model.Reference
		if r.semanticRefID.Valid {
			ref, err := persistence_utils.GetReferenceByReferenceDBID(db, r.semanticRefID)
			if err != nil {
				return nil, err
			}
			semanticRef = ref
		}

		val := ""
		switch r.valueType.String {
		case "xs:string", "xs:anyURI", "xs:base64Binary", "xs:hexBinary":
			val = r.valueText.String
		case "xs:int", "xs:integer", "xs:long", "xs:short", "xs:byte",
			"xs:unsignedInt", "xs:unsignedLong", "xs:unsignedShort", "xs:unsignedByte",
			"xs:positiveInteger", "xs:negativeInteger", "xs:nonNegativeInteger", "xs:nonPositiveInteger",
			"xs:decimal", "xs:double", "xs:float":
			val = r.valueNum.String
		case "xs:boolean":
			val = r.valueBool.String
		case "xs:time":
			val = r.valueTime.String
		case "xs:date", "xs:dateTime", "xs:duration", "xs:gDay", "xs:gMonth",
			"xs:gMonthDay", "xs:gYear", "xs:gYearMonth":
			val = r.valueDatetime.String
		default:
			if r.valueText.Valid {
				val = r.valueText.String
			}
		}

		refs, err := readExtensionReferences(db, r.id)
		if err != nil {
			return nil, err
		}

		var valueType model.DataTypeDefXsd
		valueType, err = model.NewDataTypeDefXsdFromValue(r.valueType.String)
		if err != nil {
			return nil, err
		}
		out = append(out, model.Extension{
			SemanticId:              semanticRef,
			Name:                    r.name.String,
			ValueType:               valueType,
			Value:                   val,
			SupplementalSemanticIds: refs,
			RefersTo:                refs,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func readExtensionReferences(db *sql.DB, extensionID int64) ([]model.Reference, error) {
	d := goqu.Dialect(dialect)

	sqlStr, args, err := d.
		From(goqu.T(tblExtensionReference).As("er")).
		Select(goqu.I("er." + colReferenceID)).
		Where(goqu.I("er." + colExtensionID).Eq(extensionID)).
		Order(goqu.I("er." + colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Reference
	for rows.Next() {
		var refID sql.NullInt64
		if err := rows.Scan(&refID); err != nil {
			return nil, err
		}
		if refID.Valid {
			ref, err := persistence_utils.GetReferenceByReferenceDBID(db, refID)
			if err != nil {
				return nil, err
			}
			if ref != nil {
				out = append(out, *ref)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
