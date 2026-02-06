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

// Package grammar defines the data structures for representing values in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"fmt"
	"strings"
)

// Value represents a value in the grammar model, which can be a literal value or a field reference.
type Value struct {
	// Attribute corresponds to the JSON schema field "$attribute".
	Attribute AttributeValue `json:"$attribute,omitempty" yaml:"$attribute,omitempty" mapstructure:"$attribute,omitempty"`

	// BoolCast corresponds to the JSON schema field "$boolCast".
	BoolCast *Value `json:"$boolCast,omitempty" yaml:"$boolCast,omitempty" mapstructure:"$boolCast,omitempty"`

	// Boolean corresponds to the JSON schema field "$boolean".
	Boolean *bool `json:"$boolean,omitempty" yaml:"$boolean,omitempty" mapstructure:"$boolean,omitempty"`

	// DateTimeCast corresponds to the JSON schema field "$dateTimeCast".
	DateTimeCast *Value `json:"$dateTimeCast,omitempty" yaml:"$dateTimeCast,omitempty" mapstructure:"$dateTimeCast,omitempty"`

	// DateTimeVal corresponds to the JSON schema field "$dateTimeVal".
	DateTimeVal *DateTimeLiteralPattern `json:"$dateTimeVal,omitempty" yaml:"$dateTimeVal,omitempty" mapstructure:"$dateTimeVal,omitempty"`

	// DayOfMonth corresponds to the JSON schema field "$dayOfMonth".
	DayOfMonth *DateTimeLiteralPattern `json:"$dayOfMonth,omitempty" yaml:"$dayOfMonth,omitempty" mapstructure:"$dayOfMonth,omitempty"`

	// DayOfWeek corresponds to the JSON schema field "$dayOfWeek".
	DayOfWeek *DateTimeLiteralPattern `json:"$dayOfWeek,omitempty" yaml:"$dayOfWeek,omitempty" mapstructure:"$dayOfWeek,omitempty"`

	// Field corresponds to the JSON schema field "$field".
	Field *ModelStringPattern `json:"$field,omitempty" yaml:"$field,omitempty" mapstructure:"$field,omitempty"`

	// HexCast corresponds to the JSON schema field "$hexCast".
	HexCast *Value `json:"$hexCast,omitempty" yaml:"$hexCast,omitempty" mapstructure:"$hexCast,omitempty"`

	// HexVal corresponds to the JSON schema field "$hexVal".
	HexVal *HexLiteralPattern `json:"$hexVal,omitempty" yaml:"$hexVal,omitempty" mapstructure:"$hexVal,omitempty"`

	// Month corresponds to the JSON schema field "$month".
	Month *DateTimeLiteralPattern `json:"$month,omitempty" yaml:"$month,omitempty" mapstructure:"$month,omitempty"`

	// NumCast corresponds to the JSON schema field "$numCast".
	NumCast *Value `json:"$numCast,omitempty" yaml:"$numCast,omitempty" mapstructure:"$numCast,omitempty"`

	// NumVal corresponds to the JSON schema field "$numVal".
	NumVal *float64 `json:"$numVal,omitempty" yaml:"$numVal,omitempty" mapstructure:"$numVal,omitempty"`

	// StrCast corresponds to the JSON schema field "$strCast".
	StrCast *Value `json:"$strCast,omitempty" yaml:"$strCast,omitempty" mapstructure:"$strCast,omitempty"`

	// StrVal corresponds to the JSON schema field "$strVal".
	StrVal *StandardString `json:"$strVal,omitempty" yaml:"$strVal,omitempty" mapstructure:"$strVal,omitempty"`

	// TimeCast corresponds to the JSON schema field "$timeCast".
	TimeCast *Value `json:"$timeCast,omitempty" yaml:"$timeCast,omitempty" mapstructure:"$timeCast,omitempty"`

	// TimeVal corresponds to the JSON schema field "$timeVal".
	TimeVal *TimeLiteralPattern `json:"$timeVal,omitempty" yaml:"$timeVal,omitempty" mapstructure:"$timeVal,omitempty"`

	// Year corresponds to the JSON schema field "$year".
	Year *DateTimeLiteralPattern `json:"$year,omitempty" yaml:"$year,omitempty" mapstructure:"$year,omitempty"`
}

// GetValueType returns the type of value stored in a Value struct
func (v *Value) GetValueType() string {
	if v.Field != nil {
		return "$field"
	}
	if v.StrVal != nil {
		return "$strVal"
	}
	if v.NumVal != nil {
		return "$numVal"
	}
	if v.HexVal != nil {
		return "$hexVal"
	}
	if v.DateTimeVal != nil {
		return "$dateTimeVal"
	}
	if v.TimeVal != nil {
		return "$timeVal"
	}
	if v.DayOfWeek != nil {
		return "$dayOfWeek"
	}
	if v.DayOfMonth != nil {
		return "$dayOfMonth"
	}
	if v.Month != nil {
		return "$month"
	}
	if v.Year != nil {
		return "$year"
	}
	if v.Boolean != nil {
		return "$boolean"
	}
	if v.Attribute != nil {
		return "$attribute"
	}
	return "unknown"
}

