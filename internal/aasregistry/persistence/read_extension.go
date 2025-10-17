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

	de := goqu.T(tblDescriptorExtension).As("de")
	e := goqu.T(tblExtension).As("e")

	sqlStr, args, err := d.
		From(de).
		InnerJoin(
			e,
			goqu.On(de.Col(colExtensionID).Eq(e.Col(colID))),
		).
		Select(
			e.Col(colID),
			e.Col(colSemanticID),
			e.Col(colName),
			e.Col(colValueType),
			e.Col(colValueText),
			e.Col(colValueNum),
			e.Col(colValueBool),
			e.Col(colValueTime),
			e.Col(colValueDatetime),
		).
		Where(de.Col(colDescriptorID).Eq(descriptorID)).
		Order(e.Col(colID).Asc()).
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

		valueType, err := model.NewDataTypeDefXsdFromValue(r.valueType.String)
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

	er := goqu.T(tblExtensionReference).As("er")

	sqlStr, args, err := d.
		From(er).
		Select(er.Col(colReferenceID)).
		Where(er.Col(colExtensionID).Eq(extensionID)).
		Order(er.Col(colID).Asc()).
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

// Bulk: descriptorIDs -> []Extension
func readExtensionsByDescriptorIDs(
	ctx context.Context,
	db *sql.DB,
	descriptorIDs []int64,
) (map[int64][]model.Extension, error) {
	out := make(map[int64][]model.Extension, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}
	uniqDesc := dedupeInt64(descriptorIDs)

	d := goqu.Dialect(dialect)
	de := goqu.T(tblDescriptorExtension).As("de")
	e := goqu.T(tblExtension).As("e")

	// Pull all extensions for all descriptors in one go
	sqlStr, args, err := d.
		From(de).
		InnerJoin(e, goqu.On(de.Col(colExtensionID).Eq(e.Col(colID)))).
		Select(
			de.Col(colDescriptorID), // 0
			e.Col(colID),            // 1
			e.Col(colSemanticID),    // 2
			e.Col(colName),          // 3
			e.Col(colValueType),     // 4
			e.Col(colValueText),     // 5
			e.Col(colValueNum),      // 6
			e.Col(colValueBool),     // 7
			e.Col(colValueTime),     // 8
			e.Col(colValueDatetime), // 9
		).
		Where(de.Col(colDescriptorID).In(uniqDesc)).
		Order(de.Col(colDescriptorID).Asc(), e.Col(colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	type row struct {
		descID   int64
		extID    int64
		semRefID sql.NullInt64
		name     sql.NullString
		vType    sql.NullString
		vText    sql.NullString
		vNum     sql.NullString
		vBool    sql.NullString
		vTime    sql.NullString
		vDT      sql.NullString
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	perDesc := make(map[int64][]row, len(uniqDesc))
	allExtIDs := make([]int64, 0, 256)
	semRefIDs := make([]int64, 0, 128)

	for rows.Next() {
		var r row
		if err := rows.Scan(
			&r.descID,
			&r.extID,
			&r.semRefID,
			&r.name,
			&r.vType,
			&r.vText,
			&r.vNum,
			&r.vBool,
			&r.vTime,
			&r.vDT,
		); err != nil {
			return nil, err
		}
		perDesc[r.descID] = append(perDesc[r.descID], r)
		allExtIDs = append(allExtIDs, r.extID)
		if r.semRefID.Valid {
			semRefIDs = append(semRefIDs, r.semRefID.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(allExtIDs) == 0 {
		for _, id := range uniqDesc {
			if _, ok := out[id]; !ok {
				out[id] = nil
			}
		}
		return out, nil
	}

	uniqExtIDs := dedupeInt64(allExtIDs)
	uniqSemRefIDs := dedupeInt64(semRefIDs)

	// Bulk: extensionID -> []Reference (for both SupplementalSemanticIds & RefersTo)
	refsByExt, err := readExtensionReferencesByExtensionIDs(db, uniqExtIDs)
	if err != nil {
		return nil, err
	}

	// Bulk: referenceID -> *Reference (for SemanticId)
	semRefByID := make(map[int64]*model.Reference)
	if len(uniqSemRefIDs) > 0 {
		//todo: references
	}

	// Assemble
	for descID, rowsForDesc := range perDesc {
		for _, r := range rowsForDesc {
			var semanticRef *model.Reference
			if r.semRefID.Valid {
				semanticRef = semRefByID[r.semRefID.Int64]
			}

			val := ""
			switch r.vType.String {
			case "xs:string", "xs:anyURI", "xs:base64Binary", "xs:hexBinary":
				val = r.vText.String
			case "xs:int", "xs:integer", "xs:long", "xs:short", "xs:byte",
				"xs:unsignedInt", "xs:unsignedLong", "xs:unsignedShort", "xs:unsignedByte",
				"xs:positiveInteger", "xs:negativeInteger", "xs:nonNegativeInteger", "xs:nonPositiveInteger",
				"xs:decimal", "xs:double", "xs:float":
				val = r.vNum.String
			case "xs:boolean":
				val = r.vBool.String
			case "xs:time":
				val = r.vTime.String
			case "xs:date", "xs:dateTime", "xs:duration", "xs:gDay", "xs:gMonth",
				"xs:gMonthDay", "xs:gYear", "xs:gYearMonth":
				val = r.vDT.String
			default:
				if r.vText.Valid {
					val = r.vText.String
				}
			}

			vType, err := model.NewDataTypeDefXsdFromValue(r.vType.String)
			if err != nil {
				return nil, err
			}

			refs := refsByExt[r.extID]
			out[descID] = append(out[descID], model.Extension{
				SemanticId:              semanticRef,
				Name:                    r.name.String,
				ValueType:               vType,
				Value:                   val,
				SupplementalSemanticIds: refs,
				RefersTo:                refs,
			})
		}
	}

	for _, id := range uniqDesc {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	return out, nil
}

// Bulk: extensionIDs -> []Reference
func readExtensionReferencesByExtensionIDs(
	db *sql.DB,
	extensionIDs []int64,
) (map[int64][]model.Reference, error) {
	out := make(map[int64][]model.Reference, len(extensionIDs))
	if len(extensionIDs) == 0 {
		return out, nil
	}
	uniqExt := dedupeInt64(extensionIDs)

	d := goqu.Dialect(dialect)
	er := goqu.T(tblExtensionReference).As("er")

	sqlStr, args, err := d.
		From(er).
		Select(
			er.Col(colExtensionID), // 0
			er.Col(colReferenceID), // 1
		).
		Where(er.Col(colExtensionID).In(uniqExt)).
		Order(er.Col(colExtensionID).Asc(), er.Col(colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type link struct {
		extID int64
		refID sql.NullInt64
	}
	links := make([]link, 0, 256)
	allRefIDs := make([]int64, 0, 256)

	for rows.Next() {
		var l link
		if err := rows.Scan(&l.extID, &l.refID); err != nil {
			return nil, err
		}
		links = append(links, l)
		if l.refID.Valid {
			allRefIDs = append(allRefIDs, l.refID.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	refByID := make(map[int64]*model.Reference)
	if len(allRefIDs) > 0 {

		//todo: references
	}

	for _, l := range links {
		if l.refID.Valid {
			if ref := refByID[l.refID.Int64]; ref != nil {
				out[l.extID] = append(out[l.extID], *ref)
			}
		}
	}

	for _, id := range uniqExt {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	return out, nil
}
