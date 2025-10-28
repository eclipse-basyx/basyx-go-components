/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )
package querylanguage

type NumVal int64

func NewNumVal(value int64) *NumVal {
	numVal := NumVal(value)
	return &numVal
}

type Operand struct {
	Field       string  `json:"$field"`
	StrVal      string  `json:"$strVal,omitempty"`
	NumVal      *NumVal `json:"$numVal,omitempty"`
	HexVal      string  `json:"$hexVal,omitempty"`
	DateTimeVal string  `json:"$dateTimeVal,omitempty"`
	TimeVal     string  `json:"$timeVal,omitempty"`
	DayOfWeek   string  `json:"$dayOfWeek,omitempty"`
	DayOfMonth  string  `json:"$dayOfMonth,omitempty"`
	Month       string  `json:"$month,omitempty"`
	Year        string  `json:"$year,omitempty"`
	Boolean     string  `json:"$boolean,omitempty"`
}

func (o *Operand) GetOperandType() string {
	if o.Field != "" {
		return "$field"
	}
	if o.StrVal != "" {
		return "$strVal"
	}
	if o.NumVal != nil {
		return "$numVal"
	}
	if o.HexVal != "" {
		return "$hexVal"
	}
	if o.DateTimeVal != "" {
		return "$dateTimeVal"
	}
	if o.TimeVal != "" {
		return "$timeVal"
	}
	if o.DayOfWeek != "" {
		return "$dayOfWeek"
	}
	if o.DayOfMonth != "" {
		return "$dayOfMonth"
	}
	if o.Month != "" {
		return "$month"
	}
	if o.Year != "" {
		return "$year"
	}
	if o.Boolean != "" {
		return "$boolean"
	}
	return "unknown"
}

func (o *Operand) GetValue() any {
	switch o.GetOperandType() {
	case "$field":
		return o.Field
	case "$strVal":
		return o.StrVal
	case "$numVal":
		return o.NumVal
	case "$hexVal":
		return o.HexVal
	case "$dateTimeVal":
		return o.DateTimeVal
	case "$timeVal":
		return o.TimeVal
	case "$dayOfWeek":
		return o.DayOfWeek
	case "$dayOfMonth":
		return o.DayOfMonth
	case "$month":
		return o.Month
	case "$year":
		return o.Year
	case "$boolean":
		return o.Boolean
	default:
		return "unknown"
	}
}
