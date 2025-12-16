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

// Package grammar defines the data structures for representing aliases in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
//
//nolint:all
package grammar

import "time"

// StringItems represents a collection of string values used in the AAS Access Rule Language.
//
// This type alias provides a convenient way to work with arrays of StringValue instances,
// which can be used in various contexts such as attribute lists, object identifiers,
// or pattern matching operations within access control rules.
type StringItems []StringValue

// ComparisonItems represents a collection of values used for comparison operations.
//
// This type alias provides a convenient way to work with arrays of Value instances,
// typically used in logical expressions, match operations, or conditional evaluations
// within the access permission rules. Values can be literals, attributes, or references
// to AAS elements.
type ComparisonItems []Value

// DateTimeLiteralPattern represents a timestamp value in the AAS Access Rule Language.
//
// This type alias wraps the standard time.Time type to represent date and time literals
// used in access control conditions. Common use cases include:
//   - Time-based access restrictions (e.g., "allow access only during business hours")
//   - Temporal validity checks (e.g., "deny access after expiration date")
//   - Comparison with global time attributes like LOCALNOW or UTCNOW
//
// Example usage in conditions:
//   - Check if current time is within a valid access window
//   - Verify resource creation or modification timestamps
//   - Implement time-based access policies
type DateTimeLiteralPattern time.Time

// OBJECTTYPE represents the type identifier for AAS objects in access control rules.
//
// This type alias is used to specify which kind of Asset Administration Shell resource
// an access permission rule applies to. While defined as a string alias here for type safety,
// the actual valid values are defined as constants in the grammar package.
//
// Common object types include:
//   - SUBMODEL: References to AAS Submodels
//   - AAS: References to Asset Administration Shells
//   - CONCEPTDESCRIPTION: References to Concept Descriptions
//
// This type provides compile-time type checking while maintaining the flexibility
// of string-based representations for extensibility.
type OBJECTTYPE string
