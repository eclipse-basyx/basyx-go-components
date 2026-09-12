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

func TestMapValueByTypeStoresInvalidDatabaseTypedValuesAsText(t *testing.T) {
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
			mapped := MapValueByType(testCase.valueType, &testCase.value)

			if !mapped.Text.Valid || mapped.Text.String != testCase.value {
				t.Fatalf("expected invalid %s value %q to use text fallback, got %#v", testCase.name, testCase.value, mapped)
			}
			if mapped.Numeric.Valid || mapped.Boolean.Valid || mapped.Time.Valid || mapped.Date.Valid || mapped.DateTime.Valid {
				t.Fatalf("expected only text fallback to be populated, got %#v", mapped)
			}
		})
	}
}

func TestMapValueByTypeKeepsValidDatabaseTypedValuesTyped(t *testing.T) {
	booleanValue := "true"
	if mapped := MapValueByType(types.DataTypeDefXSDBoolean, &booleanValue); !mapped.Boolean.Valid || mapped.Text.Valid {
		t.Fatalf("expected valid boolean to use boolean column, got %#v", mapped)
	}

	timeValue := "13:14:15"
	if mapped := MapValueByType(types.DataTypeDefXSDTime, &timeValue); !mapped.Time.Valid || mapped.Text.Valid {
		t.Fatalf("expected valid time to use time column, got %#v", mapped)
	}

	dateValue := "2024-04-22"
	if mapped := MapValueByType(types.DataTypeDefXSDDate, &dateValue); !mapped.Date.Valid || mapped.Text.Valid {
		t.Fatalf("expected valid date to use date column, got %#v", mapped)
	}

	dateTimeValue := "2024-04-22T00:00:00Z"
	if mapped := MapValueByType(types.DataTypeDefXSDDateTime, &dateTimeValue); !mapped.DateTime.Valid || mapped.Text.Valid {
		t.Fatalf("expected valid dateTime to use dateTime column, got %#v", mapped)
	}
}

func TestMapRangeValueByTypeStoresInvalidTemporalBoundsAsText(t *testing.T) {
	mapped := MapRangeValueByType(types.DataTypeDefXSDDateTime, "22.04.2024", "23.04.2024")

	if !mapped.MinText.Valid || mapped.MinText.String != "22.04.2024" {
		t.Fatalf("expected invalid minimum to use text fallback, got %#v", mapped)
	}
	if !mapped.MaxText.Valid || mapped.MaxText.String != "23.04.2024" {
		t.Fatalf("expected invalid maximum to use text fallback, got %#v", mapped)
	}
	if mapped.MinDateTime.Valid || mapped.MaxDateTime.Valid {
		t.Fatalf("expected dateTime columns to remain empty, got %#v", mapped)
	}
}
