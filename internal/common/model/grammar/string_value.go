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

// Package grammar defines the data structures for representing string values in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
//
//nolint:all
package grammar

// StringValue represents a string value in the grammar model, which can be a literal string, a field reference, or a string cast.
type StringValue struct {
	// Attribute corresponds to the JSON schema field "$attribute".
	Attribute AttributeValue `json:"$attribute,omitempty" yaml:"$attribute,omitempty" mapstructure:"$attribute,omitempty"`

	// Field corresponds to the JSON schema field "$field".
	Field *ModelStringPattern `json:"$field,omitempty" yaml:"$field,omitempty" mapstructure:"$field,omitempty"`

	// StrCast corresponds to the JSON schema field "$strCast".
	StrCast *Value `json:"$strCast,omitempty" yaml:"$strCast,omitempty" mapstructure:"$strCast,omitempty"`

	// StrVal corresponds to the JSON schema field "$strVal".
	StrVal *StandardString `json:"$strVal,omitempty" yaml:"$strVal,omitempty" mapstructure:"$strVal,omitempty"`
}
