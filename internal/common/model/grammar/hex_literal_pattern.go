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

// Package grammar defines the data structures for representing hex literal patterns in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"fmt"
	"regexp"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// HexLiteralPattern represents a hexadecimal literal in the AAS grammar.
//
// This type enforces a specific format for hexadecimal literals used in the Asset Administration
// Shell grammar. The pattern must start with the prefix "16#" followed by one or more uppercase
// hexadecimal digits (0-9, A-F).
//
// The format is based on the IEC 61131-3 standard for representing hexadecimal literals,
// where the base (16) is specified as a prefix followed by a hash symbol.
//
// Valid examples:
//   - "16#A"
//   - "16#FF"
//   - "16#1234ABCD"
//   - "16#0"
//
// Invalid examples:
//   - "16#" (no digits)
//   - "16#abc" (lowercase not allowed)
//   - "0xFF" (wrong prefix format)
//   - "A1" (missing prefix)
type HexLiteralPattern string

// UnmarshalJSON implements the json.Unmarshaler interface for HexLiteralPattern.
//
// This custom unmarshaler validates that the JSON string value matches the required
// hexadecimal literal pattern: ^16#[0-9A-F]+$. The validation ensures that only
// properly formatted IEC 61131-3 style hexadecimal literals are accepted.
//
// Parameters:
//   - value: JSON byte slice containing the string value to unmarshal
//
// Returns:
//   - error: An error if the JSON is invalid or if the string doesn't match the required pattern.
//     The error message includes the expected pattern format.
func (j *HexLiteralPattern) UnmarshalJSON(value []byte) error {
	type Plain HexLiteralPattern
	var plain Plain
	if err := common.UnmarshalAndDisallowUnknownFields(value, &plain); err != nil {
		return err
	}
	if matched, _ := regexp.MatchString(`^16#[0-9A-F]+$`, string(plain)); !matched {
		return fmt.Errorf("field %s pattern match: must match %s", "", `^16#[0-9A-F]+$`)
	}
	*j = HexLiteralPattern(plain)
	return nil
}
