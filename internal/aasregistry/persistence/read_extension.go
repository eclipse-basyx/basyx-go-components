package aasregistrydatabase

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func readExtensionsByDescriptorID(
    ctx context.Context,
    db Queryer,
    descriptorID int64,
) ([]model.Extension, error) {
	v, err := readExtensionsByDescriptorIDs(ctx, db, []int64{descriptorID})
	return v[descriptorID], err
}

func readExtensionsByDescriptorIDs(
    ctx context.Context,
    db Queryer,
    descriptorIDs []int64,
) (map[int64][]model.Extension, error) {
	start := time.Now()
	out := make(map[int64][]model.Extension, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}
	uniqDesc := descriptorIDs

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
	defer func() {
		_ = rows.Close()
	}()

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

	uniqExtIDs := allExtIDs
	uniqSemRefIDs := semRefIDs

    suppByExt, err := readEntityReferences1ToMany(
        ctx, db, uniqExtIDs,
        "extension_supplemental_semantic_id", "extension_id", "reference_id",
    )
	if err != nil {
		return nil, err
	}

    refersByExt, err := readEntityReferences1ToMany(
        ctx, db, uniqExtIDs,
        "extension_refers_to", "extension_id", "reference_id",
    )
	if err != nil {
		return nil, err
	}

    semRefByID := make(map[int64]*model.Reference)
    if len(uniqSemRefIDs) > 0 {
        var err error
        semRefByID, err = GetReferencesByIDsBatch(db, uniqSemRefIDs)
        if err != nil {
            return nil, fmt.Errorf("GetReferencesByIdsBatch (semantic refs): %w", err)
        }
    }

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

			suppRefs := suppByExt[r.extID]
			referRefs := refersByExt[r.extID]
			out[descID] = append(out[descID], model.Extension{
				SemanticID:              semanticRef,
				Name:                    r.name.String,
				ValueType:               vType,
				Value:                   val,
				SupplementalSemanticIds: suppRefs,
				RefersTo:                referRefs,
			})
		}
	}

	for _, id := range uniqDesc {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	duration := time.Since(start)
	fmt.Printf("extension block took %v to complete\n", duration)
	return out, nil
}
