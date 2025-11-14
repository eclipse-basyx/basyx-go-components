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
	"fmt"
	"strings"
)

func TokenizeField(field string) []Token {
	str := field
	// 1. cut away everything before and including #
	str = str[strings.Index(str, "#")+1:]
	// 2. Split into array of strings at .
	rawToken := strings.Split(str, ".")
	// If includes array - Create ArrayToken else SimpleToken
	var tokens []Token
	for _, t := range rawToken {
		if strings.Contains(t, "[") && strings.Contains(t, "]") {
			// ArrayToken
			name := t[:strings.Index(t, "[")]
			indexStr := t[strings.Index(t, "[")+1 : strings.Index(t, "]")]
			var index int
			if indexStr == "" {
				index = -1 // represent wildcard
			} else {
				// Convert to int
				fmt.Sscanf(indexStr, "%d", &index)
			}
			tokens = append(tokens, ArrayToken{Name: name, Index: index})
		} else {
			// SimpleToken
			tokens = append(tokens, SimpleToken{Name: t})
		}
	}
	return tokens
}

type Token interface {
	GetName() string
}

type ArrayToken struct {
	Name  string
	Index int
}

func (ap ArrayToken) GetName() string {
	return ap.Name
}

type SimpleToken struct {
	Name string
}

func (sp SimpleToken) GetName() string {
	return sp.Name
}
