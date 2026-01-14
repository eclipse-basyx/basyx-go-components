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

// Package persistenceutils provides utility functions for persisting AAS entities
package persistenceutils

import "database/sql"

// TypedValue represents a value categorized by its XS datatype for database storage.
// Each field corresponds to a different column type in the database schema,
// allowing for type-appropriate storage and retrieval of AAS values.
type TypedValue struct {
	Text     sql.NullString // For string-like types: xs:string, xs:anyURI, xs:base64Binary, xs:hexBinary
	Numeric  sql.NullString // For numeric types: xs:int, xs:integer, xs:decimal, xs:double, xs:float, etc.
	Boolean  sql.NullString // For xs:boolean type
	Time     sql.NullString // For xs:time type
	DateTime sql.NullString // For date/time types: xs:date, xs:dateTime, xs:duration, xs:gDay, etc.
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
func MapValueByType(valueType string, value string) TypedValue {
	tv := TypedValue{}
	valid := value != ""

	switch {
	case IsTextType(valueType):
		tv.Text = sql.NullString{String: value, Valid: valid}
	case IsNumericType(valueType):
		tv.Numeric = sql.NullString{String: value, Valid: valid}
	case valueType == "xs:boolean":
		tv.Boolean = sql.NullString{String: value, Valid: valid}
	case valueType == "xs:time":
		tv.Time = sql.NullString{String: value, Valid: valid}
	case IsDateTimeType(valueType):
		tv.DateTime = sql.NullString{String: value, Valid: valid}
	default:
		// Fallback to text for unknown types
		tv.Text = sql.NullString{String: value, Valid: valid}
	}
	return tv
}

// IsTextType checks if the given XS datatype is a text/string type.
//
// Parameters:
//   - valueType: The XS datatype string to check
//
// Returns:
//   - bool: true if the type is a text type, false otherwise
func IsTextType(valueType string) bool {
	switch valueType {
	case "xs:string", "xs:anyURI", "xs:base64Binary", "xs:hexBinary":
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
func IsNumericType(valueType string) bool {
	switch valueType {
	case "xs:int", "xs:integer", "xs:long", "xs:short", "xs:byte",
		"xs:unsignedInt", "xs:unsignedLong", "xs:unsignedShort", "xs:unsignedByte",
		"xs:positiveInteger", "xs:negativeInteger", "xs:nonNegativeInteger", "xs:nonPositiveInteger",
		"xs:decimal", "xs:double", "xs:float":
		return true
	default:
		return false
	}
}

// IsDateTimeType checks if the given XS datatype is a date/time type.
//
// Parameters:
//   - valueType: The XS datatype string to check
//
// Returns:
//   - bool: true if the type is a date/time type, false otherwise
func IsDateTimeType(valueType string) bool {
	switch valueType {
	case "xs:date", "xs:dateTime", "xs:duration", "xs:gDay", "xs:gMonth",
		"xs:gMonthDay", "xs:gYear", "xs:gYearMonth":
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
func MapRangeValueByType(valueType string, minValue string, maxValue string) TypedRangeValue {
	tv := TypedRangeValue{}
	minValid := minValue != ""
	maxValid := maxValue != ""

	switch {
	case IsTextType(valueType):
		tv.MinText = sql.NullString{String: minValue, Valid: minValid}
		tv.MaxText = sql.NullString{String: maxValue, Valid: maxValid}
	case IsNumericType(valueType):
		tv.MinNumeric = sql.NullString{String: minValue, Valid: minValid}
		tv.MaxNumeric = sql.NullString{String: maxValue, Valid: maxValid}
	case valueType == "xs:time":
		tv.MinTime = sql.NullString{String: minValue, Valid: minValid}
		tv.MaxTime = sql.NullString{String: maxValue, Valid: maxValid}
	case IsDateTimeType(valueType):
		tv.MinDateTime = sql.NullString{String: minValue, Valid: minValid}
		tv.MaxDateTime = sql.NullString{String: maxValue, Valid: maxValid}
	default:
		// Fallback to text for unknown types
		tv.MinText = sql.NullString{String: minValue, Valid: minValid}
		tv.MaxText = sql.NullString{String: maxValue, Valid: maxValid}
	}
	return tv
}

// GetRangeColumnNames returns the appropriate column names for min and max values
// based on the XML Schema datatype of the Range element.
//
// Parameters:
//   - valueType: The XS datatype string
//
// Returns:
//   - minCol: The column name for the minimum value
//   - maxCol: The column name for the maximum value
func GetRangeColumnNames(valueType string) (minCol, maxCol string) {
	switch {
	case IsTextType(valueType):
		return "min_text", "max_text"
	case IsNumericType(valueType):
		return "min_num", "max_num"
	case valueType == "xs:time":
		return "min_time", "max_time"
	case IsDateTimeType(valueType):
		return "min_datetime", "max_datetime"
	default:
		// Fallback to text
		return "min_text", "max_text"
	}
}
