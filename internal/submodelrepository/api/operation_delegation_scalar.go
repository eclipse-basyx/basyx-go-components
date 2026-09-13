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

package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
)

const maxOperationDecimalExponent = 10000

func operationScalarInput(element map[string]any, value any) (string, error) {
	valueTypeName, _ := element["valueType"].(string)
	valueType, ok := stringification.DataTypeDefXSDFromString(valueTypeName)
	if !ok {
		return "", fmt.Errorf("SMREPO-OPVALREQ-VALUETYPE invalid declared valueType %q", valueTypeName)
	}
	lexical, err := operationScalarLexical(value, valueType)
	if err != nil {
		return "", err
	}
	if !verification.ValueConsistentWithXSDType(lexical, valueType) {
		return "", fmt.Errorf("SMREPO-OPVALREQ-SCALARVALUE value is invalid for %s", valueTypeName)
	}
	return lexical, nil
}

func operationScalarLexical(value any, valueType types.DataTypeDefXSD) (string, error) {
	switch scalar := value.(type) {
	case string:
		return scalar, nil
	case bool:
		if valueType == types.DataTypeDefXSDBoolean {
			return strconv.FormatBool(scalar), nil
		}
	case json.Number:
		if operationNumericType(valueType) {
			if valueType == types.DataTypeDefXSDFloat || valueType == types.DataTypeDefXSDDouble {
				return scalar.String(), nil
			}
			return operationDecimalLexical(scalar.String())
		}
	}
	return "", fmt.Errorf("SMREPO-OPVALREQ-PROPERTYSHAPE incompatible scalar %T for declared valueType", value)
}

func operationNumericType(valueType types.DataTypeDefXSD) bool {
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

func operationDecimalLexical(number string) (string, error) {
	mantissa, exponent, _ := strings.Cut(strings.ToLower(number), "e")
	shift := 0
	if exponent != "" {
		parsed, err := strconv.Atoi(exponent)
		if err != nil || parsed < -maxOperationDecimalExponent || parsed > maxOperationDecimalExponent {
			return "", fmt.Errorf("SMREPO-OPVALREQ-NUMBEREXPONENT decimal exponent exceeds supported expansion")
		}
		shift = parsed
	}
	sign := ""
	if strings.HasPrefix(mantissa, "-") {
		sign, mantissa = "-", mantissa[1:]
	}
	integer, fraction, _ := strings.Cut(mantissa, ".")
	digits := integer + fraction
	if strings.Trim(digits, "0") == "" {
		return sign + "0", nil
	}
	point := len(integer) + shift
	if point <= 0 {
		return sign + "0." + strings.Repeat("0", -point) + digits, nil
	}
	if point >= len(digits) {
		return sign + digits + strings.Repeat("0", point-len(digits)), nil
	}
	fraction = strings.TrimRight(digits[point:], "0")
	if fraction == "" {
		return sign + digits[:point], nil
	}
	return sign + digits[:point] + "." + fraction, nil
}

func applyOperationRangeValue(element map[string]any, value any) error {
	fields, err := valueOnlyObject(value, "Range")
	if err != nil {
		return err
	}
	if err := validateObjectFields(fields, []string{"min", "max"}, []string{"min", "max"}, "Range"); err != nil {
		return err
	}
	for _, field := range []string{"min", "max"} {
		lexical, err := operationScalarInput(element, fields[field])
		if err != nil {
			return fmt.Errorf("SMREPO-OPVALREQ-RANGE-%s %w", field, err)
		}
		element[field] = lexical
	}
	return nil
}

func operationScalarOutput(value *string, valueType types.DataTypeDefXSD) (any, error) {
	if value == nil {
		return nil, nil
	}
	if !verification.ValueConsistentWithXSDType(*value, valueType) {
		return nil, fmt.Errorf("SMREPO-OPVALRES-SCALARVALUE delegated value is invalid for declared valueType")
	}
	if valueType == types.DataTypeDefXSDBoolean {
		return *value == "true" || *value == "1", nil
	}
	if operationNumericType(valueType) {
		return operationJSONNumber(*value)
	}
	return *value, nil
}

func operationJSONNumber(lexical string) (json.Number, error) {
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
		return "", fmt.Errorf("SMREPO-OPVALRES-NUMBER delegated numeric value has no finite JSON representation")
	}
	return json.Number(number), nil
}
