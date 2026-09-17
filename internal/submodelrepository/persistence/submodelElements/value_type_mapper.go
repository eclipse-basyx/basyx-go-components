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
	"fmt"
	"strconv"

	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// TypedValue represents a value categorized by its XS datatype for database storage.
// Each field corresponds to a different column type in the database schema,
// allowing for type-appropriate storage and retrieval of AAS values.
type TypedValue struct {
	Text     sql.NullString // For string-like types: xs:string, xs:anyURI, xs:base64Binary, xs:hexBinary
	Numeric  sql.NullString // For numeric types: xs:int, xs:integer, xs:decimal, xs:double, xs:float, etc.
	Boolean  sql.NullString // For xs:boolean type
	Time     sql.NullString // For xs:time type
	Date     sql.NullString // For xs:date type
	DateTime sql.NullString // For date/time types: xs:dateTime, xs:duration, xs:gDay, etc.
}

// MapValueByType categorizes a value into the appropriate TypedValue field based on its XS datatype.
// This consolidates the repeated switch statements found throughout the codebase for
// determining which database column to use for storing AAS values.
//
// Parameters:
//   - valueType: The XS datatype string (e.g., "xs:string", "xs:int", "xs:boolean")
//   - value: The actual value to be stored
//
// Returns:
//   - TypedValue: A struct with the value placed in the appropriate field based on type
//   - error: A bad request when a boolean or temporal value is inconsistent with its XS datatype
func MapValueByType(valueType types.DataTypeDefXSD, value *string) (TypedValue, error) {
	tv := TypedValue{}
	valid := value != nil && (*value != "" || IsTextType(valueType))
	actualValue := ""
	if value != nil {
		actualValue = *value
	}
	if valid {
		if err := validateDatabaseTypedValue(valueType, actualValue); err != nil {
			return tv, err
		}
	}
	switch {
	case IsTextType(valueType):
		tv.Text = sql.NullString{String: actualValue, Valid: valid}
	case IsNumericType(valueType):
		if valid && !isValidNumeric(actualValue) {
			tv.Text = sql.NullString{String: actualValue, Valid: valid}
		} else {
			tv.Numeric = sql.NullString{String: actualValue, Valid: valid}
		}
	case valueType == types.DataTypeDefXSDBoolean:
		tv.Boolean = sql.NullString{String: actualValue, Valid: valid}
	case valueType == types.DataTypeDefXSDTime:
		tv.Time = sql.NullString{String: actualValue, Valid: valid}
	case valueType == types.DataTypeDefXSDDate:
		tv.Date = sql.NullString{String: actualValue, Valid: valid}
	case IsDateTimeType(valueType):
		tv.DateTime = sql.NullString{String: actualValue, Valid: valid}
	default:
		// Fallback to text for unknown types
		tv.Text = sql.NullString{String: actualValue, Valid: valid}
	}
	return tv, nil
}

// IsTextType checks if the given XS datatype is a text/string type.
//
// Parameters:
//   - valueType: The XS datatype string to check
//
// Returns:
//   - bool: true if the type is a text type, false otherwise
func IsTextType(valueType types.DataTypeDefXSD) bool {
	switch valueType {
	case types.DataTypeDefXSDString, types.DataTypeDefXSDAnyURI, types.DataTypeDefXSDBase64Binary, types.DataTypeDefXSDHexBinary:
		return true
	default:
		return false
	}
}

// IsNumericType checks if the given XS datatype is a numeric type.
//
// Parameters:
//   - valueType: The XS datatype string to check
//
// Returns:
//   - bool: true if the type is a numeric type, false otherwise
func IsNumericType(valueType types.DataTypeDefXSD) bool {
	switch valueType {
	case types.DataTypeDefXSDInt, types.DataTypeDefXSDInteger, types.DataTypeDefXSDLong, types.DataTypeDefXSDShort, types.DataTypeDefXSDByte,
		types.DataTypeDefXSDUnsignedInt, types.DataTypeDefXSDUnsignedLong, types.DataTypeDefXSDUnsignedShort, types.DataTypeDefXSDUnsignedByte,
		types.DataTypeDefXSDPositiveInteger, types.DataTypeDefXSDNegativeInteger, types.DataTypeDefXSDNonNegativeInteger, types.DataTypeDefXSDNonPositiveInteger,
		types.DataTypeDefXSDDecimal, types.DataTypeDefXSDDouble, types.DataTypeDefXSDFloat:
		return true
	default:
		return false
	}
}

