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
	"strconv"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
)

func TestMapValueByTypeRejectsInvalidDatabaseTypedValues(t *testing.T) {
	testCases := []struct {
		name      string
		valueType types.DataTypeDefXSD
		value     string
	}{
		{name: "boolean", valueType: types.DataTypeDefXSDBoolean, value: "not-a-boolean"},
		{name: "time", valueType: types.DataTypeDefXSDTime, value: "25:61:00"},
		{name: "date", valueType: types.DataTypeDefXSDDate, value: "22.04.2024"},
		{name: "dateTime", valueType: types.DataTypeDefXSDDateTime, value: "22.04.2024"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := MapValueByType(testCase.valueType, &testCase.value)
			if err == nil {
				t.Fatalf("expected invalid %s value %q to be rejected", testCase.name, testCase.value)
			}
		})
	}
}

func TestMapValueByTypeKeepsBooleanTyped(t *testing.T) {
	booleanValue := "true"
	if mapped, err := MapValueByType(types.DataTypeDefXSDBoolean, &booleanValue); err != nil || !mapped.Boolean.Valid || mapped.Text.Valid {
		t.Fatalf("expected valid boolean to use boolean column, got %#v, error %v", mapped, err)
	}
}

func TestMapTemporalValuesPreservesLexicalForms(t *testing.T) {
	for _, test := range []struct {
		valueType types.DataTypeDefXSD
		value     string
	}{
		{types.DataTypeDefXSDTime, "13:14:15.123456789+02:00"},
		{types.DataTypeDefXSDDate, "2024-04-22+02:00"},
		{types.DataTypeDefXSDDateTime, "2024-04-22T00:00:00"},
		{types.DataTypeDefXSDDateTime, "2024-04-22T00:00:00.123456789Z"},
		{types.DataTypeDefXSDDateTime, "2024-04-22T00:00:00.123456789+00:00"},
	} {
		t.Run(test.value, func(t *testing.T) {
			mapped, err := MapValueByType(test.valueType, &test.value)
			if err != nil || !mapped.Text.Valid || mapped.Text.String != test.value {
				t.Fatalf("expected original value in text column, got %#v, error %v", mapped, err)
			}
			bounds, err := MapRangeValueByType(test.valueType, &test.value, nil)
			if err != nil || !bounds.MinText.Valid || bounds.MinText.String != test.value || bounds.MaxText.Valid {
				t.Fatalf("expected original min and missing max, got %#v, error %v", bounds, err)
			}
		})
	}
}

func TestMapRangeValueByTypeRejectsInvalidTemporalBounds(t *testing.T) {
	for _, testCase := range []struct {
		name string
		min  string
		max  string
	}{
		{name: "invalid min", min: "22.04.2024", max: "2024-04-23T00:00:00Z"},
		{name: "invalid max", min: "2024-04-22T00:00:00Z", max: "23.04.2024"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := MapRangeValueByType(types.DataTypeDefXSDDateTime, &testCase.min, &testCase.max); err == nil {
				t.Fatal("expected invalid range bound to be rejected")
			}
		})
	}
}

func TestMapTemporalValuesAcceptsXSDEdgeLexicalForms(t *testing.T) {
	for _, test := range []struct {
		valueType types.DataTypeDefXSD
		value     string
	}{
		{types.DataTypeDefXSDTime, "24:00:00"},
		{types.DataTypeDefXSDDate, "-0001-01-01"},
		{types.DataTypeDefXSDDateTime, "-0001-01-01T00:00:00Z"},
		{types.DataTypeDefXSDDateTime, "2024-01-01T08:00:00.1234567891-14:00"},
	} {
		t.Run(test.value, func(t *testing.T) {
			mapped, err := MapValueByType(test.valueType, &test.value)
			if err != nil || mapped.Text.String != test.value {
				t.Fatalf("expected XSD-valid value to be stored verbatim, got %#v, error %v", mapped, err)
			}
		})
	}
}

func TestTemporalRecordsClearLegacyTypedColumns(t *testing.T) {
	value := "2024-01-01T10:00:00+02:00"
	property := types.NewProperty(types.DataTypeDefXSDDateTime)
	property.SetValue(&value)
	rangeElement := types.NewRange(types.DataTypeDefXSDDateTime)
	rangeElement.SetMin(&value)
	rangeElement.SetMax(&value)

	propertyInsert, err := PostgreSQLPropertyHandler{}.GetInsertQueryPart(nil, 1, property)
	requireNoTestError(t, err)
	rangeInsert, err := PostgreSQLRangeHandler{}.GetInsertQueryPart(nil, 1, rangeElement)
	requireNoTestError(t, err)
	records := map[string]goqu.Record{
		"property insert": propertyInsert.Record,
		"range insert":    rangeInsert.Record,
	}
	for _, isPut := range []bool{true, false} {
		propertyUpdate, updateErr := buildUpdatePropertyRecordObject(property, isPut, nil)
		requireNoTestError(t, updateErr)
		rangeUpdate, updateErr := buildUpdateRangeRecordObject(rangeElement, isPut)
		requireNoTestError(t, updateErr)
		records["property update put="+strconv.FormatBool(isPut)] = propertyUpdate
		records["range update put="+strconv.FormatBool(isPut)] = rangeUpdate
	}

	for name, record := range records {
		t.Run(name, func(t *testing.T) {
			for _, prefix := range temporalRecordPrefixes(record) {
				requireTemporalBoundStoredAsText(t, record, prefix, value)
			}
		})
	}
}

func TestRangePutWithSingleBoundClearsMissingBound(t *testing.T) {
	minValue := "2024-01-01T10:00:00Z"
	rangeElement := types.NewRange(types.DataTypeDefXSDDateTime)
	rangeElement.SetMin(&minValue)

	record, err := buildUpdateRangeRecordObject(rangeElement, true)
	requireNoTestError(t, err)
	requireTemporalBoundStoredAsText(t, record, "min", minValue)
	for _, column := range []string{"max_text", "max_num"} {
		if stored, ok := record[column].(sql.NullString); !ok || stored.Valid {
			t.Fatalf("expected %s to be cleared, got %#v", column, record[column])
		}
	}
}

func temporalRecordPrefixes(record goqu.Record) []string {
	if _, ok := record["value_text"]; ok {
		return []string{"value"}
	}
	return []string{"min", "max"}
}

func requireTemporalBoundStoredAsText(t *testing.T, record goqu.Record, prefix, value string) {
	t.Helper()
	if stored, ok := record[prefix+"_text"].(sql.NullString); !ok || !stored.Valid || stored.String != value {
		t.Fatalf("expected %s_text to hold %q, got %#v", prefix, value, record[prefix+"_text"])
	}
	for _, suffix := range []string{"_time", "_date", "_datetime"} {
		column, ok := record[prefix+suffix]
		if !ok || column != nil {
			t.Fatalf("expected %s%s to be cleared, got %#v (present %t)", prefix, suffix, column, ok)
		}
	}
}

func requireNoTestError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
