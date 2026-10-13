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

package submodelelements

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestBuildValueOnlySchemaQueryLoadsSubtreeWithValueTypes(t *testing.T) {
	t.Parallel()

	query, args, err := buildValueOnlySchemaQuery(7, []string{"A", "B"}, true)
	require.NoError(t, err)
	require.Contains(t, query, "WITH RECURSIVE value_only_schema")
	require.Contains(t, query, `"root"."parent_sme_id" IS NULL`)
	require.Contains(t, query, `COALESCE("pe"."value_type", "re"."value_type"`)
	require.Contains(t, args, "A")
	require.Contains(t, args, "B")

	query, _, err = buildValueOnlySchemaQuery(7, []string{"A.B"}, false)
	require.NoError(t, err)
	require.NotContains(t, query, `"root"."parent_sme_id" IS NULL`)
}

func TestLinkValueOnlySchemaRowsBuildsTreeInPositionOrder(t *testing.T) {
	t.Parallel()

	row := func(id int64, parentID int64, idShort string, modelType types.ModelType, path string) valueOnlySchemaRow {
		return valueOnlySchemaRow{
			node:     &gen.ValueOnlySchemaNode{ElementID: id, IDShortPath: path, ModelType: modelType},
			parentID: sql.NullInt64{Int64: parentID, Valid: parentID != 0},
			idShort:  sql.NullString{String: idShort, Valid: idShort != ""},
		}
	}
	roots := linkValueOnlySchemaRows([]valueOnlySchemaRow{
		row(1, 0, "Data", types.ModelTypeSubmodelElementCollection, "Data"),
		row(2, 1, "Rows", types.ModelTypeSubmodelElementList, "Data.Rows"),
		row(3, 2, "", types.ModelTypeProperty, "Data.Rows[0]"),
		row(4, 2, "", types.ModelTypeProperty, "Data.Rows[1]"),
		row(5, 1, "Name", types.ModelTypeProperty, "Data.Name"),
	})

	require.Len(t, roots, 1)
	data := roots["Data"]
	require.Equal(t, int64(5), data.Children["Name"].ElementID)
	rows := data.Children["Rows"]
	require.Len(t, rows.Items, 2)
	require.Equal(t, "Data.Rows[0]", rows.Items[0].IDShortPath)
	require.Equal(t, "Data.Rows[1]", rows.Items[1].IDShortPath)
}