// GetValue returns the actual value stored in a Value struct
func (v *Value) GetValue() interface{} {
	switch v.GetValueType() {
	case "$field":
		return string(*v.Field)
	case "$strVal":
		return string(*v.StrVal)
	case "$numVal":
		return *v.NumVal
	case "$hexVal":
		return string(*v.HexVal)
	case "$dateTimeVal":
		return *v.DateTimeVal
	case "$timeVal":
		return string(*v.TimeVal)
	case "$dayOfWeek":
		return *v.DayOfWeek
	case "$dayOfMonth":
		return *v.DayOfMonth
	case "$month":
		return *v.Month
	case "$year":
		return *v.Year
	case "$boolean":
		return *v.Boolean
	case "$attribute":
		return v.Attribute
	default:
		return nil
	}
}

// IsField returns true if the Value represents a field reference
func (v *Value) IsField() bool {
	return v.Field != nil
}

// IsValue returns true if the Value represents a literal value (not a field)
func (v *Value) IsValue() bool {
	return !v.IsField() && v.GetValueType() != "unknown"
}

// ComparisonKind describes the coarse-grained type category used when comparing values.
type ComparisonKind int

const (
	// KindUnknown represents an unresolved or unsupported type.
	KindUnknown ComparisonKind = iota
	// KindString represents string operands.
	KindString
	// KindField represents a field reference whose runtime type is unknown.
	KindField
	// KindNumber represents numeric operands.
	KindNumber
	// KindBool represents boolean operands.
	KindBool
	// KindDateTime represents date-time operands.
	KindDateTime
	// KindTime represents time-only operands.
	KindTime
	// KindHex represents hexadecimal operands.
	KindHex
)

// String returns a human-readable name for the comparison kind.
func (k ComparisonKind) String() string {
	switch k {
	case KindUnknown:
		return "Unknown"
	case KindString:
		return "String"
	case KindField:
		return "Field"
	case KindNumber:
		return "Number"
	case KindBool:
		return "Bool"
	case KindDateTime:
		return "DateTime"
	case KindTime:
		return "Time"
	case KindHex:
		return "Hex"
	default:
		return fmt.Sprintf("ComparisonKind(%d)", int(k))
	}
}

// EffectiveType returns a coarse type label used for validation of comparison operands.
// Fields return KindField, most attributes return KindString (except for UTCNOW-like globals which return KindDateTime).
func (v *Value) EffectiveType() ComparisonKind {
	switch {
	case v.Field != nil:
		return KindField
	case v.Attribute != nil:
		if gv, ok := attributeGlobalValue(v.Attribute); ok {
			if isNowGlobal(gv) {
				return KindDateTime
			}
		}
		return KindString
	case v.HexVal != nil, v.HexCast != nil:
		return KindHex
	case v.NumVal != nil, v.NumCast != nil, v.Year != nil, v.Month != nil, v.DayOfMonth != nil, v.DayOfWeek != nil:
		return KindNumber
	case v.StrVal != nil, v.StrCast != nil:
		return KindString
	case v.Boolean != nil, v.BoolCast != nil:
		return KindBool
	case v.DateTimeVal != nil, v.DateTimeCast != nil:
		return KindDateTime
	case v.TimeVal != nil, v.TimeCast != nil:
		return KindTime
	default:
		return KindUnknown
	}
}

// IsComparableTo checks whether two values can be compared and returns the common comparison kind.
func (v *Value) IsComparableTo(in Value) (ComparisonKind, error) {
	ltype := v.EffectiveType()
	rtype := in.EffectiveType()

	if ltype == KindUnknown || rtype == KindUnknown {
		return KindUnknown, fmt.Errorf("comparison has unknown operand types: %s vs %s", ltype.String(), rtype.String())
	}
	if ltype == KindField {
		return rtype, nil
	}

	if rtype == KindField {
		return ltype, nil
	}

	if ltype != rtype {
		return KindUnknown, fmt.Errorf("comparison requires matching operand types: %s vs %s", ltype.String(), rtype.String())
	}
	return ltype, nil
}

