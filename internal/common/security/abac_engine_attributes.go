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
// Author: Martin Stemmer ( Fraunhofer IESE )

package auth

import (
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// attributesSatisfiedAll returns true only if ALL required attributes are satisfied.
// Rules supported:
//   - GLOBAL=ANONYMOUS         → satisfied unconditionally
//   - CLAIM=<claimKey>         → user must have that claim key (presence check)
//
// If items is empty, it returns true. Unknown kinds fail closed (return false).
func attributesSatisfiedAll(items []grammar.AttributeItem, claims Claims) bool {
	// with no attributes deny access per default
	if len(items) == 0 {
		return false
	}

	for _, it := range items {
		switch it.Kind {
		case grammar.ATTRGLOBAL:
			// Currently only ANONYMOUS is supported per your comment.
			if it.Value == "ANONYMOUS" {
				// satisfied → continue checking the rest
				continue
			}
			// Unsupported GLOBAL value → ignore
			continue

		case grammar.ATTRCLAIM:
			// Presence-only check: user must have this claim key

			if _, ok := claims[it.Value]; !ok {
				return false
			}

		default:
			// Unknown attribute type → fail closed
			return false
		}
	}

	// All attributes satisfied
	return true
}
