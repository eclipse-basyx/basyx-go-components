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
	"math"
	"strconv"

	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// TypedValue represents a value categorized by its XS datatype for database storage.
// Each field corresponds to a different column type in the database schema,
// allowing for type-appropriate storage and retrieval of AAS values.
type TypedValue struct {
	// Text holds string-like, temporal, and non-numeric lexical values verbatim.
	Text sql.NullString
	// Numeric holds values of numeric XSD types that PostgreSQL can store as numeric.
	Numeric sql.NullString
	// Boolean holds xs:boolean values.
	Boolean sql.NullString
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
	valid := isStorableLexical(valueType, value)
	actualValue := lexicalOrEmpty(value)
	if valid {
		if err := validateStoredLexical(valueType, actualValue); err != nil {
			return tv, err
		}
	}
	switch {
	case gen.IsNumericXSDType(valueType):
		if valid && !isValidNumeric(actualValue) {
			tv.Text = sql.NullString{String: actualValue, Valid: valid}
		} else {
			tv.Numeric = sql.NullString{String: actualValue, Valid: valid}
		}
	case valueType == types.DataTypeDefXSDBoolean:
		tv.Boolean = sql.NullString{String: actualValue, Valid: valid}
	default:
		tv.Text = sql.NullString{String: actualValue, Valid: valid}
	}
	return tv, nil
}

// isStorableLexical reports whether a lexical value is stored as a value instead of NULL.
// The empty string is a valid lexical value only for text types.
func isStorableLexical(valueType types.DataTypeDefXSD, lexical *string) bool {
	return lexical != nil && (*lexical != "" || IsTextType(valueType))
}

func lexicalOrEmpty(lexical *string) string {
	if lexical == nil {
		return ""
	}
	return *lexical
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

// isValidNumeric reports whether a NUMERIC column stores the value without altering it.
// INF, -INF and NaN are kept as text to preserve their XSD lexical form.
func isValidNumeric(value string) bool {
	number, err := strconv.ParseFloat(value, 64)
	return err == nil && !math.IsInf(number, 0) && !math.IsNaN(number)
}

func validateStoredLexical(valueType types.DataTypeDefXSD, value string) error {
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

// TypedRangeValue represents the values stored for the two bounds of a Range.
type TypedRangeValue struct {
	// MinText holds the lower bound verbatim when it is not stored as numeric.
	MinText sql.NullString
	// MaxText holds the upper bound verbatim when it is not stored as numeric.
	MaxText sql.NullString
	// MinNumeric holds the lower bound of a numeric Range.
	MinNumeric sql.NullString
	// MaxNumeric holds the upper bound of a numeric Range.
	MaxNumeric sql.NullString
}

// MapRangeValueByType categorizes min/max values into the appropriate TypedRangeValue fields based on XS datatype.
//
// Parameters:
//   - valueType: The XS datatype string (e.g., "xs:string", "xs:int", "xs:time")
//   - minValue: The minimum value to be stored; nil, or empty for non-text types, is stored as NULL
//   - maxValue: The maximum value to be stored; nil, or empty for non-text types, is stored as NULL
//
// Returns:
//   - TypedRangeValue: A struct with the values placed in the appropriate fields based on type
//   - error: A bad request when a temporal bound is inconsistent with its XS datatype
func MapRangeValueByType(valueType types.DataTypeDefXSD, minLexical *string, maxLexical *string) (TypedRangeValue, error) {
	tv := TypedRangeValue{}
	minValid := isStorableLexical(valueType, minLexical)
	maxValid := isStorableLexical(valueType, maxLexical)
	minValue := lexicalOrEmpty(minLexical)
	maxValue := lexicalOrEmpty(maxLexical)
	if minValid {
		if err := validateStoredLexical(valueType, minValue); err != nil {
			return tv, err
		}
	}
	if maxValid {
		if err := validateStoredLexical(valueType, maxValue); err != nil {
			return tv, err
		}
	}

	switch {
	case gen.IsNumericXSDType(valueType):
		minIsNumeric := !minValid || isValidNumeric(minValue)
		maxIsNumeric := !maxValid || isValidNumeric(maxValue)
		if minIsNumeric && maxIsNumeric {
			tv.MinNumeric = sql.NullString{String: minValue, Valid: minValid}
			tv.MaxNumeric = sql.NullString{String: maxValue, Valid: maxValid}
		} else {
			tv.MinText = sql.NullString{String: minValue, Valid: minValid}
			tv.MaxText = sql.NullString{String: maxValue, Valid: maxValid}
		}
	default:
		tv.MinText = sql.NullString{String: minValue, Valid: minValid}
		tv.MaxText = sql.NullString{String: maxValue, Valid: maxValid}
	}
	return tv, nil
}
