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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
)

const maxValueOnlyDecimalExponent = 10000

var (
	// ErrValueOnlyScalarKind reports a JSON scalar whose JSON type does not match the ValueOnly mapping of its valueType.
	ErrValueOnlyScalarKind = errors.New("MODEL-VALONLY-SCALAR-KIND")
	// ErrValueOnlyScalarInconsistent reports a scalar whose lexical form is not consistent with its valueType.
	ErrValueOnlyScalarInconsistent = errors.New("MODEL-VALONLY-SCALAR-INCONSISTENT")
	// ErrValueOnlyScalarValueType reports an unknown valueType.
	ErrValueOnlyScalarValueType = errors.New("MODEL-VALONLY-SCALAR-VALUETYPE")
)

// IsNumericXSDType reports whether the ValueOnly serialization maps the valueType to a JSON number.
func IsNumericXSDType(valueType types.DataTypeDefXSD) bool {
	switch valueType {
	case types.DataTypeDefXSDByte, types.DataTypeDefXSDDecimal, types.DataTypeDefXSDDouble,
		types.DataTypeDefXSDFloat, types.DataTypeDefXSDInt, types.DataTypeDefXSDInteger,
		types.DataTypeDefXSDLong, types.DataTypeDefXSDNegativeInteger, types.DataTypeDefXSDNonNegativeInteger,
		types.DataTypeDefXSDNonPositiveInteger, types.DataTypeDefXSDPositiveInteger, types.DataTypeDefXSDShort,
		types.DataTypeDefXSDUnsignedByte, types.DataTypeDefXSDUnsignedInt, types.DataTypeDefXSDUnsignedLong,
		types.DataTypeDefXSDUnsignedShort:
		return true
	default:
		return false
	}
}

// ValueOnlyScalarToJSON renders a stored lexical value with the JSON type of the ValueOnly mapping.
// Values without a JSON-native representation, such as INF or legacy invalid lexicals, are returned as strings.
func ValueOnlyScalarToJSON(lexical *string, valueType types.DataTypeDefXSD) any {
	if lexical == nil {
		return nil
	}
	if valueType != types.DataTypeDefXSDBoolean && !IsNumericXSDType(valueType) {
		return *lexical
	}
	value, err := ValueOnlyScalarToJSONStrict(lexical, valueType)
	if err != nil {
		return *lexical
	}
	return value
}

// ValueOnlyScalarToJSONStrict renders a stored lexical value with the JSON type of the ValueOnly mapping
// and rejects lexicals that are not consistent with the valueType.
func ValueOnlyScalarToJSONStrict(lexical *string, valueType types.DataTypeDefXSD) (any, error) {
	if lexical == nil {
		return nil, nil
	}
	if err := ensureConsistentLexical(*lexical, valueType); err != nil {
		return nil, err
	}
	switch {
	case valueType == types.DataTypeDefXSDBoolean:
		return *lexical == "true" || *lexical == "1", nil
	case IsNumericXSDType(valueType):
		if number, ok := jsonNumberFromLexical(*lexical); ok {
			return number, nil
		}
		return *lexical, nil
	default:
		return *lexical, nil
	}
}

// ValueOnlyScalarFromJSON converts a JSON scalar decoded with UseNumber into the lexical form of the valueType.
// JSON null yields nil. Numeric types require a JSON number, xs:boolean a JSON boolean and all other types a JSON
// string; xs:double and xs:float additionally accept the strings INF, -INF and NaN.
func ValueOnlyScalarFromJSON(value any, valueType types.DataTypeDefXSD) (*string, error) {
	if value == nil {
		return nil, nil
	}
	lexical, err := valueOnlyLexical(value, valueType)
	if err != nil {
		return nil, err
	}
	if err = ensureConsistentLexical(lexical, valueType); err != nil {
		return nil, err
	}
	return &lexical, nil
}

func valueOnlyLexical(value any, valueType types.DataTypeDefXSD) (string, error) {
	if IsNumericXSDType(valueType) {
		return numericLexical(value, valueType)
	}
	if valueType == types.DataTypeDefXSDBoolean {
		if boolean, ok := value.(bool); ok {
			return strconv.FormatBool(boolean), nil
		}
		return "", scalarKindError(value, valueType)
	}
	if text, ok := value.(string); ok {
		return text, nil
	}
	return "", scalarKindError(value, valueType)
}

