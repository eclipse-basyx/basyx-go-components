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

package builder

import (
	"strconv"
	"strings"
)

// TokenizeField parses a field reference into a slice of Tokens.
//
// The input must be a field path of the form:
//
//	<prefix>#field.part[0].subpart
//
// TokenizeField performs the following steps:
//  1. Removes everything up to and including the first '#'.
//  2. Splits the remaining expression on '.'.
//  3. Converts each segment into either a SimpleToken (no brackets)
//     or an ArrayToken (contains "[index]").
//     An empty index ("[]") is interpreted as a wildcard and stored as -1.
//
// Example:
//
//	input:  "$aasdesc#submodels[2].id"
//	output: [ SimpleToken{"submodels"}, ArrayToken{"submodels", 2}, SimpleToken{"id"} ]
func TokenizeField(field string) []Token {
	str := field

	// 1. cut away everything before and including #
	str = str[strings.Index(str, "#")+1:]

	// 2. Split at "."
	rawToken := strings.Split(str, ".")

	var tokens []Token
	for _, t := range rawToken {
		if strings.Contains(t, "[") && strings.Contains(t, "]") {

			// nolint:gocritic // safe: guarded by Contains() above
			name := t[:strings.Index(t, "[")]
			indexStr := t[strings.Index(t, "[")+1 : strings.Index(t, "]")]

			var index int
			if indexStr == "" {
				index = -1 // wildcard index
			} else {
				i, err := strconv.Atoi(indexStr)
				if err != nil {
					index = -1 // malformed index → treat as wildcard
				} else {
					index = i
				}
			}

			tokens = append(tokens, ArrayToken{Name: name, Index: index})
			continue
		}

		// SimpleToken
		tokens = append(tokens, SimpleToken{Name: t})
	}

	return tokens
}

// Token represents one part of a parsed field expression.
// Tokens correspond to segments split by '.' or array selectors such as "[0]".
type Token interface {
	// GetName returns the primary name of the token (field name or array name).
	GetName() string
}

// ArrayToken represents a token with an array index, such as "foo[3]".
// An Index value of -1 indicates a wildcard index ("[]").
type ArrayToken struct {
	Name  string
	Index int
}

// GetName returns the array name without the index.
func (ap ArrayToken) GetName() string {
	return ap.Name
}

// SimpleToken represents a plain, non-array field name such as "id" or "submodel".
type SimpleToken struct {
	Name string
}

// GetName returns the name of the simple token.
func (sp SimpleToken) GetName() string {
	return sp.Name
}
