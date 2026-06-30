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

const supplementalSemanticIDValuePattern = `supplementalSemanticIds(?:` + arrayIndexPattern + `)?(?:\.(?:` + referencePartValuePattern + `))?`
const supplementalSemanticIDFragmentPattern = `supplementalSemanticIds(?:` + arrayIndexPattern + `)?(?:\.` + referencePartFragmentPattern + `)?`

const referenceIdentifierInstance = `\("[A-Za-z0-9/\*\[\]\(\) _@#\\+\-\.,:\$\^]+"\)`
const referenceIdentifierPattern = `^(?:` +
	`\$aas` + referenceIdentifierInstance + `#(?:idShort|id|assetInformation\.assetKind|assetInformation\.assetType|assetInformation\.globalAssetId|assetInformation\.` + specificAssetIDValuePattern + `|` + submodelReferenceValuePattern + `)|` +
	`\$sm` + referenceIdentifierInstance + `#(?:` + semanticIDValuePattern + `|` + supplementalSemanticIDValuePattern + `|idShort|id)|` +
	`\$cd` + referenceIdentifierInstance + `#(?:idShort|id)|` +
	`\$sme` + referenceIdentifierInstance + `\.` + idShortPathPattern + `#(?:` + semanticIDValuePattern + `|` + supplementalSemanticIDValuePattern + `|idShort|value|valueType|language)` +
	`)$`

var referenceIdentifierRegex = regexp.MustCompile(referenceIdentifierPattern)

// ValidateReferenceIdentifier validates an attribute REFERENCE identifier.
//
// Runtime resolution of REFERENCE attributes is intentionally separate from
// grammar validation.
func ValidateReferenceIdentifier(value string) error {
	normalized := strings.TrimSpace(value)
	if referenceIdentifierRegex.MatchString(normalized) {
		return nil
	}

	return fmt.Errorf("GRAMMAR-REFERENCE-PATTERN: REFERENCE must be a valid ReferenceIdentifier")
}
