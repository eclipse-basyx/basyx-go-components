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

package model

import (
	"encoding/json"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestValueOnlyScalarToJSONUsesNativeTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		lexical   string
		valueType types.DataTypeDefXSD
		expected  string
	}{
		{"int", "42", types.DataTypeDefXSDInt, `42`},
		{"float", "42.0", types.DataTypeDefXSDFloat, `42.0`},
		{"double exponent", "1.5E3", types.DataTypeDefXSDDouble, `1.5e3`},
		{"leading plus and zeros", "+007", types.DataTypeDefXSDInteger, `7`},
		{"fraction without integer", ".5", types.DataTypeDefXSDDecimal, `0.5`},
		{"unsigned long max", "18446744073709551615", types.DataTypeDefXSDUnsignedLong, `18446744073709551615`},
		{"decimal precision", "1234567890.1234567890123456789", types.DataTypeDefXSDDecimal, `1234567890.1234567890123456789`},
		{"boolean true", "true", types.DataTypeDefXSDBoolean, `true`},
		{"boolean one", "1", types.DataTypeDefXSDBoolean, `true`},
		{"boolean zero", "0", types.DataTypeDefXSDBoolean, `false`},
		{"numeric string", "42", types.DataTypeDefXSDString, `"42"`},
		{"date", "2000-01-01", types.DataTypeDefXSDDate, `"2000-01-01"`},
		{"infinity", "INF", types.DataTypeDefXSDDouble, `"INF"`},
		{"negative infinity", "-INF", types.DataTypeDefXSDFloat, `"-INF"`},
		{"not a number", "NaN", types.DataTypeDefXSDDouble, `"NaN"`},
		{"invalid legacy integer", "abc", types.DataTypeDefXSDInt, `"abc"`},
		{"invalid legacy boolean", "yes", types.DataTypeDefXSDBoolean, `"yes"`},
		{"fraction in integer", "1.5", types.DataTypeDefXSDInt, `"1.5"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			lexical := test.lexical
			actual, err := json.Marshal(ValueOnlyScalarToJSON(&lexical, test.valueType))
			require.NoError(t, err)
			require.Equal(t, test.expected, string(actual))
		})
	}
}

func TestValueOnlyScalarToJSONHandlesAbsentAndUnknownTypes(t *testing.T) {
	t.Parallel()

	require.Nil(t, ValueOnlyScalarToJSON(nil, types.DataTypeDefXSDInt))
	lexical := "42"
	require.Equal(t, "42", ValueOnlyScalarToJSON(&lexical, types.DataTypeDefXSD(999)))
	_, err := ValueOnlyScalarToJSONStrict(&lexical, types.DataTypeDefXSD(999))
	require.ErrorIs(t, err, ErrValueOnlyScalarValueType)
}

func TestValueOnlyScalarToJSONStrictRejectsInconsistentLexicals(t *testing.T) {
	t.Parallel()

	lexical := "abc"
	_, err := ValueOnlyScalarToJSONStrict(&lexical, types.DataTypeDefXSDInt)
	require.ErrorIs(t, err, ErrValueOnlyScalarInconsistent)
	infinity := "INF"
	value, err := ValueOnlyScalarToJSONStrict(&infinity, types.DataTypeDefXSDDouble)
	require.NoError(t, err)
	require.Equal(t, "INF", value)
}

func TestValueOnlyScalarFromJSONAcceptsNativeTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		payload   string
		valueType types.DataTypeDefXSD
		expected  *string
	}{
		{"int", `42`, types.DataTypeDefXSDInt, stringPointer("42")},
		{"int exponent", `3e2`, types.DataTypeDefXSDInt, stringPointer("300")},
		{"int integral fraction", `3.0`, types.DataTypeDefXSDInt, stringPointer("3")},
		{"int fractional exponent", `0.5e1`, types.DataTypeDefXSDInt, stringPointer("5")},
		{"negative zero", `-0`, types.DataTypeDefXSDInt, stringPointer("-0")},
		{"small decimal", `5e-3`, types.DataTypeDefXSDDecimal, stringPointer("0.005")},
		{"unsigned long max", `18446744073709551615`, types.DataTypeDefXSDUnsignedLong, stringPointer("18446744073709551615")},
		{"decimal precision", `1234567890.1234567890123456789`, types.DataTypeDefXSDDecimal, stringPointer("1234567890.1234567890123456789")},
		{"double literal", `1.5e3`, types.DataTypeDefXSDDouble, stringPointer("1.5e3")},
		{"float", `42.0`, types.DataTypeDefXSDFloat, stringPointer("42.0")},
		{"double infinity", `"INF"`, types.DataTypeDefXSDDouble, stringPointer("INF")},
		{"float negative infinity", `"-INF"`, types.DataTypeDefXSDFloat, stringPointer("-INF")},
		{"double not a number", `"NaN"`, types.DataTypeDefXSDDouble, stringPointer("NaN")},
		{"boolean", `false`, types.DataTypeDefXSDBoolean, stringPointer("false")},
		{"string", `"42"`, types.DataTypeDefXSDString, stringPointer("42")},
		{"empty string", `""`, types.DataTypeDefXSDString, stringPointer("")},
		{"dateTime", `"2000-01-01T14:23:00Z"`, types.DataTypeDefXSDDateTime, stringPointer("2000-01-01T14:23:00Z")},
		{"null clears", `null`, types.DataTypeDefXSDInt, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			lexical, err := ValueOnlyScalarFromJSON(decodeTestJSON(t, test.payload), test.valueType)
			require.NoError(t, err)
			require.Equal(t, test.expected, lexical)
		})
	}
}

func TestValueOnlyScalarFromJSONRejectsWrongKindsAndValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		payload   string
		valueType types.DataTypeDefXSD
		expected  error
	}{
		{"numeric string for int", `"7"`, types.DataTypeDefXSDInt, ErrValueOnlyScalarKind},
		{"boolean for int", `true`, types.DataTypeDefXSDInt, ErrValueOnlyScalarKind},
		{"object for int", `{"value":1}`, types.DataTypeDefXSDInt, ErrValueOnlyScalarKind},
		{"array for string", `["a"]`, types.DataTypeDefXSDString, ErrValueOnlyScalarKind},
		{"number for string", `42`, types.DataTypeDefXSDString, ErrValueOnlyScalarKind},
		{"string for boolean", `"true"`, types.DataTypeDefXSDBoolean, ErrValueOnlyScalarKind},
		{"number for boolean", `1`, types.DataTypeDefXSDBoolean, ErrValueOnlyScalarKind},
		{"infinity for decimal", `"INF"`, types.DataTypeDefXSDDecimal, ErrValueOnlyScalarKind},
		{"lowercase infinity for double", `"inf"`, types.DataTypeDefXSDDouble, ErrValueOnlyScalarKind},
		{"number string for double", `"1.5"`, types.DataTypeDefXSDDouble, ErrValueOnlyScalarKind},
		{"fraction for int", `1.5`, types.DataTypeDefXSDInt, ErrValueOnlyScalarInconsistent},
		{"int overflow", `2147483648`, types.DataTypeDefXSDInt, ErrValueOnlyScalarInconsistent},
		{"negative unsigned", `-1`, types.DataTypeDefXSDUnsignedInt, ErrValueOnlyScalarInconsistent},
		{"exponent overflow", `1e10001`, types.DataTypeDefXSDInteger, ErrValueOnlyScalarInconsistent},
		{"invalid date", `"2000-13-01"`, types.DataTypeDefXSDDate, ErrValueOnlyScalarInconsistent},
		{"unknown valueType", `"x"`, types.DataTypeDefXSD(999), ErrValueOnlyScalarValueType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := ValueOnlyScalarFromJSON(decodeTestJSON(t, test.payload), test.valueType)
			require.ErrorIs(t, err, test.expected)
		})
	}
}

func TestIsNumericXSDTypeCoversAllNumericTypes(t *testing.T) {
	t.Parallel()

	numeric := 0
	for valueType := types.DataTypeDefXSDAnyURI; valueType <= types.DataTypeDefXSDUnsignedShort; valueType++ {
		if IsNumericXSDType(valueType) {
			numeric++
		}
	}
	require.Equal(t, 16, numeric)
	require.False(t, IsNumericXSDType(types.DataTypeDefXSDBoolean))
	require.False(t, IsNumericXSDType(types.DataTypeDefXSDString))
}

func decodeTestJSON(t *testing.T, payload string) any {
	t.Helper()
	value, err := decodeValueOnlyJSON(json.RawMessage(payload))
	require.NoError(t, err)
	return value
}

func stringPointer(value string) *string {
	return &value
}