// extractFieldOperandAndCast walks through cast wrappers to find the underlying field operand
// and returns the outermost cast target type (if any).
func extractFieldOperandAndCast(v *Value) (*Value, string) {
	cur := v
	castType := ""
	for cur != nil {
		// Record only the outermost cast.
		if castType == "" {
			switch {
			case cur.StrCast != nil:
				castType = "text"
			case cur.NumCast != nil:
				castType = "double precision"
			case cur.BoolCast != nil:
				castType = "boolean"
			case cur.TimeCast != nil:
				castType = "time"
			case cur.DateTimeCast != nil:
				castType = "timestamptz"
			case cur.HexCast != nil:
				castType = "text"
			}
		}

		if cur.Field != nil {
			return cur, castType
		}
		switch {
		case cur.StrCast != nil:
			cur = cur.StrCast
		case cur.NumCast != nil:
			cur = cur.NumCast
		case cur.BoolCast != nil:
			cur = cur.BoolCast
		case cur.TimeCast != nil:
			cur = cur.TimeCast
		case cur.DateTimeCast != nil:
			cur = cur.DateTimeCast
		case cur.HexCast != nil:
			cur = cur.HexCast
		default:
			return nil, ""
		}
	}
	return nil, ""
}

// WrapCastAroundField wraps a field value in an explicit cast to align both operands' types.
func WrapCastAroundField(v Value, kind ComparisonKind) Value {
	if v.EffectiveType() != KindField {
		return v
	}

	orig := v
	switch kind {
	case KindString, KindField:
		return Value{StrCast: &orig}
	case KindDateTime:
		return Value{DateTimeCast: &orig}
	case KindTime:
		return Value{TimeCast: &orig}
	case KindBool:
		return Value{BoolCast: &orig}
	case KindNumber:
		return Value{NumCast: &orig}
	case KindHex:
		return Value{HexCast: &orig}
	default:
		return v
	}
}

// EffectiveTypeWithCast prefers the target type of an explicit cast over the raw EffectiveType.
// This keeps type validation in sync with the SQL that will actually be generated.
func (v *Value) EffectiveTypeWithCast() ComparisonKind {
	if v == nil {
		return KindUnknown
	}
	switch {
	case v.NumCast != nil:
		return KindNumber
	case v.BoolCast != nil:
		return KindBool
	case v.TimeCast != nil:
		return KindTime
	case v.DateTimeCast != nil:
		return KindDateTime
	case v.HexCast != nil:
		return KindHex
	case v.StrCast != nil:
		return KindString
	default:
		return v.EffectiveType()
	}
}

func attributeGlobalValue(attr AttributeValue) (string, bool) {
	switch a := attr.(type) {
	case map[string]string:
		if v, ok := a["GLOBAL"]; ok {
			return v, true
		}
	case map[string]any:
		if v, ok := a["GLOBAL"]; ok {
			return fmt.Sprint(v), true
		}
	}
	return "", false
}

func isNowGlobal(v string) bool {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "UTCNOW", "LOCALNOW", "CLIENTNOW":
		return true
	default:
		return false
	}
}

// AssertValueRequired checks if the required fields are not zero-ed
func AssertValueRequired(obj Value) error {
	if obj.StrCast != nil {
		if err := AssertValueRequired(*obj.StrCast); err != nil {
			return err
		}
	}
	if obj.NumCast != nil {
		if err := AssertValueRequired(*obj.NumCast); err != nil {
			return err
		}
	}
	if obj.HexCast != nil {
		if err := AssertValueRequired(*obj.HexCast); err != nil {
			return err
		}
	}
	if obj.BoolCast != nil {
		if err := AssertValueRequired(*obj.BoolCast); err != nil {
			return err
		}
	}
	if obj.DateTimeCast != nil {
		if err := AssertValueRequired(*obj.DateTimeCast); err != nil {
			return err
		}
	}
	if obj.TimeCast != nil {
		if err := AssertValueRequired(*obj.TimeCast); err != nil {
			return err
		}
	}
	return nil
}

// AssertValueConstraints checks if the values respects the defined constraints
func AssertValueConstraints(obj Value) error {
	if obj.StrCast != nil {
		if err := AssertValueConstraints(*obj.StrCast); err != nil {
			return err
		}
	}
	if obj.NumCast != nil {
		if err := AssertValueConstraints(*obj.NumCast); err != nil {
			return err
		}
	}
	if obj.HexCast != nil {
		if err := AssertValueConstraints(*obj.HexCast); err != nil {
			return err
		}
	}
	if obj.BoolCast != nil {
		if err := AssertValueConstraints(*obj.BoolCast); err != nil {
			return err
		}
	}
	if obj.DateTimeCast != nil {
		if err := AssertValueConstraints(*obj.DateTimeCast); err != nil {
			return err
		}
	}
	if obj.TimeCast != nil {
		if err := AssertValueConstraints(*obj.TimeCast); err != nil {
			return err
		}
	}
	return nil
}
