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
// Author: Martin Stemmer ( Fraunhofer IESE )

package descriptors

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/lib/pq"
)

// ReadExtensionsByDescriptorID returns all extensions that belong to a single
// descriptor identified by the given descriptorID.
//
// It is a convenience wrapper around ReadExtensionsByDescriptorIDs and simply
// returns the slice mapped to the provided ID. If the descriptor exists but has
// no extensions, the returned slice is empty. If the descriptorID does not
// produce any rows, the returned slice is nil and no error is raised.
//
// The provided context is used for cancellation and deadline control of the
// underlying database call.
//
// Errors originate from ReadExtensionsByDescriptorIDs (SQL build/exec/scan
// failures or type conversion issues) and are returned verbatim.
func ReadExtensionsByDescriptorID(
	ctx context.Context,
	db *sql.DB,
	descriptorID int64,
) ([]model.Extension, error) {
	v, err := ReadExtensionsByDescriptorIDs(ctx, db, []int64{descriptorID})
	return v[descriptorID], err
}

// ReadExtensionsByDescriptorIDs retrieves extensions for the provided
// descriptorIDs in a single database round trip.
//
// Return value is a map keyed by descriptor ID, each value containing that
// descriptor's extensions. When descriptorIDs is empty, an empty map is
// returned without querying the database.
//
// Result semantics and ordering:
//   - Extensions are ordered by descriptor_id ASC, then extension id ASC.
//   - The extension Value is selected from one of the typed columns based on the
//     stored ValueType (xs:string/URI->text; numeric types->num; xs:boolean->bool;
//     xs:time->time; date/datetime/duration/g*->datetime). When no explicit
//     match exists, falls back to text if present.
//   - SemanticID may be nil when not set; supplemental semantic IDs and RefersTo
//     references are loaded via the respective link tables.
//
// Implementation notes:
//   - Uses pq.Array with SQL ANY for efficient multi-key filtering.
//   - Performs a single join to fetch base extension rows, then batches lookups
//     for references to minimize round trips.
//   - Converts ValueType strings to model.DataTypeDefXsd via
//     model.NewDataTypeDefXsdFromValue; invalid values propagate an error.
//
// Errors may occur while building the SQL statement, executing the query,
// scanning columns, or converting types.
func ReadExtensionsByDescriptorIDs(
	ctx context.Context,
	db DBQueryer,
	descriptorIDs []int64,
) (map[int64][]model.Extension, error) {
	out := make(map[int64][]model.Extension, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}

	d := goqu.Dialect(dialect)
	de := goqu.T(tblDescriptorExtension).As("de")
	e := goqu.T(tblExtension).As("e")

	// Pull all extensions for all descriptors in one go
	arr := pq.Array(descriptorIDs)
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
		Where(goqu.L("de.descriptor_id = ANY(?::bigint[])", arr)).
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

	perDesc := make(map[int64][]row, len(descriptorIDs))
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
		return out, nil
	}

	uniqExtIDs := allExtIDs
	uniqSemRefIDs := semRefIDs

	suppByExt, err := readEntityReferences1ToMany(
		ctx, db, uniqExtIDs,
		tblExtensionSuppSemantic, colExtensionID, colReferenceID,
	)
	if err != nil {
		return nil, err
	}

	refersByExt, err := readEntityReferences1ToMany(
		ctx, db, uniqExtIDs,
		tblExtensionRefersTo, colExtensionID, colReferenceID,
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

	return out, nil
}
