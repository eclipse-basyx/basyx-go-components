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
// Author: Martin Stemmer ( Fraunhofer IESE )

package auth

import (
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// attributesSatisfiedAll returns true when all required claims are present and
// the attributes identify either a claim-based or anonymous subject. Date-time
// globals do not restrict access at this stage and are resolved when evaluating
// formulas. Unknown attributes fail closed.
func attributesSatisfiedAll(items []grammar.AttributeItem, claims Claims) bool {
	if len(items) == 0 {
		return false
	}

	hasSubjectAttribute := false
	for _, it := range items {
		switch it.Kind {
		case grammar.ATTRGLOBAL:
			switch it.Value {
			case "ANONYMOUS":
				hasSubjectAttribute = true
			case "UTCNOW", "LOCALNOW", "CLIENTNOW":
			default:
				return false
			}
		case grammar.ATTRCLAIM:
			hasSubjectAttribute = true
			if _, ok := claims[it.Value]; !ok {
				return false
			}
		default:
			return false
		}
	}

	return hasSubjectAttribute
}
