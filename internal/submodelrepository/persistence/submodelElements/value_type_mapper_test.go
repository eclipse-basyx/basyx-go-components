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
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
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

func TestMapValueByTypeKeepsValidDatabaseTypedValuesTyped(t *testing.T) {
	booleanValue := "true"
	if mapped, err := MapValueByType(types.DataTypeDefXSDBoolean, &booleanValue); err != nil || !mapped.Boolean.Valid || mapped.Text.Valid {
		t.Fatalf("expected valid boolean to use boolean column, got %#v, error %v", mapped, err)
	}

	timeValue := "13:14:15"
	if mapped, err := MapValueByType(types.DataTypeDefXSDTime, &timeValue); err != nil || !mapped.Time.Valid || mapped.Text.Valid {
		t.Fatalf("expected valid time to use time column, got %#v, error %v", mapped, err)
	}

	dateValue := "2024-04-22"
	if mapped, err := MapValueByType(types.DataTypeDefXSDDate, &dateValue); err != nil || !mapped.Date.Valid || mapped.Text.Valid {
		t.Fatalf("expected valid date to use date column, got %#v, error %v", mapped, err)
	}

	dateTimeValue := "2024-04-22T00:00:00Z"
	if mapped, err := MapValueByType(types.DataTypeDefXSDDateTime, &dateTimeValue); err != nil || !mapped.DateTime.Valid || mapped.Text.Valid {
		t.Fatalf("expected valid dateTime to use dateTime column, got %#v, error %v", mapped, err)
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
			if _, err := MapRangeValueByType(types.DataTypeDefXSDDateTime, testCase.min, testCase.max); err == nil {
				t.Fatal("expected invalid range bound to be rejected")
			}
		})
	}
}
