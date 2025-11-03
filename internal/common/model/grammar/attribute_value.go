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

// Package grammar defines the data structures for representing attribute values in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package grammar

// AttributeValue represents a value associated with an attribute in the AAS access control grammar.
//
// This type is defined as 'any' to allow maximum flexibility in representing attribute values
// of different types. Attributes in the access control system can have values of various types
// including strings, numbers, booleans, arrays, or complex objects, depending on the attribute's
// source and purpose.
//
// Common value types:
//   - string: Text values from claims, model references, or configuration
//   - int/float64: Numeric values for comparisons or measurements
//   - bool: Boolean flags or conditions
//   - []any: Arrays of values for multi-valued attributes
//   - map[string]any: Structured data from complex attributes
//
// The actual type of the value depends on the context in which the attribute is used
// and should be validated or type-checked when performing operations on the value.
type AttributeValue any
