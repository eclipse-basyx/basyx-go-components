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

package grammar

import (
	"fmt"
	"regexp"
	"strings"
)

const arrayIndexPattern = `\[(?:0|[1-9][0-9]*)?\]`
const idShortSegmentPattern = `[A-Za-z](?:[A-Za-z0-9_-]*[A-Za-z0-9_])?`
const idShortPathPattern = idShortSegmentPattern + `(?:` + arrayIndexPattern + `)*(?:\.` + idShortSegmentPattern + `(?:` + arrayIndexPattern + `)*)*`

const referencePartValuePattern = `(?:type|keys` + arrayIndexPattern + `\.(?:type|value))`
const referencePartFragmentPattern = `(?:keys` + arrayIndexPattern + `)?`

const semanticIDValuePattern = `semanticId(?:\.(?:` + referencePartValuePattern + `))?`
const semanticIDFragmentPattern = `semanticId(?:\.` + referencePartFragmentPattern + `)?`

const supplementalSemanticIDValuePattern = `supplementalSemanticIds` + arrayIndexPattern + `\.(?:` + referencePartValuePattern + `)`
const supplementalSemanticIDFragmentPattern = `supplementalSemanticIds` + arrayIndexPattern + `(?:\.` + referencePartFragmentPattern + `)?`

var referenceIdentifierInstancePattern = regexp.MustCompile(`^(\$(?:aas|sm|sme|cd|aasdesc|smdesc))\s*\(\s*"(\*|[^"]+)"\s*\)(.*)$`)

// ValidateReferenceIdentifier validates an attribute REFERENCE identifier.
//
// Runtime resolution of REFERENCE attributes is intentionally separate from
// grammar validation. This accepts the PR #88 quoted identifier form by removing
// the identifier instance and validating the remaining field path.
func ValidateReferenceIdentifier(value string) error {
	normalized := strings.TrimSpace(value)
	if modelPatternRegex.MatchString(normalized) {
		return nil
	}

	match := referenceIdentifierInstancePattern.FindStringSubmatch(normalized)
	if match == nil {
		return fmt.Errorf("GRAMMAR-REFERENCE-PATTERN: REFERENCE must be a valid ReferenceIdentifier")
	}

	fieldIdentifier := match[1] + match[3]
	if modelPatternRegex.MatchString(fieldIdentifier) {
		return nil
	}

	return fmt.Errorf("GRAMMAR-REFERENCE-PATTERN: REFERENCE must be a valid ReferenceIdentifier")
}