func isValidNumeric(value string) bool {
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

func validateDatabaseTypedValue(valueType types.DataTypeDefXSD, value string) error {
	switch valueType {
	case types.DataTypeDefXSDBoolean,
		types.DataTypeDefXSDTime,
		types.DataTypeDefXSDDate,
		types.DataTypeDefXSDDateTime:
		if !verification.ValueConsistentWithXSDType(value, valueType) {
			return common.NewErrBadRequest(fmt.Sprintf(
				"SMREPO-MAPVALUE-INVALIDXSD value %q is not consistent with %s",
				value,
				stringification.MustDataTypeDefXSDToString(valueType),
			))
		}
	}
	return nil
}

// IsDateTimeType checks if the given XS datatype should be stored in TIMESTAMPTZ columns.
//
// Parameters:
//   - valueType: The XS datatype string to check
//
// Returns:
//   - bool: true if the type is xs:dateTime (stored in TIMESTAMPTZ), false otherwise
func IsDateTimeType(valueType types.DataTypeDefXSD) bool {
	switch valueType {
	case types.DataTypeDefXSDDateTime:
		return true
	default:
		return false
	}
}

// TypedRangeValue represents min/max values categorized by XS datatype for database storage.
// Used for Range submodel elements that have both min and max values.
type TypedRangeValue struct {
	MinText     sql.NullString
	MaxText     sql.NullString
	MinNumeric  sql.NullString
	MaxNumeric  sql.NullString
	MinTime     sql.NullString
	MaxTime     sql.NullString
	MinDate     sql.NullString
	MaxDate     sql.NullString
	MinDateTime sql.NullString
	MaxDateTime sql.NullString
}

// MapRangeValueByType categorizes min/max values into the appropriate TypedRangeValue fields based on XS datatype.
//
// Parameters:
//   - valueType: The XS datatype string (e.g., "xs:string", "xs:int", "xs:time")
//   - minValue: The minimum value to be stored
//   - maxValue: The maximum value to be stored
//
// Returns:
//   - TypedRangeValue: A struct with the values placed in the appropriate fields based on type
//   - error: A bad request when a temporal bound is inconsistent with its XS datatype
func MapRangeValueByType(valueType types.DataTypeDefXSD, minValue string, maxValue string) (TypedRangeValue, error) {
	tv := TypedRangeValue{}
	minValid := minValue != ""
	maxValid := maxValue != ""
	if minValid {
		if err := validateDatabaseTypedValue(valueType, minValue); err != nil {
			return tv, err
		}
	}
	if maxValid {
		if err := validateDatabaseTypedValue(valueType, maxValue); err != nil {
			return tv, err
		}
	}

	switch {
	case IsTextType(valueType):
		tv.MinText = sql.NullString{String: minValue, Valid: minValid}
		tv.MaxText = sql.NullString{String: maxValue, Valid: maxValid}
	case IsNumericType(valueType):
		minIsNumeric := !minValid || isValidNumeric(minValue)
		maxIsNumeric := !maxValid || isValidNumeric(maxValue)
		if minIsNumeric && maxIsNumeric {
			tv.MinNumeric = sql.NullString{String: minValue, Valid: minValid}
			tv.MaxNumeric = sql.NullString{String: maxValue, Valid: maxValid}
		} else {
			tv.MinText = sql.NullString{String: minValue, Valid: minValid}
			tv.MaxText = sql.NullString{String: maxValue, Valid: maxValid}
		}
	case valueType == types.DataTypeDefXSDTime:
		tv.MinTime = sql.NullString{String: minValue, Valid: minValid}
		tv.MaxTime = sql.NullString{String: maxValue, Valid: maxValid}
	case valueType == types.DataTypeDefXSDDate:
		tv.MinDate = sql.NullString{String: minValue, Valid: minValid}
		tv.MaxDate = sql.NullString{String: maxValue, Valid: maxValid}
	case IsDateTimeType(valueType):
		tv.MinDateTime = sql.NullString{String: minValue, Valid: minValid}
		tv.MaxDateTime = sql.NullString{String: maxValue, Valid: maxValid}
	default:
		// Fallback to text for unknown types
		tv.MinText = sql.NullString{String: minValue, Valid: minValid}
		tv.MaxText = sql.NullString{String: maxValue, Valid: maxValid}
	}
	return tv, nil
}