func numericLexical(value any, valueType types.DataTypeDefXSD) (string, error) {
	floating := valueType == types.DataTypeDefXSDDouble || valueType == types.DataTypeDefXSDFloat
	switch scalar := value.(type) {
	case json.Number:
		if floating {
			return scalar.String(), nil
		}
		return decimalLexicalFromJSONNumber(scalar.String())
	case string:
		if floating && isNonFiniteLexical(scalar) {
			return scalar, nil
		}
	}
	return "", scalarKindError(value, valueType)
}

func isNonFiniteLexical(lexical string) bool {
	return lexical == "INF" || lexical == "-INF" || lexical == "NaN"
}

func ensureConsistentLexical(lexical string, valueType types.DataTypeDefXSD) error {
	name, ok := stringification.DataTypeDefXSDToString(valueType)
	if !ok {
		return fmt.Errorf("%w: unknown valueType %d", ErrValueOnlyScalarValueType, valueType)
	}
	if !verification.ValueConsistentWithXSDType(lexical, valueType) {
		return fmt.Errorf("%w: value %q is not consistent with %s", ErrValueOnlyScalarInconsistent, lexical, name)
	}
	return nil
}

func scalarKindError(value any, valueType types.DataTypeDefXSD) error {
	name, _ := stringification.DataTypeDefXSDToString(valueType)
	return fmt.Errorf("%w: expected JSON %s for %s but got JSON %s", ErrValueOnlyScalarKind, expectedJSONKind(valueType), name, jsonKind(value))
}

func expectedJSONKind(valueType types.DataTypeDefXSD) string {
	switch {
	case IsNumericXSDType(valueType):
		return "number"
	case valueType == types.DataTypeDefXSDBoolean:
		return "boolean"
	default:
		return "string"
	}
}

func jsonKind(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case json.Number, float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	default:
		return "object"
	}
}

func jsonNumberFromLexical(lexical string) (json.Number, bool) {
	number := strings.TrimPrefix(lexical, "+")
	sign := ""
	if strings.HasPrefix(number, "-") {
		sign, number = "-", number[1:]
	}
	mantissa, exponent, hasExponent := strings.Cut(strings.ToLower(number), "e")
	integer, fraction, hasFraction := strings.Cut(mantissa, ".")
	integer = strings.TrimLeft(integer, "0")
	if integer == "" {
		integer = "0"
	}
	number = sign + integer
	if hasFraction && fraction != "" {
		number += "." + fraction
	}
	if hasExponent {
		number += "e" + exponent
	}
	if !json.Valid([]byte(number)) {
		return "", false
	}
	return json.Number(number), true
}

func decimalLexicalFromJSONNumber(number string) (string, error) {
	mantissa, exponent, _ := strings.Cut(strings.ToLower(number), "e")
	shift := 0
	if exponent != "" {
		parsed, err := strconv.Atoi(exponent)
		if err != nil || parsed < -maxValueOnlyDecimalExponent || parsed > maxValueOnlyDecimalExponent {
			return "", fmt.Errorf("%w: decimal exponent of %s exceeds the supported expansion", ErrValueOnlyScalarInconsistent, number)
		}
		shift = parsed
	}
	sign := ""
	if strings.HasPrefix(mantissa, "-") {
		sign, mantissa = "-", mantissa[1:]
	}
	integer, fraction, _ := strings.Cut(mantissa, ".")
	return expandDecimal(sign, integer, fraction, shift), nil
}

func expandDecimal(sign string, integer string, fraction string, shift int) string {
	digits := strings.TrimLeft(integer+fraction, "0")
	if strings.Trim(digits, "0") == "" {
		return sign + "0"
	}
	point := len(integer) + shift - (len(integer) + len(fraction) - len(digits))
	if point <= 0 {
		return sign + "0." + strings.Repeat("0", -point) + strings.TrimRight(digits, "0")
	}
	if point >= len(digits) {
		return sign + digits + strings.Repeat("0", point-len(digits))
	}
	fraction = strings.TrimRight(digits[point:], "0")
	if fraction == "" {
		return sign + digits[:point]
	}
	return sign + digits[:point] + "." + fraction
}