func TestApplyValueOnlyUpdatesWritesByElementID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "property_element" SET .* WHERE \("id" = \$7\)`).
		WithArgs(nil, nil, nil, "42", nil, nil, int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "range_element" SET "min_date"=\$1,"min_datetime"=\$2,"min_num"=\$3,"min_text"=\$4,"min_time"=\$5 WHERE \("id" = \$6\)`).
		WithArgs(nil, nil, nil, "INF", nil, int64(12)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "blob_element" SET "value"=\$1 WHERE \("id" = \$2\)`).
		WithArgs(common.Encode([]byte{0x00, 0xff}), int64(13)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "multilanguage_property_value" WHERE \("submodel_element_id" = \$1\)`).
		WithArgs(int64(14)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`INSERT INTO "multilanguage_property_value"`).
		WithArgs("en", int64(14), "Name", "de", int64(14), "Bezeichnung").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`UPDATE "entity_element" SET "entity_type"=\$1 WHERE \("id" = \$2\)`).
		WithArgs(int64(types.EntityTypeCoManagedEntity), int64(15)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	blob := []byte{0x00, 0xff}
	entityType := types.EntityTypeCoManagedEntity
	err = ApplyValueOnlyUpdates(tx, db, []gen.ValueOnlyUpdate{
		{ElementID: 11, ModelType: types.ModelTypeProperty, Value: gen.PropertyValue{Value: stringPointer("42"), ValueType: types.DataTypeDefXSDInt}},
		{ElementID: 12, ModelType: types.ModelTypeRange, Value: gen.RangeValuePatch{ValueType: types.DataTypeDefXSDDouble, Min: gen.ScalarPatch{Set: true, Value: stringPointer("INF")}}},
		{ElementID: 13, ModelType: types.ModelTypeBlob, Value: gen.BlobValuePatch{Value: &blob}},
		{ElementID: 14, ModelType: types.ModelTypeMultiLanguageProperty, Value: gen.MultiLanguagePropertyValue{{"en": "Name"}, {"de": "Bezeichnung"}}},
		{ElementID: 15, ModelType: types.ModelTypeEntity, Value: gen.EntityValuePatch{EntityType: &entityType}},
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyValueOnlyUpdatesSkipsEmptyPatches(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	mock.ExpectCommit()
	tx, err := db.Begin()
	require.NoError(t, err)
	err = ApplyValueOnlyUpdates(tx, db, []gen.ValueOnlyUpdate{
		{ElementID: 1, ModelType: types.ModelTypeEntity, Value: gen.EntityValuePatch{}},
		{ElementID: 2, ModelType: types.ModelTypeRelationshipElement, Value: gen.RelationshipValuePatch{ModelType: types.ModelTypeRelationshipElement}},
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyValueOnlyUpdatesRejectsElementsWithoutValueOnlyRepresentation(t *testing.T) {
	t.Parallel()

	err := ApplyValueOnlyUpdates(nil, nil, []gen.ValueOnlyUpdate{{ModelType: types.ModelTypeOperation}})
	require.True(t, common.IsErrBadRequest(err))
}

func TestBuildUpdateBlobRecordObjectStoresInsertEncoding(t *testing.T) {
	t.Parallel()

	blob := types.NewBlob()
	blob.SetValue([]byte{0x00, 0xff, 0xfe})
	record := buildUpdateBlobRecordObject(false, blob)
	require.Equal(t, common.Encode([]byte{0x00, 0xff, 0xfe}), record["value"])
}

func TestMapValueByTypeKeepsNonFiniteLexicalsAsText(t *testing.T) {
	t.Parallel()

	for _, lexical := range []string{"INF", "-INF", "NaN"} {
		value := lexical
		typedValue, err := MapValueByType(types.DataTypeDefXSDDouble, &value)
		require.NoError(t, err)
		require.Equal(t, sql.NullString{String: lexical, Valid: true}, typedValue.Text)
		require.False(t, typedValue.Numeric.Valid)
	}
}

func stringPointer(value string) *string {
	return &value
}

func TestMapRangeValueByTypeKeepsEmptyTextBounds(t *testing.T) {
	t.Parallel()

	empty := ""
	typedValue, err := MapRangeValueByType(types.DataTypeDefXSDString, &empty, nil)
	require.NoError(t, err)
	require.Equal(t, sql.NullString{Valid: true}, typedValue.MinText)
	require.False(t, typedValue.MaxText.Valid)

	typedValue, err = MapRangeValueByType(types.DataTypeDefXSDInt, &empty, nil)
	require.NoError(t, err)
	require.False(t, typedValue.MinText.Valid)
	require.False(t, typedValue.MinNumeric.Valid)
}

func TestApplyValueOnlyUpdatesStoresEmptyTextRangeBound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "range_element" SET "min_date"=\$1,"min_datetime"=\$2,"min_num"=\$3,"min_text"=\$4,"min_time"=\$5 WHERE \("id" = \$6\)`).
		WithArgs(nil, nil, nil, "", nil, int64(21)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = ApplyValueOnlyUpdates(tx, db, []gen.ValueOnlyUpdate{{
		ElementID: 21,
		ModelType: types.ModelTypeRange,
		Value:     gen.RangeValuePatch{ValueType: types.DataTypeDefXSDString, Min: gen.ScalarPatch{Set: true, Value: stringPointer("")}},
	}})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}
